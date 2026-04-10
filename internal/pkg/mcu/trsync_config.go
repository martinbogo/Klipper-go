package mcu

import "fmt"

type TrsyncConfigPlan struct {
	ConfigCmds          []string
	RestartCmds         []string
	StartLookupFormat   string
	SetTimeoutFormat    string
	TriggerFormat       string
	QueryRequestFormat  string
	QueryResponseFormat string
	StepperStopFormat   string
	SetTimeoutTagFormat string
	TriggerTagFormat    string
	StateTagFormat      string
}

func BuildTrsyncConfigPlan(oid int) TrsyncConfigPlan {
	return TrsyncConfigPlan{
		ConfigCmds: []string{
			fmt.Sprintf("config_trsync oid=%d", oid),
		},
		RestartCmds: []string{
			fmt.Sprintf("trsync_start oid=%d report_clock=0 report_ticks=0 expire_reason=0", oid),
		},
		StartLookupFormat:   "trsync_start oid=%c report_clock=%u report_ticks=%u expire_reason=%c",
		SetTimeoutFormat:    "trsync_set_timeout oid=%c clock=%u",
		TriggerFormat:       "trsync_trigger oid=%c reason=%c",
		QueryRequestFormat:  "trsync_trigger oid=%c reason=%c",
		QueryResponseFormat: "trsync_state oid=%c can_trigger=%c trigger_reason=%c clock=%u",
		StepperStopFormat:   "stepper_stop_on_trigger oid=%c trsync_oid=%c",
		SetTimeoutTagFormat: "trsync_set_timeout oid=%c clock=%u",
		TriggerTagFormat:    "trsync_trigger oid=%c reason=%c",
		StateTagFormat:      "trsync_state oid=%c can_trigger=%c trigger_reason=%c clock=%u",
	}
}
