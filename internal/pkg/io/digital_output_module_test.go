package io

import (
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeDigitalOutEstimator struct {
	estimated []float64
	result    float64
}

func (self *fakeDigitalOutEstimator) EstimatedPrintTime(eventtime float64) float64 {
	self.estimated = append(self.estimated, eventtime)
	return self.result
}

type fakeDigitalOutPin struct {
	estimator    printerpkg.PrintTimeEstimator
	maxDurations []float64
	setCalls     [][2]float64
}

func (self *fakeDigitalOutPin) MCU() printerpkg.PrintTimeEstimator {
	return self.estimator
}

func (self *fakeDigitalOutPin) SetupMaxDuration(maxDuration float64) {
	self.maxDurations = append(self.maxDurations, maxDuration)
}

func (self *fakeDigitalOutPin) SetDigital(printTime float64, value int) {
	self.setCalls = append(self.setCalls, [2]float64{printTime, float64(value)})
}

type fakeDigitalPinRegistry struct {
	pin         printerpkg.DigitalOutPin
	requestedOn []string
}

func (self *fakeDigitalPinRegistry) SetupDigitalOut(pin string) printerpkg.DigitalOutPin {
	self.requestedOn = append(self.requestedOn, pin)
	return self.pin
}

func (self *fakeDigitalPinRegistry) SetupADC(pin string) printerpkg.ADCPin {
	return nil
}

type fakeDigitalWebhookRequest struct {
	ints map[string]int
}

func (self *fakeDigitalWebhookRequest) String(name string, defaultValue string) string {
	return defaultValue
}

func (self *fakeDigitalWebhookRequest) Float(name string, defaultValue float64) float64 {
	if value, ok := self.ints[name]; ok {
		return float64(value)
	}
	return defaultValue
}

func (self *fakeDigitalWebhookRequest) Int(name string, defaultValue int) int {
	if value, ok := self.ints[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeDigitalWebhookRequest) GetParams() map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range self.ints {
		m[k] = v
	}
	return m
}

type fakeDigitalWebhookRegistry struct {
	paths              []string
	requestHandlers    map[string]func(printerpkg.WebhookRequest) (interface{}, error)
	requestRegisterErr error
}

func (self *fakeDigitalWebhookRegistry) RegisterEndpoint(path string, handler func() (interface{}, error)) error {
	self.paths = append(self.paths, path)
	return nil
}

func (self *fakeDigitalWebhookRegistry) RegisterEndpointWithRequest(path string, handler func(printerpkg.WebhookRequest) (interface{}, error)) error {
	self.paths = append(self.paths, path)
	if self.requestHandlers == nil {
		self.requestHandlers = map[string]func(printerpkg.WebhookRequest) (interface{}, error){}
	}
	self.requestHandlers[path] = handler
	return self.requestRegisterErr
}

type fakeDigitalReactor struct {
	monotonic float64
}

func (self *fakeDigitalReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return nil
}

func (self *fakeDigitalReactor) Monotonic() float64 {
	return self.monotonic
}

type fakeDigitalPrinter struct {
	lookup        map[string]interface{}
	reactor       printerpkg.ModuleReactor
	webhooks      printerpkg.WebhookRegistry
	eventHandlers map[string]func([]interface{}) error
}

func (self *fakeDigitalPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeDigitalPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeDigitalPrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeDigitalPrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeDigitalPrinter) AddObject(name string, obj interface{}) error { return nil }

func (self *fakeDigitalPrinter) LookupObjects(module string) []interface{} { return nil }

func (self *fakeDigitalPrinter) HasStartArg(name string) bool { return false }

func (self *fakeDigitalPrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeDigitalPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }

func (self *fakeDigitalPrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }

func (self *fakeDigitalPrinter) InvokeShutdown(msg string) {}

func (self *fakeDigitalPrinter) IsShutdown() bool { return false }

func (self *fakeDigitalPrinter) Reactor() printerpkg.ModuleReactor { return self.reactor }

func (self *fakeDigitalPrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeDigitalPrinter) GCode() printerpkg.GCodeRuntime { return nil }

func (self *fakeDigitalPrinter) GCodeMove() printerpkg.MoveTransformController { return nil }

func (self *fakeDigitalPrinter) Webhooks() printerpkg.WebhookRegistry { return self.webhooks }

type fakeDigitalConfig struct {
	printer printerpkg.ModulePrinter
	name    string
	strings map[string]string
	floats  map[string]float64
}

func (self *fakeDigitalConfig) Name() string { return self.name }

func (self *fakeDigitalConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeDigitalConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeDigitalConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeDigitalConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeDigitalConfig) LoadObject(section string) interface{} { return nil }

func (self *fakeDigitalConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeDigitalConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeDigitalConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigPrefixDigitalOutRegistersHooksAndDrivesPin(t *testing.T) {
	estimator := &fakeDigitalOutEstimator{result: 10.0}
	pin := &fakeDigitalOutPin{estimator: estimator}
	pins := &fakeDigitalPinRegistry{pin: pin}
	webhooks := &fakeDigitalWebhookRegistry{}
	printer := &fakeDigitalPrinter{
		lookup:   map[string]interface{}{"pins": pins},
		reactor:  &fakeDigitalReactor{monotonic: 2.0},
		webhooks: webhooks,
	}
	module := LoadConfigPrefixDigitalOut(&fakeDigitalConfig{
		printer: printer,
		name:    "output_pin power_pin",
		strings: map[string]string{"pin": "PB4"},
		floats:  map[string]float64{"value": 1, "shutdown_value": 0},
	}).(*DigitalOutputModule)

	if len(pins.requestedOn) != 1 || pins.requestedOn[0] != "PB4" {
		t.Fatalf("unexpected digital pin requests: %#v", pins.requestedOn)
	}
	if len(pin.maxDurations) != 1 || pin.maxDurations[0] != 0.0 {
		t.Fatalf("unexpected max duration calls: %#v", pin.maxDurations)
	}
	if _, ok := printer.eventHandlers["project:ready"]; !ok {
		t.Fatalf("expected ready handler registration")
	}
	if _, ok := printer.eventHandlers["project:shutdown"]; !ok {
		t.Fatalf("expected shutdown handler registration")
	}
	if _, ok := webhooks.requestHandlers["power/set_power_pin"]; !ok {
		t.Fatalf("expected power/set_power_pin webhook registration")
	}

	if err := printer.eventHandlers["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}
	if len(pin.setCalls) != 1 || pin.setCalls[0] != [2]float64{10.1, 1} {
		t.Fatalf("unexpected ready pin set calls: %#v", pin.setCalls)
	}
	if len(estimator.estimated) != 1 || estimator.estimated[0] != 2.0 {
		t.Fatalf("unexpected estimator calls: %#v", estimator.estimated)
	}

	if _, err := webhooks.requestHandlers["power/set_power_pin"](&fakeDigitalWebhookRequest{ints: map[string]int{"S": 0}}); err != nil {
		t.Fatalf("power pin webhook returned error: %v", err)
	}
	if len(pin.setCalls) != 2 || pin.setCalls[1] != [2]float64{10.1, 0} {
		t.Fatalf("unexpected power pin set calls: %#v", pin.setCalls)
	}
	status := module.Get_status(0)
	if status["value"] != 0 {
		t.Fatalf("unexpected module status after webhook: %#v", status)
	}

	if err := printer.eventHandlers["project:shutdown"](nil); err != nil {
		t.Fatalf("shutdown handler returned error: %v", err)
	}
	if len(pin.setCalls) != 3 || pin.setCalls[2] != [2]float64{10.1, 0} {
		t.Fatalf("unexpected shutdown pin set calls: %#v", pin.setCalls)
	}
}
