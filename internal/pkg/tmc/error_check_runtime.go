package tmc

import (
	"errors"
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/sys"
	"math"
	"strings"
)

type ErrorCheckReactor interface {
	Monotonic() float64
	Pause(waketime float64) float64
	RegisterTimer(callback func(float64) float64, waketime float64) interface{}
	UnregisterTimer(timer interface{})
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
}

func NewErrorCheckRuntime(driverName string, stepperName string, mcuTMC RegisterAccess, reactor ErrorCheckReactor, shutdown ShutdownHandler, registerMonitor MonitorRegistrar) *ErrorCheckRuntime {
	if shutdown == nil {
		shutdown = func(string) {}
	}
	self := &ErrorCheckRuntime{
		reactor:     reactor,
		shutdown:    shutdown,
		stepperName: stepperName,
		mcuTMC:      mcuTMC,
		fields:      mcuTMC.Get_fields(),
		clearGSTAT:  true,
		irunField:   "irun",
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

func (self *ErrorCheckRuntime) doPeriodicCheck(eventtime float64) float64 {
	defer sys.CatchPanic()
	if _, err := self.queryRegister(&self.drvStatusRegInfo, false); err != nil {
		self.shutdown(err.Error())
		return constants.NEVER
	}

	if self.gstatRegInfo != nil {
		if _, err := self.queryRegister(self.gstatRegInfo, false); err != nil {
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
	return eventtime + 1
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
	if _, err := self.queryRegister(&self.drvStatusRegInfo, false); err != nil {
		self.shutdown(err.Error())
	}

	clearedFlags := int64(0)
	if self.gstatRegInfo != nil {
		cleared, err := self.queryRegister(self.gstatRegInfo, self.clearGSTAT)
		if err != nil {
			self.shutdown(err.Error())
		} else {
			clearedFlags = cleared
		}
	}

	curtime := self.reactor.Monotonic()
	self.checkTimer = self.reactor.RegisterTimer(self.doPeriodicCheck, curtime+1.)

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
