package kinematics

import "testing"

func TestBuildRailConfigPlanDefaultsRangeWithoutMinMax(t *testing.T) {
	plan := BuildRailConfigPlan(5, false, -1, 10, false)
	if plan.PositionMin != 0 || plan.PositionMax != 5 {
		t.Fatalf("unexpected range %#v", plan)
	}
	if !plan.HomingPositiveDir {
		t.Fatalf("expected homing direction to infer positive at the implicit maximum")
	}
}

func TestBuildRailConfigPlanInfersPositiveDirectionNearMaximum(t *testing.T) {
	plan := BuildRailConfigPlan(9, true, 0, 10, false)
	if !plan.HomingPositiveDir {
		t.Fatalf("expected homing direction to infer positive near the maximum")
	}
}

func TestBuildRailConfigPlanRejectsPositionOutsideRange(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected out-of-range position_endstop to panic")
		}
	}()
	BuildRailConfigPlan(11, true, 0, 10, false)
}

func TestBuildRailConfigPlanRejectsAmbiguousDirection(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected ambiguous homing direction to panic")
		}
	}()
	BuildRailConfigPlan(5, true, 0, 10, false)
}

func TestBuildRailConfigPlanRejectsPositiveDirectionAtMinimum(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected invalid positive homing direction to panic")
		}
	}()
	BuildRailConfigPlan(0, true, 0, 10, true)
}
