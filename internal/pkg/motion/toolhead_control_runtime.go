package motion

import (
	"fmt"
	"math"
)

type ToolheadMoveRuntime interface {
	CommandedPosition() []float64
	MoveConfig() MoveConfig
	SetCommandedPosition(position []float64)
	CheckKinematicMove(move *Move)
	CheckExtruderMove(move *Move)
	QueueMove(move *Move)
	PrintTime() float64
	NeedCheckPause() float64
	CheckPause()
}

func RunToolheadMove(runtime ToolheadMoveRuntime, newpos []float64, speed float64) bool {
	move := NewMove(runtime.MoveConfig(), runtime.CommandedPosition(), newpos, speed)
	if move.Move_d == 0.0 {
		return false
	}
	if move.Is_kinematic_move {
		runtime.CheckKinematicMove(move)
	}
	if move.Axes_d[3] != 0.0 {
		runtime.CheckExtruderMove(move)
	}
	runtime.SetCommandedPosition(move.End_pos)
	runtime.QueueMove(move)
	if runtime.PrintTime() > runtime.NeedCheckPause() {
		runtime.CheckPause()
	}
	return true
}

func BuildToolheadManualMoveTarget(commandedPos []float64, coord []interface{}) []float64 {
	length := int(math.Max(float64(len(commandedPos)), float64(len(coord))))
	curpos := make([]float64, length)
	copy(curpos, commandedPos)
	for i := 0; i < len(coord); i++ {
		if coord[i] != nil {
			curpos[i] = coord[i].(float64)
		}
	}
	return curpos
}

type ToolheadDwellRuntime interface {
	GetLastMoveTime() float64
	AdvanceMoveTime(nextPrintTime float64)
	CheckPause()
}

func RunToolheadDwell(runtime ToolheadDwellRuntime, delay float64) {
	nextPrintTime := runtime.GetLastMoveTime() + math.Max(0.0, delay)
	runtime.AdvanceMoveTime(nextPrintTime)
	runtime.CheckPause()
}

type ToolheadVelocitySettings struct {
	MaxVelocity           float64
	MaxAccel              float64
	RequestedAccelToDecel float64
	SquareCornerVelocity  float64
}

type ToolheadVelocityLimitUpdate struct {
	MaxVelocity           *float64
	MaxAccel              *float64
	RequestedAccelToDecel *float64
	SquareCornerVelocity  *float64
}

type ToolheadVelocityLimitResult struct {
	Settings          ToolheadVelocitySettings
	JunctionDeviation float64
	MaxAccelToDecel   float64
	Summary           string
}

func CalcToolheadJunctionDeviation(squareCornerVelocity float64, maxAccel float64, requestedAccelToDecel float64) (float64, float64) {
	scv2 := squareCornerVelocity * squareCornerVelocity
	junctionDeviation := scv2 * (math.Sqrt(2.0) - 1.0) / maxAccel
	maxAccelToDecel := math.Min(requestedAccelToDecel, maxAccel)
	return junctionDeviation, maxAccelToDecel
}

func ApplyToolheadVelocityLimitUpdate(settings ToolheadVelocitySettings, update ToolheadVelocityLimitUpdate) ToolheadVelocityLimitResult {
	result := ToolheadVelocityLimitResult{Settings: settings}
	if update.MaxVelocity != nil {
		result.Settings.MaxVelocity = *update.MaxVelocity
	}
	if update.MaxAccel != nil {
		result.Settings.MaxAccel = *update.MaxAccel
	}
	if update.SquareCornerVelocity != nil {
		result.Settings.SquareCornerVelocity = *update.SquareCornerVelocity
	}
	if update.RequestedAccelToDecel != nil {
		result.Settings.RequestedAccelToDecel = *update.RequestedAccelToDecel
	}
	result.JunctionDeviation, result.MaxAccelToDecel = CalcToolheadJunctionDeviation(
		result.Settings.SquareCornerVelocity,
		result.Settings.MaxAccel,
		result.Settings.RequestedAccelToDecel,
	)
	result.Summary = fmt.Sprintf("max_velocity: %.6f\nmax_accel: %.6f\nmax_accel_to_decel: %.6f\nsquare_corner_velocity: %.6f",
		result.Settings.MaxVelocity,
		result.Settings.MaxAccel,
		result.Settings.RequestedAccelToDecel,
		result.Settings.SquareCornerVelocity,
	)
	return result
}
