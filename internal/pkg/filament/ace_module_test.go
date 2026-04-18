package filament

import (
	"fmt"
	"testing"

	"goklipper/common/utils/object"
	printerpkg "goklipper/internal/pkg/printer"
	reactorpkg "goklipper/internal/pkg/reactor"
)

type fakeACEMutex struct{}

func (self *fakeACEMutex) Lock()   {}
func (self *fakeACEMutex) Unlock() {}

type fakeACEGCode struct {
	commands map[string]func(printerpkg.Command) error
	muxCalls []fakeACEGCodeMuxCall
}

type fakeACEGCodeMuxCall struct {
	cmd   string
	key   string
	value string
}

func newFakeACEGCode() *fakeACEGCode {
	return &fakeACEGCode{commands: map[string]func(printerpkg.Command) error{}}
}

func (self *fakeACEGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	_ = whenNotReady
	_ = desc
	self.commands[cmd] = handler
}

func (self *fakeACEGCode) RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string) {
	_ = handler
	_ = desc
	self.muxCalls = append(self.muxCalls, fakeACEGCodeMuxCall{cmd: cmd, key: key, value: value})
}

func (self *fakeACEGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakeACEGCode) RunScriptFromCommand(script string) { _ = script }
func (self *fakeACEGCode) RunScript(script string)            { _ = script }
func (self *fakeACEGCode) IsBusy() bool                       { return false }
func (self *fakeACEGCode) Mutex() printerpkg.Mutex            { return &fakeACEMutex{} }
func (self *fakeACEGCode) RespondInfo(msg string, log bool)   { _, _ = msg, log }
func (self *fakeACEGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	_ = whenNotReady
	_ = desc
	old := self.commands[cmd]
	self.commands[cmd] = handler
	return old
}

type fakeACEPrinter struct {
	reactor       reactorpkg.IReactor
	moduleReactor *fakeACEModuleReactor
	gcode         *fakeACEGCode
	objects       map[string]interface{}
	eventHandlers map[string]func([]interface{}) error
}

type fakeACEModuleTimer struct{}

func (self *fakeACEModuleTimer) Update(waketime float64) {
	_ = waketime
}

type fakeACEModuleReactor struct {
	monotonic float64
}

func (self *fakeACEModuleReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	_ = callback
	_ = waketime
	return &fakeACEModuleTimer{}
}

func (self *fakeACEModuleReactor) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeACEModuleReactor) Pause(waketime float64) float64 {
	self.monotonic = waketime
	return waketime
}

func (self *fakeACEModuleReactor) RegisterCallback(callback func(float64), waketime float64) {
	self.monotonic = waketime
	callback(waketime)
}

type fakeACEButtons struct {
	pins []string
}

func (self *fakeACEButtons) Register_buttons(pins []string, callback func(float64, int)) {
	_ = callback
	self.pins = append([]string{}, pins...)
}

type fakeACEEndstop struct{}

func (self *fakeACEEndstop) Query_endstop(printTime float64) int {
	_ = printTime
	return 0
}

type fakeACEPins struct {
	parsedPins         []string
	allowedPins        []string
	setupRequests      []string
	setupEndstopResult interface{}
}

func (self *fakeACEPins) Parse_pin(pinDesc string, canInvert bool, canPullup bool) map[string]interface{} {
	_ = canInvert
	_ = canPullup
	self.parsedPins = append(self.parsedPins, pinDesc)
	chipName := "mcu"
	pinName := pinDesc
	if idx := len(pinDesc); idx > 0 {
		for i, ch := range pinDesc {
			if ch == ':' {
				chipName = pinDesc[:i]
				pinName = pinDesc[i+1:]
				break
			}
		}
	}
	return map[string]interface{}{
		"chip_name": chipName,
		"pin":       pinName,
	}
}

func (self *fakeACEPins) Allow_multi_use_pin(pinDesc string) {
	self.allowedPins = append(self.allowedPins, pinDesc)
}

func (self *fakeACEPins) Setup_pin(pinType, pinDesc string) interface{} {
	self.setupRequests = append(self.setupRequests, fmt.Sprintf("%s:%s", pinType, pinDesc))
	if self.setupEndstopResult != nil {
		return self.setupEndstopResult
	}
	return &fakeACEEndstop{}
}

type fakeACEQueryEndstops struct {
	registrations []string
}

func (self *fakeACEQueryEndstops) Register_endstop(endstop interface{}, name string) {
	_ = endstop
	self.registrations = append(self.registrations, name)
}

func newFakeACEPrinter() *fakeACEPrinter {
	return &fakeACEPrinter{
		reactor:       reactorpkg.NewSelectReactor(false),
		moduleReactor: &fakeACEModuleReactor{},
		gcode:         newFakeACEGCode(),
		objects:       map[string]interface{}{},
		eventHandlers: map[string]func([]interface{}) error{},
	}
}

func (self *fakeACEPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if obj, ok := self.objects[name]; ok {
		return obj
	}
	return defaultValue
}
func (self *fakeACEPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	self.eventHandlers[event] = callback
}
func (self *fakeACEPrinter) SendEvent(event string, params []interface{}) { _, _ = event, params }
func (self *fakeACEPrinter) CurrentExtruderName() string                  { return "" }
func (self *fakeACEPrinter) AddObject(name string, obj interface{}) error {
	self.objects[name] = obj
	return nil
}
func (self *fakeACEPrinter) LookupObjects(module string) []interface{}                { _ = module; return nil }
func (self *fakeACEPrinter) HasStartArg(name string) bool                             { _ = name; return false }
func (self *fakeACEPrinter) LookupHeater(name string) printerpkg.HeaterRuntime        { _ = name; return nil }
func (self *fakeACEPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }
func (self *fakeACEPrinter) LookupMCU(name string) printerpkg.MCURuntime              { _ = name; return nil }
func (self *fakeACEPrinter) InvokeShutdown(msg string)                                { _ = msg }
func (self *fakeACEPrinter) IsShutdown() bool                                         { return false }
func (self *fakeACEPrinter) Reactor() printerpkg.ModuleReactor                        { return self.moduleReactor }
func (self *fakeACEPrinter) StepperEnable() printerpkg.StepperEnableRuntime           { return nil }
func (self *fakeACEPrinter) GCode() printerpkg.GCodeRuntime                           { return self.gcode }
func (self *fakeACEPrinter) GCodeMove() printerpkg.MoveTransformController            { return nil }
func (self *fakeACEPrinter) Webhooks() printerpkg.WebhookRegistry                     { return nil }
func (self *fakeACEPrinter) Get_reactor() reactorpkg.IReactor                         { return self.reactor }

type fakeACEConfig struct {
	name    string
	printer *fakeACEPrinter
	values  map[string]interface{}
}

func (self *fakeACEConfig) Name() string { return self.name }
func (self *fakeACEConfig) String(option string, defaultValue string, noteValid bool) string {
	_ = noteValid
	if value, ok := self.values[option]; ok {
		if typed, ok := value.(string); ok {
			return typed
		}
	}
	return defaultValue
}
func (self *fakeACEConfig) Bool(option string, defaultValue bool) bool {
	if value, ok := self.values[option]; ok {
		if typed, ok := value.(bool); ok {
			return typed
		}
	}
	return defaultValue
}
func (self *fakeACEConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.values[option]; ok {
		if typed, ok := value.(float64); ok {
			return typed
		}
	}
	return defaultValue
}
func (self *fakeACEConfig) OptionalFloat(option string) *float64 { _ = option; return nil }
func (self *fakeACEConfig) LoadObject(section string) interface{} {
	return self.printer.LookupObject(section, object.Sentinel{})
}
func (self *fakeACEConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	_, _, _ = module, option, defaultValue
	return nil
}
func (self *fakeACEConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	_, _ = module, option
	return nil
}
func (self *fakeACEConfig) Printer() printerpkg.ModulePrinter { return self.printer }
func (self *fakeACEConfig) Get(option string, defaultValue interface{}, noteValid bool) interface{} {
	_ = noteValid
	if value, ok := self.values[option]; ok {
		return value
	}
	return defaultValue
}
func (self *fakeACEConfig) Getint(option string, defaultValue interface{}, minval, maxval int, noteValid bool) int {
	_, _, _ = minval, maxval, noteValid
	if value, ok := self.values[option]; ok {
		if typed, ok := value.(int); ok {
			return typed
		}
	}
	if typed, ok := defaultValue.(int); ok {
		return typed
	}
	return 0
}
func (self *fakeACEConfig) Getboolean(option string, defaultValue interface{}, noteValid bool) bool {
	_ = noteValid
	if value, ok := self.values[option]; ok {
		if typed, ok := value.(bool); ok {
			return typed
		}
	}
	if typed, ok := defaultValue.(bool); ok {
		return typed
	}
	return false
}

func (self *fakeACEConfig) HasOption(option string) bool {
	_, ok := self.values[option]
	return ok
}

func TestLoadConfigACERegistersCommandsAndEvents(t *testing.T) {
	printer := newFakeACEPrinter()
	config := &fakeACEConfig{
		name:    "ace",
		printer: printer,
		values: map[string]interface{}{
			"serial":                 "/tmp/fake-ace",
			"v2_baud":                115200,
			"endless_spool":          true,
			"feed_speed":             60,
			"retract_speed":          40,
			"toolchange_load_length": 700,
		},
	}

	module := LoadConfigACE(config)
	ace, ok := module.(*ACE)
	if !ok {
		t.Fatalf("expected *ACE module, got %T", module)
	}
	if ace.gcode != printer.gcode {
		t.Fatalf("expected module to retain printer gcode runtime")
	}
	if !ace.state.EndlessSpoolEnabled {
		t.Fatalf("expected endless spool config to propagate")
	}
	if ace.feed_speed != 60 || ace.retract_speed != 40 {
		t.Fatalf("expected configured feed/retract speeds, got %d/%d", ace.feed_speed, ace.retract_speed)
	}

	for _, command := range []string{
		"ACE_SET_SLOT",
		"ACE_QUERY_SLOTS",
		"ACE_DEBUG",
		"ACE_START_DRYING",
		"ACE_STOP_DRYING",
		"ACE_ENABLE_FEED_ASSIST",
		"ACE_DISABLE_FEED_ASSIST",
		"ACE_FEED",
		"ACE_RETRACT",
		"ACE_CHANGE_TOOL",
		"ACE_ENABLE_ENDLESS_SPOOL",
		"ACE_DISABLE_ENDLESS_SPOOL",
		"ACE_ENDLESS_SPOOL_STATUS",
		"ACE_SAVE_INVENTORY",
		"ACE_TEST_RUNOUT_SENSOR",
		"FEED_FILAMENT",
		"UNWIND_FILAMENT",
	} {
		if _, ok := printer.gcode.commands[command]; !ok {
			t.Fatalf("expected command %s to be registered", command)
		}
	}

	if _, ok := printer.eventHandlers["project:ready"]; !ok {
		t.Fatalf("expected project:ready handler registration")
	}
	if _, ok := printer.eventHandlers["project:disconnect"]; !ok {
		t.Fatalf("expected project:disconnect handler registration")
	}
}

func TestLoadConfigACEAllowsMissingSaveVariables(t *testing.T) {
	printer := newFakeACEPrinter()
	config := &fakeACEConfig{
		name:    "ace",
		printer: printer,
		values: map[string]interface{}{
			"serial": "/tmp/fake-ace",
		},
	}

	module := LoadConfigACE(config)
	ace, ok := module.(*ACE)
	if !ok {
		t.Fatalf("expected *ACE module, got %T", module)
	}
	if ace.state == nil {
		t.Fatalf("expected ACE runtime state to be initialized")
	}
	if ace.state.Variables == nil {
		t.Fatalf("expected ACE runtime state to seed default variables")
	}
	if got := ace.state.CurrentIndex(); got != 0 {
		t.Fatalf("expected default current index to be seeded, got %d", got)
	}
	if _, ok := ace.state.Variables["ace_inventory"]; !ok {
		t.Fatalf("expected default inventory variables to be seeded")
	}
	if _, ok := printer.gcode.commands["ACE_QUERY_SLOTS"]; !ok {
		t.Fatalf("expected ACE commands to register even without save_variables")
	}
}

func TestLoadConfigACEBootstrapsExtruderSensorFromPin(t *testing.T) {
	printer := newFakeACEPrinter()
	buttons := &fakeACEButtons{}
	pins := &fakeACEPins{setupEndstopResult: &fakeACEEndstop{}}
	queryEndstops := &fakeACEQueryEndstops{}
	printer.objects["buttons"] = buttons
	printer.objects["pins"] = pins
	printer.objects["query_endstops"] = queryEndstops
	config := &fakeACEConfig{
		name:    "ace",
		printer: printer,
		values: map[string]interface{}{
			"serial":              "/tmp/fake-ace",
			"extruder_sensor_pin": "nozzle_mcu:PB0",
		},
	}

	module := LoadConfigACE(config)
	ace, ok := module.(*ACE)
	if !ok {
		t.Fatalf("expected *ACE module, got %T", module)
	}
	if sensor := ace.lookupFilamentSensor("extruder_sensor"); sensor == nil {
		t.Fatalf("expected extruder sensor to be auto-created from extruder_sensor_pin")
	}
	if got, want := buttons.pins, []string{"nozzle_mcu:PB0"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("registered sensor pins = %v, want %v", got, want)
	}
	if got, want := queryEndstops.registrations, []string{"nozzle_mcu:PB0"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("registered endstops = %v, want %v", got, want)
	}
	if _, ok := ace.endstops["extruder_sensor"]; !ok {
		t.Fatalf("expected extruder sensor endstop registration")
	}
	if len(printer.gcode.muxCalls) != 2 {
		t.Fatalf("expected filament sensor mux commands to register, got %d", len(printer.gcode.muxCalls))
	}
}
