package tmc

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type UARTMutexCache struct {
	entries map[interface{}]interface{}
}

func NewUARTMutexCache() *UARTMutexCache {
	return &UARTMutexCache{entries: make(map[interface{}]interface{})}
}

func (self *UARTMutexCache) Lookup(key interface{}, factory func() interface{}) interface{} {
	if entry, ok := self.entries[key]; ok {
		return entry
	}
	entry := factory()
	self.entries[key] = entry
	return entry
}

type UARTSelectPin struct {
	Owner  interface{}
	Pin    interface{}
	Invert bool
}

type UARTMuxActivator interface {
	Activate(instanceID []int64)
}

type UARTMuxCommand interface {
	Send(data interface{}, minclock, reqclock int64)
}

type UARTMuxOwner interface {
	CreateOID() int
	AddConfigCmd(cmd string, isInit, onRestart bool)
	RegisterConfigCallback(func())
	LookupCommand(msgformat string, cmdQueue interface{}) (UARTMuxCommand, error)
}

type UARTMuxOwnerFuncs struct {
	CreateOIDFunc              func() int
	AddConfigCmdFunc           func(cmd string, isInit, onRestart bool)
	RegisterConfigCallbackFunc func(func())
	LookupCommandFunc          func(msgformat string, cmdQueue interface{}) (UARTMuxCommand, error)
}

func (funcs UARTMuxOwnerFuncs) CreateOID() int {
	return funcs.CreateOIDFunc()
}

func (funcs UARTMuxOwnerFuncs) AddConfigCmd(cmd string, isInit, onRestart bool) {
	funcs.AddConfigCmdFunc(cmd, isInit, onRestart)
}

func (funcs UARTMuxOwnerFuncs) RegisterConfigCallback(callback func()) {
	funcs.RegisterConfigCallbackFunc(callback)
}

func (funcs UARTMuxOwnerFuncs) LookupCommand(msgformat string, cmdQueue interface{}) (UARTMuxCommand, error) {
	return funcs.LookupCommandFunc(msgformat, cmdQueue)
}

type UARTAnalogMuxRuntime struct {
	owner        UARTMuxOwner
	cmdQueue     interface{}
	oids         []int
	pins         []interface{}
	pinValues    []int64
	updatePinCmd UARTMuxCommand
}

func NewUARTAnalogMuxRuntime(owner UARTMuxOwner, cmdQueue interface{}, pins []interface{}) *UARTAnalogMuxRuntime {
	runtime := &UARTAnalogMuxRuntime{
		owner:     owner,
		cmdQueue:  cmdQueue,
		oids:      make([]int, 0, len(pins)),
		pins:      append([]interface{}(nil), pins...),
		pinValues: make([]int64, 0, len(pins)),
	}
	for range pins {
		runtime.pinValues = append(runtime.pinValues, -1)
	}
	for i, pin := range runtime.pins {
		oid := runtime.owner.CreateOID()
		runtime.oids = append(runtime.oids, oid)
		runtime.owner.AddConfigCmd(
			fmt.Sprintf("config_digital_out oid=%d pin=%s value=0 default_value=0 max_duration=0", oid, pin),
			false,
			false,
		)
		_ = i
	}
	runtime.owner.RegisterConfigCallback(runtime.buildConfig)
	return runtime
}

func (runtime *UARTAnalogMuxRuntime) Pins() []interface{} {
	return append([]interface{}(nil), runtime.pins...)
}

func (runtime *UARTAnalogMuxRuntime) buildConfig() {
	runtime.updatePinCmd, _ = runtime.owner.LookupCommand("update_digital_out oid=%c value=%c", runtime.cmdQueue)
}

func (runtime *UARTAnalogMuxRuntime) Activate(instanceID []int64) {
	for i := 0; i < len(runtime.oids); i++ {
		oid, oldValue, newValue := runtime.oids[i], runtime.pinValues[i], instanceID[i]
		if oldValue != newValue {
			runtime.updatePinCmd.Send([]int64{int64(oid), newValue}, 0, 0)
		}
	}
	runtime.pinValues = append(runtime.pinValues[:0], instanceID...)
}

type UARTSharedResourceRuntime struct {
	rxPin     interface{}
	txPin     interface{}
	muxOwner  interface{}
	muxPins   []interface{}
	mux       UARTMuxActivator
	instances map[string]bool
}

func NewUARTSharedResourceRuntime(rxPin, txPin interface{}, muxOwner interface{}, muxPins []interface{}, mux UARTMuxActivator) *UARTSharedResourceRuntime {
	return &UARTSharedResourceRuntime{
		rxPin:     rxPin,
		txPin:     txPin,
		muxOwner:  muxOwner,
		muxPins:   append([]interface{}(nil), muxPins...),
		mux:       mux,
		instances: make(map[string]bool),
	}
}

func (self *UARTSharedResourceRuntime) RegisterInstance(rxPin, txPin interface{}, selectPins []UARTSelectPin, addr int) ([]int64, error) {
	if rxPin != self.rxPin || txPin != self.txPin || (len(selectPins) != 0) != (self.mux != nil) {
		return nil, fmt.Errorf(
			"Shared TMC uarts must use the same pins (got rx=%#v (%T) tx=%#v (%T) select_pins=%d mux=%t, existing rx=%#v (%T) tx=%#v (%T) select_pins=%d mux=%t)",
			rxPin,
			rxPin,
			txPin,
			txPin,
			len(selectPins),
			len(selectPins) != 0,
			self.rxPin,
			self.rxPin,
			self.txPin,
			self.txPin,
			len(self.muxPins),
			self.mux != nil,
		)
	}
	instanceID := make([]int64, 0, len(selectPins))
	if self.mux != nil {
		for _, selectPin := range selectPins {
			if selectPin.Owner != self.muxOwner {
				return nil, errors.New("TMC mux pins must be on the same mcu")
			}
			if !selectPin.Invert {
				instanceID = append(instanceID, 1)
			} else {
				instanceID = append(instanceID, 0)
			}
		}
		pins := make([]interface{}, 0, len(selectPins))
		for _, selectPin := range selectPins {
			pins = append(pins, selectPin.Pin)
		}
		if !reflect.DeepEqual(pins, self.muxPins) {
			return nil, errors.New("All TMC mux instances must use identical pins")
		}
	}
	key := uartSharedInstanceKey(instanceID, addr)
	if self.instances[key] {
		return nil, errors.New("Shared TMC uarts need unique address or select_pins polarity")
	}
	self.instances[key] = true
	return instanceID, nil
}

func (self *UARTSharedResourceRuntime) PrepareTransfer(instanceID []int64) {
	if self.mux == nil {
		return
	}
	self.mux.Activate(instanceID)
}

func uartSharedInstanceKey(instanceID []int64, addr int) string {
	parts := make([]string, 0, len(instanceID))
	for _, value := range instanceID {
		parts = append(parts, strconv.FormatInt(value, 10))
	}
	return strings.Join(parts, ",") + "###" + strconv.Itoa(addr)
}
