package mcu

import "testing"

func TestBuildEndstopConfigPlan(t *testing.T) {
	plan := BuildEndstopConfigPlan(9, "PA1", 1)
	if len(plan.ConfigCmds) != 1 || plan.ConfigCmds[0] != "config_endstop oid=9 pin=PA1 pull_up=1" {
		t.Fatalf("unexpected config commands %#v", plan.ConfigCmds)
	}
	if len(plan.RestartCmds) != 1 || plan.RestartCmds[0] != "endstop_home oid=9 clock=0 sample_ticks=0 sample_count=0 rest_ticks=0 pin_value=0 trsync_oid=0 trigger_reason=0" {
		t.Fatalf("unexpected restart commands %#v", plan.RestartCmds)
	}
	if plan.HomeLookupFormat == "" || plan.QueryRequestFormat == "" || plan.QueryResponseFormat == "" {
		t.Fatalf("expected lookup formats to be populated: %#v", plan)
	}
}
