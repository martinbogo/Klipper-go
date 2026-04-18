package printer

import (
	"reflect"
	"testing"
)

func TestHeaterRuntimeAdapterDelegatesToWrappedHeater(t *testing.T) {
	source := struct{ name string }{name: "extruder"}
	adapter := NewHeaterRuntimeAdapter(source, func(eventtime float64) (float64, float64) {
		if eventtime != 12.5 {
			t.Fatalf("unexpected eventtime: %v", eventtime)
		}
		return 210.0, 215.0
	})

	current, target := adapter.GetTemperature(12.5)
	if current != 210.0 || target != 215.0 {
		t.Fatalf("unexpected temperatures: %v %v", current, target)
	}
	if adapter.Source() != source {
		t.Fatalf("unexpected source: %#v", adapter.Source())
	}
}

func TestHeaterManagerAdapterDelegates(t *testing.T) {
	type fakeConfig struct{}
	var lookedUp string
	var setupConfig ModuleConfig
	var setupID string
	var setHeater interface{}
	var setTemp float64
	var setWait bool

	adapter := NewHeaterManagerAdapter(HeaterManagerAdapterOptions{
		LookupHeater: func(name string) interface{} {
			lookedUp = name
			return "heater:bed"
		},
		SetupHeater: func(config ModuleConfig, gcodeID string) interface{} {
			setupConfig = config
			setupID = gcodeID
			return "setup"
		},
		SetTemperature: func(heater interface{}, temp float64, wait bool) error {
			setHeater = heater
			setTemp = temp
			setWait = wait
			return nil
		},
	})

	if got := adapter.LookupHeater("heater_bed"); got != "heater:bed" {
		t.Fatalf("unexpected heater lookup result: %#v", got)
	}
	if lookedUp != "heater_bed" {
		t.Fatalf("lookup not forwarded: %q", lookedUp)
	}
	if got := adapter.SetupHeater(nil, "B"); got != "setup" {
		t.Fatalf("unexpected setup result: %#v", got)
	}
	if setupConfig != nil || setupID != "B" {
		t.Fatalf("setup not forwarded: %#v %q", setupConfig, setupID)
	}
	if err := adapter.Set_temperature(&fakeConfig{}, 60.5, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := setHeater.(*fakeConfig); !ok || setTemp != 60.5 || !setWait {
		t.Fatalf("set temperature not forwarded: %#v %v %v", setHeater, setTemp, setWait)
	}
}

func TestTemperatureSensorRegistryAdapterDelegates(t *testing.T) {
	var sensorType string
	var factory TemperatureSensorFactory
	adapter := NewTemperatureSensorRegistryAdapter(func(argType string, argFactory TemperatureSensorFactory) {
		sensorType = argType
		factory = argFactory
	})

	wantFactory := TemperatureSensorFactory(func(ModuleConfig) TemperatureSensor { return nil })
	adapter.AddSensorFactory("thermistor", wantFactory)
	if sensorType != "thermistor" {
		t.Fatalf("unexpected sensor type: %q", sensorType)
	}
	if reflect.ValueOf(factory).Pointer() != reflect.ValueOf(wantFactory).Pointer() {
		t.Fatalf("unexpected factory forwarded")
	}
}

func TestPinRegistryRuntimeAdapterDelegates(t *testing.T) {
	var registeredName string
	var registeredChip interface{}
	var pwmPin string
	var doutPin string
	var adcPin string
	var lookupArgs struct {
		pinDesc   string
		canInvert bool
		canPullup bool
		shareType interface{}
	}

	adapter := NewPinRegistryRuntimeAdapter(PinRegistryRuntimeAdapterOptions{
		RegisterChip: func(name string, chip interface{}) {
			registeredName = name
			registeredChip = chip
		},
		SetupPWM: func(pin string) interface{} {
			pwmPin = pin
			return "pwm"
		},
		SetupDigitalOut: func(pin string) DigitalOutPin {
			doutPin = pin
			return nil
		},
		SetupADC: func(pin string) ADCPin {
			adcPin = pin
			return nil
		},
		LookupPin: func(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{} {
			lookupArgs.pinDesc = pinDesc
			lookupArgs.canInvert = canInvert
			lookupArgs.canPullup = canPullup
			lookupArgs.shareType = shareType
			return map[string]interface{}{"pin": pinDesc, "share": shareType}
		},
	})

	chip := struct{ id int }{id: 7}
	adapter.RegisterChip("probe", chip)
	if registeredName != "probe" || registeredChip != chip {
		t.Fatalf("register chip not forwarded: %q %#v", registeredName, registeredChip)
	}
	if got := adapter.SetupPWM("PA0"); got != "pwm" || pwmPin != "PA0" {
		t.Fatalf("setup pwm not forwarded: %#v %q", got, pwmPin)
	}
	adapter.SetupDigitalOut("PB1")
	adapter.SetupADC("PC2")
	if doutPin != "PB1" || adcPin != "PC2" {
		t.Fatalf("pin setup not forwarded: %q %q", doutPin, adcPin)
	}
	lookup := adapter.LookupPin("PD3", true, false, "stepper")
	if lookupArgs.pinDesc != "PD3" || !lookupArgs.canInvert || lookupArgs.canPullup || lookupArgs.shareType != "stepper" {
		t.Fatalf("lookup pin args not forwarded: %#v", lookupArgs)
	}
	if lookup["pin"] != "PD3" || lookup["share"] != "stepper" {
		t.Fatalf("lookup pin result not forwarded: %#v", lookup)
	}
}