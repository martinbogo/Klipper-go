package tmc

import (
	"math"
	"reflect"
	"testing"
)

type fakeUARTBitbangQueryCall struct {
	data     interface{}
	minclock int64
	reqclock int64
}

type fakeUARTBitbangQuery struct {
	response interface{}
	sends    []fakeUARTBitbangQueryCall
}

func (self *fakeUARTBitbangQuery) Send(data interface{}, minclock, reqclock int64) interface{} {
	self.sends = append(self.sends, fakeUARTBitbangQueryCall{data: data, minclock: minclock, reqclock: reqclock})
	return self.response
}

type fakeUARTBitbangOwner struct {
	nextOID       int
	configCmds    []string
	callbacks     []func()
	muxCommand    *fakeUARTMuxCommand
	queryCommand  *fakeUARTBitbangQuery
	secondsInputs []float64
	printTimes    []float64
	mcuType       string
}

func (self *fakeUARTBitbangOwner) CreateOID() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeUARTBitbangOwner) AllocCommandQueue() interface{} {
	return "queue"
}

func (self *fakeUARTBitbangOwner) AddConfigCmd(cmd string, isInit, onRestart bool) {
	self.configCmds = append(self.configCmds, cmd)
	_ = isInit
	_ = onRestart
}

func (self *fakeUARTBitbangOwner) RegisterConfigCallback(callback func()) {
	self.callbacks = append(self.callbacks, callback)
}

func (self *fakeUARTBitbangOwner) LookupCommand(msgformat string, cmdQueue interface{}) (UARTMuxCommand, error) {
	_ = msgformat
	_ = cmdQueue
	return self.muxCommand, nil
}

func (self *fakeUARTBitbangOwner) LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) UARTBitbangQuery {
	_ = msgformat
	_ = respformat
	_ = oid
	_ = cmdQueue
	_ = isAsync
	return self.queryCommand
}

func (self *fakeUARTBitbangOwner) SecondsToClock(seconds float64) int64 {
	self.secondsInputs = append(self.secondsInputs, seconds)
	return 1234
}

func (self *fakeUARTBitbangOwner) PrintTimeToClock(printTime float64) int64 {
	self.printTimes = append(self.printTimes, printTime)
	return 5678
}

func (self *fakeUARTBitbangOwner) MCUType() string {
	return self.mcuType
}

func (self *fakeUARTBitbangOwner) runCallbacks() {
	for _, callback := range self.callbacks {
		callback()
	}
}

func TestUARTBitbangBaudForMCUType(t *testing.T) {
	if got := UARTBitbangBaudForMCUType("atmega2560"); got != UARTBitbangBaudRateAVR {
		t.Fatalf("expected AVR baud rate, got %v", got)
	}
	if got := UARTBitbangBaudForMCUType("at90usb1286"); got != UARTBitbangBaudRateAVR {
		t.Fatalf("expected AVR usb baud rate, got %v", got)
	}
	if got := UARTBitbangBaudForMCUType("stm32f103"); got != UARTBitbangBaudRate {
		t.Fatalf("expected default baud rate, got %v", got)
	}
}

func TestUARTBitbangRuntimeBuildsConfigAndTransfersRegisters(t *testing.T) {
	query := &fakeUARTBitbangQuery{}
	owner := &fakeUARTBitbangOwner{
		nextOID:      11,
		muxCommand:   &fakeUARTMuxCommand{},
		queryCommand: query,
		mcuType:      "atmega2560",
	}
	value := int64(0x12345678)
	encoded := EncodeUARTWrite(0x05, 0xff, 0x22, value)
	read := make([]int, 0, len(encoded))
	for _, part := range encoded {
		read = append(read, int(part))
	}
	query.response = map[string]interface{}{"read": read}

	runtime := NewUARTBitbangRuntime(owner, "mcu0", 0, "rx0", "tx0", []interface{}{"sel0"})
	if len(owner.callbacks) != 2 {
		t.Fatalf("expected two config callbacks, got %d", len(owner.callbacks))
	}
	owner.runCallbacks()

	if len(owner.configCmds) != 2 {
		t.Fatalf("expected mux and uart config commands, got %d", len(owner.configCmds))
	}
	if want := "config_tmcuart oid=11 rx_pin=rx0 pull_up=0 tx_pin=tx0 bit_time=1234"; owner.configCmds[1] != want {
		t.Fatalf("unexpected uart config command %q", owner.configCmds[1])
	}
	if len(owner.secondsInputs) != 1 || math.Abs(owner.secondsInputs[0]-(1.0/UARTBitbangBaudRateAVR)) > 1e-12 {
		t.Fatalf("unexpected seconds-to-clock inputs %#v", owner.secondsInputs)
	}

	instanceID, err := runtime.RegisterInstance("rx0", "tx0", []UARTSelectPin{{Owner: "mcu0", Pin: "sel0", Invert: false}}, 1)
	if err != nil {
		t.Fatalf("RegisterInstance returned error: %v", err)
	}
	if !reflect.DeepEqual(instanceID, []int64{1}) {
		t.Fatalf("unexpected instance id %#v", instanceID)
	}

	got, ok := runtime.ReadRegister(instanceID, 1, 0x22)
	if !ok || got != value {
		t.Fatalf("unexpected read result value=%d ok=%v", got, ok)
	}
	if len(owner.muxCommand.sends) != 1 || !reflect.DeepEqual(owner.muxCommand.sends[0], []int64{12, 1}) {
		t.Fatalf("unexpected mux sends %#v", owner.muxCommand.sends)
	}
	if len(query.sends) != 1 {
		t.Fatalf("expected one UART query send, got %d", len(query.sends))
	}
	firstSend := query.sends[0]
	if firstSend.minclock != 0 || firstSend.reqclock != 0 {
		t.Fatalf("unexpected read clocks %#v", firstSend)
	}
	firstPayload := firstSend.data.([]interface{})
	if firstPayload[0] != int64(11) || firstPayload[2] != int64(10) {
		t.Fatalf("unexpected read payload header %#v", firstPayload)
	}
	if got := firstPayload[1].([]int64); !reflect.DeepEqual(got, EncodeUARTRead(0xf5, 1, 0x22)) {
		t.Fatalf("unexpected read payload %#v", got)
	}

	printTime := 2.5
	runtime.WriteRegister(instanceID, 1, 0x22, value, &printTime)
	if len(owner.printTimes) != 1 || owner.printTimes[0] != printTime {
		t.Fatalf("unexpected print-time conversions %#v", owner.printTimes)
	}
	if len(owner.muxCommand.sends) != 1 {
		t.Fatalf("expected mux state reuse, got %#v", owner.muxCommand.sends)
	}
	if len(query.sends) != 2 {
		t.Fatalf("expected one read and one write send, got %d", len(query.sends))
	}
	secondSend := query.sends[1]
	if secondSend.minclock != 5678 || secondSend.reqclock != 0 {
		t.Fatalf("unexpected write clocks %#v", secondSend)
	}
	secondPayload := secondSend.data.([]interface{})
	if secondPayload[0] != int64(11) || secondPayload[2] != int64(0) {
		t.Fatalf("unexpected write payload header %#v", secondPayload)
	}
	if got := secondPayload[1].([]int64); !reflect.DeepEqual(got, EncodeUARTWrite(0xf5, 1, 0x22|0x80, value)) {
		t.Fatalf("unexpected write payload %#v", got)
	}
}

func TestUARTBitbangRuntimeRegistersWithoutSelectPins(t *testing.T) {
	owner := &fakeUARTBitbangOwner{
		nextOID:      3,
		queryCommand: &fakeUARTBitbangQuery{},
		mcuType:      "stm32f103",
	}

	runtime := NewUARTBitbangRuntime(owner, "mcu0", 0, "rx0", "tx0", nil)
	instanceID, err := runtime.RegisterInstance("rx0", "tx0", nil, 1)
	if err != nil {
		t.Fatalf("RegisterInstance returned error: %v", err)
	}
	if len(instanceID) != 0 {
		t.Fatalf("expected empty instance id without select pins, got %#v", instanceID)
	}

	secondInstanceID, err := runtime.RegisterInstance("rx0", "tx0", nil, 2)
	if err != nil {
		t.Fatalf("second RegisterInstance returned error: %v", err)
	}
	if len(secondInstanceID) != 0 {
		t.Fatalf("expected empty second instance id without select pins, got %#v", secondInstanceID)
	}
}
