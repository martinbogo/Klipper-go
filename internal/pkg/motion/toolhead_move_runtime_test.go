package motion

import (
	"errors"
	"reflect"
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

type fakeProcessMoveRuntime struct {
	core              *ToolheadCoreState
	queuedKinematicAt []float64
	queuedExtruderAt  []float64
	noteCalls         [][2]float64
	advanceCalls      []float64
	canPause          bool
}

func (self *fakeProcessMoveRuntime) QueueKinematicMove(printTime float64, move *Move) {
	_ = move
	self.queuedKinematicAt = append(self.queuedKinematicAt, printTime)
}

func (self *fakeProcessMoveRuntime) QueueExtruderMove(printTime float64, move *Move) {
	_ = move
	self.queuedExtruderAt = append(self.queuedExtruderAt, printTime)
}

func (self *fakeProcessMoveRuntime) PrintTime() float64 {
	return self.core.PrintTime
}

func (self *fakeProcessMoveRuntime) KinFlushDelay() float64 {
	return self.core.KinFlushDelay
}

func (self *fakeProcessMoveRuntime) CanPause() bool {
	return self.canPause
}

func (self *fakeProcessMoveRuntime) NoteMovequeueActivity(mqTime float64, setStepGenTime bool) {
	flag := 0.0
	if setStepGenTime {
		flag = 1.0
	}
	self.noteCalls = append(self.noteCalls, [2]float64{mqTime, flag})
	self.core.NoteMovequeueActivity(mqTime, setStepGenTime)
}

func (self *fakeProcessMoveRuntime) AdvanceMoveTime(nextPrintTime float64) {
	self.advanceCalls = append(self.advanceCalls, nextPrintTime)
	self.core.PrintTime = nextPrintTime
}

func (self *fakeProcessMoveRuntime) Monotonic() float64 {
	return 0.0
}

func (self *fakeProcessMoveRuntime) EstimatedPrintTime(eventtime float64) float64 {
	_ = eventtime
	return 0.0
}

func TestProcessToolheadMoveBatchClearsPrimingStateAndAdvancesTiming(t *testing.T) {
	core := &ToolheadCoreState{
		PrintTime:           5.0,
		SpecialQueuingState: "NeedPrime",
		NeedCheckPause:      2.5,
		KinFlushDelay:       0.1,
		DoKickFlushTimer:    true,
	}
	runtime := &fakeProcessMoveRuntime{core: core, canPause: true}
	calcCount := 0
	moves := []*Move{{
		Is_kinematic_move: true,
		Axes_d:            []float64{1.0, 0.0, 0.0, 1.0},
		Accel_t:           0.2,
		Cruise_t:          0.3,
		Decel_t:           0.1,
	}}

	err := ProcessToolheadMoveBatch(core, moves, runtime, func() {
		calcCount++
		core.PrintTime = 6.0
	}, runtime, runtime, &fakeDripCompletion{}, testToolheadDripConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calcCount != 1 {
		t.Fatalf("expected one calc-print-time call, got %d", calcCount)
	}
	if core.SpecialQueuingState != "" || !almostEqualFloat64(core.NeedCheckPause, -1.0) {
		t.Fatalf("expected priming state reset, got state=%q need=%v", core.SpecialQueuingState, core.NeedCheckPause)
	}
	if !reflect.DeepEqual(runtime.queuedKinematicAt, []float64{6.0}) || !reflect.DeepEqual(runtime.queuedExtruderAt, []float64{6.0}) {
		t.Fatalf("unexpected queued move times kin=%#v ext=%#v", runtime.queuedKinematicAt, runtime.queuedExtruderAt)
	}
	assertFloat64Pairs(t, runtime.noteCalls, [][2]float64{{6.7, 1.0}})
	if !reflect.DeepEqual(runtime.advanceCalls, []float64{6.6}) {
		t.Fatalf("unexpected advance calls %#v", runtime.advanceCalls)
	}
	if !almostEqualFloat64(core.PrintTime, 6.6) {
		t.Fatalf("unexpected core print time %v", core.PrintTime)
	}
}

func TestProcessToolheadMoveBatchPropagatesDripCompletionEnd(t *testing.T) {
	core := &ToolheadCoreState{
		PrintTime:           5.0,
		SpecialQueuingState: "Drip",
		KinFlushDelay:       0.1,
	}
	runtime := &fakeProcessMoveRuntime{core: core, canPause: true}
	moves := []*Move{{
		Is_kinematic_move: true,
		Axes_d:            []float64{1.0, 0.0, 0.0, 0.0},
		Accel_t:           0.2,
	}}

	err := ProcessToolheadMoveBatch(core, moves, runtime, func() {}, runtime, runtime, &fakeDripCompletion{testResults: []bool{true}}, testToolheadDripConfig())
	if !errors.Is(err, ErrDripModeEnd) {
		t.Fatalf("expected ErrDripModeEnd, got %v", err)
	}
	if len(runtime.noteCalls) != 0 {
		t.Fatalf("expected no post-error note calls, got %#v", runtime.noteCalls)
	}
	if len(runtime.advanceCalls) != 0 {
		t.Fatalf("expected no post-error advance calls, got %#v", runtime.advanceCalls)
	}
	if !reflect.DeepEqual(runtime.queuedKinematicAt, []float64{5.0}) {
		t.Fatalf("unexpected queued move times %#v", runtime.queuedKinematicAt)
	}
}
