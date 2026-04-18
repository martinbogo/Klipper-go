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
	BufferTimeLow         float64
	BgFlushLowTime        float64
	BgFlushHighTime       float64
	BgFlushSgLowTime      float64
	BgFlushSgHighTime     float64
	BgFlushBatchTime      float64
	BgFlushExtraTime      float64
	StepcompressFlushTime float64
}

type ToolheadFlushHandlerState struct {
	PrintTime           float64
	LastFlushTime       float64
	LastStepGenTime     float64
	NeedFlushTime       float64
	NeedStepGenTime     float64
	SpecialQueuingState string
	KinFlushDelay       float64
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
	lastStepGenTime := state.LastStepGenTime
	aggressiveStepGenTime := state.NeedStepGenTime - 2.0*state.KinFlushDelay
	if lastStepGenTime < aggressiveStepGenTime {
		wantStepGenTime := estimatedPrintTime + config.BgFlushSgHighTime
		batchTime := config.BgFlushSgHighTime - config.BgFlushSgLowTime
		nextBatchTime := lastStepGenTime + batchTime
		if nextBatchTime > estimatedPrintTime {
			if nextBatchTime > wantStepGenTime+0.005 {
				nextBatchTime = lastStepGenTime
			}
			wantStepGenTime = nextBatchTime
		}
		wantStepGenTime = math.Min(wantStepGenTime, aggressiveStepGenTime)
		if wantStepGenTime > lastStepGenTime {
			flushTime := math.Max(lastFlushTime, wantStepGenTime-config.StepcompressFlushTime)
			plan.AdvanceFlushTimes = append(plan.AdvanceFlushTimes, flushTime)
			lastFlushTime = flushTime
			lastStepGenTime = wantStepGenTime
		}
		if lastStepGenTime < aggressiveStepGenTime {
			plan.NextWakeTime = eventtime + lastStepGenTime - config.BgFlushSgLowTime - estimatedPrintTime
			return plan
		}
	}

	endFlush := state.NeedFlushTime + config.BgFlushExtraTime
	if lastFlushTime < endFlush {
		flushTime := math.Min(estimatedPrintTime+config.BgFlushHighTime, endFlush)
		if flushTime > lastFlushTime {
			plan.AdvanceFlushTimes = append(plan.AdvanceFlushTimes, flushTime)
			lastFlushTime = flushTime
		}
	}
	if lastFlushTime >= endFlush {
		plan.ReturnNever = true
		plan.KickFlushTimer = true
		return plan
	}
	plan.NextWakeTime = eventtime + lastFlushTime - config.BgFlushLowTime - estimatedPrintTime
	return plan
}
