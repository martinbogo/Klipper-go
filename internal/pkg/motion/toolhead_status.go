package motion

import (
	"fmt"
	"math"
)

type ToolheadStatsSnapshot struct {
	PrintTime           float64
	LastFlushTime       float64
	EstimatedPrintTime  float64
	PrintStall          float64
	MoveHistoryExpire   float64
	SpecialQueuingState string
}

type ToolheadStatsResult struct {
	MaxQueueTime     float64
	ClearHistoryTime float64
	BufferTime       float64
	IsActive         bool
	Summary          string
}

func BuildToolheadStats(snapshot ToolheadStatsSnapshot) ToolheadStatsResult {
	maxQueueTime := math.Max(snapshot.PrintTime, snapshot.LastFlushTime)
	clearHistoryTime := snapshot.EstimatedPrintTime - snapshot.MoveHistoryExpire
	bufferTime := snapshot.PrintTime - snapshot.EstimatedPrintTime
	isActive := bufferTime > -60.0 || snapshot.SpecialQueuingState == ""
	displayBufferTime := bufferTime
	if snapshot.SpecialQueuingState == "Drip" {
		displayBufferTime = 0.0
	}
	return ToolheadStatsResult{
		MaxQueueTime:     maxQueueTime,
		ClearHistoryTime: clearHistoryTime,
		BufferTime:       displayBufferTime,
		IsActive:         isActive,
		Summary: fmt.Sprintf("print_time=%.3f buffer_time=%.3f print_stall=%.f",
			snapshot.PrintTime, math.Max(displayBufferTime, 0.0), snapshot.PrintStall),
	}
}

type ToolheadBusyState struct {
	PrintTime          float64
	EstimatedPrintTime float64
	LookaheadEmpty     bool
}

func BuildToolheadBusyState(printTime float64, estimatedPrintTime float64, lookaheadEmpty bool) ToolheadBusyState {
	return ToolheadBusyState{
		PrintTime:          printTime,
		EstimatedPrintTime: estimatedPrintTime,
		LookaheadEmpty:     lookaheadEmpty,
	}
}

type ToolheadStatusSnapshot struct {
	KinematicsStatus      map[string]interface{}
	PrintTime             float64
	EstimatedPrintTime    float64
	PrintStall            float64
	ExtruderName          string
	CommandedPosition     []float64
	MaxVelocity           float64
	MaxAccel              float64
	RequestedAccelToDecel float64
	SquareCornerVelocity  float64
}

func BuildToolheadStatus(snapshot ToolheadStatusSnapshot) map[string]interface{} {
	status := cloneToolheadStatus(snapshot.KinematicsStatus)
	status["print_time"] = snapshot.PrintTime
	status["stalls"] = snapshot.PrintStall
	status["estimated_print_time"] = snapshot.EstimatedPrintTime
	status["extruder"] = snapshot.ExtruderName
	status["position"] = append([]float64{}, snapshot.CommandedPosition...)
	status["max_velocity"] = snapshot.MaxVelocity
	status["max_accel"] = snapshot.MaxAccel
	status["max_accel_to_decel"] = snapshot.RequestedAccelToDecel
	status["square_corner_velocity"] = snapshot.SquareCornerVelocity
	return status
}

func cloneToolheadStatus(status map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(status)+8)
	for key, value := range status {
		cloned[key] = value
	}
	return cloned
}
