package heater

import (
	"math"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeADCModuleSensorRegistry struct {
	factories map[string]printerpkg.TemperatureSensorFactory
}

func (self *fakeADCModuleSensorRegistry) AddSensorFactory(sensorType string, factory printerpkg.TemperatureSensorFactory) {
	if self.factories == nil {
		self.factories = map[string]printerpkg.TemperatureSensorFactory{}
	}
	self.factories[sensorType] = factory
}

type fakeADCModulePin struct {
	callback      func(float64, float64)
	callbackTimes []float64
	minmaxCalls   [][5]float64
	lastValue     [2]float64
}

func (self *fakeADCModulePin) SetupCallback(reportTime float64, callback func(float64, float64)) {
	self.callbackTimes = append(self.callbackTimes, reportTime)
	self.callback = callback
}

func (self *fakeADCModulePin) SetupMinMax(sampleTime float64, sampleCount int, minval float64, maxval float64, rangeCheckCount int) {
	self.minmaxCalls = append(self.minmaxCalls, [5]float64{sampleTime, float64(sampleCount), minval, maxval, float64(rangeCheckCount)})
}

func (self *fakeADCModulePin) GetLastValue() [2]float64 {
	return self.lastValue
}

type fakeADCModulePinRegistry struct {
	adcPin    printerpkg.ADCPin
	requested []string
}

func (self *fakeADCModulePinRegistry) SetupDigitalOut(pin string) printerpkg.DigitalOutPin {
	return nil
}

func (self *fakeADCModulePinRegistry) SetupADC(pin string) printerpkg.ADCPin {
	self.requested = append(self.requested, pin)
	return self.adcPin
}

type fakeADCModuleQueryRegistry struct {
	names []string
	adcs  []printerpkg.ADCQueryReader
}

func (self *fakeADCModuleQueryRegistry) RegisterADC(name string, adc printerpkg.ADCQueryReader) {
	self.names = append(self.names, name)
	self.adcs = append(self.adcs, adc)
}

type fakeADCModulePrinter struct {
	registry printerpkg.TemperatureSensorRegistry
	lookup   map[string]interface{}
}

func (self *fakeADCModulePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeADCModulePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}

func (self *fakeADCModulePrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeADCModulePrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeADCModulePrinter) AddObject(name string, obj interface{}) error { return nil }

func (self *fakeADCModulePrinter) LookupObjects(module string) []interface{} { return nil }

func (self *fakeADCModulePrinter) HasStartArg(name string) bool { return false }

func (self *fakeADCModulePrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeADCModulePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return self.registry
}

func (self *fakeADCModulePrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }

func (self *fakeADCModulePrinter) InvokeShutdown(msg string) {}

func (self *fakeADCModulePrinter) IsShutdown() bool { return false }

func (self *fakeADCModulePrinter) Reactor() printerpkg.ModuleReactor { return nil }

func (self *fakeADCModulePrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeADCModulePrinter) GCode() printerpkg.GCodeRuntime { return nil }

func (self *fakeADCModulePrinter) GCodeMove() printerpkg.MoveTransformController { return nil }

func (self *fakeADCModulePrinter) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeADCModuleConfig struct {
	printer    printerpkg.ModulePrinter
	name       string
	strings    map[string]string
	floats     map[string]float64
	loadObject map[string]interface{}
	loaded     []string
}

func (self *fakeADCModuleConfig) Name() string { return self.name }

func (self *fakeADCModuleConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeADCModuleConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeADCModuleConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeADCModuleConfig) OptionalFloat(option string) *float64 {
	value, ok := self.floats[option]
	if !ok {
		return nil
	}
	return &value
}

func (self *fakeADCModuleConfig) LoadObject(section string) interface{} {
	self.loaded = append(self.loaded, section)
	if value, ok := self.loadObject[section]; ok {
		return value
	}
	return nil
}

func (self *fakeADCModuleConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeADCModuleConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeADCModuleConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigPrefixCustomLinearRegistersNamedFactoryAndBuildsADCSensor(t *testing.T) {
	registry := &fakeADCModuleSensorRegistry{}
	queryRegistry := &fakeADCModuleQueryRegistry{}
	pin := &fakeADCModulePin{}
	pins := &fakeADCModulePinRegistry{adcPin: pin}
	printer := &fakeADCModulePrinter{
		registry: registry,
		lookup:   map[string]interface{}{"pins": pins},
	}

	if got := LoadConfigPrefixCustomLinear(&fakeADCModuleConfig{
		printer: printer,
		name:    "customLinear chamber_sensor",
		floats: map[string]float64{
			"temperature1": 25.0,
			"resistance1":  1000.0,
			"temperature2": 100.0,
			"resistance2":  500.0,
		},
	}); got != nil {
		t.Fatalf("expected nil loader result, got %#v", got)
	}
	factory := registry.factories["chamber_sensor"]
	if factory == nil {
		t.Fatalf("expected customLinear factory to register chamber_sensor, got %#v", registry.factories)
	}

	sensor := factory(&fakeADCModuleConfig{
		printer: printer,
		name:    "heater_generic chamber",
		strings: map[string]string{"sensor_pin": "PA0"},
		floats: map[string]float64{
			"pullup_resistor": 4700.0,
		},
		loadObject: map[string]interface{}{"query_adc": queryRegistry},
	})
	adcSensor, ok := sensor.(*PrinterADCtoTemperature)
	if !ok {
		t.Fatalf("expected PrinterADCtoTemperature, got %T", sensor)
	}
	if len(pins.requested) != 1 || pins.requested[0] != "PA0" {
		t.Fatalf("unexpected ADC pin requests: %#v", pins.requested)
	}
	if len(queryRegistry.names) != 1 || queryRegistry.names[0] != "heater_generic chamber" {
		t.Fatalf("unexpected ADC query registrations: %#v", queryRegistry.names)
	}
	if len(pin.callbackTimes) != 1 || pin.callbackTimes[0] != ADCReportTime {
		t.Fatalf("unexpected ADC callback registration: %#v", pin.callbackTimes)
	}

	var callbackTime float64
	var callbackTemp float64
	adcSensor.SetupCallback(func(readTime float64, temp float64) {
		callbackTime = readTime
		callbackTemp = temp
	})
	linearResistance, err := NewLinearResistance(4700.0, [][]float64{{25.0, 1000.0}, {100.0, 500.0}}, "heater_generic chamber")
	if err != nil {
		t.Fatalf("failed to create test linear resistance: %v", err)
	}
	pin.callback(10.0, linearResistance.Calc_adc(100.0))
	if math.Abs(callbackTime-10.008) > 1e-9 {
		t.Fatalf("unexpected callback time: %v", callbackTime)
	}
	if math.Abs(callbackTemp-100.0) > 1e-9 {
		t.Fatalf("unexpected callback temp: %v", callbackTemp)
	}

	adcSensor.SetupMinMax(25.0, 100.0)
	if len(pin.minmaxCalls) != 1 {
		t.Fatalf("expected one minmax call, got %#v", pin.minmaxCalls)
	}
	call := pin.minmaxCalls[0]
	if call[0] != ADCSampleTime || call[1] != ADCSampleCount || call[4] != ADCRangeCheckCount {
		t.Fatalf("unexpected minmax metadata: %#v", call)
	}
	if !(call[2] < call[3]) {
		t.Fatalf("unexpected minmax bounds: %#v", call)
	}
	if adcSensor.GetReportTimeDelta() != ADCReportTime {
		t.Fatalf("unexpected report time delta: %v", adcSensor.GetReportTimeDelta())
	}
}

func TestLoadConfigThermistorRegistersFactoryAndBuildsADCSensor(t *testing.T) {
	registry := &fakeADCModuleSensorRegistry{}
	queryRegistry := &fakeADCModuleQueryRegistry{}
	pin := &fakeADCModulePin{}
	pins := &fakeADCModulePinRegistry{adcPin: pin}
	printer := &fakeADCModulePrinter{
		registry: registry,
		lookup:   map[string]interface{}{"pins": pins},
	}

	if got := LoadConfigThermistor(&fakeADCModuleConfig{
		printer: printer,
		name:    "thermistor NTC 100K Custom",
		floats: map[string]float64{
			"temperature1": 25.0,
			"resistance1":  100000.0,
			"beta":         3950.0,
		},
	}); got != nil {
		t.Fatalf("expected nil loader result, got %#v", got)
	}
	factory := registry.factories["NTC 100K Custom"]
	if factory == nil {
		t.Fatalf("expected thermistor factory to register custom sensor, got %#v", registry.factories)
	}

	sensor := factory(&fakeADCModuleConfig{
		printer: printer,
		name:    "heater_bed",
		strings: map[string]string{"sensor_pin": "PA1"},
		floats: map[string]float64{
			"pullup_resistor": 4700.0,
		},
		loadObject: map[string]interface{}{"query_adc": queryRegistry},
	})
	adcSensor, ok := sensor.(*PrinterADCtoTemperature)
	if !ok {
		t.Fatalf("expected PrinterADCtoTemperature, got %T", sensor)
	}
	if len(queryRegistry.names) != 1 || queryRegistry.names[0] != "heater_bed" {
		t.Fatalf("unexpected ADC query registrations: %#v", queryRegistry.names)
	}
	if len(pins.requested) != 1 || pins.requested[0] != "PA1" {
		t.Fatalf("unexpected ADC pin requests: %#v", pins.requested)
	}

	var callbackTemp float64
	adcSensor.SetupCallback(func(readTime float64, temp float64) {
		callbackTemp = temp
	})
	thermistor := NewThermistor(4700.0, 0.0)
	thermistor.Setup_coefficients_beta(25.0, 100000.0, 3950.0)
	pin.callback(2.0, thermistor.Calc_adc(25.0))
	if math.Abs(callbackTemp-25.0) > 0.01 {
		t.Fatalf("unexpected thermistor callback temp: %v", callbackTemp)
	}
}

func TestLoadConfigADCTemperatureRegistersDefaultSensors(t *testing.T) {
	registry := &fakeADCModuleSensorRegistry{}
	printer := &fakeADCModulePrinter{registry: registry}

	if got := LoadConfigADCTemperature(&fakeADCModuleConfig{printer: printer, name: "adc_temperature"}); got != nil {
		t.Fatalf("expected nil loader result, got %#v", got)
	}
	if registry.factories["AD595"] == nil {
		t.Fatalf("expected AD595 default sensor registration")
	}
	if registry.factories["PT1000"] == nil {
		t.Fatalf("expected PT1000 default sensor registration")
	}
}
