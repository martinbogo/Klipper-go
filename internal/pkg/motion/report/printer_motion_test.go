package report

import "testing"

func TestUpdateTrackedObjectStatusSortsNames(t *testing.T) {
	status := UpdateTrackedObjectStatus(NewPrinterMotionStatus(), map[string]interface{}{"stepper_z": nil, "stepper_x": nil}, map[string]interface{}{"extruder": nil, "toolhead": nil})
	steppers := status["steppers"].([]string)
	trapqs := status["trapq"].([]string)
	if len(steppers) != 2 || steppers[0] != "stepper_x" || steppers[1] != "stepper_z" {
		t.Fatalf("unexpected sorted steppers %#v", steppers)
	}
	if len(trapqs) != 2 || trapqs[0] != "extruder" || trapqs[1] != "toolhead" {
		t.Fatalf("unexpected sorted trapqs %#v", trapqs)
	}
}

func TestBuildStatusRefreshDecision(t *testing.T) {
	skipped := BuildStatusRefreshDecision(1.0, 2.0, 1, StatusRefreshTime)
	if skipped.ShouldUpdate || skipped.NextStatusTime != 2.0 {
		t.Fatalf("expected cached status decision, got %#v", skipped)
	}
	updated := BuildStatusRefreshDecision(2.0, 1.0, 2, StatusRefreshTime)
	if !updated.ShouldUpdate || updated.NextStatusTime != 2.25 {
		t.Fatalf("expected refresh decision, got %#v", updated)
	}
	noTrapq := BuildStatusRefreshDecision(2.0, 1.0, 0, StatusRefreshTime)
	if noTrapq.ShouldUpdate {
		t.Fatalf("expected missing trapq to skip refresh, got %#v", noTrapq)
	}
}

func TestBuildPrinterMotionStatus(t *testing.T) {
	status := BuildPrinterMotionStatus(NewPrinterMotionStatus(), []float64{1, 2, 3}, 4.5, []float64{6}, 7.5)
	position := status["live_position"].([]float64)
	if len(position) != 4 || position[0] != 1 || position[3] != 6 {
		t.Fatalf("unexpected live position %#v", position)
	}
	if status["live_velocity"].(float64) != 4.5 || status["live_extruder_velocity"].(float64) != 7.5 {
		t.Fatalf("unexpected velocities %#v", status)
	}
}

func TestBuildShutdownDumpPlan(t *testing.T) {
	plan := BuildShutdownDumpPlan([]ShutdownStepperSnapshot{
		{Name: "x", ShutdownClock: 150, ShutdownTime: 1.5, ClockWindow: 20},
		{Name: "y", ShutdownClock: 10, ShutdownTime: 1.0, ClockWindow: 25},
	}, NeverTime)
	if plan.ShutdownTime != 1.0 {
		t.Fatalf("unexpected shutdown time %v", plan.ShutdownTime)
	}
	x := plan.StepperWindows["x"]
	if x.StartClock != 130 || x.EndClock != 170 {
		t.Fatalf("unexpected x shutdown window %#v", x)
	}
	y := plan.StepperWindows["y"]
	if y.StartClock != 0 || y.EndClock != 35 {
		t.Fatalf("unexpected y shutdown window %#v", y)
	}
}
