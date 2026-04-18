package mcu

import (
	"fmt"
	"goklipper/common/utils/object"
	motionpkg "goklipper/internal/pkg/motion"
	kinematicspkg "goklipper/internal/pkg/motion/kinematics"
	printerpkg "goklipper/internal/pkg/printer"
)

type legacyStepperPins interface {
	LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{}
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

type LegacyStepperEventPrinter interface {
	RegisterEventHandler(event string, callback func([]interface{}) error)
	SendEvent(event string, params []interface{})
}

type LegacyStepperModuleRegistrar func(module interface{}, stepper *LegacyStepper)

type legacyStepperEnableModule interface {
	Register_stepper(config interface{}, stepper stepperEnableStepper)
}

type legacyForceMoveModule interface {
	RegisterStepper(stepper motionpkg.ForceMoveStepperDriver)
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
		StepPinParams:     pins.LookupPin(stepPin, true, false, nil),
		DirPinParams:      pins.LookupPin(dirPin, true, false, nil),
		RotationDist:      rotationDist,
		StepsPerRotation:  stepsPerRotation,
		StepPulseDuration: config.GetfloatNone("step_pulse_duration", nil, 0, 0, .001, 0, false),
	}
}

func NewLegacyPrinterStepper(name string, stepPinParams map[string]interface{}, dirPinParams map[string]interface{}, rotationDist float64, stepsPerRotation int,
	stepPulseDuration interface{}, unitsInRadians bool, printer LegacyStepperEventPrinter) *LegacyStepper {
	var registerConnectHandler func(func([]interface{}) error)
	var sendPrinterEvent func(string, []interface{})
	if printer != nil {
		registerConnectHandler = func(handler func([]interface{}) error) {
			printer.RegisterEventHandler("project:connect", handler)
		}
		sendPrinterEvent = printer.SendEvent
	}
	stepper := NewLegacyStepper(name, stepPinParams, dirPinParams, rotationDist, stepsPerRotation, stepPulseDuration, unitsInRadians, registerConnectHandler, sendPrinterEvent)
	stepper.BindEventSelf(stepper)
	return stepper
}

func LoadLegacyPrinterStepper(config LegacyStepperFactoryConfig, unitsInRadians bool, printer LegacyStepperEventPrinter, load LegacyStepperModuleLoader,
	registerStepperEnable LegacyStepperModuleRegistrar, registerForceMove LegacyStepperModuleRegistrar) *LegacyStepper {
	plan := BuildLegacyStepperFactoryPlan(config, unitsInRadians)
	stepper := NewLegacyPrinterStepper(plan.Name, plan.StepPinParams, plan.DirPinParams, plan.RotationDist, plan.StepsPerRotation, plan.StepPulseDuration, unitsInRadians, printer)
	RegisterLegacyStepperModules(load, func(module interface{}) {
		if registerStepperEnable != nil {
			registerStepperEnable(module, stepper)
		}
	}, func(module interface{}) {
		if registerForceMove != nil {
			registerForceMove(module, stepper)
		}
	})
	return stepper
}

func LoadLegacyPrinterStepperWithDefaultModules(config LegacyStepperFactoryConfig, unitsInRadians bool, printer LegacyStepperEventPrinter, load LegacyStepperModuleLoader) *LegacyStepper {
	return LoadLegacyPrinterStepper(config, unitsInRadians, printer, load, func(module interface{}, stepper *LegacyStepper) {
		registrar, ok := module.(legacyStepperEnableModule)
		if !ok {
			panic(fmt.Sprintf("stepper_enable module does not implement legacyStepperEnableModule: %T", module))
		}
		registrar.Register_stepper(config, stepper)
	}, func(module interface{}, stepper *LegacyStepper) {
		registrar, ok := module.(legacyForceMoveModule)
		if !ok {
			panic(fmt.Sprintf("force_move module does not implement legacyForceMoveModule: %T", module))
		}
		registrar.RegisterStepper(stepper)
	})
}

type LegacyStepperModuleLoader func(moduleName string) interface{}

func RegisterLegacyStepperModules(load LegacyStepperModuleLoader, registerStepperEnable func(interface{}), registerForceMove func(interface{})) {
	if load == nil {
		return
	}
	if module := load("stepper_enable"); module != nil && registerStepperEnable != nil {
		registerStepperEnable(module)
	}
	if module := load("force_move"); module != nil && registerForceMove != nil {
		registerForceMove(module)
	}
}
