package mcu

import "testing"

func TestBuildStepperPulseConfigPlanDefaultsPulseDuration(t *testing.T) {
	plan := BuildStepperPulseConfigPlan(nil, false, 1, nil, func(seconds float64) int64 {
		return int64(seconds * 1000000)
	})
	if plan.StepPulseDuration != defaultStepPulseDuration {
		t.Fatalf("expected default pulse duration %v, got %v", defaultStepPulseDuration, plan.StepPulseDuration)
	}
	if plan.StepBothEdge {
		t.Fatalf("expected both-edge mode to stay disabled")
	}
	if plan.InvertStep != 1 {
		t.Fatalf("expected invert step to stay unchanged, got %d", plan.InvertStep)
	}
	if plan.StepPulseTicks != 2 {
		t.Fatalf("expected 2 step pulse ticks, got %d", plan.StepPulseTicks)
	}
}

func TestBuildStepperPulseConfigPlanEnablesBothEdgeMode(t *testing.T) {
	plan := BuildStepperPulseConfigPlan(0.000000100, true, 0, float64(1), func(seconds float64) int64 {
		return int64(seconds * 1000000)
	})
	if !plan.StepBothEdge {
		t.Fatalf("expected both-edge mode to be enabled")
	}
	if plan.StepPulseDuration != 0 {
		t.Fatalf("expected pulse duration to be zeroed in both-edge mode, got %v", plan.StepPulseDuration)
	}
	if plan.InvertStep != -1 {
		t.Fatalf("expected invert step -1 in both-edge mode, got %d", plan.InvertStep)
	}
	if plan.StepPulseTicks != 0 {
		t.Fatalf("expected zero step pulse ticks in both-edge mode, got %d", plan.StepPulseTicks)
	}
}

func TestBuildStepperPulseConfigPlanRequiresSupportForBothEdge(t *testing.T) {
	plan := BuildStepperPulseConfigPlan(0.000000100, true, 0, float64(0), func(seconds float64) int64 {
		return int64(seconds * 1000000)
	})
	if plan.StepBothEdge {
		t.Fatalf("expected both-edge mode to remain disabled without MCU support")
	}
	if plan.StepPulseDuration != 0.000000100 {
		t.Fatalf("expected pulse duration to stay unchanged, got %v", plan.StepPulseDuration)
	}
	if plan.InvertStep != 0 {
		t.Fatalf("expected invert step to stay unchanged, got %d", plan.InvertStep)
	}
}

func TestBuildStepperPulseConfigPlanRequiresShortPulseForBothEdge(t *testing.T) {
	plan := BuildStepperPulseConfigPlan(0.000003, true, 0, float64(1), func(seconds float64) int64 {
		return int64(seconds * 1000000)
	})
	if plan.StepBothEdge {
		t.Fatalf("expected both-edge mode to remain disabled for long pulses")
	}
	if plan.StepPulseDuration != 0.000003 {
		t.Fatalf("expected pulse duration to stay unchanged, got %v", plan.StepPulseDuration)
	}
	if plan.StepPulseTicks != 3 {
		t.Fatalf("expected 3 step pulse ticks, got %d", plan.StepPulseTicks)
	}
}
