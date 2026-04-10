package heater

import (
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"math"
)

const HintThermal = `
See the 'verify_heater' section in docs/Config_Reference.md
for the parameters that control this check.
`

type VerifyHeater struct {
	heaterName        string
	hysteresis        float64
	maxError          float64
	heatingGain       float64
	checkGainTime     float64
	approachingTarget bool
	startingApproach  bool
	lastTarget        float64
	goalTemp          float64
	errorValue        float64
	goalSystime       float64
}

func NewVerifyHeater(heaterName string, hysteresis, maxError, heatingGain, checkGainTime float64) *VerifyHeater {
	return &VerifyHeater{
		heaterName:        heaterName,
		hysteresis:        hysteresis,
		maxError:          maxError,
		heatingGain:       heatingGain,
		checkGainTime:     checkGainTime,
		approachingTarget: false,
		startingApproach:  false,
		lastTarget:        0,
		goalTemp:          0,
		errorValue:        0,
		goalSystime:       constants.NEVER,
	}
}

func (self *VerifyHeater) Check(eventtime, temp, target float64) (float64, string) {
	if temp >= target-self.hysteresis || target <= 0.0 {
		if self.approachingTarget && target > 0.0 {
			logger.Infof("Heater %s within range of %.3f", self.heaterName, target)
		}
		self.approachingTarget = false
		self.startingApproach = false
		if temp <= target+self.hysteresis {
			self.errorValue = 0.0
		}
		self.lastTarget = target
		return eventtime + 1.0, ""
	}

	self.errorValue += (target - self.hysteresis) - temp
	if !self.approachingTarget {
		if target != self.lastTarget {
			logger.Infof("Heater %s approaching new target of %.3f", self.heaterName, target)
			self.approachingTarget = true
			self.startingApproach = true
			self.goalTemp = temp + self.heatingGain
			self.goalSystime = eventtime + self.checkGainTime
		} else if self.errorValue >= self.maxError {
			return constants.NEVER, self.FaultMessage()
		}
	} else if temp >= self.goalTemp {
		self.startingApproach = false
		self.errorValue = 0.0
		self.goalTemp = temp + self.heatingGain
		self.goalSystime = eventtime + self.checkGainTime
	} else if eventtime >= self.goalSystime {
		self.approachingTarget = false
		logger.Infof("Heater %s no longer approaching target %.3f", self.heaterName, target)
	} else if self.startingApproach {
		self.goalTemp = math.Min(self.goalTemp, temp+self.heatingGain)
	}
	self.lastTarget = target
	return eventtime + 1.0, ""
}

func (self *VerifyHeater) FaultMessage() string {
	return fmt.Sprintf("Heater %s not heating at expected rate", self.heaterName)
}
