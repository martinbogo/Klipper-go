package mcu

import "testing"

func TestBuildDigitalOutConfigSetupPlan(t *testing.T) {
	plan := BuildDigitalOutConfigSetupPlan(7, "PA1", 1, 1, DigitalOutConfigPlan{MaxDurationTicks: 2000})
	if len(plan.Commands) != 2 {
		t.Fatalf("expected two digital-out commands, got %d", len(plan.Commands))
	}
	if plan.Commands[0].Cmd != "config_digital_out oid=7 pin=PA1 value=1 default_value=1 max_duration=2000" {
		t.Fatalf("unexpected config command %q", plan.Commands[0].Cmd)
	}
	if plan.Commands[0].IsInit || plan.Commands[0].OnRestart {
		t.Fatalf("expected config command to be a normal config command %#v", plan.Commands[0])
	}
	if plan.Commands[1].Cmd != "update_digital_out oid=7 value=1" || !plan.Commands[1].OnRestart || plan.Commands[1].IsInit {
		t.Fatalf("unexpected restart command %#v", plan.Commands[1])
	}
	if plan.LookupFormat != "queue_digital_out oid=%c clock=%u on_ticks=%u" {
		t.Fatalf("unexpected lookup format %q", plan.LookupFormat)
	}
}

func TestBuildPWMConfigSetupPlanHardware(t *testing.T) {
	plan := BuildPWMConfigSetupPlan(4, "PB2", true, PWMConfigPlan{
		CycleTicks:          100,
		MaxDurationTicks:    500,
		StartConfigValue:    63,
		ShutdownConfigValue: 127,
		InitialQueueValue:   64,
		LastClock:           11200,
	})
	if len(plan.Commands) != 2 {
		t.Fatalf("expected two hardware PWM commands, got %d", len(plan.Commands))
	}
	if plan.Commands[0].Cmd != "config_pwm_out oid=4 pin=PB2 cycle_ticks=100 value=63 default_value=127 max_duration=500" {
		t.Fatalf("unexpected hardware config command %q", plan.Commands[0].Cmd)
	}
	if plan.Commands[1].Cmd != "queue_pwm_out oid=4 clock=11200 value=64" || !plan.Commands[1].OnRestart {
		t.Fatalf("unexpected hardware restart command %#v", plan.Commands[1])
	}
	if plan.LookupFormat != "queue_pwm_out oid=%c clock=%u value=%hu" {
		t.Fatalf("unexpected lookup format %q", plan.LookupFormat)
	}
}

func TestBuildPWMConfigSetupPlanSoftware(t *testing.T) {
	plan := BuildPWMConfigSetupPlan(9, "PC3", false, PWMConfigPlan{
		CycleTicks:          250,
		MaxDurationTicks:    500,
		StartConfigValue:    0,
		ShutdownConfigValue: 1,
		InitialQueueValue:   188,
		LastClock:           5700,
	})
	if len(plan.Commands) != 3 {
		t.Fatalf("expected three software PWM commands, got %d", len(plan.Commands))
	}
	if plan.Commands[0].Cmd != "config_digital_out oid=9 pin=PC3 value=0 default_value=1 max_duration=500" {
		t.Fatalf("unexpected software pwm config command %q", plan.Commands[0].Cmd)
	}
	if plan.Commands[1].Cmd != "set_digital_out_pwm_cycle oid=9 cycle_ticks=250" || plan.Commands[1].IsInit || plan.Commands[1].OnRestart {
		t.Fatalf("unexpected cycle command %#v", plan.Commands[1])
	}
	if plan.Commands[2].Cmd != "queue_digital_out oid=9 clock=5700 on_ticks=188" || !plan.Commands[2].IsInit || plan.Commands[2].OnRestart {
		t.Fatalf("unexpected init queue command %#v", plan.Commands[2])
	}
	if plan.LookupFormat != "queue_digital_out oid=%c clock=%u on_ticks=%u" {
		t.Fatalf("unexpected lookup format %q", plan.LookupFormat)
	}
}
