package fan

import (
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeHeaterFanHeater struct {
	currentTemp float64
	targetTemp  float64
	calls       []float64
}

func (self *fakeHeaterFanHeater) GetTemperature(eventtime float64) (float64, float64) {
	self.calls = append(self.calls, eventtime)
	return self.currentTemp, self.targetTemp
}

type fakeHeaterFanTimerHandle struct{}

func (self *fakeHeaterFanTimerHandle) Update(waketime float64) {}

type fakeHeaterFanReactor struct {
	monotonic     float64
	timerCallback func(float64) float64
	timerWake     float64
}

func (self *fakeHeaterFanReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	self.timerCallback = callback
	self.timerWake = waketime
	return &fakeHeaterFanTimerHandle{}
}

func (self *fakeHeaterFanReactor) Monotonic() float64 {
	return self.monotonic
}

type fakeHeaterFanPrinter struct {
	lookup        map[string]interface{}
	heaters       map[string]printerpkg.HeaterRuntime
	reactor       printerpkg.ModuleReactor
	eventHandlers map[string]func([]interface{}) error
}

func (self *fakeHeaterFanPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterFanPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeHeaterFanPrinter) SendEvent(event string, params []interface{}) {}
func (self *fakeHeaterFanPrinter) CurrentExtruderName() string                  { return "extruder" }
func (self *fakeHeaterFanPrinter) AddObject(name string, obj interface{}) error { return nil }
func (self *fakeHeaterFanPrinter) LookupObjects(module string) []interface{}    { return nil }
func (self *fakeHeaterFanPrinter) HasStartArg(name string) bool                 { return false }
func (self *fakeHeaterFanPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return self.heaters[name]
}
func (self *fakeHeaterFanPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }
func (self *fakeHeaterFanPrinter) LookupMCU(name string) printerpkg.MCURuntime              { return nil }
func (self *fakeHeaterFanPrinter) InvokeShutdown(msg string)                                {}
func (self *fakeHeaterFanPrinter) IsShutdown() bool                                         { return false }
func (self *fakeHeaterFanPrinter) Reactor() printerpkg.ModuleReactor                        { return self.reactor }
func (self *fakeHeaterFanPrinter) StepperEnable() printerpkg.StepperEnableRuntime           { return nil }
func (self *fakeHeaterFanPrinter) GCode() printerpkg.GCodeRuntime                           { return nil }
func (self *fakeHeaterFanPrinter) GCodeMove() printerpkg.MoveTransformController            { return nil }
func (self *fakeHeaterFanPrinter) Webhooks() printerpkg.WebhookRegistry                     { return nil }

type fakeHeaterFanConfig struct {
	printer       printerpkg.ModulePrinter
	name          string
	strings       map[string]string
	floats        map[string]float64
	bools         map[string]bool
	loadedObjects []string
}

func (self *fakeHeaterFanConfig) Name() string { return self.name }

func (self *fakeHeaterFanConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterFanConfig) Bool(option string, defaultValue bool) bool {
	if value, ok := self.bools[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterFanConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterFanConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeHeaterFanConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	return nil
}

func (self *fakeHeaterFanConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeHeaterFanConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeHeaterFanConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigHeaterFanRegistersReadyHandlerAndDrivesFan(t *testing.T) {
	queueMCU := &fakeFanQueueMCU{estimatedValue: 9.0}
	pwmPin := &fakeFanPWMPin{mcu: queueMCU}
	pins := &fakeFanPins{pwm: pwmPin}
	reactor := &fakeHeaterFanReactor{monotonic: 3.0}
	heater := &fakeHeaterFanHeater{currentTemp: 60.0, targetTemp: 0.0}
	printer := &fakeHeaterFanPrinter{
		lookup:  map[string]interface{}{"pins": pins},
		heaters: map[string]printerpkg.HeaterRuntime{"extruder": heater},
		reactor: reactor,
	}
	config := &fakeHeaterFanConfig{
		printer: printer,
		name:    "heater_fan extruder_fan",
		strings: map[string]string{"pin": "PA2", "heater": "extruder"},
		floats: map[string]float64{
			"fan_speed":       0.8,
			"heater_temp":     50.0,
			"kick_start_time": 0.0,
		},
		bools: map[string]bool{},
	}

	module := LoadConfigHeaterFan(config).(*HeaterFanModule)
	if module == nil {
		t.Fatalf("expected heater fan module instance")
	}
	if !reflect.DeepEqual(config.loadedObjects, []string{"heaters"}) {
		t.Fatalf("unexpected load object calls: %#v", config.loadedObjects)
	}
	if printer.eventHandlers["project:ready"] == nil || printer.eventHandlers["gcode:request_restart"] == nil {
		t.Fatalf("expected ready and restart handlers to be registered")
	}

	if err := printer.eventHandlers["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}
	assertApprox(t, reactor.timerWake, 3.1)
	if reactor.timerCallback == nil {
		t.Fatalf("expected timer callback registration")
	}
	if next := reactor.timerCallback(3.1); next != 4.1 {
		t.Fatalf("unexpected next timer wake: %v", next)
	}
	if len(pwmPin.setCalls) != 1 {
		t.Fatalf("expected one pwm set call after heater callback, got %#v", pwmPin.setCalls)
	}
	assertApprox(t, pwmPin.setCalls[0][0], 9.0)
	assertApprox(t, pwmPin.setCalls[0][1], 0.8)
	if !reflect.DeepEqual(heater.calls, []float64{3.1}) {
		t.Fatalf("unexpected heater reads: %#v", heater.calls)
	}

	status := module.Get_status(5.0)
	assertApprox(t, status["speed"], 0.8)
	assertApprox(t, status["rpm"], 0.0)

	if err := printer.eventHandlers["gcode:request_restart"]([]interface{}{12.0}); err != nil {
		t.Fatalf("restart handler returned error: %v", err)
	}
	if len(pwmPin.setCalls) != 2 {
		t.Fatalf("expected second pwm set call after restart, got %#v", pwmPin.setCalls)
	}
	assertApprox(t, pwmPin.setCalls[1][0], 12.0)
	assertApprox(t, pwmPin.setCalls[1][1], 0.0)
	if len(queueMCU.flushCallbacks) != 1 {
		t.Fatalf("expected one queue flush callback, got %#v", queueMCU.flushCallbacks)
	}
}