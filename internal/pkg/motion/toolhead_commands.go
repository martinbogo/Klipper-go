package motion

import (
	"fmt"
	"math"
)

type ToolheadCommand interface {
	Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64
	Get_commandline() string
	RespondInfo(msg string, log bool)
}

type ToolheadCommandContext interface {
	Dwell(delay float64)
	WaitMoves()
	VelocitySettings() ToolheadVelocitySettings
	ApplyVelocityLimitResult(result ToolheadVelocityLimitResult)
	SetRolloverInfo(msg string)
}

func HandleToolheadG4Command(context ToolheadCommandContext, command ToolheadCommand) error {
	minval := 0.0
	delay := command.Get_float("P", 0.0, &minval, nil, nil, nil) / 1000.0
	context.Dwell(delay)
	return nil
}

func HandleToolheadM400Command(context ToolheadCommandContext) error {
	context.WaitMoves()
	return nil
}

func HandleToolheadSetVelocityLimitCommand(context ToolheadCommandContext, command ToolheadCommand) (ToolheadVelocityLimitResult, bool) {
	above := 0.0
	minval := 0.0
	below := 1.0
	maxVelocity := command.Get_float("VELOCITY", math.NaN(), nil, nil, &above, nil)
	maxAccel := command.Get_float("ACCEL", math.NaN(), nil, nil, &above, nil)
	squareCornerVelocity := command.Get_float("SQUARE_CORNER_VELOCITY", math.NaN(), &minval, nil, nil, nil)
	requestedAccelToDecel := command.Get_float("ACCEL_TO_DECEL", math.NaN(), nil, nil, &above, nil)
	minimumCruiseRatio := command.Get_float("MINIMUM_CRUISE_RATIO", math.NaN(), &minval, nil, nil, &below)
	toOptional := func(value float64) *float64 {
		if math.IsNaN(value) {
			return nil
		}
		valueCopy := value
		return &valueCopy
	}
	effectiveMaxAccel := context.VelocitySettings().MaxAccel
	if !math.IsNaN(maxAccel) {
		effectiveMaxAccel = maxAccel
	}
	requestedAccelToDecelOptional := toOptional(requestedAccelToDecel)
	if !math.IsNaN(minimumCruiseRatio) {
		requestedFromMCR := effectiveMaxAccel * (1.0 - minimumCruiseRatio)
		requestedAccelToDecelOptional = &requestedFromMCR
	}
	result := ApplyToolheadVelocityLimitUpdate(context.VelocitySettings(), ToolheadVelocityLimitUpdate{
		MaxVelocity:           toOptional(maxVelocity),
		MaxAccel:              toOptional(maxAccel),
		RequestedAccelToDecel: requestedAccelToDecelOptional,
		SquareCornerVelocity:  toOptional(squareCornerVelocity),
	})
	context.ApplyVelocityLimitResult(result)
	context.SetRolloverInfo(fmt.Sprintf("toolhead: %s", result.Summary))
	queryOnly := math.IsNaN(maxVelocity) && math.IsNaN(maxAccel) && math.IsNaN(squareCornerVelocity) && math.IsNaN(requestedAccelToDecel) && math.IsNaN(minimumCruiseRatio)
	return result, queryOnly
}

func ApplyToolheadAcceleration(context ToolheadCommandContext, accel float64) {
	result := ApplyToolheadVelocityLimitUpdate(context.VelocitySettings(), ToolheadVelocityLimitUpdate{MaxAccel: &accel})
	context.ApplyVelocityLimitResult(result)
}

func HandleToolheadM204Command(context ToolheadCommandContext, command ToolheadCommand) error {
	above := 0.0
	accel := command.Get_float("S", math.NaN(), nil, nil, &above, nil)
	if math.IsNaN(accel) {
		p := command.Get_float("P", math.NaN(), nil, nil, &above, nil)
		t := command.Get_float("T", math.NaN(), nil, nil, &above, nil)

		if !math.IsNaN(p) && !math.IsNaN(t) {
			accel = math.Min(p, t)
		} else if !math.IsNaN(p) {
			accel = p
		} else if !math.IsNaN(t) {
			accel = t
		} else {
			command.RespondInfo(fmt.Sprintf("Invalid M204 command: %s", command.Get_commandline()), true)
			return nil
		}
	}

	ApplyToolheadAcceleration(context, accel)
	return nil
}
