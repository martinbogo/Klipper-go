package gcode

import (
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeMacroConfig struct {
	name          string
	values        map[string]interface{}
	prefixOptions []string
	printer       *fakeMacroPrinter
	template      *fakeMacroTemplate
}

func (self *fakeMacroConfig) Name() string { return self.name }
func (self *fakeMacroConfig) Get_name() string { return self.name }
func (self *fakeMacroConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.values[option]; ok {
		return value.(string)
	}
	return defaultValue
}
func (self *fakeMacroConfig) Bool(option string, defaultValue bool) bool { return defaultValue }
func (self *fakeMacroConfig) Float(option string, defaultValue float64) float64 { return defaultValue }
func (self *fakeMacroConfig) OptionalFloat(option string) *float64 { return nil }
func (self *fakeMacroConfig) LoadObject(section string) interface{} { return nil }
func (self *fakeMacroConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return self.template
}
func (self *fakeMacroConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return self.template
}
func (self *fakeMacroConfig) Printer() printerpkg.ModulePrinter { return self.printer }
func (self *fakeMacroConfig) Get(option string, default1 interface{}, noteValid bool) interface{} {
	if value, ok := self.values[option]; ok {
		return value
	}
	return default1
}
func (self *fakeMacroConfig) Get_prefix_options(prefix string) []string { return self.prefixOptions }

type fakeMacroPrinter struct {
	gcode         *fakeMacroGCode
	eventHandlers map[string]func([]interface{}) error
}

func (self *fakeMacroPrinter) LookupObject(name string, defaultValue interface{}) interface{} { return nil }
func (self *fakeMacroPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}
func (self *fakeMacroPrinter) SendEvent(event string, params []interface{})                       {}
func (self *fakeMacroPrinter) CurrentExtruderName() string                                        { return "" }
func (self *fakeMacroPrinter) AddObject(name string, obj interface{}) error                       { return nil }
func (self *fakeMacroPrinter) LookupObjects(module string) []interface{}                          { return nil }
func (self *fakeMacroPrinter) HasStartArg(name string) bool                                       { return false }
func (self *fakeMacroPrinter) LookupHeater(name string) printerpkg.HeaterRuntime                  { return nil }
func (self *fakeMacroPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry           { return nil }
func (self *fakeMacroPrinter) LookupMCU(name string) printerpkg.MCURuntime                        { return nil }
func (self *fakeMacroPrinter) InvokeShutdown(msg string)                                          {}
func (self *fakeMacroPrinter) IsShutdown() bool                                                   { return false }
func (self *fakeMacroPrinter) Reactor() printerpkg.ModuleReactor                                  { return nil }
func (self *fakeMacroPrinter) StepperEnable() printerpkg.StepperEnableRuntime                     { return nil }
func (self *fakeMacroPrinter) GCode() printerpkg.GCodeRuntime                                     { return self.gcode }
func (self *fakeMacroPrinter) GCodeMove() printerpkg.MoveTransformController                      { return nil }
func (self *fakeMacroPrinter) Webhooks() printerpkg.WebhookRegistry                               { return nil }

type fakeMacroGCode struct {
	commands    map[string]func(printerpkg.Command) error
	muxCommands map[string]func(printerpkg.Command) error
	traditional map[string]bool
}

func (self *fakeMacroGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}
func (self *fakeMacroGCode) RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string) {
	if self.muxCommands == nil {
		self.muxCommands = map[string]func(printerpkg.Command) error{}
	}
	self.muxCommands[cmd+":"+key+":"+value] = handler
}
func (self *fakeMacroGCode) IsTraditionalGCode(cmd string) bool { return self.traditional[cmd] }
func (self *fakeMacroGCode) RunScriptFromCommand(script string)  {}
func (self *fakeMacroGCode) RunScript(script string)             {}
func (self *fakeMacroGCode) IsBusy() bool                        { return false }
func (self *fakeMacroGCode) Mutex() printerpkg.Mutex             { return nil }
func (self *fakeMacroGCode) RespondInfo(msg string, log bool)    {}
func (self *fakeMacroGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.commands[cmd] = handler
	return old
}

type fakeMacroTemplate struct {
	context  map[string]interface{}
	runCalls []map[string]interface{}
}

func (self *fakeMacroTemplate) CreateContext(eventtime interface{}) map[string]interface{} {
	return self.context
}
func (self *fakeMacroTemplate) Render(context map[string]interface{}) (string, error) { return "", nil }
func (self *fakeMacroTemplate) RunGcodeFromCommand(context map[string]interface{}) error {
	copy := map[string]interface{}{}
	for k, v := range context {
		copy[k] = v
	}
	self.runCalls = append(self.runCalls, copy)
	return nil
}

type fakeMacroCommand struct {
	params    map[string]string
	rawparams string
}

func (self *fakeMacroCommand) String(name string, defaultValue string) string {
	if value, ok := self.params[name]; ok {
		return value
	}
	return defaultValue
}
func (self *fakeMacroCommand) Float(name string, defaultValue float64) float64 { return defaultValue }
func (self *fakeMacroCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}
func (self *fakeMacroCommand) Parameters() map[string]string { return self.params }
func (self *fakeMacroCommand) RespondInfo(msg string, log bool) {}
func (self *fakeMacroCommand) RespondRaw(msg string) {}
func (self *fakeMacroCommand) Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
	if value, ok := self.params[name]; ok {
		return value
	}
	if defaultValue, ok := _default.(string); ok {
		return defaultValue
	}
		panic("missing parameter")
}
func (self *fakeMacroCommand) RawParameters() string { return self.rawparams }

func TestGCodeMacroSetVariableParsesLiteral(t *testing.T) {
	template := &fakeMacroTemplate{context: map[string]interface{}{}}
	gcode := &fakeMacroGCode{traditional: map[string]bool{}}
	printer := &fakeMacroPrinter{gcode: gcode}
	config := &fakeMacroConfig{
		name:          "gcode_macro test",
		values:        map[string]interface{}{"variable_speed": "100", "description": "G-Code macro"},
		prefixOptions: []string{"variable_speed"},
		printer:       printer,
		template:      template,
	}
	macro := NewGCodeMacro(config)
	cmd := &fakeMacroCommand{params: map[string]string{"VARIABLE": "speed", "VALUE": "123"}}
	if err := macro.cmdSetGCodeVariable(cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := macro.variables["speed"].([]byte); !ok || string(got) != "123" {
		t.Fatalf("unexpected variable value %#v", macro.variables["speed"])
	}
}

func TestGCodeMacroRunPassesTemplateContextAndRawParams(t *testing.T) {
	template := &fakeMacroTemplate{context: map[string]interface{}{"printer": "ctx"}}
	gcode := &fakeMacroGCode{traditional: map[string]bool{}}
	printer := &fakeMacroPrinter{gcode: gcode}
	config := &fakeMacroConfig{
		name:          "gcode_macro test",
		values:        map[string]interface{}{"variable_speed": "100", "description": "G-Code macro"},
		prefixOptions: []string{"variable_speed"},
		printer:       printer,
		template:      template,
	}
	macro := NewGCodeMacro(config)
	cmd := &fakeMacroCommand{params: map[string]string{"S": "10"}, rawparams: "S=10 F=20"}
	if err := macro.cmd(cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(template.runCalls) != 1 {
		t.Fatalf("expected one template run, got %d", len(template.runCalls))
	}
	call := template.runCalls[0]
	if call["printer"] != "ctx" {
		t.Fatalf("expected template context to include printer, got %#v", call)
	}
	if !reflect.DeepEqual(call["params"], map[string]string{"S": "10"}) {
		t.Fatalf("unexpected params %#v", call["params"])
	}
	if call["rawparams"] != "S=10 F=20" {
		t.Fatalf("unexpected rawparams %#v", call["rawparams"])
	}
	if call["speed"] != "100" {
		t.Fatalf("unexpected macro variable %#v", call["speed"])
	}
}