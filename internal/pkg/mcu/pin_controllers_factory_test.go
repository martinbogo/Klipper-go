package mcu

import "testing"

type fakeLegacyPinController struct {
	configCallbacks []func()
	nextOID         int
}

func (self *fakeLegacyPinController) CreateOID() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeLegacyPinController) RegisterConfigCallback(cb func()) {
	self.configCallbacks = append(self.configCallbacks, cb)
}

func (self *fakeLegacyPinController) Request_move_queue_slot() {}

func (self *fakeLegacyPinController) AddConfigCmd(cmd string, isInit bool, onRestart bool) {
	_, _, _ = cmd, isInit, onRestart
}

func (self *fakeLegacyPinController) AllocCommandQueue() interface{} { return nil }

func (self *fakeLegacyPinController) LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error) {
	_, _ = msgformat, cq
	return nil, nil
}

func (self *fakeLegacyPinController) SecondsToClock(time float64) int64 { return int64(time * 1000) }

func (self *fakeLegacyPinController) PrintTimeToClock(printTime float64) int64 {
	return int64(printTime * 1000)
}

func (self *fakeLegacyPinController) EstimatedPrintTime(eventtime float64) float64 { return eventtime }

func (self *fakeLegacyPinController) Get_constant_float(name string) float64 {
	if name == "ADC_MAX" {
		return 4095
	}
	if name == "_pwm_max" {
		return 255
	}
	return 0
}

func (self *fakeLegacyPinController) Monotonic() float64 { return 0 }

func (self *fakeLegacyPinController) RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{}) {
	_, _, _ = cb, msg, oid
}

func (self *fakeLegacyPinController) GetQuerySlot(oid int) int64 { return int64(oid) }

func (self *fakeLegacyPinController) Clock32ToClock64(clock32 int64) int64 { return clock32 }

func (self *fakeLegacyPinController) ClockToPrintTime(clock int64) float64 {
	return float64(clock) / 1000.0
}

func (self *fakeLegacyPinController) MCU() printerPrintTimeEstimatorStub { return printerPrintTimeEstimatorStub{} }

type printerPrintTimeEstimatorStub struct{}

func (printerPrintTimeEstimatorStub) EstimatedPrintTime(eventtime float64) float64 { return eventtime }

func TestSetupLegacyControllerPinDispatchesKnownPinTypes(t *testing.T) {
	controller := &fakeLegacyPinController{}
	endstopCalls := 0
	endstopPin := SetupLegacyControllerPin(controller, "endstop", map[string]interface{}{"pin": "PA1"}, func(pinParams map[string]interface{}) interface{} {
		endstopCalls++
		if pinParams["pin"] != "PA1" {
			t.Fatalf("unexpected endstop pin params %#v", pinParams)
		}
		return "endstop-pin"
	})
	if endstopPin != "endstop-pin" || endstopCalls != 1 {
		t.Fatalf("expected endstop factory result, got %#v / %d", endstopPin, endstopCalls)
	}

	digital := SetupLegacyControllerPin(controller, "digital_out", map[string]interface{}{"pin": "PA0", "invert": 0}, nil)
	if _, ok := digital.(*DigitalOutPin); !ok {
		t.Fatalf("expected digital out pin, got %T", digital)
	}
	if len(controller.configCallbacks) != 1 {
		t.Fatalf("expected one config callback registration, got %d", len(controller.configCallbacks))
	}

	if unknown := SetupLegacyControllerPin(controller, "mystery", map[string]interface{}{}, nil); unknown != nil {
		t.Fatalf("expected nil for unknown pin type, got %#v", unknown)
	}
}