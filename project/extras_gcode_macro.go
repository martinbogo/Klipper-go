package project

import (
	"fmt"
	"goklipper/common/jinja2"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/value"
	gcodepkg "goklipper/internal/pkg/gcode"
	"reflect"
	"strings"
)

type GetStatusWrapper struct {
	printer   *Printer
	eventtime float64
	cache     map[string]interface{}
}

func NewGetStatusWrapper(printer *Printer, eventtime float64) *GetStatusWrapper {
	self := new(GetStatusWrapper)

	self.printer = printer
	self.eventtime = eventtime
	self.cache = make(map[string]interface{})
	return self
}

func (self *GetStatusWrapper) __getitem__(val string) (interface{}, error) {
	sval := strings.TrimSpace(val)
	if _, ok := self.cache[sval]; ok {
		return self.cache[sval], nil
	}

	po := self.printer.Lookup_object(sval, nil)
	if po == nil {
		//raise KeyError(val)
		panic(val)
	}
	if self.eventtime == 0 {
		self.eventtime = self.printer.Get_reactor().Monotonic()
	}
	if reflect.ValueOf(po).MethodByName("Get_status").IsNil() {
		return nil, fmt.Errorf("GetStatusWrapper %#v not method Get_status", po)
	}
	res := reflect.ValueOf(po).MethodByName("Get_status").Call([]reflect.Value{
		reflect.ValueOf(self.eventtime),
	})

	var ret = make(map[string]interface{})
	if len(res) >= 1 && res[0].Type().Kind() == reflect.Map { // assert res[0] is map[string]float64 map[string]interface{}
		for _, key := range res[0].MapKeys() {
			ret[key.String()] = res[0].MapIndex(key).Interface()
			self.cache[key.String()] = ret[key.String()]
		}
	}

	return ret[sval], nil
}

func (self *GetStatusWrapper) __contains__(val string) bool {
	_, err := self.__getitem__(val)
	if err == nil {
		return true
	}
	return false
}

func (self *GetStatusWrapper) Iter() <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, obj := range self.printer.Lookup_objects("") {
			for name, _ := range obj.(map[string]interface{}) {
				if self.__contains__(name) {
					ch <- name
				}
			}
		}
	}()
	return ch
}

type TemplateWrapper struct {
	printer                 *Printer
	name                    string
	gcode                   *GCodeDispatch
	create_template_context func(interface{}) interface{}
	template                *jinja2.Template
}

func NewTemplateWrapper(printer *Printer, env *jinja2.Environment, name, script string) *TemplateWrapper {
	self := new(TemplateWrapper)
	self.printer = printer
	self.name = name
	obj := self.printer.Lookup_object("gcode", object.Sentinel{})
	self.gcode = obj.(*GCodeDispatch)

	obj = self.printer.Lookup_object("gcode_macro_1", object.Sentinel{})
	gcode_macro := obj.(*PrinterGCodeMacro)
	self.create_template_context = gcode_macro.Create_template_context

	var err error
	self.template, err = env.From_string(script)
	if err != nil {
		msg := fmt.Sprintf("Error loading template '%s': %s", name, err)
		logger.Error(msg)
		panic(msg)
	}
	return self
}

func (self *TemplateWrapper) Render(context map[string]interface{}) (string, error) {
	if context == nil {
		ctx := self.create_template_context(nil)

		context, _ = ctx.(map[string]interface{})
	}
	return self.template.Render(context)
}

func (self *TemplateWrapper) CreateContext(eventtime interface{}) map[string]interface{} {
	return self.create_template_context(eventtime).(map[string]interface{})
}

func (self *TemplateWrapper) Run_gcode_from_command(context map[string]interface{}) error {
	content, err := self.Render(context)
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, "    ", "\n")
	self.gcode.Run_script_from_command(content)
	return nil
}

func (self *TemplateWrapper) RunGcodeFromCommand(context map[string]interface{}) error {
	return self.Run_gcode_from_command(context)
}

type PrinterGCodeMacro struct {
	printer *Printer
	env     *jinja2.Environment
}

func NewPrinterGCodeMacro(config *ConfigWrapper) *PrinterGCodeMacro {
	self := new(PrinterGCodeMacro)
	self.printer = config.Get_printer()
	self.env = jinja2.NewEnvironment()
	return self
}

func (self *PrinterGCodeMacro) Load_template(config *ConfigWrapper, option, def string) *TemplateWrapper {
	name := fmt.Sprintf("%s:%s", config.Get_name(), option)

	var script string
	if value.IsNone(def) {
		script = cast.ToString(config.Get(option, object.Sentinel{}, true))
	} else {
		script = cast.ToString(config.Get(option, def, true))
	}
	return NewTemplateWrapper(self.printer, self.env, name, script)
}

func (self *PrinterGCodeMacro) _action_emergency_stop(arg interface{}, _ interface{}) interface{} {
	msg := cast.ToString(arg)
	if msg == "" {
		msg = "action_emergency_stop"
	}
	self.printer.Invoke_shutdown(fmt.Sprintf("Shutdown due to %s", (msg)))
	return ""
}

func (self *PrinterGCodeMacro) _action_respond_info(arg interface{}, _ interface{}) interface{} {
	msg := cast.ToString(arg)
	obj := self.printer.Lookup_object("gcode", object.Sentinel{})
	obj.(*GCodeDispatch).Respond_info(msg, true)
	return ""
}

func (self *PrinterGCodeMacro) _action_raise_error(arg interface{}, _ interface{}) interface{} {
	return fmt.Errorf("_action_raise_error: %v", arg)
}

func (self *PrinterGCodeMacro) _action_call_remote_method(arg interface{}, kwargs interface{}) interface{} {
	obj := self.printer.Lookup_object("webhooks", object.Sentinel{})
	webhooks := obj.(*WebHooks)

	method := cast.ToString(arg)
	err := webhooks.Call_remote_method(method, kwargs)
	if err != nil {
		logger.Errorf("Remote Call Error, method: %s, error: %v", method, err)
		return err
	}
	return ""
}

func (self *PrinterGCodeMacro) Create_template_context(arg interface{}) interface{} {
	return map[string]interface{}{
		"printer":                   NewGetStatusWrapper(self.printer, cast.ToFloat64(arg)),
		"action_emergency_stop":     self._action_emergency_stop,
		"action_respond_info":       self._action_respond_info,
		"action_raise_error":        self._action_raise_error,
		"action_call_remote_method": self._action_call_remote_method,
	}
}

func Load_config_printer_gcode_macro(config *ConfigWrapper) interface{} {
	return NewPrinterGCodeMacro(config)
}

func Load_config_gcode_macro(config *ConfigWrapper) interface{} {
	return gcodepkg.LoadConfigGCodeMacro(config)
}
