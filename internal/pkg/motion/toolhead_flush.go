package motion

import "math"

type ToolheadFlushReset struct {
	SpecialQueuingState string
	NeedCheckPause      float64
	LookaheadFlushTime  float64
	CheckStallTime      float64
}

func BuildToolheadFlushReset(lookaheadFlushTime float64) ToolheadFlushReset {
	return ToolheadFlushReset{
		SpecialQueuingState: "NeedPrime",
		NeedCheckPause:      -1.0,
		LookaheadFlushTime:  lookaheadFlushTime,
		CheckStallTime:      0.0,
	}
}

type ToolheadFlushConfig struct {
	BufferTimeLow    float64
	BgFlushLowTime   float64
	BgFlushBatchTime float64
	BgFlushExtraTime float64
}

type ToolheadFlushHandlerState struct {
	PrintTime           float64
	LastFlushTime       float64
	NeedFlushTime       float64
	SpecialQueuingState string
}

type ToolheadFlushHandlerPlan struct {
	ShouldFlushLookahead bool
	AdvanceFlushTimes    []float64
	NextWakeTime         float64
	ReturnNever          bool
	KickFlushTimer       bool
}

func BuildToolheadFlushHandlerPlan(eventtime float64, estimatedPrintTime float64, state ToolheadFlushHandlerState, config ToolheadFlushConfig) ToolheadFlushHandlerPlan {
	if state.SpecialQueuingState == "" {
		bufferTime := state.PrintTime - estimatedPrintTime
		if bufferTime > config.BufferTimeLow {
			return ToolheadFlushHandlerPlan{NextWakeTime: eventtime + bufferTime - config.BufferTimeLow}
		}
	}

	plan := ToolheadFlushHandlerPlan{ShouldFlushLookahead: state.SpecialQueuingState == ""}
	lastFlushTime := state.LastFlushTime
	for {
		endFlush := state.NeedFlushTime + config.BgFlushExtraTime
		if lastFlushTime >= endFlush {
			plan.ReturnNever = true
			plan.KickFlushTimer = true
			return plan
		}
		bufferTime := lastFlushTime - estimatedPrintTime
		if bufferTime > config.BgFlushLowTime {
			plan.NextWakeTime = eventtime + bufferTime - config.BgFlushLowTime
			return plan
		}
		flushTime := math.Min(endFlush, estimatedPrintTime+config.BgFlushLowTime+config.BgFlushBatchTime)
		flushTime = math.Max(flushTime, lastFlushTime)
		plan.AdvanceFlushTimes = append(plan.AdvanceFlushTimes, flushTime)
		lastFlushTime = flushTime
	}
}
