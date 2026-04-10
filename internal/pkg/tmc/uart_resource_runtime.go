package tmc

import (
	"errors"
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
		return nil, errors.New("Shared TMC uarts must use the same pins")
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