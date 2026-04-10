package vibration

import (
	"reflect"
	"testing"

	"goklipper/internal/pkg/chelper"
	printerpkg "goklipper/internal/pkg/printer"
)

type fakeInputShaperCommand struct {
	strings       map[string]string
	floats        map[string]float64
	infoResponses []string
}

func (self *fakeInputShaperCommand) String(name string, defaultValue string) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeInputShaperCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeInputShaperCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeInputShaperCommand) Parameters() map[string]string {
	params := map[string]string{}
	for key, value := range self.strings {
		params[key] = value
	}
	return params
}

func (self *fakeInputShaperCommand) RespondInfo(msg string, log bool) {
	self.infoResponses = append(self.infoResponses, msg)
	_ = log
}

func (self *fakeInputShaperCommand) RespondRaw(msg string) {}

func (self *fakeInputShaperCommand) Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	if defaultString, ok := _default.(string); ok {
		return defaultString
	}
	return ""
}

func (self *fakeInputShaperCommand) Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	if defaultFloat, ok := _default.(float64); ok {
		return defaultFloat
	}
	return 0.
}

type fakeInputShaperGCode struct {
	commands map[string]func(printerpkg.Command) error
}

func (self *fakeInputShaperGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
	_, _ = whenNotReady, desc
}

func (self *fakeInputShaperGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakeInputShaperGCode) RunScriptFromCommand(script string) {}
func (self *fakeInputShaperGCode) RunScript(script string)            {}
func (self *fakeInputShaperGCode) IsBusy() bool                       { return false }
func (self *fakeInputShaperGCode) Mutex() printerpkg.Mutex            { return nil }
func (self *fakeInputShaperGCode) RespondInfo(msg string, log bool)   {}
func (self *fakeInputShaperGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeInputShaperStepper struct {
	calls   []interface{}
	current interface{}
}

func (self *fakeInputShaperStepper) Set_stepper_kinematics(sk interface{}) interface{} {
	self.calls = append(self.calls, sk)
	previous := self.current
	self.current = sk
	return previous
}

type fakeInputShaperKinematics struct {
	steppers []interface{}
}

func (self *fakeInputShaperKinematics) Get_steppers() []interface{} {
	return self.steppers
}

type fakeInputShaperToolhead struct {
	kinematics    *fakeInputShaperKinematics
	flushCount    int
	scanTimeCalls [][2]float64
}

func (self *fakeInputShaperToolhead) Get_kinematics() interface{} {
	return self.kinematics
}

func (self *fakeInputShaperToolhead) Flush_step_generation() {
	self.flushCount++
}

func (self *fakeInputShaperToolhead) Note_step_generation_scan_time(delay, old_delay float64) {
	self.scanTimeCalls = append(self.scanTimeCalls, [2]float64{delay, old_delay})
}

type fakeInputShaperPrinter struct {
	lookup        map[string]interface{}
	gcode         *fakeInputShaperGCode
	eventHandlers map[string]func([]interface{}) error
}

func (self *fakeInputShaperPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeInputShaperPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeInputShaperPrinter) SendEvent(event string, params []interface{}) {}
func (self *fakeInputShaperPrinter) CurrentExtruderName() string                  { return "extruder" }
func (self *fakeInputShaperPrinter) AddObject(name string, obj interface{}) error { return nil }
func (self *fakeInputShaperPrinter) LookupObjects(module string) []interface{}    { return nil }
func (self *fakeInputShaperPrinter) HasStartArg(name string) bool                 { return false }
func (self *fakeInputShaperPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}
func (self *fakeInputShaperPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}
func (self *fakeInputShaperPrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }
func (self *fakeInputShaperPrinter) InvokeShutdown(msg string)                   {}
func (self *fakeInputShaperPrinter) IsShutdown() bool                            { return false }
func (self *fakeInputShaperPrinter) Reactor() printerpkg.ModuleReactor           { return nil }
func (self *fakeInputShaperPrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return nil
}
func (self *fakeInputShaperPrinter) GCode() printerpkg.GCodeRuntime                { return self.gcode }
func (self *fakeInputShaperPrinter) GCodeMove() printerpkg.MoveTransformController { return nil }
func (self *fakeInputShaperPrinter) Webhooks() printerpkg.WebhookRegistry          { return nil }

type fakeInputShaperConfig struct {
	printer *fakeInputShaperPrinter
	name    string
	strings map[string]string
	floats  map[string]float64
}

func (self *fakeInputShaperConfig) Name() string { return self.name }

func (self *fakeInputShaperConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeInputShaperConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeInputShaperConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeInputShaperConfig) OptionalFloat(option string) *float64  { return nil }
func (self *fakeInputShaperConfig) LoadObject(section string) interface{} { return nil }
func (self *fakeInputShaperConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}
func (self *fakeInputShaperConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}
func (self *fakeInputShaperConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func newFakeInputShaperConfig() (*fakeInputShaperConfig, *fakeInputShaperPrinter, *fakeInputShaperToolhead, []*fakeInputShaperStepper) {
	steppers := []*fakeInputShaperStepper{
		{current: chelper.Cartesian_stepper_alloc(int8('x'))},
		{current: chelper.Cartesian_stepper_alloc(int8('y'))},
	}
	kinematics := &fakeInputShaperKinematics{steppers: []interface{}{steppers[0], steppers[1]}}
	toolhead := &fakeInputShaperToolhead{kinematics: kinematics}
	printer := &fakeInputShaperPrinter{
		lookup: map[string]interface{}{"toolhead": toolhead},
		gcode:  &fakeInputShaperGCode{},
	}
	config := &fakeInputShaperConfig{
		printer: printer,
		name:    "input_shaper",
		strings: map[string]string{"shaper_type": "mzv"},
		floats:  map[string]float64{"shaper_freq_x": 0., "shaper_freq_y": 0.},
	}
	return config, printer, toolhead, steppers
}

func TestLoadConfigInputShaperRegistersCommandAndConnectsSteppers(t *testing.T) {
	config, printer, toolhead, steppers := newFakeInputShaperConfig()
	module := LoadConfigInputShaper(config).(*InputShaper)
	if module == nil {
		t.Fatalf("expected input shaper module instance")
	}
	if printer.gcode.commands["SET_INPUT_SHAPER"] == nil {
		t.Fatalf("expected SET_INPUT_SHAPER command registration")
	}
	if printer.eventHandlers["project:connect"] == nil {
		t.Fatalf("expected project:connect handler registration")
	}
	if err := module.Connect(nil); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if len(module.stepper_kinematics) != 2 || len(module.orig_stepper_kinematics) != 2 {
		t.Fatalf("expected stepper kinematics to be initialized: %#v %#v", module.stepper_kinematics, module.orig_stepper_kinematics)
	}
	for idx, stepper := range steppers {
		if len(stepper.calls) != 1 {
			t.Fatalf("expected one Set_stepper_kinematics call for stepper %d, got %d", idx, len(stepper.calls))
		}
	}
	if toolhead.flushCount == 0 {
		t.Fatalf("expected Connect to flush step generation")
	}
	if len(toolhead.scanTimeCalls) == 0 {
		t.Fatalf("expected Connect to note step generation scan time")
	}
}

func TestInputShaperCommandDisableAndEnablePreserveCompatibilityMethods(t *testing.T) {
	config, _, toolhead, _ := newFakeInputShaperConfig()
	module := NewInputShaper(config)
	if err := module.Connect(nil); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	initialFlushes := toolhead.flushCount
	cmd := &fakeInputShaperCommand{floats: map[string]float64{"SHAPER_FREQ_X": 48.0}}
	if err := module.Cmd_SET_INPUT_SHAPER(cmd); err != nil {
		t.Fatalf("Cmd_SET_INPUT_SHAPER returned error: %v", err)
	}
	if module.shapers[0].params.shaper_freq != 48.0 {
		t.Fatalf("expected x shaper frequency to update, got %v", module.shapers[0].params.shaper_freq)
	}
	if len(cmd.infoResponses) != 2 {
		t.Fatalf("expected one report per axis, got %#v", cmd.infoResponses)
	}
	if toolhead.flushCount <= initialFlushes {
		t.Fatalf("expected SET_INPUT_SHAPER to reconfigure toolhead, flush count %d -> %d", initialFlushes, toolhead.flushCount)
	}
	beforeDisable := toolhead.flushCount
	module.Disable_shaping()
	if module.shapers[0].saved == nil {
		t.Fatalf("expected Disable_shaping to save active shaper state")
	}
	if module.shapers[0].n != 0 || !reflect.DeepEqual(module.shapers[0].A, []float64{}) || !reflect.DeepEqual(module.shapers[0].T, []float64{}) {
		t.Fatalf("expected Disable_shaping to switch to none shaper, got n=%d A=%v T=%v", module.shapers[0].n, module.shapers[0].A, module.shapers[0].T)
	}
	if toolhead.flushCount <= beforeDisable {
		t.Fatalf("expected Disable_shaping to flush toolhead")
	}
	beforeEnable := toolhead.flushCount
	module.Enable_shaping()
	if module.shapers[0].saved != nil {
		t.Fatalf("expected Enable_shaping to restore and clear saved state")
	}
	if module.shapers[0].n == 0 {
		t.Fatalf("expected Enable_shaping to restore non-empty shaper")
	}
	if toolhead.flushCount <= beforeEnable {
		t.Fatalf("expected Enable_shaping to flush toolhead")
	}
}
