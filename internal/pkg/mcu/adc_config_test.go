package mcu

import "testing"

func TestADCRuntimeStateBuildConfigPlan(t *testing.T) {
	state := &ADCRuntimeState{}
	plan := state.BuildConfigPlan(7, 0.01, 0.2, 0.1, 0.5, 4, 3, func(oid int) int64 {
		if oid != 7 {
			t.Fatalf("unexpected oid %d", oid)
		}
		return 1234
	}, func(seconds float64) int64 {
		return int64(seconds * 1000)
	}, 1024)
	if state.InvMaxADC != 1.0/4096.0 {
		t.Fatalf("unexpected inv max adc %f", state.InvMaxADC)
	}
	if state.ReportClock != 200 {
		t.Fatalf("unexpected report clock %d", state.ReportClock)
	}
	if plan.QueryClock != 1234 || plan.SampleTicks != 10 || plan.SampleCount != 4 || plan.ReportClock != 200 || plan.MinSample != 409 || plan.MaxSample != 2048 || plan.RangeCheckCount != 3 {
		t.Fatalf("unexpected adc config plan %#v", plan)
	}
}

func TestADCRuntimeStateBuildConfigPlanClampsRange(t *testing.T) {
	state := &ADCRuntimeState{}
	plan := state.BuildConfigPlan(1, 0.01, 0.2, -1.0, 100.0, 1, 0, func(oid int) int64 {
		return int64(oid)
	}, func(seconds float64) int64 {
		return int64(seconds * 1000)
	}, 1024)
	if plan.MinSample != 0 || plan.MaxSample != 65535 {
		t.Fatalf("unexpected clamped range %#v", plan)
	}
}
