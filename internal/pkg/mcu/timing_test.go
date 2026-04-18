package mcu

import (
	"errors"
	"reflect"
	"testing"
)

type fakeQuerySlotSource struct {
	monotonic float64
	estimated float64
	clockRate int64
}

func (self *fakeQuerySlotSource) SecondsToClock(time float64) int64 {
	return int64(time * float64(self.clockRate))
}

func (self *fakeQuerySlotSource) EstimatedPrintTime(eventtime float64) float64 {
	_ = eventtime
	return self.estimated
}

func (self *fakeQuerySlotSource) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeQuerySlotSource) PrintTimeToClock(printTime float64) int64 {
	return int64(printTime * float64(self.clockRate))
}

type fakeStepperSync struct {
	setTimeCalls [][]float64
	flushCalls   [][2]uint64
	flushRet     int
}

func (self *fakeStepperSync) SetTime(offset float64, freq float64) {
	self.setTimeCalls = append(self.setTimeCalls, []float64{offset, freq})
}

func (self *fakeStepperSync) Flush(clock uint64, clearHistoryClock uint64) int {
	self.flushCalls = append(self.flushCalls, [2]uint64{clock, clearHistoryClock})
	return self.flushRet
}

type fakeMoveQueueTimingSource struct {
	clockRate         int64
	calibrateClock    []float64
	clockSyncActive   bool
	fileoutput        bool
	negativePrintTime map[float64]bool
}

func (self *fakeMoveQueueTimingSource) PrintTimeToClock(printTime float64) int64 {
	if self.negativePrintTime != nil && self.negativePrintTime[printTime] {
		return -1
	}
	return int64(printTime * float64(self.clockRate))
}

func (self *fakeMoveQueueTimingSource) CalibrateClock(printTime float64, eventtime float64) []float64 {
	_, _ = printTime, eventtime
	return append([]float64(nil), self.calibrateClock...)
}

func (self *fakeMoveQueueTimingSource) ClockSyncActive() bool {
	return self.clockSyncActive
}

func (self *fakeMoveQueueTimingSource) IsFileoutput() bool {
	return self.fileoutput
}

type fakeStepGenerationOps struct {
	activeTime       float64
	generateRet      int32
	checkActiveCalls []float64
	generateCalls    []float64
}

func (self *fakeStepGenerationOps) CheckActive(flushTime float64) float64 {
	self.checkActiveCalls = append(self.checkActiveCalls, flushTime)
	return self.activeTime
}

func (self *fakeStepGenerationOps) Generate(flushTime float64) int32 {
	self.generateCalls = append(self.generateCalls, flushTime)
	return self.generateRet
}

func TestQuerySlotUsesEstimatedPrintTimeAndOidSpacing(t *testing.T) {
	source := &fakeQuerySlotSource{monotonic: 12.0, estimated: 4.0, clockRate: 1000}
	slot := QuerySlot(3, source)
	if slot != 5530 {
		t.Fatalf("unexpected query slot %d", slot)
	}
}

func TestFlushMovesInvokesCallbacksAndFlushesSteppersync(t *testing.T) {
	source := &fakeMoveQueueTimingSource{clockRate: 1000}
	sync := &fakeStepperSync{}
	callbackCalls := [][2]float64{}
	err := FlushMoves(4.25, 1.5, source, sync, []func(float64, int64){func(printTime float64, clock int64) {
		callbackCalls = append(callbackCalls, [2]float64{printTime, float64(clock)})
	}})
	if err != nil {
		t.Fatalf("unexpected flush error %v", err)
	}
	if !reflect.DeepEqual(callbackCalls, [][2]float64{{4.25, 4250}}) {
		t.Fatalf("unexpected callback calls %#v", callbackCalls)
	}
	if !reflect.DeepEqual(sync.flushCalls, [][2]uint64{{4250, 1500}}) {
		t.Fatalf("unexpected flush calls %#v", sync.flushCalls)
	}
}

func TestFlushMovesSkipsNegativeClock(t *testing.T) {
	source := &fakeMoveQueueTimingSource{clockRate: 1000, negativePrintTime: map[float64]bool{4.25: true}}
	sync := &fakeStepperSync{}
	err := FlushMoves(4.25, 1.5, source, sync, []func(float64, int64){func(float64, int64) { t.Fatal("unexpected callback invocation") }})
	if err != nil {
		t.Fatalf("unexpected flush error %v", err)
	}
	if len(sync.flushCalls) != 0 {
		t.Fatalf("unexpected flush calls %#v", sync.flushCalls)
	}
}

func TestFlushMovesReturnsStepcompressError(t *testing.T) {
	source := &fakeMoveQueueTimingSource{clockRate: 1000}
	sync := &fakeStepperSync{flushRet: 1}
	err := FlushMoves(4.25, 1.5, source, sync, nil)
	if !errors.Is(err, ErrStepcompress) {
		t.Fatalf("expected stepcompress error, got %v", err)
	}
}

func TestMoveQueueTimingStateCheckActiveSetsTimeAndFlagsTimeout(t *testing.T) {
	source := &fakeMoveQueueTimingSource{clockRate: 1000, calibrateClock: []float64{1.5, 48000000}}
	sync := &fakeStepperSync{}
	state := &MoveQueueTimingState{}
	timedOut := state.CheckActive(10.0, 12.0, source, sync)
	if !timedOut {
		t.Fatal("expected timeout indication")
	}
	if !state.IsTimeout {
		t.Fatal("expected timeout state to be latched")
	}
	if !reflect.DeepEqual(sync.setTimeCalls, [][]float64{{1.5, 48000000}}) {
		t.Fatalf("unexpected set time calls %#v", sync.setTimeCalls)
	}
}

func TestMoveQueueTimingStateCheckActiveSkipsTimeoutWhenClockActive(t *testing.T) {
	source := &fakeMoveQueueTimingSource{clockRate: 1000, calibrateClock: []float64{1.5, 48000000}, clockSyncActive: true}
	sync := &fakeStepperSync{}
	state := &MoveQueueTimingState{}
	if state.CheckActive(10.0, 12.0, source, sync) {
		t.Fatal("expected no timeout while clock sync is active")
	}
	if state.IsTimeout {
		t.Fatal("expected timeout state to remain false")
	}
}

func TestBuildMoveQueueTimeoutPlan(t *testing.T) {
	plan := BuildMoveQueueTimeoutPlan(true, "mcu", 12.5)
	if !plan.TimedOut {
		t.Fatal("expected timeout plan to mark timeout")
	}
	if plan.ShutdownMessage != "Lost communication with MCU mcu" {
		t.Fatalf("unexpected shutdown message %q", plan.ShutdownMessage)
	}
	if plan.LogMessage != "Timeout with MCU 'mcu' (eventtime=12.500000), ERROR:Lost communication with MCU mcu" {
		t.Fatalf("unexpected log message %q", plan.LogMessage)
	}

	skipped := BuildMoveQueueTimeoutPlan(false, "mcu", 12.5)
	if skipped.TimedOut || skipped.ShutdownMessage != "" || skipped.LogMessage != "" {
		t.Fatalf("expected empty skipped plan, got %#v", skipped)
	}
}

func TestStepGenerationStateGenerateStepsRunsCallbacksAndClearsThem(t *testing.T) {
	ops := &fakeStepGenerationOps{activeTime: 8.5}
	callbackTimes := []float64{}
	state := &StepGenerationState{ActiveCallbacks: []func(float64){func(printTime float64) {
		callbackTimes = append(callbackTimes, printTime)
	}}}
	if err := state.GenerateSteps(10.0, ops); err != nil {
		t.Fatalf("unexpected generation error %v", err)
	}
	if !reflect.DeepEqual(callbackTimes, []float64{8.5}) {
		t.Fatalf("unexpected callback times %#v", callbackTimes)
	}
	if len(state.ActiveCallbacks) != 0 {
		t.Fatalf("expected callbacks to be cleared, got %#v", state.ActiveCallbacks)
	}
	if !reflect.DeepEqual(ops.checkActiveCalls, []float64{10.0}) {
		t.Fatalf("unexpected active checks %#v", ops.checkActiveCalls)
	}
	if !reflect.DeepEqual(ops.generateCalls, []float64{10.0}) {
		t.Fatalf("unexpected generate calls %#v", ops.generateCalls)
	}
}

func TestStepGenerationStateGenerateStepsReturnsStepcompressError(t *testing.T) {
	ops := &fakeStepGenerationOps{generateRet: 1}
	state := &StepGenerationState{}
	err := state.GenerateSteps(10.0, ops)
	if !errors.Is(err, ErrStepcompress) {
		t.Fatalf("expected stepcompress error, got %v", err)
	}
}
