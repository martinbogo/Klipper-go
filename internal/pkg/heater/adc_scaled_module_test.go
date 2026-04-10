package heater

import "testing"

type fakeScaledADCPin struct {
	mcuRef          interface{}
	raw             interface{}
	callbackInvoked bool
}

func (self *fakeScaledADCPin) SetCallback(reportTime float64, callback func(float64, float64)) {
	if callback != nil {
		callback(1.5, 0.25)
		self.callbackInvoked = true
	}
}

func (self *fakeScaledADCPin) SetMinMax(sampleTime float64, sampleCount int, minval float64, maxval float64, rangeCheckCount int) {
}

func (self *fakeScaledADCPin) MCURef() interface{} {
	return self.mcuRef
}

func (self *fakeScaledADCPin) Raw() interface{} {
	if self.raw != nil {
		return self.raw
	}
	return self
}

func TestInitializeScaledADCModuleReturnsChipAndMCURef(t *testing.T) {
	mcuRef := &struct{}{}
	namedPins := map[string]*fakeScaledADCPin{}
	registered := []struct {
		name string
		adc  interface{}
	}{}
	module, err := InitializeScaledADCModule(
		"scaled",
		2.0,
		func(pinName string, callback func(float64, float64)) ADCPin {
			pin := &fakeScaledADCPin{mcuRef: mcuRef}
			namedPins[pinName] = pin
			callback(0.1, 0.2)
			return pin
		},
		func(pinParams map[string]interface{}) ADCPin {
			return &fakeScaledADCPin{mcuRef: mcuRef, raw: pinParams["pin"]}
		},
		func(name string, adc interface{}) {
			registered = append(registered, struct {
				name string
				adc  interface{}
			}{name: name, adc: adc})
		},
	)
	if err != nil {
		t.Fatalf("InitializeScaledADCModule returned error: %v", err)
	}
	if module.MCURef != mcuRef {
		t.Fatalf("expected mcu ref %p, got %p", mcuRef, module.MCURef)
	}
	if module.Chip == nil {
		t.Fatal("expected chip to be initialized")
	}
	if _, ok := namedPins["vref"]; !ok {
		t.Fatal("expected vref pin to be configured")
	}
	if _, ok := namedPins["vssa"]; !ok {
		t.Fatal("expected vssa pin to be configured")
	}
	reader := module.Chip.SetupReader(map[string]interface{}{"pin": "PA1"})
	if reader == nil {
		t.Fatal("expected reader to be created")
	}
	if len(registered) != 1 {
		t.Fatalf("expected one registered adc, got %d", len(registered))
	}
	if registered[0].name != "scaled:PA1" {
		t.Fatalf("expected registered name scaled:PA1, got %q", registered[0].name)
	}
	if registered[0].adc != "PA1" {
		t.Fatalf("expected raw adc payload to be forwarded, got %#v", registered[0].adc)
	}
}

func TestInitializeScaledADCModulePropagatesDifferentMCUs(t *testing.T) {
	vrefMCU := &struct{ id int }{id: 1}
	vssaMCU := &struct{ id int }{id: 2}
	_, err := InitializeScaledADCModule(
		"scaled",
		2.0,
		func(pinName string, callback func(float64, float64)) ADCPin {
			if pinName == "vref" {
				return &fakeScaledADCPin{mcuRef: vrefMCU, raw: pinName}
			}
			return &fakeScaledADCPin{mcuRef: vssaMCU, raw: pinName}
		},
		func(pinParams map[string]interface{}) ADCPin {
			return &fakeScaledADCPin{}
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected different-mcu error")
	}
	if err.Error() != (ErrDifferentMCUs{}).Error() {
		t.Fatalf("unexpected error %q", err.Error())
	}
}
