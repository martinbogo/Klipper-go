package project

import (
	"errors"
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	"goklipper/common/value"
	heaterpkg "goklipper/internal/pkg/heater"
	mcupkg "goklipper/internal/pkg/mcu"
	motionpkg "goklipper/internal/pkg/motion"
	kinematicspkg "goklipper/internal/pkg/motion/kinematics"
	tmcpkg "goklipper/internal/pkg/tmc"
	"math"
	"strings"
)

type tmcErrorCheckReactorAdapter struct {
	reactor IReactor
}

func (self *tmcErrorCheckReactorAdapter) Monotonic() float64 {
	return self.reactor.Monotonic()
}

func (self *tmcErrorCheckReactorAdapter) Pause(waketime float64) float64 {
	return self.reactor.Pause(waketime)
}

func (self *tmcErrorCheckReactorAdapter) RegisterTimer(callback func(float64) float64, waketime float64) interface{} {
	return self.reactor.Register_timer(callback, waketime)
}

func (self *tmcErrorCheckReactorAdapter) UnregisterTimer(timer interface{}) {
	self.reactor.Unregister_timer(timer.(*ReactorTimer))
}

func newTMCErrorCheckRuntime(config *ConfigWrapper, mcuTMC tmcpkg.RegisterAccess) *tmcpkg.ErrorCheckRuntime {
	printer := config.Get_printer()
	nameParts := strings.Split(config.Get_name(), " ")
	return tmcpkg.NewErrorCheckRuntime(
		nameParts[0],
		strings.Join(nameParts[1:], " "),
		mcuTMC,
		&tmcErrorCheckReactorAdapter{reactor: printer.Get_reactor()},
		func(msg string) {
			printer.Invoke_shutdown(msg)
		},
		func() {
			pheaters := printer.Load_object(config, "heaters", object.Sentinel{}).(*heaterpkg.PrinterHeaters)
			pheaters.Register_monitor(config)
		},
	)
}

const (
	cmd_INIT_TMC_help        = "Initialize TMC stepper driver registers"
	cmd_SET_TMC_FIELD_help   = "Set a register field of a TMC driver"
	cmd_SET_TMC_CURRENT_help = "Set the current of a TMC driver"
	cmd_DUMP_TMC_help        = "Read and display TMC stepper driver registers"
)

type tmcCommandStepperEnableAdapter struct {
	stepper_enable *mcupkg.PrinterStepperEnableModule
}

func (self *tmcCommandStepperEnableAdapter) Lookup_enable(name string) (tmcpkg.CommandEnableLine, error) {
	return self.stepper_enable.Lookup_enable(name)
}

type tmcCommandMutexAdapter struct {
	mutex *ReactorMutex
}

func (self *tmcCommandMutexAdapter) Lock() {
	self.mutex.Lock()
}

func (self *tmcCommandMutexAdapter) Unlock() {
	self.mutex.Unlock()
}

type tmcDriverCommandHelper struct {
	printer     *Printer
	stepperName string
	name        string
	runtime     *tmcpkg.CommandRuntime
}

func newTMCDriverCommandHelper(config *ConfigWrapper, mcuTMC tmcpkg.RegisterAccess, currentHelper tmcpkg.CurrentControl) *tmcDriverCommandHelper {
	self := &tmcDriverCommandHelper{}
	self.printer = config.Get_printer()
	nameParts := strings.Split(config.Get_name(), " ")
	self.stepperName = strings.Join(nameParts[1:], " ")
	self.name = str.LastName(config.Get_name())
	stepperEnable := self.printer.Load_object(config, "stepper_enable", object.Sentinel{}).(*mcupkg.PrinterStepperEnableModule)
	self.runtime = tmcpkg.NewCommandRuntime(
		self.stepperName,
		mcuTMC,
		currentHelper,
		newTMCErrorCheckRuntime(config, mcuTMC),
		&tmcCommandStepperEnableAdapter{stepper_enable: stepperEnable},
	)
	self.printer.Register_event_handler("stepper:sync_mcu_position",
		self.handle_sync_mcu_pos)
	self.printer.Register_event_handler("stepper:set_sdir_inverted",
		self.handle_sync_mcu_pos)
	self.printer.Register_event_handler("project:mcu_identify",
		self.handle_mcu_identify)
	self.printer.Register_event_handler("project:connect",
		self.handle_connect)
	_ = applyTMCMicrostepSettings(config, mcuTMC)

	gcode := MustLookupGcode(self.printer)
	gcode.Register_mux_command("SET_TMC_FIELD", "STEPPER", self.name,
		self.Cmd_SET_TMC_FIELD,
		cmd_SET_TMC_FIELD_help)
	gcode.Register_mux_command("INIT_TMC", "STEPPER", self.name,
		self.Cmd_INIT_TMC,
		cmd_INIT_TMC_help)
	gcode.Register_mux_command("SET_TMC_CURRENT", "STEPPER", self.name,
		self.Cmd_SET_TMC_CURRENT,
		cmd_SET_TMC_CURRENT_help)
	return self
}

func (self *tmcDriverCommandHelper) init_registers(printTime *float64) {
	_ = self.runtime.InitRegisters(printTime)
}

func (self *tmcDriverCommandHelper) Cmd_INIT_TMC(argv interface{}) error {
	logger.Infof("INIT_TMC %s", self.name)
	printTime := MustLookupToolhead(self.printer).Get_last_move_time()
	self.init_registers(cast.Float64P(printTime))
	return nil
}

func (self *tmcDriverCommandHelper) Cmd_SET_TMC_FIELD(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	fieldName := strings.ToLower(gcmd.Get("FIELD", object.Sentinel{}, "", nil, nil, nil, nil))
	fieldValue := gcmd.Get_int("VALUE", 0, nil, nil)
	printTime := MustLookupToolhead(self.printer).Get_last_move_time()
	return self.runtime.SetField(fieldName, int64(fieldValue), printTime)
}

func (self *tmcDriverCommandHelper) Cmd_SET_TMC_CURRENT(argv interface{}) error {
	current := self.runtime.GetCurrent()
	maxCur := current[3]

	gcmd := argv.(*GCodeCommand)
	runCurrentArg := gcmd.Get_floatP("CURRENT", nil, cast.Float64P(0.), cast.Float64P(maxCur), nil, nil)
	holdCurrentArg := gcmd.Get_floatP("HOLDCURRENT", nil, nil, cast.Float64P(maxCur),
		cast.Float64P(0.), nil)
	var runCurrent *float64
	var holdCurrent *float64
	if value.IsNotNone(runCurrentArg) {
		runCurrent = cast.Float64P(cast.Float64(runCurrentArg))
	}
	if value.IsNotNone(holdCurrentArg) {
		holdCurrent = cast.Float64P(cast.Float64(holdCurrentArg))
	}
	if value.IsNotNone(runCurrent) || value.IsNotNone(holdCurrent) {
		toolhead := MustLookupToolhead(self.printer)
		current = self.runtime.UpdateCurrent(runCurrent, holdCurrent, toolhead.Get_last_move_time())
	}

	if current[1] == -1 {
		gcmd.Respond_info(fmt.Sprintf("Run Current: %0.2fA", current[0]), true)
	} else {
		gcmd.Respond_info(fmt.Sprintf("Run Current: %0.2fA Hold Current: %0.2fA", current[0], current[1]), true)
	}
	return nil
}

func (self *tmcDriverCommandHelper) GetPhaseOffset() (*int, int) {
	return self.runtime.GetPhaseOffset()
}

func (self *tmcDriverCommandHelper) handle_sync_mcu_pos(argv []interface{}) error {
	stepper := argv[0].(*MCU_stepper)
	return self.runtime.HandleSyncMCUPos(stepper)
}

func (self *tmcDriverCommandHelper) do_enable(printTime *float64) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("tmcDriverCommandHelper.do_enable panic: %v %v", r, self.stepperName)
			self.printer.Invoke_shutdown(r)
		}
	}()
	if err := self.runtime.HandleEnable(MustLookupToolhead(self.printer), &tmcCommandMutexAdapter{mutex: MustLookupGcode(self.printer).Get_mutex()}); err != nil {
		panic(err)
	}
}

func (self *tmcDriverCommandHelper) do_disable(printTime *float64) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("tmcDriverCommandHelper.do_disable panic: %v", r)
			self.printer.Invoke_shutdown(r)
		}
	}()
	if err := self.runtime.HandleDisable(printTime); err != nil {
		panic(err)
	}
}

func (self *tmcDriverCommandHelper) handle_mcu_identify(argv []interface{}) error {
	forceMove, ok := self.printer.Lookup_object("force_move", object.Sentinel{}).(*motionpkg.ForceMoveModule)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "force_move", self.printer.Lookup_object("force_move", object.Sentinel{})))
	}
	return self.runtime.HandleMCUIdentify(func(name string) tmcpkg.CommandStepper {
		return forceMove.Lookup_stepper(name)
	})
}

func (self *tmcDriverCommandHelper) handle_stepper_enable(printTime float64, isEnable bool) {
	var cb func(interface{}) interface{}
	if isEnable {
		cb = func(ev interface{}) interface{} {
			self.do_enable(&printTime)
			return nil
		}
	} else {
		cb = func(ev interface{}) interface{} {
			self.do_disable(&printTime)
			return nil
		}
	}
	self.printer.Get_reactor().Register_callback(cb, constants.NOW)
}

func (self *tmcDriverCommandHelper) handle_connect(argv []interface{}) error {
	return self.runtime.HandleConnect(self.handle_stepper_enable)
}

func (self *tmcDriverCommandHelper) GetStatus(eventtime float64) map[string]interface{} {
	return self.runtime.GetStatus(eventtime)
}

func (self *tmcDriverCommandHelper) SetupRegisterDump(readRegisters []string, readTranslate func(string, int64) (string, int64)) {
	self.runtime.SetupRegisterDump(readRegisters, readTranslate)
	gcode := MustLookupGcode(self.printer)
	gcode.Register_mux_command("DUMP_TMC", "STEPPER", self.name, self.Cmd_DUMP_TMC, cmd_DUMP_TMC_help)
}

func (self *tmcDriverCommandHelper) Cmd_DUMP_TMC(argv interface{}) error {
	logger.Debugf("DUMP_TMC %s", self.name)
	gcmd := argv.(*GCodeCommand)
	regName := gcmd.Get("REGISTER", nil, "", nil, nil, nil, nil)
	if regName != "" {
		regName = strings.ToUpper(regName)
		line, err := self.runtime.DumpRegister(regName)
		if err != nil {
			panic(err)
		}
		gcmd.Respond_info(line, true)
	} else {
		lines, err := self.runtime.DumpAllRegisters()
		if err != nil {
			panic(err)
		}
		for i, line := range lines {
			gcmd.Respond_info(line, i == 0 || i == len(lines)-1)
		}
	}
	return nil
}

type tmcVirtualPinHelper struct {
	printer    *Printer
	runtime    *tmcpkg.VirtualPinRuntime
	mcuEndstop interface{}
}

type tmcVirtualPinEventRuntime struct {
	helper *tmcVirtualPinHelper
}

func (self *tmcVirtualPinEventRuntime) MatchesHomingMoveEndstop(endstop interface{}) bool {
	return self.helper.mcuEndstop == endstop
}

func (self *tmcVirtualPinEventRuntime) BeginMoveHoming() error {
	return self.helper.runtime.BeginMoveHoming()
}

func (self *tmcVirtualPinEventRuntime) EndMoveHoming() error {
	return self.helper.runtime.EndMoveHoming()
}

func (self *tmcVirtualPinEventRuntime) BeginHoming() error {
	return self.helper.runtime.BeginHoming()
}

func (self *tmcVirtualPinEventRuntime) EndHoming() error {
	return self.helper.runtime.EndHoming()
}

func newTMCVirtualPinHelper(config *ConfigWrapper, mcuTMC tmcpkg.RegisterAccess) *tmcVirtualPinHelper {
	self := &tmcVirtualPinHelper{}
	self.printer = config.Get_printer()
	self.runtime = tmcpkg.NewVirtualPinRuntime(config, mcuTMC)
	ppins := MustLookupPins(self.printer)
	ppins.Register_chip(self.runtime.ChipName(config), self)
	return self
}

func (self *tmcVirtualPinHelper) Setup_pin(pinType string, pinParams map[string]interface{}) interface{} {
	ppins := MustLookupPins(self.printer)
	if pinType != "endstop" || pinParams["pin"] != "virtual_endstop" {
		return errors.New("tmc virtual endstop only useful as endstop")
	}
	if pinParams["invert"] == true || pinParams["pullup"] == true {
		return errors.New("Can not pullup/invert tmc virtual pin")
	}
	if value.IsNone(self.runtime.DiagPin()) {
		return errors.New("tmc virtual endstop requires diag pin config")
	}

	self.printer.Register_event_handler("homing:homing_move_begin",
		self.handle_homing_move_begin)
	self.printer.Register_event_handler("homing:homing_move_end",
		self.handle_homing_move_end)
	self.printer.Register_event_handler("homing:homing_begin",
		self.handle_homing_begin)
	self.printer.Register_event_handler("homing:homing_end",
		self.handle_homing_end)
	self.mcuEndstop = ppins.Setup_pin("endstop", cast.ToString(self.runtime.DiagPin()))
	return self.mcuEndstop
}

func (self *tmcVirtualPinHelper) handle_homing_move_begin(argv []interface{}) error {
	hmove := argv[0].(*HomingMove)
	return tmcpkg.HandleVirtualPinHomingMoveBegin(&tmcVirtualPinEventRuntime{helper: self}, tmcHomingMoveEndstops(hmove))
}

func (self *tmcVirtualPinHelper) handle_homing_move_end(argv []interface{}) error {
	hmove := argv[0].(*HomingMove)
	return tmcpkg.HandleVirtualPinHomingMoveEnd(&tmcVirtualPinEventRuntime{helper: self}, tmcHomingMoveEndstops(hmove))
}

func (self *tmcVirtualPinHelper) handle_homing_begin(argv []interface{}) error {
	return tmcpkg.HandleVirtualPinHomingBegin(&tmcVirtualPinEventRuntime{helper: self})
}

func (self *tmcVirtualPinHelper) handle_homing_end(argv []interface{}) error {
	return tmcpkg.HandleVirtualPinHomingEnd(&tmcVirtualPinEventRuntime{helper: self})
}

func tmcHomingMoveEndstops(move *HomingMove) []interface{} {
	endstops := make([]interface{}, 0, len(move.Endstops))
	for _, namedEndstop := range move.Endstops {
		if namedEndstop.Front() == nil {
			continue
		}
		endstops = append(endstops, namedEndstop.Front().Value)
	}
	return endstops
}

func applyTMCMicrostepSettings(config *ConfigWrapper, mcuTMC tmcpkg.RegisterAccess) error {
	fields := mcuTMC.Get_fields()
	stepperName := strings.Join(strings.Split(config.Get_name(), " ")[1:], " ")
	if !config.Has_section(stepperName) {
		return fmt.Errorf("Could not find config section '[%s]' required by tmc driver", stepperName)
	}

	stepperConfig := config.Getsection(stepperName)
	microstepConfig := config.Getsection(stepperName)
	if value.IsNone(stepperConfig.Get("microsteps", value.None, false)) &&
		value.IsNotNone(config.Get("microsteps", value.None, false)) {
		microstepConfig = config
	}

	steps := map[interface{}]interface{}{256: 0, 128: 1, 64: 2, 32: 3, 16: 4, 8: 5, 4: 6, 2: 7, 1: 8}
	mres := microstepConfig.Getchoice("microsteps", steps, nil, true)
	tmcpkg.ApplyMicrostepSettings(fields, mres, config.Getboolean("interpolate", true, true))
	return nil
}

func applyTMCStealthchop(config *ConfigWrapper, mcuTMC tmcpkg.RegisterAccess, tmcFrequency float64) {
	fields := mcuTMC.Get_fields()
	velocity := config.Getfloat("stealthchop_threshold", math.NaN(), 0., 0, 0, 0, true)
	if !math.IsNaN(velocity) {
		stepperName := strings.Join(strings.Split(config.Get_name(), " ")[1:], " ")
		stepperConfig := config.Getsection(stepperName)
		rotationDist, stepsPerRotation := kinematicspkg.ParseStepperDistance(stepperConfig, nil, true)
		stepDist := rotationDist / float64(stepsPerRotation)
		tmcpkg.ApplyStealthchop(fields, tmcFrequency, stepDist, velocity)
		return
	}
	tmcpkg.ApplyStealthchop(fields, tmcFrequency, 0, math.NaN())
}

type tmcDriverAdapter struct{}

func unwrapTMCDriverConfig(config tmcpkg.DriverConfig) *ConfigWrapper {
	typed, ok := config.(*ConfigWrapper)
	if !ok {
		panic(fmt.Sprintf("unexpected TMC driver config type %T", config))
	}
	return typed
}

func (tmcDriverAdapter) NewUART(config tmcpkg.DriverConfig, nameToReg map[string]int64, fields *tmcpkg.FieldHelper, maxAddr int64, tmcFrequency float64) tmcpkg.RegisterAccess {
	return NewMCU_TMC_uart(unwrapTMCDriverConfig(config), nameToReg, fields, maxAddr, tmcFrequency)
}

func (tmcDriverAdapter) NewSPI(config tmcpkg.DriverConfig, nameToReg map[string]int64, fields *tmcpkg.FieldHelper) tmcpkg.RegisterAccess {
	return NewMCU_TMC_SPI(unwrapTMCDriverConfig(config), nameToReg, fields)
}

func (tmcDriverAdapter) NewTMC2660SPI(config tmcpkg.DriverConfig, nameToReg map[string]int64, fields *tmcpkg.FieldHelper) tmcpkg.RegisterAccess {
	return NewMCU_TMC2660_SPI(unwrapTMCDriverConfig(config), nameToReg, fields)
}

func (tmcDriverAdapter) AttachVirtualPin(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess) {
	newTMCVirtualPinHelper(unwrapTMCDriverConfig(config), mcuTMC)
}

func (tmcDriverAdapter) NewCommandHelper(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess, currentHelper tmcpkg.CurrentControl) tmcpkg.DriverCommandHelper {
	return newTMCDriverCommandHelper(unwrapTMCDriverConfig(config), mcuTMC, currentHelper)
}

func (tmcDriverAdapter) ApplyStealthchop(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess, tmcFrequency float64) {
	applyTMCStealthchop(unwrapTMCDriverConfig(config), mcuTMC, tmcFrequency)
}

func (tmcDriverAdapter) NewTMC2660CurrentHelper(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess) tmcpkg.CurrentControl {
	cfg := unwrapTMCDriverConfig(config)
	printer := cfg.Get_printer()
	return tmcpkg.NewTMC2660CurrentHelper(
		cfg,
		mcuTMC,
		func(event string, callback func([]interface{}) error) {
			printer.Register_event_handler(event, callback)
		},
		func(callback func(interface{}) interface{}, eventtime float64) {
			printer.Get_reactor().Register_callback(callback, eventtime)
		},
	)
}

var projectTMCDriverAdapter tmcpkg.DriverAdapter = tmcDriverAdapter{}

func Load_config_TMC2209(config *ConfigWrapper) interface{} {
	return tmcpkg.LoadConfigTMC2209(config, projectTMCDriverAdapter)
}
func Load_config_TMC2240(config *ConfigWrapper) interface{} {
	return tmcpkg.LoadConfigTMC2240(config, projectTMCDriverAdapter)
}
