package motion

import (
	"errors"
	"testing"
)

type fakeDripCompletion struct {
	testResults []bool
	testIndex   int
	waitCalls   []float64
}

func (self *fakeDripCompletion) Test() bool {
	if len(self.testResults) == 0 {
		return false
	}
	idx := self.testIndex
	if idx >= len(self.testResults) {
		idx = len(self.testResults) - 1
	}
	self.testIndex++
	return self.testResults[idx]
}

func (self *fakeDripCompletion) Wait(waketime float64, waketimeResult interface{}) interface{} {
	_ = waketimeResult
	self.waitCalls = append(self.waitCalls, waketime)
	return nil
}

type fakeDripTimeSource struct {
	monotonic []float64
	estimates []float64
	monoIdx   int
	estIdx    int
}

func (self *fakeDripTimeSource) Monotonic() float64 {
	if len(self.monotonic) == 0 {
		return 0.0
	}
	idx := self.monoIdx
	if idx >= len(self.monotonic) {
		idx = len(self.monotonic) - 1
	}
	self.monoIdx++
	return self.monotonic[idx]
}

func (self *fakeDripTimeSource) EstimatedPrintTime(eventtime float64) float64 {
	_ = eventtime
	if len(self.estimates) == 0 {
		return 0.0
	}
	idx := self.estIdx
	if idx >= len(self.estimates) {
		idx = len(self.estimates) - 1
	}
	self.estIdx++
	return self.estimates[idx]
}

type fakeDripRuntime struct {
	printTime    float64
	kinFlushDelay float64
	canPause     bool
	noteCalls    [][2]float64
	advanceCalls []float64
}

func (self *fakeDripRuntime) PrintTime() float64 {
	return self.printTime
}

func (self *fakeDripRuntime) KinFlushDelay() float64 {
	return self.kinFlushDelay
}

func (self *fakeDripRuntime) CanPause() bool {
	return self.canPause
}

func (self *fakeDripRuntime) NoteMovequeueActivity(mqTime float64, setStepGenTime bool) {
	flag := 0.0
	if setStepGenTime {
		flag = 1.0
	}
	self.noteCalls = append(self.noteCalls, [2]float64{mqTime, flag})
}

func (self *fakeDripRuntime) AdvanceMoveTime(nextPrintTime float64) {
	self.advanceCalls = append(self.advanceCalls, nextPrintTime)
	self.printTime = nextPrintTime
}

func testToolheadDripConfig() ToolheadDripConfig {
	return ToolheadDripConfig{
		DripTime:             0.100,
		StepcompressFlushTime: 0.050,
		DripSegmentTime:      0.050,
	}
}

func TestUpdateToolheadDripTimeStopsWhenCompletionSignalsEnd(t *testing.T) {
	runtime := &fakeDripRuntime{printTime: 5.0, kinFlushDelay: 0.2, canPause: true}
	completion := &fakeDripCompletion{testResults: []bool{true}}
	source := &fakeDripTimeSource{}

	err := UpdateToolheadDripTime(runtime, source, completion, 6.0, testToolheadDripConfig())
	if !errors.Is(err, ErrDripModeEnd) {
		t.Fatalf("expected ErrDripModeEnd, got %v", err)
	}
	if len(runtime.advanceCalls) != 0 {
		t.Fatalf("expected no move advancement, got %#v", runtime.advanceCalls)
	}
}

func TestUpdateToolheadDripTimeWaitsUntilQueueNeedsMoreMoves(t *testing.T) {
	runtime := &fakeDripRuntime{printTime: 10.0, kinFlushDelay: 0.2, canPause: true}
	completion := &fakeDripCompletion{testResults: []bool{false, false, false}}
	source := &fakeDripTimeSource{monotonic: []float64{20.0, 20.0, 20.0}, estimates: []float64{8.0, 10.0, 10.0}}

	err := UpdateToolheadDripTime(runtime, source, completion, 10.1, testToolheadDripConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(completion.waitCalls) != 1 || !almostEqualFloat64(completion.waitCalls[0], 21.65) {
		t.Fatalf("unexpected wait calls %#v", completion.waitCalls)
	}
	assertFloat64Pairs(t, runtime.noteCalls, [][2]float64{{10.25, 1.0}, {10.3, 1.0}})
	if len(runtime.advanceCalls) != 2 || !almostEqualFloat64(runtime.advanceCalls[0], 10.05) || !almostEqualFloat64(runtime.advanceCalls[1], 10.1) {
		t.Fatalf("unexpected advance calls %#v", runtime.advanceCalls)
	}
}

func TestUpdateToolheadDripTimeSkipsWaitWhenPauseDisabled(t *testing.T) {
	runtime := &fakeDripRuntime{printTime: 3.0, kinFlushDelay: 0.1, canPause: false}
	completion := &fakeDripCompletion{testResults: []bool{false, false}}
	source := &fakeDripTimeSource{monotonic: []float64{1.0, 1.0}, estimates: []float64{0.0, 0.0}}

	err := UpdateToolheadDripTime(runtime, source, completion, 3.05, testToolheadDripConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(completion.waitCalls) != 0 {
		t.Fatalf("expected no wait calls, got %#v", completion.waitCalls)
	}
	if len(runtime.advanceCalls) != 1 || !almostEqualFloat64(runtime.advanceCalls[0], 3.05) {
		t.Fatalf("unexpected advance calls %#v", runtime.advanceCalls)
	}
}