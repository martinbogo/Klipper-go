package mcu

import (
	"strings"
	"testing"
)

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

func TestMCUStatusTrackerTracksStatusAndStats(t *testing.T) {
	tracker := NewMCUStatusTracker()
	tracker.SetStatusInfo(map[string]interface{}{"mcu_version": "v1"})
	tracker.SetStatsSumsqBase(1.0)
	if err := tracker.HandleMCUStats(map[string]interface{}{
		"count": int64(2),
		"sum":   int64(4),
		"sumsq": int64(8),
	}, 2.0); err != nil {
		t.Fatalf("unexpected stats error: %v", err)
	}
	ok, summary := tracker.Stats("mcu", "serial=1", "clock=2")
	if !ok {
		t.Fatalf("expected status tracker stats to succeed")
	}
	if !strings.Contains(summary, "mcu: mcu_awake=2.000 mcu_task_avg=1.000000 mcu_task_stddev=0.000000") {
		t.Fatalf("unexpected summary %q", summary)
	}
	lastStats, ok := tracker.StatusInfo()["last_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected last_stats map, got %#v", tracker.StatusInfo()["last_stats"])
	}
	if lastStats["serial"] != 1 || lastStats["clock"] != 2 {
		t.Fatalf("unexpected parsed last_stats %#v", lastStats)
	}
	copy := tracker.GetStatus()
	copy["mcu_version"] = "mutated"
	if tracker.StatusInfo()["mcu_version"] != "v1" {
		t.Fatalf("expected deep-copied status info, got %#v", tracker.StatusInfo())
	}
}
