package mcu

import "testing"

func TestBuildTrsyncConfigPlan(t *testing.T) {
	plan := BuildTrsyncConfigPlan(7)
	if len(plan.ConfigCmds) != 1 || plan.ConfigCmds[0] != "config_trsync oid=7" {
		t.Fatalf("unexpected config commands %#v", plan.ConfigCmds)
	}
	if len(plan.RestartCmds) != 1 || plan.RestartCmds[0] != "trsync_start oid=7 report_clock=0 report_ticks=0 expire_reason=0" {
		t.Fatalf("unexpected restart commands %#v", plan.RestartCmds)
	}
	if plan.StartLookupFormat == "" || plan.QueryResponseFormat == "" || plan.StateTagFormat == "" {
		t.Fatalf("expected lookup/tag formats to be populated: %#v", plan)
	}
}
