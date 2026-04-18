package config

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSaveConfigPlanFormatsAutosaveAndFilenames(t *testing.T) {
	autosave := ParseConfigText("[printer]\nmax_velocity = 250\n", "autosave.cfg")
	plan, err := BuildSaveConfigPlan(
		"/tmp/printer.cfg",
		"[printer]\nmax_accel = 3000\n",
		autosave,
		func() time.Time { return time.Date(2026, time.April, 11, 10, 9, 8, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("BuildSaveConfigPlan returned error: %v", err)
	}
	if plan.BackupName != "/tmp/printer-20260411_100908.cfg" {
		t.Fatalf("unexpected backup name: %q", plan.BackupName)
	}
	if plan.TempName != "/tmp/printer_autosave.cfg" {
		t.Fatalf("unexpected temp name: %q", plan.TempName)
	}
	if !strings.Contains(plan.Data, AutosaveHeader) {
		t.Fatalf("expected autosave header in output: %q", plan.Data)
	}
	if !strings.Contains(plan.Data, "#*# max_velocity = 250") {
		t.Fatalf("expected autosave data in output: %q", plan.Data)
	}
}

func TestBuildSaveConfigPlanCommentsOutDuplicateAutosaveSourceFields(t *testing.T) {
	autosave := ParseConfigText("[printer]\nmax_velocity = 250\n", "autosave.cfg")
	plan, err := BuildSaveConfigPlan(
		"/tmp/printer.cfg",
		"[printer]\nmax_velocity = 300\n",
		autosave,
		time.Now,
	)
	if err != nil {
		t.Fatalf("BuildSaveConfigPlan returned error: %v", err)
	}
	if !strings.Contains(plan.Data, "#max_velocity = 300") {
		t.Fatalf("expected duplicate source field to be commented out, got %q", plan.Data)
	}
}
