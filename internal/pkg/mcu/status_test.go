package mcu

import "testing"

func TestBuildEmergencyStopDecision(t *testing.T) {
	if !BuildEmergencyStopDecision(false, false, false).Skip {
		t.Fatalf("expected missing emergency stop command to skip")
	}
	if !BuildEmergencyStopDecision(true, true, false).Skip {
		t.Fatalf("expected shutdown without force to skip")
	}
	if BuildEmergencyStopDecision(true, true, true).Skip {
		t.Fatalf("expected forced shutdown to proceed")
	}
	if BuildEmergencyStopDecision(true, false, false).Skip {
		t.Fatalf("expected active emergency stop to proceed")
	}
}

func TestBuildMCUConstantSummary(t *testing.T) {
	summary := BuildMCUConstantSummary(map[string]interface{}{
		"B": 2,
		"A": "one",
	})
	if summary != "A=one B=2" {
		t.Fatalf("unexpected constant summary %q", summary)
	}
	if BuildMCUConstantSummary(nil) != "" {
		t.Fatalf("expected empty constant summary for nil input")
	}
}

func TestBuildMCULogInfo(t *testing.T) {
	info := BuildMCULogInfo("mcu", 12, "v1", "build1", map[string]interface{}{"CLOCK_FREQ": 16000000})
	expected := "Loaded MCU 'mcu' 12 commands (v1 / build1) MCU 'mcu' config: CLOCK_FREQ=16000000"
	if info != expected {
		t.Fatalf("unexpected log info %q", info)
	}
}

func TestBuildConfiguredMCUInfo(t *testing.T) {
	info := BuildConfiguredMCUInfo("mcu", 42, "base-info")
	if info.MoveMessage != "Configured MCU 'mcu' (42 moves)" {
		t.Fatalf("unexpected move message %q", info.MoveMessage)
	}
	if info.RolloverInfo != "base-info\nConfigured MCU 'mcu' (42 moves)" {
		t.Fatalf("unexpected rollover info %q", info.RolloverInfo)
	}
}
