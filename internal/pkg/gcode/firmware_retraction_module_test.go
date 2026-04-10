package gcode

import (
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeFirmwareCommand struct {
	params      map[string]string
	respondInfo []string
	respondRaw  []string
}

func (self *fakeFirmwareCommand) String(name string, defaultValue string) string {
	return defaultValue
}

func (self *fakeFirmwareCommand) Float(name string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeFirmwareCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeFirmwareCommand) Parameters() map[string]string {
	if self.params == nil {
		return map[string]string{}
	}
	return self.params
}

func (self *fakeFirmwareCommand) RespondInfo(msg string, log bool) {
	self.respondInfo = append(self.respondInfo, msg)
}

func (self *fakeFirmwareCommand) RespondRaw(msg string) {
	self.respondRaw = append(self.respondRaw, msg)
}

type fakeFirmwareMutex struct{}

func (self *fakeFirmwareMutex) Lock()   {}
func (self *fakeFirmwareMutex) Unlock() {}

type fakeFirmwareGCode struct {
	commands map[string]func(printerpkg.Command) error
	scripts  []string
	mutex    printerpkg.Mutex
}

func (self *fakeFirmwareGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeFirmwareGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeFirmwareGCode) RunScriptFromCommand(script string) {
	self.scripts = append(self.scripts, script)
}

func (self *fakeFirmwareGCode) RunScript(script string) {
	self.scripts = append(self.scripts, script)
}

func (self *fakeFirmwareGCode) IsBusy() bool {
	return false
}

func (self *fakeFirmwareGCode) Mutex() printerpkg.Mutex {
	if self.mutex == nil {
		self.mutex = &fakeFirmwareMutex{}
	}
	return self.mutex
}

func (self *fakeFirmwareGCode) RespondInfo(msg string, log bool) {}

func (self *fakeFirmwareGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return old
}

type fakeFirmwarePrinter struct {
	gcode printerpkg.GCodeRuntime
}

func (self *fakeFirmwarePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	return defaultValue
}

func (self *fakeFirmwarePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}

func (self *fakeFirmwarePrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeFirmwarePrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeFirmwarePrinter) AddObject(name string, obj interface{}) error { return nil }

func (self *fakeFirmwarePrinter) LookupObjects(module string) []interface{} { return nil }

func (self *fakeFirmwarePrinter) HasStartArg(name string) bool { return false }

func (self *fakeFirmwarePrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeFirmwarePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeFirmwarePrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }

func (self *fakeFirmwarePrinter) InvokeShutdown(msg string) {}

func (self *fakeFirmwarePrinter) IsShutdown() bool { return false }

func (self *fakeFirmwarePrinter) Reactor() printerpkg.ModuleReactor { return nil }

func (self *fakeFirmwarePrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeFirmwarePrinter) GCode() printerpkg.GCodeRuntime { return self.gcode }

func (self *fakeFirmwarePrinter) GCodeMove() printerpkg.MoveTransformController { return nil }

func (self *fakeFirmwarePrinter) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeFirmwareConfig struct {
	printer              printerpkg.ModulePrinter
	retractLength        float64
	retractSpeed         float64
	unretractExtraLength float64
	unretractSpeed       float64
}

func (self *fakeFirmwareConfig) Name() string { return "firmware_retraction" }

func (self *fakeFirmwareConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeFirmwareConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeFirmwareConfig) Float(option string, defaultValue float64) float64 {
	switch option {
	case "retract_length":
		return self.retractLength
	case "retract_speed":
		return self.retractSpeed
	case "unretract_extra_length":
		return self.unretractExtraLength
	case "unretract_speed":
		return self.unretractSpeed
	default:
		return defaultValue
	}
}

func (self *fakeFirmwareConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeFirmwareConfig) LoadObject(section string) interface{} { return nil }

func (self *fakeFirmwareConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeFirmwareConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeFirmwareConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigFirmwareRetractionRegistersCommands(t *testing.T) {
	gcode := &fakeFirmwareGCode{}
	printer := &fakeFirmwarePrinter{gcode: gcode}
	module := LoadConfigFirmwareRetraction(&fakeFirmwareConfig{
		printer:              printer,
		retractLength:        0.8,
		retractSpeed:         25.0,
		unretractExtraLength: 0.1,
		unretractSpeed:       15.0,
	}).(*FirmwareRetractionModule)
	for _, cmd := range []string{"SET_RETRACTION", "GET_RETRACTION", "G10", "G11"} {
		if _, ok := gcode.commands[cmd]; !ok {
			t.Fatalf("expected %s to be registered", cmd)
		}
	}
	status := module.Get_status(0)
	if status["retract_length"] != 0.8 || status["retract_speed"] != 25.0 {
		t.Fatalf("unexpected initial status: %#v", status)
	}
}

func TestFirmwareRetractionModuleUpdatesAndRunsCommands(t *testing.T) {
	gcode := &fakeFirmwareGCode{}
	printer := &fakeFirmwarePrinter{gcode: gcode}
	module := LoadConfigFirmwareRetraction(&fakeFirmwareConfig{
		printer:              printer,
		retractLength:        0.8,
		retractSpeed:         25.0,
		unretractExtraLength: 0.1,
		unretractSpeed:       15.0,
	}).(*FirmwareRetractionModule)
	setCmd := &fakeFirmwareCommand{params: map[string]string{
		"RETRACT_LENGTH":         "1.2",
		"RETRACT_SPEED":          "30.0",
		"UNRETRACT_EXTRA_LENGTH": "0.3",
		"UNRETRACT_SPEED":        "12.0",
	}}
	if err := module.cmdSetRetraction(setCmd); err != nil {
		t.Fatalf("cmdSetRetraction returned error: %v", err)
	}
	status := module.Get_status(0)
	if status["retract_length"] != 1.2 || status["unretract_extra_length"] != 0.3 {
		t.Fatalf("unexpected updated status: %#v", status)
	}
	getCmd := &fakeFirmwareCommand{}
	if err := module.cmdGetRetraction(getCmd); err != nil {
		t.Fatalf("cmdGetRetraction returned error: %v", err)
	}
	if len(getCmd.respondInfo) != 1 {
		t.Fatalf("expected one response info message, got %d", len(getCmd.respondInfo))
	}
	if err := module.cmdG10(&fakeFirmwareCommand{}); err != nil {
		t.Fatalf("cmdG10 returned error: %v", err)
	}
	if len(gcode.scripts) != 1 {
		t.Fatalf("expected one retract script, got %d", len(gcode.scripts))
	}
	if err := module.cmdG11(&fakeFirmwareCommand{}); err != nil {
		t.Fatalf("cmdG11 returned error: %v", err)
	}
	if len(gcode.scripts) != 2 {
		t.Fatalf("expected retract and unretract scripts, got %d", len(gcode.scripts))
	}
}
