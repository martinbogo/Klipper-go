package mcu

import "testing"

func TestLegacyStepperRuntimeStateShortNameAndUnits(t *testing.T) {
	runtime := NewLegacyStepperRuntimeState("stepper_x", nil, true)

	if got := runtime.Name(true); got != "x" {
		t.Fatalf("expected short name x, got %q", got)
	}
	if got := runtime.Name(false); got != "stepper_x" {
		t.Fatalf("expected full name stepper_x, got %q", got)
	}
	if !runtime.UnitsInRadians() {
		t.Fatalf("expected units in radians to be true")
	}
}

func TestLegacyStepperRuntimeStateTracksPulseDefaultsAndPlan(t *testing.T) {
	runtime := NewLegacyStepperRuntimeState("stepper_x", nil, false)
	runtime.SetupDefaultPulseDuration(0.000000100, true)

	pulseDuration, requestedBothEdge := runtime.PulseDuration()
	if pulseDuration != 0.000000100 || !requestedBothEdge {
		t.Fatalf("unexpected pulse defaults: %v / %v", pulseDuration, requestedBothEdge)
	}

	plan := runtime.BuildPulseConfigPlan(0, float64(1), func(seconds float64) int64 {
		return int64(seconds * 1000000)
	})
	if !plan.StepBothEdge {
		t.Fatalf("expected both-edge plan")
	}
	if pulseDuration, _ := runtime.PulseDuration(); pulseDuration != float64(0) {
		t.Fatalf("expected runtime pulse duration to be updated to zero, got %v", pulseDuration)
	}
}
