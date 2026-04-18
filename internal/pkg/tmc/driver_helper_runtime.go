package tmc

import (
	"errors"
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	"goklipper/common/value"
	"strings"
	"sync"
	"time"
)

const (
	cmd_INIT_TMC_help        = "Initialize TMC stepper driver registers"
	cmd_SET_TMC_FIELD_help   = "Set a register field of a TMC driver"
	cmd_SET_TMC_CURRENT_help = "Set the current of a TMC driver"
	cmd_DUMP_TMC_help        = "Read and display TMC stepper driver registers"
)

var (
	virtualPinHomingSequenceLock sync.Mutex
	virtualPinHomingActiveCount  int
)

func beginVirtualPinHomingSequence() bool {
	virtualPinHomingSequenceLock.Lock()
	defer virtualPinHomingSequenceLock.Unlock()
	delayNeeded := virtualPinHomingActiveCount > 0
	virtualPinHomingActiveCount++
	return delayNeeded
}

func endVirtualPinHomingSequence() {
	virtualPinHomingSequenceLock.Lock()
	defer virtualPinHomingSequenceLock.Unlock()
	if virtualPinHomingActiveCount > 0 {
		virtualPinHomingActiveCount--
	}
}

type DriverErrorCheckOptions struct {
	Reactor         ErrorCheckReactor
	Shutdown        func(string)
	RegisterMonitor func()
}

func NewDriverErrorCheckRuntime(config DriverConfig, mcuTMC RegisterAccess, options DriverErrorCheckOptions) *ErrorCheckRuntime {
	nameParts := strings.Split(config.Get_name(), " ")
	return NewErrorCheckRuntime(
		nameParts[0],
		strings.Join(nameParts[1:], " "),
		mcuTMC,
		options.Reactor,
		options.Shutdown,
		options.RegisterMonitor,
	)
}

type DriverCommandInput interface {
	Get(name string, defaultValue interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	Get_int(name string, defaultValue interface{}, minval *int, maxval *int) int
	Get_floatP(name string, defaultValue *float64, minval *float64, maxval *float64, above *float64, below *float64) *float64
	RespondInfo(msg string, log bool)
}

type CommandMuxRegistrar func(cmd string, key string, value string, handler func(interface{}) error, desc string)
type CommandToolheadLookup func() CommandToolhead
type CommandMutexLookup func() CommandMutex
type PanicShutdownHandler func(interface{}) interface{}

type DriverCommandHelperOptions struct {
	StatusChecker      CommandStatusChecker
	StepperEnable      CommandStepperEnable
	RegisterEvent      EventRegistrar
	RegisterMuxCommand CommandMuxRegistrar
	LookupToolhead     CommandToolheadLookup
	LookupStepper      CommandStepperLookup
	LookupMutex        CommandMutexLookup
	ScheduleCallback   ReactorCallbackRegistrar
	Shutdown           PanicShutdownHandler
}

type DriverCommandHelperRuntime struct {
	stepperName string
	name        string
	runtime     *CommandRuntime
	options     DriverCommandHelperOptions
	homingMutex sync.Mutex
	homingDepth int
}

type driverCommandTimedToolhead interface {
	CommandToolhead
	EstimatedPrintTime(eventtime float64) float64
	Monotonic() float64
	Pause(waketime float64) float64
}

const driverCommandPrintTimePollInterval = .001
const driverCommandEnableRetryDelay = .050
const driverCommandEnableRetryCount = 7
const driverCommandEnableRetryDelayHoming = .100
const driverCommandEnableRetryCountHoming = 20

var _ DriverCommandHelper = (*DriverCommandHelperRuntime)(nil)

func NewDriverCommandHelper(config DriverConfig, mcuTMC RegisterAccess, currentHelper CurrentControl, options DriverCommandHelperOptions) *DriverCommandHelperRuntime {
	nameParts := strings.Split(config.Get_name(), " ")
	self := &DriverCommandHelperRuntime{
		stepperName: strings.Join(nameParts[1:], " "),
		name:        str.LastName(config.Get_name()),
		options:     options,
	}
	self.runtime = NewCommandRuntime(self.stepperName, mcuTMC, currentHelper, options.StatusChecker, options.StepperEnable)
	if options.RegisterEvent != nil {
		options.RegisterEvent("stepper:sync_mcu_position", self.handle_sync_mcu_pos)
		options.RegisterEvent("stepper:set_dir_inverted", self.handle_sync_mcu_pos)
		options.RegisterEvent("project:mcu_identify", self.handle_mcu_identify)
		options.RegisterEvent("project:connect", self.handle_connect)
		// Suppress DRV_STATUS failure counting while any homing move is active.
		// The shared bitbang UART is saturated by stall-detection writes for all
		// drivers during homing, causing periodic poll failures that are not real
		// driver errors. These handlers apply to all drivers regardless of which
		// endstop is in use because the UART bus is shared.
		options.RegisterEvent("homing:homing_move_begin", self.handle_homing_move_begin)
		options.RegisterEvent("homing:homing_move_end", self.handle_homing_move_end)
	}
	if options.RegisterMuxCommand != nil {
		options.RegisterMuxCommand("SET_TMC_FIELD", "STEPPER", self.name, self.Cmd_SET_TMC_FIELD, cmd_SET_TMC_FIELD_help)
		options.RegisterMuxCommand("INIT_TMC", "STEPPER", self.name, self.Cmd_INIT_TMC, cmd_INIT_TMC_help)
		options.RegisterMuxCommand("SET_TMC_CURRENT", "STEPPER", self.name, self.Cmd_SET_TMC_CURRENT, cmd_SET_TMC_CURRENT_help)
	}
	return self
}

func (self *DriverCommandHelperRuntime) commandInput(argv interface{}) (DriverCommandInput, error) {
	gcmd, ok := argv.(DriverCommandInput)
	if !ok {
		return nil, fmt.Errorf("unexpected TMC command input type %T", argv)
	}
	return gcmd, nil
}

func (self *DriverCommandHelperRuntime) currentToolhead() (CommandToolhead, error) {
	if self.options.LookupToolhead == nil {
		return nil, fmt.Errorf("TMC toolhead lookup not configured for %s", self.stepperName)
	}
	toolhead := self.options.LookupToolhead()
	if toolhead == nil {
		return nil, fmt.Errorf("TMC toolhead not found for %s", self.stepperName)
	}
	return toolhead, nil
}

func (self *DriverCommandHelperRuntime) currentMutex() CommandMutex {
	if self.options.LookupMutex == nil {
		return CommandMutexFuncs{}
	}
	mutex := self.options.LookupMutex()
	if mutex == nil {
		return CommandMutexFuncs{}
	}
	return mutex
}

func (self *DriverCommandHelperRuntime) init_registers(printTime *float64) {
	_ = self.runtime.InitRegisters(printTime)
}

func normalizeDriverCommandPanic(reason interface{}) string {
	if err, ok := reason.(error); ok {
		return err.Error()
	}
	return fmt.Sprint(reason)
}

func isDriverCommandTransientEnableError(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "Unable to write tmc uart ")
}

func pauseDriverCommandToolhead(toolhead CommandToolhead, delay float64) {
	timedToolhead, ok := toolhead.(driverCommandTimedToolhead)
	if !ok {
		return
	}
	eventtime := timedToolhead.Monotonic()
	timedToolhead.Pause(eventtime + delay)
}

func synchronizeDriverCommandPrintTimeWithDelay(toolhead CommandToolhead, printTime *float64, delay float64) *float64 {
	if printTime == nil || toolhead == nil {
		return printTime
	}

	targetPrintTime := *printTime + delay

	// TMC UART register writes are immediately read back via IFCNT / MSCNT.
	// When the enable callback is scheduled ahead of the actual print time,
	// wait for that print time to arrive and then issue the register writes
	// immediately instead of attaching the future timestamp to the write.
	timedToolhead, ok := toolhead.(driverCommandTimedToolhead)
	if !ok {
		toolhead.Wait_moves()
		return nil
	}

	for {
		eventtime := timedToolhead.Monotonic()
		if timedToolhead.EstimatedPrintTime(eventtime) >= targetPrintTime {
			return nil
		}
		timedToolhead.Pause(eventtime + driverCommandPrintTimePollInterval)
	}
}

func synchronizeDriverCommandPrintTime(toolhead CommandToolhead, printTime *float64) *float64 {
	return synchronizeDriverCommandPrintTimeWithDelay(toolhead, printTime, 0)
}

func estimateDriverCommandWakeTime(toolhead CommandToolhead, printTime *float64, delay float64) float64 {
	if printTime == nil || toolhead == nil {
		return constants.NOW
	}

	timedToolhead, ok := toolhead.(driverCommandTimedToolhead)
	if !ok {
		return constants.NOW
	}

	eventtime := timedToolhead.Monotonic()
	targetPrintTime := *printTime + delay
	remaining := targetPrintTime - timedToolhead.EstimatedPrintTime(eventtime)
	if remaining <= 0 {
		return constants.NOW
	}
	return eventtime + remaining
}

func (self *DriverCommandHelperRuntime) Cmd_INIT_TMC(argv interface{}) error {
	logger.Infof("INIT_TMC %s", self.name)
	toolhead, err := self.currentToolhead()
	if err != nil {
		return err
	}
	printTime := toolhead.Get_last_move_time()
	self.init_registers(cast.Float64P(printTime))
	return nil
}

func (self *DriverCommandHelperRuntime) Cmd_SET_TMC_FIELD(argv interface{}) error {
	gcmd, err := self.commandInput(argv)
	if err != nil {
		return err
	}
	fieldName := strings.ToLower(gcmd.Get("FIELD", object.Sentinel{}, "", nil, nil, nil, nil))
	fieldValue := gcmd.Get_int("VALUE", 0, nil, nil)
	toolhead, err := self.currentToolhead()
	if err != nil {
		return err
	}
	printTime := toolhead.Get_last_move_time()
	return self.runtime.SetField(fieldName, int64(fieldValue), printTime)
}

func (self *DriverCommandHelperRuntime) Cmd_SET_TMC_CURRENT(argv interface{}) error {
	gcmd, err := self.commandInput(argv)
	if err != nil {
		return err
	}
	current := self.runtime.GetCurrent()
	maxCur := current[3]

	runCurrentArg := gcmd.Get_floatP("CURRENT", nil, cast.Float64P(0.), cast.Float64P(maxCur), nil, nil)
	holdCurrentArg := gcmd.Get_floatP("HOLDCURRENT", nil, nil, cast.Float64P(maxCur), cast.Float64P(0.), nil)
	var runCurrent *float64
	var holdCurrent *float64
	if value.IsNotNone(runCurrentArg) {
		runCurrent = cast.Float64P(cast.Float64(runCurrentArg))
	}
	if value.IsNotNone(holdCurrentArg) {
		holdCurrent = cast.Float64P(cast.Float64(holdCurrentArg))
	}
	if value.IsNotNone(runCurrent) || value.IsNotNone(holdCurrent) {
		toolhead, toolheadErr := self.currentToolhead()
		if toolheadErr != nil {
			return toolheadErr
		}
		current = self.runtime.UpdateCurrent(runCurrent, holdCurrent, toolhead.Get_last_move_time())
	}

	if current[1] == -1 {
		gcmd.RespondInfo(fmt.Sprintf("Run Current: %0.2fA", current[0]), true)
	} else {
		gcmd.RespondInfo(fmt.Sprintf("Run Current: %0.2fA Hold Current: %0.2fA", current[0], current[1]), true)
	}
	return nil
}

func (self *DriverCommandHelperRuntime) GetPhaseOffset() (*int, int) {
	return self.runtime.GetPhaseOffset()
}

func (self *DriverCommandHelperRuntime) handle_sync_mcu_pos(argv []interface{}) error {
	stepper, ok := argv[0].(CommandStepper)
	if !ok {
		return fmt.Errorf("unexpected sync stepper type %T", argv[0])
	}
	var pauseRetry func()
	if toolhead, err := self.currentToolhead(); err == nil {
		pauseRetry = func() {
			pauseDriverCommandToolhead(toolhead, commandRuntimePhaseQueryRetryDelay)
		}
	}
	return self.runtime.handleSyncMCUPos(stepper, pauseRetry)
}

func (self *DriverCommandHelperRuntime) do_enable(printTime *float64) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("DriverCommandHelperRuntime.do_enable panic: %v %v", r, self.stepperName)
			if self.options.Shutdown != nil {
				_ = self.options.Shutdown(normalizeDriverCommandPanic(r))
			}
		}
	}()
	toolhead, err := self.currentToolhead()
	if err != nil {
		panic(err)
	}
	applyPrintTime := synchronizeDriverCommandPrintTime(toolhead, printTime)
	retryCount := driverCommandEnableRetryCount
	retryDelay := driverCommandEnableRetryDelay
	if self.runtime.IsHomingActive() {
		retryCount = driverCommandEnableRetryCountHoming
		retryDelay = driverCommandEnableRetryDelayHoming
	}
	for attempt := 0; attempt < retryCount; attempt++ {
		err = self.runtime.HandleEnable(toolhead, self.currentMutex(), applyPrintTime)
		if err == nil {
			return
		}
		if !isDriverCommandTransientEnableError(err) || attempt == retryCount-1 {
			panic(err)
		}
		pauseDriverCommandToolhead(toolhead, retryDelay)
	}
}

func (self *DriverCommandHelperRuntime) do_disable(printTime *float64) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("DriverCommandHelperRuntime.do_disable panic: %v", r)
			if self.options.Shutdown != nil {
				_ = self.options.Shutdown(normalizeDriverCommandPanic(r))
			}
		}
	}()
	var applyPrintTime *float64
	if printTime != nil {
		toolhead, err := self.currentToolhead()
		if err != nil {
			panic(err)
		}
		applyPrintTime = synchronizeDriverCommandPrintTime(toolhead, printTime)
	}
	if err := self.runtime.HandleDisable(applyPrintTime); err != nil {
		panic(err)
	}
}

func (self *DriverCommandHelperRuntime) handle_mcu_identify(argv []interface{}) error {
	if self.options.LookupStepper == nil {
		return fmt.Errorf("TMC stepper lookup not configured for %s", self.stepperName)
	}
	return self.runtime.HandleMCUIdentify(self.options.LookupStepper)
}

func (self *DriverCommandHelperRuntime) handle_stepper_enable(printTime float64, isEnable bool) {
	callback := func(interface{}) interface{} {
		if isEnable {
			self.do_enable(&printTime)
		} else {
			self.do_disable(&printTime)
		}
		return nil
	}
	if self.options.ScheduleCallback != nil {
		self.options.ScheduleCallback(callback, constants.NOW)
		return
	}
	callback(nil)
}

func (self *DriverCommandHelperRuntime) handle_connect(argv []interface{}) error {
	return self.runtime.HandleConnect(self.handle_stepper_enable)
}

func (self *DriverCommandHelperRuntime) handle_homing_move_begin(argv []interface{}) error {
	self.homingMutex.Lock()
	defer self.homingMutex.Unlock()
	self.homingDepth++
	if self.homingDepth == 1 {
		self.runtime.SetHomingActive(true)
	}
	return nil
}

func (self *DriverCommandHelperRuntime) handle_homing_move_end(argv []interface{}) error {
	self.homingMutex.Lock()
	defer self.homingMutex.Unlock()
	if self.homingDepth == 0 {
		return nil
	}
	self.homingDepth--
	if self.homingDepth == 0 {
		self.runtime.SetHomingActive(false)
	}
	return nil
}

func (self *DriverCommandHelperRuntime) GetStatus(eventtime float64) map[string]interface{} {
	return self.runtime.GetStatus(eventtime)
}

func (self *DriverCommandHelperRuntime) SetupRegisterDump(readRegisters []string, readTranslate func(string, int64) (string, int64)) {
	self.runtime.SetupRegisterDump(readRegisters, readTranslate)
	if self.options.RegisterMuxCommand != nil {
		self.options.RegisterMuxCommand("DUMP_TMC", "STEPPER", self.name, self.Cmd_DUMP_TMC, cmd_DUMP_TMC_help)
	}
}

func (self *DriverCommandHelperRuntime) Cmd_DUMP_TMC(argv interface{}) error {
	logger.Debugf("DUMP_TMC %s", self.name)
	gcmd, err := self.commandInput(argv)
	if err != nil {
		return err
	}
	regName := gcmd.Get("REGISTER", nil, "", nil, nil, nil, nil)
	if regName != "" {
		regName = strings.ToUpper(regName)
		line, err := self.runtime.DumpRegister(regName)
		if err != nil {
			gcmd.RespondInfo(fmt.Sprintf("DUMP_TMC %s failed: %s", self.name, err.Error()), true)
			return nil
		}
		gcmd.RespondInfo(line, true)
	} else {
		lines, err := self.runtime.DumpAllRegisters()
		if err != nil {
			gcmd.RespondInfo(fmt.Sprintf("DUMP_TMC %s failed: %s", self.name, err.Error()), true)
			return nil
		}
		for i, line := range lines {
			gcmd.RespondInfo(line, i == 0 || i == len(lines)-1)
		}
	}
	return nil
}

type DriverVirtualPinSetupFunc func(pinType string, pin string) interface{}
type DriverVirtualPinMoveEndstops func(move interface{}) []interface{}
type DriverVirtualPinChipRegistrar func(name string, chip interface{})

type DriverVirtualPinHelperOptions struct {
	RegisterChip              DriverVirtualPinChipRegistrar
	RegisterEvent             EventRegistrar
	SetupPin                  DriverVirtualPinSetupFunc
	ExtractHomingMoveEndstops DriverVirtualPinMoveEndstops
	// ReactorPause and ReactorMonotonic allow the homing handler to yield
	// the reactor instead of blocking with time.Sleep. Must be provided to
	// avoid stalling motion planning during the inter-driver UART delay.
	ReactorPause     func(waketime float64) float64
	ReactorMonotonic func() float64
}

type DriverVirtualPinHelper struct {
	runtime          *VirtualPinRuntime
	registerEvent    EventRegistrar
	setupPin         DriverVirtualPinSetupFunc
	extractEndstops  DriverVirtualPinMoveEndstops
	reactorPause     func(waketime float64) float64
	reactorMonotonic func() float64
	mcuEndstop       interface{}
}

type driverVirtualPinEventRuntime struct {
	helper *DriverVirtualPinHelper
}

func (self *driverVirtualPinEventRuntime) MatchesHomingMoveEndstop(endstop interface{}) bool {
	return self.helper.mcuEndstop == endstop
}

func (self *driverVirtualPinEventRuntime) BeginMoveHoming() error {
	return self.helper.runtime.BeginMoveHoming()
}

func (self *driverVirtualPinEventRuntime) EndMoveHoming() error {
	return self.helper.runtime.EndMoveHoming()
}

func NewDriverVirtualPinHelper(config VirtualPinConfig, mcuTMC RegisterAccess, options DriverVirtualPinHelperOptions) *DriverVirtualPinHelper {
	self := &DriverVirtualPinHelper{
		runtime:          NewVirtualPinRuntime(config, mcuTMC),
		registerEvent:    options.RegisterEvent,
		setupPin:         options.SetupPin,
		extractEndstops:  options.ExtractHomingMoveEndstops,
		reactorPause:     options.ReactorPause,
		reactorMonotonic: options.ReactorMonotonic,
	}
	if options.RegisterChip != nil {
		options.RegisterChip(self.runtime.ChipName(config), self)
	}
	return self
}

func (self *DriverVirtualPinHelper) Setup_pin(pinType string, pinParams map[string]interface{}) interface{} {
	if pinType != "endstop" || pinParams["pin"] != "virtual_endstop" {
		return errors.New("tmc virtual endstop only useful as endstop")
	}
	if pinParams["invert"] == true || pinParams["pullup"] == true {
		return errors.New("Can not pullup/invert tmc virtual pin")
	}
	if value.IsNone(self.runtime.DiagPin()) {
		return errors.New("tmc virtual endstop requires diag pin config")
	}
	if self.setupPin == nil {
		return errors.New("tmc virtual endstop requires pin setup adapter")
	}

	if self.registerEvent != nil {
		self.registerEvent("homing:homing_move_begin", self.handle_homing_move_begin)
		self.registerEvent("homing:homing_move_end", self.handle_homing_move_end)
	}
	self.mcuEndstop = self.setupPin("endstop", cast.ToString(self.runtime.DiagPin()))
	return self.mcuEndstop
}

func (self *DriverVirtualPinHelper) moveEndstops(argv []interface{}) []interface{} {
	if len(argv) == 0 || self.extractEndstops == nil {
		return nil
	}
	return self.extractEndstops(argv[0])
}

func (self *DriverVirtualPinHelper) handle_homing_move_begin(argv []interface{}) error {
	runtime := &driverVirtualPinEventRuntime{helper: self}
	endstops := self.moveEndstops(argv)
	if !hasMatchingVirtualPinEndstop(endstops, runtime.MatchesHomingMoveEndstop) {
		return nil
	}

	// Serialize virtual pin homing setup only across the helpers actually taking
	// part in this homing move. Matching helpers share one UART and write GCONF,
	// thresholds, and other registers in rapid succession, so delay the 2nd/3rd
	// participants while leaving unrelated helpers on the fast path.
	delayNeeded := beginVirtualPinHomingSequence()

	// Add delay for 2nd and 3rd motors to prevent UART collisions.
	// Use reactor.Pause so the event loop keeps running (timers, MCU
	// messages, motion planning) during the inter-driver gap.
	if delayNeeded {
		if self.reactorPause != nil && self.reactorMonotonic != nil {
			self.reactorPause(self.reactorMonotonic() + 0.075)
		} else {
			time.Sleep(75 * time.Millisecond)
		}
	}

	if err := runtime.BeginMoveHoming(); err != nil {
		endVirtualPinHomingSequence()
		return err
	}
	return nil
}

func (self *DriverVirtualPinHelper) handle_homing_move_end(argv []interface{}) error {
	runtime := &driverVirtualPinEventRuntime{helper: self}
	endstops := self.moveEndstops(argv)
	if !hasMatchingVirtualPinEndstop(endstops, runtime.MatchesHomingMoveEndstop) {
		return nil
	}
	defer endVirtualPinHomingSequence()
	return runtime.EndMoveHoming()
}
