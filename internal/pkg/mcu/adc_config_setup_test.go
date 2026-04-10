package mcu

import "testing"

func TestBuildADCConfigSetupPlan(t *testing.T) {
	plan := BuildADCConfigSetupPlan(7, "PA1", ADCConfigPlan{
		QueryClock:      100,
		SampleTicks:     10,
		SampleCount:     4,
		ReportClock:     200,
		MinSample:       111,
		MaxSample:       222,
		RangeCheckCount: 3,
	})
	if plan.ConfigCmd != "config_analog_in oid=7 pin=PA1" {
		t.Fatalf("unexpected config command %q", plan.ConfigCmd)
	}
	expectedQuery := "query_analog_in oid=7 clock=100 sample_ticks=10 sample_count=4 rest_ticks=200 min_value=111 max_value=222 range_check_count=3"
	if plan.QueryCmd != expectedQuery {
		t.Fatalf("unexpected query command %q", plan.QueryCmd)
	}
	if plan.ResponseName != "analog_in_state" || plan.ResponseLogLabel != "analog_in_state7" {
		t.Fatalf("unexpected response metadata %#v", plan)
	}
}
