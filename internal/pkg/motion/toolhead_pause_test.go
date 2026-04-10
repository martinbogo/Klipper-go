package motion

import (
	"goklipper/common/constants"
	"testing"
)

type fakePauseTimeSource struct {
	monotonic    float64
	estimates    []float64
	pauseReturns []float64
	pauseCalls   []float64
	estimateIdx  int
	pauseIdx     int
}

func (self *fakePauseTimeSource) Monotonic() float64 {
	return self.monotonic
}

func (self *fakePauseTimeSource) EstimatedPrintTime(eventtime float64) float64 {
	_ = eventtime
	if len(self.estimates) == 0 {
		return 0.0
	}
	idx := self.estimateIdx
	if idx >= len(self.estimates) {
		idx = len(self.estimates) - 1
	}
	value := self.estimates[idx]
	self.estimateIdx++
	return value
}

func (self *fakePauseTimeSource) Pause(waketime float64) float64 {
	self.pauseCalls = append(self.pauseCalls, waketime)
	if len(self.pauseReturns) == 0 {
		return waketime
	}
	idx := self.pauseIdx
	if idx >= len(self.pauseReturns) {
		idx = len(self.pauseReturns) - 1
	}
	value := self.pauseReturns[idx]
	self.pauseIdx++
	return value
}

func testToolheadPauseConfig() ToolheadPauseConfig {
	return ToolheadPauseConfig{
		BufferTimeLow:    1.0,
		BufferTimeHigh:   2.0,
		PauseCheckOffset: 0.100,
		MaxPauseDuration: 1.0,
		MinPrimingDelay:  0.100,
		WaitMoveDelay:    0.100,
	}
}

func TestRunToolheadPauseCheckTransitionsToPrimingAndCountsStall(t *testing.T) {
	source := &fakePauseTimeSource{monotonic: 10.0, estimates: []float64{4.0}}
	result := RunToolheadPauseCheck(ToolheadPauseState{
		PrintTime:           5.0,
		CheckStallTime:      4.5,
		PrintStall:          2,
		SpecialQueuingState: "NeedPrime",
		CanPause:            true,
	}, source, testToolheadPauseConfig())

	if result.State.SpecialQueuingState != "Priming" {
		t.Fatalf("expected priming state, got %#v", result.State)
	}
	if result.State.PrintStall != 3 {
		t.Fatalf("expected stall counter increment, got %v", result.State.PrintStall)
	}
	if result.State.CheckStallTime != -1.0 {
		t.Fatalf("expected priming stall marker, got %v", result.State.CheckStallTime)
	}
	if !result.NeedsPrimingTimer || !almostEqualFloat64(result.PrimingWakeTime, 10.1) {
		t.Fatalf("unexpected priming timer result %#v", result)
	}
}

func TestRunToolheadPauseCheckPausesUntilBufferDrops(t *testing.T) {
	source := &fakePauseTimeSource{
		monotonic:    10.0,
		estimates:    []float64{6.5, 7.2},
		pauseReturns: []float64{10.8},
	}
	result := RunToolheadPauseCheck(ToolheadPauseState{
		PrintTime:           9.0,
		SpecialQueuingState: "",
		CanPause:            true,
	}, source, testToolheadPauseConfig())

	if len(source.pauseCalls) != 1 || !almostEqualFloat64(source.pauseCalls[0], 10.5) {
		t.Fatalf("unexpected pause calls %#v", source.pauseCalls)
	}
	if !almostEqualFloat64(result.State.NeedCheckPause, 9.3) {
		t.Fatalf("unexpected next pause check %v", result.State.NeedCheckPause)
	}
	if !almostEqualFloat64(result.BufferTime, 1.8) {
		t.Fatalf("unexpected final buffer time %v", result.BufferTime)
	}
}

func TestRunToolheadPauseCheckStopsWhenPauseDisabled(t *testing.T) {
	source := &fakePauseTimeSource{monotonic: 10.0, estimates: []float64{6.0}}
	result := RunToolheadPauseCheck(ToolheadPauseState{
		PrintTime:           9.0,
		SpecialQueuingState: "",
		CanPause:            false,
	}, source, testToolheadPauseConfig())

	if result.State.NeedCheckPause != constants.NEVER {
		t.Fatalf("expected NEVER pause wakeup, got %v", result.State.NeedCheckPause)
	}
	if len(source.pauseCalls) != 0 {
		t.Fatalf("did not expect pause calls, got %#v", source.pauseCalls)
	}
}

func TestHandleToolheadPrimingTimer(t *testing.T) {
	result := HandleToolheadPrimingTimer("Priming", 12.5)
	if !result.ShouldFlushLookahead || !almostEqualFloat64(result.CheckStallTime, 12.5) {
		t.Fatalf("unexpected priming timer result %#v", result)
	}
	if other := HandleToolheadPrimingTimer("", 1.0); other.ShouldFlushLookahead {
		t.Fatalf("expected empty result outside priming state, got %#v", other)
	}
}

func TestWaitToolheadMovesPausesUntilPrintCatchesUp(t *testing.T) {
	source := &fakePauseTimeSource{
		monotonic:    5.0,
		estimates:    []float64{4.0, 9.1},
		pauseReturns: []float64{5.1},
	}
	endTime := WaitToolheadMoves(ToolheadWaitMovesState{
		SpecialQueuingState: "NeedPrime",
		PrintTime:           9.0,
		CanPause:            true,
	}, source, testToolheadPauseConfig())

	if len(source.pauseCalls) != 1 || !almostEqualFloat64(source.pauseCalls[0], 5.1) {
		t.Fatalf("unexpected wait-move pause calls %#v", source.pauseCalls)
	}
	if !almostEqualFloat64(endTime, 5.1) {
		t.Fatalf("unexpected wait-move end time %v", endTime)
	}
	blocked := WaitToolheadMoves(ToolheadWaitMovesState{SpecialQueuingState: "", PrintTime: 9.0, CanPause: false}, &fakePauseTimeSource{monotonic: 7.0, estimates: []float64{1.0}}, testToolheadPauseConfig())
	if !almostEqualFloat64(blocked, 7.0) {
		t.Fatalf("expected non-pausing wait to return initial monotonic time, got %v", blocked)
	}
}
