package mcu

import (
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeOutputCommandSender struct {
	calls []fakeOutputCommandCall
}

type fakeOutputCommandCall struct {
	data     interface{}
	minclock int64
	reqclock int64
}

func (self *fakeOutputCommandSender) Send(data interface{}, minclock, reqclock int64) {
	self.calls = append(self.calls, fakeOutputCommandCall{data: data, minclock: minclock, reqclock: reqclock})
}

type fakePinControllerMCU struct{}

func (self *fakePinControllerMCU) EstimatedPrintTime(eventtime float64) float64 {
	return eventtime + 1
}

func (self *fakePinControllerMCU) CreateOID() int { return 0 }

func (self *fakePinControllerMCU) RegisterConfigCallback(func()) {}

func (self *fakePinControllerMCU) Request_move_queue_slot() {}

func (self *fakePinControllerMCU) AddConfigCmd(string, bool, bool) {}

func (self *fakePinControllerMCU) AllocCommandQueue() interface{} { return nil }

func (self *fakePinControllerMCU) LookupCommandRaw(string, interface{}) (interface{}, error) {
	return nil, nil
}

func (self *fakePinControllerMCU) SecondsToClock(time float64) int64 { return int64(time * 1000) }

func (self *fakePinControllerMCU) PrintTimeToClock(printTime float64) int64 {
	return int64(printTime * 1000)
}

func (self *fakePinControllerMCU) Get_constant_float(name string) float64 {
	if name == "_pwm_max" {
		return 255
	}
	if name == "ADC_MAX" {
		return 4095
	}
	return 0
}

func (self *fakePinControllerMCU) Monotonic() float64 { return 10 }

func (self *fakePinControllerMCU) GetQuerySlot(oid int) int64 { return int64(oid) }

func (self *fakePinControllerMCU) RegisterResponse(func(map[string]interface{}) error, string, interface{}) {
}

func (self *fakePinControllerMCU) Clock32ToClock64(clock32 int64) int64 { return clock32 }

func (self *fakePinControllerMCU) ClockToPrintTime(clock int64) float64 {
	return float64(clock) / 1000.0
}

func TestDigitalOutRuntimeStateSetupStartValue(t *testing.T) {
	state := &DigitalOutRuntimeState{Invert: 1}
	startValue, shutdownValue := state.SetupStartValue(1.0, 0.0)
	if startValue != 0 || shutdownValue != 1 {
		t.Fatalf("unexpected start/shutdown values %d %d", startValue, shutdownValue)
	}
}

func TestDigitalOutRuntimeStateSetDigital(t *testing.T) {
	state := &DigitalOutRuntimeState{Invert: 1, LastClock: 25}
	sender := &fakeOutputCommandSender{}
	state.SetDigital(2.5, 1, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, sender, 7)
	if state.LastClock != 2500 {
		t.Fatalf("unexpected last clock %d", state.LastClock)
	}
	if len(sender.calls) != 1 {
		t.Fatalf("expected one send call, got %d", len(sender.calls))
	}
	if sender.calls[0].minclock != 25 || sender.calls[0].reqclock != 2500 {
		t.Fatalf("unexpected send timing %#v", sender.calls[0])
	}
	args, ok := sender.calls[0].data.([]int64)
	if !ok || len(args) != 3 || args[0] != 7 || args[1] != 2500 || args[2] != 0 {
		t.Fatalf("unexpected send args %#v", sender.calls[0].data)
	}
}

func TestDigitalOutPinMCUReturnsEstimator(t *testing.T) {
	mcu := &fakePinControllerMCU{}
	pin := NewDigitalOutPin(mcu, map[string]interface{}{"pin": "PA0", "invert": 0})
	estimator := pin.MCU()
	if _, ok := estimator.(printerpkg.PrintTimeEstimator); !ok {
		t.Fatalf("digital out MCU has unexpected type %T", estimator)
	}
	if estimator.EstimatedPrintTime(2.5) != 3.5 {
		t.Fatalf("unexpected estimator value %v", estimator.EstimatedPrintTime(2.5))
	}
}

func TestPWMRuntimeStateSetupStartValue(t *testing.T) {
	state := &PWMRuntimeState{Invert: 1}
	startValue, shutdownValue := state.SetupStartValue(0.25, 1.5)
	if startValue != 0.75 || shutdownValue != 0.0 {
		t.Fatalf("unexpected start/shutdown values %f %f", startValue, shutdownValue)
	}
}

func TestPWMRuntimeStateSetPWM(t *testing.T) {
	state := &PWMRuntimeState{Invert: 0, PWMMax: 255, LastClock: 10}
	sender := &fakeOutputCommandSender{}
	state.SetPWM(1.25, 0.5, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, sender, 3)
	if state.LastClock != 1250 {
		t.Fatalf("unexpected last clock %d", state.LastClock)
	}
	if len(sender.calls) != 1 {
		t.Fatalf("expected one send call, got %d", len(sender.calls))
	}
	args, ok := sender.calls[0].data.([]int64)
	if !ok || len(args) != 3 || args[0] != 3 || args[1] != 1250 || args[2] != 128 {
		t.Fatalf("unexpected send args %#v", sender.calls[0].data)
	}
	if sender.calls[0].minclock != 10 || sender.calls[0].reqclock != 1250 {
		t.Fatalf("unexpected send timing %#v", sender.calls[0])
	}
}

func TestPWMRuntimeStateSetPWMInvertsAndClamps(t *testing.T) {
	state := &PWMRuntimeState{Invert: 1, PWMMax: 100, LastClock: 0}
	sender := &fakeOutputCommandSender{}
	state.SetPWM(1.0, -0.5, func(printTime float64) int64 {
		return int64(printTime * 100)
	}, sender, 2)
	args := sender.calls[0].data.([]int64)
	if args[2] != 100 {
		t.Fatalf("expected inverted clamped value 100, got %d", args[2])
	}
}

func TestPWMPinMCUReturnsController(t *testing.T) {
	mcu := &fakePinControllerMCU{}
	pin := NewPWMPin(mcu, map[string]interface{}{"pin": "PA1", "invert": 0})
	if pin.MCU() != mcu {
		t.Fatalf("unexpected pwm MCU reference %#v", pin.MCU())
	}
}
