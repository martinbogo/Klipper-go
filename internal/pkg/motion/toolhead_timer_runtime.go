package motion

import "goklipper/common/constants"

type ToolheadPauseRuntime interface {
	PauseState() ToolheadPauseState
	ApplyPauseState(state ToolheadPauseState)
	EnsurePrimingTimer(waketime float64)
}

func CheckToolheadPause(runtime ToolheadPauseRuntime, source PauseTimeSource, config ToolheadPauseConfig) {
	result := RunToolheadPauseCheck(runtime.PauseState(), source, config)
	runtime.ApplyPauseState(result.State)
	if result.NeedsPrimingTimer {
		runtime.EnsurePrimingTimer(result.PrimingWakeTime)
	}
}

type ToolheadPrimingRuntime interface {
	SpecialQueuingState() string
	PrintTime() float64
	ClearPrimingTimer()
	FlushLookahead()
	SetCheckStallTime(value float64)
}

func HandleToolheadPrimingCallback(runtime ToolheadPrimingRuntime) float64 {
	runtime.ClearPrimingTimer()
	result := HandleToolheadPrimingTimer(runtime.SpecialQueuingState(), runtime.PrintTime())
	if result.ShouldFlushLookahead {
		runtime.FlushLookahead()
		runtime.SetCheckStallTime(result.CheckStallTime)
	}
	return constants.NEVER
}

type ToolheadFlushRuntime interface {
	FlushHandlerState() ToolheadFlushHandlerState
	PrintTime() float64
	FlushLookahead()
	SetCheckStallTime(value float64)
	AdvanceFlushTime(flushTime float64)
	SetDoKickFlushTimer(value bool)
}

func HandleToolheadFlushCallback(eventtime float64, estimatedPrintTime float64, runtime ToolheadFlushRuntime, config ToolheadFlushConfig) float64 {
	plan := BuildToolheadFlushHandlerPlan(eventtime, estimatedPrintTime, runtime.FlushHandlerState(), config)
	if plan.ShouldFlushLookahead {
		printTime := runtime.PrintTime()
		runtime.FlushLookahead()
		if printTime != runtime.PrintTime() {
			runtime.SetCheckStallTime(runtime.PrintTime())
		}
	}
	for _, flushTime := range plan.AdvanceFlushTimes {
		runtime.AdvanceFlushTime(flushTime)
	}
	if plan.KickFlushTimer {
		runtime.SetDoKickFlushTimer(true)
	}
	if plan.ReturnNever {
		return constants.NEVER
	}
	return plan.NextWakeTime
}
