package motion

import "testing"

func testToolheadFlushConfig() ToolheadFlushConfig {
	return ToolheadFlushConfig{
		BufferTimeLow:         1.0,
		BgFlushLowTime:        0.2,
		BgFlushHighTime:       0.4,
		BgFlushSgLowTime:      0.45,
		BgFlushSgHighTime:     0.7,
		BgFlushBatchTime:      0.2,
		BgFlushExtraTime:      0.25,
		StepcompressFlushTime: 0.05,
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
		LastStepGenTime:     9.0,
		NeedFlushTime:       9.0,
		NeedStepGenTime:     9.0,
		SpecialQueuingState: "",
		KinFlushDelay:       0.001,
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
		LastStepGenTime:     9.4,
		NeedFlushTime:       9.0,
		NeedStepGenTime:     9.0,
		SpecialQueuingState: "",
		KinFlushDelay:       0.001,
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
		LastStepGenTime:     9.8,
		NeedFlushTime:       9.6,
		NeedStepGenTime:     10.0,
		SpecialQueuingState: "NeedPrime",
		KinFlushDelay:       0.2,
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

func TestBuildToolheadFlushHandlerPlanUsesAggressiveStepGenerationPath(t *testing.T) {
	plan := BuildToolheadFlushHandlerPlan(100.0, 9.0, ToolheadFlushHandlerState{
		PrintTime:           9.0,
		LastFlushTime:       8.9,
		LastStepGenTime:     9.1,
		NeedFlushTime:       9.6,
		NeedStepGenTime:     10.0,
		SpecialQueuingState: "NeedPrime",
		KinFlushDelay:       0.2,
	}, testToolheadFlushConfig())

	if plan.ShouldFlushLookahead {
		t.Fatalf("did not expect lookahead flush in aggressive path %#v", plan)
	}
	if len(plan.AdvanceFlushTimes) != 1 || !almostEqualFloat64(plan.AdvanceFlushTimes[0], 9.3) {
		t.Fatalf("unexpected aggressive flush advance %#v", plan.AdvanceFlushTimes)
	}
	if !almostEqualFloat64(plan.NextWakeTime, 99.9) {
		t.Fatalf("unexpected aggressive wake time %v", plan.NextWakeTime)
	}
	if plan.ReturnNever || plan.KickFlushTimer {
		t.Fatalf("did not expect aggressive path completion %#v", plan)
	}
}
