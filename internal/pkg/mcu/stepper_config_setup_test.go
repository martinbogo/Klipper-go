package mcu

import "testing"

func TestBuildStepperConfigSetupPlan(t *testing.T) {
	plan := BuildStepperConfigSetupPlan(7, "PA1", "PA2", 1, 42, 0.000125, func(seconds float64) int64 {
		return int64(seconds * 1000000)
	})

	if len(plan.ConfigCmds) != 2 {
		t.Fatalf("expected two config commands, got %#v", plan.ConfigCmds)
	}
	if plan.ConfigCmds[0] != "config_stepper oid=7 step_pin=PA1 dir_pin=PA2 invert_step=1 step_pulse_ticks=42" {
		t.Fatalf("unexpected config command %q", plan.ConfigCmds[0])
	}
	if plan.ConfigCmds[1] != "reset_step_clock oid=7 clock=0" {
		t.Fatalf("unexpected reset command %q", plan.ConfigCmds[1])
	}
	if plan.StepQueueLookup != "queue_step oid=%c interval=%u count=%hu add=%hi" ||
		plan.DirLookup != "set_next_step_dir oid=%c dir=%c" ||
		plan.ResetLookup != "reset_step_clock oid=%c clock=%u" {
		t.Fatalf("unexpected lookup formats %#v", plan)
	}
	if plan.PositionQueryFormat != "stepper_get_position oid=%c" || plan.PositionReplyFormat != "stepper_position oid=%c pos=%i" {
		t.Fatalf("unexpected position query formats %#v", plan)
	}
	if plan.MaxErrorTicks != 125 {
		t.Fatalf("unexpected max error ticks %d", plan.MaxErrorTicks)
	}
}
