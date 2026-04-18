package gcode

import (
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	printerpkg "goklipper/internal/pkg/printer"
	"sort"
	"strings"
)

type DispatcherHost interface {
	StartArgs() map[string]interface{}
	RegisterEventHandler(event string, callback func([]interface{}) error)
	LookupObject(name string, defaultValue interface{}) interface{}
	InvokeShutdown(msg string)
	SendEvent(event string, params []interface{})
	RequestExit(result string)
	StateMessage() (string, string)
}

type DispatcherHostFuncs struct {
	StartArgsFunc            func() map[string]interface{}
	RegisterEventHandlerFunc func(string, func([]interface{}) error)
	LookupObjectFunc         func(string, interface{}) interface{}
	InvokeShutdownFunc       func(string)
	SendEventFunc            func(string, []interface{})
	RequestExitFunc          func(string)
	StateMessageFunc         func() (string, string)
}

var _ DispatcherHost = (*DispatcherHostFuncs)(nil)

func (f *DispatcherHostFuncs) StartArgs() map[string]interface{} {
	if f == nil || f.StartArgsFunc == nil {
		return nil
	}
	return f.StartArgsFunc()
}

func (f *DispatcherHostFuncs) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if f == nil || f.RegisterEventHandlerFunc == nil {
		return
	}
	f.RegisterEventHandlerFunc(event, callback)
}

func (f *DispatcherHostFuncs) LookupObject(name string, defaultValue interface{}) interface{} {
	if f == nil || f.LookupObjectFunc == nil {
		return defaultValue
	}
	return f.LookupObjectFunc(name, defaultValue)
}

func (f *DispatcherHostFuncs) InvokeShutdown(msg string) {
	if f == nil || f.InvokeShutdownFunc == nil {
		return
	}
	f.InvokeShutdownFunc(msg)
}

func (f *DispatcherHostFuncs) SendEvent(event string, params []interface{}) {
	if f == nil || f.SendEventFunc == nil {
		return
	}
	f.SendEventFunc(event, params)
}

func (f *DispatcherHostFuncs) RequestExit(result string) {
	if f == nil || f.RequestExitFunc == nil {
		return
	}
	f.RequestExitFunc(result)
}

func (f *DispatcherHostFuncs) StateMessage() (string, string) {
	if f == nil || f.StateMessageFunc == nil {
		return "", ""
	}
	return f.StateMessageFunc()
}

type dispatcherToolhead interface {
	Get_last_move_time() float64
	Dwell(delay float64)
	Wait_moves()
}

type DispatcherOptions struct {
	Host   DispatcherHost
	Lock   func()
	Unlock func()
	IsBusy func() bool
}

type Dispatcher struct {
	Coord                     []string
	host                      DispatcherHost
	dispatch                  *DispatchRuntime
	isFileInput               bool
	outputCallbacks           []func(string)
	isPrinterReady            bool
	lock                      func()
	unlock                    func()
	isBusy                    func() bool
	Cmd_RESTART_help          string
	Cmd_FIRMWARE_RESTART_help string
	Cmd_STATUS_help           string
	Cmd_HELP_help             string
}

type dispatcherMutexAdapter struct {
	lock   func()
	unlock func()
}

func (self *dispatcherMutexAdapter) Lock() {
	if self.lock != nil {
		self.lock()
	}
}

func (self *dispatcherMutexAdapter) Unlock() {
	if self.unlock != nil {
		self.unlock()
	}
}

func NewDispatcher(options DispatcherOptions) *Dispatcher {
	if options.Host == nil {
		panic("gcode dispatcher requires host")
	}
	if options.Lock == nil {
		options.Lock = func() {}
	}
	if options.Unlock == nil {
		options.Unlock = func() {}
	}
	if options.IsBusy == nil {
		options.IsBusy = func() bool { return false }
	}

	self := &Dispatcher{
		host:            options.Host,
		outputCallbacks: []func(string){},
		lock:            options.Lock,
		unlock:          options.Unlock,
		isBusy:          options.IsBusy,
		dispatch: NewDispatchRuntime(DispatchRuntimeOptions{
			ExtendParams: func(params interface{}) interface{} {
				gcmd := params.(*DispatchCommand)
				gcmd.Params, _ = ParseExtendedParams(gcmd.RawParameters())
				return gcmd
			},
		}),
	}

	if isCheck, ok := options.Host.StartArgs()["debuginput"]; ok && fmt.Sprint(isCheck) != "" {
		self.isFileInput = true
	}

	options.Host.RegisterEventHandler("project:ready", self.Handle_ready)
	options.Host.RegisterEventHandler("project:shutdown", self.Handle_shutdown)
	options.Host.RegisterEventHandler("project:disconnect", self.Handle_disconnect)

	self.Cmd_RESTART_help = "Reload config file and restart host software"
	self.Cmd_FIRMWARE_RESTART_help = "Restart firmware, host, and reload config"
	self.Cmd_STATUS_help = "Report the printer status"
	self.Cmd_HELP_help = "Report the list of available extended G-Code commands"

	for _, binding := range []struct {
		cmd          string
		whenNotReady bool
		desc         string
		handler      func(interface{}) error
	}{
		{cmd: "M110", whenNotReady: true, handler: func(arg interface{}) error { self.Cmd_M110(arg); return nil }},
		{cmd: "M115", whenNotReady: true, handler: func(arg interface{}) error { self.Cmd_M115(arg); return nil }},
		{cmd: "M117", whenNotReady: true, handler: func(arg interface{}) error { self.Cmd_default(arg); return nil }},
		{cmd: "RESTART", whenNotReady: true, desc: self.Cmd_RESTART_help, handler: func(arg interface{}) error { self.Cmd_RESTART(arg); return nil }},
		{cmd: "FIRMWARE_RESTART", whenNotReady: true, desc: self.Cmd_FIRMWARE_RESTART_help, handler: func(arg interface{}) error { self.Cmd_FIRMWARE_RESTART(arg); return nil }},
		{cmd: "ECHO", whenNotReady: true, handler: func(arg interface{}) error { self.Cmd_ECHO(arg); return nil }},
		{cmd: "STATUS", whenNotReady: true, desc: self.Cmd_STATUS_help, handler: func(arg interface{}) error { self.Cmd_STATUS(arg); return nil }},
		{cmd: "HELP", whenNotReady: true, desc: self.Cmd_HELP_help, handler: func(arg interface{}) error { self.Cmd_HELP(arg); return nil }},
	} {
		self.Register_command(binding.cmd, binding.handler, binding.whenNotReady, binding.desc)
	}

	self.Coord = []string{"x", "y", "z", "e"}
	return self
}

func (self *Dispatcher) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	self.Register_command(cmd, func(arg interface{}) error {
		return handler(arg.(*DispatchCommand))
	}, whenNotReady, desc)
}

func (self *Dispatcher) GetCommandHelp() map[string]string {
	return self.Get_command_help()
}

func (self *Dispatcher) RegisterOutputHandler(cb func(string)) {
	self.Register_output_handler(cb)
}

func (self *Dispatcher) RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string) {
	self.Register_mux_command(cmd, key, value, func(arg interface{}) error {
		return handler(arg.(*DispatchCommand))
	}, desc)
}

func (self *Dispatcher) CreateCommand(cmd string, raw string, params map[string]string) printerpkg.Command {
	return self.Create_gcode_command(cmd, raw, params)
}

func (self *Dispatcher) IsTraditionalGCode(cmd string) bool {
	return IsTraditionalGCode(cmd)
}

func (self *Dispatcher) RunScriptFromCommand(script string) {
	self.Run_script_from_command(script)
}

func (self *Dispatcher) RunScript(script string) {
	self.Run_script(script)
}

func (self *Dispatcher) IsBusy() bool {
	return self.isBusy()
}

func (self *Dispatcher) IsPrinterReady() bool {
	return self.isPrinterReady
}

func (self *Dispatcher) Mutex() printerpkg.Mutex {
	return &dispatcherMutexAdapter{lock: self.lock, unlock: self.unlock}
}

func (self *Dispatcher) RespondInfo(msg string, log bool) {
	self.Respond_info(msg, log)
}

func (self *Dispatcher) RespondRaw(msg string) {
	self.Respond_raw(msg)
}

func (self *Dispatcher) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	oldRaw := self.Register_command(cmd, nil, whenNotReady, desc)
	var oldHandler func(printerpkg.Command) error
	if oldRaw != nil {
		oldHandler = func(command printerpkg.Command) error {
			return oldRaw.(func(interface{}) error)(command.(*DispatchCommand))
		}
	}
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return oldHandler
}

func (self *Dispatcher) Register_command(cmd string, handler interface{}, whenNotReady bool, desc string) interface{} {
	return self.dispatch.RegisterCommand(cmd, handler, whenNotReady, desc)
}

func (self *Dispatcher) Register_mux_command(cmd string, key string, value string, handler func(interface{}) error, desc string) {
	if _, ok := self.dispatch.MuxCommands[cmd]; !ok {
		muxHandler := func(gcmd interface{}) error {
			return self.Cmd_mux(cmd, gcmd)
		}
		self.dispatch.RegisterCommand(cmd, muxHandler, false, desc)
	}
	self.dispatch.RegisterMuxCommand(cmd, key, value, handler, desc)
}

func (self *Dispatcher) Get_command_help() map[string]string {
	return self.dispatch.Help
}

func (self *Dispatcher) Register_output_handler(cb func(string)) {
	self.outputCallbacks = append(self.outputCallbacks, cb)
}

func (self *Dispatcher) Handle_shutdown([]interface{}) error {
	if !self.isPrinterReady {
		return nil
	}
	self.dispatch.SetReady(false)
	self.isPrinterReady = false
	self.Respond_state("Shutdown")
	return nil
}

func (self *Dispatcher) Handle_disconnect([]interface{}) error {
	self.Respond_state("Disconnect")
	return nil
}

func (self *Dispatcher) Handle_ready([]interface{}) error {
	self.dispatch.SetReady(true)
	self.isPrinterReady = true
	self.Respond_state("Ready")
	return nil
}

func (self *Dispatcher) Run_script_from_command(script string) {
	self.dispatch.ProcessCommands(strings.Split(script, "\n"), false, DispatchProcessOptions{
		NewCommand: self.newCommand,
		DefaultHandler: func(gcmd *DispatchCommand) error {
			return self.Cmd_default(gcmd)
		},
		HandleError: self.handleDispatchError,
	})
}

func (self *Dispatcher) Run_script(script string) {
	if script == "CANCEL_PRINT" {
		self.host.SendEvent("project:pre_cancel", nil)
	}
	self.lock()
	defer func() {
		self.unlock()
		if script == "CANCEL_PRINT" {
			self.host.SendEvent("project:post_cancel", nil)
		}
	}()
	self.dispatch.ProcessCommands(strings.Split(script, "\n"), false, DispatchProcessOptions{
		NewCommand: self.newCommand,
		DefaultHandler: func(gcmd *DispatchCommand) error {
			return self.Cmd_default(gcmd)
		},
		HandleError: self.handleDispatchError,
	})
}

func (self *Dispatcher) Create_gcode_command(command string, commandline string, params map[string]string) *DispatchCommand {
	return NewDispatchCommand(self.Respond_info, self.Respond_raw, command, commandline, params, false)
}

func (self *Dispatcher) newCommand(parsed ParsedDispatchCommand, needAck bool) *DispatchCommand {
	return NewDispatchCommand(self.Respond_info, self.Respond_raw, parsed.Command, parsed.OriginalLine, parsed.Params, needAck)
}

func (self *Dispatcher) handleDispatchError(parsed ParsedDispatchCommand, err error) {
	logger.Error(err)
	msg := fmt.Sprintf("Internal error on command: %s \n", parsed.Command)
	self.host.InvokeShutdown(msg)
	self.Respond_error(msg)
	panic(err)
}

func (self *Dispatcher) Respond_raw(msg string) {
	for _, cb := range self.outputCallbacks {
		cb(msg)
	}
}

func (self *Dispatcher) Respond_info(msg string, log bool) {
	if log {
		logger.Info(msg)
		lines := make([]string, 0)
		for _, line := range strings.Split(strings.TrimSpace(msg), "\n") {
			lines = append(lines, strings.TrimSpace(line))
		}
		self.Respond_raw(strings.Join(lines, "\n // "))
		return
	}
	logger.Info(msg)
}

func (self *Dispatcher) Respond_error(msg string) {
	lines := strings.Split(strings.TrimSpace(msg), "\n")
	if len(lines) > 1 {
		self.Respond_info(fmt.Sprintf("%s\n", lines), true)
	}
	self.Respond_raw(fmt.Sprintf("!! %s", strings.TrimSpace(lines[0])))
	if self.isFileInput {
		self.host.RequestExit("error_exit")
	}
}

func (self *Dispatcher) Respond_state(state string) {
	self.Respond_info(fmt.Sprintf("project state: %s", state), true)
}

func (self *Dispatcher) Cmd_default(argv interface{}) error {
	gcmd := argv.(*DispatchCommand)
	cmd := gcmd.Command
	if cmd == "M105" {
		gcmd.Ack("T:0")
		return nil
	}
	if cmd == "M21" {
		return nil
	}
	if !self.isPrinterReady {
		return nil
	}

	if strings.Contains(cmd, "") {
		realcmd := strings.Split(cmd, " ")[0]
		if realcmd == "M117" || realcmd == "M118" || realcmd == "M23" {
			handler := self.dispatch.ActiveHandlers()[realcmd]
			if handler != nil {
				gcmd.Command = realcmd
				return handler.(func(interface{}) error)(gcmd)
			}
		}
	} else if cmd == "M140" || cmd == "M104" && gcmd.Get_float("S", 0., nil, nil, nil, nil) == -1 {
		return nil
	} else if cmd == "M107" || (cmd == "M106" && (gcmd.Get_float("S", 1., nil, nil, nil, nil) == -1 || self.isFileInput)) {
		return nil
	}
	if cmd != "" {
		logger.Warn("Unknown command:", cmd)
	}
	return nil
}

func (self *Dispatcher) Cmd_mux(command string, argv interface{}) error {
	gcmd := argv.(*DispatchCommand)
	cmdList := self.dispatch.MuxCommands[command]
	key := cmdList.Remove(cmdList.Front()).(string)
	values := cmdList.Remove(cmdList.Back())
	var keyParam string
	if _, ok := values.(map[string]interface{})[""]; ok {
		keyParam = gcmd.Get(key, nil, "", nil, nil, nil, nil)
	} else {
		keyParam = gcmd.Get(key, object.Sentinel{}, "", nil, nil, nil, nil)
	}
	vals := values.(map[string]interface{})
	if vals[keyParam] == nil {
		panic(fmt.Sprintf("The value %s is not valid for %s", keyParam, key))
	}
	return vals[keyParam].(func(interface{}) error)(argv)
}

func (self *Dispatcher) Cmd_M110(argv interface{}) {
	logger.Debug("Cmd_M110")
}

func (self *Dispatcher) Cmd_M112(argv interface{}) {
	self.host.InvokeShutdown("Shutdown due to M112 command")
}

func (self *Dispatcher) Cmd_M115(argv interface{}) {
	gcmd := argv.(*DispatchCommand)
	softwareVersion := self.host.StartArgs()["software_version"]
	kw := map[string]string{"FIRMWARE_NAME": "project", "FIRMWARE_VERSION": softwareVersion.(string)}
	msg := ""
	for key, value := range kw {
		msg += fmt.Sprintf("%s:%s ", key, value)
	}

	if didAck := gcmd.Ack(msg); !didAck {
		gcmd.Respond_info(msg, true)
	}
}

func (self *Dispatcher) Request_restart(result string) {
	if self.isPrinterReady {
		toolheadObj := self.host.LookupObject("toolhead", object.Sentinel{})
		toolhead, ok := toolheadObj.(dispatcherToolhead)
		if !ok {
			panic(fmt.Errorf("lookup object %s type invalid: %#v", "toolhead", toolheadObj))
		}
		printTime := toolhead.Get_last_move_time()
		if result == "exit" {
			logger.Info("Exiting (print time %.3fs)", printTime)
		}
		self.host.SendEvent("gcode:request_restart", []interface{}{printTime})
		toolhead.Dwell(0.500)
		toolhead.Wait_moves()
	}
	self.host.RequestExit(result)
}

func (self *Dispatcher) Cmd_RESTART(argv interface{}) {
	self.Request_restart("restart")
}

func (self *Dispatcher) Cmd_FIRMWARE_RESTART(argv interface{}) {
	self.Request_restart("firmware_restart")
}

func (self *Dispatcher) Cmd_ECHO(argv interface{}) {
	gcmd := argv.(*DispatchCommand)
	gcmd.Respond_info(gcmd.Commandline, true)
}

func (self *Dispatcher) Cmd_STATUS(argv interface{}) {
	if self.isPrinterReady {
		self.Respond_state("Ready")
		return
	}
	msg, _ := self.host.StateMessage()
	msg = strings.TrimRight(msg, "") + "\n state: Not ready"
	panic(msg)
}

func (self *Dispatcher) Cmd_HELP(argv interface{}) {
	gcmd := argv.(*DispatchCommand)
	cmdhelp := make([]string, 0)
	if !self.isPrinterReady {
		cmdhelp = append(cmdhelp, "Printer is not ready - not all commands available.")
	}
	cmdhelp = append(cmdhelp, "Available extended commands:")
	keys := make([]string, 0, len(self.dispatch.ActiveHandlers()))
	for key := range self.dispatch.ActiveHandlers() {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	for _, cmd := range keys {
		if self.dispatch.Help[cmd] != "" {
			cmdhelp = append(cmdhelp, fmt.Sprintf("%-10s: %s", cmd, self.dispatch.Help[cmd]))
		}
	}

	gcmd.Respond_info(strings.Join(cmdhelp, "\n"), true)
}
