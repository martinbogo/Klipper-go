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

type ToolheadStatsRuntime interface {
	PrintTime() float64
	LastFlushTime() float64
	EstimatedPrintTime(eventtime float64) float64
	PrintStall() float64
	SpecialQueuingState() string
	SetClearHistoryTime(value float64)
	CheckActiveDrivers(maxQueueTime float64, eventtime float64)
}

func BuildToolheadStatsReport(runtime ToolheadStatsRuntime, eventtime float64, moveHistoryExpire float64) (bool, string) {
	stats := BuildToolheadStats(ToolheadStatsSnapshot{
		PrintTime:           runtime.PrintTime(),
		LastFlushTime:       runtime.LastFlushTime(),
		EstimatedPrintTime:  runtime.EstimatedPrintTime(eventtime),
		PrintStall:          runtime.PrintStall(),
		MoveHistoryExpire:   moveHistoryExpire,
		SpecialQueuingState: runtime.SpecialQueuingState(),
	})
	runtime.CheckActiveDrivers(stats.MaxQueueTime, eventtime)
	runtime.SetClearHistoryTime(stats.ClearHistoryTime)
	return stats.IsActive, stats.Summary
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

type ToolheadBusyRuntime interface {
	PrintTime() float64
	EstimatedPrintTime(eventtime float64) float64
	LookaheadEmpty() bool
}

func BuildToolheadBusyReport(runtime ToolheadBusyRuntime, eventtime float64) ToolheadBusyState {
	return BuildToolheadBusyState(
		runtime.PrintTime(),
		runtime.EstimatedPrintTime(eventtime),
		runtime.LookaheadEmpty(),
	)
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
	status["minimum_cruise_ratio"] = CalcMinimumCruiseRatio(snapshot.MaxAccel, snapshot.RequestedAccelToDecel)
	status["max_accel_to_decel"] = snapshot.RequestedAccelToDecel
	status["square_corner_velocity"] = snapshot.SquareCornerVelocity
	return status
}

type ToolheadStatusRuntime interface {
	KinematicsStatus(eventtime float64) map[string]interface{}
	PrintTime() float64
	EstimatedPrintTime(eventtime float64) float64
	PrintStall() float64
	ExtruderName() string
	CommandedPosition() []float64
	VelocitySettings() ToolheadVelocitySettings
}

func BuildToolheadStatusReport(runtime ToolheadStatusRuntime, eventtime float64) map[string]interface{} {
	velocitySettings := runtime.VelocitySettings()
	return BuildToolheadStatus(ToolheadStatusSnapshot{
		KinematicsStatus:      runtime.KinematicsStatus(eventtime),
		PrintTime:             runtime.PrintTime(),
		EstimatedPrintTime:    runtime.EstimatedPrintTime(eventtime),
		PrintStall:            runtime.PrintStall(),
		ExtruderName:          runtime.ExtruderName(),
		CommandedPosition:     runtime.CommandedPosition(),
		MaxVelocity:           velocitySettings.MaxVelocity,
		MaxAccel:              velocitySettings.MaxAccel,
		RequestedAccelToDecel: velocitySettings.RequestedAccelToDecel,
		SquareCornerVelocity:  velocitySettings.SquareCornerVelocity,
	})
}

type ToolheadHomingStatusRuntime interface {
	KinematicsStatus(eventtime float64) map[string]interface{}
}

func ToolheadHomedAxes(runtime ToolheadHomingStatusRuntime, eventtime float64) string {
	status := runtime.KinematicsStatus(eventtime)
	axes, _ := status["homed_axes"].(string)
	return axes
}

func NoteToolheadZNotHomed(kinematics interface{}) {
	if kin, ok := kinematics.(interface{ Note_z_not_homed() }); ok {
		kin.Note_z_not_homed()
	}
}

func cloneToolheadStatus(status map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(status)+8)
	for key, value := range status {
		cloned[key] = value
	}
	return cloned
}
