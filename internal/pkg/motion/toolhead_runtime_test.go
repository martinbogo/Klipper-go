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

func TestNewToolheadCoreStateDefaults(t *testing.T) {
	state := NewToolheadCoreState(false)

	if !reflect.DeepEqual(state.CommandedPos, []float64{0.0, 0.0, 0.0, 0.0}) {
		t.Fatalf("unexpected commanded position %#v", state.CommandedPos)
	}
	if state.SpecialQueuingState != "NeedPrime" {
		t.Fatalf("unexpected special queuing state %q", state.SpecialQueuingState)
	}
	if !almostEqualFloat64(state.NeedCheckPause, -1.0) {
		t.Fatalf("unexpected need check pause %v", state.NeedCheckPause)
	}
	if !almostEqualFloat64(state.KinFlushDelay, DefaultSdsCheckTime) {
		t.Fatalf("unexpected kin flush delay %v", state.KinFlushDelay)
	}
	if !state.DoKickFlushTimer {
		t.Fatal("expected new core state to kick flush timer")
	}
	if state.CanPause {
		t.Fatal("expected fileoutput-style core state to disable pausing")
	}
}

func TestToolheadCoreStateVelocityAndCommandedPosition(t *testing.T) {
	state := NewToolheadCoreState(true)
	result := ApplyToolheadVelocityLimitUpdate(ToolheadVelocitySettings{}, ToolheadVelocityLimitUpdate{
		MaxVelocity:           ptrFloat64(300.0),
		MaxAccel:              ptrFloat64(5000.0),
		RequestedAccelToDecel: ptrFloat64(2400.0),
		SquareCornerVelocity:  ptrFloat64(8.0),
	})
	state.ApplyVelocityLimitResult(result)
	state.SetCommandedPosition([]float64{1.0, 2.0, 3.0, 4.0})

	if !reflect.DeepEqual(state.VelocitySettings(), result.Settings) {
		t.Fatalf("unexpected velocity settings %#v", state.VelocitySettings())
	}
	if !reflect.DeepEqual(state.MoveConfig(), MoveConfig{
		Max_accel:          5000.0,
		Junction_deviation: result.JunctionDeviation,
		Max_velocity:       300.0,
		Max_accel_to_decel: result.MaxAccelToDecel,
	}) {
		t.Fatalf("unexpected move config %#v", state.MoveConfig())
	}
	commanded := state.CommandedPosition()
	commanded[0] = 99.0
	if !reflect.DeepEqual(state.CommandedPos, []float64{1.0, 2.0, 3.0, 4.0}) {
		t.Fatalf("expected commanded position copy, got %#v", state.CommandedPos)
	}
}

func TestToolheadCoreStatePauseTimingAndFlushReset(t *testing.T) {
	state := ToolheadCoreState{
		PrintTime:           12.0,
		CheckStallTime:      1.5,
		PrintStall:          2.0,
		SpecialQueuingState: "Drip",
		NeedCheckPause:      4.5,
		CanPause:            true,
		LastStepGenTime:     7.8,
		LastFlushTime:       8.0,
		MinRestartTime:      7.5,
		NeedFlushTime:       8.5,
		StepGenTime:         9.0,
		ClearHistoryTime:    3.0,
		KinFlushDelay:       0.25,
		KinFlushTimes:       []float64{0.1, 0.2},
		DoKickFlushTimer:    false,
	}

	if !reflect.DeepEqual(state.PauseState(), ToolheadPauseState{
		PrintTime:           12.0,
		CheckStallTime:      1.5,
		PrintStall:          2.0,
		SpecialQueuingState: "Drip",
		NeedCheckPause:      4.5,
		CanPause:            true,
	}) {
		t.Fatalf("unexpected pause state %#v", state.PauseState())
	}

	timing := state.TimingState()
	timing.KinFlushTimes[0] = 9.9
	if !reflect.DeepEqual(state.KinFlushTimes, []float64{0.1, 0.2}) {
		t.Fatalf("expected timing state copy, got %#v", state.KinFlushTimes)
	}

	state.ApplyPauseState(ToolheadPauseState{
		PrintTime:           20.0,
		CheckStallTime:      6.0,
		PrintStall:          3.0,
		SpecialQueuingState: "Priming",
		NeedCheckPause:      10.0,
		CanPause:            false,
	})
	state.ApplyTimingState(ToolheadTimingState{
		PrintTime:        21.0,
		LastFlushTime:    11.0,
		LastStepGenTime:  11.2,
		MinRestartTime:   12.0,
		NeedFlushTime:    13.0,
		StepGenTime:      14.0,
		ClearHistoryTime: 15.0,
		KinFlushDelay:    0.4,
		KinFlushTimes:    []float64{0.3, 0.6},
		DoKickFlushTimer: true,
		CanPause:         true,
	})
	reset := state.ResetAfterLookaheadFlush(2.0)

	if !reflect.DeepEqual(reset, ToolheadFlushReset{
		SpecialQueuingState: "NeedPrime",
		NeedCheckPause:      -1.0,
		LookaheadFlushTime:  2.0,
		CheckStallTime:      0.0,
	}) {
		t.Fatalf("unexpected flush reset %#v", reset)
	}
	if state.SpecialQueuingState != "NeedPrime" || !almostEqualFloat64(state.NeedCheckPause, -1.0) || !almostEqualFloat64(state.CheckStallTime, 0.0) {
		t.Fatalf("unexpected core state after reset %#v", state)
	}
	if !reflect.DeepEqual(state.KinFlushTimes, []float64{0.3, 0.6}) {
		t.Fatalf("unexpected timing state application %#v", state.KinFlushTimes)
	}
}

func TestToolheadCoreStateAdvanceMoveTimeMutatesState(t *testing.T) {
	state := ToolheadCoreState{
		PrintTime:     10.0,
		LastFlushTime: 0.0,
		KinFlushDelay: 0.1,
		CanPause:      true,
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

func TestToolheadCoreStateTimingHelpersMutateCore(t *testing.T) {
	state := ToolheadCoreState{
		PrintTime:        10.0,
		MinRestartTime:   10.5,
		KinFlushDelay:    0.301,
		KinFlushTimes:    []float64{0.1, 0.3},
		NeedFlushTime:    2.0,
		StepGenTime:      3.0,
		DoKickFlushTimer: true,
	}
	notifier := &fakeSyncPrintTimeNotifier{}

	state.CalcPrintTime(testToolheadTimingConfig(), fakePrintTimeSource{monotonic: 12.0, estimated: 10.0}, notifier)
	if !almostEqualFloat64(state.PrintTime, 10.801) {
		t.Fatalf("unexpected print time %v", state.PrintTime)
	}
	assertFloat64Triples(t, notifier.calls, [][3]float64{{12.0, 10.0, 10.801}})

	state.UpdateStepGenerationScanDelay(0.2, 0.1, testToolheadTimingConfig())
	if !reflect.DeepEqual(state.KinFlushTimes, []float64{0.3, 0.2}) {
		t.Fatalf("unexpected scan times %#v", state.KinFlushTimes)
	}
	if !almostEqualFloat64(state.KinFlushDelay, 0.301) {
		t.Fatalf("unexpected kin flush delay %v", state.KinFlushDelay)
	}

	if !state.NoteMovequeueActivity(4.0, true) {
		t.Fatal("expected first movequeue activity to kick flush timer")
	}
	if !almostEqualFloat64(state.NeedFlushTime, 4.0) || !almostEqualFloat64(state.StepGenTime, 4.0) {
		t.Fatalf("unexpected movequeue state need=%v step=%v", state.NeedFlushTime, state.StepGenTime)
	}
	if state.DoKickFlushTimer {
		t.Fatal("expected kick flag to be cleared")
	}
}

func ptrFloat64(value float64) *float64 {
	return &value
}

func TestToolheadTimingStateAdvanceFlushTime(t *testing.T) {
	state := ToolheadTimingState{
		PrintTime:        10.0,
		LastFlushTime:    7.0,
		LastStepGenTime:  7.2,
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
	if !almostEqualFloat64(state.LastStepGenTime, 8.05) {
		t.Fatalf("unexpected last step generation time %v", state.LastStepGenTime)
	}
	if !almostEqualFloat64(state.MinRestartTime, 8.05) {
		t.Fatalf("unexpected min restart time %v", state.MinRestartTime)
	}
}

func TestToolheadTimingStateAdvanceFlushTimeExpiresHistoryWithoutPause(t *testing.T) {
	state := ToolheadTimingState{
		PrintTime:        50.0,
		LastFlushTime:    0.0,
		LastStepGenTime:  0.0,
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

	assertFloat64Pairs(t, finalizeCalls, [][2]float64{{39.95, 9.95}})
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
