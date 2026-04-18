package io

import (
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeTrackerButtons struct {
	registerPins [][]string
	callback     func(float64, int)
}

func (self *fakeTrackerButtons) Register_buttons(pins []string, callback func(float64, int)) {
	copyPins := append([]string{}, pins...)
	self.registerPins = append(self.registerPins, copyPins)
	self.callback = callback
}

type fakeTrackerConfig struct {
	printer       printerpkg.ModulePrinter
	strings       map[string]string
	floats        map[string]float64
	loadedObjects []string
	objects       map[string]interface{}
}

func (self *fakeTrackerConfig) Name() string { return "filament_tracker" }

func (self *fakeTrackerConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeTrackerConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeTrackerConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeTrackerConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeTrackerConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	return self.objects[section]
}

func (self *fakeTrackerConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeTrackerConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeTrackerConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigFilamentTrackerGPIOUsesButtonsAndCallbacks(t *testing.T) {
	buttons := &fakeTrackerButtons{}
	reactor := &fakeButtonsReactor{monotonic: 6.0}
	printer := &fakeButtonsPrinter{
		lookup:  map[string]interface{}{},
		mcus:    map[string]printerpkg.MCURuntime{},
		reactor: reactor,
	}
	config := &fakeTrackerConfig{
		printer: printer,
		strings: map[string]string{
			"tracker_break_pin": "PA1",
			"tracker_block_pin": "PA2",
			"signal_type":       "gpio",
		},
		floats: map[string]float64{
			"safe_unwind_len":  120.0,
			"length_per_pulse": 1.5,
		},
		objects: map[string]interface{}{"buttons": buttons},
	}

	module := LoadConfigFilamentTracker(config).(*FilamentTrackerModule)
	if module == nil {
		t.Fatalf("expected filament tracker module instance")
	}
	if !reflect.DeepEqual(config.loadedObjects, []string{"buttons"}) {
		t.Fatalf("unexpected load object calls: %#v", config.loadedObjects)
	}
	if len(buttons.registerPins) != 1 || !reflect.DeepEqual(buttons.registerPins[0], []string{"PA1", "PA2"}) {
		t.Fatalf("unexpected button registrations: %#v", buttons.registerPins)
	}

	events := []int{}
	times := []float64{}
	module.Register_callback(func(eventtime float64, state int) {
		times = append(times, eventtime)
		events = append(events, state)
	})
	buttons.callback(3.5, 1)
	if !reflect.DeepEqual(events, []int{1}) || !reflect.DeepEqual(times, []float64{3.5}) {
		t.Fatalf("unexpected gpio callback events: times=%#v events=%#v", times, events)
	}
	if module.filamentPresent != 1 {
		t.Fatalf("expected gpio path to update internal filamentPresent, got %d", module.filamentPresent)
	}
	if module.Get_safe_unwind_len() != 120 {
		t.Fatalf("unexpected safe unwind len: %d", module.Get_safe_unwind_len())
	}
	if status := module.Get_status(0); status["filament_present"].(int) != 0 {
		t.Fatalf("unexpected tracker status for gpio path: %#v", status)
	}
}

func TestLoadConfigFilamentTrackerADCConfiguresPinsAndUpdatesState(t *testing.T) {
	pins := &fakeButtonsPinRegistry{}
	reactor := &fakeButtonsReactor{monotonic: 5.0}
	printer := &fakeButtonsPrinter{
		lookup:  map[string]interface{}{"pins": pins},
		mcus:    map[string]printerpkg.MCURuntime{},
		reactor: reactor,
	}
	config := &fakeTrackerConfig{
		printer: printer,
		strings: map[string]string{
			"tracker_break_pin": "ADC0",
			"tracker_block_pin": "ADC1",
			"signal_type":       "adc",
		},
		floats: map[string]float64{
			"safe_unwind_len": 100.0,
		},
		objects: map[string]interface{}{},
	}

	module := LoadConfigFilamentTracker(config).(*FilamentTrackerModule)
	if module == nil {
		t.Fatalf("expected filament tracker module instance")
	}
	if !reflect.DeepEqual(pins.adcSetupPins, []string{"ADC0", "ADC1"}) {
		t.Fatalf("unexpected adc setup pins: %#v", pins.adcSetupPins)
	}
	breakADC := pins.adcPins["ADC0"]
	blockADC := pins.adcPins["ADC1"]
	if breakADC == nil || blockADC == nil {
		t.Fatalf("expected both adc pins to be configured")
	}
	if len(breakADC.reportTimes) != 1 || breakADC.reportTimes[0] != filamentTrackerADCReportTime {
		t.Fatalf("unexpected break adc report times: %#v", breakADC.reportTimes)
	}
	if len(blockADC.reportTimes) != 1 || blockADC.reportTimes[0] != filamentTrackerADCReportTime {
		t.Fatalf("unexpected block adc report times: %#v", blockADC.reportTimes)
	}
	if len(breakADC.minmaxCalls) != 1 || breakADC.minmaxCalls[0] != [5]float64{filamentTrackerADCSampleTime, filamentTrackerADCSampleCount, 0, 1, 0} {
		t.Fatalf("unexpected break adc minmax: %#v", breakADC.minmaxCalls)
	}
	if len(blockADC.minmaxCalls) != 1 || blockADC.minmaxCalls[0] != [5]float64{filamentTrackerADCSampleTime, filamentTrackerADCSampleCount, 0, 1, 0} {
		t.Fatalf("unexpected block adc minmax: %#v", blockADC.minmaxCalls)
	}

	events := []int{}
	times := []float64{}
	module.Register_callback(func(eventtime float64, state int) {
		times = append(times, eventtime)
		events = append(events, state)
	})
	blockADC.callback(1.0, 0.8)
	breakADC.callback(1.2, 0.6)
	blockADC.callback(1.4, 0.6)
	if !reflect.DeepEqual(events, []int{1}) || !reflect.DeepEqual(times, []float64{5.0}) {
		t.Fatalf("unexpected adc callback events: times=%#v events=%#v", times, events)
	}
	status := module.Get_status(0)
	if status["filament_present"].(int) != 1 {
		t.Fatalf("unexpected filament_present in status: %#v", status)
	}
	if status["encoder_pulse"].(int) != 2 {
		t.Fatalf("unexpected encoder pulse count: %#v", status)
	}
	if status["encoder_signal_state"].(int) != 0 {
		t.Fatalf("unexpected encoder signal state: %#v", status)
	}
	if !module.Is_filament_present() {
		t.Fatalf("expected adc path to mark filament present")
	}
}

func TestLoadConfigFilamentTrackerADCAcceptsLegacyDetectEncoderPins(t *testing.T) {
	pins := &fakeButtonsPinRegistry{}
	reactor := &fakeButtonsReactor{monotonic: 5.0}
	printer := &fakeButtonsPrinter{
		lookup:  map[string]interface{}{"pins": pins},
		mcus:    map[string]printerpkg.MCURuntime{},
		reactor: reactor,
	}
	config := &fakeTrackerConfig{
		printer: printer,
		strings: map[string]string{
			"tracker_detect_pin":  "PB0",
			"tracker_encoder_pin": "PB1",
			"signal_type":         "adc",
		},
		floats:  map[string]float64{},
		objects: map[string]interface{}{},
	}

	module := LoadConfigFilamentTracker(config).(*FilamentTrackerModule)
	if module == nil {
		t.Fatalf("expected filament tracker module instance")
	}
	if !reflect.DeepEqual(pins.adcSetupPins, []string{"PB0", "PB1"}) {
		t.Fatalf("unexpected adc setup pins for legacy names: %#v", pins.adcSetupPins)
	}
	if module.detectADC == nil || module.encoderADC == nil {
		t.Fatalf("expected adc pins to be configured for legacy names")
	}
	if _, ok := pins.adcPins["PB0"]; !ok {
		t.Fatalf("expected detect adc pin registration for legacy name")
	}
	if _, ok := pins.adcPins["PB1"]; !ok {
		t.Fatalf("expected encoder adc pin registration for legacy name")
	}
	if module.Get_safe_unwind_len() != 100 {
		t.Fatalf("unexpected default safe unwind len: %d", module.Get_safe_unwind_len())
	}
	if module.Get_status(0)["filament_present"].(int) != 0 {
		t.Fatalf("unexpected initial tracker status for legacy names: %#v", module.Get_status(0))
	}
}
