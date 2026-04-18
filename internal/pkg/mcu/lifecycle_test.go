package mcu

import (
	"testing"
	"time"
)

func TestLifecycleStateTracksShutdownAndTimeout(t *testing.T) {
	state := NewLifecycleState()
	if state.ShutdownActive() {
		t.Fatalf("expected new lifecycle state to start inactive")
	}
	if message := state.StartingShutdownMessage("mcu"); message != "MCU 'mcu' spontaneous restart" {
		t.Fatalf("unexpected starting shutdown message %q", message)
	}
	plan := ShutdownPlan{HasShutdownClock: true, ShutdownClock: 42, ShutdownMessage: "fatal"}
	if !state.HandleShutdownPlan(plan) {
		t.Fatalf("expected first shutdown plan to apply")
	}
	if state.ShutdownClock() != 42 || state.ShutdownMessage() != "fatal" || !state.ShutdownActive() {
		t.Fatalf("unexpected lifecycle state after shutdown")
	}
	if state.HandleShutdownPlan(ShutdownPlan{HasShutdownClock: true, ShutdownClock: 99, ShutdownMessage: "ignored"}) {
		t.Fatalf("expected repeated shutdown plan to be ignored")
	}
	state.ClearShutdown()
	if state.ShutdownActive() {
		t.Fatalf("expected shutdown clear to reset active state")
	}
	state.MarkShutdown()
	if !state.ShutdownActive() {
		t.Fatalf("expected mark shutdown to set active state")
	}
	state.SetTimeout(true)
	if !state.TimeoutActive() {
		t.Fatalf("expected timeout state to be tracked")
	}
}

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

func TestExecuteFirmwareRestartPlanRoutesAction(t *testing.T) {
	steps := []string{}
	ExecuteFirmwareRestartPlan(FirmwareRestartPlan{Action: FirmwareRestartActionCommand}, FirmwareRestartExecutionHooks{
		RestartRPIUSB: func() {
			steps = append(steps, "rpi_usb")
		},
		RestartViaCommand: func() {
			steps = append(steps, "command")
		},
		RestartCheetah: func() {
			steps = append(steps, "cheetah")
		},
		RestartArduino: func() {
			steps = append(steps, "arduino")
		},
	})
	if len(steps) != 1 || steps[0] != "command" {
		t.Fatalf("expected command restart path, got %#v", steps)
	}

	steps = steps[:0]
	ExecuteFirmwareRestartPlan(FirmwareRestartPlan{Action: FirmwareRestartActionRPIUSB}, FirmwareRestartExecutionHooks{
		RestartRPIUSB: func() {
			steps = append(steps, "rpi_usb")
		},
	})
	if len(steps) != 1 || steps[0] != "rpi_usb" {
		t.Fatalf("expected rpi_usb restart path, got %#v", steps)
	}

	steps = steps[:0]
	ExecuteFirmwareRestartPlan(FirmwareRestartPlan{Skip: true}, FirmwareRestartExecutionHooks{
		RestartArduino: func() {
			steps = append(steps, "arduino")
		},
	})
	if len(steps) != 0 {
		t.Fatalf("expected skipped restart plan to do nothing, got %#v", steps)
	}
}

func TestExecuteCommandResetConfigReset(t *testing.T) {
	plan := BuildCommandResetPlan(false, true, true, "mcu")
	steps := []string{}
	errorMessage := ExecuteCommandReset(plan, CommandResetExecutionHooks{
		DebugLog: func(message string) {
			steps = append(steps, "debug:"+message)
		},
		MarkShutdown: func() {
			steps = append(steps, "mark")
		},
		SendEmergencyStop: func(force bool) {
			if !force {
				t.Fatalf("expected forced emergency stop")
			}
			steps = append(steps, "emergency_stop")
		},
		PauseSeconds: func(seconds float64) {
			if seconds != plan.PreSendPauseSeconds {
				t.Fatalf("unexpected pause seconds %f", seconds)
			}
			steps = append(steps, "pause")
		},
		SendConfigReset: func() {
			steps = append(steps, "config_reset")
		},
		Sleep: func(duration time.Duration) {
			if duration != time.Duration(plan.PostSendPauseSeconds*float64(time.Second)) {
				t.Fatalf("unexpected sleep duration %s", duration)
			}
			steps = append(steps, "sleep")
		},
		Disconnect: func() {
			steps = append(steps, "disconnect")
		},
	})
	if errorMessage != "" {
		t.Fatalf("unexpected execute reset error %q", errorMessage)
	}
	expected := []string{"debug:" + plan.LogMessage, "mark", "emergency_stop", "pause", "config_reset", "sleep", "disconnect"}
	if len(steps) != len(expected) {
		t.Fatalf("unexpected step count %#v", steps)
	}
	for i, step := range expected {
		if steps[i] != step {
			t.Fatalf("unexpected step order %#v", steps)
		}
	}
}

func TestExecuteCommandResetResetCommand(t *testing.T) {
	plan := BuildCommandResetPlan(true, true, true, "mcu")
	resetSent := false
	errorMessage := ExecuteCommandReset(plan, CommandResetExecutionHooks{
		SendReset: func() {
			resetSent = true
		},
	})
	if errorMessage != "" {
		t.Fatalf("unexpected execute reset error %q", errorMessage)
	}
	if !resetSent {
		t.Fatalf("expected reset command to be sent")
	}
}
