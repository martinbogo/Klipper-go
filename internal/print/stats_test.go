package print

import (
	"math"
	"testing"
)

func TestStatsPauseResumeAccounting(t *testing.T) {
	stats := NewStats()
	stats.SetCurrentFile("cube.gcode")
	stats.NoteStart(10, ExtrusionStatus{Position: 0, ExtrudeFactor: 1})

	stats.NotePause(13, ExtrusionStatus{Position: 4, ExtrudeFactor: 1})
	stats.NoteStart(18, ExtrusionStatus{Position: 4, ExtrudeFactor: 1})

	status := stats.GetStatus(20, ExtrusionStatus{Position: 6, ExtrudeFactor: 1})
	if got := status["filename"].(string); got != "cube.gcode" {
		t.Fatalf("unexpected filename: %q", got)
	}
	if got := status["total_duration"].(float64); got != 10 {
		t.Fatalf("unexpected total duration: %v", got)
	}
	if got := status["print_duration"].(float64); got != 5 {
		t.Fatalf("unexpected print duration: %v", got)
	}
	if got := status["filament_used"].(float64); got != 6 {
		t.Fatalf("unexpected filament usage: %v", got)
	}

	stats.SetInfo(12, 20)
	info := stats.GetStatus(20, ExtrusionStatus{Position: 6, ExtrudeFactor: 1})["info"].(map[string]int)
	if info["total_layer"] != 12 || info["current_layer"] != 12 {
		t.Fatalf("unexpected layer info: %#v", info)
	}

	stats.NoteComplete(21)
	status = stats.GetStatus(21, ExtrusionStatus{Position: 6, ExtrudeFactor: 1})
	if got := status["state"].(string); got != "complete" {
		t.Fatalf("unexpected final state: %q", got)
	}
	if got := status["total_duration"].(float64); got != 11 {
		t.Fatalf("unexpected completed duration: %v", got)
	}
}

func TestStatsZeroExtrudeFactorDoesNotExplode(t *testing.T) {
	stats := NewStats()
	stats.SetCurrentFile("zero.gcode")
	stats.NoteStart(0, ExtrusionStatus{Position: 1, ExtrudeFactor: 1})

	status := stats.GetStatus(2, ExtrusionStatus{Position: 5, ExtrudeFactor: 0})
	filamentUsed := status["filament_used"].(float64)
	if math.IsNaN(filamentUsed) || math.IsInf(filamentUsed, 0) {
		t.Fatalf("filament usage should remain finite, got %v", filamentUsed)
	}
	if filamentUsed != 0 {
		t.Fatalf("expected zero filament usage when extrude factor is zero, got %v", filamentUsed)
	}
}
