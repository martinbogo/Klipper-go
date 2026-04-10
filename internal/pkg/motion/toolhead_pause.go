package motion

import (
	"goklipper/common/constants"
	"math"
)

type PauseTimeSource interface {
	Monotonic() float64
	EstimatedPrintTime(eventtime float64) float64
	Pause(waketime float64) float64
}

type ToolheadPauseConfig struct {
	BufferTimeLow    float64
	BufferTimeHigh   float64
	PauseCheckOffset float64
	MaxPauseDuration float64
	MinPrimingDelay  float64
	WaitMoveDelay    float64
}

type ToolheadPauseState struct {
	PrintTime           float64
	CheckStallTime      float64
	PrintStall          float64
	SpecialQueuingState string
	NeedCheckPause      float64
	CanPause            bool
}

type ToolheadPauseResult struct {
	State              ToolheadPauseState
	NeedsPrimingTimer  bool
	PrimingWakeTime    float64
	EventTime          float64
	EstimatedPrintTime float64
	BufferTime         float64
}

func RunToolheadPauseCheck(state ToolheadPauseState, source PauseTimeSource, config ToolheadPauseConfig) ToolheadPauseResult {
	eventtime := source.Monotonic()
	estPrintTime := source.EstimatedPrintTime(eventtime)
	bufferTime := state.PrintTime - estPrintTime
	result := ToolheadPauseResult{State: state, EventTime: eventtime, EstimatedPrintTime: estPrintTime, BufferTime: bufferTime}

	if len(result.State.SpecialQueuingState) > 0 {
		if result.State.CheckStallTime > 0.0 {
			if estPrintTime < result.State.CheckStallTime {
				result.State.PrintStall += 1
			}
			result.State.CheckStallTime = 0.0
		}
		result.State.SpecialQueuingState = "Priming"
		result.State.CheckStallTime = -1.0
		result.NeedsPrimingTimer = true
		result.PrimingWakeTime = eventtime + math.Max(config.MinPrimingDelay, bufferTime-config.BufferTimeLow)
	}

	for {
		pauseTime := bufferTime - config.BufferTimeHigh
		if pauseTime <= 0.0 {
			break
		}
		if !result.State.CanPause {
			result.State.NeedCheckPause = constants.NEVER
			result.EstimatedPrintTime = estPrintTime
			result.BufferTime = bufferTime
			return result
		}
		eventtime = source.Pause(eventtime + math.Min(config.MaxPauseDuration, pauseTime))
		estPrintTime = source.EstimatedPrintTime(eventtime)
		bufferTime = result.State.PrintTime - estPrintTime
	}
	if result.State.SpecialQueuingState == "" {
		result.State.NeedCheckPause = estPrintTime + config.BufferTimeHigh + config.PauseCheckOffset
	}
	result.EventTime = eventtime
	result.EstimatedPrintTime = estPrintTime
	result.BufferTime = bufferTime
	return result
}

type ToolheadPrimingResult struct {
	ShouldFlushLookahead bool
	CheckStallTime       float64
}

func HandleToolheadPrimingTimer(specialQueuingState string, printTime float64) ToolheadPrimingResult {
	if specialQueuingState != "Priming" {
		return ToolheadPrimingResult{}
	}
	return ToolheadPrimingResult{ShouldFlushLookahead: true, CheckStallTime: printTime}
}

type ToolheadWaitMovesState struct {
	SpecialQueuingState string
	PrintTime           float64
	CanPause            bool
}

func WaitToolheadMoves(state ToolheadWaitMovesState, source PauseTimeSource, config ToolheadPauseConfig) float64 {
	eventtime := source.Monotonic()
	for state.SpecialQueuingState == "" || state.PrintTime >= source.EstimatedPrintTime(eventtime) {
		if !state.CanPause {
			break
		}
		eventtime = source.Pause(eventtime + config.WaitMoveDelay)
	}
	return eventtime
}
