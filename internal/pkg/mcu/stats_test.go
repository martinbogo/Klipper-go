package mcu

import (
	"math"
	"strings"
	"testing"
)

func TestStatsStateHandleMCUStats(t *testing.T) {
	state := &StatsState{}
	state.HandleMCUStats(map[string]interface{}{"count": int64(5), "sum": int64(10), "sumsq": int64(30)}, 100.0, 1.0)
	if math.Abs(state.TickAvg-0.02) > 1e-9 {
		t.Fatalf("expected tick avg 0.02, got %v", state.TickAvg)
	}
	if math.Abs(state.TickStddev-0.01414213562373095) > 1e-9 {
		t.Fatalf("unexpected tick stddev %v", state.TickStddev)
	}
	if math.Abs(state.TickAwake-0.1) > 1e-9 {
		t.Fatalf("expected tick awake 0.1, got %v", state.TickAwake)
	}
}

func TestStatsStateBuildStatsSummary(t *testing.T) {
	state := &StatsState{TickAwake: 0.1, TickAvg: 0.02, TickStddev: 0.014}
	ok, summary, lastStats := state.BuildStatsSummary("mcu", "serial_rx=7", "clock_sync=1.500")
	if !ok {
		t.Fatalf("expected ok summary")
	}
	if !strings.HasPrefix(summary, "mcu: mcu_awake=0.100") {
		t.Fatalf("unexpected summary %q", summary)
	}
	if lastStats["serial_rx"] != 7 {
		t.Fatalf("expected serial_rx int metric, got %#v", lastStats["serial_rx"])
	}
	if math.Abs(lastStats["clock_sync"].(float64)-1.5) > 1e-9 {
		t.Fatalf("expected clock_sync float metric, got %#v", lastStats["clock_sync"])
	}
}

func TestBuildReadyFrequencyCheck(t *testing.T) {
	matching := BuildReadyFrequencyCheck(false, 16000000, 10, func(value float64) int64 {
		return int64(value * 16000000)
	})
	if matching.Skip || matching.IsMismatch {
		t.Fatalf("expected matching frequency check, got %#v", matching)
	}
	mismatch := BuildReadyFrequencyCheck(false, 16000000, 10, func(value float64) int64 {
		return int64(value * 14000000)
	})
	if !mismatch.IsMismatch || mismatch.MCUFreqMHz != 16 || mismatch.CalcFreqMHz != 14 {
		t.Fatalf("expected mismatch report, got %#v", mismatch)
	}
	skipped := BuildReadyFrequencyCheck(true, 16000000, 10, func(value float64) int64 {
		return int64(value)
	})
	if !skipped.Skip {
		t.Fatalf("expected fileoutput check to skip, got %#v", skipped)
	}
}
