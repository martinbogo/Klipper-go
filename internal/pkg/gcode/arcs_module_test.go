package gcode

import (
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeArcCommand struct {
	floats map[string]float64
	params map[string]string
	raw    []string
}

func (self *fakeArcCommand) String(name string, defaultValue string) string {
	return defaultValue
}

func (self *fakeArcCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeArcCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeArcCommand) Parameters() map[string]string {
	if self.params == nil {
		return map[string]string{}
	}
	return self.params
}

func (self *fakeArcCommand) RespondInfo(msg string, log bool) {}

func (self *fakeArcCommand) RespondRaw(msg string) {
	self.raw = append(self.raw, msg)
}

type fakeArcMutex struct{}

func (self *fakeArcMutex) Lock()   {}
func (self *fakeArcMutex) Unlock() {}

type fakeArcGCode struct {
	commands map[string]func(printerpkg.Command) error
	mutex    printerpkg.Mutex
}

func (self *fakeArcGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeArcGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeArcGCode) RunScriptFromCommand(script string) {}

func (self *fakeArcGCode) RunScript(script string) {}

func (self *fakeArcGCode) IsBusy() bool {
	return false
}

func (self *fakeArcGCode) Mutex() printerpkg.Mutex {
	if self.mutex == nil {
		self.mutex = &fakeArcMutex{}
	}
	return self.mutex
}

func (self *fakeArcGCode) RespondInfo(msg string, log bool) {}

func (self *fakeArcGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return old
}

type fakeArcMoveController struct {
	state       printerpkg.GCodeMoveState
	linearMoves []map[string]string
	resetCount  int
}

func (self *fakeArcMoveController) SetMoveTransform(transform printerpkg.MoveTransform, force bool) printerpkg.MoveTransform {
	return nil
}

func (self *fakeArcMoveController) GCodePositionZ() float64 {
	return self.state.GCodePosition[2]
}

func (self *fakeArcMoveController) State() printerpkg.GCodeMoveState {
	position := make([]float64, len(self.state.GCodePosition))
	copy(position, self.state.GCodePosition)
	return printerpkg.GCodeMoveState{
		GCodePosition:       position,
		AbsoluteCoordinates: self.state.AbsoluteCoordinates,
		AbsoluteExtrude:     self.state.AbsoluteExtrude,
	}
}

func (self *fakeArcMoveController) LinearMove(params map[string]string) error {
	copyParams := map[string]string{}
	for key, value := range params {
		copyParams[key] = value
	}
	self.linearMoves = append(self.linearMoves, copyParams)
	return nil
}

func (self *fakeArcMoveController) ResetLastPosition() {
	self.resetCount++
}

type fakeArcPrinter struct {
	gcode     printerpkg.GCodeRuntime
	gcodeMove printerpkg.MoveTransformController
}

func (self *fakeArcPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	return defaultValue
}

func (self *fakeArcPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {}

func (self *fakeArcPrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeArcPrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeArcPrinter) AddObject(name string, obj interface{}) error { return nil }

func (self *fakeArcPrinter) LookupObjects(module string) []interface{} { return nil }

func (self *fakeArcPrinter) HasStartArg(name string) bool { return false }

func (self *fakeArcPrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeArcPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }

func (self *fakeArcPrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }

func (self *fakeArcPrinter) InvokeShutdown(msg string) {}

func (self *fakeArcPrinter) IsShutdown() bool { return false }

func (self *fakeArcPrinter) Reactor() printerpkg.ModuleReactor { return nil }

func (self *fakeArcPrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeArcPrinter) GCode() printerpkg.GCodeRuntime { return self.gcode }

func (self *fakeArcPrinter) GCodeMove() printerpkg.MoveTransformController { return self.gcodeMove }

func (self *fakeArcPrinter) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeArcConfig struct {
	printer    printerpkg.ModulePrinter
	resolution float64
}

func (self *fakeArcConfig) Name() string { return "gcode_arcs" }

func (self *fakeArcConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeArcConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeArcConfig) Float(option string, defaultValue float64) float64 {
	if option == "resolution" {
		return self.resolution
	}
	return defaultValue
}

func (self *fakeArcConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeArcConfig) LoadObject(section string) interface{} { return nil }

func (self *fakeArcConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeArcConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeArcConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigArcSupportRegistersCommands(t *testing.T) {
	gcode := &fakeArcGCode{}
	move := &fakeArcMoveController{state: printerpkg.GCodeMoveState{GCodePosition: []float64{0, 0, 0, 0}, AbsoluteCoordinates: true, AbsoluteExtrude: true}}
	printer := &fakeArcPrinter{gcode: gcode, gcodeMove: move}
	module := LoadConfigArcSupport(&fakeArcConfig{printer: printer, resolution: 1.0}).(*ArcSupportModule)
	if module.core.Plane() != ArcPlaneXY {
		t.Fatalf("expected default XY plane, got %d", module.core.Plane())
	}
	for _, cmd := range []string{"G2", "G3", "G17", "G18", "G19"} {
		if _, ok := gcode.commands[cmd]; !ok {
			t.Fatalf("expected %s to be registered", cmd)
		}
	}
	_ = module.cmdG18(&fakeArcCommand{})
	if module.core.Plane() != ArcPlaneXZ {
		t.Fatalf("expected XZ plane after G18")
	}
	_ = module.cmdG19(&fakeArcCommand{})
	if module.core.Plane() != ArcPlaneYZ {
		t.Fatalf("expected YZ plane after G19")
	}
	_ = module.cmdG17(&fakeArcCommand{})
	if module.core.Plane() != ArcPlaneXY {
		t.Fatalf("expected XY plane after G17")
	}
}

func TestArcSupportModuleGeneratesLinearMoves(t *testing.T) {
	gcode := &fakeArcGCode{}
	move := &fakeArcMoveController{state: printerpkg.GCodeMoveState{GCodePosition: []float64{0, 0, 0, 0}, AbsoluteCoordinates: true, AbsoluteExtrude: true}}
	printer := &fakeArcPrinter{gcode: gcode, gcodeMove: move}
	module := LoadConfigArcSupport(&fakeArcConfig{printer: printer, resolution: 1.0}).(*ArcSupportModule)
	cmd := &fakeArcCommand{
		floats: map[string]float64{
			"X": 1,
			"Y": 1,
			"I": 1,
			"J": 0,
			"F": 1200,
		},
		params: map[string]string{"X": "1", "Y": "1", "I": "1", "J": "0", "F": "1200"},
	}
	if err := module.cmdG2(cmd); err != nil {
		t.Fatalf("cmdG2 returned error: %v", err)
	}
	if len(move.linearMoves) == 0 {
		t.Fatalf("expected generated G1 moves")
	}
	last := move.linearMoves[len(move.linearMoves)-1]
	if last["X"] != "1" || last["Y"] != "1" || last["F"] != "1200" {
		t.Fatalf("unexpected final linear move: %#v", last)
	}
}
