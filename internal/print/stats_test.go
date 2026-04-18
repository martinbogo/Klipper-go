package print

import (
	"math"
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakePrintStatsTimer struct{}

func (self *fakePrintStatsTimer) Update(waketime float64) {
	_ = waketime
}

type fakePrintStatsReactor struct {
	monotonic float64
}

func (self *fakePrintStatsReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	_ = callback
	_ = waketime
	return &fakePrintStatsTimer{}
}

func (self *fakePrintStatsReactor) Monotonic() float64 {
	return self.monotonic
}

type fakePrintStatsMutex struct{}

func (self *fakePrintStatsMutex) Lock()   {}
func (self *fakePrintStatsMutex) Unlock() {}

type fakePrintStatsGCode struct {
	commands map[string]func(printerpkg.Command) error
}

func (self *fakePrintStatsGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
	_, _ = whenNotReady, desc
}

func (self *fakePrintStatsGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakePrintStatsGCode) RunScriptFromCommand(script string) {}
func (self *fakePrintStatsGCode) RunScript(script string)            {}
func (self *fakePrintStatsGCode) IsBusy() bool                       { return false }
func (self *fakePrintStatsGCode) Mutex() printerpkg.Mutex            { return &fakePrintStatsMutex{} }
func (self *fakePrintStatsGCode) RespondInfo(msg string, log bool)   {}
func (self *fakePrintStatsGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakePrintStatsPrinter struct {
	reactor           *fakePrintStatsReactor
	gcode             *fakePrintStatsGCode
	lookupObjectCalls []string
}

func (self *fakePrintStatsPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	self.lookupObjectCalls = append(self.lookupObjectCalls, name)
	return defaultValue
}

func (self *fakePrintStatsPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}
func (self *fakePrintStatsPrinter) SendEvent(event string, params []interface{})      {}
func (self *fakePrintStatsPrinter) CurrentExtruderName() string                       { return "extruder" }
func (self *fakePrintStatsPrinter) AddObject(name string, obj interface{}) error      { return nil }
func (self *fakePrintStatsPrinter) LookupObjects(module string) []interface{}         { return nil }
func (self *fakePrintStatsPrinter) HasStartArg(name string) bool                      { return false }
func (self *fakePrintStatsPrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }
func (self *fakePrintStatsPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}
func (self *fakePrintStatsPrinter) LookupMCU(name string) printerpkg.MCURuntime    { return nil }
func (self *fakePrintStatsPrinter) InvokeShutdown(msg string)                      {}
func (self *fakePrintStatsPrinter) IsShutdown() bool                               { return false }
func (self *fakePrintStatsPrinter) Reactor() printerpkg.ModuleReactor              { return self.reactor }
func (self *fakePrintStatsPrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }
func (self *fakePrintStatsPrinter) GCode() printerpkg.GCodeRuntime                 { return self.gcode }
func (self *fakePrintStatsPrinter) GCodeMove() printerpkg.MoveTransformController  { return nil }
func (self *fakePrintStatsPrinter) Webhooks() printerpkg.WebhookRegistry           { return nil }

type fakePrintStatsConfig struct {
	printer       *fakePrintStatsPrinter
	objects       map[string]interface{}
	loadedObjects []string
}

func (self *fakePrintStatsConfig) Name() string { return "print_stats" }

func (self *fakePrintStatsConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakePrintStatsConfig) Bool(option string, defaultValue bool) bool { return defaultValue }
func (self *fakePrintStatsConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}
func (self *fakePrintStatsConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakePrintStatsConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	return self.objects[section]
}

func (self *fakePrintStatsConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakePrintStatsConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakePrintStatsConfig) Printer() printerpkg.ModulePrinter { return self.printer }

type fakeExtrusionStatusSource struct{}

func (self *fakeExtrusionStatusSource) Get_status(eventtime float64) map[string]interface{} {
	_ = eventtime
	return map[string]interface{}{
		"position":       []float64{0, 0, 0, 0},
		"extrude_factor": 1.0,
	}
}

type fakeMutableExtrusionStatusSource struct {
	position      float64
	extrudeFactor float64
}

func (self *fakeMutableExtrusionStatusSource) Get_status(eventtime float64) map[string]interface{} {
	_ = eventtime
	return map[string]interface{}{
		"position":       []float64{0, 0, 0, self.position},
		"extrude_factor": self.extrudeFactor,
	}
}

func TestStatsPauseResumeAccounting(t *testing.T) {
	stats := NewStats()
	stats.SetCurrentFile("cube.gcode")
	stats.NoteStart(10, ExtrusionStatus{Position: 0, ExtrudeFactor: 1})

	stats.NotePause(13, ExtrusionStatus{Position: 4, ExtrudeFactor: 1})
	stats.NoteStart(18, ExtrusionStatus{Position: 4, ExtrudeFactor: 1})

	status := stats.GetStatus(20, ExtrusionStatus{Position: 6, ExtrudeFactor: 1})
	if got := status["filename"].(string); got != "cube.gcode" {
		t.Fatalf("unexpected filename: %q", got)
	}
	if got := status["total_duration"].(float64); got != 10 {
		t.Fatalf("unexpected total duration: %v", got)
	}
	if got := status["print_duration"].(float64); got != 5 {
		t.Fatalf("unexpected print duration: %v", got)
	}
	if got := status["filament_used"].(float64); got != 6 {
		t.Fatalf("unexpected filament usage: %v", got)
	}

	stats.SetInfo(12, 20)
	info := stats.GetStatus(20, ExtrusionStatus{Position: 6, ExtrudeFactor: 1})["info"].(map[string]int)
	if info["total_layer"] != 12 || info["current_layer"] != 12 {
		t.Fatalf("unexpected layer info: %#v", info)
	}

	stats.NoteComplete(21)
	status = stats.GetStatus(21, ExtrusionStatus{Position: 6, ExtrudeFactor: 1})
	if got := status["state"].(string); got != "complete" {
		t.Fatalf("unexpected final state: %q", got)
	}
	if got := status["total_duration"].(float64); got != 11 {
		t.Fatalf("unexpected completed duration: %v", got)
	}
}

func TestStatsZeroExtrudeFactorDoesNotExplode(t *testing.T) {
	stats := NewStats()
	stats.SetCurrentFile("zero.gcode")
	stats.NoteStart(0, ExtrusionStatus{Position: 1, ExtrudeFactor: 1})

	status := stats.GetStatus(2, ExtrusionStatus{Position: 5, ExtrudeFactor: 0})
	filamentUsed := status["filament_used"].(float64)
	if math.IsNaN(filamentUsed) || math.IsInf(filamentUsed, 0) {
		t.Fatalf("filament usage should remain finite, got %v", filamentUsed)
	}
	if filamentUsed != 0 {
		t.Fatalf("expected zero filament usage when extrude factor is zero, got %v", filamentUsed)
	}
}

func TestLoadConfigPrintStatsLoadsGCodeMoveViaConfig(t *testing.T) {
	gcodeMove := &fakeMutableExtrusionStatusSource{position: 12.5, extrudeFactor: 1.0}
	printer := &fakePrintStatsPrinter{
		reactor: &fakePrintStatsReactor{monotonic: 42},
		gcode:   &fakePrintStatsGCode{},
	}
	config := &fakePrintStatsConfig{
		printer: printer,
		objects: map[string]interface{}{
			"gcode_move": gcodeMove,
		},
	}

	module := LoadConfigPrintStats(config).(*PrintStatsModule)
	if module == nil {
		t.Fatalf("expected print_stats module")
	}
	if got := config.loadedObjects; !reflect.DeepEqual(got, []string{"gcode_move"}) {
		t.Fatalf("loaded objects = %v, want [gcode_move]", got)
	}
	if got := printer.lookupObjectCalls; len(got) != 0 {
		t.Fatalf("expected no direct printer lookups, got %v", got)
	}
	if _, ok := printer.gcode.commands["SET_PRINT_STATS_INFO"]; !ok {
		t.Fatalf("expected SET_PRINT_STATS_INFO command to be registered")
	}
	module.Set_current_file("cube.gcode")
	module.Note_start()
	gcodeMove.position = 15.5
	status := module.Get_status(42)
	if got := status["filament_used"].(float64); got != 3 {
		t.Fatalf("filament_used = %v, want 3", got)
	}
}
