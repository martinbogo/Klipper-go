package mcu

import (
	"fmt"
	"goklipper/internal/pkg/chelper"
)

func AllocateLegacyStepperKinematics(allocFunc string, params interface{}) (interface{}, error) {
	switch allocFunc {
	case "cartesian_stepper_alloc":
		axis := params.([]interface{})[0].(uint8)
		return chelper.Cartesian_stepper_alloc(int8(axis)), nil
	case "corexy_stepper_alloc":
		axis := params.([]interface{})[0].(uint8)
		return chelper.Corexy_stepper_alloc(int8(axis)), nil
	default:
		return nil, fmt.Errorf("stepper itersolve allocator not implemented: %s", allocFunc)
	}
}

func ConfigureLegacyStepperKinematics(sk interface{}, stepqueue interface{}, stepDist float64, trapq interface{}) {
	chelper.Itersolve_set_stepcompress(sk, stepqueue, stepDist)
	chelper.Itersolve_set_trapq(sk, trapq)
}

func SetLegacyStepperTrapq(sk interface{}, trapq interface{}) {
	chelper.Itersolve_set_trapq(sk, trapq)
}

func CalcLegacyStepperPositionFromCoord(sk interface{}, coord []float64) float64 {
	return chelper.Itersolve_calc_position_from_coord(sk, coord[0], coord[1], coord[2])
}

func SetLegacyStepperPosition(sk interface{}, coord []float64) {
	chelper.Itersolve_set_position(sk, coord[0], coord[1], coord[2])
}

func GetLegacyStepperCommandedPosition(sk interface{}) float64 {
	return chelper.Itersolve_get_commanded_pos(sk)
}

func LegacyStepperActiveAxis(sk interface{}, axis int8) int32 {
	return chelper.Itersolve_is_active_axis(sk, uint8(axis))
}
