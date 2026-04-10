package gcode

import (
	"strconv"
	"strings"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeGCodeMoveGCode struct {
	commands map[string]func(printerpkg.Command) error
	mutex    printerpkg.Mutex
}

func (self *fakeGCodeMoveGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeGCodeMoveGCode) IsTraditionalGCode(cmd string) bool { return false }

func (self *fakeGCodeMoveGCode) RunScriptFromCommand(script string) {}

func (self *fakeGCodeMoveGCode) RunScript(script string) {}

func (self *fakeGCodeMoveGCode) IsBusy() bool { return false }

func (self *fakeGCodeMoveGCode) Mutex() printerpkg.Mutex { return self.mutex }

func (self *fakeGCodeMoveGCode) RespondInfo(msg string, log bool) {}

func (self *fakeGCodeMoveGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return old
}

type fakeGCodeMovePrinter struct {
	gcode   printerpkg.GCodeRuntime
	objects map[string]interface{}
	events  map[string]func([]interface{}) error
}

func (self *fakeGCodeMovePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.objects[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeGCodeMovePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.events == nil {
		self.events = map[string]func([]interface{}) error{}
	}
	self.events[event] = callback
}

func (self *fakeGCodeMovePrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeGCodeMovePrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeGCodeMovePrinter) AddObject(name string, obj interface{}) error { return nil }

func (self *fakeGCodeMovePrinter) LookupObjects(module string) []interface{} { return nil }

func (self *fakeGCodeMovePrinter) HasStartArg(name string) bool { return false }

func (self *fakeGCodeMovePrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeGCodeMovePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }

func (self *fakeGCodeMovePrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }

func (self *fakeGCodeMovePrinter) InvokeShutdown(msg string) {}

func (self *fakeGCodeMovePrinter) IsShutdown() bool { return false }

func (self *fakeGCodeMovePrinter) Reactor() printerpkg.ModuleReactor { return nil }

func (self *fakeGCodeMovePrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeGCodeMovePrinter) GCode() printerpkg.GCodeRuntime { return self.gcode }

func (self *fakeGCodeMovePrinter) GCodeMove() printerpkg.MoveTransformController { return nil }

func (self *fakeGCodeMovePrinter) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeGCodeMoveConfig struct {
	printer printerpkg.ModulePrinter
}

func (self *fakeGCodeMoveConfig) Name() string { return "gcode_move" }

func (self *fakeGCodeMoveConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeGCodeMoveConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeGCodeMoveConfig) Float(option string, defaultValue float64) float64 { return defaultValue }

func (self *fakeGCodeMoveConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeGCodeMoveConfig) LoadObject(section string) interface{} { return nil }

func (self *fakeGCodeMoveConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeGCodeMoveConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeGCodeMoveConfig) Printer() printerpkg.ModulePrinter { return self.printer }

type fakeStepper struct {
	name            string
	mcuPosition     int
	commandedPos    float64
}

func (self *fakeStepper) Get_name(ignore bool) string { return self.name }

func (self *fakeStepper) Get_mcu_position() int { return self.mcuPosition }

func (self *fakeStepper) Get_commanded_position() float64 { return self.commandedPos }

type fakeKinematics struct {
	steppers []interface{}
	position []float64
}

func (self *fakeKinematics) Get_steppers() []interface{} { return self.steppers }

func (self *fakeKinematics) Calc_position(cinfo map[string]float64) []float64 {
	return append([]float64{}, self.position...)
}

type fakeLegacyTransform struct {
	position []float64
	moves    [][]float64
	speeds   []float64
}

func (self *fakeLegacyTransform) Move(newpos []float64, speed float64) {
	self.position = append([]float64{}, newpos...)
	self.moves = append(self.moves, append([]float64{}, newpos...))
	self.speeds = append(self.speeds, speed)
}

func (self *fakeLegacyTransform) Get_position() []float64 {
	return append([]float64{}, self.position...)
}

type fakeBootstrapTransform struct {
	position []float64
	moves    [][]float64
	speeds   []float64
}

func (self *fakeBootstrapTransform) GetPosition() []float64 {
	return append([]float64{}, self.position...)
}

func (self *fakeBootstrapTransform) Move(newpos []float64, speed float64) {
	self.position = append([]float64{}, newpos...)
	self.moves = append(self.moves, append([]float64{}, newpos...))
	self.speeds = append(self.speeds, speed)
}

type fakeToolhead struct {
	position  []float64
	transform LegacyMoveTransform
	kin       interface{}
	moves     [][]float64
	speeds    []float64
}

func (self *fakeToolhead) Move(newpos []float64, speed float64) {
	self.position = append([]float64{}, newpos...)
	self.moves = append(self.moves, append([]float64{}, newpos...))
	self.speeds = append(self.speeds, speed)
}

func (self *fakeToolhead) Get_position() []float64 {
	return append([]float64{}, self.position...)
}

func (self *fakeToolhead) Get_transform() LegacyMoveTransform {
	return self.transform
}

func (self *fakeToolhead) Get_kinematics() interface{} {
	return self.kin
}

type fakeMoveCommand struct {
	params       map[string]string
	commandline  string
	infoMessages []string
	rawMessages  []string
}

func (self *fakeMoveCommand) String(name string, defaultValue string) string {
	if value, ok := self.params[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeMoveCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.params[name]; ok {
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed
		}
	}
	return defaultValue
}

func (self *fakeMoveCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	if value, ok := self.params[name]; ok {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return defaultValue
}

func (self *fakeMoveCommand) Parameters() map[string]string {
	copyParams := map[string]string{}
	for key, value := range self.params {
		copyParams[key] = value
	}
	return copyParams
}

func (self *fakeMoveCommand) RespondInfo(msg string, log bool) {
	self.infoMessages = append(self.infoMessages, msg)
}

func (self *fakeMoveCommand) RespondRaw(msg string) {
	self.rawMessages = append(self.rawMessages, msg)
}

func (self *fakeMoveCommand) Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
	if value, ok := self.params[name]; ok {
		return value
	}
	if _default == nil {
		return ""
	}
	if value, ok := _default.(string); ok {
		return value
	}
	return ""
}

func (self *fakeMoveCommand) Get_int(name string, _default interface{}, minval *int, maxval *int) int {
	if value, ok := self.params[name]; ok {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	if _default == nil {
		return 0
	}
	if value, ok := _default.(int); ok {
		return value
	}
	return 0
}

func (self *fakeMoveCommand) Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	if value, ok := self.params[name]; ok {
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed
		}
	}
	if _default == nil {
		return 0
	}
	if value, ok := _default.(float64); ok {
		return value
	}
	return 0
}

func (self *fakeMoveCommand) Get_command_parameters() map[string]string {
	return self.Parameters()
}

func (self *fakeMoveCommand) Get_commandline() string {
	if self.commandline != "" {
		return self.commandline
	}
	return "G1"
}

func TestLoadConfigGCodeMoveRegistersCommandsAndTracksMoves(t *testing.T) {
	gcode := &fakeGCodeMoveGCode{}
	toolhead := &fakeToolhead{position: []float64{0, 0, 0, 0}}
	printer := &fakeGCodeMovePrinter{
		gcode:   gcode,
		objects: map[string]interface{}{"toolhead": toolhead},
	}
	module := LoadConfigGCodeMove(&fakeGCodeMoveConfig{printer: printer}).(*GCodeMoveModule)

	for _, cmd := range []string{"G0", "G1", "G20", "G21", "M82", "M83", "G90", "G91", "G92", "M114", "M220", "M221", "SET_GCODE_OFFSET", "SAVE_GCODE_STATE", "RESTORE_GCODE_STATE", "GET_POSITION"} {
		if _, ok := gcode.commands[cmd]; !ok {
			t.Fatalf("expected %s to be registered", cmd)
		}
	}
	if err := printer.events["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}
	cmd := &fakeMoveCommand{params: map[string]string{"X": "1.5", "Y": "2.5", "F": "120"}, commandline: "G1 X1.5 Y2.5 F120"}
	if err := module.Cmd_G1(cmd); err != nil {
		t.Fatalf("Cmd_G1 returned error: %v", err)
	}
	if got := toolhead.position; got[0] != 1.5 || got[1] != 2.5 || got[2] != 0 || got[3] != 0 {
		t.Fatalf("unexpected toolhead position: %#v", got)
	}
	status := module.Get_status(0)
	position := status["position"].([]float64)
	if position[0] != 1.5 || position[1] != 2.5 {
		t.Fatalf("unexpected gcode position: %#v", position)
	}
	if status["speed"].(float64) != 120 {
		t.Fatalf("expected speed 120, got %v", status["speed"])
	}
}

func TestGCodeMoveSetMoveTransformSupportsCompatibilityMethods(t *testing.T) {
	gcode := &fakeGCodeMoveGCode{}
	oldTransform := &fakeLegacyTransform{position: []float64{9, 8, 7, 6}}
	toolhead := &fakeToolhead{position: []float64{0, 0, 0, 0}, transform: oldTransform}
	printer := &fakeGCodeMovePrinter{
		gcode:   gcode,
		objects: map[string]interface{}{"toolhead": toolhead},
	}
	module := LoadConfigGCodeMove(&fakeGCodeMoveConfig{printer: printer}).(*GCodeMoveModule)
	if err := printer.events["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}

	legacyTransform := &fakeLegacyTransform{position: []float64{5, 6, 7, 8}}
	previous := module.Set_move_transform(legacyTransform, false)
	if previous == nil || previous.Get_position()[0] != 9 {
		t.Fatalf("expected previous transform to come from toolhead, got %#v", previous)
	}
	legacyTransform.position = []float64{2, 3, 4, 5}
	if err := module.Reset_last_position(nil); err != nil {
		t.Fatalf("Reset_last_position returned error: %v", err)
	}
	status := module.Get_status(0)
	position := status["position"].([]float64)
	if position[0] != 2 || position[1] != 3 || position[2] != 4 || position[3] != 5 {
		t.Fatalf("unexpected reset position: %#v", position)
	}

	bootstrapTransform := &fakeBootstrapTransform{position: []float64{6, 7, 8, 9}}
	bootstrapPrevious := module.SetMoveTransform(bootstrapTransform, true)
	if bootstrapPrevious == nil {
		t.Fatalf("expected previous transform from bootstrap adapter path")
	}
	if got := bootstrapPrevious.GetPosition(); got[0] != 2 || got[1] != 3 || got[2] != 4 || got[3] != 5 {
		t.Fatalf("unexpected bootstrap previous transform position: %#v", got)
	}
	bootstrapTransform.position = []float64{10, 11, 12, 13}
	module.ResetLastPosition()
	position = module.Get_status(0)["position"].([]float64)
	if position[0] != 10 || position[1] != 11 || position[2] != 12 || position[3] != 13 {
		t.Fatalf("unexpected bootstrap reset position: %#v", position)
	}
	if module.GCodePositionZ() != 12 {
		t.Fatalf("expected gcode Z position 12, got %v", module.GCodePositionZ())
	}
}

func TestGCodeMoveCmdGetPositionReportsStepperState(t *testing.T) {
	gcode := &fakeGCodeMoveGCode{}
	kinematics := &fakeKinematics{
		steppers: []interface{}{
			&fakeStepper{name: "stepper_x", mcuPosition: 10, commandedPos: 1.25},
			&fakeStepper{name: "stepper_y", mcuPosition: 20, commandedPos: 2.5},
		},
		position: []float64{1.25, 2.5, 3.75},
	}
	toolhead := &fakeToolhead{position: []float64{1.25, 2.5, 3.75, 4.0}, kin: kinematics}
	printer := &fakeGCodeMovePrinter{
		gcode:   gcode,
		objects: map[string]interface{}{"toolhead": toolhead},
	}
	module := LoadConfigGCodeMove(&fakeGCodeMoveConfig{printer: printer}).(*GCodeMoveModule)
	if err := printer.events["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}
	cmd := &fakeMoveCommand{}
	module.Cmd_GET_POSITION(cmd)
	if len(cmd.infoMessages) != 1 {
		t.Fatalf("expected one info response, got %d", len(cmd.infoMessages))
	}
	message := cmd.infoMessages[0]
	for _, want := range []string{"mcu: stepper_x:10 stepper_y:20", "stepper: stepper_x:1.250000 stepper_y:2.500000", "gcode homing:"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected %q in GET_POSITION output: %s", want, message)
		}
	}
}