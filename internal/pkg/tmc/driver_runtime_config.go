package tmc

import (
	"fmt"
	kinematicspkg "goklipper/internal/pkg/motion/kinematics"
	"math"
	"strings"

	"goklipper/common/value"
)

type DriverSectionLookup func(section string) DriverConfig

type DriverSectionProvider[T DriverConfig] interface {
	Has_section(section string) bool
	Getsection(section string) T
}

func DriverStepperName(config DriverConfig) string {
	nameParts := strings.Split(config.Get_name(), " ")
	if len(nameParts) <= 1 {
		return ""
	}
	return strings.Join(nameParts[1:], " ")
}

func ApplyDriverMicrostepConfig(config DriverConfig, lookupSection DriverSectionLookup, mcuTMC RegisterAccess) error {
	stepperName := DriverStepperName(config)
	stepperConfig := lookupDriverSection(stepperName, lookupSection)
	if stepperConfig == nil {
		return fmt.Errorf("Could not find config section '[%s]' required by tmc driver", stepperName)
	}

	microstepConfig := stepperConfig
	if value.IsNone(stepperConfig.Get("microsteps", value.None, false)) &&
		value.IsNotNone(config.Get("microsteps", value.None, false)) {
		microstepConfig = config
	}

	steps := map[interface{}]interface{}{256: 0, 128: 1, 64: 2, 32: 3, 16: 4, 8: 5, 4: 6, 2: 7, 1: 8}
	mres := microstepConfig.Getchoice("microsteps", steps, nil, true)
	ApplyMicrostepSettings(mcuTMC.Get_fields(), mres, config.Getboolean("interpolate", true, true))
	return nil
}

func ApplyDriverStealthchopConfig(config DriverConfig, lookupSection DriverSectionLookup, mcuTMC RegisterAccess, tmcFrequency float64) error {
	fields := mcuTMC.Get_fields()
	velocity := config.Getfloat("stealthchop_threshold", math.NaN(), 0., 0, 0, 0, true)
	if math.IsNaN(velocity) {
		ApplyStealthchop(fields, tmcFrequency, 0, math.NaN())
		return nil
	}

	stepDist, err := lookupDriverStepDistance(config, lookupSection)
	if err != nil {
		return err
	}
	ApplyStealthchop(fields, tmcFrequency, stepDist, velocity)
	return nil
}

func ApplyDriverCoolstepThresholdConfig(config DriverConfig, lookupSection DriverSectionLookup, mcuTMC RegisterAccess, tmcFrequency float64) error {
	fields := mcuTMC.Get_fields()
	velocity := config.Getfloat("coolstep_threshold", math.NaN(), 0., 0, 0, 0, true)
	tcoolthrs := 0
	if !math.IsNaN(velocity) {
		stepDist, err := lookupDriverStepDistance(config, lookupSection)
		if err != nil {
			return err
		}
		mres := fields.Get_field("mres", value.None, nil)
		tcoolthrs = TMCtstepHelper(stepDist, int(mres), tmcFrequency, velocity)
	}
	fields.Set_field("tcoolthrs", tcoolthrs, value.None, nil)
	return nil
}

func ApplyDriverHighVelocityThresholdConfig(config DriverConfig, lookupSection DriverSectionLookup, mcuTMC RegisterAccess, tmcFrequency float64) error {
	fields := mcuTMC.Get_fields()
	velocity := config.Getfloat("high_velocity_threshold", math.NaN(), 0., 0, 0, 0, true)
	thigh := 0
	if !math.IsNaN(velocity) {
		stepDist, err := lookupDriverStepDistance(config, lookupSection)
		if err != nil {
			return err
		}
		mres := fields.Get_field("mres", value.None, nil)
		thigh = TMCtstepHelper(stepDist, int(mres), tmcFrequency, velocity)
	}
	fields.Set_field("thigh", thigh, value.None, nil)
	return nil
}

func lookupDriverStepDistance(config DriverConfig, lookupSection DriverSectionLookup) (float64, error) {
	stepperName := DriverStepperName(config)
	stepperConfig := lookupDriverSection(stepperName, lookupSection)
	if stepperConfig == nil {
		return 0, fmt.Errorf("Could not find config section '[%s]' required by tmc driver", stepperName)
	}

	rotationDist, stepsPerRotation := kinematicspkg.ParseStepperDistance(stepperConfig, nil, true)
	return rotationDist / float64(stepsPerRotation), nil
}

func lookupDriverSection(section string, lookupSection DriverSectionLookup) DriverConfig {
	if lookupSection == nil || section == "" {
		return nil
	}
	return lookupSection(section)
}

func LookupDriverSectionFromConfig[T DriverConfig](config DriverSectionProvider[T], section string) DriverConfig {
	if config == nil || section == "" || !config.Has_section(section) {
		return nil
	}
	return config.Getsection(section)
}
