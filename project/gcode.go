// Parse gcode commands
//
// Copyright (C) 2016-2021  Kevin O"Connor <kevin@koconnor.net>
//
// This file may be distributed under the terms of the GNU GPLv3 license.
package project

import (
	"container/list"
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	"goklipper/common/utils/reflects"
	gcodepkg "goklipper/internal/pkg/gcode"
	printerpkg "goklipper/internal/pkg/printer"
	"reflect"
	"runtime/debug"

	//"os"
	"regexp"
	"sort"
	"strings"
)

type CommandError struct {
	E string
}

type GCodeCommand struct {
	gcodepkg.CommandParams // provides Command, Commandline, Params and typed accessors
	Need_ack               bool
	Respond_info           func(msg string, log bool)
	Respond_raw            func(msg string)
}

func NewGCodeCommand(gcode *GCodeDispatch, command string, commandline string, params map[string]string, need_ack bool) *GCodeCommand {
	var self = GCodeCommand{}
	self.CommandParams = gcodepkg.CommandParams{
		Command:     command,
		Commandline: commandline,
		Params:      params,
	}
	self.Need_ack = need_ack
	// Method wrappers
	self.Respond_info = gcode.Respond_info
	self.Respond_raw = gcode.Respond_raw
	return &self
}

func (self *GCodeCommand) Get_command() string {
	return self.Command
}

func (self *GCodeCommand) Get_commandline() string {
	return self.Commandline
}

func (self *GCodeCommand) Get_command_parameters() map[string]string {
	return self.Params
}

func (self *GCodeCommand) Get_raw_command_parameters() string {
	return self.CommandParams.GetRawCommandParameters()
}

func (self *GCodeCommand) Ack(msg string) bool {
	if self.Need_ack == false {
		return false
	}
	var ok_msg = "ok"
	if msg != "" {
		ok_msg = fmt.Sprintf("ok %s", msg)
	}
	self.Respond_raw(ok_msg)
	self.Need_ack = false
	return true
}

func (self *GCodeCommand) String(name string, defaultValue string) string {
	return self.Get(name, defaultValue, nil, nil, nil, nil, nil)
}

func (self *GCodeCommand) Float(name string, defaultValue float64) float64 {
	return self.Get_float(name, defaultValue, nil, nil, nil, nil)
}

func (self *GCodeCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return self.Get_int(name, defaultValue, minValue, maxValue)
}

func (self *GCodeCommand) Parameters() map[string]string {
	return self.Get_command_parameters()
}

func (self *GCodeCommand) RawParameters() string {
	return self.Get_raw_command_parameters()
}

func (self *GCodeCommand) RespondInfo(msg string, log bool) {
	self.Respond_info(msg, log)
}

func (self *GCodeCommand) RespondRaw(msg string) {
	self.Respond_raw(msg)
}

// Parse and dispatch G-Code commands
type GCodeDispatch struct {
	Error                     *CommandError
	Coord                     []string
	Printer                   *Printer
	Is_fileinput              bool
	mutex                     *ReactorMutex
	Output_callbacks          []func(string)
	Base_gcode_handlers       map[string]interface{}
	Ready_gcode_handlers      map[string]interface{}
	Mux_commands              map[string]list.List
	Gcode_help                map[string]string
	Is_printer_ready          bool
	Gcode_handlers            map[string]interface{}
	Cmd_RESTART_help          string
	Cmd_FIRMWARE_RESTART_help string
	Cmd_STATUS_help           string
	Cmd_HELP_help             string
}

func NewGCodeDispatch(printer *Printer) *GCodeDispatch {
	var self = GCodeDispatch{}
	self.Printer = printer
	var is_check = printer.Get_start_args()["debuginput"]
	if is_check != "" {
		self.Is_fileinput = true
	} else {
		self.Is_fileinput = false
	}

	printer.Register_event_handler("project:ready", self.Handle_ready)
	printer.Register_event_handler("project:shutdown", self.Handle_shutdown)
	printer.Register_event_handler("project:disconnect",
		self.Handle_disconnect)
	// Command handling
	self.Is_printer_ready = false
	self.mutex = printer.Get_reactor().Mutex(false)
	self.Output_callbacks = []func(string){}
	self.Base_gcode_handlers = map[string]interface{}{}
	self.Gcode_handlers = self.Base_gcode_handlers
	self.Ready_gcode_handlers = map[string]interface{}{}
	self.Mux_commands = map[string]list.List{}
	self.Gcode_help = map[string]string{}
	// Register commands needed before config file is loaded
	var handlers = [...]string{"M110", "M115", "M117", "RESTART", "FIRMWARE_RESTART", "ECHO", "STATUS", "HELP"}
	self.Cmd_RESTART_help = "Reload config file and restart host software"
	self.Cmd_FIRMWARE_RESTART_help = "Restart firmware, host, and reload config"
	self.Cmd_STATUS_help = "Report the printer status"
	self.Cmd_HELP_help = "Report the list of available extended G-Code commands"

	for _, cmd := range handlers {
		var _func = reflects.GetMethod(&self, "Cmd_"+cmd)
		var desc = reflects.ReflectFieldValue(&self, "cmd_"+cmd+"_help")
		if desc == nil {
			desc = ""
		}

		self.Register_command(cmd, _func, true, desc.(string))
	}

	self.Coord = []string{"x", "y", "z", "e"}
	return &self
}

func (self *GCodeDispatch) Is_traditional_gcode(cmd string) bool {
	return gcodepkg.IsTraditionalGCode(cmd)
}

func (self *GCodeDispatch) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	self.Register_command(cmd, func(arg interface{}) error {
		return handler(arg.(*GCodeCommand))
	}, whenNotReady, desc)
}

func (self *GCodeDispatch) RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string) {
	self.Register_mux_command(cmd, key, value, func(arg interface{}) error {
		return handler(arg.(*GCodeCommand))
	}, desc)
}

func (self *GCodeDispatch) CreateCommand(cmd string, raw string, params map[string]string) printerpkg.Command {
	return self.Create_gcode_command(cmd, raw, params)
}

func (self *GCodeDispatch) IsTraditionalGCode(cmd string) bool {
	return self.Is_traditional_gcode(cmd)
}

func (self *GCodeDispatch) RunScriptFromCommand(script string) {
	self.Run_script_from_command(script)
}

func (self *GCodeDispatch) RunScript(script string) {
	self.Run_script(script)
}

func (self *GCodeDispatch) IsBusy() bool {
	return self.Get_mutex().Test()
}

func (self *GCodeDispatch) Mutex() printerpkg.Mutex {
	return &gcodeMutexAdapter{mutex: self.Get_mutex()}
}

func (self *GCodeDispatch) RespondInfo(msg string, log bool) {
	self.Respond_info(msg, log)
}

func (self *GCodeDispatch) RespondRaw(msg string) {
	self.Respond_raw(msg)
}

func (self *GCodeDispatch) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	oldRaw := self.Register_command(cmd, nil, whenNotReady, desc)
	var oldHandler func(printerpkg.Command) error
	if oldRaw != nil {
		oldHandler = func(command printerpkg.Command) error {
			return oldRaw.(func(interface{}) error)(command.(*GCodeCommand))
		}
	}
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return oldHandler
}

func (self *GCodeDispatch) Register_command(cmd string, _func interface{}, when_not_ready bool, desc string) interface{} {
	if _func == nil {
		var old_cmd = self.Ready_gcode_handlers[cmd]
		if self.Ready_gcode_handlers[cmd] != nil {
			delete(self.Ready_gcode_handlers, cmd)
		}
		if self.Base_gcode_handlers[cmd] != nil {
			delete(self.Base_gcode_handlers, cmd)
		}
		return old_cmd
	}
	_, ok := self.Ready_gcode_handlers[cmd]
	if ok {
		panic(fmt.Sprintf("gcode command %s already registered", cmd))
	}
	if self.Is_traditional_gcode(cmd) == false {
		if !gcodepkg.IsCommandValid(cmd) {
			panic(fmt.Errorf(
				"Can't register '%s' as it is an invalid name", cmd))
		}
		var origfunc = _func
		_func = func(params interface{}) error {
			paras := self.Get_extended_params(params.(*GCodeCommand))
			m := reflect.TypeOf(origfunc).Name()
			if m == "Value" {
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
	self.Ready_gcode_handlers[cmd] = _func
	if when_not_ready {
		self.Base_gcode_handlers[cmd] = _func
	}
	if desc != "" {
		self.Gcode_help[cmd] = desc
	}
	return nil
}

func (self *GCodeDispatch) Register_mux_command(cmd string, key string, value string, _func func(interface{}) error, desc string) {
	prev, ok := self.Mux_commands[cmd]
	if !ok && prev.Len() <= 0 {
		var handler = func(gcmd interface{}) error {
			return self.Cmd_mux(cmd, gcmd)
		}
		self.Register_command(cmd, handler, false, desc)
		prev.PushBack(key)
		prev.PushBack(map[string]interface{}{})
		self.Mux_commands[cmd] = prev
	}
	var prev_key, prev_values = prev.Front(), prev.Back()
	if prev_key.Value.(string) != key {
		panic(fmt.Sprintf(
			"mux command %s %s %s may have only one key (%#v)",
			cmd, key, value, prev_key))
	}
	if prev_values.Value.(map[string]interface{})[value] != nil {
		panic(fmt.Sprintf(
			"mux command %s %s %s already registered (%#v)",
			cmd, key, value, prev_values))
	}
	prev_values.Value.(map[string]interface{})[value] = _func
}

func (self *GCodeDispatch) Get_command_help() map[string]string {
	return self.Gcode_help
}

func (self *GCodeDispatch) Register_output_handler(cb func(string)) {
	self.Output_callbacks = append(self.Output_callbacks, cb)
}

func (self *GCodeDispatch) Handle_shutdown([]interface{}) error {
	if self.Is_printer_ready == false {
		return nil
	}
	self.Is_printer_ready = false
	self.Gcode_handlers = self.Base_gcode_handlers
	self.Respond_state("Shutdown")
	return nil
}

func (self *GCodeDispatch) Handle_disconnect([]interface{}) error {
	self.Respond_state("Disconnect")
	return nil
}

func (self *GCodeDispatch) Handle_ready([]interface{}) error {
	self.Is_printer_ready = true
	self.Gcode_handlers = self.Ready_gcode_handlers
	self.Respond_state("Ready")
	return nil
}

func (self *GCodeDispatch) parseGcode(input string) []string {
	return gcodepkg.ParseGcodeTokens(input)
}

func (self *GCodeDispatch) Process_commands(commands []string, need_ack bool) {
	for _, line := range commands {
		// Ignore comments and leading/trailing spaces
		line = strings.Trim(line, " ")
		origline := line
		if cpos := strings.Index(line, ";"); cpos != -1 {
			line = line[:cpos]
		}

		parts := self.parseGcode(strings.ToUpper(line))
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
		var gcmd = NewGCodeCommand(self, cmd, origline, params, need_ack)
		// Invoke handler for command
		var handler = self.Gcode_handlers[cmd]
		if handler == nil {
			handler = self.Cmd_default
		}
		m := reflect.TypeOf(handler).Name()
		if m == "Value" {
			argv := []reflect.Value{reflect.ValueOf(gcmd)}
			res := handler.(reflect.Value).Call(argv)
			if len(res) > 1 {
				logger.Debug(res)
			}
		} else {
			var _handler = handler.(func(interface{}) error)
			err := _handler(gcmd)
			if err != nil {
				logger.Error(err)
				msg := fmt.Sprintf("Internal error on command: %s \n", cmd)
				self.Printer.Invoke_shutdown(msg)
				self.Respond_error(msg)
				panic(err)
			}
		}
		gcmd.Ack("")
	}
}

func (self *GCodeDispatch) Run_script_from_command(script string) {
	self.Process_commands(strings.Split(script, "\n"), false)
}

func (self *GCodeDispatch) Run_script(script string) {
	if script == "CANCEL_PRINT" {
		self.Printer.Send_event("project:pre_cancel", nil)
	}
	self.mutex.Lock()
	defer func() {
		self.mutex.Unlock()
		if script == "CANCEL_PRINT" {
			self.Printer.Send_event("project:post_cancel", nil)
		}
	}()
	self.Process_commands(strings.Split(script, "\n"), false)
}

func (self *GCodeDispatch) Get_mutex() *ReactorMutex {
	return self.mutex
}

type gcodeMutexAdapter struct {
	mutex *ReactorMutex
}

func (self *gcodeMutexAdapter) Lock() {
	self.mutex.Lock()
}

func (self *gcodeMutexAdapter) Unlock() {
	self.mutex.Unlock()
}

func (self *GCodeDispatch) Create_gcode_command(command string, commandline string, params map[string]string) *GCodeCommand {
	return NewGCodeCommand(self, command, commandline, params, false)
}

// Response handling
func (self *GCodeDispatch) Respond_raw(msg string) {
	for _, cb := range self.Output_callbacks {
		cb(msg)
	}
}

func (self *GCodeDispatch) Respond_info(msg string, _log bool) {
	if _log {
		logger.Info(msg)
		var lines []string
		for _, l := range strings.Split(strings.TrimSpace(msg), "\n") {
			lines = append(lines, strings.TrimSpace(l))
		}
		self.Respond_raw(strings.Join(lines, "\n // "))
	} else {
		logger.Info(msg)
	}
}

func (self *GCodeDispatch) Respond_error(msg string) {
	//logging.warning(msg)
	var lines = strings.Split(strings.TrimSpace(msg), "\n")
	if len(lines) > 1 {
		self.Respond_info(fmt.Sprintf("%s\n", lines), true)
	}
	self.Respond_raw(fmt.Sprintf("!! %s", strings.TrimSpace(lines[0])))
	if self.Is_fileinput == true {
		self.Printer.Request_exit("error_exit")
	}
}

func (self *GCodeDispatch) Respond_state(state string) {
	self.Respond_info(fmt.Sprintf("project state: %s", state), true)
}

func (self *GCodeDispatch) Get_extended_params(gcmd *GCodeCommand) *GCodeCommand {
	gcmd.Params, _ = gcodepkg.ParseExtendedParams(gcmd.Get_raw_command_parameters())
	return gcmd
}

// G-Code special command handlers
func (self *GCodeDispatch) Cmd_default(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	var cmd = gcmd.Get_command()
	if cmd == "M105" {
		// Don"t warn about temperature requests when not ready
		gcmd.Ack("T:0")
		return nil
	}
	if cmd == "M21" {
		// Don"t warn about sd card init when not ready
		return nil
	}
	if self.Is_printer_ready == false {
		//panic(fmt.Sprintf(self.Is_fileinput.Get_state_message()[0]))
		return nil
	}

	if strings.Contains(cmd, "") {
		//# Handle M117/M118 gcode with numeric and special characters
		realcmd := strings.Split(cmd, " ")[0]
		if realcmd == "M117" || realcmd == "M118" || realcmd == "M23" {
			handler := self.Gcode_handlers[realcmd]
			if handler != nil {
				gcmd.Command = realcmd
				handler.(func(interface{}) error)(gcmd)
				return nil
			}
		}
	} else if cmd == "M140" || cmd == "M104" && gcmd.Get_float("S", 0., nil, nil, nil, nil) == -1 {
		// Don"t warn about requests to turn off heaters when not present
		return nil
	} else if cmd == "M107" || (cmd == "M106" && (gcmd.Get_float("S", 1., nil, nil, nil, nil) == -1 || self.Is_fileinput)) {
		// Don"t warn about requests to turn off fan when fan not present
		return nil
	}
	if cmd != "" {
		logger.Warn("Unknown command:", cmd)
	}
	return nil
}

func (self *GCodeDispatch) Cmd_mux(command string, argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	var cmd_list = self.Mux_commands[command]
	var key = cmd_list.Remove(cmd_list.Front()).(string)
	var values = cmd_list.Remove(cmd_list.Back())
	var key_param string
	if _, ok := values.(map[string]interface{})[""]; ok {
		key_param = gcmd.Get(key, nil, "", nil, nil, nil, nil)
	} else {
		key_param = gcmd.Get(key, object.Sentinel{}, "", nil, nil, nil, nil)
	}
	var vals = values.(map[string]interface{})
	if vals[key_param] == nil {
		panic(fmt.Sprintf("The value %s is not valid for %s", key_param, key))
	}
	return vals[key_param].(func(interface{}) error)(argv)
}

// Low-level G-Code commands that are needed before the config file is loaded
func (self *GCodeDispatch) Cmd_M110(argv interface{}) {
	logger.Debug("Cmd_M110")
	// Set Current Line Number
}

func (self *GCodeDispatch) Cmd_M112(argv interface{}) {
	// Emergency Stop
	self.Printer.Invoke_shutdown("Shutdown due to M112 command")
}

func (self *GCodeDispatch) Cmd_M115(argv interface{}) {
	gcmd := argv.(*GCodeCommand)
	// Get Firmware Version and Capabilities
	var software_version = self.Printer.Get_start_args()["software_version"]
	var kw = map[string]string{"FIRMWARE_NAME": "project", "FIRMWARE_VERSION": software_version.(string)}
	var msg string
	for k, v := range kw {
		msg += fmt.Sprintf("%s:%s ", k, v)
	}

	var did_ack = gcmd.Ack(msg)
	if did_ack == false {
		gcmd.Respond_info(msg, true)
	}
}

func (self *GCodeDispatch) Request_restart(result string) {
	if self.Is_printer_ready == true {
		toolhead := self.Printer.Lookup_object("toolhead", object.Sentinel{})
		var print_time = toolhead.(*Toolhead).Get_last_move_time()
		if result == "exit" {
			logger.Info("Exiting (print time %.3fs)", print_time)
		}
		self.Printer.Send_event("gcode:request_restart", []interface{}{print_time})
		toolhead.(*Toolhead).Dwell(0.500)
		toolhead.(*Toolhead).Wait_moves()
	}
	self.Printer.Request_exit(result)

}

func (self *GCodeDispatch) Cmd_RESTART(argv interface{}) {
	self.Request_restart("restart")
}

func (self *GCodeDispatch) Cmd_FIRMWARE_RESTART(argv interface{}) {
	self.Request_restart("firmware_restart")
}

func (self *GCodeDispatch) Cmd_ECHO(argv interface{}) {
	gcmd := argv.(*GCodeCommand)
	gcmd.Respond_info(gcmd.Get_commandline(), true)
}

func (self *GCodeDispatch) Cmd_STATUS(argv interface{}) {
	if self.Is_printer_ready == true {
		self.Respond_state("Ready")
		return
	}
	msg, _ := self.Printer.get_state_message()
	msg = strings.TrimRight(msg, "") + "\n state: Not ready"
	panic(msg)
}

func (self *GCodeDispatch) Cmd_HELP(argv interface{}) {
	gcmd := argv.(*GCodeCommand)
	var cmdhelp []string
	if self.Is_printer_ready == false {
		cmdhelp = append(cmdhelp, "Printer is not ready - not all commands available.")
	}
	cmdhelp = append(cmdhelp, "Available extended commands:")
	var ks []string
	for k, _ := range self.Gcode_handlers {
		ks = append(ks, k)
	}

	sort.Sort(sort.StringSlice(ks))
	for _, cmd := range ks {
		if self.Gcode_help[cmd] != "" {
			cmdhelp = append(cmdhelp, fmt.Sprintf("%-10s: %s", cmd, self.Gcode_help[cmd]))
		}
	}

	gcmd.Respond_info(strings.Join(cmdhelp, "\n"), true)
}

// Support reading gcode from a pseudo-tty interface
type GCodeIO struct {
	Printer            *Printer
	Gcode              *GCodeDispatch
	Fd                 interface{}
	Gcode_mutex        *ReactorMutex
	Reactor            IReactor
	Is_printer_ready   bool
	Is_processing_data bool
	Is_fileinput       bool
	Pipe_is_active     bool
	Fd_handle          *ReactorFileHandler
	Partial_input      string
	Pending_commands   []string
	Bytes_read         int
	Input_log          list.List
	M112_r             *regexp.Regexp
}

func NewGCodeIO(printer *Printer) *GCodeIO {
	var self = GCodeIO{}
	self.Printer = printer
	printer.Register_event_handler("project:ready", self.Handle_ready)
	printer.Register_event_handler("project:shutdown", self.Handle_shutdown)
	Gcode := printer.Lookup_object("gcode", object.Sentinel{})
	self.Gcode = Gcode.(*GCodeDispatch)
	self.Gcode_mutex = self.Gcode.Get_mutex()
	self.Fd = printer.Get_start_args()["gcode_fd"]
	self.Reactor = printer.Get_reactor()
	self.Is_printer_ready = false
	self.Is_processing_data = false
	var is_check = printer.Get_start_args()["debuginput"]
	if is_check != nil {
		self.Is_fileinput = true
	} else {
		self.Is_fileinput = false
	}
	self.Pipe_is_active = true
	self.Fd_handle = nil
	if self.Is_fileinput == false {
		self.Gcode.Register_output_handler(self.Respond_raw)
		self.Fd_handle = self.Reactor.Register_fd(self.Fd.(int),
			self.Process_data, nil)
	}
	self.Partial_input = ""
	self.Pending_commands = make([]string, 0)
	self.Bytes_read = 0
	self.Input_log = list.List{}
	return &self
}

func (self *GCodeIO) Handle_ready([]interface{}) error {
	self.Is_printer_ready = true
	if self.Is_fileinput && self.Fd_handle == nil {
		self.Fd_handle = self.Reactor.Register_fd(self.Fd.(int),
			self.Process_data, nil)
	}
	return nil
}

func (self *GCodeIO) Dump_debug() {
	var out []string
	out = append(out, fmt.Sprintf("Dumping gcode input %d blocks", self.Input_log.Len()))
	for i := 0; i < self.Input_log.Len(); i++ {
		var tmp_list = self.Input_log.Remove(self.Input_log.Front()).(list.List)
		var eventtime = tmp_list.Remove(tmp_list.Front()).(float64)
		var data = tmp_list.Remove(tmp_list.Front()).(string)
		out = append(out, fmt.Sprintf("Read %f: %s", eventtime, data))
	}
	for i := 0; i < len(out); i++ {
		logger.Debug(fmt.Sprintf("%s\n", out[i]))
	}
}

func (self *GCodeIO) Handle_shutdown([]interface{}) error {
	if self.Is_printer_ready == false {
		return nil
	}
	self.Is_printer_ready = false
	self.Dump_debug()
	if self.Is_fileinput == true {
		self.Printer.Request_exit("error_exit")
	}
	self.M112_r, _ = regexp.Compile("^(?:[nN][0-9]+)?\\s*[mM]112(?:\\s|$)")
	return nil
}

func (self *GCodeIO) Process_data(eventtime float64) interface{} {
	// Read input, separate by newline, and add to pending_commands
	// try:
	//var data = string(os.read(self.Fd.(int), 4096).decode())
	var data = ""
	// except (os.error, UnicodeDecodeError):
	// 	logging.exception("Read g-code")
	// 	return
	var tmp = list.New()
	tmp.PushBack(eventtime)
	tmp.PushBack(data)
	self.Input_log.PushBack(tmp)
	self.Bytes_read += len(data)
	var lines = strings.Split(data, "\n")
	lines[0] = self.Partial_input + lines[0]
	self.Partial_input = lines[len(lines)-1]
	var pending_commands = self.Pending_commands
	pending_commands = append(pending_commands, lines...)
	self.Pipe_is_active = true
	// Special handling for debug file input EOF
	if len(data) == 0 && self.Is_fileinput == true {
		if self.Is_processing_data == false {
			//self.Reactor.Unregister_fd(self.Fd_handle)
			self.Fd_handle = nil
			self.Gcode.Request_restart("exit")
		}
		pending_commands = append(pending_commands, "")
	}
	// Handle case where multiple commands pending
	if self.Is_processing_data || len(pending_commands) > 1 {
		if len(pending_commands) < 20 {
			// Check for M112 out-of-order
			for _, line := range lines {
				if self.M112_r.Match([]byte(line)) != false {
					self.Gcode.Cmd_M112(&GCodeCommand{})
				}
			}
		}
		if self.Is_processing_data == true {
			if len(pending_commands) >= 20 {
				// Stop reading input
				self.Reactor.Unregister_fd(self.Fd_handle)
				self.Fd_handle = nil
			}
			return nil
		}
	}
	// Process commands
	self.Is_processing_data = true
	for {
		self.Pending_commands = nil
		self.Gcode_mutex.Lock()
		self.Gcode.Process_commands(pending_commands, true)
		self.Gcode_mutex.Unlock()
		pending_commands = self.Pending_commands
		if pending_commands == nil {
			break
		}
	}
	self.Is_processing_data = false
	if self.Fd_handle != nil {
		self.Fd_handle = self.Reactor.Register_fd(self.Fd.(int),
			self.Process_data, nil)
	}
	return nil
}

func (self *GCodeIO) Respond_raw(msg string) {
	if self.Pipe_is_active == true {
		// try:
		//os.write(self.Fd.(int), (msg + "\n").encode())
		// except os.error:
		// 	logging.exception("Write g-code response")
		// 	self.Pipe_is_active = false
	}
}

func (self *GCodeIO) Stats(eventtime float64) (bool, string) {
	return true, fmt.Sprintf("gcodein=%d", self.Bytes_read)
}

func Add_early_printer_objects1(printer *Printer) {
	printer.Add_object("gcode", NewGCodeDispatch(printer))
	//printer.Add_object("gcode_io", NewGCodeIO(printer))
}
