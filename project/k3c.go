package project

import (
	"flag"
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	"goklipper/common/utils/sys"
	"goklipper/common/value"
	addonpkg "goklipper/internal/addon"
	gcodepkg "goklipper/internal/pkg/gcode"
	heaterpkg "goklipper/internal/pkg/heater"
	mcupkg "goklipper/internal/pkg/mcu"
	"goklipper/internal/pkg/msgproto"
	printerpkg "goklipper/internal/pkg/printer"
	reactorpkg "goklipper/internal/pkg/reactor"
	"goklipper/internal/pkg/util"
	"runtime/debug"

	"os"
	"runtime"
	"strings"
	"time"
)

const (
	message_ready   = "Printer is ready"
	message_startup = "Printer is not ready\nThe project host software is attempting to connect.Please\nretry in a few moments."

	message_restart = "Once the underlying issue is corrected, use the \"RESTART\"\ncommand to reload the config and restart the host software.\nPrinter is halted"

	message_protocol_error1 = "This is frequently caused by running an older version of the\nfirmware on the MCU(s).Fix by recompiling and flashing the\nfirmware."

	message_protocol_error2 = "Once the underlying issue is corrected, use the \"RESTART\"\ncommand to reload the config and restart the host software."

	message_mcu_connect_error = "Once the underlying issue is corrected, use the\n\"FIRMWARE_RESTART\" command to reset the firmware, reload the\nconfig, and restart the host software.\nError configuring printer"

	message_shutdown = "Once the underlying issue is corrected, use the\n\"FIRMWARE_RESTART\" command to reset the firmware, reload the\nconfig, and restart the host software.Printer is shutdown"
)

type (
	IReactor           = reactorpkg.IReactor
	ReactorFileHandler = reactorpkg.ReactorFileHandler
	ReactorTimer       = reactorpkg.ReactorTimer
	ReactorCompletion  = reactorpkg.ReactorCompletion
	ReactorCallback    = reactorpkg.ReactorCallback
	ReactorMutex       = reactorpkg.ReactorMutex
	SelectReactor      = reactorpkg.SelectReactor
	Poll               = reactorpkg.Poll
	PollReactor        = reactorpkg.PollReactor
	EPollReactor       = reactorpkg.EPollReactor
)

var (
	NewReactorFileHandler = reactorpkg.NewReactorFileHandler
	NewReactorTimer       = reactorpkg.NewReactorTimer
	NewReactorCompletion  = reactorpkg.NewReactorCompletion
	NewReactorCallback    = reactorpkg.NewReactorCallback
	NewReactorMutex       = reactorpkg.NewReactorMutex
	NewSelectReactor      = reactorpkg.NewSelectReactor
	NewPollReactor        = reactorpkg.NewPollReactor
	NewEPollReactor       = reactorpkg.NewEPollReactor
)

type Printer struct {
	runtime       *printerpkg.Runtime
	config_error  *Config_error
	Command_error *CommandError
	reactor       IReactor
	Module        *printerpkg.ModuleRegistry
}

type printerRuntimeReactorAdapter struct {
	reactor IReactor
}

func (self *printerRuntimeReactorAdapter) Run() error {
	return self.reactor.Run()
}

func (self *printerRuntimeReactorAdapter) Monotonic() float64 {
	return self.reactor.Monotonic()
}

func (self *printerRuntimeReactorAdapter) End() {
	self.reactor.End()
}

func (self *printerRuntimeReactorAdapter) Register_callback(callback func(interface{}) interface{}, eventtime float64) {
	self.reactor.Register_callback(callback, eventtime)
}

func (self *printerRuntimeReactorAdapter) Register_async_callback(callback func(argv interface{}) interface{}, waketime float64) {
	self.reactor.Register_async_callback(callback, waketime)
}

func (self *printerRuntimeReactorAdapter) Get_gc_stats() [3]float64 {
	return self.reactor.Get_gc_stats()
}

func newPrinterRuntimeReactorAdapter(reactor IReactor) printerpkg.Reactor {
	return &printerRuntimeReactorAdapter{reactor: reactor}
}

func NewPrinter(main_reactor IReactor, start_args map[string]interface{}) *Printer {
	self := Printer{}
	self.config_error = &Config_error{}
	self.Command_error = &CommandError{}
	self.reactor = main_reactor
	self.runtime = printerpkg.NewRuntime(newPrinterRuntimeReactorAdapter(main_reactor), start_args)
	self.reactor.Register_callback(self._connect, constants.NOW)
	// Init printer components that must be setup prior to config
	for _, m := range []func(*Printer){Add_early_printer_objects1,
		Add_early_printer_objects_webhooks} {
		m(&self)
	}
	self.Module = LoadMainModule()
	return &self
}
func (self *Printer) Get_start_args() map[string]interface{} {
	return self.runtime.GetStartArgs()
}
func (self *Printer) Get_reactor() IReactor {
	return self.reactor
}
func (self *Printer) get_state_message() (string, string) {
	return self.runtime.StateMessage()
}
func (self *Printer) Is_shutdown() bool {
	return self.runtime.IsShutdown()
}
func (self *Printer) _set_state(msg string) {
	self.runtime.SetState(msg)
}
func (self *Printer) Add_object(name string, obj interface{}) error {
	return self.runtime.AddObject(name, obj)
}
func (self *Printer) Lookup_object(name string, default1 interface{}) interface{} {
	return self.runtime.LookupObject(name, default1)
}

func MustLookupToolhead(printer *Printer) *Toolhead {
	toolhead, ok := printer.Lookup_object("toolhead", object.Sentinel{}).(*Toolhead)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "toolhead", printer.Lookup_object("toolhead", object.Sentinel{})))
	}
	return toolhead
}

func MustLookupPins(printer *Printer) *printerpkg.PrinterPins {
	pins, ok := printer.Lookup_object("pins", object.Sentinel{}).(*printerpkg.PrinterPins)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "pins", printer.Lookup_object("pins", object.Sentinel{})))
	}
	return pins
}

func MustLookupGcode(printer *Printer) *GCodeDispatch {
	gcode, ok := printer.Lookup_object("gcode", object.Sentinel{}).(*GCodeDispatch)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "gcode", printer.Lookup_object("gcode", object.Sentinel{})))
	}
	return gcode
}

func MustLookupGCodeMove(printer *Printer) *gcodepkg.GCodeMoveModule {
	gcodeMove, ok := printer.Lookup_object("gcode_move", object.Sentinel{}).(*gcodepkg.GCodeMoveModule)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "gcode_move", printer.Lookup_object("gcode_move", object.Sentinel{})))
	}
	return gcodeMove
}

func (self *Printer) LookupObject(name string, defaultValue interface{}) interface{} {
	obj := self.Lookup_object(name, defaultValue)
	switch name {
	case "pins":
		if typed, ok := obj.(*printerpkg.PrinterPins); ok {
			return &pinRegistryAdapter{pins: typed}
		}
	case "heaters":
		if typed, ok := obj.(*heaterpkg.PrinterHeaters); ok {
			return &heaterManagerAdapter{heaters: typed}
		}
	case "toolhead":
		if typed, ok := obj.(*Toolhead); ok {
			return typed
		}
	case "configfile":
		if typed, ok := obj.(*PrinterConfig); ok {
			return typed
		}
	}
	return obj
}

func (self *Printer) RegisterEventHandler(event string, callback func([]interface{}) error) {
	self.Register_event_handler(event, callback)
}

func (self *Printer) SendEvent(event string, params []interface{}) {
	_, _ = self.Send_event(event, params)
}

func (self *Printer) CurrentExtruderName() string {
	return MustLookupToolhead(self).Get_extruder().Get_name()
}

func (self *Printer) AddObject(name string, obj interface{}) error {
	return self.Add_object(name, obj)
}

func (self *Printer) LookupObjects(module string) []interface{} {
	return self.Lookup_objects(module)
}

func (self *Printer) HasStartArg(name string) bool {
	_, ok := self.Get_start_args()[name]
	return ok
}

func (self *Printer) LookupHeater(name string) printerpkg.HeaterRuntime {
	pheaters := self.Lookup_object("heaters", object.Sentinel{}).(*heaterpkg.PrinterHeaters)
	return &heaterRuntimeAdapter{heater: pheaters.Lookup_heater(name)}
}

func (self *Printer) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	pheaters := self.Lookup_object("heaters", object.Sentinel{}).(*heaterpkg.PrinterHeaters)
	return &temperatureSensorRegistryAdapter{heaters: pheaters}
}

func (self *Printer) LookupMCU(name string) printerpkg.MCURuntime {
	return Get_printer_mcu(self, name)
}

func (self *Printer) InvokeShutdown(msg string) {
	self.Invoke_shutdown(msg)
}

func (self *Printer) IsShutdown() bool {
	return self.Is_shutdown()
}

func (self *Printer) Reactor() printerpkg.ModuleReactor {
	return &moduleReactorAdapter{reactor: self.Get_reactor()}
}

func (self *Printer) StepperEnable() printerpkg.StepperEnableRuntime {
	stepperEnable, ok := self.Lookup_object("stepper_enable", object.Sentinel{}).(*mcupkg.PrinterStepperEnableModule)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "stepper_enable", self.Lookup_object("stepper_enable", object.Sentinel{})))
	}
	return stepperEnable
}

func (self *Printer) GCode() printerpkg.GCodeRuntime {
	return MustLookupGcode(self)
}

func (self *Printer) GCodeMove() printerpkg.MoveTransformController {
	return MustLookupGCodeMove(self)
}

func (self *Printer) Webhooks() printerpkg.WebhookRegistry {
	webhooks, ok := self.Lookup_object("webhooks", object.Sentinel{}).(*WebHooks)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "webhooks", self.Lookup_object("webhooks", object.Sentinel{})))
	}
	return webhooks
}
func (self *Printer) Lookup_objects(module string) []interface{} {
	return self.runtime.LookupObjects(module)
}

type moduleReactorAdapter struct {
	reactor IReactor
}

func (self *moduleReactorAdapter) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return &moduleTimerHandle{reactor: self.reactor, timer: self.reactor.Register_timer(callback, waketime)}
}

func (self *moduleReactorAdapter) RegisterAsyncCallback(callback func(float64)) {
	self.reactor.Register_async_callback(func(argv interface{}) interface{} {
		callback(argv.(float64))
		return nil
	}, 0)
}

func (self *moduleReactorAdapter) RegisterCallback(callback func(float64), waketime float64) {
	self.reactor.Register_callback(func(argv interface{}) interface{} {
		callback(argv.(float64))
		return nil
	}, waketime)
}

func (self *moduleReactorAdapter) Monotonic() float64 {
	return self.reactor.Monotonic()
}

func (self *moduleReactorAdapter) Pause(waketime float64) float64 {
	return self.reactor.Pause(waketime)
}

func (self *moduleReactorAdapter) Completion() interface{} {
	return self.reactor.Completion()
}

func (self *moduleReactorAdapter) AsyncComplete(completion interface{}, result map[string]interface{}) {
	typed, ok := completion.(*ReactorCompletion)
	if !ok {
		panic("unsupported reactor completion type")
	}
	self.reactor.Async_complete(typed, result)
}

type moduleTimerHandle struct {
	reactor IReactor
	timer   *ReactorTimer
}

func (self *moduleTimerHandle) Update(waketime float64) {
	self.reactor.Update_timer(self.timer, waketime)
}

type heaterRuntimeAdapter struct {
	heater *heaterpkg.Heater
}

func (self *heaterRuntimeAdapter) GetTemperature(eventtime float64) (float64, float64) {
	return self.heater.Get_temp(eventtime)
}

type heaterManagerAdapter struct {
	heaters *heaterpkg.PrinterHeaters
}

func (self *heaterManagerAdapter) LookupHeater(name string) interface{} {
	return self.heaters.Lookup_heater(name)
}

func (self *heaterManagerAdapter) SetupHeater(config printerpkg.ModuleConfig, gcodeID string) interface{} {
	return self.heaters.Setup_heater(config, gcodeID)
}

func (self *heaterManagerAdapter) Set_temperature(heater interface{}, temp float64, wait bool) error {
	switch typed := heater.(type) {
	case *heaterRuntimeAdapter:
		return self.heaters.Set_temperature(typed.heater, temp, wait)
	case *heaterpkg.Heater:
		return self.heaters.Set_temperature(typed, temp, wait)
	default:
		panic("unsupported heater adapter type")
	}
}

type temperatureSensorRegistryAdapter struct {
	heaters *heaterpkg.PrinterHeaters
}

func (self *temperatureSensorRegistryAdapter) AddSensorFactory(sensorType string, factory printerpkg.TemperatureSensorFactory) {
	self.heaters.Add_sensor_factory(sensorType, factory)
}

type pinRegistryAdapter struct {
	pins *printerpkg.PrinterPins
}

func (self *pinRegistryAdapter) SetupPWM(pin string) interface{} {
	return self.pins.Setup_pin("pwm", pin).(*MCU_pwm)
}

func (self *pinRegistryAdapter) SetupDigitalOut(pin string) printerpkg.DigitalOutPin {
	return self.pins.Setup_pin("digital_out", pin).(*MCU_digital_out)
}

func (self *pinRegistryAdapter) SetupADC(pin string) printerpkg.ADCPin {
	return self.pins.Setup_pin("adc", pin).(*MCU_adc)
}

func (self *pinRegistryAdapter) LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{} {
	return self.pins.Lookup_pin(pinDesc, canInvert, canPullup, shareType)
}

func (self *Printer) load_object1(config *ConfigWrapper, section string) interface{} {
	return self.Module.LoadObject(section,
		func(name string) interface{} {
			return self.Lookup_object(name, nil)
		},
		func(name string) interface{} {
			return config.Getsection(name)
		},
		self.runtime.StoreObject,
	)
}

func (self *Printer) reload_object(config *ConfigWrapper, section string) interface{} {
	return self.Module.ReloadObject(section,
		func(name string) interface{} {
			return config.Getsection(name)
		},
		self.runtime.StoreObject,
		func(name string) interface{} {
			return self.Lookup_object(name, nil)
		},
	)
}

func (self *Printer) Load_object(config *ConfigWrapper, section string, default1 interface{}) interface{} {
	obj := self.load_object1(config, section)
	if obj == nil {
		if _, ok := default1.(*object.Sentinel); ok {
			self.config_error.E = fmt.Sprintf("Unable to load module '%s'", section)
			logger.Info("moudle as ", section, " is not support")
		} else {
			if _, ok := default1.(object.Sentinel); ok {
				self.config_error.E = fmt.Sprintf("Unable to load module '%s'", section)
				logger.Info("moudle as ", section, " is not support")
			} else {
				return default1
			}

		}
	}
	return obj
}

func (self *Printer) _read_config() {
	pconfig := NewPrinterConfig(self)
	self.runtime.StoreObject("configfile", pconfig)

	config := pconfig.Read_main_config()
	// Create printer components
	config.Get_printer().Add_object("pins", printerpkg.NewPrinterPins())
	for _, m := range []func(*ConfigWrapper){Add_printer_objects_mcu} {
		m(config)
	}

	for _, section_config := range config.Get_prefix_sections("") {
		self.Load_object(config, section_config.Get_name(), value.None)
	}
	Add_printer_objects_toolhead(config)
	addonpkg.LoadTuningTower(config)
	// Validate that there are no undefined parameters in the config file
	pconfig.Check_unused_options(config)
}
func (self *Printer) _build_protocol_error_message(e interface{}) string {
	host_version := self.Get_start_args()["software_version"]
	msg_update := []string{}
	msg_updated := []string{}
	for _, m := range self.Lookup_objects("mcu") {
		mcu := m.(map[string]interface{})["mcu"].(*MCU)
		mcu_name := m.(map[string]interface{})["mcu_name"]
		mcu_version := mcu.Get_status(0)["mcu_version"]

		if mcu_version != host_version {
			msg_update = append(msg_update, fmt.Sprintf("%s: Current version %s", strings.TrimSpace(mcu_name.(string)), mcu_version))
		} else {
			msg_updated = append(msg_updated, fmt.Sprintf("%s: Current version %s", strings.TrimSpace(mcu_name.(string)), mcu_version))
		}
	}
	if len(msg_update) == 0 {
		msg_update = append(msg_update, "<none>")
	}
	if len(msg_updated) == 0 {
		msg_updated = append(msg_updated, "<none>")
	}
	msg := "MCU Protocol error"
	return strings.Join([]string{msg, "\n"}, "")
}
func (self *Printer) _connect(eventtime interface{}) interface{} {
	self.tryCatchConnect1()
	self.tryCatchConnect2()
	return nil
}
func (self *Printer) tryCatchConnect1() {
	defer func() {
		if err := recover(); err != nil {
			_, ok1 := err.(*printerpkg.PinError)
			_, ok2 := err.(*Config_error)
			if ok1 || ok2 {
				logger.Error("Config error", err, string(debug.Stack()))
				self._set_state(fmt.Sprintf("%s\n%s", err, message_restart))
				return
			}
			_, ok11 := err.(msgproto.MsgprotoError)
			if ok11 {
				logger.Error("Protocol error", string(debug.Stack()))
				self._set_state(self._build_protocol_error_message(err))
				util.Dump_mcu_build()
				return
			}
			e, ok22 := err.(*erro)
			if ok22 {
				logger.Error("MCU error during connect", string(debug.Stack()))
				self._set_state(fmt.Sprintf("%s%s", e.err, message_mcu_connect_error))
				util.Dump_mcu_build()
				return
			}
			logger.Errorf("Unhandled exception during connect: %v, debug stack:\n%s\n", err, string(debug.Stack()))
			self._set_state(fmt.Sprintf("Internal error during connect: %s\n%s", err, message_restart))
			panic(err)
		}
	}()

	self._read_config()
	self.Send_event("project:mcu_identify", nil)
	cbs := self.runtime.EventHandlers("project:connect")
	logger.Info("Klipper-go: start running project:connect event handlers")
	for _, cb := range cbs {
		stateMessage, _ := self.get_state_message()
		if stateMessage != message_startup {
			return
		}
		err := cb(nil)
		if err != nil {
			logger.Error("Config error: ", err)
			self._set_state(fmt.Sprintf("%s\n%s", err.Error(), message_restart))
			return
		}
	}
	logger.Info("Klipper-go: finished running project:connect event handlers")
}
func (self *Printer) tryCatchConnect2() {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("Unhandled exception during ready callback", err)
			self.Invoke_shutdown(fmt.Sprintf("Internal error during ready callback: %s", err))
			return
		}
	}()
	self._set_state(message_ready)
	logger.Info("Klipper-go: entered message_ready state!")
	cbs := self.runtime.EventHandlers("project:ready")
	for _, cb := range cbs {
		stateMessage, _ := self.get_state_message()
		if stateMessage != message_ready {
			return
		}
		err := cb(nil)
		if err != nil {
			logger.Error("Unhandled exception during ready callback")
			self._set_state(fmt.Sprintf("Internal error during ready callback:%s", err.Error()))
			return
		}
	}
}
func (self *Printer) Run() string {
	return self.runtime.Run()
}
func (self *Printer) Set_rollover_info(name, info string, isLog bool) {
	if isLog {
		logger.Debug(info)
	}
}
func (self *Printer) Invoke_shutdown(msg interface{}) interface{} {
	return self.runtime.InvokeShutdown(msg)
}

func (self *Printer) invoke_async_shutdown(msg string) {
	_func := func(argv interface{}) interface{} {
		self.Invoke_shutdown(msg)
		return nil
	}
	self.reactor.Register_async_callback(_func, constants.NOW)
}
func (self *Printer) Register_event_handler(event string, callback func([]interface{}) error) {
	self.runtime.RegisterEventHandler(event, callback)
}
func (self *Printer) Send_event(event string, params []interface{}) ([]interface{}, error) {
	return self.runtime.SendEvent(event, params)
}
func (self *Printer) Request_exit(result string) {
	self.runtime.RequestExit(result)
}
func ModuleKlipper() {

}

type OptionParser struct {
	Debuginput  string
	Inputtty    string
	Apiserver   string
	Logfile     string
	Verbose     bool
	Debugoutput string
	Dictionary  string
	Import_test bool
	log_level   int
}

type Klipper struct {
}

func NewKlipper() *Klipper {
	Klipper := Klipper{}

	return &Klipper
}

//######################################################################
//# Startup
//######################################################################

func (self Klipper) Main() {
	usage := "[options] <your config file>"
	flag.Usage = func() {
		fmt.Println(os.Args[0], usage)
	}
	options := OptionParser{}
	flag.StringVar(&options.Debuginput, "i", "", "read commands from file instead of from tty port")
	flag.StringVar(&options.Inputtty, "I", "/tmp/printer", "input tty name (default is /tmp/printer)")
	flag.StringVar(&options.Apiserver, "a", "/tmp/unix_uds", "api server unix domain socket filename")
	flag.StringVar(&options.Logfile, "l", "/tmp/gklib.log", "write log to file instead of stderr")
	flag.BoolVar(&options.Verbose, "v", true, "enable debug messages")
	flag.StringVar(&options.Debugoutput, "o", "", "write output to file instead of to serial port")
	flag.StringVar(&options.Dictionary, "d", "", "file to read for mcu protocol dictionary")
	flag.BoolVar(&options.Import_test, "import-test", false, "perform an import module test")
	flag.IntVar(&options.log_level, "ll", int(logger.DebugLevel), "set logger level")

	flag.Parse()
	args := flag.Args()
	if options.Import_test {
		import_test()
	}
	start_args := map[string]interface{}{}
	if len(args) != 1 {
		flag.Usage()
		start_args["config_file"] = "./printer.cfg"
	} else {
		start_args["config_file"] = args[0]
	}

	start_args["apiserver"] = options.Apiserver
	start_args["start_reason"] = "startup"

	if options.Debuginput != "" {
		start_args["debuginput"] = options.Debuginput
		debuginput, err := os.OpenFile(options.Debuginput, os.O_RDONLY, 0644)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(3)
		}
		//debuginput =io. open(options.debuginput, 'rb')
		start_args["gcode_fd"] = debuginput.Fd()
	} else {
		//start_args["gcode_fd"] = util.create_pty(options.Inputtty)
		//debuginput, err := os.OpenFile(options.Inputtty, os.O_SYNC, 0644)
		//if err != nil {
		//	logger.Error(err.Error())
		//	os.Exit(3)
		//}
		//start_args["gcode_fd"] = debuginput.Fd()
	}
	if options.Debugoutput != "" {
		start_args["debugoutput"] = options.Debugoutput
	}

	// init logger
	debuglevel := logger.DebugLevel
	if options.Logfile != "" {
		start_args["log_file"] = options.Logfile
		if options.log_level > 0 {
			debuglevel = logger.LogLevel(options.log_level)
		}
		logger.InitLogger(debuglevel,
			options.Logfile,
			logger.SUPPORT_COLOR,
			2,
			2,
			7,
		)
	}

	logger.Info("Starting Klipper...")

	start_args["software_version"] = sys.GetSoftwareVersion()
	start_args["cpu_info"] = sys.GetCpuInfo()

	logger.Infof("Args: %s, Git version: %s, CPU: %s",
		strings.Join(flag.Args(), ""),
		start_args["software_version"].(string),
		start_args["cpu_info"].(string))

	runtime.GC()

	// Start Printer() class
	var res string
	for {
		main_reactor := NewEPollReactor(true)
		printer := NewPrinter(main_reactor, start_args)
		res = printer.Run()
		if res == "exit" || res == "error_exit" {
			break
		}
		time.Sleep(time.Second)
		logger.Info("Restarting printer")
		start_args["start_reason"] = res
	}
	if res == "error_exit" {
		os.Exit(-1)
	}
	os.Exit(0)
}
func import_test() {

	os.Exit(0)
}
