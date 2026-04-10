package addon

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"goklipper/common/constants"
	printerpkg "goklipper/internal/pkg/printer"
)

type fakeVirtualSDTimer struct {
	callback    func(float64) float64
	waketime    float64
	updateCalls []float64
}

func (self *fakeVirtualSDTimer) Update(waketime float64) {
	self.waketime = waketime
	self.updateCalls = append(self.updateCalls, waketime)
}

type fakeVirtualSDReactor struct {
	monotonic float64
	timers    []*fakeVirtualSDTimer
	pauses    []float64
}

func (self *fakeVirtualSDReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	timer := &fakeVirtualSDTimer{callback: callback, waketime: waketime}
	self.timers = append(self.timers, timer)
	return timer
}

func (self *fakeVirtualSDReactor) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeVirtualSDReactor) Pause(waketime float64) float64 {
	self.pauses = append(self.pauses, waketime)
	self.monotonic = waketime
	return waketime
}

type fakeVirtualSDMutex struct{}

func (self *fakeVirtualSDMutex) Lock()   {}
func (self *fakeVirtualSDMutex) Unlock() {}

type fakeVirtualSDGCode struct {
	commands  map[string]func(printerpkg.Command) error
	scripts   []string
	raws      []string
	infos     []string
	busy      bool
	panicOn   map[string]interface{}
	fromCmd   []string
}

func (self *fakeVirtualSDGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeVirtualSDGCode) IsTraditionalGCode(cmd string) bool { return false }

func (self *fakeVirtualSDGCode) RunScriptFromCommand(script string) {
	self.fromCmd = append(self.fromCmd, script)
}

func (self *fakeVirtualSDGCode) RunScript(script string) {
	if panicValue, ok := self.panicOn[script]; ok {
		panic(panicValue)
	}
	self.scripts = append(self.scripts, script)
}

func (self *fakeVirtualSDGCode) IsBusy() bool { return self.busy }

func (self *fakeVirtualSDGCode) Mutex() printerpkg.Mutex { return &fakeVirtualSDMutex{} }

func (self *fakeVirtualSDGCode) RespondInfo(msg string, log bool) {
	self.infos = append(self.infos, msg)
}

func (self *fakeVirtualSDGCode) RespondRaw(msg string) {
	self.raws = append(self.raws, msg)
}

func (self *fakeVirtualSDGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeVirtualSDPrintStats struct {
	currentFile   string
	startCalls    int
	pauseCalls    int
	completeCalls int
	errorMessages []string
	cancelCalls   int
	resetCalls    int
}

func (self *fakeVirtualSDPrintStats) Set_current_file(filename string) { self.currentFile = filename }
func (self *fakeVirtualSDPrintStats) Note_start()                      { self.startCalls++ }
func (self *fakeVirtualSDPrintStats) Note_pause()                      { self.pauseCalls++ }
func (self *fakeVirtualSDPrintStats) Note_complete()                   { self.completeCalls++ }
func (self *fakeVirtualSDPrintStats) Note_error(message string) {
	self.errorMessages = append(self.errorMessages, message)
}
func (self *fakeVirtualSDPrintStats) Note_cancel() { self.cancelCalls++ }
func (self *fakeVirtualSDPrintStats) Reset()       { self.resetCalls++ }

type fakeVirtualSDTemplate struct {
	script string
}

func (self *fakeVirtualSDTemplate) CreateContext(eventtime interface{}) map[string]interface{} {
	return nil
}

func (self *fakeVirtualSDTemplate) Render(context map[string]interface{}) (string, error) {
	return self.script, nil
}

func (self *fakeVirtualSDTemplate) RunGcodeFromCommand(context map[string]interface{}) error {
	return nil
}

type fakeVirtualSDPrinter struct {
	reactor       *fakeVirtualSDReactor
	gcode         *fakeVirtualSDGCode
	objects       map[string]interface{}
	eventHandlers map[string]func([]interface{}) error
	sentEvents    []string
}

func newFakeVirtualSDPrinter() *fakeVirtualSDPrinter {
	return &fakeVirtualSDPrinter{
		reactor:       &fakeVirtualSDReactor{monotonic: 5.0},
		gcode:         &fakeVirtualSDGCode{commands: map[string]func(printerpkg.Command) error{}, panicOn: map[string]interface{}{}},
		objects:       map[string]interface{}{},
		eventHandlers: map[string]func([]interface{}) error{},
	}
}

func (self *fakeVirtualSDPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.objects[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeVirtualSDPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	self.eventHandlers[event] = callback
}

func (self *fakeVirtualSDPrinter) SendEvent(event string, params []interface{}) {
	self.sentEvents = append(self.sentEvents, event)
	_ = params
}

func (self *fakeVirtualSDPrinter) CurrentExtruderName() string { return "extruder" }
func (self *fakeVirtualSDPrinter) AddObject(name string, obj interface{}) error {
	self.objects[name] = obj
	return nil
}
func (self *fakeVirtualSDPrinter) LookupObjects(module string) []interface{} { return nil }
func (self *fakeVirtualSDPrinter) HasStartArg(name string) bool              { return false }
func (self *fakeVirtualSDPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}
func (self *fakeVirtualSDPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}
func (self *fakeVirtualSDPrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }
func (self *fakeVirtualSDPrinter) InvokeShutdown(msg string)                    {}
func (self *fakeVirtualSDPrinter) IsShutdown() bool                             { return false }
func (self *fakeVirtualSDPrinter) Reactor() printerpkg.ModuleReactor            { return self.reactor }
func (self *fakeVirtualSDPrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return nil
}
func (self *fakeVirtualSDPrinter) GCode() printerpkg.GCodeRuntime { return self.gcode }
func (self *fakeVirtualSDPrinter) GCodeMove() printerpkg.MoveTransformController {
	return nil
}
func (self *fakeVirtualSDPrinter) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeVirtualSDConfig struct {
	printer         *fakeVirtualSDPrinter
	strings         map[string]string
	loadedObjects   []string
	loadedTemplates []string
	objects         map[string]interface{}
	templates       map[string]printerpkg.Template
}

func (self *fakeVirtualSDConfig) Name() string { return "virtual_sdcard" }

func (self *fakeVirtualSDConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeVirtualSDConfig) Bool(option string, defaultValue bool) bool { return defaultValue }
func (self *fakeVirtualSDConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}
func (self *fakeVirtualSDConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeVirtualSDConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	return self.objects[section]
}

func (self *fakeVirtualSDConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	self.loadedTemplates = append(self.loadedTemplates, option)
	if template, ok := self.templates[option]; ok {
		return template
	}
	return &fakeVirtualSDTemplate{script: defaultValue}
}

func (self *fakeVirtualSDConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return self.LoadTemplate(module, option, "")
}

func (self *fakeVirtualSDConfig) Printer() printerpkg.ModulePrinter { return self.printer }

type fakeVirtualSDCommand struct {
	params    map[string]string
	rawParams string
	infos     []string
	raws      []string
}

func (self *fakeVirtualSDCommand) String(name string, defaultValue string) string {
	if value, ok := self.params[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeVirtualSDCommand) Float(name string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeVirtualSDCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	if value, ok := self.params[name]; ok {
		if value == "1" {
			return 1
		}
		if value == "0" {
			return 0
		}
	}
	return defaultValue
}

func (self *fakeVirtualSDCommand) Parameters() map[string]string {
	copyParams := map[string]string{}
	for key, value := range self.params {
		copyParams[key] = value
	}
	return copyParams
}

func (self *fakeVirtualSDCommand) RespondInfo(msg string, log bool) {
	self.infos = append(self.infos, msg)
	_ = log
}

func (self *fakeVirtualSDCommand) RespondRaw(msg string) {
	self.raws = append(self.raws, msg)
}

func (self *fakeVirtualSDCommand) RawCommandParameters() string {
	return self.rawParams
}

type fakeRecoveredCommandError struct {
	E string
}

func newFakeVirtualSDModule(t *testing.T) (*VirtualSDModule, *fakeVirtualSDPrinter, *fakeVirtualSDPrintStats) {
	t.Helper()
	tempDir := t.TempDir()
	printer := newFakeVirtualSDPrinter()
	printStats := &fakeVirtualSDPrintStats{}
	config := &fakeVirtualSDConfig{
		printer: printer,
		strings: map[string]string{"path": tempDir},
		objects: map[string]interface{}{"print_stats": printStats},
		templates: map[string]printerpkg.Template{
			"on_error_gcode": &fakeVirtualSDTemplate{script: "M117 failure"},
		},
	}
	module := LoadConfigVirtualSD(config).(*VirtualSDModule)
	return module, printer, printStats
}

func TestLoadConfigVirtualSDRegistersCommandsAndTemplate(t *testing.T) {
	module, printer, printStats := newFakeVirtualSDModule(t)
	if module == nil || printStats == nil {
		panic("expected module and print_stats")
	}
	if _, ok := printer.eventHandlers["project:shutdown"]; !ok {
			
		t.Fatalf("expected project:shutdown handler to be registered")
	}
	commands := make([]string, 0, len(printer.gcode.commands))
	for name := range printer.gcode.commands {
		commands = append(commands, name)
	}
	sort.Strings(commands)
	want := []string{"M20", "M21", "M23", "M24", "M25", "M26", "M27", "M28", "M29", "M30", "SDCARD_PRINT_FILE", "SDCARD_RESET_FILE"}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("registered commands = %v, want %v", commands, want)
	}
	config := module.printer.(*fakeVirtualSDPrinter)
	_ = config
}

func TestVirtualSDCmdM23LoadsFileFromRawParameters(t *testing.T) {
	module, _, printStats := newFakeVirtualSDModule(t)
	filename := filepath.Join(module.core.SdcardDirname, "sub")
	if err := os.MkdirAll(filename, 0o755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	filePath := filepath.Join(filename, "test.gcode")
	if err := os.WriteFile(filePath, []byte("G1 X1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	cmd := &fakeVirtualSDCommand{rawParams: "/sub/test.gcode", params: map[string]string{}}
	if err := module.Cmd_M23(cmd); err != nil {
		t.Fatalf("Cmd_M23() unexpected error: %v", err)
	}
	if got, want := printStats.currentFile, "sub/test.gcode"; got != want {
		t.Fatalf("current file = %q, want %q", got, want)
	}
	if module.core.CurrentFile == nil {
		t.Fatalf("expected current file to be opened")
	}
	if module.core.FileSize <= 0 {
		t.Fatalf("expected file size to be populated")
	}
}

func TestVirtualSDWorkHandlerCompletesAndUpdatesPrintStats(t *testing.T) {
	module, printer, printStats := newFakeVirtualSDModule(t)
	filePath := filepath.Join(module.core.SdcardDirname, "print.gcode")
	if err := os.WriteFile(filePath, []byte("G1 X1\nG1 X2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	if err := module.Load_file(&fakeVirtualSDCommand{}, "print.gcode", true); err != nil {
		t.Fatalf("Load_file() unexpected error: %v", err)
	}
	if err := module.Do_resume(); err != nil {
		t.Fatalf("Do_resume() unexpected error: %v", err)
	}
	if len(printer.reactor.timers) != 1 {
		t.Fatalf("registered timers = %d, want 1", len(printer.reactor.timers))
	}
	if got := printer.reactor.timers[0].callback(constants.NOW); got != constants.NEVER {
		t.Fatalf("work handler returned %v, want %v", got, constants.NEVER)
	}
	if module.Is_active() {
		t.Fatalf("module should be inactive after completion")
	}
	if got, want := printer.gcode.scripts, []string{"G1 X1", "G1 X2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("run scripts = %v, want %v", got, want)
	}
	if got, want := printStats.startCalls, 1; got != want {
		t.Fatalf("Note_start calls = %d, want %d", got, want)
	}
	if got, want := printStats.completeCalls, 1; got != want {
		t.Fatalf("Note_complete calls = %d, want %d", got, want)
	}
	if len(printStats.errorMessages) != 0 || printStats.pauseCalls != 0 {
		t.Fatalf("unexpected print_stats state: errors=%v pauseCalls=%d", printStats.errorMessages, printStats.pauseCalls)
	}
	if got, want := printer.gcode.raws, []string{"Done printing file"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("raw responses = %v, want %v", got, want)
	}
}

func TestVirtualSDWorkHandlerRunsOnErrorTemplateForCommandError(t *testing.T) {
	module, printer, printStats := newFakeVirtualSDModule(t)
	printer.gcode.panicOn["BAD"] = &fakeRecoveredCommandError{E: "boom"}
	filePath := filepath.Join(module.core.SdcardDirname, "bad.gcode")
	if err := os.WriteFile(filePath, []byte("BAD\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	if err := module.Load_file(&fakeVirtualSDCommand{}, "bad.gcode", true); err != nil {
		t.Fatalf("Load_file() unexpected error: %v", err)
	}
	if err := module.Do_resume(); err != nil {
		t.Fatalf("Do_resume() unexpected error: %v", err)
	}
	printer.reactor.timers[0].callback(constants.NOW)
	if got, want := printer.gcode.scripts, []string{"M117 failure"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("run scripts = %v, want %v", got, want)
	}
	if got, want := printer.gcode.raws, []string{"boom"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("raw responses = %v, want %v", got, want)
	}
	if got, want := printStats.errorMessages, []string{"boom"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("error messages = %v, want %v", got, want)
	}
	if printStats.completeCalls != 0 {
		t.Fatalf("unexpected Note_complete calls = %d", printStats.completeCalls)
	}
}