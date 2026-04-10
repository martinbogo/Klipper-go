package kinematics

type RailConfigPlan struct {
	PositionMin       float64
	PositionMax       float64
	HomingPositiveDir bool
}

func BuildRailConfigPlan(positionEndstop float64, needPositionMinMax bool, positionMin float64, positionMax float64, homingPositiveDir bool) RailConfigPlan {
	if !needPositionMinMax {
		positionMin = 0
		positionMax = positionEndstop
	}
	if positionEndstop < positionMin || positionEndstop > positionMax {
		panic("position_endstop must be between position_min and position_max")
	}
	if !homingPositiveDir {
		axisLen := positionMax - positionMin
		if positionEndstop <= positionMin+axisLen/4 {
			homingPositiveDir = false
		} else if positionEndstop >= positionMax-axisLen/4 {
			homingPositiveDir = true
		} else {
			panic("unable to infer homing_positive_dir")
		}
	} else if positionEndstop == positionMin {
		panic("invalid homing_positive_dir / position_endstop")
	}
	return RailConfigPlan{
		PositionMin:       positionMin,
		PositionMax:       positionMax,
		HomingPositiveDir: homingPositiveDir,
	}
}
