package fan

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeControllerFanStepperLine struct {
	enabled bool
}

func (self *fakeControllerFanStepperLine) MotorEnable(printTime float64)  {}
func (self *fakeControllerFanStepperLine) MotorDisable(printTime float64) {}
func (self *fakeControllerFanStepperLine) IsMotorEnabled() bool {
	return self.enabled
}

type fakeControllerFanStepperEnable struct {
	names       []string
	lines       map[string]printerpkg.StepperEnableLine
	lookupCalls []string
}

func (self *fakeControllerFanStepperEnable) LookupEnable(name string) (printerpkg.StepperEnableLine, error) {
	self.lookupCalls = append(self.lookupCalls, name)
	line, ok := self.lines[name]
	if !ok {
		return nil, fmt.Errorf("Unknown stepper '%s'", name)
	}
	return line, nil
}

func (self *fakeControllerFanStepperEnable) StepperNames() []string {
	return append([]string{}, self.names...)
}

type fakeControllerFanTimerHandle struct{}

func (self *fakeControllerFanTimerHandle) Update(waketime float64) {}

type fakeControllerFanReactor struct {
	monotonic     float64
	timerCallback func(float64) float64
	timerWake     float64
}

func (self *fakeControllerFanReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	self.timerCallback = callback
	self.timerWake = waketime
	return &fakeControllerFanTimerHandle{}
}

func (self *fakeControllerFanReactor) Monotonic() float64 {
	return self.monotonic
}

type fakeControllerFanPrinter struct {
	lookup        map[string]interface{}
	heaters       map[string]printerpkg.HeaterRuntime
	reactor       printerpkg.ModuleReactor
	stepperEnable printerpkg.StepperEnableRuntime
	eventHandlers map[string]func([]interface{}) error
}

func (self *fakeControllerFanPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeControllerFanPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeControllerFanPrinter) SendEvent(event string, params []interface{}) {}
func (self *fakeControllerFanPrinter) CurrentExtruderName() string                  { return "extruder" }
func (self *fakeControllerFanPrinter) AddObject(name string, obj interface{}) error { return nil }
func (self *fakeControllerFanPrinter) LookupObjects(module string) []interface{}    { return nil }
func (self *fakeControllerFanPrinter) HasStartArg(name string) bool                 { return false }
func (self *fakeControllerFanPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return self.heaters[name]
}
func (self *fakeControllerFanPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }
func (self *fakeControllerFanPrinter) LookupMCU(name string) printerpkg.MCURuntime              { return nil }
func (self *fakeControllerFanPrinter) InvokeShutdown(msg string)                                {}
func (self *fakeControllerFanPrinter) IsShutdown() bool                                         { return false }
func (self *fakeControllerFanPrinter) Reactor() printerpkg.ModuleReactor                        { return self.reactor }
func (self *fakeControllerFanPrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return self.stepperEnable
}
func (self *fakeControllerFanPrinter) GCode() printerpkg.GCodeRuntime                { return nil }
func (self *fakeControllerFanPrinter) GCodeMove() printerpkg.MoveTransformController { return nil }
func (self *fakeControllerFanPrinter) Webhooks() printerpkg.WebhookRegistry          { return nil }

type fakeControllerFanConfig struct {
	printer       printerpkg.ModulePrinter
	name          string
	strings       map[string]string
	floats        map[string]float64
	bools         map[string]bool
	loadedObjects []string
}

func (self *fakeControllerFanConfig) Name() string { return self.name }

func (self *fakeControllerFanConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeControllerFanConfig) Bool(option string, defaultValue bool) bool {
	if value, ok := self.bools[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeControllerFanConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeControllerFanConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeControllerFanConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	return nil
}

func (self *fakeControllerFanConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeControllerFanConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeControllerFanConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigControllerFanResolvesHeatersAndSchedulesFan(t *testing.T) {
	queueMCU := &fakeFanQueueMCU{estimatedValue: 15.0}
	pwmPin := &fakeFanPWMPin{mcu: queueMCU}
	pins := &fakeFanPins{pwm: pwmPin}
	reactor := &fakeControllerFanReactor{monotonic: 2.0}
	heater := &fakeHeaterFanHeater{currentTemp: 40.0, targetTemp: 210.0}
	stepperEnable := &fakeControllerFanStepperEnable{
		names: []string{"stepper_x"},
		lines: map[string]printerpkg.StepperEnableLine{"stepper_x": &fakeControllerFanStepperLine{enabled: false}},
	}
	printer := &fakeControllerFanPrinter{
		lookup:        map[string]interface{}{"pins": pins},
		heaters:       map[string]printerpkg.HeaterRuntime{"extruder": heater},
		reactor:       reactor,
		stepperEnable: stepperEnable,
	}
	config := &fakeControllerFanConfig{
		printer: printer,
		name:    "controller_fan controller_fan",
		strings: map[string]string{
			"pin":          "PA4",
			"stepper":      "stepper_x",
			"heater":       "extruder",
			"idle_timeout": "4",
		},
		floats: map[string]float64{
			"fan_speed":       0.75,
			"idle_speed":      0.20,
			"kick_start_time": 0.0,
		},
		bools: map[string]bool{},
	}

	module := LoadConfigControllerFan(config).(*ControllerFanModule)
	if module == nil {
		t.Fatalf("expected controller fan module instance")
	}
	if !reflect.DeepEqual(config.loadedObjects, []string{"stepper_enable", "heaters"}) {
		t.Fatalf("unexpected load object calls: %#v", config.loadedObjects)
	}
	if printer.eventHandlers["project:connect"] == nil || printer.eventHandlers["project:ready"] == nil || printer.eventHandlers["gcode:request_restart"] == nil {
		t.Fatalf("expected connect, ready, and restart handlers to be registered")
	}

	if err := printer.eventHandlers["project:connect"](nil); err != nil {
		t.Fatalf("connect handler returned error: %v", err)
	}
	if err := printer.eventHandlers["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}
	assertApprox(t, reactor.timerWake, 2.1)
	if reactor.timerCallback == nil {
		t.Fatalf("expected timer callback registration")
	}
	if next := reactor.timerCallback(2.1); next != 3.1 {
		t.Fatalf("unexpected next timer wake: %v", next)
	}
	if !reflect.DeepEqual(stepperEnable.lookupCalls, []string{"stepper_x"}) {
		t.Fatalf("unexpected stepper lookups: %#v", stepperEnable.lookupCalls)
	}
	if !reflect.DeepEqual(heater.calls, []float64{2.1}) {
		t.Fatalf("unexpected heater reads: %#v", heater.calls)
	}
	if len(pwmPin.setCalls) != 1 {
		t.Fatalf("expected one pwm set call after callback, got %#v", pwmPin.setCalls)
	}
	assertApprox(t, pwmPin.setCalls[0][0], 15.0)
	assertApprox(t, pwmPin.setCalls[0][1], 0.75)

	status := module.Get_status(5.0)
	assertApprox(t, status["speed"], 0.75)
	assertApprox(t, status["rpm"], 0.0)

	if err := printer.eventHandlers["gcode:request_restart"]([]interface{}{18.0}); err != nil {
		t.Fatalf("restart handler returned error: %v", err)
	}
	if len(pwmPin.setCalls) != 2 {
		t.Fatalf("expected second pwm set call after restart, got %#v", pwmPin.setCalls)
	}
	assertApprox(t, pwmPin.setCalls[1][0], 18.0)
	assertApprox(t, pwmPin.setCalls[1][1], 0.0)
	if len(queueMCU.flushCallbacks) != 1 {
		t.Fatalf("expected one queue flush callback, got %#v", queueMCU.flushCallbacks)
	}
}

func TestControllerFanConnectRejectsUnknownStepper(t *testing.T) {
	queueMCU := &fakeFanQueueMCU{estimatedValue: 11.0}
	pwmPin := &fakeFanPWMPin{mcu: queueMCU}
	pins := &fakeFanPins{pwm: pwmPin}
	stepperEnable := &fakeControllerFanStepperEnable{
		names: []string{"stepper_x"},
		lines: map[string]printerpkg.StepperEnableLine{"stepper_x": &fakeControllerFanStepperLine{enabled: false}},
	}
	printer := &fakeControllerFanPrinter{
		lookup:        map[string]interface{}{"pins": pins},
		heaters:       map[string]printerpkg.HeaterRuntime{"extruder": &fakeHeaterFanHeater{}},
		reactor:       &fakeControllerFanReactor{monotonic: 1.0},
		stepperEnable: stepperEnable,
	}
	config := &fakeControllerFanConfig{
		printer: printer,
		name:    "controller_fan controller_fan",
		strings: map[string]string{
			"pin":          "PA5",
			"stepper":      "stepper_z",
			"heater":       "extruder",
			"idle_timeout": "4",
		},
		floats: map[string]float64{
			"kick_start_time": 0.0,
		},
		bools: map[string]bool{},
	}

	LoadConfigControllerFan(config)
	err := printer.eventHandlers["project:connect"](nil)
	if err == nil {
		t.Fatalf("expected connect handler error for unknown stepper")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}