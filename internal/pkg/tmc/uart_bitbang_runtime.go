package tmc

import (
	"fmt"
	"goklipper/common/utils/cast"
	"strings"
)

const (
	UARTBitbangBaudRate    float64 = 40000
	UARTBitbangBaudRateAVR float64 = 9000
)

func UARTBitbangBaudForMCUType(mcuType string) float64 {
	if strings.HasPrefix(mcuType, "atmega") || strings.HasPrefix(mcuType, "at90usb") {
		return UARTBitbangBaudRateAVR
	}
	return UARTBitbangBaudRate
}

type UARTBitbangQuery interface {
	Send(data interface{}, minclock, reqclock int64) interface{}
}

type UARTBitbangQueryFuncs struct {
	SendFunc func(data interface{}, minclock, reqclock int64) interface{}
}

func (funcs UARTBitbangQueryFuncs) Send(data interface{}, minclock, reqclock int64) interface{} {
	return funcs.SendFunc(data, minclock, reqclock)
}

type UARTBitbangOwner interface {
	UARTMuxOwner
	AllocCommandQueue() interface{}
	LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) UARTBitbangQuery
	SecondsToClock(seconds float64) int64
	PrintTimeToClock(printTime float64) int64
	MCUType() string
}

type UARTBitbangOwnerFuncs struct {
	CreateOIDFunc              func() int
	AllocCommandQueueFunc      func() interface{}
	AddConfigCmdFunc           func(cmd string, isInit, onRestart bool)
	RegisterConfigCallbackFunc func(func())
	LookupCommandFunc          func(msgformat string, cmdQueue interface{}) (UARTMuxCommand, error)
	LookupQueryCommandFunc     func(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) UARTBitbangQuery
	SecondsToClockFunc         func(seconds float64) int64
	PrintTimeToClockFunc       func(printTime float64) int64
	MCUTypeFunc                func() string
}

func (funcs UARTBitbangOwnerFuncs) CreateOID() int {
	return funcs.CreateOIDFunc()
}

func (funcs UARTBitbangOwnerFuncs) AllocCommandQueue() interface{} {
	return funcs.AllocCommandQueueFunc()
}

func (funcs UARTBitbangOwnerFuncs) AddConfigCmd(cmd string, isInit, onRestart bool) {
	funcs.AddConfigCmdFunc(cmd, isInit, onRestart)
}

func (funcs UARTBitbangOwnerFuncs) RegisterConfigCallback(callback func()) {
	funcs.RegisterConfigCallbackFunc(callback)
}

func (funcs UARTBitbangOwnerFuncs) LookupCommand(msgformat string, cmdQueue interface{}) (UARTMuxCommand, error) {
	return funcs.LookupCommandFunc(msgformat, cmdQueue)
}

func (funcs UARTBitbangOwnerFuncs) LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) UARTBitbangQuery {
	return funcs.LookupQueryCommandFunc(msgformat, respformat, oid, cmdQueue, isAsync)
}

func (funcs UARTBitbangOwnerFuncs) SecondsToClock(seconds float64) int64 {
	return funcs.SecondsToClockFunc(seconds)
}

func (funcs UARTBitbangOwnerFuncs) PrintTimeToClock(printTime float64) int64 {
	return funcs.PrintTimeToClockFunc(printTime)
}

func (funcs UARTBitbangOwnerFuncs) MCUType() string {
	return funcs.MCUTypeFunc()
}

type UARTBitbangRuntime struct {
	owner     UARTBitbangOwner
	pullUp    interface{}
	rxPin     interface{}
	txPin     interface{}
	oid       int
	cmdQueue  interface{}
	analogMux *UARTAnalogMuxRuntime
	resources *UARTSharedResourceRuntime
	sendCmd   UARTBitbangQuery
}

func NewUARTBitbangRuntime(owner UARTBitbangOwner, muxOwner interface{}, pullUp interface{}, rxPin, txPin interface{}, muxPins []interface{}) *UARTBitbangRuntime {
	runtime := &UARTBitbangRuntime{
		owner:    owner,
		pullUp:   pullUp,
		rxPin:    rxPin,
		txPin:    txPin,
		oid:      owner.CreateOID(),
		cmdQueue: owner.AllocCommandQueue(),
	}
	if len(muxPins) != 0 {
		runtime.analogMux = NewUARTAnalogMuxRuntime(owner, runtime.cmdQueue, muxPins)
	}
	sharedMuxPins := []interface{}(nil)
	if runtime.analogMux != nil {
		sharedMuxPins = runtime.analogMux.Pins()
	}
	var sharedMux UARTMuxActivator
	if runtime.analogMux != nil {
		sharedMux = runtime.analogMux
	}
	runtime.resources = NewUARTSharedResourceRuntime(rxPin, txPin, muxOwner, sharedMuxPins, sharedMux)
	owner.RegisterConfigCallback(runtime.BuildConfig)
	return runtime
}

func (runtime *UARTBitbangRuntime) BuildConfig() {
	baud := UARTBitbangBaudForMCUType(runtime.owner.MCUType())
	bitTicks := runtime.owner.SecondsToClock(1. / baud)
	runtime.owner.AddConfigCmd(
		fmt.Sprintf("config_tmcuart oid=%d rx_pin=%s pull_up=%d tx_pin=%s bit_time=%d", runtime.oid, runtime.rxPin, cast.ToInt(runtime.pullUp), runtime.txPin, bitTicks),
		false,
		false,
	)
	runtime.sendCmd = runtime.owner.LookupQueryCommand(
		"tmcuart_send oid=%c write=%*s read=%c",
		"tmcuart_response oid=%c read=%*s",
		runtime.oid,
		runtime.cmdQueue,
		true,
	)
}

func (runtime *UARTBitbangRuntime) RegisterInstance(rxPin, txPin interface{}, selectPins []UARTSelectPin, addr int) ([]int64, error) {
	return runtime.resources.RegisterInstance(rxPin, txPin, selectPins, addr)
}

func (runtime *UARTBitbangRuntime) ReadRegister(instanceID []int64, addr, reg int64) (int64, bool) {
	runtime.resources.PrepareTransfer(instanceID)
	payload := EncodeUARTRead(0xf5, addr, reg)
	params := runtime.sendCmd.Send([]interface{}{int64(runtime.oid), payload, int64(10)}, 0, 0)
	rawRead := uartReadResponseValues(params.(map[string]interface{})["read"])
	val, ok := DecodeUARTRead(reg, rawRead)
	return val, ok
}

func (runtime *UARTBitbangRuntime) WriteRegister(instanceID []int64, addr, reg, val int64, printTime *float64) {
	minclock := int64(0)
	if printTime != nil {
		minclock = runtime.owner.PrintTimeToClock(*printTime)
	}
	runtime.resources.PrepareTransfer(instanceID)
	writeReg := reg | 0x80
	payload := EncodeUARTWrite(0xf5, addr, writeReg, val)
	runtime.sendCmd.Send([]interface{}{int64(runtime.oid), payload, int64(0)}, minclock, 0)
}

func uartReadResponseValues(raw interface{}) []int64 {
	switch values := raw.(type) {
	case []int:
		result := make([]int64, 0, len(values))
		for _, value := range values {
			result = append(result, int64(value))
		}
		return result
	case []int64:
		return append([]int64(nil), values...)
	case []interface{}:
		result := make([]int64, 0, len(values))
		for _, value := range values {
			switch typed := value.(type) {
			case int:
				result = append(result, int64(typed))
			case int64:
				result = append(result, typed)
			default:
				panic(fmt.Sprintf("unexpected tmcuart read element %T", value))
			}
		}
		return result
	default:
		panic(fmt.Sprintf("unexpected tmcuart read payload %T", raw))
	}
}
