package motion

import (
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeForceMoveCommand struct {
	strings   map[string]string
	floats    map[string]float64
	responses []string
	raw       []string
}

func (self *fakeForceMoveCommand) String(name string, defaultValue string) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeForceMoveCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeForceMoveCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeForceMoveCommand) Parameters() map[string]string {
	return map[string]string{}
}

func (self *fakeForceMoveCommand) RespondInfo(msg string, log bool) {
	self.responses = append(self.responses, msg)
}

func (self *fakeForceMoveCommand) RespondRaw(msg string) {
	self.raw = append(self.raw, msg)
}

type fakeForceMoveMutex struct{}

func (self *fakeForceMoveMutex) Lock()   {}
func (self *fakeForceMoveMutex) Unlock() {}

type fakeForceMoveGCode struct {
	commands map[string]func(printerpkg.Command) error
	mutex    printerpkg.Mutex
}

func (self *fakeForceMoveGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeForceMoveGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeForceMoveGCode) RunScriptFromCommand(script string) {}

func (self *fakeForceMoveGCode) RunScript(script string) {}

func (self *fakeForceMoveGCode) IsBusy() bool {
	return false
}

func (self *fakeForceMoveGCode) Mutex() printerpkg.Mutex {
	if self.mutex == nil {
		self.mutex = &fakeForceMoveMutex{}
	}
	return self.mutex
}

func (self *fakeForceMoveGCode) RespondInfo(msg string, log bool) {}

func (self *fakeForceMoveGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return old
}

type fakeForceMoveEnableLine struct {
	enabled      bool
	enableCalls  []float64
	disableCalls []float64
}

func (self *fakeForceMoveEnableLine) MotorEnable(printTime float64) {
	self.enabled = true
	self.enableCalls = append(self.enableCalls, printTime)
}

func (self *fakeForceMoveEnableLine) MotorDisable(printTime float64) {
	self.enabled = false
	self.disableCalls = append(self.disableCalls, printTime)
}

func (self *fakeForceMoveEnableLine) IsMotorEnabled() bool {
	return self.enabled
}

type fakeForceMoveStepperEnable struct {
	lines map[string]*fakeForceMoveEnableLine
}

func (self *fakeForceMoveStepperEnable) LookupEnable(name string) (printerpkg.StepperEnableLine, error) {
	return self.lines[name], nil
}

func (self *fakeForceMoveStepperEnable) StepperNames() []string {
	names := make([]string, 0, len(self.lines))
	for name := range self.lines {
		names = append(names, name)
	}
	return names
}

type fakeForceMoveToolhead struct {
	position      []float64
	flushCount    int
	lastMoveTime  float64
	dwellCalls    []float64
	queueActivity []float64
	setPositions  [][]float64
}

func (self *fakeForceMoveToolhead) Flush_step_generation() {
	self.flushCount++
}

func (self *fakeForceMoveToolhead) Get_last_move_time() float64 {
	return self.lastMoveTime
}

func (self *fakeForceMoveToolhead) Note_mcu_movequeue_activity(mqTime float64, setStepGenTime bool) {
	self.queueActivity = append(self.queueActivity, mqTime)
	self.lastMoveTime = mqTime
}

func (self *fakeForceMoveToolhead) Dwell(delay float64) {
	self.dwellCalls = append(self.dwellCalls, delay)
}

func (self *fakeForceMoveToolhead) Get_position() []float64 {
	pos := make([]float64, len(self.position))
	copy(pos, self.position)
	return pos
}

func (self *fakeForceMoveToolhead) Set_position(newpos []float64, homingAxes []int) {
	pos := make([]float64, len(newpos))
	copy(pos, newpos)
	self.position = pos
	self.setPositions = append(self.setPositions, pos)
}

type fakeForceMoveStepper struct {
	name             string
	unitsInRadians   bool
	pulseDuration    interface{}
	stepBothEdge     bool
	position         []float64
	setKinematics    int
	setTrapq         int
	generateSteps    []float64
	mcuPosition      int
	commandedPosFrom int
}

func (self *fakeForceMoveStepper) Get_name(short bool) string {
	return self.name
}

func (self *fakeForceMoveStepper) Units_in_radians() bool {
	return self.unitsInRadians
}

func (self *fakeForceMoveStepper) Setup_default_pulse_duration(pulseduration interface{}, stepBothEdge bool) {
	self.pulseDuration = pulseduration
	self.stepBothEdge = stepBothEdge
}

func (self *fakeForceMoveStepper) Get_pulse_duration() (interface{}, bool) {
	return self.pulseDuration, self.stepBothEdge
}

func (self *fakeForceMoveStepper) Mcu_to_commanded_position(mcuPos int) float64 {
	self.commandedPosFrom = mcuPos
	return float64(mcuPos) * 0.1
}

func (self *fakeForceMoveStepper) Get_dir_inverted() (uint32, uint32) {
	return 0, 0
}

func (self *fakeForceMoveStepper) Get_mcu_position() int {
	return self.mcuPosition
}

func (self *fakeForceMoveStepper) Set_stepper_kinematics(sk interface{}) interface{} {
	self.setKinematics++
	return nil
}

func (self *fakeForceMoveStepper) Set_trapq(tq interface{}) interface{} {
	self.setTrapq++
	return nil
}

func (self *fakeForceMoveStepper) Set_position(coord []float64) {
	self.position = append([]float64{}, coord...)
}

func (self *fakeForceMoveStepper) Generate_steps(flushTime float64) {
	self.generateSteps = append(self.generateSteps, flushTime)
}

type fakeForceMovePrinter struct {
	gcode         printerpkg.GCodeRuntime
	stepperEnable printerpkg.StepperEnableRuntime
	lookup        map[string]interface{}
}

func (self *fakeForceMovePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeForceMovePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}

func (self *fakeForceMovePrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeForceMovePrinter) CurrentExtruderName() string {
	return "extruder"
}

func (self *fakeForceMovePrinter) AddObject(name string, obj interface{}) error {
	if self.lookup == nil {
		self.lookup = map[string]interface{}{}
	}
	self.lookup[name] = obj
	return nil
}

func (self *fakeForceMovePrinter) LookupObjects(module string) []interface{} {
	return nil
}

func (self *fakeForceMovePrinter) HasStartArg(name string) bool {
	return false
}

func (self *fakeForceMovePrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}

func (self *fakeForceMovePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeForceMovePrinter) LookupMCU(name string) printerpkg.MCURuntime {
	return nil
}

func (self *fakeForceMovePrinter) InvokeShutdown(msg string) {}

func (self *fakeForceMovePrinter) IsShutdown() bool {
	return false
}

func (self *fakeForceMovePrinter) Reactor() printerpkg.ModuleReactor {
	return nil
}

func (self *fakeForceMovePrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return self.stepperEnable
}

func (self *fakeForceMovePrinter) GCode() printerpkg.GCodeRuntime {
	return self.gcode
}

func (self *fakeForceMovePrinter) GCodeMove() printerpkg.MoveTransformController {
	return nil
}

func (self *fakeForceMovePrinter) Webhooks() printerpkg.WebhookRegistry {
	return nil
}

type fakeForceMoveConfig struct {
	printer         printerpkg.ModulePrinter
	enableForceMove bool
}

func (self *fakeForceMoveConfig) Name() string {
	return "force_move"
}

func (self *fakeForceMoveConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeForceMoveConfig) Bool(option string, defaultValue bool) bool {
	if option == "enable_force_move" {
		return self.enableForceMove
	}
	return defaultValue
}

func (self *fakeForceMoveConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeForceMoveConfig) OptionalFloat(option string) *float64 {
	return nil
}

func (self *fakeForceMoveConfig) LoadObject(section string) interface{} {
	return nil
}

func (self *fakeForceMoveConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeForceMoveConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeForceMoveConfig) Printer() printerpkg.ModulePrinter {
	return self.printer
}

func buildForceMoveModule(enableForceMove bool) (*ForceMoveModule, *fakeForceMoveGCode, *fakeForceMoveToolhead, *fakeForceMoveEnableLine) {
	gcode := &fakeForceMoveGCode{}
	toolhead := &fakeForceMoveToolhead{position: []float64{1, 2, 3, 4}, lastMoveTime: 5}
	enableLine := &fakeForceMoveEnableLine{}
	printer := &fakeForceMovePrinter{
		gcode:         gcode,
		stepperEnable: &fakeForceMoveStepperEnable{lines: map[string]*fakeForceMoveEnableLine{"stepper_x": enableLine}},
		lookup:        map[string]interface{}{"toolhead": toolhead},
	}
	module := LoadConfigForceMove(&fakeForceMoveConfig{printer: printer, enableForceMove: enableForceMove}).(*ForceMoveModule)
	module.core = &ForceMover{
		trapq: struct{}{},
		trapqAppend: func(tq interface{}, printTime, accelT, cruiseT, decelT, startPosX, startPosY, startPosZ, axesRX, axesRY, axesRZ, startV, cruiseV, accel float64) {
		},
		trapqFinalizeMoves: func(interface{}, float64, float64) {},
		stepperKinematics:  struct{}{},
	}
	return module, gcode, toolhead, enableLine
}

func TestLoadConfigForceMoveRegistersCommands(t *testing.T) {
	module, gcode, _, _ := buildForceMoveModule(true)
	if module == nil {
		t.Fatalf("expected module instance")
	}
	for _, cmd := range []string{"STEPPER_BUZZ", "FORCE_MOVE", "SET_KINEMATIC_POSITION"} {
		if _, ok := gcode.commands[cmd]; !ok {
			t.Fatalf("expected %s to be registered", cmd)
		}
	}

	_, gcodeDisabled, _, _ := buildForceMoveModule(false)
	if _, ok := gcodeDisabled.commands["STEPPER_BUZZ"]; !ok {
		t.Fatalf("expected STEPPER_BUZZ to always be registered")
	}
	if _, ok := gcodeDisabled.commands["FORCE_MOVE"]; ok {
		t.Fatalf("did not expect FORCE_MOVE when disabled")
	}
	if _, ok := gcodeDisabled.commands["SET_KINEMATIC_POSITION"]; ok {
		t.Fatalf("did not expect SET_KINEMATIC_POSITION when disabled")
	}
}

func TestForceMoveModuleCommandsAndStepperLookup(t *testing.T) {
	module, _, toolhead, enableLine := buildForceMoveModule(true)
	stepper := &fakeForceMoveStepper{name: "stepper_x"}
	module.RegisterStepper(stepper)

	if got := module.LookupStepper("stepper_x"); got != stepper {
		t.Fatalf("unexpected stepper lookup result: %#v", got)
	}
	if got := module.Lookup_stepper("stepper_x"); got != stepper {
		t.Fatalf("unexpected compatibility lookup result: %#v", got)
	}

	if err := module.cmdForceMove(&fakeForceMoveCommand{
		strings: map[string]string{"STEPPER": "stepper_x"},
		floats:  map[string]float64{"DISTANCE": 12, "VELOCITY": 6, "ACCEL": 3},
	}); err != nil {
		t.Fatalf("cmdForceMove returned error: %v", err)
	}
	if len(enableLine.enableCalls) != 1 {
		t.Fatalf("expected force enable call, got %#v", enableLine.enableCalls)
	}
	if len(stepper.generateSteps) != 1 {
		t.Fatalf("expected generated steps, got %#v", stepper.generateSteps)
	}
	if toolhead.flushCount == 0 {
		t.Fatalf("expected toolhead flushes during manual move")
	}

	if err := module.cmdSetKinematicPosition(&fakeForceMoveCommand{floats: map[string]float64{"X": 9, "Y": 8, "Z": 7}}); err != nil {
		t.Fatalf("cmdSetKinematicPosition returned error: %v", err)
	}
	if len(toolhead.setPositions) != 1 {
		t.Fatalf("expected set position call, got %#v", toolhead.setPositions)
	}
	if got := toolhead.setPositions[0]; got[0] != 9 || got[1] != 8 || got[2] != 7 || got[3] != 4 {
		t.Fatalf("unexpected kinematic position: %#v", got)
	}
}

func TestStepperBuzzRestoresMotorEnable(t *testing.T) {
	module, _, _, enableLine := buildForceMoveModule(true)
	stepper := &fakeForceMoveStepper{name: "stepper_x"}
	module.RegisterStepper(stepper)

	if err := module.cmdStepperBuzz(&fakeForceMoveCommand{strings: map[string]string{"STEPPER": "stepper_x"}}); err != nil {
		t.Fatalf("cmdStepperBuzz returned error: %v", err)
	}
	if len(enableLine.enableCalls) != 1 {
		t.Fatalf("expected one motor enable, got %#v", enableLine.enableCalls)
	}
	if len(enableLine.disableCalls) != 1 {
		t.Fatalf("expected one motor disable, got %#v", enableLine.disableCalls)
	}
	if len(stepper.generateSteps) != 20 {
		t.Fatalf("expected 20 buzz moves, got %d", len(stepper.generateSteps))
	}
}
