package kinematics

import (
	"fmt"
	"goklipper/common/utils/object"
)

type legacyRailConfig interface {
	Get_name() string
	Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, note_valid bool) float64
	Getboolean(option string, default1 interface{}, note_valid bool) bool
}

type railPositionEndstopProvider interface {
	Get_position_endstop() float64
}

type RailSettings struct {
	PositionEndstop    float64
	PositionMin        float64
	PositionMax        float64
	HomingSpeed        float64
	SecondHomingSpeed  float64
	HomingRetractSpeed float64
	HomingRetractDist  float64
	HomingPositiveDir  bool
}

func resolveRailPositionEndstop(config legacyRailConfig, endstop interface{}, defaultPositionEndstop interface{}) float64 {
	if provider, ok := endstop.(railPositionEndstopProvider); ok {
		return provider.Get_position_endstop()
	}
	if typedDefault, ok := defaultPositionEndstop.(*float64); ok && typedDefault == nil {
		return config.Getfloat("position_endstop", object.Sentinel{}, 0, 0, 0, 0, false)
	}
	return config.Getfloat("position_endstop", defaultPositionEndstop, 0, 0, 0, 0, false)
}

func BuildLegacyRailSettings(config legacyRailConfig, endstop interface{}, needPositionMinMax bool, defaultPositionEndstop interface{}) RailSettings {
	positionEndstop := resolveRailPositionEndstop(config, endstop, defaultPositionEndstop)
	positionMin := 0.
	positionMax := positionEndstop
	if needPositionMinMax {
		positionMin = config.Getfloat("position_min", 0., 0, 0, 0, 0, false)
		positionMax = config.Getfloat("position_max", object.Sentinel{}, 0, 0, positionMin, 0, false)
	}
	homingSpeed := config.Getfloat("homing_speed", 5.0, 0, 0, 0., 0, false)
	secondHomingSpeed := config.Getfloat("second_homing_speed", homingSpeed/2., 0, 0, 0., 0, false)
	homingRetractSpeed := config.Getfloat("homing_retract_speed", homingSpeed, 0, 0, 0., 0, false)
	homingRetractDist := config.Getfloat("homing_retract_dist", 5., 0, 0, 0., 0, false)
	requestedHomingPositiveDir := config.Getboolean("homing_positive_dir", false, false)
	defer func() {
		if recovered := recover(); recovered != nil {
			switch recovered.(string) {
			case "position_endstop must be between position_min and position_max":
				panic(fmt.Errorf("Position_endstop '%f' in section '%s' must be between position_min and position_max", positionEndstop, config.Get_name()))
			case "unable to infer homing_positive_dir":
				panic(fmt.Errorf("Unable to infer homing_positive_dir in section '%s'", config.Get_name()))
			case "invalid homing_positive_dir / position_endstop":
				panic(fmt.Errorf("Invalid homing_positive_dir / Position_endstop in '%s'", config.Get_name()))
			default:
				panic(recovered)
			}
		}
	}()
	plan := BuildRailConfigPlan(positionEndstop, needPositionMinMax, positionMin, positionMax, requestedHomingPositiveDir)
	if requestedHomingPositiveDir == false {
		config.Getboolean("homing_positive_dir", plan.HomingPositiveDir, false)
	}
	return RailSettings{
		PositionEndstop:    positionEndstop,
		PositionMin:        plan.PositionMin,
		PositionMax:        plan.PositionMax,
		HomingSpeed:        homingSpeed,
		SecondHomingSpeed:  secondHomingSpeed,
		HomingRetractSpeed: homingRetractSpeed,
		HomingRetractDist:  homingRetractDist,
		HomingPositiveDir:  plan.HomingPositiveDir,
	}
}
