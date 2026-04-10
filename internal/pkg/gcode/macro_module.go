package gcode

import (
	"encoding/json"
	"fmt"
	"goklipper/common/utils/LiteralEval"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/utils/sys"
	"goklipper/common/value"
	printerpkg "goklipper/internal/pkg/printer"
	"strings"
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

type GCodeMacro struct {
	alias           string
	printer         printerpkg.ModulePrinter
	template        printerpkg.Template
	gcode           macroGCode
	renameExisting  string
	cmdDesc         string
	inScript        bool
	variables       map[string]interface{}
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