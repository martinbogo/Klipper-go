package mcu

import "strings"

type LegacyStepperRuntimeState struct {
	name              string
	stepPulseDuration interface{}
	unitsInRadians    bool
	reqStepBothEdge   bool
}

func NewLegacyStepperRuntimeState(name string, stepPulseDuration interface{}, unitsInRadians bool) *LegacyStepperRuntimeState {
	return &LegacyStepperRuntimeState{
		name:              name,
		stepPulseDuration: stepPulseDuration,
		unitsInRadians:    unitsInRadians,
	}
}

func (self *LegacyStepperRuntimeState) Name(short bool) string {
	if self == nil {
		return ""
	}
	if short && strings.HasPrefix(self.name, "stepper_") {
		return self.name[8:]
	}
	return self.name
}

func (self *LegacyStepperRuntimeState) UnitsInRadians() bool {
	return self != nil && self.unitsInRadians
}

func (self *LegacyStepperRuntimeState) PulseDuration() (interface{}, bool) {
	if self == nil {
		return nil, false
	}
	return self.stepPulseDuration, self.reqStepBothEdge
}

func (self *LegacyStepperRuntimeState) SetupDefaultPulseDuration(pulseDuration interface{}, stepBothEdge bool) {
	if self == nil {
		return
	}
	if self.stepPulseDuration == nil {
		self.stepPulseDuration = pulseDuration
	}
	self.reqStepBothEdge = stepBothEdge
}

func (self *LegacyStepperRuntimeState) BuildPulseConfigPlan(invertStep int, stepperBothEdgeConstant interface{}, secondsToClock func(float64) int64) StepperPulseConfigPlan {
	if self == nil {
		return BuildStepperPulseConfigPlan(nil, false, invertStep, stepperBothEdgeConstant, secondsToClock)
	}
	plan := BuildStepperPulseConfigPlan(self.stepPulseDuration, self.reqStepBothEdge, invertStep, stepperBothEdgeConstant, secondsToClock)
	self.stepPulseDuration = plan.StepPulseDuration
	return plan
}
