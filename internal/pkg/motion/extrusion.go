package motion

import (
	"errors"
	"fmt"
	"goklipper/common/logger"
	"math"
)

type ExtrusionLimits struct {
	CanExtrude      bool
	NozzleDiameter  float64
	FilamentArea    float64
	MaxExtrudeRatio float64
	MaxEVelocity    float64
	MaxEAccel       float64
	MaxEDistance    float64
	InstantCornerV  float64
}

type ExtrusionMove struct {
	Accel              float64
	StartV             float64
	CruiseV            float64
	AccelT             float64
	CruiseT            float64
	DecelT             float64
	StartPosition      float64
	CanPressureAdvance float64
}

func CheckExtrusionMove(move *Move, limits ExtrusionLimits) error {
	axisR := move.Axes_r[3]
	if !limits.CanExtrude {
		return errors.New("Extrude below minimum temp\n" +
			"See the 'min_extrude_temp' config option for details")
	}
	if (move.Axes_d[0] == 0.0 && move.Axes_d[1] == 0.0) || axisR < 0. {
		if math.Abs(move.Axes_d[3]) > limits.MaxEDistance {
			return fmt.Errorf(
				"Extrude only move too long (%.3fmm vs %.3fmm)\n"+
					"See the 'max_extrude_only_distance' config"+
					" option for details", move.Axes_d[3], limits.MaxEDistance)
		}
		invExtrudeR := 1. / math.Abs(axisR)
		move.Limit_speed(limits.MaxEVelocity*invExtrudeR,
			limits.MaxEAccel*invExtrudeR)
		return nil
	}
	if axisR > limits.MaxExtrudeRatio {
		if move.Axes_d[3] <= limits.NozzleDiameter*limits.MaxExtrudeRatio {
			return nil
		}
		area := axisR * limits.FilamentArea
		logger.Debugf("Overextrude: %f vs %f (area=%.3f dist=%.3f)",
			axisR, limits.MaxExtrudeRatio, area, move.Move_d)
		return fmt.Errorf("Move exceeds maximum extrusion (%.3fmm^2 vs %.3fmm^2)\n"+
			"See the 'max_extrude_cross_section' config option for details",
			area, limits.MaxExtrudeRatio*limits.FilamentArea)
	}
	return nil
}

func CalcExtrusionJunction(prevMove, move *Move, instantCornerV float64) float64 {
	diffR := move.Axes_r[3] - prevMove.Axes_r[3]
	if diffR != 0.0 {
		return math.Pow(instantCornerV/math.Abs(diffR), 2)
	}
	return move.Max_cruise_v2
}

func BuildExtrusionMove(move *Move) ExtrusionMove {
	axisR := move.Axes_r[3]
	accel := move.Accel * axisR
	startV := move.Start_v * axisR
	cruiseV := move.Cruise_v * axisR
	canPressureAdvance := 0.0
	if axisR > 0. && (move.Axes_d[0] != 0.0 || move.Axes_d[1] != 0.0) {
		canPressureAdvance = 1.0
	}
	return ExtrusionMove{
		Accel:              accel,
		StartV:             startV,
		CruiseV:            cruiseV,
		AccelT:             move.Accel_t,
		CruiseT:            move.Cruise_t,
		DecelT:             move.Decel_t,
		StartPosition:      move.Start_pos[3],
		CanPressureAdvance: canPressureAdvance,
	}
}

func NoExtruderMoveError(move *Move) error {
	return move.Move_error("Extrude when no extruder present")
}

func DefaultExtrusionJunction(move *Move) float64 {
	return move.Max_cruise_v2
}
