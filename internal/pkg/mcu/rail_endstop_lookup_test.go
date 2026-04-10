package mcu

import "testing"

func TestBuildRailEndstopLookupPlanCreatesNewEntryWhenMissing(t *testing.T) {
	plan := BuildRailEndstopLookupPlan("mcu", "PA1", 0, 1, map[string]RailEndstopEntry{})
	if !plan.NeedsNewEndstop {
		t.Fatalf("expected a new endstop entry, got %#v", plan)
	}
	if plan.PinName != "mcu:PA1" {
		t.Fatalf("expected normalized pin name mcu:PA1, got %q", plan.PinName)
	}
	if plan.SharedSettingsConflict {
		t.Fatalf("did not expect a conflict for a new entry")
	}
}

func TestBuildRailEndstopLookupPlanReusesMatchingEntry(t *testing.T) {
	endstop := "endstop"
	plan := BuildRailEndstopLookupPlan("mcu", "PA1", 0, 1, map[string]RailEndstopEntry{
		"mcu:PA1": {Endstop: endstop, Invert: 0, Pullup: 1},
	})
	if plan.NeedsNewEndstop || plan.SharedSettingsConflict {
		t.Fatalf("expected reusable entry, got %#v", plan)
	}
	if plan.ExistingEndstop != endstop {
		t.Fatalf("expected existing endstop to be reused, got %#v", plan.ExistingEndstop)
	}
}

func TestBuildRailEndstopLookupPlanDetectsInvertConflict(t *testing.T) {
	plan := BuildRailEndstopLookupPlan("mcu", "PA1", 1, 1, map[string]RailEndstopEntry{
		"mcu:PA1": {Endstop: "endstop", Invert: 0, Pullup: 1},
	})
	if !plan.SharedSettingsConflict {
		t.Fatalf("expected invert mismatch to trigger a conflict, got %#v", plan)
	}
}

func TestBuildRailEndstopLookupPlanDetectsPullupConflict(t *testing.T) {
	plan := BuildRailEndstopLookupPlan("mcu", "PA1", 0, 0, map[string]RailEndstopEntry{
		"mcu:PA1": {Endstop: "endstop", Invert: 0, Pullup: 1},
	})
	if !plan.SharedSettingsConflict {
		t.Fatalf("expected pullup mismatch to trigger a conflict, got %#v", plan)
	}
}
