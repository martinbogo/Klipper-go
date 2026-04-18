package motion

import (
	"testing"

	"goklipper/common/constants"
)

type fakeToolheadPauseRuntime struct {
	state          ToolheadPauseState
	primingWakeups []float64
}

func (self *fakeToolheadPauseRuntime) PauseState() ToolheadPauseState {
	return self.state
}

func (self *fakeToolheadPauseRuntime) ApplyPauseState(state ToolheadPauseState) {
	self.state = state
}

func (self *fakeToolheadPauseRuntime) EnsurePrimingTimer(waketime float64) {
	self.primingWakeups = append(self.primingWakeups, waketime)
}

type fakeToolheadPrimingRuntime struct {
	specialQueuingState string
	printTime           float64
	clearCount          int
	flushCount          int
	checkStallTime      float64
}

func (self *fakeToolheadPrimingRuntime) SpecialQueuingState() string {
	return self.specialQueuingState
}

func (self *fakeToolheadPrimingRuntime) PrintTime() float64 {
	return self.printTime
}

func (self *fakeToolheadPrimingRuntime) ClearPrimingTimer() {
	self.clearCount++
}

func (self *fakeToolheadPrimingRuntime) FlushLookahead() {
	self.flushCount++
}

func (self *fakeToolheadPrimingRuntime) SetCheckStallTime(value float64) {
	self.checkStallTime = value
}

type fakeToolheadFlushRuntime struct {
	state            ToolheadFlushHandlerState
	printTime        float64
	flushed          bool
	printTimeAfter   float64
	checkStallTime   float64
	advanceFlushes   []float64
	doKickFlushTimer bool
}

func (self *fakeToolheadFlushRuntime) FlushHandlerState() ToolheadFlushHandlerState {
	return self.state
}

func (self *fakeToolheadFlushRuntime) PrintTime() float64 {
	return self.printTime
}

func (self *fakeToolheadFlushRuntime) FlushLookahead() {
	self.flushed = true
	self.printTime = self.printTimeAfter
}

func (self *fakeToolheadFlushRuntime) SetCheckStallTime(value float64) {
	self.checkStallTime = value
}

func (self *fakeToolheadFlushRuntime) AdvanceFlushTime(flushTime float64) {
	self.advanceFlushes = append(self.advanceFlushes, flushTime)
}

func (self *fakeToolheadFlushRuntime) SetDoKickFlushTimer(value bool) {
	self.doKickFlushTimer = value
}

func TestCheckToolheadPauseSchedulesPrimingTimer(t *testing.T) {
	runtime := &fakeToolheadPauseRuntime{state: ToolheadPauseState{
		PrintTime:           5.0,
		CheckStallTime:      4.5,
		PrintStall:          2,
		SpecialQueuingState: "NeedPrime",
		CanPause:            true,
	}}
	source := &fakePauseTimeSource{monotonic: 10.0, estimates: []float64{4.0}}

	CheckToolheadPause(runtime, source, testToolheadPauseConfig())

	if runtime.state.SpecialQueuingState != "Priming" || runtime.state.PrintStall != 3 {
		t.Fatalf("unexpected updated pause state %#v", runtime.state)
	}
	if len(runtime.primingWakeups) != 1 || !almostEqualFloat64(runtime.primingWakeups[0], 10.1) {
		t.Fatalf("unexpected priming wakeups %#v", runtime.primingWakeups)
	}
}

func TestHandleToolheadPrimingCallbackClearsTimerAndFlushes(t *testing.T) {
	runtime := &fakeToolheadPrimingRuntime{specialQueuingState: "Priming", printTime: 12.5}

	waketime := HandleToolheadPrimingCallback(runtime)

	if waketime != constants.NEVER {
		t.Fatalf("unexpected waketime %v", waketime)
	}
	if runtime.clearCount != 1 || runtime.flushCount != 1 || !almostEqualFloat64(runtime.checkStallTime, 12.5) {
		t.Fatalf("unexpected priming runtime state %#v", runtime)
	}
}

func TestHandleToolheadPrimingCallbackNoopOutsidePriming(t *testing.T) {
	runtime := &fakeToolheadPrimingRuntime{specialQueuingState: "", printTime: 2.0}

	HandleToolheadPrimingCallback(runtime)

	if runtime.clearCount != 1 {
		t.Fatalf("expected timer clear even on noop, got %#v", runtime)
	}
	if runtime.flushCount != 0 || runtime.checkStallTime != 0 {
		t.Fatalf("unexpected noop priming side effects %#v", runtime)
	}
}

func TestHandleToolheadFlushCallbackFlushesLookaheadAndCompletes(t *testing.T) {
	runtime := &fakeToolheadFlushRuntime{
		state: ToolheadFlushHandlerState{
			PrintTime:           9.0,
			LastFlushTime:       9.4,
			LastStepGenTime:     9.4,
			NeedFlushTime:       9.0,
			NeedStepGenTime:     9.0,
			SpecialQueuingState: "",
			KinFlushDelay:       0.001,
		},
		printTime:      9.0,
		printTimeAfter: 9.2,
	}

	waketime := HandleToolheadFlushCallback(10.0, 9.0, runtime, testToolheadFlushConfig())

	if waketime != constants.NEVER {
		t.Fatalf("expected NEVER waketime, got %v", waketime)
	}
	if !runtime.flushed || !almostEqualFloat64(runtime.checkStallTime, 9.2) || !runtime.doKickFlushTimer {
		t.Fatalf("unexpected flush runtime state %#v", runtime)
	}
	if len(runtime.advanceFlushes) != 0 {
		t.Fatalf("did not expect extra flushes %#v", runtime.advanceFlushes)
	}
}

func TestHandleToolheadFlushCallbackAdvancesFlushes(t *testing.T) {
	runtime := &fakeToolheadFlushRuntime{
		state: ToolheadFlushHandlerState{
			PrintTime:           9.0,
			LastFlushTime:       8.9,
			LastStepGenTime:     9.8,
			NeedFlushTime:       9.6,
			NeedStepGenTime:     10.0,
			SpecialQueuingState: "NeedPrime",
			KinFlushDelay:       0.2,
		},
		printTime:      9.0,
		printTimeAfter: 9.0,
	}

	waketime := HandleToolheadFlushCallback(10.0, 9.0, runtime, testToolheadFlushConfig())

	if runtime.flushed {
		t.Fatalf("did not expect lookahead flush %#v", runtime)
	}
	if len(runtime.advanceFlushes) != 1 || !almostEqualFloat64(runtime.advanceFlushes[0], 9.4) {
		t.Fatalf("unexpected advance flushes %#v", runtime.advanceFlushes)
	}
	if !almostEqualFloat64(waketime, 10.2) {
		t.Fatalf("unexpected next waketime %v", waketime)
	}
	if runtime.doKickFlushTimer {
		t.Fatalf("did not expect kick flush timer %#v", runtime)
	}
}

func TestHandleToolheadFlushCallbackUsesAggressiveStepGenerationPath(t *testing.T) {
	runtime := &fakeToolheadFlushRuntime{
		state: ToolheadFlushHandlerState{
			PrintTime:           9.0,
			LastFlushTime:       8.9,
			LastStepGenTime:     9.1,
			NeedFlushTime:       9.6,
			NeedStepGenTime:     10.0,
			SpecialQueuingState: "NeedPrime",
			KinFlushDelay:       0.2,
		},
		printTime:      9.0,
		printTimeAfter: 9.0,
	}

	waketime := HandleToolheadFlushCallback(100.0, 9.0, runtime, testToolheadFlushConfig())

	if runtime.flushed {
		t.Fatalf("did not expect lookahead flush %#v", runtime)
	}
	if len(runtime.advanceFlushes) != 1 || !almostEqualFloat64(runtime.advanceFlushes[0], 9.3) {
		t.Fatalf("unexpected aggressive advance flushes %#v", runtime.advanceFlushes)
	}
	if !almostEqualFloat64(waketime, 99.9) {
		t.Fatalf("unexpected aggressive waketime %v", waketime)
	}
	if runtime.doKickFlushTimer {
		t.Fatalf("did not expect kick flush timer %#v", runtime)
	}
}
