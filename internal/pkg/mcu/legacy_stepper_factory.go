package mcu

import (
	"fmt"
	"goklipper/common/utils/object"
	kinematicspkg "goklipper/internal/pkg/motion/kinematics"
	printerpkg "goklipper/internal/pkg/printer"
)

type legacyStepperPins interface {
	Lookup_pin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{}
}

type legacyStepperPulseConfig interface {
	GetfloatNone(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) interface{}
}

type LegacyStepperFactoryConfig interface {
	printerpkg.ModuleConfig
	kinematicspkg.StepperDistanceConfig
	legacyStepperPulseConfig
}

type LegacyStepperFactoryPlan struct {
	Name              string
	StepPinParams     map[string]interface{}
	DirPinParams      map[string]interface{}
	RotationDist      float64
	StepsPerRotation  int
	StepPulseDuration interface{}
}

func BuildLegacyStepperFactoryPlan(config LegacyStepperFactoryConfig, unitsInRadians bool) LegacyStepperFactoryPlan {
	pinsObj := config.Printer().LookupObject("pins", nil)
	pins, ok := pinsObj.(legacyStepperPins)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement legacyStepperPins: %T", pinsObj))
	}
	stepPin := config.Get("step_pin", object.Sentinel{}, true).(string)
	dirPin := config.Get("dir_pin", object.Sentinel{}, true).(string)
	rotationDist, stepsPerRotation := kinematicspkg.ParseStepperDistance(config, unitsInRadians, true)
	return LegacyStepperFactoryPlan{
		Name:              config.Get_name(),
		StepPinParams:     pins.Lookup_pin(stepPin, true, false, nil),
		DirPinParams:      pins.Lookup_pin(dirPin, true, false, nil),
		RotationDist:      rotationDist,
		StepsPerRotation:  stepsPerRotation,
		StepPulseDuration: config.GetfloatNone("step_pulse_duration", nil, 0, 0, .001, 0, false),
	}
}

type LegacyStepperModuleLoader func(moduleName string) interface{}

type LegacyStepperModuleRegistrar interface {
	RegisterStepperEnable(module interface{})
	RegisterForceMove(module interface{})
}

func RegisterLegacyStepperModules(load LegacyStepperModuleLoader, registrar LegacyStepperModuleRegistrar) {
	if load == nil || registrar == nil {
		return
	}
	if module := load("stepper_enable"); module != nil {
		registrar.RegisterStepperEnable(module)
	}
	if module := load("force_move"); module != nil {
		registrar.RegisterForceMove(module)
	}
}
