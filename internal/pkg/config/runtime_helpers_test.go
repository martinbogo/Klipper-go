package config

import (
	"strings"
	"testing"
)

func TestRenderConfigProducesTrimmedConfig(t *testing.T) {
	fileconfig := ParseConfigText("[printer]\nmax_velocity = 250\n", "printer.cfg")
	rendered := RenderConfig(fileconfig)
	if strings.HasSuffix(rendered, "\n") {
		t.Fatalf("expected trimmed config output, got trailing newline in %q", rendered)
	}
	if !strings.Contains(rendered, "[printer]") || !strings.Contains(rendered, "max_velocity = 250") {
		t.Fatalf("expected rendered config content, got %q", rendered)
	}
}

func TestBuildStatusSnapshotBuildsNestedMaps(t *testing.T) {
	fileconfig := ParseConfigText("[printer]\nkinematics = cartesian\nmax_velocity = 250\n[extruder]\nrotation_distance = 22.5\n", "printer.cfg")
	snapshot := BuildStatusSnapshot(
		fileconfig,
		map[string]interface{}{"printer:max_velocity": 250.0},
		map[string]interface{}{"printer:max_velocity:": "deprecated"},
	)

	printerStatus, ok := snapshot.RawConfig["printer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected printer raw config section, got %#v", snapshot.RawConfig["printer"])
	}
	if printerStatus["kinematics"] != "cartesian" {
		t.Fatalf("expected kinematics in raw config, got %#v", printerStatus["kinematics"])
	}
	printerSettings, ok := snapshot.Settings["printer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected printer settings section, got %#v", snapshot.Settings["printer"])
	}
	if printerSettings["max_velocity"] != 250.0 {
		t.Fatalf("expected tracked setting, got %#v", printerSettings["max_velocity"])
	}
	if len(snapshot.Warnings) != 1 {
		t.Fatalf("expected one warning, got %d", len(snapshot.Warnings))
	}
	warning, ok := snapshot.Warnings[0].(map[string]string)
	if !ok {
		t.Fatalf("expected warning map, got %#v", snapshot.Warnings[0])
	}
	if warning["section"] != "printer" || warning["option"] != "max_velocity" {
		t.Fatalf("unexpected warning payload: %#v", warning)
	}
}

func TestValidateAutosaveConflictsDetectsIncludedOverride(t *testing.T) {
	autosave := ParseConfigText("[printer]\nmax_velocity = 300\n", "autosave.cfg")
	err := ValidateAutosaveConflicts("[printer]\nmax_velocity = 200\n", "printer.cfg", autosave)
	if err == nil {
		t.Fatal("expected autosave conflict error")
	}
	if !strings.Contains(err.Error(), "conflicts with included value") {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := ValidateAutosaveConflicts("[printer]\nmax_accel = 3000\n", "printer.cfg", autosave); err != nil {
		t.Fatalf("expected non-conflicting autosave to pass, got %v", err)
	}
}

func TestBuildMainConfigBundleStripsAutosaveDuplicates(t *testing.T) {
	autosaveBlock := FormatAutosaveBlock(strings.Join([]string{
		"[printer]",
		"max_velocity = 250",
		"max_accel = 3000",
	}, "\n"))
	bundle := BuildMainConfigBundle(strings.Join([]string{
		"[printer]",
		"max_velocity = 200",
		strings.TrimRight(autosaveBlock, "\n"),
	}, "\n"), "printer.cfg")

	autosavePrinterOptions, err := bundle.Autosave.Options("printer")
	if err != nil {
		t.Fatalf("expected autosave printer section, got %v", err)
	}
	if _, exists := autosavePrinterOptions["max_velocity"]; exists {
		t.Fatalf("expected duplicate autosave option to be stripped, got %#v", autosavePrinterOptions)
	}
	if bundle.Combined.Get("printer", "max_velocity") != "200" {
		t.Fatalf("expected combined config to prefer regular value, got %#v", bundle.Combined.Get("printer", "max_velocity"))
	}
	if bundle.Combined.Get("printer", "max_accel") != "3000" {
		t.Fatalf("expected combined config to retain unique autosave value, got %#v", bundle.Combined.Get("printer", "max_accel"))
	}
	if bundle.Regular.Get("printer", "max_velocity") != "200" {
		t.Fatalf("expected regular config to retain original value, got %#v", bundle.Regular.Get("printer", "max_velocity"))
	}
}
