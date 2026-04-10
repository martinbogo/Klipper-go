package probe

import (
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeQueryEndstopsMutex struct {
	lockCount   int
	unlockCount int
}

func (self *fakeQueryEndstopsMutex) Lock() {
	self.lockCount++
}

func (self *fakeQueryEndstopsMutex) Unlock() {
	self.unlockCount++
}

type fakeQueryEndstopsCommand struct {
	raw []string
}

func (self *fakeQueryEndstopsCommand) String(name string, defaultValue string) string {
	return defaultValue
}

func (self *fakeQueryEndstopsCommand) Float(name string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeQueryEndstopsCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeQueryEndstopsCommand) Parameters() map[string]string {
	return map[string]string{}
}

func (self *fakeQueryEndstopsCommand) RespondInfo(msg string, log bool) {}

func (self *fakeQueryEndstopsCommand) RespondRaw(msg string) {
	self.raw = append(self.raw, msg)
}

type fakeQueryEndstopsGCode struct {
	commands map[string]func(printerpkg.Command) error
	mutex    *fakeQueryEndstopsMutex
}

func (self *fakeQueryEndstopsGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeQueryEndstopsGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeQueryEndstopsGCode) RunScriptFromCommand(script string) {}

func (self *fakeQueryEndstopsGCode) RunScript(script string) {}

func (self *fakeQueryEndstopsGCode) IsBusy() bool {
	return false
}

func (self *fakeQueryEndstopsGCode) Mutex() printerpkg.Mutex {
	if self.mutex == nil {
		self.mutex = &fakeQueryEndstopsMutex{}
	}
	return self.mutex
}

func (self *fakeQueryEndstopsGCode) RespondInfo(msg string, log bool) {}

func (self *fakeQueryEndstopsGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return old
}

type fakeQueryEndstopsWebhooks struct {
	handlers map[string]func() (interface{}, error)
}

func (self *fakeQueryEndstopsWebhooks) RegisterEndpoint(path string, handler func() (interface{}, error)) error {
	if self.handlers == nil {
		self.handlers = map[string]func() (interface{}, error){}
	}
	self.handlers[path] = handler
	return nil
}

func (self *fakeQueryEndstopsWebhooks) RegisterEndpointWithRequest(path string, handler func(printerpkg.WebhookRequest) (interface{}, error)) error {
	return nil
}

type fakeQueryEndstopsToolhead struct {
	lastMoveTime float64
}

func (self *fakeQueryEndstopsToolhead) Get_last_move_time() float64 {
	return self.lastMoveTime
}

type fakeQueryEndstop struct {
	values    []int
	index     int
	queriedAt []float64
}

func (self *fakeQueryEndstop) Query_endstop(printTime float64) int {
	self.queriedAt = append(self.queriedAt, printTime)
	if len(self.values) == 0 {
		return 0
	}
	if self.index >= len(self.values) {
		return self.values[len(self.values)-1]
	}
	value := self.values[self.index]
	self.index++
	return value
}

type fakeQueryEndstopsPrinter struct {
	gcode    printerpkg.GCodeRuntime
	webhooks printerpkg.WebhookRegistry
	lookup   map[string]interface{}
}

func (self *fakeQueryEndstopsPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeQueryEndstopsPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}

func (self *fakeQueryEndstopsPrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeQueryEndstopsPrinter) CurrentExtruderName() string {
	return "extruder"
}

func (self *fakeQueryEndstopsPrinter) AddObject(name string, obj interface{}) error {
	if self.lookup == nil {
		self.lookup = map[string]interface{}{}
	}
	self.lookup[name] = obj
	return nil
}

func (self *fakeQueryEndstopsPrinter) LookupObjects(module string) []interface{} {
	return nil
}

func (self *fakeQueryEndstopsPrinter) HasStartArg(name string) bool {
	return false
}

func (self *fakeQueryEndstopsPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}

func (self *fakeQueryEndstopsPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeQueryEndstopsPrinter) LookupMCU(name string) printerpkg.MCURuntime {
	return nil
}

func (self *fakeQueryEndstopsPrinter) InvokeShutdown(msg string) {}

func (self *fakeQueryEndstopsPrinter) IsShutdown() bool {
	return false
}

func (self *fakeQueryEndstopsPrinter) Reactor() printerpkg.ModuleReactor {
	return nil
}

func (self *fakeQueryEndstopsPrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return nil
}

func (self *fakeQueryEndstopsPrinter) GCode() printerpkg.GCodeRuntime {
	return self.gcode
}

func (self *fakeQueryEndstopsPrinter) GCodeMove() printerpkg.MoveTransformController {
	return nil
}

func (self *fakeQueryEndstopsPrinter) Webhooks() printerpkg.WebhookRegistry {
	return self.webhooks
}

type fakeQueryEndstopsConfig struct {
	printer printerpkg.ModulePrinter
}

func (self *fakeQueryEndstopsConfig) Name() string {
	return "query_endstops"
}

func (self *fakeQueryEndstopsConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeQueryEndstopsConfig) Bool(option string, defaultValue bool) bool {
	return defaultValue
}

func (self *fakeQueryEndstopsConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeQueryEndstopsConfig) OptionalFloat(option string) *float64 {
	return nil
}

func (self *fakeQueryEndstopsConfig) LoadObject(section string) interface{} {
	return nil
}

func (self *fakeQueryEndstopsConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeQueryEndstopsConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeQueryEndstopsConfig) Printer() printerpkg.ModulePrinter {
	return self.printer
}

func TestQueryEndstopsModuleRegistersAndReports(t *testing.T) {
	gcode := &fakeQueryEndstopsGCode{}
	webhooks := &fakeQueryEndstopsWebhooks{}
	toolhead := &fakeQueryEndstopsToolhead{lastMoveTime: 12.5}
	printer := &fakeQueryEndstopsPrinter{
		gcode:    gcode,
		webhooks: webhooks,
		lookup:   map[string]interface{}{"toolhead": toolhead},
	}
	module := LoadConfigQueryEndstops(&fakeQueryEndstopsConfig{printer: printer}).(*QueryEndstopsModule)
	openEndstop := &fakeQueryEndstop{values: []int{0, 0, 0}}
	closedEndstop := &fakeQueryEndstop{values: []int{1, 1, 1}}
	module.RegisterEndstop(openEndstop, "x")
	module.Register_endstop(closedEndstop, "y")

	if _, ok := gcode.commands["QUERY_ENDSTOPS"]; !ok {
		t.Fatalf("expected QUERY_ENDSTOPS command to be registered")
	}
	if _, ok := gcode.commands["M119"]; !ok {
		t.Fatalf("expected M119 command to be registered")
	}
	if _, ok := webhooks.handlers["query_endstops/status"]; !ok {
		t.Fatalf("expected webhook endpoint to be registered")
	}

	cmd := &fakeQueryEndstopsCommand{}
	if err := module.cmdQueryEndstops(cmd); err != nil {
		t.Fatalf("cmdQueryEndstops returned error: %v", err)
	}
	if len(cmd.raw) != 1 || cmd.raw[0] != "x:open y:TRIGGERED" {
		t.Fatalf("unexpected raw response: %#v", cmd.raw)
	}
	if len(openEndstop.queriedAt) != 1 || openEndstop.queriedAt[0] != 12.5 {
		t.Fatalf("unexpected print time for open endstop: %#v", openEndstop.queriedAt)
	}

	statusValue, err := webhooks.handlers["query_endstops/status"]()
	if err != nil {
		t.Fatalf("webhook handler returned error: %v", err)
	}
	statusText, ok := statusValue.(map[string]string)
	if !ok {
		t.Fatalf("unexpected webhook payload type: %T", statusValue)
	}
	if statusText["x"] != "open" || statusText["y"] != "TRIGGERED" {
		t.Fatalf("unexpected webhook payload: %#v", statusText)
	}
	if gcode.mutex.lockCount != 1 || gcode.mutex.unlockCount != 1 {
		t.Fatalf("expected webhook query to lock/unlock once, got %+v", gcode.mutex)
	}

	status := module.Get_status(nil)
	lastQuery := status["last_query"]
	if lastQuery["x"].(int) != 0 || lastQuery["y"].(int) != 1 {
		t.Fatalf("unexpected cached status: %#v", status)
	}
}
