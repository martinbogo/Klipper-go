package filament

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPrepareEndlessSpoolChangeMarksCurrentSlotEmpty(t *testing.T) {
	inventory := []map[string]interface{}{
		ReadyInventorySlot([]interface{}{1, 2, 3}, "PLA", 210),
		ReadyInventorySlot([]interface{}{4, 5, 6}, "PETG", 240),
		EmptyInventorySlot(),
		EmptyInventorySlot(),
	}
	variables := map[string]interface{}{}
	aceInfo := map[string]interface{}{
		"slots": []interface{}{
			map[string]interface{}{"status": "ready"},
			map[string]interface{}{"status": "ready"},
			map[string]interface{}{"status": "empty"},
			map[string]interface{}{"status": "empty"},
		},
	}

	nextTool, payload, changed, err := PrepareEndlessSpoolChange(0, inventory, aceInfo, variables)
	if err != nil {
		t.Fatalf("PrepareEndlessSpoolChange returned error: %v", err)
	}
	if nextTool != 1 {
		t.Fatalf("expected next tool 1, got %d", nextTool)
	}
	if !changed {
		t.Fatalf("expected inventoryChanged to be true")
	}
	if inventory[0]["status"] != "empty" {
		t.Fatalf("expected current slot to be emptied, got %#v", inventory[0])
	}
	if !reflect.DeepEqual(variables["ace_inventory"], inventory) {
		t.Fatalf("expected variables ace_inventory to be updated")
	}

	var persisted []map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &persisted); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if persisted[0]["status"] != "empty" {
		t.Fatalf("expected persisted current slot to be empty, got %#v", persisted[0])
	}
}

func TestBuildACEHubStatusDoesNotMutateInput(t *testing.T) {
	info := map[string]interface{}{
		"status": "ready",
		"slots": []interface{}{
			map[string]interface{}{"status": "ready"},
		},
	}
	fwInfo := map[string]interface{}{"model": "ACE 2.0"}

	status := BuildACEHubStatus(info, fwInfo, true, true, false)
	if status["auto_refill"] != 1 {
		t.Fatalf("expected auto_refill to be 1, got %v", status["auto_refill"])
	}

	hubs, ok := status["filament_hubs"].([]interface{})
	if !ok || len(hubs) != 1 {
		t.Fatalf("expected one filament hub, got %#v", status["filament_hubs"])
	}
	hub, ok := hubs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hub payload to be a map, got %#v", hubs[0])
	}
	if hub["id"] != 0 {
		t.Fatalf("expected hub id to be 0, got %v", hub["id"])
	}
	if _, ok := hub["endless_spool"].(map[string]interface{}); !ok {
		t.Fatalf("expected endless_spool payload, got %#v", hub["endless_spool"])
	}
	if !reflect.DeepEqual(hub["fw_info"], fwInfo) {
		t.Fatalf("expected fw_info to be copied into hub status, got %#v", hub["fw_info"])
	}

	if _, exists := info["id"]; exists {
		t.Fatalf("expected input info map to remain untouched, got %#v", info)
	}
	if _, exists := info["endless_spool"]; exists {
		t.Fatalf("expected input info map to remain untouched, got %#v", info)
	}
	if _, exists := info["fw_info"]; exists {
		t.Fatalf("expected input info map to remain untouched, got %#v", info)
	}
}
