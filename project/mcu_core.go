package project

import (
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/file"
	"goklipper/common/utils/object"
	"goklipper/common/utils/sys"
	"goklipper/common/value"
	"goklipper/internal/pkg/chelper"
	mcupkg "goklipper/internal/pkg/mcu"
	msgprotopkg "goklipper/internal/pkg/msgproto"
	printerpkg "goklipper/internal/pkg/printer"
	serialpkg "goklipper/internal/pkg/serialhdl"
	"log"
	"reflect"
	"strings"
	"time"
)

type erro struct {
	err string
}

type MCU struct {
	error                string
	_printer             *Printer
	_clocksync           serialpkg.ClockSyncAble
	_reactor             IReactor
	_name                string
	Serial               *serialpkg.SerialReader
	_baud                int
	_canbus_iface        interface{}
	_serialport          string
	_restart_method      string
	_reset_cmd           *serialpkg.CommandWrapper
	_is_mcu_bridge       bool
	_emergency_stop_cmd  *serialpkg.CommandWrapper
	_is_shutdown         bool
	_shutdown_clock      int64
	_shutdown_msg        string
	Oid_count            int
	_config_callbacks    []interface{}
	_config_cmds         []string
	_restart_cmds        []string
	_init_cmds           []string
	_mcu_freq            float64
	_ffi_lib             interface{}
	_max_stepper_error   float64
	_reserved_move_slots int64
	_stepqueues          []interface{}
	_steppersync         interface{}
	_get_status_info     map[string]interface{}
	_stats_sumsq_base    float64
	_mcu_tick_avg        float64
	_mcu_tick_stddev     float64
	_mcu_tick_awake      float64
	_config_reset_cmd    *serialpkg.CommandWrapper
	_is_timeout          bool
	_flush_callbacks     []func(float64, int64)
}

func NewMCU(config *ConfigWrapper, clocksync serialpkg.ClockSyncAble) *MCU {
	self := MCU{}
	self._printer = config.Get_printer()
	printer := config.Get_printer()
	self._clocksync = clocksync
	self._reactor = printer.Get_reactor()
	self._name = config.Get_name()
	self._name = strings.TrimPrefix(self._name, "mcu ")
	// Serial port
	wp := fmt.Sprintf("mcu '%s': ", self._name)
	self.Serial = serialpkg.NewSerialReader(serialpkg.NewReactorAdapter(self._reactor), wp)
	self._baud = 0
	self._canbus_iface = nil
	canbus_uuid := config.Get("canbus_uuid", value.None, true)
	if canbus_uuid != nil && canbus_uuid.(string) != "" {
		self._serialport = canbus_uuid.(string)
		self._canbus_iface = config.Get("canbus_interface", "can0", true)
		//cbid := self._printer.Load_object(config, "canbus_ids")
		//cbid.Add_uuid(config, canbus_uuid, self._canbus_iface)
	} else {
		self._serialport = config.Get("serial", object.Sentinel{}, true).(string)
		if (strings.HasPrefix(self._serialport, "/dev/rpmsg_") ||
			strings.HasPrefix(self._serialport, "/tmp/klipper_host_")) == false {
			self._baud = config.Getint("baud", 250000, 2400, 0, true)
		}
	}
	// Restarts
	restart_methods := []string{"", "arduino", "cheetah", "command", "rpi_usb"}
	self._restart_method = "command"
	if self._baud > 0 {
		rmethods := map[interface{}]interface{}{}
		for _, m := range restart_methods {
			rmethods[m] = m
		}
		self._restart_method = config.Getchoice("restart_method",
			rmethods, nil, true).(string)
	}
	self._reset_cmd, self._config_reset_cmd = nil, nil
	self._is_mcu_bridge = false
	self._emergency_stop_cmd = nil
	self._is_shutdown, self._is_timeout = false, false
	self._shutdown_clock = 0
	self._shutdown_msg = ""
	// Config building
	pins := printer.Lookup_object("pins", object.Sentinel{})
	pins.(*printerpkg.PrinterPins).Register_chip(self._name, &self)
	self.Oid_count = 0
	self._config_callbacks = []interface{}{}
	self._config_cmds = []string{}
	self._restart_cmds = []string{}
	self._init_cmds = []string{}
	self._mcu_freq = 0.
	// Move command queuing
	self._ffi_lib = chelper.Get_ffi()
	//ffi_main := self._ffi_lib
	self._max_stepper_error = config.Getfloat("max_stepper_error", 0.000025,
		0., 0, 0, 0, true)
	self._reserved_move_slots = 0
	self._stepqueues = []interface{}{}
	self._steppersync = nil
	// Stats
	self._get_status_info = map[string]interface{}{}
	self._stats_sumsq_base = 0.
	self._mcu_tick_avg = 0.
	self._mcu_tick_stddev = 0.
	self._mcu_tick_awake = 0.
	// Register handlers
	printer.Register_event_handler("project:firmware_restart",
		self._firmware_restart)
	printer.Register_event_handler("project:mcu_identify",
		self._mcu_identify)
	printer.Register_event_handler("project:connect", self._connect)
	printer.Register_event_handler("project:shutdown", self._shutdown)
	printer.Register_event_handler("project:disconnect", self._disconnect)
	printer.Register_event_handler("project:ready", self._ready)
	return &self
}

func (self *MCU) _handle_mcu_stats(params map[string]interface{}) error {
	state := mcupkg.StatsState{TickAvg: self._mcu_tick_avg, TickStddev: self._mcu_tick_stddev, TickAwake: self._mcu_tick_awake}
	state.HandleMCUStats(params, self._mcu_freq, self._stats_sumsq_base)
	self._mcu_tick_avg = state.TickAvg
	self._mcu_tick_stddev = state.TickStddev
	self._mcu_tick_awake = state.TickAwake
	return nil
}

func (self *MCU) _handle_shutdown(params map[string]interface{}) error {
	if self._is_shutdown {
		return nil
	}
	plan := mcupkg.BuildShutdownPlan(self._name, params, self._clocksync.Dump_debug(), self.Serial.Dump_debug())
	self._is_shutdown = true
	if plan.HasShutdownClock {
		self._shutdown_clock = plan.ShutdownClock
	}
	self._shutdown_msg = plan.ShutdownMessage
	log.Println(plan.LogMessage)
	self._printer.invoke_async_shutdown(plan.AsyncMessage)
	gcode := self._printer.Lookup_object("gcode", object.Sentinel{}).(*GCodeDispatch)
	gcode.Respond_info(plan.RespondInfo, true)
	return nil
}

func (self *MCU) _handle_starting(params map[string]interface{}) {
	if message := mcupkg.BuildStartingShutdownMessage(self._is_shutdown, self._name); message != "" {
		self._printer.invoke_async_shutdown(message)
	}
}

// Connection phase
func (self *MCU) _check_restart(reason string) {
	decision := mcupkg.BuildRestartCheckDecision(self._printer.Get_start_args()["start_reason"], self._name, reason)
	if decision.Skip {
		return
	}
	logger.Debugf(decision.LogMessage)
	self._printer.Request_exit(decision.ExitReason)
	self._reactor.Pause(self._reactor.Monotonic() + decision.PauseSeconds)
	panic(decision.PanicMessage)
}

func (self *MCU) _connect_file(pace bool) {
	// In a debugging mode.  Open debug output file and read data dictionary
	//start_args := self._printer.Get_start_args()
	if self._name == "mcu" {
		//out_fname := start_args["debugoutput"]
		//dict_fname := start_args["dictionary"]
	} else {
		//out_fname := start_args["debugoutput"+"-"+self._name]
		//dict_fname := start_args["dictionary_"+self._name]
	}
	//outfile := open(out_fname, "wb")
	//dfile := open(dict_fname, "rb")
	//dict_data := dfile.read()
	//dfile.close()
	//self.Serial.Connect_file(outfile, dict_data)
	//self._clocksync.Connect_file(self.Serial, pace)
	//// Handle pacing
	//if pace == false {
	//	self.estimated_print_time = func(eventtime float64) float64 {
	//		return 0.
	//	}
	//}
}

func (self *MCU) _send_config(prev_crc *uint32) error {
	// Build config commands
	for _, cb := range self._config_callbacks {
		cb.(func())()
	}
	// Resolve pin names
	self.Serial.Get_msgparser().Get_constant("MCU", nil, reflect.String)
	ppins := self._printer.Lookup_object("pins", object.Sentinel{})
	pin_resolver := ppins.(*printerpkg.PrinterPins).Get_pin_resolver(self._name)
	plan := mcupkg.BuildConfigPlan(self.Oid_count, self._config_cmds, self._restart_cmds, self._init_cmds, pin_resolver.Update_command)
	self._config_cmds = plan.ConfigCmds
	self._restart_cmds = plan.RestartCmds
	self._init_cmds = plan.InitCmds
	if prev_crc != nil {
		logger.Debug("config_crc:", plan.ConfigCRC, " prev_crc:", *prev_crc)
	} else {
		logger.Debug("config_crc:", plan.ConfigCRC)
	}
	if prev_crc != nil && plan.ConfigCRC != *prev_crc {
		self._check_restart("CRC mismatch")
		panic(fmt.Sprintf("MCU '%s' CRC does not match config", self._name))
	}
	// Transmit config messages (if needed)
	self.Register_response(self._handle_starting, "starting", nil)
	if prev_crc == nil {
		logger.Debugf("Sending MCU '%s' printer configuration...",
			self._name)
		for _, c := range plan.ConfigCmds {
			self.Serial.Send(c, 0, 0)
		}
	} else {
		for _, c := range plan.RestartCmds {
			self.Serial.Send(c, 0, 0)
		}
	}
	// Transmit init messages
	for _, c := range plan.InitCmds {
		self.Serial.Send(c, 0, 0)
	}
	return nil
}

func (self *MCU) _send_get_config() *mcupkg.ConfigSnapshot {
	get_config_cmd := self.Lookup_query_command(
		"get_config",
		"config is_config=%c crc=%u is_shutdown=%c move_count=%hu", -1, nil, false)
	if self.Is_fileoutput() {
		return mcupkg.DefaultFileoutputConfigSnapshot()
	}
	configParams := get_config_cmd.Send([]int64{}, 0, 0)
	if configParams == nil {
		return nil
	}
	snapshot := mcupkg.ParseConfigSnapshot(configParams.(map[string]interface{}))
	decision := mcupkg.EvaluateConfigQuery(snapshot, self._is_shutdown, self._shutdown_msg, self._name, self.Try_lookup_command("clear_shutdown") != nil)
	if decision.ErrorMessage != "" {
		panic(&erro{decision.ErrorMessage})
	}
	if !decision.NeedsClearShutdown {
		return snapshot
	}
	logger.Warnf("Attempting to send clear_shutdown to reset '%s' MCU state", self._name)
	cmd := self.Try_lookup_command("clear_shutdown")
	cmd.(*serialpkg.CommandWrapper).Send([]interface{}{}, 0, 0)

	// Re-query config to ensure shutdown is cleared
	time.Sleep(100 * time.Millisecond)
	configParams = get_config_cmd.Send([]interface{}{}, 0, 0)
	if configParams != nil {
		snapshot = mcupkg.ParseConfigSnapshot(configParams.(map[string]interface{}))
	}
	if errorMessage := mcupkg.EvaluateClearedShutdownSnapshot(snapshot, self._name); errorMessage != "" {
		panic(&erro{errorMessage})
	}
	logger.Infof("Successfully cleared shutdown on MCU '%s'", self._name)
	self._is_shutdown = false
	return snapshot
}

func (self *MCU) _log_info() string {
	msgparser := self.Serial.Get_msgparser()
	message_count := len(msgparser.Get_messages())
	version, build_versions := msgparser.Get_version_info()
	return mcupkg.BuildMCULogInfo(self._name, message_count, version, build_versions, self.Get_constants())
}

func (self *MCU) _connect([]interface{}) error {
	configSnapshot := self._send_get_config()
	startReason := self._printer.Get_start_args()["start_reason"].(string)
	decision := mcupkg.BuildConnectDecision(configSnapshot, self._restart_method, startReason, self._name)
	if decision.ReturnError != "" {
		return fmt.Errorf("%s", decision.ReturnError)
	}
	if decision.PanicMessage != "" {
		panic(decision.PanicMessage)
	}
	if decision.NeedsPreConfigReset {
		// Only configure mcu after usb power reset
		self._check_restart("full reset before config")
	}
	if decision.SendConfig {
		if decision.UsePrevCRC {
			aa := decision.PrevCRC
			self._send_config(&aa)
		} else {
			self._send_config(nil)
		}
	}
	if decision.NeedsRequery {
		configSnapshot = self._send_get_config()
	}
	if errorMessage := mcupkg.ValidateConfiguredSnapshot(configSnapshot, self.Is_fileoutput(), self._reserved_move_slots, self._name); errorMessage != "" {
		if strings.HasPrefix(errorMessage, "Too few moves available") {
			panic(&erro{errorMessage})
		}
		panic(errorMessage)
	}
	// Setup steppersync with the move_count returned by get_config
	move_count := configSnapshot.MoveCount

	self._steppersync =
		chelper.Steppersync_alloc(self.Serial.Serialqueue, self._stepqueues,
			len(self._stepqueues), int(move_count-self._reserved_move_slots))

	chelper.Steppersync_set_time(self._steppersync, 0., self._mcu_freq)
	// log config information
	configuredInfo := mcupkg.BuildConfiguredMCUInfo(self._name, move_count, self._log_info())
	logger.Debugf(configuredInfo.MoveMessage)
	self._printer.Set_rollover_info(self._name, configuredInfo.RolloverInfo, false)
	return nil
}

func (self *MCU) _mcu_identify(argv []interface{}) error {
	serialPathExists := true
	plan := mcupkg.BuildConnectionPlan(self.Is_fileoutput(), self._restart_method, self._serialport, self._baud, self._canbus_iface != nil, serialPathExists)
	if plan.Mode == mcupkg.ConnectionModeFileoutput {
		self._connect_file(false)
	} else {
		exist, err := file.PathExists(self._serialport)
		serialPathExists = exist
		if err != nil {
			logger.Error(err.Error())
		}
		plan = mcupkg.BuildConnectionPlan(self.Is_fileoutput(), self._restart_method, self._serialport, self._baud, self._canbus_iface != nil, serialPathExists)
		if plan.NeedsPowerEnableReset {
			// Try toggling usb power
			self._check_restart("enable power")
		}

		if plan.Mode == mcupkg.ConnectionModeCanbus {
			//cbid := self._printer.Lookup_object("canbus_ids")
			//nodeid := cbid.Get_nodeid(self._serialport)
			//self.Serial.Connect_canbus(self._serialport, nodeid,
			//	self._canbus_iface)
		} else if plan.Mode == mcupkg.ConnectionModeRemote {
			self.Serial.Connect_remote(self._serialport)
		} else if plan.Mode == mcupkg.ConnectionModeUART {
			// Cheetah boards require RTS to be deasserted
			// else a reset will trigger the built-in bootloader.
			self.Serial.Connect_uart(self._serialport, self._baud, plan.RTS)
		} else {
			self.Serial.Connect_pipe(self._serialport)
		}
		if plan.NeedsClockSyncConnect {
			self._clocksync.Connect(self.Serial)
		}
	}
	logger.Info(self._log_info())
	ppins := self._printer.Lookup_object("pins", object.Sentinel{})
	pin_resolver := ppins.(*printerpkg.PrinterPins).Get_pin_resolver(self._name)
	constants := self.Get_constants()
	for _, reserved := range mcupkg.CollectReservedPins(constants) {
		pin_resolver.Reserve_pin(reserved.Pin, reserved.Owner)
	}
	self._mcu_freq = self.Get_constant_float("CLOCK_FREQ")
	self._stats_sumsq_base = self.Get_constant_float("STATS_SUMSQ_BASE")
	self._emergency_stop_cmd, _ = self.Lookup_command("emergency_stop", nil)
	reset_cmd := self.Try_lookup_command("reset")
	if reset_cmd != nil {
		self._reset_cmd = reset_cmd.(*serialpkg.CommandWrapper)
	}
	config_reset_cmd := self.Try_lookup_command("config_reset")
	if config_reset_cmd != nil {
		self._config_reset_cmd = config_reset_cmd.(*serialpkg.CommandWrapper)
	}
	msgparser := self.Serial.Get_msgparser()
	version, build_versions := msgparser.Get_version_info()
	identifyPlan := mcupkg.BuildIdentifyFinalizePlan(self._restart_method, self._reset_cmd != nil, self._config_reset_cmd != nil, msgparser.Get_constant("SERIAL_BAUD", nil, reflect.Float64), msgparser.Get_constant("CANBUS_BRIDGE", 0, reflect.String), version, build_versions, constants)
	self._restart_method = identifyPlan.RestartMethod
	if identifyPlan.IsMCUBridge {
		self._is_mcu_bridge = true
		self._printer.Register_event_handler("project:firmware_restart",
			self._firmware_restart_bridge)
	}
	self._get_status_info = identifyPlan.StatusInfo
	self.Register_response(self._handle_shutdown, "shutdown", nil)
	self.Register_response(self._handle_shutdown, "is_shutdown", nil)
	self.Register_response(self._handle_mcu_stats, "stats", nil)
	return nil
}

// Restarts
func (self *MCU) _disconnect(argv []interface{}) error {
	self.Serial.Disconnect()
	chelper.Steppersync_free(self._steppersync)
	self._steppersync = nil
	return nil
}

func (self *MCU) _ready(argv []interface{}) error {
	check := mcupkg.BuildReadyFrequencyCheck(self.Is_fileoutput(), self._mcu_freq, self._reactor.Monotonic(), self._clocksync.Get_clock)
	if check.Skip {
		return nil
	}
	// Check that reported mcu frequency is in range
	if check.IsMismatch {
		logger.Errorf("MCU %s configured for %dMhz but running at %dMhz!",
			self._name, check.MCUFreqMHz, check.CalcFreqMHz)
	} else {
		logger.Debugf("MCU %s configured for %dMhz running at %dMhz",
			self._name, check.MCUFreqMHz, check.CalcFreqMHz)
	}
	return nil
}

func (self *MCU) _shutdown(argv []interface{}) error {
	force := false
	if len(argv) != 0 {
		force = argv[0].(bool)
	}
	decision := mcupkg.BuildEmergencyStopDecision(self._emergency_stop_cmd != nil, self._is_shutdown, force)
	if decision.Skip {
		return nil
	}
	self._emergency_stop_cmd.Send([]int64{}, 0, 0)
	return nil
}

func (self *MCU) _restart_arduino() {
	logger.Debugf("Attempting MCU '%s' reset", self._name)
	self._disconnect([]interface{}{})
	serialpkg.ArduinoReset(self._serialport, serialpkg.NewReactorAdapter(self._reactor))
}

func (self *MCU) _restart_cheetah() {
	logger.Debugf("Attempting MCU '%s' Cheetah-style reset", self._name)
	self._disconnect([]interface{}{})
	serialpkg.CheetahReset(self._serialport, serialpkg.NewReactorAdapter(self._reactor))
}

func (self *MCU) _restart_via_command() {
	plan := mcupkg.BuildCommandResetPlan(self._reset_cmd != nil, self._config_reset_cmd != nil, self._clocksync.Is_active(), self._name)
	if plan.ErrorMessage != "" {
		logger.Errorf(plan.ErrorMessage)
		return
	}
	if plan.Mode == mcupkg.CommandResetModeConfigReset {
		// Attempt reset via config_reset command
		logger.Debugf(plan.LogMessage)
		if plan.MarkShutdown {
			self._is_shutdown = true
		}
		if plan.NeedsEmergencyStop {
			self._shutdown([]interface{}{true})
		}
		self._reactor.Pause(self._reactor.Monotonic() + plan.PreSendPauseSeconds)
		self._config_reset_cmd.Send([]int64{}, 0, 0)
	} else {
		// Attempt reset via reset command
		logger.Debugf(plan.LogMessage)
		self._reset_cmd.Send([]int64{}, 0, 0)
	}
	time.Sleep(time.Duration(plan.PostSendPauseSeconds * float64(time.Second)))

	self._disconnect([]interface{}{})
}

func (self *MCU) _restart_rpi_usb() {
	logger.Debugf("Attempting MCU '%s' reset via rpi usb power", self._name)
	self._disconnect([]interface{}{})
	//chelper.Run_hub_ctrl(0)
	self._reactor.Pause(self._reactor.Monotonic() + 2.)
	//chelper.Run_hub_ctrl(1)
}

func (self *MCU) _firmware_restart(argv []interface{}) error {
	var force bool
	if argv != nil {
		force = argv[0].(bool)
	} else {
		force = false
	}
	plan := mcupkg.BuildFirmwareRestartPlan(force, self._is_mcu_bridge, self._restart_method)
	if plan.Skip {
		return nil
	}
	if plan.Action == mcupkg.FirmwareRestartActionRPIUSB {
		self._restart_rpi_usb()
	} else if plan.Action == mcupkg.FirmwareRestartActionCommand {
		self._restart_via_command()
	} else if plan.Action == mcupkg.FirmwareRestartActionCheetah {
		self._restart_cheetah()
	} else {
		self._restart_arduino()
	}
	return nil
}

func (self *MCU) _firmware_restart_bridge([]interface{}) error {
	self._firmware_restart([]interface{}{true})
	return nil
}

// Move queue tracking
func (self *MCU) register_stepqueue(stepqueue interface{}) {
	self._stepqueues = append(self._stepqueues, stepqueue)
}

func (self *MCU) request_move_queue_slot() {
	self._reserved_move_slots += 1
}

func (self *MCU) Register_flush_callback(callback func(float64, int64)) {
	self._flush_callbacks = append(self._flush_callbacks, callback)
}

func (self *MCU) CalibrateClock(printTime float64, eventtime float64) []float64 {
	return self._clocksync.Calibrate_clock(printTime, eventtime)
}

func (self *MCU) ClockSyncActive() bool {
	return self._clocksync.Is_active()
}

func (self *MCU) IsFileoutput() bool {
	return self.Is_fileoutput()
}

func (self *MCU) SetTime(offset float64, freq float64) {
	chelper.Steppersync_set_time(self._steppersync, offset, freq)
}

func (self *MCU) Flush(clock uint64, clearHistoryClock uint64) int {
	return chelper.Steppersync_flush(self._steppersync, clock, clearHistoryClock)
}

func (self *MCU) Flush_moves(print_time float64, clear_history_time float64) {
	var sync mcupkg.StepperSync
	if self._steppersync != nil {
		sync = self
	}
	err := mcupkg.FlushMoves(print_time, clear_history_time, self, sync, self._flush_callbacks)
	if err != nil {
		logger.Error("Internal error in MCU stepcompress:", self._name)
	}
}

func (self *MCU) Check_active(print_time float64, eventtime float64) {
	state := mcupkg.MoveQueueTimingState{IsTimeout: self._is_timeout}
	var sync mcupkg.StepperSync
	if self._steppersync != nil {
		sync = self
	}
	timedOut := state.CheckActive(print_time, eventtime, self, sync)
	self._is_timeout = state.IsTimeout
	if !timedOut {
		return
	}
	errStr := fmt.Sprintf("Lost communication with MCU %s", self._name)
	logger.Errorf("Timeout with MCU '%s' (eventtime=%f), ERROR:%s",
		self._name, eventtime, errStr)
	self._printer.Invoke_shutdown(errStr)

	gcode := self._printer.Lookup_object("gcode", object.Sentinel{}).(*GCodeDispatch)
	gcode.Respond_info(errStr, true)
}

// Misc external commands
func (self *MCU) Is_fileoutput() bool {
	return self._printer.Get_start_args()["debugoutput"] != nil
}

func (self *MCU) Is_shutdown() bool {
	return self._is_shutdown
}

func (self *MCU) Get_shutdown_clock() int64 {
	return self._shutdown_clock
}

func (self *MCU) Get_status(eventtime float64) map[string]interface{} {
	return sys.DeepCopyMap(self._get_status_info)
}

func (self *MCU) Stats(eventtime float64) (bool, string) {
	state := mcupkg.StatsState{TickAvg: self._mcu_tick_avg, TickStddev: self._mcu_tick_stddev, TickAwake: self._mcu_tick_awake}
	ok, summary, lastStats := state.BuildStatsSummary(self._name, self.Serial.Stats(eventtime), self._clocksync.Stats(eventtime))
	self._get_status_info["last_stats"] = lastStats
	return ok, summary
}

func (self *MCU) Get_status_info() map[string]interface{} {
	return self._get_status_info
}

// Config creation helpers
func (self *MCU) Setup_pin(pin_type string, pin_params map[string]interface{}) interface{} {
	pcs := map[string]interface{}{"endstop": NewMCU_endstop,
		"digital_out": NewMCU_digital_out, "pwm": NewMCU_pwm, "adc": NewMCU_adc}
	if pcs[pin_type] == nil {
		return nil
	}
	return pcs[pin_type].(func(*MCU, map[string]interface{}) interface{})(self, pin_params)
}

type MCU_digital_out struct {
	Pin            string
	Invert         int
	Mcu            *MCU
	_last_clock    int64
	Max_duration   float64
	_set_cmd       *serialpkg.CommandWrapper
	Oid            int
	Start_value    int
	Shutdown_value int
}

func NewMCU_digital_out(mcu *MCU, pin_params map[string]interface{}) interface{} {
	self := MCU_digital_out{}
	self.Mcu = mcu
	self.Oid = -1
	self.Mcu.Register_config_callback(self.Build_config)
	self.Pin = pin_params["pin"].(string)
	self.Invert = pin_params["invert"].(int)
	self.Start_value = self.Invert
	self.Shutdown_value = self.Start_value
	self.Max_duration = 2.0
	self._last_clock = 0
	self._set_cmd = nil
	return &self
}

func (self *MCU_digital_out) Get_mcu() *MCU {
	return self.Mcu
}

func (self *MCU_digital_out) MCU() printerpkg.PrintTimeEstimator {
	return self.Get_mcu()
}

func (self *MCU_digital_out) SetupMaxDuration(maxDuration float64) {
	self.Setup_max_duration(maxDuration)
}

func (self *MCU_digital_out) Setup_max_duration(max_duration float64) {
	self.Max_duration = max_duration
}

func (self *MCU_digital_out) Setup_start_value(start_value float64, shutdown_value float64) {
	state := mcupkg.DigitalOutRuntimeState{Invert: self.Invert}
	self.Start_value, self.Shutdown_value = state.SetupStartValue(start_value, shutdown_value)
}

func (self *MCU_digital_out) Build_config() {
	plan := mcupkg.BuildDigitalOutConfigPlan(self.Max_duration, self.Start_value, self.Shutdown_value, self.Mcu.Seconds_to_clock)

	self.Mcu.Request_move_queue_slot()
	self.Oid = self.Mcu.Create_oid()
	setupPlan := mcupkg.BuildDigitalOutConfigSetupPlan(self.Oid, self.Pin, self.Start_value, self.Shutdown_value, plan)
	for _, cmd := range setupPlan.Commands {
		self.Mcu.Add_config_cmd(cmd.Cmd, cmd.IsInit, cmd.OnRestart)
	}
	cmd_queue := self.Mcu.Alloc_command_queue()
	self._set_cmd, _ = self.Mcu.Lookup_command(setupPlan.LookupFormat, cmd_queue)
}

func (self *MCU_digital_out) Set_digital(print_time float64, value int) {
	state := mcupkg.DigitalOutRuntimeState{Invert: self.Invert, LastClock: self._last_clock}
	state.SetDigital(print_time, value, self.Mcu.Print_time_to_clock, self._set_cmd, self.Oid)
	self._last_clock = state.LastClock
}

func (self *MCU_digital_out) SetDigital(printTime float64, value int) {
	self.Set_digital(printTime, value)
}

type MCU_pwm struct {
	Pin            string
	Invert         int
	Mcu            *MCU
	_pwm_max       float64
	Hardware_pwm   interface{}
	_last_clock    int64
	Cycle_time     float64
	Start_value    float64
	_set_cmd       *serialpkg.CommandWrapper
	Oid            int
	Max_duration   float64
	Shutdown_value float64
}

func NewMCU_pwm(mcu *MCU, pin_params map[string]interface{}) interface{} {
	self := MCU_pwm{}
	self.Mcu = mcu
	self.Hardware_pwm = false
	self.Cycle_time = 0.1
	self.Max_duration = 2.0
	self.Oid = -1
	self.Mcu.Register_config_callback(self.Build_config)
	self.Pin = pin_params["pin"].(string)
	self.Invert = pin_params["invert"].(int)
	self.Start_value = float64(self.Invert)
	self.Shutdown_value = self.Start_value
	self._last_clock = 0
	self._pwm_max = 0.0
	self._set_cmd = nil
	return &self
}

func (self *MCU_pwm) Get_mcu() *MCU {
	return self.Mcu
}

func (self *MCU_pwm) MCU() interface{} {
	return self.Get_mcu()
}

func (self *MCU_pwm) SetupMaxDuration(maxDuration float64) {
	self.Setup_max_duration(maxDuration)
}

func (self *MCU_pwm) SetupCycleTime(cycleTime float64, hardwarePWM bool) {
	self.Setup_cycle_time(cycleTime, hardwarePWM)
}

func (self *MCU_pwm) SetupStartValue(startValue float64, shutdownValue float64) {
	self.Setup_start_value(startValue, shutdownValue)
}

func (self *MCU_pwm) Setup_max_duration(max_duration float64) {
	self.Max_duration = max_duration
}

func (self *MCU_pwm) Setup_cycle_time(cycle_time float64, hardware_pwm bool) {
	self.Cycle_time = cycle_time
	self.Hardware_pwm = hardware_pwm
}

func (self *MCU_pwm) Setup_start_value(start_value float64, shutdown_value float64) {
	state := mcupkg.PWMRuntimeState{Invert: self.Invert}
	self.Start_value, self.Shutdown_value = state.SetupStartValue(start_value, shutdown_value)
}

func (self *MCU_pwm) Build_config() {
	cmd_queue := self.Mcu.Alloc_command_queue()
	plan := mcupkg.BuildPWMConfigPlan(self.Max_duration, self.Cycle_time, self.Start_value, self.Shutdown_value, self.Hardware_pwm.(bool), self.Mcu.Get_constant_float("_pwm_max"), self.Mcu.Get_printer().Get_reactor().Monotonic, self.Mcu.Estimated_print_time, self.Mcu.Print_time_to_clock, self.Mcu.Seconds_to_clock)
	self._last_clock = plan.LastClock
	self._pwm_max = plan.PWMMax
	self.Mcu.Request_move_queue_slot()
	self.Oid = self.Mcu.Create_oid()
	setupPlan := mcupkg.BuildPWMConfigSetupPlan(self.Oid, self.Pin, self.Hardware_pwm.(bool), plan)
	for _, cmd := range setupPlan.Commands {
		self.Mcu.Add_config_cmd(cmd.Cmd, cmd.IsInit, cmd.OnRestart)
	}
	self._set_cmd, _ = self.Mcu.Lookup_command(setupPlan.LookupFormat, cmd_queue)
}

func (self *MCU_pwm) Set_pwm(print_time float64, val float64) {
	state := mcupkg.PWMRuntimeState{Invert: self.Invert, PWMMax: self._pwm_max, LastClock: self._last_clock}
	state.SetPWM(print_time, val, self.Mcu.Print_time_to_clock, self._set_cmd, self.Oid)
	self._last_clock = state.LastClock
}

func (self *MCU_pwm) SetPWM(printTime float64, value float64) {
	self.Set_pwm(printTime, value)
}

type MCU_adc struct {
	Pin               string
	Mcu               *MCU
	Report_clock      int64
	Min_sample        float64
	Max_sample        float64
	Last_state        []float64
	Inv_max_adc       float64
	Sample_count      int
	Range_check_count int
	Oid               int
	Callback          func(float64, float64)
	Sample_time       float64
	Report_time       float64
}

func NewMCU_adc(mcu *MCU, pin_params map[string]interface{}) interface{} {
	self := MCU_adc{}
	self.Mcu = mcu
	self.Pin = pin_params["pin"].(string)
	self.Min_sample = 0.0
	self.Max_sample = self.Min_sample
	self.Sample_time = 0.0
	self.Report_time = self.Sample_time
	self.Sample_count = 0
	self.Range_check_count = self.Sample_count
	self.Report_clock = 0
	self.Last_state = []float64{0.0, 0.0}
	self.Oid = -1
	self.Callback = nil
	self.Mcu.Register_config_callback(self.Build_config)
	self.Inv_max_adc = 0.0
	return &self
}

func (self *MCU_adc) Get_mcu() *MCU {
	return self.Mcu
}

func (self *MCU_adc) Setup_minmax(sample_time float64, sample_count int, minval float64, maxval float64, range_check_count int) {
	self.Sample_time = sample_time
	self.Sample_count = sample_count
	self.Min_sample = minval
	self.Max_sample = maxval
	self.Range_check_count = range_check_count
}

func (self *MCU_adc) Setup_adc_callback(report_time float64, callback func(float64, float64)) {
	self.Report_time = report_time
	self.Callback = callback
}

func (self *MCU_adc) Get_last_value() []float64 {
	return self.Last_state
}

func (self *MCU_adc) SetupCallback(reportTime float64, callback func(float64, float64)) {
	self.Setup_adc_callback(reportTime, callback)
}

func (self *MCU_adc) SetupMinMax(sampleTime float64, sampleCount int, minval float64, maxval float64, rangeCheckCount int) {
	self.Setup_minmax(sampleTime, sampleCount, minval, maxval, rangeCheckCount)
}

func (self *MCU_adc) GetLastValue() [2]float64 {
	return [2]float64{self.Last_state[0], self.Last_state[1]}
}

func (self *MCU_adc) runtimeState() mcupkg.ADCRuntimeState {
	state := mcupkg.ADCRuntimeState{
		InvMaxADC:   self.Inv_max_adc,
		ReportClock: self.Report_clock,
	}
	if len(self.Last_state) >= 2 {
		state.LastValue = [2]float64{self.Last_state[0], self.Last_state[1]}
	}
	return state
}

func (self *MCU_adc) applyRuntimeState(state mcupkg.ADCRuntimeState) {
	self.Last_state = []float64{state.LastValue[0], state.LastValue[1]}
	self.Inv_max_adc = state.InvMaxADC
	self.Report_clock = state.ReportClock
}

func (self *MCU_adc) Build_config() {
	if self.Sample_count == 0 {
		return
	}
	self.Oid = self.Mcu.Create_oid()
	state := self.runtimeState()
	plan := state.BuildConfigPlan(self.Oid, self.Sample_time, self.Report_time, self.Min_sample, self.Max_sample, self.Sample_count, self.Range_check_count, self.Mcu.Get_query_slot, self.Mcu.Seconds_to_clock, self.Mcu.Get_constant_float("ADC_MAX"))
	self.applyRuntimeState(state)
	setupPlan := mcupkg.BuildADCConfigSetupPlan(self.Oid, self.Pin, plan)
	self.Mcu.Add_config_cmd(setupPlan.ConfigCmd, false, false)
	self.Mcu.Add_config_cmd(setupPlan.QueryCmd, true, false)
	logger.Infof("REGISTERING %s", setupPlan.ResponseLogLabel)
	self.Mcu.Register_response(self.Handle_analog_in_state, setupPlan.ResponseName, self.Oid)
}

func (self *MCU_adc) Handle_analog_in_state(params map[string]interface{}) error {
	state := self.runtimeState()
	lastState := state.ProcessAnalogInState(params, self.Mcu.Clock32_to_clock64, self.Mcu.Clock_to_print_time)
	self.applyRuntimeState(state)
	if self.Callback != nil {
		self.Callback(lastState[1], lastState[0])
	}
	return nil
}

func (self *MCU) Create_oid() int {
	self.Oid_count += 1
	return self.Oid_count - 1
}

func (self *MCU) Register_config_callback(cb interface{}) {
	self._config_callbacks = append(self._config_callbacks, cb)
}

func (self *MCU) Add_config_cmd(cmd string, is_init bool, on_restart bool) {
	if is_init {
		self._init_cmds = append(self._init_cmds, cmd)
	} else if on_restart {
		self._restart_cmds = append(self._restart_cmds, cmd)
	} else {
		self._config_cmds = append(self._config_cmds, cmd)
	}
}

func (self *MCU) Get_query_slot(oid int) int64 {
	return mcupkg.QuerySlot(oid, self)
}

func (self *MCU) SecondsToClock(time float64) int64 {
	return self.Seconds_to_clock(time)
}

func (self *MCU) EstimatedPrintTime(eventtime float64) float64 {
	return self.Estimated_print_time(eventtime)
}

func (self *MCU) Monotonic() float64 {
	return self._reactor.Monotonic()
}

func (self *MCU) PrintTimeToClock(printTime float64) int64 {
	return self.Print_time_to_clock(printTime)
}

func (self *MCU) Register_stepqueue(stepqueue interface{}) {
	self._stepqueues = append(self._stepqueues, stepqueue)
}

func (self *MCU) Request_move_queue_slot() {
	self._reserved_move_slots += 1
}

func (self *MCU) Seconds_to_clock(time float64) int64 {
	return int64(time * self._mcu_freq)
}

func (self *MCU) Get_max_stepper_error() float64 {
	return self._max_stepper_error
}

// Wrapper functions
func (self *MCU) Get_printer() *Printer {
	return self._printer
}

func (self *MCU) Get_name() string {
	return self._name
}

func (self *MCU) Register_response(cb interface{}, msg string, oid interface{}) {
	self.Serial.Register_response(cb, msg, oid)
}

func (self *MCU) Alloc_command_queue() interface{} {
	return self.Serial.Alloc_command_queue()
}

func (self *MCU) Lookup_command(msgformat string, cq interface{}) (*serialpkg.CommandWrapper, error) {
	return serialpkg.NewCommandWrapper(self.Serial, msgformat, cq)
}

func (self *MCU) Lookup_query_command(msgformat string, respformat string, oid int,
	cq interface{}, is_async bool) *serialpkg.CommandQueryWrapper {
	return serialpkg.NewCommandQueryWrapper(self.Serial, msgformat, respformat, oid,
		cq, is_async, self._printer.Command_error)
}

func (self *MCU) Try_lookup_command(msgformat string) interface{} {
	ret, err := self.Lookup_command(msgformat, nil)
	if err != nil {
		return nil
	}
	return ret
}

func (self *MCU) Lookup_command_tag(msgformat string) interface{} {
	return msgprotopkg.FindCommandTag(self.Serial.Get_msgparser().Get_messages(), msgformat)
}

func (self *MCU) Get_enumerations() map[string]interface{} {
	return self.Serial.Get_msgparser().Get_enumerations()
}

func (self *MCU) Get_constants() map[string]interface{} {
	return self.Serial.Get_msgparser().Get_constants()
}

func (self *MCU) Get_constant_float(name string) float64 {
	return self.Serial.Get_msgparser().Get_constant_float(name, 0.0)
}

func (self *MCU) Print_time_to_clock(print_time float64) int64 {
	return self._clocksync.Print_time_to_clock(print_time)
}

func (self *MCU) Clock_to_print_time(clock int64) float64 {
	return self._clocksync.Clock_to_print_time(clock)
}

func (self *MCU) Estimated_print_time(eventtime float64) float64 {
	return self._clocksync.Estimated_print_time(eventtime)
}

func (self *MCU) Clock32_to_clock64(clock32 int64) int64 {
	return self._clocksync.Clock32_to_clock64(clock32)
}

func (self *MCU) CreateOID() int {
	return self.Create_oid()
}

func (self *MCU) RegisterConfigCallback(cb func()) {
	self.Register_config_callback(cb)
}

func (self *MCU) AddConfigCmd(cmd string, isInit bool, onRestart bool) {
	self.Add_config_cmd(cmd, isInit, onRestart)
}

func (self *MCU) GetQuerySlot(oid int) int64 {
	return self.Get_query_slot(oid)
}

func (self *MCU) RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{}) {
	self.Register_response(cb, msg, oid)
}

func (self *MCU) ClockToPrintTime(clock int64) float64 {
	return self.Clock_to_print_time(clock)
}

func (self *MCU) Clock32ToClock64(clock32 int64) int64 {
	return self.Clock32_to_clock64(clock32)
}

func (self *MCU) RegisterFlushCallback(callback func(float64, int64)) {
	self.Register_flush_callback(callback)
}

func (self *MCU) AllocCommandQueue() interface{} {
	return self.Alloc_command_queue()
}

func (self *MCU) LookupCommand(msgformat string, cq interface{}) (interface{}, error) {
	command, err := self.Lookup_command(msgformat, cq)
	if err != nil {
		return nil, err
	}
	return &mcuCommandAdapter{command: command}, nil
}

func (self *MCU) LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error) {
	return self.Lookup_command(msgformat, cq)
}

func (self *MCU) LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{} {
	return self.Lookup_query_command(msgformat, respformat, oid, cq, isAsync)
}

func (self *MCU) GetStatus(eventtime float64) map[string]interface{} {
	return self.Get_status(eventtime)
}

func (self *MCU) NewTrsyncCommandQueue() interface{} {
	trdispatch := chelper.Trdispatch_alloc()
	return NewMCU_trsync(self, trdispatch).Get_command_queue()
}

type mcuCommandAdapter struct {
	command *serialpkg.CommandWrapper
}

func (self *mcuCommandAdapter) Send(args []int64, minclock int64, reqclock int64) {
	self.command.Send(args, minclock, reqclock)
}

func Add_printer_objects_mcu(config *ConfigWrapper) {
	printer := config.Get_printer()
	reactor := printer.Get_reactor()
	mainsync := serialpkg.NewClockSync(serialpkg.NewReactorAdapter(reactor))
	printer.Add_object("mcu", NewMCU(config.Getsection("mcu"), mainsync))
	for _, s := range config.Get_prefix_sections("mcu ") {
		printer.Add_object(s.Section, NewMCU(
			s, serialpkg.NewSecondarySync(serialpkg.NewReactorAdapter(reactor), mainsync)))
	}
}

func Get_printer_mcu(printer *Printer, name string) *MCU {
	if name == "mcu" {
		return printer.Lookup_object(name, object.Sentinel{}).(*MCU)
	}

	return printer.Lookup_object("mcu "+name, object.Sentinel{}).(*MCU)
}

var _ mcupkg.QuerySlotSource = (*MCU)(nil)
var _ mcupkg.MoveQueueTimingSource = (*MCU)(nil)
var _ mcupkg.StepperSync = (*MCU)(nil)
