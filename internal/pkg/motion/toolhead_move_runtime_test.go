package motion

import (
	"errors"
	"testing"

	"goklipper/common/constants"
)

type fakeToolheadWaitRuntime struct {
	events []string
	state  ToolheadWaitMovesState
}

func (self *fakeToolheadWaitRuntime) FlushLookahead() {
	self.events = append(self.events, "flush")
	self.state = ToolheadWaitMovesState{SpecialQueuingState: "NeedPrime", PrintTime: 5.0, CanPause: false}
}

func (self *fakeToolheadWaitRuntime) WaitMovesState() ToolheadWaitMovesState {
	self.events = append(self.events, "state")
	return self.state
}

type fakeToolheadDripMoveRuntime struct {
	kinFlushDelay          float64
	dwellCalls             []float64
	flushCalls             []bool
	specialQueuingState    string
	needCheckPause         float64
	flushTimerUpdates      []float64
	doKickFlushTimer       bool
	lookaheadFlushTime     float64
	checkStallTime         float64
	dripCompletion         DripCompletion
	submitCalls            [][]float64
	submitSpeeds           []float64
	submitPanic            interface{}
	flushPanic             interface{}
	flushPanicCall         int
	flushStepGenerationCnt int
	resetLookaheadCnt      int
	finalizeDripMovesCnt   int
	isCommandError         func(recovered interface{}) bool
}

func (self *fakeToolheadDripMoveRuntime) KinFlushDelay() float64 {
	return self.kinFlushDelay
}

func (self *fakeToolheadDripMoveRuntime) Dwell(delay float64) {
	self.dwellCalls = append(self.dwellCalls, delay)
}

func (self *fakeToolheadDripMoveRuntime) FlushLookaheadQueue(lazy bool) {
	self.flushCalls = append(self.flushCalls, lazy)
	if self.flushPanicCall != 0 && len(self.flushCalls) == self.flushPanicCall {
		panic(self.flushPanic)
	}
}

func (self *fakeToolheadDripMoveRuntime) SetSpecialQueuingState(state string) {
	self.specialQueuingState = state
}

func (self *fakeToolheadDripMoveRuntime) SetNeedCheckPause(value float64) {
	self.needCheckPause = value
}

func (self *fakeToolheadDripMoveRuntime) UpdateFlushTimer(waketime float64) {
	self.flushTimerUpdates = append(self.flushTimerUpdates, waketime)
}

func (self *fakeToolheadDripMoveRuntime) SetDoKickFlushTimer(value bool) {
	self.doKickFlushTimer = value
}

func (self *fakeToolheadDripMoveRuntime) SetLookaheadFlushTime(value float64) {
	self.lookaheadFlushTime = value
}

func (self *fakeToolheadDripMoveRuntime) SetCheckStallTime(value float64) {
	self.checkStallTime = value
}

func (self *fakeToolheadDripMoveRuntime) SetDripCompletion(completion DripCompletion) {
	self.dripCompletion = completion
}

func (self *fakeToolheadDripMoveRuntime) SubmitMove(newpos []float64, speed float64) {
	self.submitCalls = append(self.submitCalls, append([]float64{}, newpos...))
	self.submitSpeeds = append(self.submitSpeeds, speed)
	if self.submitPanic != nil {
		panic(self.submitPanic)
	}
}

func (self *fakeToolheadDripMoveRuntime) FlushStepGeneration() {
	self.flushStepGenerationCnt++
}

func (self *fakeToolheadDripMoveRuntime) ResetLookaheadQueue() {
	self.resetLookaheadCnt++
}

func (self *fakeToolheadDripMoveRuntime) FinalizeDripMoves() {
	self.finalizeDripMovesCnt++
}

func (self *fakeToolheadDripMoveRuntime) IsCommandError(recovered interface{}) bool {
	if self.isCommandError != nil {
		return self.isCommandError(recovered)
	}
	return false
}

type fakeDripCommandError struct{}

func TestHandleToolheadWaitMovesFlushesBeforeReadingState(t *testing.T) {
	runtime := &fakeToolheadWaitRuntime{}
	endTime := HandleToolheadWaitMoves(runtime, &fakePauseTimeSource{monotonic: 7.0, estimates: []float64{1.0}}, testToolheadPauseConfig())

	if len(runtime.events) != 2 || runtime.events[0] != "flush" || runtime.events[1] != "state" {
		t.Fatalf("unexpected call order %#v", runtime.events)
	}
	if endTime != 7.0 {
		t.Fatalf("unexpected end time %v", endTime)
	}
}

func TestRunToolheadDripMoveTransitionsAndFinishes(t *testing.T) {
	completion := &fakeDripCompletion{}
	runtime := &fakeToolheadDripMoveRuntime{kinFlushDelay: 0.2, doKickFlushTimer: true}

	RunToolheadDripMove(runtime, []float64{1.0, 2.0, 3.0}, 45.0, completion, ToolheadDripMoveConfig{LookaheadFlushTime: 2.0})

	if len(runtime.dwellCalls) != 1 || runtime.dwellCalls[0] != 0.2 {
		t.Fatalf("unexpected dwell calls %#v", runtime.dwellCalls)
	}
	if len(runtime.flushCalls) != 2 || runtime.flushCalls[0] || runtime.flushCalls[1] {
		t.Fatalf("unexpected flush calls %#v", runtime.flushCalls)
	}
	if runtime.specialQueuingState != "Drip" || runtime.needCheckPause != constants.NEVER {
		t.Fatalf("unexpected drip state %#v", runtime)
	}
	if len(runtime.flushTimerUpdates) != 2 || runtime.flushTimerUpdates[0] != constants.NEVER || runtime.flushTimerUpdates[1] != constants.NOW {
		t.Fatalf("unexpected flush timer updates %#v", runtime.flushTimerUpdates)
	}
	if runtime.doKickFlushTimer {
		t.Fatalf("expected kick flush timer to be disabled %#v", runtime)
	}
	if runtime.lookaheadFlushTime != 2.0 || runtime.checkStallTime != 0.0 {
		t.Fatalf("unexpected drip runtime config %#v", runtime)
	}
	if runtime.dripCompletion != completion {
		t.Fatalf("unexpected drip completion %#v", runtime.dripCompletion)
	}
	if len(runtime.submitCalls) != 1 || len(runtime.submitCalls[0]) != 3 || runtime.submitSpeeds[0] != 45.0 {
		t.Fatalf("unexpected submit calls %#v speeds %#v", runtime.submitCalls, runtime.submitSpeeds)
	}
	if runtime.flushStepGenerationCnt != 1 || runtime.resetLookaheadCnt != 0 || runtime.finalizeDripMovesCnt != 0 {
		t.Fatalf("unexpected final drip state %#v", runtime)
	}
}

func TestRunToolheadDripMoveCommandErrorFlushesAndRepanics(t *testing.T) {
	runtime := &fakeToolheadDripMoveRuntime{
		kinFlushDelay: 0.1,
		submitPanic:   fakeDripCommandError{},
		isCommandError: func(recovered interface{}) bool {
			_, ok := recovered.(fakeDripCommandError)
			return ok
		},
	}

	defer func() {
		recovered := recover()
		if _, ok := recovered.(fakeDripCommandError); !ok {
			t.Fatalf("expected fakeDripCommandError panic, got %#v", recovered)
		}
		if len(runtime.flushTimerUpdates) != 2 || runtime.flushTimerUpdates[0] != constants.NEVER || runtime.flushTimerUpdates[1] != constants.NOW {
			t.Fatalf("unexpected flush timer updates %#v", runtime.flushTimerUpdates)
		}
		if runtime.flushStepGenerationCnt != 1 {
			t.Fatalf("expected flush step generation on command error %#v", runtime)
		}
		if len(runtime.flushCalls) != 1 {
			t.Fatalf("expected only the drip-entry flush to run %#v", runtime.flushCalls)
		}
	}()

	RunToolheadDripMove(runtime, []float64{9.0}, 12.0, nil, ToolheadDripMoveConfig{LookaheadFlushTime: 2.0})
}

func TestRunToolheadDripMoveSuppressesErrDripModeEndAndFinalizes(t *testing.T) {
	runtime := &fakeToolheadDripMoveRuntime{
		kinFlushDelay:  0.1,
		flushPanic:     ErrDripModeEnd,
		flushPanicCall: 2,
	}

	RunToolheadDripMove(runtime, []float64{9.0}, 12.0, nil, ToolheadDripMoveConfig{LookaheadFlushTime: 2.0})

	if runtime.resetLookaheadCnt != 1 || runtime.finalizeDripMovesCnt != 1 {
		t.Fatalf("expected drip end cleanup %#v", runtime)
	}
	if len(runtime.flushTimerUpdates) != 2 || runtime.flushTimerUpdates[1] != constants.NOW {
		t.Fatalf("unexpected flush timer updates %#v", runtime.flushTimerUpdates)
	}
	if runtime.flushStepGenerationCnt != 1 {
		t.Fatalf("expected final flush step generation %#v", runtime)
	}
}

func TestRunToolheadDripMoveRepanicsUnexpectedFlushError(t *testing.T) {
	panicErr := errors.New("boom")
	runtime := &fakeToolheadDripMoveRuntime{
		kinFlushDelay:  0.1,
		flushPanic:     panicErr,
		flushPanicCall: 2,
	}

	defer func() {
		recovered := recover()
		recoveredErr, ok := recovered.(error)
		if !ok || !errors.Is(recoveredErr, panicErr) {
			t.Fatalf("expected panicErr, got %#v", recovered)
		}
		if runtime.resetLookaheadCnt != 0 || runtime.finalizeDripMovesCnt != 0 {
			t.Fatalf("unexpected drip end cleanup %#v", runtime)
		}
		if len(runtime.flushTimerUpdates) != 1 || runtime.flushTimerUpdates[0] != constants.NEVER {
			t.Fatalf("unexpected flush timer updates %#v", runtime.flushTimerUpdates)
		}
		if runtime.flushStepGenerationCnt != 0 {
			t.Fatalf("did not expect final flush step generation %#v", runtime)
		}
	}()

	RunToolheadDripMove(runtime, []float64{9.0}, 12.0, nil, ToolheadDripMoveConfig{LookaheadFlushTime: 2.0})
}
