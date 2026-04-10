package motion

import (
	"math"
	"reflect"
	"testing"
)

type fakeMoveQueueFlusher struct {
	calls [][2]float64
}

func (self *fakeMoveQueueFlusher) FlushMoves(flushTime float64, clearHistoryTime float64) {
	self.calls = append(self.calls, [2]float64{flushTime, clearHistoryTime})
}

type fakePrintTimeSource struct {
	monotonic float64
	estimated float64
}

func (self fakePrintTimeSource) Monotonic() float64 {
	return self.monotonic
}

func (self fakePrintTimeSource) EstimatedPrintTime(eventtime float64) float64 {
	return self.estimated
}

type fakeSyncPrintTimeNotifier struct {
	calls [][3]float64
}

func (self *fakeSyncPrintTimeNotifier) SyncPrintTime(curTime float64, estPrintTime float64, printTime float64) {
	self.calls = append(self.calls, [3]float64{curTime, estPrintTime, printTime})
}

func testToolheadTimingConfig() ToolheadTimingConfig {
	return ToolheadTimingConfig{
		BufferTimeStart:       0.250,
		MinKinTime:            0.100,
		MoveBatchTime:         0.500,
		MoveHistoryExpire:     30.0,
		ScanTimeOffset:        0.001,
		StepcompressFlushTime: 0.050,
	}
}

func almostEqualFloat64(a float64, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func assertFloat64Pairs(t *testing.T, got [][2]float64, want [][2]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected pair count got=%d want=%d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if !almostEqualFloat64(got[i][0], want[i][0]) || !almostEqualFloat64(got[i][1], want[i][1]) {
			t.Fatalf("unexpected pair at %d got=%#v want=%#v", i, got[i], want[i])
		}
	}
}

func assertFloat64Triples(t *testing.T, got [][3]float64, want [][3]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected triple count got=%d want=%d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if !almostEqualFloat64(got[i][0], want[i][0]) || !almostEqualFloat64(got[i][1], want[i][1]) || !almostEqualFloat64(got[i][2], want[i][2]) {
			t.Fatalf("unexpected triple at %d got=%#v want=%#v", i, got[i], want[i])
		}
	}
}

func TestToolheadTimingStateAdvanceFlushTime(t *testing.T) {
	state := ToolheadTimingState{
		PrintTime:        10.0,
		LastFlushTime:    7.0,
		MinRestartTime:   6.2,
		ClearHistoryTime: 2.5,
		KinFlushDelay:    0.2,
		CanPause:         true,
	}
	stepGenTimes := []float64{}
	finalizeCalls := [][2]float64{}
	extruderCalls := [][2]float64{}
	flusher := &fakeMoveQueueFlusher{}

	state.AdvanceFlushTime(8.0, testToolheadTimingConfig(), FlushActions{
		StepGenerators: []func(float64){func(flushTime float64) {
			stepGenTimes = append(stepGenTimes, flushTime)
		}},
		FinalizeMoves: func(freeTime float64, clearHistoryTime float64) {
			finalizeCalls = append(finalizeCalls, [2]float64{freeTime, clearHistoryTime})
		},
		UpdateExtruderMoveTime: func(freeTime float64, clearHistoryTime float64) {
			extruderCalls = append(extruderCalls, [2]float64{freeTime, clearHistoryTime})
		},
		FlushDrivers: []MoveQueueFlusher{flusher},
	})

	if len(stepGenTimes) != 1 || !almostEqualFloat64(stepGenTimes[0], 8.05) {
		t.Fatalf("unexpected step generator flush times %#v", stepGenTimes)
	}
	assertFloat64Pairs(t, finalizeCalls, [][2]float64{{7.85, 2.5}})
	assertFloat64Pairs(t, extruderCalls, [][2]float64{{7.85, 2.5}})
	assertFloat64Pairs(t, flusher.calls, [][2]float64{{8.0, 2.5}})
	if !almostEqualFloat64(state.LastFlushTime, 8.0) {
		t.Fatalf("unexpected last flush time %v", state.LastFlushTime)
	}
	if !almostEqualFloat64(state.MinRestartTime, 8.05) {
		t.Fatalf("unexpected min restart time %v", state.MinRestartTime)
	}
}

func TestToolheadTimingStateAdvanceFlushTimeExpiresHistoryWithoutPause(t *testing.T) {
	state := ToolheadTimingState{
		PrintTime:        50.0,
		LastFlushTime:    0.0,
		ClearHistoryTime: 12.0,
		KinFlushDelay:    0.1,
		CanPause:         false,
	}
	finalizeCalls := [][2]float64{}

	state.AdvanceFlushTime(40.0, testToolheadTimingConfig(), FlushActions{
		FinalizeMoves: func(freeTime float64, clearHistoryTime float64) {
			finalizeCalls = append(finalizeCalls, [2]float64{freeTime, clearHistoryTime})
		},
	})

	assertFloat64Pairs(t, finalizeCalls, [][2]float64{{39.95, 10.0}})
}

func TestToolheadTimingStateAdvanceMoveTimeBatchesFlushes(t *testing.T) {
	state := ToolheadTimingState{
		PrintTime:      10.0,
		LastFlushTime:  0.0,
		KinFlushDelay:  0.1,
		CanPause:       true,
		MinRestartTime: 0.0,
	}
	flusher := &fakeMoveQueueFlusher{}

	state.AdvanceMoveTime(11.2, testToolheadTimingConfig(), FlushActions{
		FlushDrivers: []MoveQueueFlusher{flusher},
	})

	assertFloat64Pairs(t, flusher.calls, [][2]float64{{10.35, 0.0}, {10.85, 0.0}, {11.05, 0.0}})
	if !almostEqualFloat64(state.PrintTime, 11.2) {
		t.Fatalf("unexpected print time %v", state.PrintTime)
	}
	if !almostEqualFloat64(state.LastFlushTime, 11.05) {
		t.Fatalf("unexpected last flush time %v", state.LastFlushTime)
	}
}

func TestToolheadTimingStateCalcPrintTime(t *testing.T) {
	state := ToolheadTimingState{
		PrintTime:      10.0,
		MinRestartTime: 10.5,
		KinFlushDelay:  0.2,
	}
	notifier := &fakeSyncPrintTimeNotifier{}

	state.CalcPrintTime(testToolheadTimingConfig(), fakePrintTimeSource{monotonic: 12.0, estimated: 10.0}, notifier)

	if !almostEqualFloat64(state.PrintTime, 10.7) {
		t.Fatalf("unexpected print time %v", state.PrintTime)
	}
	assertFloat64Triples(t, notifier.calls, [][3]float64{{12.0, 10.0, 10.7}})
}

func TestToolheadTimingStateCalcPrintTimeNoopWhenAlreadyAhead(t *testing.T) {
	state := ToolheadTimingState{
		PrintTime:      12.0,
		MinRestartTime: 10.5,
		KinFlushDelay:  0.2,
	}
	notifier := &fakeSyncPrintTimeNotifier{}

	state.CalcPrintTime(testToolheadTimingConfig(), fakePrintTimeSource{monotonic: 12.0, estimated: 10.0}, notifier)

	if !almostEqualFloat64(state.PrintTime, 12.0) {
		t.Fatalf("unexpected print time %v", state.PrintTime)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("unexpected notifier calls %#v", notifier.calls)
	}
}

func TestToolheadTimingStateUpdateStepGenerationScanDelay(t *testing.T) {
	state := ToolheadTimingState{
		KinFlushDelay: 0.301,
		KinFlushTimes: []float64{0.1, 0.3},
	}

	state.UpdateStepGenerationScanDelay(0.2, 0.1, testToolheadTimingConfig())

	if !reflect.DeepEqual(state.KinFlushTimes, []float64{0.3, 0.2}) {
		t.Fatalf("unexpected scan times %#v", state.KinFlushTimes)
	}
	if !almostEqualFloat64(state.KinFlushDelay, 0.301) {
		t.Fatalf("unexpected kin flush delay %v", state.KinFlushDelay)
	}
}

func TestToolheadTimingStateNoteMovequeueActivity(t *testing.T) {
	state := ToolheadTimingState{
		NeedFlushTime:    2.0,
		StepGenTime:      3.0,
		DoKickFlushTimer: true,
	}

	if !state.NoteMovequeueActivity(4.0, true) {
		t.Fatal("expected first movequeue activity to kick flush timer")
	}
	if state.NoteMovequeueActivity(3.5, false) {
		t.Fatal("expected second movequeue activity to leave flush timer alone")
	}
	if !almostEqualFloat64(state.NeedFlushTime, 4.0) {
		t.Fatalf("unexpected need flush time %v", state.NeedFlushTime)
	}
	if !almostEqualFloat64(state.StepGenTime, 4.0) {
		t.Fatalf("unexpected step gen time %v", state.StepGenTime)
	}
	if state.DoKickFlushTimer {
		t.Fatal("expected flush timer kick flag to be cleared")
	}
}
