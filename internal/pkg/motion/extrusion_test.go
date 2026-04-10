package motion

import (
	"strings"
	"testing"
)

func testExtrusionLimits() ExtrusionLimits {
	return ExtrusionLimits{
		CanExtrude:      true,
		NozzleDiameter:  0.4,
		FilamentArea:    2.4,
		MaxExtrudeRatio: 0.5,
		MaxEVelocity:    5,
		MaxEAccel:       7,
		MaxEDistance:    6,
		InstantCornerV:  1.5,
	}
}

func TestCheckExtrusionMoveRejectsColdExtrusion(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{0, 0, 0, 1}, 20)
	err := CheckExtrusionMove(move, ExtrusionLimits{})
	if err == nil || err.Error() != "Extrude below minimum temp\nSee the 'min_extrude_temp' config option for details" {
		t.Fatalf("expected cold extrusion error, got %v", err)
	}
}

func TestCheckExtrusionMoveLimitsExtrudeOnlySpeedAndAccel(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{0, 0, 0, 4}, 20)
	err := CheckExtrusionMove(move, testExtrusionLimits())
	if err != nil {
		t.Fatalf("unexpected extrusion check error: %v", err)
	}
	if move.Max_cruise_v2 != 25 {
		t.Fatalf("expected extrude-only velocity limit 25, got %v", move.Max_cruise_v2)
	}
	if move.Accel != 7 {
		t.Fatalf("expected extrude-only accel 7, got %v", move.Accel)
	}
}

func TestCheckExtrusionMoveRejectsOverlongExtrudeOnlyMove(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{0, 0, 0, 8}, 20)
	err := CheckExtrusionMove(move, testExtrusionLimits())
	if err == nil || !strings.Contains(err.Error(), "Extrude only move too long") {
		t.Fatalf("expected overlong extrude-only error, got %v", err)
	}
}

func TestCheckExtrusionMoveRejectsOverextrusion(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{1, 0, 0, 1}, 20)
	err := CheckExtrusionMove(move, testExtrusionLimits())
	if err == nil || !strings.Contains(err.Error(), "Move exceeds maximum extrusion") {
		t.Fatalf("expected overextrusion error, got %v", err)
	}
}

func TestCheckExtrusionMoveAllowsTinyOverextrusion(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{1, 0, 0, 0.1}, 20)
	err := CheckExtrusionMove(move, testExtrusionLimits())
	if err != nil {
		t.Fatalf("expected tiny overextrusion to pass, got %v", err)
	}
}

func TestCalcExtrusionJunctionUsesInstantCornerVelocity(t *testing.T) {
	prevMove := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{1, 0, 0, 1}, 20)
	move := NewMove(testMoveConfig(), []float64{1, 0, 0, 1}, []float64{2, 0, 0, 3}, 20)
	got := CalcExtrusionJunction(prevMove, move, 2)
	if got != 4 {
		t.Fatalf("expected junction v2 4, got %v", got)
	}
}

func TestBuildExtrusionMove(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{1, 0, 0, 1}, 20)
	move.Set_junction(4, 9, 1)
	result := BuildExtrusionMove(move)
	if result.StartPosition != 0 {
		t.Fatalf("expected start position 0, got %v", result.StartPosition)
	}
	if result.Accel != move.Accel || result.StartV != move.Start_v || result.CruiseV != move.Cruise_v {
		t.Fatalf("unexpected extrusion move result %#v", result)
	}
	if result.CanPressureAdvance != 1.0 {
		t.Fatalf("expected pressure advance flag, got %v", result.CanPressureAdvance)
	}
}

func TestNoExtruderMoveErrorUsesMoveErrorFormat(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{1, 2, 3, 4}, 20)
	err := NoExtruderMoveError(move)
	if err == nil || !strings.Contains(err.Error(), "Extrude when no extruder present") {
		t.Fatalf("expected no-extruder move error, got %v", err)
	}
}
