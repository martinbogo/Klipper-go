package print

import (
	"goklipper/common/constants"
	printerpkg "goklipper/internal/pkg/printer"
	"testing"
)

type fakeIdleTemplate struct {
	script string
	err    error
}

func (self *fakeIdleTemplate) CreateContext(eventtime interface{}) map[string]interface{} {
	return map[string]interface{}{}
}

func (self *fakeIdleTemplate) Render(context map[string]interface{}) (string, error) {
	return self.script, self.err
}

func (self *fakeIdleTemplate) RunGcodeFromCommand(context map[string]interface{}) error {
	return nil
}

type fakeIdleTimer struct {
	updated []float64
}

func (self *fakeIdleTimer) Update(waketime float64) {
	self.updated = append(self.updated, waketime)
}

type fakeIdleReactor struct {
	monotonic float64
	timer     *fakeIdleTimer
}

func (self *fakeIdleReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return self.timer
}

func (self *fakeIdleReactor) Monotonic() float64 {
	return self.monotonic
}

type fakeIdleCommand struct {
	floatValues map[string]float64
	responses   []string
	raw         []string
}

func (self *fakeIdleCommand) String(name string, defaultValue string) string {
	return defaultValue
}

func (self *fakeIdleCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floatValues[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeIdleCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeIdleCommand) Parameters() map[string]string {
	return map[string]string{}
}

func (self *fakeIdleCommand) RespondInfo(msg string, log bool) {
	self.responses = append(self.responses, msg)
}

func (self *fakeIdleCommand) RespondRaw(msg string) {
	self.raw = append(self.raw, msg)
}

type fakeIdleMutex struct{}

func (self *fakeIdleMutex) Lock()   {}
func (self *fakeIdleMutex) Unlock() {}

type fakeIdleGCode struct {
	busy      bool
	scripts   []string
	commands  map[string]func(printerpkg.Command) error
	responses []string
	mutex     printerpkg.Mutex
}

func (self *fakeIdleGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeIdleGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeIdleGCode) RunScriptFromCommand(script string) {
	self.scripts = append(self.scripts, script)
}

func (self *fakeIdleGCode) RunScript(script string) {
	self.scripts = append(self.scripts, script)
}

func (self *fakeIdleGCode) IsBusy() bool {
	return self.busy
}

func (self *fakeIdleGCode) Mutex() printerpkg.Mutex {
	if self.mutex == nil {
		self.mutex = &fakeIdleMutex{}
	}
	return self.mutex
}

func (self *fakeIdleGCode) RespondInfo(msg string, log bool) {
	self.responses = append(self.responses, msg)
}

func (self *fakeIdleGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return old
}

type fakeIdleToolhead struct {
	lastMove       float64
	printTime      float64
	estPrintTime   float64
	lookaheadEmpty bool
}

func (self *fakeIdleToolhead) Get_last_move_time() float64 {
	return self.lastMove
}

func (self *fakeIdleToolhead) Check_busy(eventtime float64) (float64, float64, bool) {
	return self.printTime, self.estPrintTime, self.lookaheadEmpty
}

type fakeIdlePrinter struct {
	reactor       printerpkg.ModuleReactor
	gcode         printerpkg.GCodeRuntime
	lookup        map[string]interface{}
	eventHandlers map[string]func([]interface{}) error
	events        []string
	eventArgs     [][]interface{}
}

func (self *fakeIdlePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeIdlePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeIdlePrinter) SendEvent(event string, params []interface{}) {
	self.events = append(self.events, event)
	self.eventArgs = append(self.eventArgs, params)
}

func (self *fakeIdlePrinter) CurrentExtruderName() string {
	return "extruder"
}

func (self *fakeIdlePrinter) AddObject(name string, obj interface{}) error {
	if self.lookup == nil {
		self.lookup = map[string]interface{}{}
	}
	self.lookup[name] = obj
	return nil
}

func (self *fakeIdlePrinter) LookupObjects(module string) []interface{} {
	return nil
}

func (self *fakeIdlePrinter) HasStartArg(name string) bool {
	return false
}

func (self *fakeIdlePrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}

func (self *fakeIdlePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeIdlePrinter) LookupMCU(name string) printerpkg.MCURuntime {
	return nil
}

func (self *fakeIdlePrinter) InvokeShutdown(msg string) {}

func (self *fakeIdlePrinter) IsShutdown() bool {
	return false
}

func (self *fakeIdlePrinter) Reactor() printerpkg.ModuleReactor {
	return self.reactor
}

func (self *fakeIdlePrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return nil
}

func (self *fakeIdlePrinter) GCode() printerpkg.GCodeRuntime {
	return self.gcode
}

func (self *fakeIdlePrinter) GCodeMove() printerpkg.MoveTransformController {
	return nil
}

func (self *fakeIdlePrinter) Webhooks() printerpkg.WebhookRegistry {
	return nil
}

type fakeIdleConfig struct {
	printer  printerpkg.ModulePrinter
	timeout  float64
	template printerpkg.Template
}

func (self *fakeIdleConfig) Name() string {
	return "idle_timeout"
}

func (self *fakeIdleConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeIdleConfig) Bool(option string, defaultValue bool) bool {
	return defaultValue
}

func (self *fakeIdleConfig) Float(option string, defaultValue float64) float64 {
	if option == "timeout" {
		return self.timeout
	}
	return defaultValue
}

func (self *fakeIdleConfig) OptionalFloat(option string) *float64 {
	return nil
}

func (self *fakeIdleConfig) LoadObject(section string) interface{} {
	return nil
}

func (self *fakeIdleConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return self.template
}

func (self *fakeIdleConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return self.template
}

func (self *fakeIdleConfig) Printer() printerpkg.ModulePrinter {
	return self.printer
}

func TestIdleTimeoutSyncAndStatus(t *testing.T) {
	core := NewIdleTimeout(30)

	decision, changed := core.SyncPrintTime(10, 4, 6)
	if !changed {
		t.Fatalf("expected sync transition to change state")
	}
	if core.State() != StatePrinting {
		t.Fatalf("unexpected state after sync: %q", core.State())
	}
	if decision.EventName != "idle_timeout:printing" {
		t.Fatalf("unexpected event name: %q", decision.EventName)
	}
	if decision.NextWake != 12.5 {
		t.Fatalf("unexpected next wake: %v", decision.NextWake)
	}

	status := core.GetStatus(13)
	if status["state"].(string) != StatePrinting {
		t.Fatalf("unexpected status state: %#v", status)
	}
	if status["printing_time"].(float64) != 3 {
		t.Fatalf("unexpected printing time: %#v", status)
	}

	_, changed = core.SyncPrintTime(14, 7, 9)
	if changed {
		t.Fatalf("second sync should not re-enter printing state")
	}
}

func TestIdleTimeoutReadyAndIdleTransitions(t *testing.T) {
	core := NewIdleTimeout(20)

	decision := core.TimeoutHandler(100, 5, 6, true, false)
	if core.State() != StateReady {
		t.Fatalf("expected ready state, got %q", core.State())
	}
	if decision.EventName != "idle_timeout:ready" {
		t.Fatalf("unexpected ready event: %#v", decision)
	}
	if decision.NextWake != 120 {
		t.Fatalf("unexpected ready wake: %v", decision.NextWake)
	}
	if got := decision.EventArgs[0].(float64); got != 6+PinMinTime {
		t.Fatalf("unexpected ready event arg: %v", got)
	}

	decision = core.CheckIdleTimeout(200, 5, 40, true, false)
	if !decision.EnterIdle {
		t.Fatalf("expected idle entry decision, got %#v", decision)
	}

	core.BeginIdleTransition()
	completed := core.CompleteIdleTransition(42)
	if core.State() != StateIdle {
		t.Fatalf("expected idle state after completion, got %q", core.State())
	}
	if completed.NextWake != constants.NEVER {
		t.Fatalf("unexpected completed wake: %v", completed.NextWake)
	}
	if completed.EventName != "idle_timeout:idle" {
		t.Fatalf("unexpected idle event: %#v", completed)
	}
	if completed.EventArgs[0].(float64) != 42 {
		t.Fatalf("unexpected idle event arg: %#v", completed.EventArgs)
	}
}

func TestIdleTimeoutFailedIdleTransitionAndTimeoutUpdate(t *testing.T) {
	core := NewIdleTimeout(15)
	core.BeginIdleTransition()
	core.FailIdleTransition()
	if core.State() != StateReady {
		t.Fatalf("expected ready state after failed transition, got %q", core.State())
	}

	core.SetTimeout(99)
	if core.Timeout() != 99 {
		t.Fatalf("unexpected timeout value: %v", core.Timeout())
	}

	decision := core.CheckIdleTimeout(10, 10, 10.5, false, false)
	if decision.NextWake != 109 {
		t.Fatalf("unexpected busy wake: %#v", decision)
	}

	decision = core.CheckIdleTimeout(10, 10, 120, true, true)
	if decision.NextWake != 11 {
		t.Fatalf("unexpected gcode-busy wake: %#v", decision)
	}
}

func TestIdleTimeoutModuleFlow(t *testing.T) {
	timer := &fakeIdleTimer{}
	reactor := &fakeIdleReactor{monotonic: 50, timer: timer}
	gcode := &fakeIdleGCode{}
	toolhead := &fakeIdleToolhead{lastMove: 42, printTime: 5, estPrintTime: 6, lookaheadEmpty: true}
	printer := &fakeIdlePrinter{
		reactor: reactor,
		gcode:   gcode,
		lookup:  map[string]interface{}{"toolhead": toolhead},
	}
	config := &fakeIdleConfig{
		printer:  printer,
		timeout:  30,
		template: &fakeIdleTemplate{script: "M84"},
	}

	module := NewIdleTimeoutModule(config, printer, reactor, gcode, nil)
	if _, ok := gcode.commands["SET_IDLE_TIMEOUT"]; !ok {
		t.Fatalf("expected SET_IDLE_TIMEOUT command registration")
	}
	if err := module.Handle_ready(nil); err != nil {
		t.Fatalf("Handle_ready returned error: %v", err)
	}

	if err := module.Handle_sync_print_time([]interface{}{10., 4., 6.}); err != nil {
		t.Fatalf("Handle_sync_print_time returned error: %v", err)
	}
	if len(timer.updated) == 0 || timer.updated[0] != 12.5 {
		t.Fatalf("unexpected sync timer updates: %#v", timer.updated)
	}
	if len(printer.events) == 0 || printer.events[0] != "idle_timeout:printing" {
		t.Fatalf("unexpected sync events: %#v", printer.events)
	}

	readyWake := module.Timeout_handler(100)
	if readyWake != 130 {
		t.Fatalf("unexpected ready wake: %v", readyWake)
	}
	if len(printer.events) < 2 || printer.events[1] != "idle_timeout:ready" {
		t.Fatalf("unexpected ready events: %#v", printer.events)
	}

	toolhead.estPrintTime = 40
	idleWake := module.Timeout_handler(200)
	if idleWake != constants.NEVER {
		t.Fatalf("unexpected idle wake: %v", idleWake)
	}
	if len(gcode.scripts) != 1 || gcode.scripts[0] != "M84" {
		t.Fatalf("unexpected scripts: %#v", gcode.scripts)
	}
	if len(printer.events) < 3 || printer.events[2] != "idle_timeout:idle" {
		t.Fatalf("unexpected idle events: %#v", printer.events)
	}
	if got := printer.eventArgs[2][0].(float64); got != 42 {
		t.Fatalf("unexpected idle event args: %#v", printer.eventArgs[2])
	}
}

func TestIdleTimeoutModuleCommandUpdatesTimerWhenReady(t *testing.T) {
	timer := &fakeIdleTimer{}
	reactor := &fakeIdleReactor{monotonic: 25, timer: timer}
	gcode := &fakeIdleGCode{}
	printer := &fakeIdlePrinter{reactor: reactor, gcode: gcode}
	module := NewIdleTimeoutModule(&fakeIdleConfig{
		printer:  printer,
		timeout:  30,
		template: &fakeIdleTemplate{script: "M84"},
	}, printer, reactor, gcode, &fakeIdleToolhead{})
	module.timeoutTimer = timer
	module.core.SetTimeout(30)
	module.core.state = StateReady

	command := &fakeIdleCommand{floatValues: map[string]float64{"TIMEOUT": 99}}
	if err := module.cmdSetIdleTimeout(command); err != nil {
		t.Fatalf("cmdSetIdleTimeout returned error: %v", err)
	}
	if module.core.Timeout() != 99 {
		t.Fatalf("unexpected timeout value: %v", module.core.Timeout())
	}
	if len(timer.updated) != 1 || timer.updated[0] != 124 {
		t.Fatalf("unexpected timer updates: %#v", timer.updated)
	}
	if len(command.responses) != 1 || command.responses[0] != "idle_timeout: Timeout set to 99.00 s" {
		t.Fatalf("unexpected responses: %#v", command.responses)
	}
}
