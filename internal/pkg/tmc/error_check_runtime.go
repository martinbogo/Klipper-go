package tmc

import (
	"errors"
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/sys"
	"math"
	"strings"
	"sync"
)

const errorCheckMaxTransientUARTReadFailures = 5

// After homing ends, shared-UART traffic (phase sync, status reads, delayed
// register verification) can remain elevated for several seconds. Keep
// transient UART read failures out of the shutdown budget during this cooldown
// period.
const errorCheckPostHomingTransientGraceSeconds = 30.0

// pollStaggerInterval spreads concurrent drivers across the 1-second poll cycle
// so their DRV_STATUS/GSTAT reads don't fire simultaneously on the shared UART.
const pollStaggerInterval = 0.333

var (
	pollStartMu    sync.Mutex
	pollStartCount int
)

type ErrorCheckReactor interface {
	Monotonic() float64
	Pause(waketime float64) float64
	RegisterTimer(callback func(float64) float64, waketime float64) interface{}
	UnregisterTimer(timer interface{})
}

type ErrorCheckReactorFuncs struct {
	MonotonicFunc       func() float64
	PauseFunc           func(float64) float64
	RegisterTimerFunc   func(func(float64) float64, float64) interface{}
	UnregisterTimerFunc func(interface{})
}

func (funcs ErrorCheckReactorFuncs) Monotonic() float64 {
	return funcs.MonotonicFunc()
}

func (funcs ErrorCheckReactorFuncs) Pause(waketime float64) float64 {
	return funcs.PauseFunc(waketime)
}

func (funcs ErrorCheckReactorFuncs) RegisterTimer(callback func(float64) float64, waketime float64) interface{} {
	return funcs.RegisterTimerFunc(callback, waketime)
}

func (funcs ErrorCheckReactorFuncs) UnregisterTimer(timer interface{}) {
	funcs.UnregisterTimerFunc(timer)
}

type ShutdownHandler func(msg string)

type MonitorRegistrar func()

type errorCheckRegisterInfo struct {
	lastValue    int64
	regName      string
	mask         int64
	errMask      int64
	csActualMask int64
}

type ErrorCheckRuntime struct {
	reactor          ErrorCheckReactor
	shutdown         ShutdownHandler
	stepperName      string
	mcuTMC           RegisterAccess
	fields           *FieldHelper
	checkTimer       interface{}
	lastDrvStatus    int64
	hasLastDrvStatus bool
	lastDrvFields    map[string]interface{}
	gstatRegInfo     *errorCheckRegisterInfo
	clearGSTAT       bool
	irunField        string
	drvStatusRegInfo errorCheckRegisterInfo
	adcTemp          *int64
	adcTempReg       string
	readFailureCount int
	pollOffset       float64
	homingActive     bool
	homingGraceUntil float64
}

func (self *ErrorCheckRuntime) isHomingOrGrace(eventtime float64) bool {
	return self.homingActive || eventtime < self.homingGraceUntil
}

func isTransientUARTReadError(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "Unable to read tmc uart")
}

func NewErrorCheckRuntime(driverName string, stepperName string, mcuTMC RegisterAccess, reactor ErrorCheckReactor, shutdown ShutdownHandler, registerMonitor MonitorRegistrar) *ErrorCheckRuntime {
	if shutdown == nil {
		shutdown = func(string) {}
	}
	pollStartMu.Lock()
	idx := pollStartCount
	pollStartCount++
	pollStartMu.Unlock()

	self := &ErrorCheckRuntime{
		reactor:     reactor,
		shutdown:    shutdown,
		stepperName: stepperName,
		mcuTMC:      mcuTMC,
		fields:      mcuTMC.Get_fields(),
		clearGSTAT:  true,
		irunField:   "irun",
		pollOffset:  float64(idx) * pollStaggerInterval,
	}
	self.checkTimer = reactor.RegisterTimer(self.doPeriodicCheck, constants.NEVER)

	if regName, ok := self.fields.Lookup_register("drv_err", nil).(string); ok && regName != "" {
		self.gstatRegInfo = &errorCheckRegisterInfo{
			regName: regName,
			mask:    0xffffffff,
			errMask: 0xffffffff,
		}
	}

	regName := "DRV_STATUS"
	if driverName == "tmc2130" {
		self.clearGSTAT = false
		if regFields, ok := self.fields.All_fields()[regName]; ok {
			self.drvStatusRegInfo.csActualMask = regFields["cs_actual"]
		}
	} else if driverName == "tmc2660" {
		self.irunField = "cs"
		regName = "READRSP@RDSEL2"
		if regFields, ok := self.fields.All_fields()[regName]; ok {
			self.drvStatusRegInfo.csActualMask = regFields["se"]
		}
	}
	self.drvStatusRegInfo.regName = regName
	if regFields, ok := self.fields.All_fields()[regName]; ok {
		for _, fieldName := range []string{"ot", "s2ga", "s2gb", "s2vsa", "s2vsb"} {
			self.drvStatusRegInfo.mask |= regFields[fieldName]
			self.drvStatusRegInfo.errMask |= regFields[fieldName]
		}
		for _, fieldName := range []string{"otpw", "t120", "t143", "t150", "t157"} {
			self.drvStatusRegInfo.mask |= regFields[fieldName]
		}
	}

	if regName, ok := self.fields.Lookup_register("adc_temp", nil).(string); ok && regName != "" {
		self.adcTempReg = regName
		if registerMonitor != nil {
			registerMonitor()
		}
	}

	return self
}

func (self *ErrorCheckRuntime) queryRegister(regInfo *errorCheckRegisterInfo, tryClear bool) (int64, error) {
	lastValue := regInfo.lastValue
	clearedFlags := int64(0)
	count := 0

	for {
		val, err := self.mcuTMC.Get_register(regInfo.regName)
		if err != nil {
			count++
			if count < 3 && strings.HasPrefix(err.Error(), "Unable to read tmc uart") {
				self.reactor.Pause(self.reactor.Monotonic() + 0.050)
				continue
			}
			return 0, err
		}

		if val&regInfo.mask != lastValue&regInfo.mask {
			logger.Infof("TMC %s reports %s", self.stepperName, self.fields.Pretty_format(regInfo.regName, val))
		}

		regInfo.lastValue = val
		lastValue = val
		if (val & regInfo.errMask) == 0 {
			if regInfo.csActualMask == 0 || (val&regInfo.csActualMask) != 0 {
				break
			}

			irun := self.fields.Get_field(self.irunField, nil, nil)
			if self.checkTimer == nil || irun < 4 {
				break
			}

			if self.irunField == "irun" && self.fields.Get_field("ihold", nil, nil) == 0 {
				break
			}
		}

		count++
		if count >= 3 {
			return 0, fmt.Errorf("TMC %s reports error: %s", self.stepperName, self.fields.Pretty_format(regInfo.regName, val))
		}

		if tryClear && (val&regInfo.errMask) != 0 {
			tryClear = false
			clearedFlags |= val & regInfo.errMask
			_ = self.mcuTMC.Set_register(regInfo.regName, val&regInfo.errMask, nil)
		}
	}

	return clearedFlags, nil
}

func (self *ErrorCheckRuntime) queryTemperature() (err error) {
	defer func() {
		if r := recover(); r != nil {
			self.adcTemp = nil
			err = errors.New("get adc temp failed")
		}
	}()
	val, _ := self.mcuTMC.Get_register(self.adcTempReg)
	self.adcTemp = &val
	return nil
}

// handleTransientUARTReadError handles a transient UART read error in the periodic
// check loop. During homing the shared UART bus is saturated by stall-detection
// register writes for all drivers, so individual DRV_STATUS poll failures are
// expected and must not consume the shutdown failure budget. Returns (nextWake,
// true) when the caller should reschedule without counting a failure, or
// (0, false) when the threshold has been exceeded and shutdown should be called.
func (self *ErrorCheckRuntime) handleTransientUARTReadError(eventtime float64) (float64, bool) {
	if self.isHomingOrGrace(eventtime) {
		return eventtime + 1, true
	}
	self.readFailureCount++
	logger.Infof("TMC %s transient DRV_STATUS read failure %d/%d", self.stepperName, self.readFailureCount, errorCheckMaxTransientUARTReadFailures)
	if self.readFailureCount < errorCheckMaxTransientUARTReadFailures {
		return eventtime + 1, true
	}
	return 0, false
}

func (self *ErrorCheckRuntime) doPeriodicCheck(eventtime float64) float64 {
	defer sys.CatchPanic()
	if self.isHomingOrGrace(eventtime) {
		// Avoid adding more traffic to the shared UART while homing is active
		// or immediately after homing. Polling during this period mainly yields
		// transport noise and can interfere with motion-critical transactions.
		return eventtime + 1
	}
	if _, err := self.queryRegister(&self.drvStatusRegInfo, false); err != nil {
		if isTransientUARTReadError(err) {
			if next, ok := self.handleTransientUARTReadError(eventtime); ok {
				return next
			}
		}
		self.shutdown(err.Error())
		return constants.NEVER
	}

	if self.gstatRegInfo != nil {
		if _, err := self.queryRegister(self.gstatRegInfo, false); err != nil {
			if isTransientUARTReadError(err) {
				if next, ok := self.handleTransientUARTReadError(eventtime); ok {
					return next
				}
			}
			self.shutdown(err.Error())
			return constants.NEVER
		}
	}
	if self.adcTempReg != "" {
		if err := self.queryTemperature(); err != nil {
			self.shutdown(err.Error())
			return constants.NEVER
		}
	}
	self.readFailureCount = 0
	return eventtime + 1
}

// SetHomingActive marks whether a homing move is in progress. When true,
// transient UART read failures in the periodic check loop are silently
// suppressed (not counted toward the shutdown threshold) because the shared
// bitbang UART is saturated by stall-detection register writes. The failure
// counter is reset both when homing starts and when it ends so that neither
// pre-homing nor during-homing failures pollute the post-homing budget.
func (self *ErrorCheckRuntime) SetHomingActive(active bool) {
	self.homingActive = active
	if active {
		self.homingGraceUntil = 0
	} else {
		self.homingGraceUntil = self.reactor.Monotonic() + errorCheckPostHomingTransientGraceSeconds
	}
	self.readFailureCount = 0
}

func (self *ErrorCheckRuntime) StopChecks() {
	if self.checkTimer == nil {
		return
	}
	self.reactor.UnregisterTimer(self.checkTimer)
	self.checkTimer = nil
}

func (self *ErrorCheckRuntime) StartChecks() bool {
	if self.checkTimer != nil {
		self.StopChecks()
	}
	self.readFailureCount = 0
	curtime := self.reactor.Monotonic()
	if self.isHomingOrGrace(curtime) {
		// During homing and the immediate post-homing grace window, avoid
		// enable-time UART reads. Shared-UART traffic is elevated in this
		// period and startup reads can fail transiently, causing false
		// shutdowns before motion-critical traffic settles.
		self.checkTimer = self.reactor.RegisterTimer(self.doPeriodicCheck, curtime+1.+self.pollOffset)
		return false
	}
	// Match Python: if the initial DRV_STATUS read fails, call shutdown and
	// return immediately — do NOT register the periodic timer.
	if _, err := self.queryRegister(&self.drvStatusRegInfo, false); err != nil {
		self.shutdown(err.Error())
		return false
	}

	clearedFlags := int64(0)
	if self.gstatRegInfo != nil {
		cleared, err := self.queryRegister(self.gstatRegInfo, self.clearGSTAT)
		if err != nil {
			self.shutdown(err.Error())
			return false
		}
		clearedFlags = cleared
	}

	self.checkTimer = self.reactor.RegisterTimer(self.doPeriodicCheck, curtime+1.+self.pollOffset)

	if clearedFlags != 0 {
		if gstatFields, ok := self.fields.All_fields()["GSTAT"]; ok {
			if (clearedFlags & gstatFields["reset"]) != 0 {
				return true
			}
		}
	}
	return false
}

func (self *ErrorCheckRuntime) GetStatus(eventtime float64) map[string]interface{} {
	_ = eventtime
	if self.checkTimer == nil {
		return map[string]interface{}{"drv_status": nil}
	}
	temperature := 0.0
	if self.adcTemp != nil {
		temperature = math.Round(float64(*self.adcTemp-2038)/7.7) / 100
	}

	lastValue := self.drvStatusRegInfo.lastValue
	if !self.hasLastDrvStatus || lastValue != self.lastDrvStatus {
		self.lastDrvStatus = lastValue
		self.hasLastDrvStatus = true
		fieldValues := self.fields.Get_reg_fields(self.drvStatusRegInfo.regName, lastValue)
		filtered := make(map[string]int64)
		for key, value := range fieldValues {
			if value != 0 {
				filtered[key] = value
			}
		}
		self.lastDrvFields = map[string]interface{}{"drv_status": filtered}
	}
	return map[string]interface{}{"drv_status": self.lastDrvFields, "temperature": temperature}
}
