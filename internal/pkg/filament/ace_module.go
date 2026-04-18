package filament

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	addonpkg "goklipper/internal/addon"
	iopkg "goklipper/internal/pkg/io"
	printerpkg "goklipper/internal/pkg/printer"
	reactorpkg "goklipper/internal/pkg/reactor"
	printpkg "goklipper/internal/print"
)

const (
	RECONNECT_COUNT = 10
)

type aceFilamentSensor interface {
	FilamentPresent() bool
}

type aceConfig interface {
	printerpkg.ModuleConfig
	Get(option string, defaultValue interface{}, noteValid bool) interface{}
	Getint(option string, defaultValue interface{}, minval, maxval int, noteValid bool) int
	Getboolean(option string, defaultValue interface{}, noteValid bool) bool
}

type aceLegacyPrinter interface {
	printerpkg.ModulePrinter
	Get_reactor() reactorpkg.IReactor
}

type aceToolhead interface {
	Get_last_move_time() float64
	Get_position() []float64
	Move(newpos []float64, speed float64)
	Get_status(eventtime float64) map[string]interface{}
}

type aceCommand interface {
	printerpkg.Command
	Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	Get_int(name string, _default interface{}, minval *int, maxval *int) int
}

type aceEndstop interface {
	Query_endstop(printTime float64) int
}

type acePinRegistry interface {
	Parse_pin(pinDesc string, canInvert bool, canPullup bool) map[string]interface{}
	Allow_multi_use_pin(pinDesc string)
	Setup_pin(pinType, pinDesc string) interface{}
}

type aceQueryEndstops interface {
	Register_endstop(endstop interface{}, name string)
}

type aceSwitchSensorConfig struct {
	parent      printerpkg.ModuleConfig
	section     string
	stringVals  map[string]string
	boolVals    map[string]bool
	presentOpts map[string]bool
}

func (self *aceSwitchSensorConfig) Name() string {
	return self.section
}

func (self *aceSwitchSensorConfig) String(option string, defaultValue string, noteValid bool) string {
	_ = noteValid
	if value, ok := self.stringVals[option]; ok {
		return value
	}
	return defaultValue
}

func (self *aceSwitchSensorConfig) Bool(option string, defaultValue bool) bool {
	if value, ok := self.boolVals[option]; ok {
		return value
	}
	return defaultValue
}

func (self *aceSwitchSensorConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}

func (self *aceSwitchSensorConfig) OptionalFloat(option string) *float64 {
	_ = option
	return nil
}

func (self *aceSwitchSensorConfig) LoadObject(section string) interface{} {
	return self.parent.LoadObject(section)
}

func (self *aceSwitchSensorConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return self.parent.LoadTemplate(module, option, defaultValue)
}

func (self *aceSwitchSensorConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return self.parent.LoadRequiredTemplate(module, option)
}

func (self *aceSwitchSensorConfig) Printer() printerpkg.ModulePrinter {
	return self.parent.Printer()
}

func (self *aceSwitchSensorConfig) HasOption(option string) bool {
	return self.presentOpts[option]
}

func requireACEConfig(config printerpkg.ModuleConfig) aceConfig {
	legacy, ok := config.(aceConfig)
	if !ok {
		panic(fmt.Sprintf("ACE config does not implement legacy getters: %T", config))
	}
	return legacy
}

func requireACEPrinter(printer printerpkg.ModulePrinter) aceLegacyPrinter {
	legacy, ok := printer.(aceLegacyPrinter)
	if !ok {
		panic(fmt.Sprintf("printer does not implement ACE legacy printer interface: %T", printer))
	}
	return legacy
}

func requireACEToolhead(printer printerpkg.ModulePrinter) aceToolhead {
	toolheadObj := printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(aceToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement aceToolhead: %T", toolheadObj))
	}
	return toolhead
}

func requireACECommand(gcmd printerpkg.Command) aceCommand {
	cmd, ok := gcmd.(aceCommand)
	if !ok {
		panic(fmt.Sprintf("ACE command does not implement legacy getters: %T", gcmd))
	}
	return cmd
}

type ACE struct {
	printer  printerpkg.ModulePrinter
	reactor  reactorpkg.IReactor
	gcode    printerpkg.GCodeRuntime
	toolhead aceToolhead

	ace_commun *AceCommun
	ace_dev_fd *reactorpkg.ReactorFileHandler

	connect_timer  *reactorpkg.ReactorTimer
	heatbeat_timer *reactorpkg.ReactorTimer

	endstops map[string]interface{}

	state *ACERuntimeState

	endless_spool_timer     *reactorpkg.ReactorTimer
	change_tool_in_progress bool

	feed_speed                       int
	retract_speed                    int
	toolchange_retract_length        int
	toolchange_load_length           int
	toolhead_sensor_to_nozzle_length int

	max_dryer_temperature int
	reconneted_count      int
}

func NewACE(config printerpkg.ModuleConfig) *ACE {
	cfg := requireACEConfig(config)
	printer := cfg.Printer()
	self := &ACE{}
	self.printer = printer
	self.reactor = requireACEPrinter(printer).Get_reactor()
	self.gcode = printer.GCode()

	serial := cfg.Get("serial", "/dev/ttyACM0", true).(string)
	if serial == "" {
		matches, _ := filepath.Glob("/dev/ttyACM*")
		if len(matches) > 0 {
			serial = matches[0]
		} else {
			serial = "/dev/ttyACM0"
		}
	}
	baud := cfg.Getint("v2_baud", 230400, 0, 0, true)
	self.ace_commun = NewAceCommunication(serial, baud)

	_ = cfg.Get("switch_pin", "", true).(string)
	self.feed_speed = cfg.Getint("feed_speed", 50, 0, 0, true)
	self.retract_speed = cfg.Getint("retract_speed", 50, 0, 0, true)
	self.toolchange_retract_length = cfg.Getint("toolchange_retract_length", 150, 0, 0, true)
	self.toolchange_load_length = cfg.Getint("toolchange_load_length", 630, 0, 0, true)
	self.toolhead_sensor_to_nozzle_length = cfg.Getint("toolhead_sensor_to_nozzle", 0, 0, 0, true)
	self.max_dryer_temperature = cfg.Getint("max_dryer_temperature", 55, 0, 0, true)

	vObj := self.printer.LookupObject("save_variables", nil)
	var variables map[string]interface{}
	if vObj != nil {
		sv := vObj.(*addonpkg.SaveVariablesModule)
		if sv != nil {
			variables = sv.Variables()
		}
	}
	self.state = NewACERuntimeState(
		variables,
		cfg.Getboolean("endless_spool", SavedEndlessSpoolEnabled(variables), true),
		AmsConfigPath,
	)

	self.change_tool_in_progress = false
	self.endstops = map[string]interface{}{}
	self.bootstrapFilamentSensor(cfg, "extruder_sensor_pin", "extruder_sensor")

	self.printer.RegisterEventHandler("project:ready", self._handle_ready)
	self.printer.RegisterEventHandler("project:disconnect", self._handle_disconnect)

	self.gcode.RegisterCommand(
		"ACE_SET_SLOT", self.cmd_ACE_SET_SLOT, true,
		"Set slot inventory: INDEX= COLOR= MATERIAL= TEMP= | Set status to empty with EMPTY=1")
	self.gcode.RegisterCommand(
		"ACE_QUERY_SLOTS", self.cmd_ACE_QUERY_SLOTS, true,
		"Query all slot inventory as JSON")
	self.gcode.RegisterCommand(
		"ACE_DEBUG", self.cmd_ACE_DEBUG, true,
		"self.cmd_ACE_DEBUG_help")
	self.gcode.RegisterCommand(
		"ACE_START_DRYING", self.cmd_ACE_START_DRYING, true,
		"Starts ACE Pro dryer")
	self.gcode.RegisterCommand(
		"ACE_STOP_DRYING", self.cmd_ACE_STOP_DRYING, true,
		"Stops ACE Pro dryer")
	self.gcode.RegisterCommand(
		"ACE_ENABLE_FEED_ASSIST", self.cmd_ACE_ENABLE_FEED_ASSIST, true,
		"Enables ACE feed assist")
	self.gcode.RegisterCommand(
		"ACE_DISABLE_FEED_ASSIST", self.cmd_ACE_DISABLE_FEED_ASSIST, true,
		"Disables ACE feed assist")
	self.gcode.RegisterCommand(
		"ACE_FEED", self.cmd_ACE_FEED, true,
		"Feeds filament from ACE")
	self.gcode.RegisterCommand(
		"ACE_RETRACT", self.cmd_ACE_RETRACT, true,
		"Retracts filament back to ACE")
	self.gcode.RegisterCommand(
		"ACE_CHANGE_TOOL", self.cmd_ACE_CHANGE_TOOL, true,
		"Changes tool")
	self.gcode.RegisterCommand(
		"ACE_ENABLE_ENDLESS_SPOOL", self.cmd_ACE_ENABLE_ENDLESS_SPOOL, true,
		"Enable endless spool feature")
	self.gcode.RegisterCommand(
		"ACE_DISABLE_ENDLESS_SPOOL", self.cmd_ACE_DISABLE_ENDLESS_SPOOL, true,
		"Disable endless spool feature")
	self.gcode.RegisterCommand(
		"ACE_ENDLESS_SPOOL_STATUS", self.cmd_ACE_ENDLESS_SPOOL_STATUS, true,
		"Show endless spool status")
	self.gcode.RegisterCommand(
		"ACE_SAVE_INVENTORY", self.cmd_ACE_SAVE_INVENTORY, true,
		"Manually save current inventory to persistent storage")
	self.gcode.RegisterCommand(
		"ACE_TEST_RUNOUT_SENSOR", self.cmd_ACE_TEST_RUNOUT_SENSOR, true,
		"Test and display runout sensor states")
	self.gcode.RegisterCommand("FEED_FILAMENT", self.cmd_FEED_FILAMENT, true, "Native UI Feed Filament")
	self.gcode.RegisterCommand("UNWIND_FILAMENT", self.cmd_UNWIND_FILAMENT, true, "Native UI Unwind Filament")
	self.heatbeat_timer = self.reactor.Register_timer(self._periodic_heartbeat_event, constants.NEVER)
	return self
}

func (self *ACE) _handle_ready([]interface{}) error {
	self.toolhead = requireACEToolhead(self.printer)
	logger.Debug("ACE: Connecting to", self.ace_commun.Name())
	self.connect_timer = self.reactor.Register_timer(self._connect, constants.NOW)
	return nil
}

func (self *ACE) _handle_disconnect([]interface{}) error {
	logger.Debugf("ACE: Closing connection", self.ace_commun.Name())
	self._disconnect()
	return nil
}

func (self *ACE) lookupFilamentSensor(name string) aceFilamentSensor {
	section := fmt.Sprintf("filament_switch_sensor %s", name)
	obj := self.printer.LookupObject(section, nil)
	sensor, ok := obj.(aceFilamentSensor)
	if !ok {
		return nil
	}
	return sensor
}

func (self *ACE) lookupRequiredFilamentSensor(name string) aceFilamentSensor {
	sensor := self.lookupFilamentSensor(name)
	if sensor == nil {
		panic(fmt.Sprintf("filament sensor %s not found", name))
	}
	return sensor
}

func (self *ACE) bootstrapFilamentSensor(config printerpkg.ModuleConfig, pinOption string, name string) {
	typedConfig, ok := config.(interface {
		Get(option string, defaultValue interface{}, noteValid bool) interface{}
	})
	if !ok {
		return
	}
	pin, _ := typedConfig.Get(pinOption, "", true).(string)
	pin = strings.TrimSpace(pin)
	if pin == "" {
		return
	}
	if self.lookupFilamentSensor(name) == nil {
		section := fmt.Sprintf("filament_switch_sensor %s", name)
		sensorConfig := &aceSwitchSensorConfig{
			parent:  config,
			section: section,
			stringVals: map[string]string{
				"switch_pin": pin,
			},
			boolVals: map[string]bool{
				"pause_on_runout": false,
			},
			presentOpts: map[string]bool{
				"switch_pin":      true,
				"pause_on_runout": true,
			},
		}
		if err := self.printer.AddObject(section, iopkg.LoadConfigSwitchSensor(sensorConfig)); err != nil {
			panic(err)
		}
	}
	pinsObj := self.printer.LookupObject("pins", nil)
	pins, ok := pinsObj.(acePinRegistry)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement acePinRegistry: %T", pinsObj))
	}
	pinParams := pins.Parse_pin(pin, true, true)
	shareName := fmt.Sprintf("%s:%s", pinParams["chip_name"], pinParams["pin"])
	pins.Allow_multi_use_pin(shareName)
	mcuEndstop := pins.Setup_pin("endstop", pin)
	queryEndstopsObj := config.LoadObject("query_endstops")
	queryEndstops, ok := queryEndstopsObj.(aceQueryEndstops)
	if !ok {
		panic(fmt.Sprintf("query_endstops object does not implement aceQueryEndstops: %T", queryEndstopsObj))
	}
	queryEndstops.Register_endstop(mcuEndstop, shareName)
	self.endstops[name] = mcuEndstop
}

func (self *ACE) _connect(eventtime float64) float64 {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(err)
			self._disconnect()
		}
	}()

	self.gcode.RespondInfo("ACE: Try connecting ACE", true)
	err := self.ace_commun.Connect()
	if err != nil {
		logger.Warnf("Unable to open ace_commun port %s: %s", self.ace_commun.Name(), err)
		if self.reconneted_count <= RECONNECT_COUNT {
			self.reconneted_count++
			delay := CalcReconnectTimeout(self.reconneted_count)
			self.gcode.RespondInfo(fmt.Sprintf("ACE: Will auto reconnect after %f S ", delay), true)
			return eventtime + delay
		}
		self.gcode.RespondInfo("ACE: Reconnection exceeded the number of times, timeout exceeded 10 seconds.", true)
		return eventtime + 10.0
	}
	logger.Infof("ACE: Connected to %s", self.ace_commun.Name())
	self.gcode.RespondInfo(fmt.Sprintf("ACE: Connected to %s ", self.ace_commun.Name()), true)
	self.ace_dev_fd = self.reactor.Register_fd(self.ace_commun.Fd(), self.read_handle, self.write_handle)

	self.send_request(map[string]interface{}{"method": "get_info"},
		func(response map[string]interface{}) {
			message, useV2 := self.state.ApplyFirmwareInfoResult(response["result"])
			self.ace_commun.IsV2 = useV2
			if message != "" {
				self.gcode.RespondInfo(message, true)
			}
		})

	if self.heatbeat_timer != nil {
		self.reactor.Update_timer(self.heatbeat_timer, constants.NOW)
	}

	if self.state.EndlessSpoolEnabled {
		self.endless_spool_timer = self.reactor.Register_timer(self._endless_spool_monitor, eventtime+1.0)
	}

	ace_current_index := self.state.CurrentIndex()
	if ace_current_index != -1 {
		self.gcode.RespondInfo(fmt.Sprintf("ACE: Re-enabling feed assist on reconnect for index {%d}", ace_current_index), true)
		self.set_feed_assist(ace_current_index, true)
	}
	self.reconneted_count = 0
	return constants.NEVER
}

func (self *ACE) _disconnect() {
	logger.Debug("ACE: Disconnet...")
	self.gcode.RespondInfo("ACE: Disconnet...", true)

	if self.heatbeat_timer != nil {
		self.reactor.Update_timer(self.heatbeat_timer, constants.NEVER)
	}

	if self.endless_spool_timer != nil {
		self.reactor.Unregister_timer(self.endless_spool_timer)
		self.endless_spool_timer = nil
	}

	if self.ace_dev_fd != nil {
		self.reactor.Set_fd_wake(self.ace_dev_fd, false, false)
		self.reactor.Unregister_fd(self.ace_dev_fd)
		self.ace_dev_fd = nil
	}
	if self.ace_commun != nil {
		self.ace_commun.Disconnect()
	}
}

func (self *ACE) write_handle(eventtime float64) interface{} {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(eventtime, err)
		}
	}()
	if self.ace_commun != nil && self.ace_dev_fd != nil {
		self.ace_commun.Writer(eventtime)
		if self.ace_commun.Is_send_queue_empty() {
			self.reactor.Set_fd_wake(self.ace_dev_fd, true, false)
		}
	}
	return nil
}

func (self *ACE) read_handle(eventtime float64) interface{} {
	err := self.ace_commun.Reader(eventtime)
	if err != nil {
		logger.Error(err)
		self._disconnect()
		if strings.Contains(err.Error(), RespondTimeoutError) ||
			strings.Contains(err.Error(), UnableToCommunError) {
			if self.reconneted_count <= RECONNECT_COUNT {
				self.reconneted_count++
				delay := CalcReconnectTimeout(self.reconneted_count)
				self.gcode.RespondInfo(fmt.Sprintf("ACE: Will auto reconnect after %f S ", delay), true)
				self.reactor.Update_timer(self.connect_timer, eventtime+delay)
			}
		}
	}
	return nil
}

func (self *ACE) _periodic_heartbeat_event(eventtime float64) float64 {
	self.send_request(map[string]interface{}{"method": "get_status"},
		func(response map[string]interface{}) {
			res, ok := response["result"].(map[string]interface{})
			if !ok {
				return
			}
			self.state.SyncStatusResult(res)
		})
	return eventtime + 1.5
}

func (self *ACE) dwell(delay float64) {
	currTs := self.reactor.Monotonic()
	self.reactor.Pause(currTs + delay)
}

func (self *ACE) send_request(request map[string]interface{}, callback func(map[string]interface{})) {
	self.ace_commun.Push_send_queue(request, callback)
	if self.ace_dev_fd != nil {
		self.reactor.Set_fd_wake(self.ace_dev_fd, true, true)
	}
}

func (self *ACE) run_ace_command(request map[string]interface{}, delay float64, onSuccess func(), successMessage string) {
	self.send_request(request, func(response map[string]interface{}) {
		if err := ACECommandError(response); err != nil {
			panic(err)
		}
		if onSuccess != nil {
			onSuccess()
		}
		if successMessage != "" {
			self.gcode.RespondInfo(successMessage, true)
		}
	})
	if delay > 0 {
		self.dwell(delay)
	}
}

func (self *ACE) run_command_plan(plan ACECommandPlan, onSuccess func()) {
	self.run_ace_command(plan.Request, plan.Delay, onSuccess, plan.SuccessMessage)
}

func (self *ACE) set_feed_assist(index int, enable bool) {
	plan, err := BuildFeedAssistPlan(index, enable)
	if err != nil {
		panic(err)
	}
	self.run_command_plan(plan.ACECommandPlan, func() {
		self.state.FeedAssistIndex = plan.ResultIndex
	})
}

func (self *ACE) _check_endstop_state(name string) bool {
	printTime := self.toolhead.Get_last_move_time()
	if endstop, ok := self.endstops[name].(aceEndstop); ok {
		return endstop.Query_endstop(printTime) > 0
	}
	return false
}

func (self *ACE) wait_ace_ready() {
	for {
		if status, ok := self.state.Info["status"].(string); ok && status == "ready" {
			break
		}
		self.dwell(0.5)
	}
}

func (self *ACE) _extruder_move(length, speed float64) float64 {
	pos := self.toolhead.Get_position()
	pos[3] += length
	self.toolhead.Move(pos, speed)
	return pos[3]
}

func (self *ACE) _endless_spool_monitor(eventtime float64) (next float64) {
	if !self.state.EndlessSpoolEnabled {
		return eventtime + 1.0
	}
	if self.change_tool_in_progress || self.state.EndlessSpoolInProgress {
		return eventtime + 0.2
	}
	current_tool := self.state.CurrentIndex()
	if current_tool == -1 {
		return eventtime + 1.0
	}
	next = eventtime + 0.2
	is_printing := false
	defer func() {
		if err := recover(); err != nil {
			is_printing = false
			logger.Error(fmt.Errorf("ACE: Endless spool monitor error: %v", err))
			next = eventtime + 0.2
		}
		print_stats := self.printer.LookupObject("print_stats", nil).(*printpkg.PrintStatsModule)
		is_printing = false

		if print_stats != nil {
			stats := print_stats.Get_status(eventtime)
			if stats["state"] == "printing" {
				is_printing = true
			}
		}

		printer_idle := self.printer.LookupObject("idle_timeout", nil).(*printpkg.IdleTimeoutModule)
		idle_state := printer_idle.Get_status(eventtime)["state"]
		if idle_state == "Printing" {
			is_printing = true
		}
		if is_printing && current_tool >= 0 {
			self._endless_spool_runout_handler()
		}

		if is_printing {
			next = eventtime + 0.05
		} else {
			next = eventtime + 1.0
		}
	}()
	return
}

func (self *ACE) cmd_ACE_START_DRYING(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)
	temperature := cmd.Get_int("TEMP", 0, nil, nil)
	duration := cmd.Get_int("DURATION", 240, nil, nil)
	plan, err := BuildDryingPlan(temperature, duration, self.max_dryer_temperature)
	if err != nil {
		panic(err)
	}

	self.gcode.RespondInfo("ACE: Started ACE drying", true)
	self.run_command_plan(plan, nil)
	return nil
}

func (self *ACE) cmd_ACE_STOP_DRYING(printerpkg.Command) error {
	self.gcode.RespondInfo("ACE: Stopped ACE drying", true)
	self.run_command_plan(BuildStopDryingPlan(), nil)
	return nil
}

func (self *ACE) cmd_ACE_ENABLE_FEED_ASSIST(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)
	index := cmd.Get_int("INDEX", -1, nil, nil)
	self.set_feed_assist(index, true)
	return nil
}

func (self *ACE) cmd_ACE_DISABLE_FEED_ASSIST(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)
	defaultIndex := -1
	if self.state.FeedAssistIndex != -1 {
		defaultIndex = self.state.FeedAssistIndex
	}
	index := cmd.Get_int("INDEX", defaultIndex, nil, nil)
	self.set_feed_assist(index, false)
	return nil
}

func (self *ACE) cmd_ACE_FEED(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)
	index := cmd.Get_int("INDEX", -1, nil, nil)
	length := cmd.Get_int("LENGTH", -1, nil, nil)
	speed := cmd.Get_int("SPEED", self.feed_speed, nil, nil)
	plan, err := BuildStrictFeedPlan(index, length, speed)
	if err != nil {
		panic(err)
	}
	self.run_command_plan(plan, nil)
	return nil
}

func (self *ACE) cmd_FEED_FILAMENT(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)
	index := cmd.Get_int("INDEX", 0, nil, nil)
	length := cmd.Get_int("LENGTH", self.toolchange_load_length, nil, nil)
	speed := cmd.Get_int("SPEED", self.feed_speed, nil, nil)
	plan := BuildUIFeedCommandPlan(index, length, speed, 100, 50)
	self.gcode.RespondInfo(fmt.Sprintf("ACE UI FEED idx=%d", plan.EffectiveIndex), true)
	self.run_command_plan(plan.ACECommandPlan, nil)
	return nil
}

func (self *ACE) cmd_ACE_RETRACT(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)
	index := cmd.Get_int("INDEX", -1, nil, nil)
	length := cmd.Get_int("LENGTH", -1, nil, nil)
	speed := cmd.Get_int("SPEED", self.retract_speed, nil, nil)
	plan, err := BuildStrictRetractPlan(index, length, speed)
	if err != nil {
		panic(err)
	}
	self.run_command_plan(plan, nil)
	return nil
}

func (self *ACE) cmd_UNWIND_FILAMENT(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)
	index := cmd.Get_int("INDEX", -1, nil, nil)
	length := cmd.Get_int("LENGTH", self.toolchange_retract_length, nil, nil)
	speed := cmd.Get_int("SPEED", self.feed_speed, nil, nil)
	plan := BuildUIUnwindCommandPlan(index, self.state.FeedAssistIndex, length, speed, 100, 50)
	self.gcode.RespondInfo(fmt.Sprintf("ACE UI UNWIND idx=%d", plan.EffectiveIndex), true)
	self.run_command_plan(plan.ACECommandPlan, nil)
	return nil
}

func (self *ACE) _feed_to_toolhead(tool int) error {
	sensor_extruder := self.lookupRequiredFilamentSensor("extruder_sensor")

	for {
		if sensor_extruder.FilamentPresent() {
			break
		}
		self.wait_ace_ready()
		if self.state.Info["slots"].([]interface{})[tool].(map[string]interface{})["status"] == "ready" {
			self.run_ace_command(
				BuildFeedRequest(tool, self.toolchange_load_length, self.retract_speed),
				MotionCommandDelay(self.toolchange_load_length, self.retract_speed),
				nil,
				"")
			self.state.Variables["ace_filament_pos"] = "bowden"
			self.dwell(0.1)
		} else {
			logger.Info("Spool is empty")
			printer_idle := self.printer.LookupObject("idle_timeout", nil).(*printpkg.IdleTimeoutModule)
			idle_state := printer_idle.Get_status(self.reactor.Monotonic())["state"]
			if idle_state == "Printing" {
				self.gcode.RunScriptFromCommand("PAUSE")
			}
			return fmt.Errorf("spool is empty")
		}
	}

	if !sensor_extruder.FilamentPresent() {
		panic(fmt.Errorf("Filament stuck %v", sensor_extruder.FilamentPresent()))
	} else {
		self.state.Variables["ace_filament_pos"] = "spliter"
	}

	self.set_feed_assist(tool, true)

	self.state.Variables["ace_filament_pos"] = "toolhead"

	self._extruder_move(float64(self.toolhead_sensor_to_nozzle_length), 5)
	self.state.Variables["ace_filament_pos"] = "nozzle"
	self.gcode.RunScriptFromCommand("MOVE_THROW_POS")

	return nil
}

func (self *ACE) cmd_ACE_CHANGE_TOOL(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)

	tool := cmd.Get_int("TOOL", -1, nil, nil)
	sensor_extruder := self.lookupRequiredFilamentSensor("extruder_sensor")

	if tool < 0 || tool >= 4 {
		panic("Wrong index")
	}

	was := self.state.CurrentIndex()
	if was == tool {
		gcmd.RespondInfo(fmt.Sprintf("ACE: Already tool %d", tool), true)
		return nil
	}

	if tool != -1 {
		status := self.state.Info["slots"].([]interface{})[tool].(map[string]interface{})["status"]
		if status != "ready" {
			gcmd.RespondInfo("ACE: Spool is empty", true)
			printer_idle := self.printer.LookupObject("idle_timeout", nil).(*printpkg.IdleTimeoutModule)
			idle_state := printer_idle.Get_status(self.reactor.Monotonic())["state"]
			if idle_state == "Printing" {
				self.gcode.RunScriptFromCommand("PAUSE")
			}
			return nil
		}
	}

	endless_spool_was_enabled := self.state.BeginManualToolchange()
	self.change_tool_in_progress = true
	self.gcode.RunScriptFromCommand(fmt.Sprintf("_ACE_PRE_TOOLCHANGE FROM=%d TO=%d", was, tool))

	logger.Infof(fmt.Sprintf("ACE: Toolchange %d => %d", was, tool))
	var err error
	if was != -1 {
		self.set_feed_assist(was, false)
		self.wait_ace_ready()
		ace_filament_pos := self.state.Variables["ace_filament_pos"]
		if ace_filament_pos == nil {
			ace_filament_pos = "spliter"
		}

		if ace_filament_pos.(string) == "nozzle" {
			self.gcode.RunScriptFromCommand("CUT_TIP")
			self.state.Variables["ace_filament_pos"] = "toolhead"
		}

		if ace_filament_pos.(string) == "toolhead" {
			for {
				if sensor_extruder.FilamentPresent() {
					break
				}
				self._extruder_move(-50, 10)
				self.run_ace_command(
					BuildRetractRequest(was, 100, self.retract_speed),
					MotionCommandDelay(100, self.retract_speed),
					nil,
					"")
				self.wait_ace_ready()
			}
			self.state.Variables["ace_filament_pos"] = "bowden"
		}
		self.wait_ace_ready()

		self.run_ace_command(
			BuildRetractRequest(was, self.toolchange_retract_length, self.retract_speed),
			MotionCommandDelay(self.toolchange_retract_length, self.retract_speed),
			nil,
			"")
		self.wait_ace_ready()
		self.state.Variables["ace_filament_pos"] = "spliter"

		if tool != -1 {
			err = self._feed_to_toolhead(tool)
		}
	} else {
		err = self._feed_to_toolhead(tool)
	}

	if err != nil {
		self.change_tool_in_progress = false
		self.state.RestoreManualToolchange(endless_spool_was_enabled)
		logger.Error(err)
		return nil
	}

	self.printer.GCodeMove().ResetLastPosition()
	self.gcode.RunScriptFromCommand(fmt.Sprintf("_ACE_POST_TOOLCHANGE FROM=%d TO=%d", was, tool))

	self.state.SetCurrentIndex(tool)
	self.printer.GCodeMove().ResetLastPosition()
	self.gcode.RunScriptFromCommand(fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_current_index VALUE=%d", tool))
	self.gcode.RunScriptFromCommand(
		fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_filament_pos VALUE=%s", self.state.Variables["ace_filament_pos"]))
	self.change_tool_in_progress = false

	self.state.RestoreManualToolchange(endless_spool_was_enabled)

	gcmd.RespondInfo(fmt.Sprintf("ACE: Tool {%d} load", tool), true)
	return nil
}

func (self *ACE) _endless_spool_runout_handler() {
	defer func() {
		if err := recover(); err != nil {
			logger.Errorf("ACE: Runout detection error: %v", err)
		}
	}()

	if !self.state.EndlessSpoolEnabled || self.state.EndlessSpoolInProgress {
		return
	}

	current_tool := self.state.CurrentIndex()
	if current_tool == -1 {
		return
	}
	sensor_extruder := self.lookupFilamentSensor("extruder_sensor")

	if sensor_extruder != nil {
		runout_helper_present := sensor_extruder.FilamentPresent()
		endstop_triggered := self._check_endstop_state("extruder_sensor")
		if self.state.MarkRunoutIfTriggered(runout_helper_present, endstop_triggered) {
			self.gcode.RespondInfo("ACE: Endless spool runout detected, switching immediately", true)
			logger.Debugf("ACE: Runout detected - helper=%v, endstop=%v", runout_helper_present, endstop_triggered)
			self._execute_endless_spool_change()
		}
	}
}

func (self *ACE) _execute_endless_spool_change() {
	defer func() {
		if err := recover(); err != nil {
			self.gcode.RespondInfo(fmt.Sprintf("ACE: Endless spool change failed: {%v}", err), true)
			self.gcode.RunScriptFromCommand("PAUSE")
			self.state.AbortEndlessSpoolChange()
		}
	}()
	if !self.state.BeginEndlessSpoolChange() {
		return
	}

	change, err := self.state.PrepareEndlessSpoolChangePlan()
	if err != nil {
		panic(err)
	}

	if change.NextTool == -1 {
		self.gcode.RespondInfo("ACE: No available slots for endless spool, pausing print", true)
		self.gcode.RunScriptFromCommand("PAUSE")
		self.state.EndlessSpoolRunoutDetected = false
		self.state.AbortEndlessSpoolChange()
		return
	}

	self.gcode.RespondInfo(fmt.Sprintf("ACE: Endless spool changing from slot %d to slot %d", change.CurrentTool, change.NextTool), true)

	if change.InventoryChanged {
		self.gcode.RunScriptFromCommand(
			fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_inventory VALUE=%s", change.InventorySaveValue))
	}

	if change.CurrentTool != -1 {
		self.set_feed_assist(change.CurrentTool, false)
		self.wait_ace_ready()
	}

	sensor_extruder := self.lookupRequiredFilamentSensor("extruder_sensor")

	for {
		if !sensor_extruder.FilamentPresent() {
			break
		}
		self.run_ace_command(
			BuildFeedRequest(change.NextTool, self.toolchange_load_length, self.retract_speed),
			MotionCommandDelay(self.toolchange_load_length, self.retract_speed),
			nil,
			"")
		self.wait_ace_ready()
		self.dwell(0.1)
	}

	if !sensor_extruder.FilamentPresent() {
		panic("Filament stuck during endless spool change")
	}

	self.set_feed_assist(change.NextTool, true)

	self.state.CompleteEndlessSpoolChange(change.NextTool)
	self.gcode.RunScriptFromCommand(fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_current_index VALUE=%d", change.NextTool))

	self.gcode.RespondInfo(fmt.Sprintf("ACE: Endless spool completed, now using slot {%d}", change.NextTool), true)
}

func (self *ACE) cmd_ACE_ENABLE_ENDLESS_SPOOL(gcmd printerpkg.Command) error {
	saveValue, response := self.state.ToggleEndlessSpool(true)
	self.gcode.RunScriptFromCommand(
		fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_endless_spool_enabled VALUE=%s", saveValue))
	gcmd.RespondInfo(response, true)
	return nil
}

func (self *ACE) cmd_ACE_DISABLE_ENDLESS_SPOOL(gcmd printerpkg.Command) error {
	saveValue, response := self.state.ToggleEndlessSpool(false)
	self.gcode.RunScriptFromCommand(
		fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_endless_spool_enabled VALUE=%s", saveValue))
	gcmd.RespondInfo(response, true)
	return nil
}

func (self *ACE) cmd_ACE_ENDLESS_SPOOL_STATUS(gcmd printerpkg.Command) error {
	for _, line := range self.state.BuildEndlessSpoolStatusLines() {
		gcmd.RespondInfo(line, true)
	}
	return nil
}

func (self *ACE) cmd_ACE_DEBUG(gcmd printerpkg.Command) error {
	defer func() {
		if err := recover(); err != nil {
			self.gcode.RespondInfo(fmt.Sprintf("Error: %v", err), true)
		}
	}()
	cmd := requireACECommand(gcmd)
	method := cmd.Get("METHOD", object.Sentinel{}, "", nil, nil, nil, nil)
	params := cmd.Get("PARAMS", "", "", nil, nil, nil, nil)
	callback := func(response map[string]interface{}) {
		s, _ := json.Marshal(response)
		self.gcode.RespondInfo("ACE: Response:"+string(s), true)
	}

	if params != "" {
		_params, _ := json.Marshal(params)
		self.send_request(map[string]interface{}{
			"method": method,
			"params": _params,
		}, callback)
	} else {
		self.send_request(map[string]interface{}{
			"method": method,
		}, callback)
	}

	return nil
}

func (self *ACE) Get_status(eventtime float64) map[string]interface{} {
	_ = eventtime
	return self.state.BuildHubStatus()
}

func (self *ACE) cmd_ACE_SET_SLOT(gcmd printerpkg.Command) error {
	cmd := requireACECommand(gcmd)
	saveValue, response, err := self.state.ApplyInventorySlotUpdate(ACEInventorySlotUpdate{
		Index:    cmd.Get_int("INDEX", -1, nil, nil),
		Empty:    cmd.Get_int("EMPTY", 0, nil, nil) == 1,
		Color:    cmd.Get("COLOR", "", "", nil, nil, nil, nil),
		Material: cmd.Get("MATERIAL", "", "", nil, nil, nil, nil),
		Temp:     cmd.Get_int("TEMP", 0, nil, nil),
	})
	if err != nil {
		return err
	}
	self.gcode.RunScriptFromCommand(
		fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_inventory VALUE=%s", saveValue))
	gcmd.RespondInfo(response, true)
	return nil
}

func (self *ACE) cmd_ACE_QUERY_SLOTS(gcmd printerpkg.Command) error {
	data, err := self.state.InventoryJSON()
	if err != nil {
		return err
	}
	gcmd.RespondInfo("ACE: query slots:"+string(data), true)
	return nil
}

func (self *ACE) cmd_ACE_SAVE_INVENTORY(gcmd printerpkg.Command) error {
	saveValue, err := self.state.SaveInventory()
	if err != nil {
		return err
	}
	self.gcode.RunScriptFromCommand(
		fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_inventory VALUE=%s", saveValue))
	gcmd.RespondInfo("ACE: Inventory saved to persistent storage", true)
	return nil
}

func (self *ACE) cmd_ACE_TEST_RUNOUT_SENSOR(gcmd printerpkg.Command) error {
	sensor_extruder := self.lookupFilamentSensor("extruder_sensor")

	if sensor_extruder != nil {
		runout_helper_present := sensor_extruder.FilamentPresent()
		endstop_triggered := self._check_endstop_state("extruder_sensor")
		for _, line := range self.state.BuildRunoutSensorStatusLines(runout_helper_present, endstop_triggered) {
			gcmd.RespondInfo(line, true)
		}
	} else {
		gcmd.RespondInfo("ACE: Extruder sensor not found", true)
	}
	return nil
}

func LoadConfigACE(config printerpkg.ModuleConfig) interface{} {
	return NewACE(config)
}

func LoadConfigFilamentHub(config printerpkg.ModuleConfig) interface{} {
	return NewACE(config)
}

func (self *ACE) Set_filament_info(index int, typ string, sku string, color []interface{}) {
	self.state.SetPanelFilamentInfo(index, typ, sku, color)
}
