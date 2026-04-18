package tmc

import (
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/maths"
	"goklipper/common/value"
	"strings"
)

const (
	commandRuntimePhaseQueryRetryCount = 3
	commandRuntimePhaseQueryRetryDelay = .050
)

type CommandStatusChecker interface {
	StartChecks() bool
	StopChecks()
	GetStatus(eventtime float64) map[string]interface{}
}

type CommandStepper interface {
	Get_name(short bool) string
	Setup_default_pulse_duration(pulseduration interface{}, step_both_edge bool)
	Get_pulse_duration() (interface{}, bool)
	Mcu_to_commanded_position(mcuPos int) float64
	Get_dir_inverted() (uint32, uint32)
	Get_mcu_position() int
}

type CommandToolhead interface {
	Get_last_move_time() float64
	Wait_moves()
}

type CommandMutex interface {
	Lock()
	Unlock()
}

type CommandMutexFuncs struct {
	LockFunc   func()
	UnlockFunc func()
}

func (funcs CommandMutexFuncs) Lock() {
	funcs.LockFunc()
}

func (funcs CommandMutexFuncs) Unlock() {
	funcs.UnlockFunc()
}

type CommandEnableLine interface {
	Register_state_callback(callback func(float64, bool))
	Is_motor_enabled() bool
	Has_dedicated_enable() bool
}

type CommandStepperEnable interface {
	Lookup_enable(name string) (CommandEnableLine, error)
}

type CommandStepperEnableLookupFunc func(name string) (CommandEnableLine, error)

func (fn CommandStepperEnableLookupFunc) Lookup_enable(name string) (CommandEnableLine, error) {
	return fn(name)
}

type CommandStepperLookup func(name string) CommandStepper

type noopCommandStatusChecker struct{}

func (noopCommandStatusChecker) StartChecks() bool { return false }
func (noopCommandStatusChecker) StopChecks()       {}
func (noopCommandStatusChecker) GetStatus(eventtime float64) map[string]interface{} {
	_ = eventtime
	return map[string]interface{}{"drv_status": nil}
}

type CommandRuntime struct {
	stepperName   string
	core          *CommandHelperCore
	currentHelper CurrentControl
	statusChecker CommandStatusChecker
	stepperEnable CommandStepperEnable
	stepper       CommandStepper
	homingActive  bool
}

func NewCommandRuntime(stepperName string, mcuTMC RegisterAccess, currentHelper CurrentControl, statusChecker CommandStatusChecker, stepperEnable CommandStepperEnable) *CommandRuntime {
	if statusChecker == nil {
		statusChecker = noopCommandStatusChecker{}
	}
	return &CommandRuntime{
		stepperName:   stepperName,
		core:          NewCommandHelperCore(mcuTMC, currentHelper),
		currentHelper: currentHelper,
		statusChecker: statusChecker,
		stepperEnable: stepperEnable,
	}
}

func isCommandRuntimeTransientPhaseQueryError(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "Unable to read tmc uart ")
}

func (self *CommandRuntime) queryPhaseWithRetry(pauseRetry func()) (int64, error) {
	var lastErr error
	for attempt := 0; attempt < commandRuntimePhaseQueryRetryCount; attempt++ {
		driverPhase, err := self.core.QueryPhase()
		if err == nil {
			return driverPhase, nil
		}
		lastErr = err
		if !isCommandRuntimeTransientPhaseQueryError(err) || attempt == commandRuntimePhaseQueryRetryCount-1 {
			return 0, err
		}
		if pauseRetry != nil {
			pauseRetry()
		}
	}
	return 0, lastErr
}

func (self *CommandRuntime) InitRegisters(printTime *float64) error {
	return self.core.InitRegisters(printTime)
}

func (self *CommandRuntime) SetField(fieldName string, fieldValue int64, printTime float64) error {
	return self.core.SetField(fieldName, fieldValue, printTime)
}

func (self *CommandRuntime) GetCurrent() []float64 {
	return self.currentHelper.Get_current()
}

func (self *CommandRuntime) UpdateCurrent(runCurrent, holdCurrent *float64, printTime float64) []float64 {
	return self.core.UpdateCurrent(runCurrent, holdCurrent, printTime)
}

func (self *CommandRuntime) GetPhaseOffset() (*int, int) {
	return self.core.GetPhaseOffset()
}

func (self *CommandRuntime) SetupRegisterDump(readRegisters []string, readTranslate func(string, int64) (string, int64)) {
	self.core.SetupRegisterDump(readRegisters, readTranslate)
}

func (self *CommandRuntime) DumpRegister(regName string) (string, error) {
	return self.core.DumpRegister(regName)
}

func (self *CommandRuntime) DumpAllRegisters() ([]string, error) {
	return self.core.DumpAllRegisters()
}

func (self *CommandRuntime) HandleSyncMCUPos(stepper CommandStepper) error {
	return self.handleSyncMCUPos(stepper, nil)
}

func (self *CommandRuntime) handleSyncMCUPos(stepper CommandStepper, pauseRetry func()) error {
	if stepper.Get_name(false) != self.stepperName {
		return nil
	}
	if self.homingActive {
		// Homing runs sensorless threshold changes and other UART-heavy traffic.
		// Skip phase-sync reads in this window to avoid adding read-retry/pause
		// pressure that can delay motion scheduling and increase noise.
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			panicErr, ok := r.(error)
			if !ok {
				panicErr = fmt.Errorf("%v", r)
			}
			logger.Infof("Unable to obtain tmc %s phase", self.stepperName)
			self.core.ClearPhaseOffset()
			if self.homingActive && isCommandRuntimeTransientPhaseQueryError(panicErr) {
				logger.Warnf("Suppressing transient tmc %s phase-sync UART read failure during homing", self.stepperName)
				return
			}
			if self.stepperEnable == nil {
				return
			}
			enableLine, err := self.stepperEnable.Lookup_enable(self.stepperName)
			if err == nil {
				if enableLine.Is_motor_enabled() {
					logger.Panicf("TMCCommandRuntime HandleSyncMCUPos %v", panicErr)
				}
			} else {
				logger.Errorf("TMCCommandRuntime HandleSyncMCUPos %v, error: %v", panicErr, err)
			}
		}
	}()

	driverPhase, err := self.queryPhaseWithRetry(pauseRetry)
	if err != nil {
		panic(err)
	}
	ret0, _ := stepper.Get_dir_inverted()
	if ret0 != 0 {
		driverPhase = 1023 - driverPhase
	}

	phases := self.core.GetPhases()
	phase := maths.PyMod(int(float64(driverPhase)/1024*float64(phases)+.5), phases)
	moff := maths.PyMod(phase-stepper.Get_mcu_position(), phases)
	phaseOffset, _ := self.core.GetPhaseOffset()
	if value.IsNotNone(phaseOffset) && cast.Int(phaseOffset) != moff {
		logger.Debugf("Stepper %s phase change (was %d now %d)",
			self.stepperName, phaseOffset, moff)
	}
	self.core.SetPhaseOffset(&moff)
	return nil
}

func (self *CommandRuntime) HandleEnable(toolhead CommandToolhead, mutex CommandMutex, printTime *float64) error {
	if err := self.core.ApplyEnableRegisters(printTime); err != nil {
		return err
	}
	didReset := self.statusChecker.StartChecks()
	if didReset {
		self.core.ClearPhaseOffset()
	}
	if self.homingActive {
		// During homing, phase-sync reads contend with sensorless/UART traffic and
		// can introduce multi-second stalls between axis moves. Treat phase offset
		// as optional while homing is active and let normal sync events refresh it
		// after homing completes.
		return nil
	}

	phaseOffset, _ := self.core.GetPhaseOffset()
	if value.IsNotNone(phaseOffset) {
		return nil
	}
	mutex.Lock()
	defer mutex.Unlock()
	phaseOffset, _ = self.core.GetPhaseOffset()
	if value.IsNotNone(phaseOffset) {
		return nil
	}
	if self.stepper == nil {
		return fmt.Errorf("TMC stepper %s not initialized", self.stepperName)
	}

	logger.Infof("Pausing toolhead to calculate %s phase offset", self.stepperName)
	toolhead.Wait_moves()
	return self.handleSyncMCUPos(self.stepper, func() {
		pauseDriverCommandToolhead(toolhead, commandRuntimePhaseQueryRetryDelay)
	})
}

func (self *CommandRuntime) HandleDisable(printTime *float64) error {
	if err := self.core.ApplyDisableRegisters(printTime); err != nil {
		return err
	}
	self.statusChecker.StopChecks()
	return nil
}

// SetHomingActive propagates the homing-active flag to the error-check runtime
// so it can suppress UART read failure counting while homing moves are in
// progress. This is a no-op if the status checker is not an ErrorCheckRuntime.
func (self *CommandRuntime) SetHomingActive(active bool) {
	self.homingActive = active
	if checker, ok := self.statusChecker.(*ErrorCheckRuntime); ok {
		checker.SetHomingActive(active)
	}
}

func (self *CommandRuntime) IsHomingActive() bool {
	return self.homingActive
}

func (self *CommandRuntime) HandleMCUIdentify(lookupStepper CommandStepperLookup) error {
	self.stepper = lookupStepper(self.stepperName)
	if self.stepper == nil {
		return fmt.Errorf("TMC stepper %s not found", self.stepperName)
	}
	self.stepper.Setup_default_pulse_duration(.000000100, true)
	return nil
}

func (self *CommandRuntime) HandleConnect(stateCallback func(float64, bool)) error {
	if self.stepper == nil {
		return fmt.Errorf("TMC stepper %s not initialized", self.stepperName)
	}
	if self.stepperEnable == nil {
		return fmt.Errorf("TMC stepper enable registry not configured for %s", self.stepperName)
	}
	_, stepBothEdge := self.stepper.Get_pulse_duration()
	if stepBothEdge {
		self.core.Fields().Set_field("dedge", 1, nil, nil)
	}

	enableLine, err := self.stepperEnable.Lookup_enable(self.stepperName)
	if err != nil {
		return err
	}
	enableLine.Register_state_callback(stateCallback)
	if !enableLine.Has_dedicated_enable() {
		self.core.EnableVirtualEnable()
		logger.Infof("Enabling TMC virtual enable for '%s'", self.stepperName)
	}

	return self.InitRegisters(nil)
}

func (self *CommandRuntime) GetStatus(eventtime float64) map[string]interface{} {
	var cpos interface{}
	phaseOffset, _ := self.core.GetPhaseOffset()
	if value.IsNotNone(self.stepper) && value.IsNotNone(phaseOffset) {
		cpos = self.stepper.Mcu_to_commanded_position(cast.Int(phaseOffset))
	}
	current := self.currentHelper.Get_current()
	res := map[string]interface{}{
		"mcu_phase_offset":      phaseOffset,
		"phase_offset_position": cpos,
		"run_current":           current[0],
		"hold_current":          current[1],
	}

	for key, value := range self.statusChecker.GetStatus(eventtime) {
		res[key] = value
	}
	return res
}
