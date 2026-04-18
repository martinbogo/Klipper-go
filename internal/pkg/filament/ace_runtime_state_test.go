package filament

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewACERuntimeStateLoadsPersistedConfigAndDefaults(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "ams_config.cfg")
	data := map[string]interface{}{
		"filaments": map[string]interface{}{
			"1": map[string]interface{}{
				"type":      "PLA",
				"color":     []interface{}{1.0, 2.0, 3.0},
				"colors":    []interface{}{[]interface{}{1.0, 2.0, 3.0, 255}},
				"sku":       "custom",
				"rfid":      1,
				"source":    2,
				"icon_type": 9,
			},
		},
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	state := NewACERuntimeState(nil, true, cfgPath)

	if !state.EndlessSpoolEnabled {
		t.Fatalf("expected endless spool to be enabled")
	}
	if state.FeedAssistIndex != -1 {
		t.Fatalf("expected feed assist index to default to -1, got %d", state.FeedAssistIndex)
	}
	if got := state.CustomSlots[1]["type"]; got != "PLA" {
		t.Fatalf("expected custom slot type from config, got %#v", got)
	}
	if got := state.Info["slots"].([]interface{})[1].(map[string]interface{})["type"]; got != "PLA" {
		t.Fatalf("expected default info to reflect custom slot metadata, got %#v", got)
	}
	if got := state.Variables["ace_current_index"]; got != int64(0) {
		t.Fatalf("expected default current index, got %#v", got)
	}
	if len(state.Inventory) != 4 {
		t.Fatalf("expected default inventory, got %#v", state.Inventory)
	}
}

func TestACERuntimeStateApplyInventorySlotUpdate(t *testing.T) {
	state := NewACERuntimeState(nil, false, "")

	saveValue, response, err := state.ApplyInventorySlotUpdate(ACEInventorySlotUpdate{
		Index:    2,
		Color:    "1,2,3",
		Material: "PETG",
		Temp:     235,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response != "ACE: Slot 2 set: color=[1 2 3], material=PETG, temp=235" {
		t.Fatalf("unexpected response: %q", response)
	}
	if state.Inventory[2]["status"] != "ready" {
		t.Fatalf("expected slot to be ready, got %#v", state.Inventory[2])
	}
	var persisted []map[string]interface{}
	if err := json.Unmarshal([]byte(saveValue), &persisted); err != nil {
		t.Fatalf("unmarshal save payload: %v", err)
	}
	if persisted[2]["material"] != "PETG" {
		t.Fatalf("expected material to persist, got %#v", persisted[2])
	}

	saveValue, response, err = state.ApplyInventorySlotUpdate(ACEInventorySlotUpdate{Index: 2, Empty: true})
	if err != nil {
		t.Fatalf("unexpected empty-slot error: %v", err)
	}
	if response != "ACE: Slot 2 set to empty" {
		t.Fatalf("unexpected empty response: %q", response)
	}
	if state.Inventory[2]["status"] != "empty" {
		t.Fatalf("expected slot to be empty, got %#v", state.Inventory[2])
	}
	if err := json.Unmarshal([]byte(saveValue), &persisted); err != nil {
		t.Fatalf("unmarshal empty save payload: %v", err)
	}
	if persisted[2]["status"] != "empty" {
		t.Fatalf("expected persisted slot to be empty, got %#v", persisted[2])
	}
}

func TestACERuntimeStateSyncStatusResultPersistsRFIDSlots(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "ams_config.cfg")
	state := NewACERuntimeState(nil, false, cfgPath)

	status := map[string]interface{}{
		"slots": []interface{}{
			map[string]interface{}{
				"status":    "ready",
				"type":      "ASA",
				"color":     []interface{}{4.0, 5.0, 6.0},
				"sku":       "rfid-slot",
				"rfid":      123,
				"source":    8,
				"icon_type": 2,
			},
		},
	}

	state.SyncStatusResult(status)

	if got := state.Info["slots"].([]interface{})[0].(map[string]interface{})["type"]; got != "ASA" {
		t.Fatalf("expected state info to update, got %#v", got)
	}
	if got := state.CustomSlots[0]["type"]; got != "ASA" {
		t.Fatalf("expected custom slot to sync from status, got %#v", got)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	var persisted map[string]interface{}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("unmarshal persisted config: %v", err)
	}
	filaments := persisted["filaments"].(map[string]interface{})
	slot := filaments["0"].(map[string]interface{})
	if slot["type"] != "ASA" {
		t.Fatalf("expected persisted slot type, got %#v", slot)
	}
	if slot["sku"] != "rfid-slot" {
		t.Fatalf("expected persisted sku, got %#v", slot)
	}
}

func TestACERuntimeStateUpdatePanelFilamentInfoRespectsRFIDState(t *testing.T) {
	state := NewACERuntimeState(nil, false, "")
	state.Info["slots"] = []interface{}{
		map[string]interface{}{
			"type":      "ABS",
			"color":     []interface{}{7.0, 8.0, 9.0},
			"colors":    []interface{}{[]interface{}{7.0, 8.0, 9.0, 255}},
			"sku":       "rfid-slot",
			"rfid":      77,
			"source":    4,
			"icon_type": 5,
		},
	}

	persisted, ok := state.UpdatePanelFilamentInfo(0, "PLA", []interface{}{1.0, 1.0, 1.0})
	if !ok {
		t.Fatalf("expected RFID slot update to request persistence")
	}
	if persisted["type"] != "ABS" {
		t.Fatalf("expected RFID metadata to win, got %#v", persisted)
	}
	if got := state.CustomSlots[0]["type"]; got != "" {
		t.Fatalf("expected RFID path not to overwrite custom slot metadata, got %#v", got)
	}
}

func TestACERuntimeStateSetPanelFilamentInfoPersistsConfiguredSlot(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "ams_config.cfg")
	state := NewACERuntimeState(nil, false, cfgPath)

	state.SetPanelFilamentInfo(1, "PETG", "", []interface{}{10.0, 20.0, 30.0})

	if got := state.CustomSlots[1]["type"]; got != "PETG" {
		t.Fatalf("expected custom slot update, got %#v", got)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	var persisted map[string]interface{}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("unmarshal persisted config: %v", err)
	}
	filaments := persisted["filaments"].(map[string]interface{})
	slot := filaments["1"].(map[string]interface{})
	if slot["type"] != "PETG" {
		t.Fatalf("expected persisted slot type, got %#v", slot)
	}
	if color, ok := slot["color"].([]interface{}); !ok || len(color) != 3 {
		t.Fatalf("expected persisted slot color, got %#v", slot["color"])
	}
}

func TestACERuntimeStateToggleEndlessSpoolTracksFlagsAndPersistence(t *testing.T) {
	state := NewACERuntimeState(nil, false, "")
	state.EndlessSpoolRunoutDetected = true
	state.EndlessSpoolInProgress = true

	saveValue, response := state.ToggleEndlessSpool(true)
	if saveValue != "true" || response == "" || !state.EndlessSpoolEnabled {
		t.Fatalf("unexpected enable toggle result: save=%q response=%q enabled=%v", saveValue, response, state.EndlessSpoolEnabled)
	}

	saveValue, response = state.ToggleEndlessSpool(false)
	if saveValue != "false" || response == "" {
		t.Fatalf("unexpected disable toggle result: save=%q response=%q", saveValue, response)
	}
	if state.EndlessSpoolEnabled || state.EndlessSpoolRunoutDetected || state.EndlessSpoolInProgress {
		t.Fatalf("expected disable to clear endless spool flags: %#v", state)
	}
	if state.Variables["ace_endless_spool_enabled"] != false {
		t.Fatalf("expected persisted variable update, got %#v", state.Variables["ace_endless_spool_enabled"])
	}
}

func TestACERuntimeStateRunoutAndChangeLifecycle(t *testing.T) {
	state := NewACERuntimeState(nil, true, "")
	state.Inventory = []map[string]interface{}{
		ReadyInventorySlot([]interface{}{1, 2, 3}, "PLA", 210),
		ReadyInventorySlot([]interface{}{4, 5, 6}, "PETG", 240),
		EmptyInventorySlot(),
		EmptyInventorySlot(),
	}
	state.Info["slots"] = []interface{}{
		map[string]interface{}{"status": "ready"},
		map[string]interface{}{"status": "ready"},
		map[string]interface{}{"status": "empty"},
		map[string]interface{}{"status": "empty"},
	}
	state.SetCurrentIndex(0)

	if !state.MarkRunoutIfTriggered(false, false) {
		t.Fatal("expected first runout trigger to arm endless spool change")
	}
	if state.MarkRunoutIfTriggered(false, false) {
		t.Fatal("expected duplicate runout trigger to be ignored")
	}

	plan, err := state.PrepareEndlessSpoolChangePlan()
	if err != nil {
		t.Fatalf("PrepareEndlessSpoolChangePlan() error = %v", err)
	}
	if plan.CurrentTool != 0 || plan.NextTool != 1 || !plan.InventoryChanged {
		t.Fatalf("unexpected change plan: %#v", plan)
	}
	if !state.BeginEndlessSpoolChange() {
		t.Fatal("expected first change begin to succeed")
	}
	if state.BeginEndlessSpoolChange() {
		t.Fatal("expected duplicate change begin to be rejected")
	}
	state.CompleteEndlessSpoolChange(plan.NextTool)
	if state.CurrentIndex() != 1 || state.EndlessSpoolInProgress {
		t.Fatalf("expected change completion to update current index and clear in-progress state: %#v", state)
	}

	wasEnabled := state.BeginManualToolchange()
	if !wasEnabled || state.EndlessSpoolEnabled || state.EndlessSpoolRunoutDetected {
		t.Fatalf("unexpected manual toolchange disable state: wasEnabled=%v state=%#v", wasEnabled, state)
	}
	state.RestoreManualToolchange(wasEnabled)
	if !state.EndlessSpoolEnabled {
		t.Fatalf("expected manual toolchange restore to re-enable endless spool")
	}

	state.BeginEndlessSpoolChange()
	state.AbortEndlessSpoolChange()
	if state.EndlessSpoolInProgress {
		t.Fatalf("expected abort to clear in-progress state")
	}
}
