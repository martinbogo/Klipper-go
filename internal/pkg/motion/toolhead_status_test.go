package motion

import "testing"

func TestBuildToolheadStatsNormalState(t *testing.T) {
	result := BuildToolheadStats(ToolheadStatsSnapshot{
		PrintTime:           25.0,
		LastFlushTime:       24.0,
		EstimatedPrintTime:  23.5,
		PrintStall:          2,
		MoveHistoryExpire:   30.0,
		SpecialQueuingState: "",
	})

	if !almostEqualFloat64(result.MaxQueueTime, 25.0) {
		t.Fatalf("unexpected max queue time %v", result.MaxQueueTime)
	}
	if !almostEqualFloat64(result.ClearHistoryTime, -6.5) {
		t.Fatalf("unexpected clear history time %v", result.ClearHistoryTime)
	}
	if !almostEqualFloat64(result.BufferTime, 1.5) {
		t.Fatalf("unexpected buffer time %v", result.BufferTime)
	}
	if !result.IsActive {
		t.Fatal("expected active toolhead stats")
	}
	if result.Summary != "print_time=25.000 buffer_time=1.500 print_stall=2" {
		t.Fatalf("unexpected summary %q", result.Summary)
	}
}

func TestBuildToolheadStatsDripStateClampsDisplayBuffer(t *testing.T) {
	result := BuildToolheadStats(ToolheadStatsSnapshot{
		PrintTime:           10.0,
		LastFlushTime:       9.0,
		EstimatedPrintTime:  8.0,
		PrintStall:          0,
		MoveHistoryExpire:   30.0,
		SpecialQueuingState: "Drip",
	})

	if !almostEqualFloat64(result.BufferTime, 0.0) {
		t.Fatalf("expected drip buffer time to clamp to zero, got %v", result.BufferTime)
	}
	if result.Summary != "print_time=10.000 buffer_time=0.000 print_stall=0" {
		t.Fatalf("unexpected drip summary %q", result.Summary)
	}
	if !result.IsActive {
		t.Fatal("expected drip toolhead stats to remain active")
	}
}

func TestBuildToolheadBusyState(t *testing.T) {
	result := BuildToolheadBusyState(12.0, 11.5, true)
	if !almostEqualFloat64(result.PrintTime, 12.0) || !almostEqualFloat64(result.EstimatedPrintTime, 11.5) || !result.LookaheadEmpty {
		t.Fatalf("unexpected busy state %#v", result)
	}
}

func TestBuildToolheadStatusClonesInputs(t *testing.T) {
	kinematicsStatus := map[string]interface{}{"homed_axes": "xyz"}
	position := []float64{1, 2, 3, 4}

	status := BuildToolheadStatus(ToolheadStatusSnapshot{
		KinematicsStatus:      kinematicsStatus,
		PrintTime:             7.5,
		EstimatedPrintTime:    7.0,
		PrintStall:            3,
		ExtruderName:          "extruder",
		CommandedPosition:     position,
		MaxVelocity:           300,
		MaxAccel:              5000,
		RequestedAccelToDecel: 2500,
		SquareCornerVelocity:  5,
	})

	position[0] = 99
	kinematicsStatus["homed_axes"] = "xy"

	if status["homed_axes"] != "xyz" {
		t.Fatalf("expected cloned kinematics status, got %#v", status)
	}
	statusPosition, ok := status["position"].([]float64)
	if !ok {
		t.Fatalf("unexpected position type %T", status["position"])
	}
	if statusPosition[0] != 1 {
		t.Fatalf("expected cloned position, got %#v", statusPosition)
	}
	if status["extruder"] != "extruder" || status["max_accel_to_decel"] != 2500.0 {
		t.Fatalf("unexpected status payload %#v", status)
	}
}
