package config

import "testing"

func TestRuntimeStatusTracksPendingAutosaveMutations(t *testing.T) {
	status := NewRuntimeStatus()
	status.NotePendingSet("bed_mesh default", "version", "1")
	status.NotePendingSet("bed_mesh default", "points", "0.1,0.2")

	snapshot := status.Snapshot()
	if snapshot["save_config_pending"] != true {
		t.Fatalf("expected pending save flag, got %#v", snapshot["save_config_pending"])
	}
	pending := snapshot["save_config_pending_items"].(map[string]interface{})
	section := pending["bed_mesh default"].(map[string]interface{})
	if section["version"] != "1" || section["points"] != "0.1,0.2" {
		t.Fatalf("unexpected pending section contents: %#v", section)
	}

	status.NotePendingRemoval("bed_mesh default", false)
	pending = status.Snapshot()["save_config_pending_items"].(map[string]interface{})
	if _, ok := pending["bed_mesh default"]; ok {
		t.Fatalf("expected pending section removal, got %#v", pending)
	}
}

func TestRuntimeStatusRebuildIncludesWarningsAndTrackedSettings(t *testing.T) {
	status := NewRuntimeStatus()
	status.Deprecate("printer", "kinematics", "", "deprecated")
	config := ParseConfigText("[printer]\nkinematics = cartesian\n", "printer.cfg")
	status.Rebuild(config, map[string]interface{}{"printer:kinematics": "cartesian"})

	snapshot := status.Snapshot()
	settings := snapshot["settings"].(map[string]interface{})
	printer := settings["printer"].(map[string]interface{})
	if printer["kinematics"] != "cartesian" {
		t.Fatalf("expected tracked setting, got %#v", printer)
	}
	warnings := snapshot["warnings"].([]interface{})
	if len(warnings) != 1 {
		t.Fatalf("expected deprecation warning, got %#v", warnings)
	}
}