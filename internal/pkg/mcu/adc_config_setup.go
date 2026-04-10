package mcu

import "fmt"

type ADCConfigSetupPlan struct {
	ConfigCmd        string
	QueryCmd         string
	ResponseName     string
	ResponseLogLabel string
}

func BuildADCConfigSetupPlan(oid int, pin string, plan ADCConfigPlan) ADCConfigSetupPlan {
	return ADCConfigSetupPlan{
		ConfigCmd: fmt.Sprintf("config_analog_in oid=%d pin=%s", oid, pin),
		QueryCmd: fmt.Sprintf("query_analog_in oid=%d clock=%d sample_ticks=%d sample_count=%d rest_ticks=%d min_value=%d max_value=%d range_check_count=%d",
			oid, plan.QueryClock, plan.SampleTicks, plan.SampleCount, plan.ReportClock, plan.MinSample, plan.MaxSample, plan.RangeCheckCount),
		ResponseName:     "analog_in_state",
		ResponseLogLabel: fmt.Sprintf("analog_in_state%d", oid),
	}
}
