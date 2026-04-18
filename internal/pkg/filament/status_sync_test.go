package filament

import (
	"reflect"
	"testing"
)

func TestSyncACEStatusSlotsPersistsRFIDBackedSlot(t *testing.T) {
	customSlots := DefaultCustomSlots()
	status := map[string]interface{}{
		"slots": []interface{}{
			map[string]interface{}{
				"type":      "PLA+",
				"color":     []interface{}{1.0, 2.0, 3.0},
				"sku":       "sku-123",
				"rfid":      9,
				"source":    7,
				"icon_type": 4,
			},
		},
	}

	persisted := SyncACEStatusSlots(status, customSlots)
	if len(persisted) != 1 {
		t.Fatalf("expected one persisted slot, got %#v", persisted)
	}

	slot := customSlots[0]
	if slot["type"] != "PLA+" {
		t.Fatalf("expected custom slot type to be updated, got %v", slot["type"])
	}
	if !reflect.DeepEqual(slot["color"], []interface{}{1.0, 2.0, 3.0}) {
		t.Fatalf("expected custom slot color to be updated, got %#v", slot["color"])
	}
	if !reflect.DeepEqual(slot["colors"], []interface{}{[]interface{}{1.0, 2.0, 3.0, 255}}) {
		t.Fatalf("expected generated ACE colors, got %#v", slot["colors"])
	}
	if !reflect.DeepEqual(status["slots"].([]interface{})[0].(map[string]interface{})["colors"], []interface{}{[]interface{}{1.0, 2.0, 3.0, 255}}) {
		t.Fatalf("expected live status colors to be populated")
	}
	if !reflect.DeepEqual(persisted[0]["colors"], []interface{}{[]interface{}{1.0, 2.0, 3.0, 255}}) {
		t.Fatalf("expected persisted payload to include generated colors, got %#v", persisted[0]["colors"])
	}
}

func TestSyncACEStatusSlotsRestoresCustomMetadataForCustomSlot(t *testing.T) {
	customSlots := DefaultCustomSlots()
	customSlots[0]["type"] = "PETG"
	customSlots[0]["color"] = []interface{}{9.0, 8.0, 7.0}
	customSlots[0]["colors"] = []interface{}{[]interface{}{9.0, 8.0, 7.0, 255}}

	status := map[string]interface{}{
		"slots": []interface{}{
			map[string]interface{}{
				"status":    "busy",
				"sku":       "custom",
				"type":      "?",
				"color":     []interface{}{0.0, 0.0, 0.0},
				"colors":    []interface{}{[]interface{}{0.0, 0.0, 0.0, 255}},
				"rfid":      99,
				"source":    99,
				"icon_type": 99,
				"remain":    10,
				"decorder":  12,
			},
		},
	}

	persisted := SyncACEStatusSlots(status, customSlots)
	if persisted != nil {
		t.Fatalf("expected no persistence payload for non-RFID slot, got %#v", persisted)
	}

	slot := status["slots"].([]interface{})[0].(map[string]interface{})
	if slot["status"] != "ready" {
		t.Fatalf("expected status to be reset to ready, got %v", slot["status"])
	}
	if slot["type"] != "PETG" {
		t.Fatalf("expected custom type to be restored, got %v", slot["type"])
	}
	if !reflect.DeepEqual(slot["color"], []interface{}{9.0, 8.0, 7.0}) {
		t.Fatalf("expected custom color to be restored, got %#v", slot["color"])
	}
	if !reflect.DeepEqual(slot["colors"], []interface{}{[]interface{}{9.0, 8.0, 7.0, 255}}) {
		t.Fatalf("expected custom colors to be restored, got %#v", slot["colors"])
	}
	if slot["rfid"] != 1 || slot["source"] != 2 || slot["icon_type"] != 0 {
		t.Fatalf("expected custom slot defaults to be restored, got rfid=%v source=%v icon_type=%v", slot["rfid"], slot["source"], slot["icon_type"])
	}
}
