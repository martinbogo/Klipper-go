package mcu

import "fmt"

type EndstopConfigPlan struct {
	ConfigCmds          []string
	RestartCmds         []string
	HomeLookupFormat    string
	QueryRequestFormat  string
	QueryResponseFormat string
}

func BuildEndstopConfigPlan(oid int, pin interface{}, pullup interface{}) EndstopConfigPlan {
	return EndstopConfigPlan{
		ConfigCmds: []string{
			fmt.Sprintf("config_endstop oid=%d pin=%s pull_up=%d", oid, pin, pullup),
		},
		RestartCmds: []string{
			fmt.Sprintf("endstop_home oid=%d clock=0 sample_ticks=0 sample_count=0 rest_ticks=0 pin_value=0 trsync_oid=0 trigger_reason=0", oid),
		},
		HomeLookupFormat:    "endstop_home oid=%c clock=%u sample_ticks=%u sample_count=%c rest_ticks=%u pin_value=%c trsync_oid=%c trigger_reason=%c",
		QueryRequestFormat:  "endstop_query_state oid=%c",
		QueryResponseFormat: "endstop_state oid=%c homing=%c next_clock=%u pin_value=%c",
	}
}
