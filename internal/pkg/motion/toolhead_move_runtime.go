package motion

import (
	"errors"
	"goklipper/common/constants"
)

type ToolheadWaitRuntime interface {
	FlushLookahead()
	WaitMovesState() ToolheadWaitMovesState
}

func HandleToolheadWaitMoves(runtime ToolheadWaitRuntime, source PauseTimeSource, config ToolheadPauseConfig) float64 {
	runtime.FlushLookahead()
	return WaitToolheadMoves(runtime.WaitMovesState(), source, config)
}

type ToolheadDripMoveRuntime interface {
	KinFlushDelay() float64
	Dwell(delay float64)
	FlushLookaheadQueue(lazy bool)
	SetSpecialQueuingState(state string)
	SetNeedCheckPause(value float64)
	UpdateFlushTimer(waketime float64)
	SetDoKickFlushTimer(value bool)
	SetLookaheadFlushTime(value float64)
	SetCheckStallTime(value float64)
	SetDripCompletion(completion DripCompletion)
	SubmitMove(newpos []float64, speed float64)
	FlushStepGeneration()
	ResetLookaheadQueue()
	FinalizeDripMoves()
	IsCommandError(recovered interface{}) bool
}

type ToolheadDripMoveConfig struct {
	LookaheadFlushTime float64
}

func RunToolheadDripMove(runtime ToolheadDripMoveRuntime, newpos []float64, speed float64, completion DripCompletion, config ToolheadDripMoveConfig) {
	runtime.Dwell(runtime.KinFlushDelay())
	runtime.FlushLookaheadQueue(false)
	runtime.SetSpecialQueuingState("Drip")
	runtime.SetNeedCheckPause(constants.NEVER)
	runtime.UpdateFlushTimer(constants.NEVER)
	runtime.SetDoKickFlushTimer(false)
	runtime.SetLookaheadFlushTime(config.LookaheadFlushTime)
	runtime.SetCheckStallTime(0.0)
	runtime.SetDripCompletion(completion)
	runToolheadDripSubmitMove(runtime, newpos, speed)
	runToolheadDripFlush(runtime, false)
	runtime.UpdateFlushTimer(constants.NOW)
	runtime.FlushStepGeneration()
}

func runToolheadDripSubmitMove(runtime ToolheadDripMoveRuntime, newpos []float64, speed float64) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if runtime.IsCommandError(recovered) {
				runtime.UpdateFlushTimer(constants.NOW)
				runtime.FlushStepGeneration()
			}
			panic(recovered)
		}
	}()
	runtime.SubmitMove(newpos, speed)
}

func runToolheadDripFlush(runtime ToolheadDripMoveRuntime, lazy bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			recoveredErr, ok := recovered.(error)
			if ok && errors.Is(recoveredErr, ErrDripModeEnd) {
				runtime.ResetLookaheadQueue()
				runtime.FinalizeDripMoves()
				return
			}
			panic(recovered)
		}
	}()
	runtime.FlushLookaheadQueue(lazy)
}
