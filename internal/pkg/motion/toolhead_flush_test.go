package motion

import "testing"

func testToolheadFlushConfig() ToolheadFlushConfig {
	return ToolheadFlushConfig{
		BufferTimeLow:    1.0,
		BgFlushLowTime:   0.2,
		BgFlushBatchTime: 0.2,
		BgFlushExtraTime: 0.25,
	}
}

func TestBuildToolheadFlushReset(t *testing.T) {
	reset := BuildToolheadFlushReset(2.0)
	if reset.SpecialQueuingState != "NeedPrime" || reset.NeedCheckPause != -1.0 || reset.CheckStallTime != 0.0 || reset.LookaheadFlushTime != 2.0 {
		t.Fatalf("unexpected flush reset %#v", reset)
	}
}

func TestBuildToolheadFlushHandlerPlanDefersMainStateCheck(t *testing.T) {
	plan := BuildToolheadFlushHandlerPlan(10.0, 7.5, ToolheadFlushHandlerState{
		PrintTime:           9.0,
		LastFlushTime:       9.0,
		NeedFlushTime:       9.0,
		SpecialQueuingState: "",
	}, testToolheadFlushConfig())

	if plan.ShouldFlushLookahead || plan.ReturnNever || len(plan.AdvanceFlushTimes) != 0 {
		t.Fatalf("unexpected main-state defer plan %#v", plan)
	}
	if !almostEqualFloat64(plan.NextWakeTime, 10.5) {
		t.Fatalf("unexpected next wake time %v", plan.NextWakeTime)
	}
}

func TestBuildToolheadFlushHandlerPlanFlushesLookaheadAndCompletes(t *testing.T) {
	plan := BuildToolheadFlushHandlerPlan(10.0, 9.0, ToolheadFlushHandlerState{
		PrintTime:           9.0,
		LastFlushTime:       9.4,
		NeedFlushTime:       9.0,
		SpecialQueuingState: "",
	}, testToolheadFlushConfig())

	if !plan.ShouldFlushLookahead || !plan.ReturnNever || !plan.KickFlushTimer {
		t.Fatalf("expected completed flush plan, got %#v", plan)
	}
	if len(plan.AdvanceFlushTimes) != 0 {
		t.Fatalf("did not expect extra flush steps, got %#v", plan.AdvanceFlushTimes)
	}
}

func TestBuildToolheadFlushHandlerPlanSchedulesBackgroundFlushAdvance(t *testing.T) {
	plan := BuildToolheadFlushHandlerPlan(10.0, 9.0, ToolheadFlushHandlerState{
		PrintTime:           9.0,
		LastFlushTime:       8.9,
		NeedFlushTime:       9.6,
		SpecialQueuingState: "NeedPrime",
	}, testToolheadFlushConfig())

	if plan.ShouldFlushLookahead {
		t.Fatalf("did not expect additional lookahead flush, got %#v", plan)
	}
	if len(plan.AdvanceFlushTimes) != 1 {
		t.Fatalf("expected one background flush step, got %#v", plan.AdvanceFlushTimes)
	}
	if !almostEqualFloat64(plan.AdvanceFlushTimes[0], 9.4) {
		t.Fatalf("unexpected background flush sequence %#v", plan.AdvanceFlushTimes)
	}
	if !almostEqualFloat64(plan.NextWakeTime, 10.2) {
		t.Fatalf("unexpected post-flush wake time %v", plan.NextWakeTime)
	}
	if plan.ReturnNever || plan.KickFlushTimer {
		t.Fatalf("did not expect completed timer plan %#v", plan)
	}
}
