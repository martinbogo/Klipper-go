package io

import (
	"fmt"
	"reflect"
	"testing"

	"goklipper/common/constants"
	printerpkg "goklipper/internal/pkg/printer"
	printpkg "goklipper/internal/print"
)

type fakeFilamentTimer struct {
	callback func(float64) float64
	updates  []float64
}

func (self *fakeFilamentTimer) Update(waketime float64) {
	self.updates = append(self.updates, waketime)
}

type fakeFilamentReactor struct {
	monotonic        float64
	callbackRequests []float64
	pauses           []float64
	timers           []*fakeFilamentTimer
}

func (self *fakeFilamentReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	timer := &fakeFilamentTimer{callback: callback, updates: []float64{waketime}}
	self.timers = append(self.timers, timer)
	return timer
}

func (self *fakeFilamentReactor) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeFilamentReactor) Pause(waketime float64) float64 {
	self.pauses = append(self.pauses, waketime)
	self.monotonic = waketime
	return waketime
}

func (self *fakeFilamentReactor) RegisterCallback(callback func(float64), waketime float64) {
	self.callbackRequests = append(self.callbackRequests, waketime)
	callback(self.monotonic)
}

type fakeFilamentGCodeMuxCall struct {
	cmd   string
	key   string
	value string
	desc  string
}

type fakeFilamentGCode struct {
	muxCalls []fakeFilamentGCodeMuxCall
	scripts  []string
	infos    []string
}

func (self *fakeFilamentGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
}

func (self *fakeFilamentGCode) RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string) {
	self.muxCalls = append(self.muxCalls, fakeFilamentGCodeMuxCall{cmd: cmd, key: key, value: value, desc: desc})
}

func (self *fakeFilamentGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeFilamentGCode) RunScriptFromCommand(script string) {
	self.scripts = append(self.scripts, script)
}

func (self *fakeFilamentGCode) RunScript(script string) {
	self.scripts = append(self.scripts, script)
}

func (self *fakeFilamentGCode) IsBusy() bool {
	return false
}

func (self *fakeFilamentGCode) Mutex() printerpkg.Mutex {
	return nil
}

func (self *fakeFilamentGCode) RespondInfo(msg string, log bool) {
	self.infos = append(self.infos, msg)
}

func (self *fakeFilamentGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeFilamentButtons struct {
	pins     []string
	callback func(float64, int)
}

func (self *fakeFilamentButtons) Register_buttons(pins []string, callback func(float64, int)) {
	self.pins = append([]string{}, pins...)
	self.callback = callback
}

type fakeFilamentIdleTimeout struct {
	state string
}

func (self *fakeFilamentIdleTimeout) Get_status(eventtime float64) map[string]interface{} {
	return map[string]interface{}{"state": self.state}
}

type fakeFilamentPauseResume struct {
	pauseCalls int
}

func (self *fakeFilamentPauseResume) Send_pause_command() {
	self.pauseCalls++
}

type fakeFilamentTemplate struct {
	script string
}

func (self *fakeFilamentTemplate) CreateContext(eventtime interface{}) map[string]interface{} {
	return nil
}

func (self *fakeFilamentTemplate) Render(context map[string]interface{}) (string, error) {
	return self.script, nil
}

func (self *fakeFilamentTemplate) RunGcodeFromCommand(context map[string]interface{}) error {
	return nil
}

type fakeFilamentExtruder struct {
	position float64
}

func (self *fakeFilamentExtruder) Find_past_position(printTime float64) float64 {
	return self.position
}

type fakeFilamentMCU struct{}

func (self *fakeFilamentMCU) Estimated_print_time(eventtime float64) float64 {
	return eventtime
}

type fakeFilamentPrinter struct {
	objects       map[string]interface{}
	eventHandlers map[string]func([]interface{}) error
	reactor       *fakeFilamentReactor
	gcode         *fakeFilamentGCode
}

func (self *fakeFilamentPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.objects[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFilamentPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	self.eventHandlers[event] = callback
}

func (self *fakeFilamentPrinter) SendEvent(event string, params []interface{}) {
}

func (self *fakeFilamentPrinter) CurrentExtruderName() string {
	return "extruder"
}

func (self *fakeFilamentPrinter) AddObject(name string, obj interface{}) error {
	return nil
}

func (self *fakeFilamentPrinter) LookupObjects(module string) []interface{} {
	return nil
}

func (self *fakeFilamentPrinter) HasStartArg(name string) bool {
	return false
}

func (self *fakeFilamentPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}

func (self *fakeFilamentPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeFilamentPrinter) LookupMCU(name string) printerpkg.MCURuntime {
	return nil
}

func (self *fakeFilamentPrinter) InvokeShutdown(msg string) {
}

func (self *fakeFilamentPrinter) IsShutdown() bool {
	return false
}

func (self *fakeFilamentPrinter) Reactor() printerpkg.ModuleReactor {
	return self.reactor
}

func (self *fakeFilamentPrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return nil
}

func (self *fakeFilamentPrinter) GCode() printerpkg.GCodeRuntime {
	return self.gcode
}

func (self *fakeFilamentPrinter) GCodeMove() printerpkg.MoveTransformController {
	return nil
}

func (self *fakeFilamentPrinter) Webhooks() printerpkg.WebhookRegistry {
	return nil
}

type fakeFilamentConfig struct {
	name           string
	printer        *fakeFilamentPrinter
	stringValues   map[string]string
	boolValues     map[string]bool
	floatValues    map[string]float64
	loadedObjects  []string
	loadedTemplates []string
	objects        map[string]interface{}
	templates      map[string]printerpkg.Template
	options        map[string]bool
}

func (self *fakeFilamentConfig) Name() string {
	return self.name
}

func (self *fakeFilamentConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.stringValues[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFilamentConfig) Bool(option string, defaultValue bool) bool {
	if value, ok := self.boolValues[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFilamentConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floatValues[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFilamentConfig) OptionalFloat(option string) *float64 {
	return nil
}

func (self *fakeFilamentConfig) HasOption(option string) bool {
	return self.options[option]
}

func (self *fakeFilamentConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	obj := self.objects[section]
	if obj != nil {
		self.printer.objects[section] = obj
	}
	return obj
}

func (self *fakeFilamentConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	self.loadedTemplates = append(self.loadedTemplates, option)
	if template, ok := self.templates[option]; ok {
		return template
	}
	return &fakeFilamentTemplate{script: defaultValue}
}

func (self *fakeFilamentConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return self.LoadTemplate(module, option, "")
}

func (self *fakeFilamentConfig) Printer() printerpkg.ModulePrinter {
	return self.printer
}

func newFakeFilamentEnvironment() (*fakeFilamentPrinter, *fakeFilamentConfig, *fakeFilamentButtons, *fakeFilamentIdleTimeout, *fakeFilamentPauseResume) {
	reactor := &fakeFilamentReactor{monotonic: 5.0}
	gcode := &fakeFilamentGCode{}
	buttons := &fakeFilamentButtons{}
	idleTimeout := &fakeFilamentIdleTimeout{state: printpkg.StateReady}
	pauseResume := &fakeFilamentPauseResume{}
	printer := &fakeFilamentPrinter{
		objects: map[string]interface{}{
			"idle_timeout": idleTimeout,
			"extruder":     &fakeFilamentExtruder{},
			"mcu":          &fakeFilamentMCU{},
		},
		eventHandlers: map[string]func([]interface{}) error{},
		reactor:       reactor,
		gcode:         gcode,
	}
	config := &fakeFilamentConfig{
		name:         "filament_switch_sensor extruder_sensor",
		printer:      printer,
		stringValues: map[string]string{"switch_pin": "PA1", "extruder": "extruder"},
		boolValues:   map[string]bool{"pause_on_runout": false},
		floatValues:  map[string]float64{},
		objects: map[string]interface{}{
			"buttons":      buttons,
			"pause_resume": pauseResume,
		},
		templates: map[string]printerpkg.Template{},
		options:   map[string]bool{"switch_pin": true, "extruder": true},
	}
	return printer, config, buttons, idleTimeout, pauseResume
}

func TestLoadConfigSwitchSensorRegistersButtonsAndMuxCommands(t *testing.T) {
	printer, config, buttons, _, _ := newFakeFilamentEnvironment()
	module := LoadConfigSwitchSensor(config).(*SwitchSensorModule)
	if module == nil {
		t.Fatalf("expected switch sensor module instance")
	}
	if got, want := buttons.pins, []string{"PA1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registered pins = %v, want %v", got, want)
	}
	if len(printer.gcode.muxCalls) != 2 {
		t.Fatalf("mux calls = %d, want 2", len(printer.gcode.muxCalls))
	}
	if got := module.Get_status(0)["name"]; got != "extruder_sensor" {
		t.Fatalf("sensor name = %v, want extruder_sensor", got)
	}
	if module.FilamentPresent() {
		t.Fatalf("FilamentPresent() should default to false")
	}
	if got, want := config.loadedObjects, []string{"buttons"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded objects = %v, want %v", got, want)
	}
}

func TestSwitchSensorRunsInsertAndRunoutScripts(t *testing.T) {
	printer, config, buttons, idleTimeout, pauseResume := newFakeFilamentEnvironment()
	config.boolValues["pause_on_runout"] = true
	config.options["insert_gcode"] = true
	config.options["runout_gcode"] = true
	config.templates["insert_gcode"] = &fakeFilamentTemplate{script: "INSERT"}
	config.templates["runout_gcode"] = &fakeFilamentTemplate{script: "RUNOUT"}
	module := LoadConfigSwitchSensor(config).(*SwitchSensorModule)
	if err := printer.eventHandlers["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}

	printer.reactor.monotonic = 15.0
	idleTimeout.state = printpkg.StateReady
	buttons.callback(15.0, 1)
	if got, want := printer.gcode.scripts, []string{"INSERT\nM400"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("scripts after insert = %v, want %v", got, want)
	}

	printer.reactor.monotonic = 20.0
	idleTimeout.state = printpkg.StatePrinting
	buttons.callback(20.0, 0)
	if got, want := printer.gcode.scripts, []string{"INSERT\nM400", "PAUSE\nRUNOUT\nM400"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("scripts after runout = %v, want %v", got, want)
	}
	if pauseResume.pauseCalls != 1 {
		t.Fatalf("pause calls = %d, want 1", pauseResume.pauseCalls)
	}
	if got, want := printer.reactor.pauses, []float64{20.5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pause wake times = %v, want %v", got, want)
	}
	if module.FilamentPresent() {
		t.Fatalf("FilamentPresent() should be false after runout")
	}
	if got, want := config.loadedObjects, []string{"buttons", "pause_resume"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded objects = %v, want %v", got, want)
	}
	if got, want := config.loadedTemplates, []string{"runout_gcode", "insert_gcode"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded templates = %v, want %v", got, want)
	}
}

func TestLoadConfigPrefixEncoderSensorTracksFilamentPresence(t *testing.T) {
	printer, config, buttons, idleTimeout, _ := newFakeFilamentEnvironment()
	config.name = "encoder_sensor motion_sensor"
	config.stringValues["switch_pin"] = "PA2"
	config.floatValues["detection_length"] = 7.0
	extruder := &fakeFilamentExtruder{position: 5.0}
	printer.objects["extruder"] = extruder
	module := LoadConfigPrefixEncoderSensor(config).(*EncoderSensorModule)
	if got, want := buttons.pins, []string{"PA2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registered pins = %v, want %v", got, want)
	}
	printer.reactor.monotonic = 1.0
	if err := printer.eventHandlers["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}
	if len(printer.reactor.timers) != 1 {
		t.Fatalf("timers = %d, want 1", len(printer.reactor.timers))
	}

	idleTimeout.state = printpkg.StatePrinting
	if err := printer.eventHandlers["idle_timeout:printing"](nil); err != nil {
		t.Fatalf("printing handler returned error: %v", err)
	}

	printer.reactor.monotonic = 4.0
	buttons.callback(4.0, 1)
	if !module.FilamentPresent() {
		t.Fatalf("FilamentPresent() should be true after encoder event")
	}

	extruder.position = 13.0
	printer.reactor.monotonic = 5.0
	nextWake := printer.reactor.timers[0].callback(5.0)
	if nextWake != 5.0+checkRunoutTimeout {
		t.Fatalf("next wake = %v, want %v", nextWake, 5.0+checkRunoutTimeout)
	}
	if module.FilamentPresent() {
		t.Fatalf("FilamentPresent() should be false after runout position is reached")
	}
	if got, want := fmt.Sprint(printer.reactor.timers[0].updates), fmt.Sprint([]float64{constants.NEVER, constants.NOW}); got != want {
		t.Fatalf("timer updates = %s, want %s", got, want)
	}
}