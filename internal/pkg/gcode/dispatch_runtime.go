package gcode

import (
	"container/list"
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/reflects"
	"reflect"
	"runtime/debug"
	"strings"
)

type DispatchRuntimeOptions struct {
	ExtendParams func(interface{}) interface{}
}

type DispatchProcessOptions struct {
	NewCommand     func(ParsedDispatchCommand, bool) *DispatchCommand
	DefaultHandler func(*DispatchCommand) error
	HandleError    func(ParsedDispatchCommand, error)
}

type DispatchRuntime struct {
	BaseHandlers  map[string]interface{}
	ReadyHandlers map[string]interface{}
	MuxCommands   map[string]list.List
	Help          map[string]string

	activeHandlers map[string]interface{}
	isPrinterReady bool
	extendParams   func(interface{}) interface{}
}

type ParsedDispatchCommand struct {
	OriginalLine string
	Command      string
	Params       map[string]string
}

func NewDispatchRuntime(options DispatchRuntimeOptions) *DispatchRuntime {
	self := &DispatchRuntime{
		BaseHandlers:   map[string]interface{}{},
		ReadyHandlers:  map[string]interface{}{},
		MuxCommands:    map[string]list.List{},
		Help:           map[string]string{},
		activeHandlers: map[string]interface{}{},
		extendParams:   options.ExtendParams,
	}
	self.activeHandlers = self.BaseHandlers
	return self
}

func (self *DispatchRuntime) ActiveHandlers() map[string]interface{} {
	return self.activeHandlers
}

func (self *DispatchRuntime) IsPrinterReady() bool {
	return self.isPrinterReady
}

func (self *DispatchRuntime) SetReady(ready bool) {
	self.isPrinterReady = ready
	if ready {
		self.activeHandlers = self.ReadyHandlers
		return
	}
	self.activeHandlers = self.BaseHandlers
}

func (self *DispatchRuntime) RegisterCommand(cmd string, handler interface{}, whenNotReady bool, desc string) interface{} {
	if handler == nil {
		oldCmd := self.ReadyHandlers[cmd]
		if self.ReadyHandlers[cmd] != nil {
			delete(self.ReadyHandlers, cmd)
		}
		if self.BaseHandlers[cmd] != nil {
			delete(self.BaseHandlers, cmd)
		}
		return oldCmd
	}
	if _, ok := self.ReadyHandlers[cmd]; ok {
		panic(fmt.Sprintf("gcode command %s already registered", cmd))
	}

	wrapped := handler
	if !IsTraditionalGCode(cmd) {
		if !IsCommandValid(cmd) {
			panic(fmt.Errorf("Can't register '%s' as it is an invalid name", cmd))
		}
		wrapped = self.wrapExtendedCommand(cmd, handler)
	}

	self.ReadyHandlers[cmd] = wrapped
	if whenNotReady {
		self.BaseHandlers[cmd] = wrapped
	}
	if desc != "" {
		self.Help[cmd] = desc
	}
	return nil
}

func (self *DispatchRuntime) wrapExtendedCommand(cmd string, handler interface{}) interface{} {
	origfunc := handler
	return func(params interface{}) error {
		paras := params
		if self.extendParams != nil {
			paras = self.extendParams(params)
		}
		if reflect.TypeOf(origfunc).Name() == "Value" {
			reflects.ReqArgs(origfunc.(reflect.Value), map[string]interface{}{"gcmd": paras})
		} else {
			err := origfunc.(func(interface{}) error)(paras)
			if err != nil {
				logger.Error("Register_command ", cmd, " error:", err, string(debug.Stack()))
				panic(err)
			}
		}
		return nil
	}
}

func (self *DispatchRuntime) RegisterMuxCommand(cmd string, key string, value string, handler func(interface{}) error, desc string) {
	prev, ok := self.MuxCommands[cmd]
	if !ok && prev.Len() <= 0 {
		prev.PushBack(key)
		prev.PushBack(map[string]interface{}{})
		self.MuxCommands[cmd] = prev
	}
	prevKey, prevValues := prev.Front(), prev.Back()
	if prevKey.Value.(string) != key {
		panic(fmt.Sprintf("mux command %s %s %s may have only one key (%#v)", cmd, key, value, prevKey))
	}
	if prevValues.Value.(map[string]interface{})[value] != nil {
		panic(fmt.Sprintf("mux command %s %s %s already registered (%#v)", cmd, key, value, prevValues))
	}
	prevValues.Value.(map[string]interface{})[value] = handler
}

func invokeDispatchHandler(handler interface{}, gcmd *DispatchCommand) error {
	if reflect.TypeOf(handler).Name() == "Value" {
		argv := []reflect.Value{reflect.ValueOf(gcmd)}
		res := handler.(reflect.Value).Call(argv)
		if len(res) > 1 {
			logger.Debug(res)
		}
		return nil
	}
	return handler.(func(interface{}) error)(gcmd)
}

func (self *DispatchRuntime) ProcessCommands(commands []string, needAck bool, options DispatchProcessOptions) {
	if options.NewCommand == nil {
		panic("dispatch runtime requires command factory")
	}
	for _, line := range commands {
		parsed := ParseDispatchCommandLine(line)
		gcmd := options.NewCommand(parsed, needAck)
		handler := self.ActiveHandlers()[parsed.Command]
		if handler == nil {
			if options.DefaultHandler == nil {
				gcmd.Ack("")
				continue
			}
			if err := options.DefaultHandler(gcmd); err != nil {
				if options.HandleError != nil {
					options.HandleError(parsed, err)
					continue
				}
				panic(err)
			}
			gcmd.Ack("")
			continue
		}
		if err := invokeDispatchHandler(handler, gcmd); err != nil {
			if options.HandleError != nil {
				options.HandleError(parsed, err)
				continue
			}
			panic(err)
		}
		gcmd.Ack("")
	}
}

func ParseDispatchCommandLine(input string) ParsedDispatchCommand {
	line := strings.Trim(input, " ")
	origline := line
	if cpos := strings.Index(line, ";"); cpos != -1 {
		line = line[:cpos]
	}

	parts := ParseGcodeTokens(strings.ToUpper(line))
	cmd := ""
	if len(parts) >= 3 && parts[1] == "N" {
		cmd = strings.TrimSpace(strings.Join(parts[3:], ""))
	} else if len(parts) >= 3 {
		cmd = strings.TrimSpace(strings.Join(parts[0:3], ""))
	}

	params := make(map[string]string)
	for i := 1; i < len(parts)-1; i += 2 {
		key := parts[i]
		value := ""
		if i+1 < len(parts) {
			value = strings.TrimSpace(parts[i+1])
		}
		params[key] = value
	}

	return ParsedDispatchCommand{
		OriginalLine: origline,
		Command:      cmd,
		Params:       params,
	}
}
