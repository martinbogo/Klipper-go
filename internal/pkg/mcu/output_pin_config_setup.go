package mcu

import "fmt"

type ConfigCommand struct {
	Cmd       string
	IsInit    bool
	OnRestart bool
}

type DigitalOutConfigSetupPlan struct {
	Commands     []ConfigCommand
	LookupFormat string
}

func BuildDigitalOutConfigSetupPlan(oid int, pin string, startValue int, shutdownValue int, plan DigitalOutConfigPlan) DigitalOutConfigSetupPlan {
	return DigitalOutConfigSetupPlan{
		Commands: []ConfigCommand{
			{Cmd: fmt.Sprintf("config_digital_out oid=%d pin=%s value=%d default_value=%d max_duration=%d", oid, pin, startValue, shutdownValue, plan.MaxDurationTicks)},
			{Cmd: fmt.Sprintf("update_digital_out oid=%d value=%d", oid, startValue), OnRestart: true},
		},
		LookupFormat: "queue_digital_out oid=%c clock=%u on_ticks=%u",
	}
}

type PWMConfigSetupPlan struct {
	Commands     []ConfigCommand
	LookupFormat string
}

func BuildPWMConfigSetupPlan(oid int, pin string, hardwarePWM bool, plan PWMConfigPlan) PWMConfigSetupPlan {
	if hardwarePWM {
		return PWMConfigSetupPlan{
			Commands: []ConfigCommand{
				{Cmd: fmt.Sprintf("config_pwm_out oid=%d pin=%s cycle_ticks=%d value=%d default_value=%d max_duration=%d", oid, pin, plan.CycleTicks, plan.StartConfigValue, plan.ShutdownConfigValue, plan.MaxDurationTicks)},
				{Cmd: fmt.Sprintf("queue_pwm_out oid=%d clock=%d value=%d", oid, plan.LastClock, plan.InitialQueueValue), OnRestart: true},
			},
			LookupFormat: "queue_pwm_out oid=%c clock=%u value=%hu",
		}
	}
	return PWMConfigSetupPlan{
		Commands: []ConfigCommand{
			{Cmd: fmt.Sprintf("config_digital_out oid=%d pin=%s value=%d default_value=%d max_duration=%d", oid, pin, plan.StartConfigValue, plan.ShutdownConfigValue, plan.MaxDurationTicks)},
			{Cmd: fmt.Sprintf("set_digital_out_pwm_cycle oid=%d cycle_ticks=%d", oid, plan.CycleTicks)},
			{Cmd: fmt.Sprintf("queue_digital_out oid=%d clock=%d on_ticks=%d", oid, plan.LastClock, plan.InitialQueueValue), IsInit: true},
		},
		LookupFormat: "queue_digital_out oid=%c clock=%u on_ticks=%u",
	}
}
