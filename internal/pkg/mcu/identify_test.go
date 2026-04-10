package mcu

import "testing"

func TestCollectReservedPins(t *testing.T) {
	reserved := CollectReservedPins(map[string]interface{}{
		"RESERVE_PINS_TEST": "PA1,PA2",
		"OTHER":             "ignore",
	})
	if len(reserved) != 2 {
		t.Fatalf("expected 2 reserved pins, got %#v", reserved)
	}
	if reserved[0].Pin != "PA1" || reserved[0].Owner != "TEST" || reserved[1].Pin != "PA2" || reserved[1].Owner != "TEST" {
		t.Fatalf("unexpected reserved pins %#v", reserved)
	}
}

func TestBuildIdentifyFinalizePlan(t *testing.T) {
	constants := map[string]interface{}{"RESERVE_PINS_TEST": "PA1", "CLOCK_FREQ": 16000000.0}
	plan := BuildIdentifyFinalizePlan("", false, true, nil, 1, "v1", "build1", constants)
	if plan.RestartMethod != "command" {
		t.Fatalf("expected command restart fallback, got %q", plan.RestartMethod)
	}
	if !plan.IsMCUBridge {
		t.Fatalf("expected MCU bridge flag to be set")
	}
	if plan.StatusInfo["mcu_version"] != "v1" || plan.StatusInfo["mcu_build_versions"] != "build1" {
		t.Fatalf("unexpected status info %#v", plan.StatusInfo)
	}
	if len(plan.ReservedPins) != 1 || plan.ReservedPins[0].Pin != "PA1" {
		t.Fatalf("unexpected reserved pins %#v", plan.ReservedPins)
	}
	noFallback := BuildIdentifyFinalizePlan("arduino", false, false, nil, 0, "v1", "build1", constants)
	if noFallback.RestartMethod != "arduino" {
		t.Fatalf("expected explicit restart method to remain unchanged, got %q", noFallback.RestartMethod)
	}
	if noFallback.IsMCUBridge {
		t.Fatalf("expected MCU bridge flag to stay false")
	}
}
