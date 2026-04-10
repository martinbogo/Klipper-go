package mcu

import "goklipper/common/utils/cast"

const (
	defaultStepPulseDuration = 0.000002
	minBothEdgeDuration      = 0.000000200
)

type StepperPulseConfigPlan struct {
	StepPulseDuration float64
	StepBothEdge      bool
	InvertStep        int
	StepPulseTicks    int64
}

func BuildStepperPulseConfigPlan(stepPulseDuration interface{}, reqStepBothEdge bool, invertStep int, stepperBothEdgeConstant interface{}, secondsToClock func(float64) int64) StepperPulseConfigPlan {
	pulseDuration := defaultStepPulseDuration
	if stepPulseDuration != nil {
		pulseDuration = cast.ToFloat64(stepPulseDuration)
	}
	stepBothEdge := false
	if reqStepBothEdge && cast.ToFloat64(stepperBothEdgeConstant) != 0 && pulseDuration <= minBothEdgeDuration {
		stepBothEdge = true
		pulseDuration = 0
		invertStep = -1
	}
	return StepperPulseConfigPlan{
		StepPulseDuration: pulseDuration,
		StepBothEdge:      stepBothEdge,
		InvertStep:        invertStep,
		StepPulseTicks:    secondsToClock(pulseDuration),
	}
}
