package gcode

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"goklipper/common/jinja2"
	"goklipper/common/logger"
	"goklipper/common/utils/LiteralEval"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/utils/sys"
	"goklipper/common/value"
	printerpkg "goklipper/internal/pkg/printer"
)

const cmdSetGCodeVariableHelp = "Set the value of a G-Code macro variable"

type macroConfig interface {
	printerpkg.ModuleConfig
	Get_name() string
	Get(option string, default1 interface{}, noteValid bool) interface{}
	Get_prefix_options(prefix string) []string
}

type macroGCode interface {
	printerpkg.GCodeRuntime
	RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string)
}

type macroCommand interface {
	printerpkg.Command
	Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	RawParameters() string
}

type macroWebhookCaller interface {
	CallRemoteMethod(method string, kwargs interface{}) error
}

type macroStatusWrapper struct {
	printer   printerpkg.ModulePrinter
	eventtime float64
	cache     map[string]interface{}
}

func requireMacroConfig(config printerpkg.ModuleConfig) macroConfig {
	legacy, ok := config.(macroConfig)
	if !ok {
		panic(fmt.Sprintf("config does not implement macroConfig: %T", config))
	}
	return legacy
}

func requireMacroGCode(gcode printerpkg.GCodeRuntime) macroGCode {
	legacy, ok := gcode.(macroGCode)
	if !ok {
		panic(fmt.Sprintf("gcode runtime does not implement macroGCode: %T", gcode))
	}
	return legacy
}

func requireMacroCommand(gcmd printerpkg.Command) macroCommand {
	legacy, ok := gcmd.(macroCommand)
	if !ok {
		panic(fmt.Sprintf("gcode command does not implement macroCommand: %T", gcmd))
	}
	return legacy
}

func newMacroStatusWrapper(printer printerpkg.ModulePrinter, eventtime float64) *macroStatusWrapper {
	return &macroStatusWrapper{printer: printer, eventtime: eventtime, cache: map[string]interface{}{}}
}

func (self *macroStatusWrapper) __getitem__(val string) (interface{}, error) {
	sval := strings.TrimSpace(val)
	if cached, ok := self.cache[sval]; ok {
		return cached, nil
	}
	po := self.printer.LookupObject(sval, nil)
	if po == nil {
		panic(val)
	}
	if self.eventtime == 0 {
		self.eventtime = self.printer.Reactor().Monotonic()
	}
	method := reflect.ValueOf(po).MethodByName("Get_status")
	if !method.IsValid() {
		return nil, fmt.Errorf("macroStatusWrapper %#v missing Get_status", po)
	}
	res := method.Call([]reflect.Value{reflect.ValueOf(self.eventtime)})
	status := map[string]interface{}{}
	if len(res) >= 1 && res[0].Type().Kind() == reflect.Map {
		for _, key := range res[0].MapKeys() {
			status[key.String()] = res[0].MapIndex(key).Interface()
			self.cache[key.String()] = status[key.String()]
		}
	}
	return status[sval], nil
}

func (self *macroStatusWrapper) __contains__(val string) bool {
	_, err := self.__getitem__(val)
	return err == nil
}

func (self *macroStatusWrapper) Iter() <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, obj := range self.printer.LookupObjects("") {
			for name := range obj.(map[string]interface{}) {
				if self.__contains__(name) {
					ch <- name
				}
			}
		}
	}()
	return ch
}

type MacroTemplate struct {
	printer  printerpkg.ModulePrinter
	gcode    printerpkg.GCodeRuntime
	template *jinja2.Template
}

func LoadMacroTemplate(config printerpkg.ModuleConfig, option string, defaultValue string) printerpkg.Template {
	cfg := requireMacroConfig(config)
	name := fmt.Sprintf("%s:%s", cfg.Get_name(), option)
	defaultOptionValue := interface{}(defaultValue)
	if value.IsNone(defaultValue) {
		defaultOptionValue = object.Sentinel{}
	}
	script := cast.ToString(cfg.Get(option, defaultOptionValue, true))
	return NewMacroTemplate(config.Printer(), jinja2.NewEnvironment(), name, script)
}

func NewMacroTemplate(printer printerpkg.ModulePrinter, env *jinja2.Environment, name string, script string) printerpkg.Template {
	template, err := env.From_string(script)
	if err != nil {
		msg := fmt.Sprintf("Error loading template '%s': %s", name, err)
		logger.Error(msg)
		panic(msg)
	}
	return &MacroTemplate{
		printer:  printer,
		gcode:    printer.GCode(),
		template: template,
	}
}

func (self *MacroTemplate) CreateContext(eventtime interface{}) map[string]interface{} {
	return CreateTemplateContext(self.printer, eventtime)
}

func (self *MacroTemplate) Render(context map[string]interface{}) (string, error) {
	if context == nil {
		context = self.CreateContext(nil)
	}
	return self.template.Render(context)
}

func (self *MacroTemplate) RunGcodeFromCommand(context map[string]interface{}) error {
	content, err := self.Render(context)
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, "    ", "\n")
	self.gcode.RunScriptFromCommand(content)
	return nil
}

func CreateTemplateContext(printer printerpkg.ModulePrinter, eventtime interface{}) map[string]interface{} {
	respondInfo := func(arg interface{}, _ interface{}) interface{} {
		msg := cast.ToString(arg)
		printer.GCode().RespondInfo(msg, true)
		return ""
	}
	raiseError := func(arg interface{}, _ interface{}) interface{} {
		return fmt.Errorf("_action_raise_error: %v", arg)
	}
	emergencyStop := func(arg interface{}, _ interface{}) interface{} {
		msg := cast.ToString(arg)
		if msg == "" {
			msg = "action_emergency_stop"
		}
		printer.InvokeShutdown(fmt.Sprintf("Shutdown due to %s", msg))
		return ""
	}
	callRemote := func(arg interface{}, kwargs interface{}) interface{} {
		method := cast.ToString(arg)
		caller, ok := printer.Webhooks().(macroWebhookCaller)
		if !ok {
			err := fmt.Errorf("webhook registry does not support remote methods")
			logger.Errorf("Remote Call Error, method: %s, error: %v", method, err)
			return err
		}
		if err := caller.CallRemoteMethod(method, kwargs); err != nil {
			logger.Errorf("Remote Call Error, method: %s, error: %v", method, err)
			return err
		}
		return ""
	}
	return map[string]interface{}{
		"printer":                   newMacroStatusWrapper(printer, cast.ToFloat64(eventtime)),
		"action_emergency_stop":     emergencyStop,
		"action_respond_info":       respondInfo,
		"action_raise_error":        raiseError,
		"action_call_remote_method": callRemote,
	}
}

type GCodeMacro struct {
	alias          string
	printer        printerpkg.ModulePrinter
	template       printerpkg.Template
	gcode          macroGCode
	renameExisting string
	cmdDesc        string
	inScript       bool
	variables      map[string]interface{}
}

func NewGCodeMacro(config printerpkg.ModuleConfig) *GCodeMacro {
	cfg := requireMacroConfig(config)
	self := &GCodeMacro{}
	names := strings.Split(cfg.Get_name(), " ")
	if len(names) > 2 {
		panic(fmt.Errorf("Name of section '%v' contains illegal whitespace", cfg.Get_name()))
	}
	name := names[1]
	self.alias = strings.ToUpper(name)
	self.printer = cfg.Printer()
	self.template = config.LoadTemplate("gcode_macro_1", "gcode", value.StringNone)
	self.gcode = requireMacroGCode(self.printer.GCode())
	self.renameExisting = cast.ToString(cfg.Get("rename_existing", value.None, true))
	self.cmdDesc = cast.ToString(cfg.Get("description", "G-Code macro", true))
	if self.renameExisting != "" {
		if self.gcode.IsTraditionalGCode(self.alias) != self.gcode.IsTraditionalGCode(self.renameExisting) {
			panic(fmt.Errorf("G-Code macro rename of different types ('%s' vs '%s')", self.alias, self.renameExisting))
		}
		self.printer.RegisterEventHandler("project:connect", self.handleConnect)
	} else {
		self.gcode.RegisterCommand(self.alias, self.cmd, false, self.cmdDesc)
	}
	self.gcode.RegisterMuxCommand("SET_GCODE_VARIABLE", "MACRO", name, self.cmdSetGCodeVariable, cmdSetGCodeVariableHelp)
	self.variables = make(map[string]interface{})
	prefix := "variable_"
	for _, option := range cfg.Get_prefix_options(prefix) {
		self.variables[option[len(prefix):]] = cfg.Get(option, object.Sentinel{}, true)
	}
	return self
}

func (self *GCodeMacro) handleConnect(_ []interface{}) error {
	prevCmd := self.gcode.ReplaceCommand(self.alias, self.cmd, false, self.cmdDesc)
	if prevCmd == nil {
		return fmt.Errorf("Existing command '%s' not found in gcode_macro rename", self.alias)
	}
	pdesc := fmt.Sprintf("Renamed builtin of '%s'", self.alias)
	self.gcode.RegisterCommand(self.renameExisting, prevCmd, false, pdesc)
	return nil
}

func (self *GCodeMacro) Get_status(eventtime float64) map[string]interface{} {
	return sys.DeepCopyMap(self.variables)
}

func (self *GCodeMacro) cmdSetGCodeVariable(gcmd printerpkg.Command) error {
	cmd := requireMacroCommand(gcmd)
	variable := cmd.Get("VARIABLE", object.Sentinel{}, nil, nil, nil, nil, nil)
	valueText := cmd.Get("VALUE", object.Sentinel{}, nil, nil, nil, nil, nil)
	if _, ok := self.variables[variable]; !ok {
		return fmt.Errorf("Unknown gcode_macro variable '%s'", variable)
	}
	parsed, err := LiteralEval.LiteralEval(valueText)
	if err != nil {
		return fmt.Errorf("Unable to parse '%s' as a literal: %s", valueText, err)
	}
	jsonBytes, err := json.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("Unable to parse '%s' as a literal: %s", valueText, err)
	}
	self.variables[variable] = jsonBytes
	return nil
}

func (self *GCodeMacro) cmd(gcmd printerpkg.Command) error {
	if self.inScript {
		return fmt.Errorf("Macro %s called recursively", self.alias)
	}
	kwparams := make(map[string]interface{})
	for k, v := range self.variables {
		kwparams[k] = v
	}
	for k, v := range self.template.CreateContext(nil) {
		kwparams[k] = v
	}
	cmd := requireMacroCommand(gcmd)
	kwparams["params"] = gcmd.Parameters()
	kwparams["rawparams"] = cmd.RawParameters()
	self.inScript = true
	defer func() { self.inScript = false }()
	return self.template.RunGcodeFromCommand(kwparams)
}

func LoadConfigGCodeMacro(config printerpkg.ModuleConfig) interface{} {
	return NewGCodeMacro(config)
}
