package addon

import (
	printerpkg "goklipper/internal/pkg/printer"
	"testing"
)

type fakeExcludeTransform struct {
	position []float64
	moves    [][]float64
	speeds   []float64
}

type fakeExcludeCommand struct {
	strings   map[string]string
	params    map[string]string
	responses []string
	raw       []string
}

func (self *fakeExcludeCommand) String(name string, defaultValue string) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeExcludeCommand) Float(name string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeExcludeCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeExcludeCommand) Parameters() map[string]string {
	if self.params == nil {
		return map[string]string{}
	}
	return self.params
}

func (self *fakeExcludeCommand) RespondInfo(msg string, log bool) {
	self.responses = append(self.responses, msg)
}

func (self *fakeExcludeCommand) RespondRaw(msg string) {
	self.raw = append(self.raw, msg)
}

type fakeExcludeMutex struct{}

func (self *fakeExcludeMutex) Lock()   {}
func (self *fakeExcludeMutex) Unlock() {}

type fakeExcludeGCode struct {
	commands  map[string]func(printerpkg.Command) error
	scripts   []string
	responses []string
	mutex     printerpkg.Mutex
}

func (self *fakeExcludeGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeExcludeGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeExcludeGCode) RunScriptFromCommand(script string) {
	self.scripts = append(self.scripts, script)
}

func (self *fakeExcludeGCode) RunScript(script string) {
	self.scripts = append(self.scripts, script)
}

func (self *fakeExcludeGCode) IsBusy() bool {
	return false
}

func (self *fakeExcludeGCode) Mutex() printerpkg.Mutex {
	if self.mutex == nil {
		self.mutex = &fakeExcludeMutex{}
	}
	return self.mutex
}

func (self *fakeExcludeGCode) RespondInfo(msg string, log bool) {
	self.responses = append(self.responses, msg)
}

func (self *fakeExcludeGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return old
}

type fakeExcludeMoveController struct {
	base        printerpkg.MoveTransform
	current     printerpkg.MoveTransform
	state       printerpkg.GCodeMoveState
	linearMoves []map[string]string
	resetCount  int
}

func (self *fakeExcludeMoveController) SetMoveTransform(transform printerpkg.MoveTransform, force bool) printerpkg.MoveTransform {
	old := self.current
	if old == nil {
		old = self.base
	}
	self.current = transform
	return old
}

func (self *fakeExcludeMoveController) GCodePositionZ() float64 {
	return 0
}

func (self *fakeExcludeMoveController) State() printerpkg.GCodeMoveState {
	return self.state
}

func (self *fakeExcludeMoveController) LinearMove(params map[string]string) error {
	copyParams := map[string]string{}
	for key, value := range params {
		copyParams[key] = value
	}
	self.linearMoves = append(self.linearMoves, copyParams)
	return nil
}

func (self *fakeExcludeMoveController) ResetLastPosition() {
	self.resetCount++
	if self.base != nil {
		self.current = self.base
	}
}

type fakeExcludePrinter struct {
	gcode               printerpkg.GCodeRuntime
	gcodeMove           printerpkg.MoveTransformController
	lookup              map[string]interface{}
	eventHandlers       map[string]func([]interface{}) error
	currentExtruderName string
}

func (self *fakeExcludePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeExcludePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeExcludePrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeExcludePrinter) CurrentExtruderName() string {
	return self.currentExtruderName
}

func (self *fakeExcludePrinter) AddObject(name string, obj interface{}) error {
	if self.lookup == nil {
		self.lookup = map[string]interface{}{}
	}
	self.lookup[name] = obj
	return nil
}

func (self *fakeExcludePrinter) LookupObjects(module string) []interface{} {
	return nil
}

func (self *fakeExcludePrinter) HasStartArg(name string) bool {
	return false
}

func (self *fakeExcludePrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}

func (self *fakeExcludePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeExcludePrinter) LookupMCU(name string) printerpkg.MCURuntime {
	return nil
}

func (self *fakeExcludePrinter) InvokeShutdown(msg string) {}

func (self *fakeExcludePrinter) IsShutdown() bool {
	return false
}

func (self *fakeExcludePrinter) Reactor() printerpkg.ModuleReactor {
	return nil
}

func (self *fakeExcludePrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return nil
}

func (self *fakeExcludePrinter) GCode() printerpkg.GCodeRuntime {
	return self.gcode
}

func (self *fakeExcludePrinter) GCodeMove() printerpkg.MoveTransformController {
	return self.gcodeMove
}

func (self *fakeExcludePrinter) Webhooks() printerpkg.WebhookRegistry {
	return nil
}

type fakeExcludeConfig struct {
	printer printerpkg.ModulePrinter
}

func (self *fakeExcludeConfig) Name() string {
	return "exclude_object"
}

func (self *fakeExcludeConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeExcludeConfig) Bool(option string, defaultValue bool) bool {
	return defaultValue
}

func (self *fakeExcludeConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeExcludeConfig) OptionalFloat(option string) *float64 {
	return nil
}

func (self *fakeExcludeConfig) LoadObject(section string) interface{} {
	return nil
}

func (self *fakeExcludeConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeExcludeConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeExcludeConfig) Printer() printerpkg.ModulePrinter {
	return self.printer
}

func (self *fakeExcludeTransform) GetPosition() []float64 {
	pos := make([]float64, len(self.position))
	copy(pos, self.position)
	return pos
}

func (self *fakeExcludeTransform) Move(newpos []float64, speed float64) {
	pos := make([]float64, len(newpos))
	copy(pos, newpos)
	self.position = pos
	self.moves = append(self.moves, pos)
	self.speeds = append(self.speeds, speed)
}

func TestExcludeObjectStateBookkeeping(t *testing.T) {
	core := NewExcludeObject()
	core.AddObjectDefinition(map[string]interface{}{"name": "B"})
	core.AddObjectDefinition(map[string]interface{}{"name": "A"})
	if got := core.Objects()[0]["name"].(string); got != "A" {
		t.Fatalf("objects should be sorted, got %#v", core.Objects())
	}

	core.StartObject("A")
	if core.CurrentObject() != "A" {
		t.Fatalf("unexpected current object: %q", core.CurrentObject())
	}
	core.Exclude("A")
	if !core.ObjectIsExcluded("A") {
		t.Fatalf("expected object to be excluded")
	}
	status := core.GetStatus()
	if status["current_object"].(string) != "A" {
		t.Fatalf("unexpected status: %#v", status)
	}
	core.Unexclude("A")
	if core.ObjectIsExcluded("A") {
		t.Fatalf("expected object to be unexcluded")
	}
	if msg := core.EndObject("A"); msg != "" {
		t.Fatalf("unexpected end message: %q", msg)
	}
}

func TestExcludeObjectSuppressesExcludedMoves(t *testing.T) {
	transform := &fakeExcludeTransform{position: []float64{0, 0, 0, 0}}
	core := NewExcludeObject()
	core.AttachTransform(transform, "extruder")

	core.Move([]float64{1, 0, 0, 1}, 100, "extruder")
	if len(transform.moves) != 1 {
		t.Fatalf("expected initial move to reach transform, got %d", len(transform.moves))
	}

	core.StartObject("OBJ")
	core.Exclude("OBJ")
	for i := 0; i < 5; i++ {
		step := float64(i + 2)
		core.Move([]float64{step, 0, 0, step}, 100, "extruder")
	}
	if len(transform.moves) != 5 {
		t.Fatalf("expected initialization moves to pass through, got %d", len(transform.moves))
	}

	core.Move([]float64{10, 0, 0, 10}, 100, "extruder")
	if len(transform.moves) != 5 {
		t.Fatalf("expected excluded move to be suppressed, got %d moves", len(transform.moves))
	}

	core.EndObject("OBJ")
	core.Move([]float64{11, 0, 0, 11}, 100, "extruder")
	if len(transform.moves) != 6 {
		t.Fatalf("expected move after excluded region to resume, got %d moves", len(transform.moves))
	}
	if got := transform.moves[5][0]; got != 11 {
		t.Fatalf("unexpected transformed x position after resume: %v", got)
	}
	if got := transform.moves[5][3]; got != 6 {
		t.Fatalf("unexpected transformed e position after resume: %v", got)
	}
	if transform.speeds[5] != 100 {
		t.Fatalf("unexpected resumed move speed: %v", transform.speeds[5])
	}
}

func TestExcludeObjectModuleRegistersCommandsAndResets(t *testing.T) {
	baseTransform := &fakeExcludeTransform{position: []float64{0, 0, 0, 0}}
	moveController := &fakeExcludeMoveController{base: baseTransform, current: baseTransform}
	gcode := &fakeExcludeGCode{}
	printer := &fakeExcludePrinter{
		gcode:               gcode,
		gcodeMove:           moveController,
		lookup:              map[string]interface{}{},
		currentExtruderName: "extruder",
	}

	loaded := LoadConfigExcludeObject(&fakeExcludeConfig{printer: printer})
	module, ok := loaded.(*ExcludeObjectModule)
	if !ok || module == nil {
		t.Fatalf("expected exclude object module, got %#v", loaded)
	}

	for _, cmd := range []string{
		"EXCLUDE_OBJECT_START",
		"EXCLUDE_OBJECT_END",
		"EXCLUDE_OBJECT",
		"EXCLUDE_OBJECT_DEFINE",
		"EXCLUDE_OBJECT_END_NO_OBJ",
		"EXCLUDE_OBJECT_SET_OBJ",
	} {
		if _, ok := gcode.commands[cmd]; !ok {
			t.Fatalf("expected command %s to be registered", cmd)
		}
	}
	if _, ok := printer.eventHandlers["virtual_sdcard:reset_file"]; !ok {
		t.Fatalf("expected virtual_sdcard reset handler to be registered")
	}

	if err := module.cmdExcludeObjectDefine(&fakeExcludeCommand{strings: map[string]string{"NAME": "part"}}); err != nil {
		t.Fatalf("cmdExcludeObjectDefine returned error: %v", err)
	}
	if len(module.Objects()) != 1 {
		t.Fatalf("expected one object definition, got %#v", module.Objects())
	}

	if err := module.cmdExcludeObject(&fakeExcludeCommand{strings: map[string]string{"NAME": "PART"}}); err != nil {
		t.Fatalf("cmdExcludeObject returned error: %v", err)
	}
	if !module.objectIsExcluded("PART") {
		t.Fatalf("expected PART to be excluded")
	}
	if moveController.current != module {
		t.Fatalf("expected exclude_object module to become active transform")
	}

	if err := module.handleResetFile(nil); err != nil {
		t.Fatalf("handleResetFile returned error: %v", err)
	}
	if len(module.Objects()) != 0 {
		t.Fatalf("expected reset to clear objects, got %#v", module.Objects())
	}
	if len(module.ExcludedObjects()) != 0 {
		t.Fatalf("expected reset to clear excluded objects, got %#v", module.ExcludedObjects())
	}
	if moveController.resetCount != 1 {
		t.Fatalf("expected ResetLastPosition to be called once, got %d", moveController.resetCount)
	}
	if moveController.current != baseTransform {
		t.Fatalf("expected base transform to be restored")
	}
}
