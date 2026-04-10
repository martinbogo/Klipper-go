package mcu

import "testing"

func TestBuildShutdownPlan(t *testing.T) {
	plan := BuildShutdownPlan("mcu", map[string]interface{}{
		"clock":            int64(42),
		"static_string_id": "fatal error",
		"#name":            "shutdown",
	}, "clock=123", "serial=456")
	if !plan.HasShutdownClock || plan.ShutdownClock != 42 {
		t.Fatalf("unexpected shutdown clock %#v", plan)
	}
	if plan.ShutdownMessage != "fatal error" {
		t.Fatalf("unexpected shutdown message %q", plan.ShutdownMessage)
	}
	if plan.AsyncMessage != "MCU 'mcu' shutdown: fatal error" {
		t.Fatalf("unexpected async message %q", plan.AsyncMessage)
	}
	if plan.RespondInfo != "MCU 'mcu' shutdown: fatal error" {
		t.Fatalf("unexpected respond info %q", plan.RespondInfo)
	}
	if plan.LogMessage != "MCU 'mcu' shutdown: fatal error\nclock=123\nserial=456" {
		t.Fatalf("unexpected log message %q", plan.LogMessage)
	}
}

func TestBuildShutdownPlanPreviousShutdownPrefix(t *testing.T) {
	plan := BuildShutdownPlan("mcu", map[string]interface{}{
		"static_string_id": "old error",
		"#name":            "is_shutdown",
	}, "clock=123", "serial=456")
	if plan.AsyncMessage != "Previous MCU 'mcu' shutdown: old error" {
		t.Fatalf("unexpected previous shutdown async message %q", plan.AsyncMessage)
	}
	if plan.HasShutdownClock {
		t.Fatalf("expected missing shutdown clock, got %#v", plan)
	}
}

func TestBuildStartingShutdownMessage(t *testing.T) {
	if msg := BuildStartingShutdownMessage(true, "mcu"); msg != "" {
		t.Fatalf("expected no message while shutdown already active, got %q", msg)
	}
	if msg := BuildStartingShutdownMessage(false, "mcu"); msg != "MCU 'mcu' spontaneous restart" {
		t.Fatalf("unexpected spontaneous restart message %q", msg)
	}
}

func TestBuildRestartCheckDecision(t *testing.T) {
	decision := BuildRestartCheckDecision("firmware_restart", "mcu", "CRC mismatch")
	if !decision.Skip {
		t.Fatalf("expected firmware restart to skip, got %#v", decision)
	}
	decision = BuildRestartCheckDecision("startup", "mcu", "CRC mismatch")
	if decision.Skip {
		t.Fatalf("expected restart check to proceed, got %#v", decision)
	}
	if decision.ExitReason != "firmware_restart" || decision.PauseSeconds != 2.0 {
		t.Fatalf("unexpected restart check decision %#v", decision)
	}
	if decision.PanicMessage != "Attempt MCU 'mcu' restart failed" {
		t.Fatalf("unexpected panic message %q", decision.PanicMessage)
	}
	if decision.LogMessage != "Attempting automated MCU 'mcu' restart: CRC mismatch" {
		t.Fatalf("unexpected log message %q", decision.LogMessage)
	}
}

func TestBuildCommandResetPlan(t *testing.T) {
	plan := BuildCommandResetPlan(false, false, true, "mcu")
	if plan.ErrorMessage != "Unable to issue reset command on MCU 'mcu'" {
		t.Fatalf("unexpected error plan %#v", plan)
	}
	plan = BuildCommandResetPlan(false, true, true, "mcu")
	if plan.Mode != CommandResetModeConfigReset || !plan.MarkShutdown || !plan.NeedsEmergencyStop {
		t.Fatalf("unexpected config reset plan %#v", plan)
	}
	if plan.PreSendPauseSeconds != 0.015 || plan.PostSendPauseSeconds != 0.015 {
		t.Fatalf("unexpected config reset timing %#v", plan)
	}
	plan = BuildCommandResetPlan(true, true, true, "mcu")
	if plan.Mode != CommandResetModeReset || plan.MarkShutdown {
		t.Fatalf("unexpected reset plan %#v", plan)
	}
	if plan.LogMessage != "Attempting MCU 'mcu' reset command" {
		t.Fatalf("unexpected reset log message %q", plan.LogMessage)
	}
}

func TestBuildFirmwareRestartPlan(t *testing.T) {
	plan := BuildFirmwareRestartPlan(false, true, "command")
	if !plan.Skip {
		t.Fatalf("expected MCU bridge restart to skip without force, got %#v", plan)
	}
	plan = BuildFirmwareRestartPlan(true, true, "command")
	if plan.Skip || plan.Action != FirmwareRestartActionCommand {
		t.Fatalf("unexpected forced bridge restart plan %#v", plan)
	}
	if BuildFirmwareRestartPlan(false, false, "rpi_usb").Action != FirmwareRestartActionRPIUSB {
		t.Fatalf("expected rpi_usb action")
	}
	if BuildFirmwareRestartPlan(false, false, "cheetah").Action != FirmwareRestartActionCheetah {
		t.Fatalf("expected cheetah action")
	}
	if BuildFirmwareRestartPlan(false, false, "arduino").Action != FirmwareRestartActionArduino {
		t.Fatalf("expected default arduino action")
	}
}
