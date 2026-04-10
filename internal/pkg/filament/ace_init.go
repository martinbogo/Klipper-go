package filament

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// DefaultCustomSlot returns a single default custom slot map.
func DefaultCustomSlot() map[string]interface{} {
	return map[string]interface{}{
		"type": "", "color": []interface{}{0.0, 0.0, 0.0},
		"colors": []interface{}{[]interface{}{0, 0, 0, 0}},
		"sku":    "", "rfid": 1, "source": 2, "icon_type": 0,
	}
}

// DefaultCustomSlots returns 4 default custom slot maps.
func DefaultCustomSlots() []map[string]interface{} {
	return []map[string]interface{}{
		DefaultCustomSlot(),
		DefaultCustomSlot(),
		DefaultCustomSlot(),
		DefaultCustomSlot(),
	}
}

// LoadCustomSlotsFromConfig reads the AMS config file and populates the given
// customSlots slice with persisted filament data (type, color, colors, sku, rfid,
// source, icon_type).
func LoadCustomSlotsFromConfig(customSlots []map[string]interface{}, cfgPath string) {
	amsBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}
	var configData map[string]interface{}
	if err := json.Unmarshal(amsBytes, &configData); err != nil {
		return
	}
	filaments, ok := configData["filaments"].(map[string]interface{})
	if !ok {
		return
	}
	for iStr, fData := range filaments {
		i, err := strconv.Atoi(iStr)
		if err != nil || i < 0 || i >= len(customSlots) {
			continue
		}
		fMap, ok := fData.(map[string]interface{})
		if !ok {
			continue
		}
		if typ, ok := fMap["type"].(string); ok {
			customSlots[i]["type"] = typ
		}
		if color, ok := fMap["color"].([]interface{}); ok {
			customSlots[i]["color"] = color
		}
		if colors, ok := fMap["colors"].([]interface{}); ok {
			customSlots[i]["colors"] = colors
		}
		if sku, ok := fMap["sku"].(string); ok {
			customSlots[i]["sku"] = sku
		}
		if rfid, ok := fMap["rfid"]; ok {
			customSlots[i]["rfid"] = rfid
		}
		if source, ok := fMap["source"]; ok {
			customSlots[i]["source"] = source
		}
		if iconType, ok := fMap["icon_type"]; ok {
			customSlots[i]["icon_type"] = iconType
		}
	}
}

// BuildDefaultACEInfo constructs the default ACE info map with dryer status
// and 4 slots populated from the given customSlots. Slots with empty type
// are marked with status "empty".
func BuildDefaultACEInfo(customSlots []map[string]interface{}) map[string]interface{} {
	slots := make([]interface{}, 4)
	for i := 0; i < 4; i++ {
		slot := map[string]interface{}{
			"index":     i,
			"status":    "ready",
			"sku":       "",
			"type":      customSlots[i]["type"],
			"color":     customSlots[i]["color"],
			"colors":    customSlots[i]["colors"],
			"icon_type": 0,
			"remain":    0,
			"decorder":  0,
			"rfid":      1,
			"source":    2,
		}
		slots[i] = slot
	}

	info := map[string]interface{}{
		"status": "ready",
		"dryer": map[string]interface{}{
			"status":      "stop",
			"target_temp": 0,
			"duration":    0,
			"remain_time": 0,
		},
		"temp":              0,
		"enable_rfid":       1,
		"fan_speed":         7000,
		"feed_assist_count": 0,
		"cont_assist_time":  0.0,
		"slots":             slots,
	}

	// Apply custom slot metadata and mark empty-type slots
	for i, s := range info["slots"].([]interface{}) {
		sm := s.(map[string]interface{})
		if i < len(customSlots) {
			cust := customSlots[i]
			if sku, ok := cust["sku"].(string); ok && strings.TrimSpace(sku) != "" {
				sm["sku"] = sku
			}
			if rfid, ok := cust["rfid"]; ok {
				sm["rfid"] = rfid
			}
			if source, ok := cust["source"]; ok {
				sm["source"] = source
			}
			if iconType, ok := cust["icon_type"]; ok {
				sm["icon_type"] = iconType
			}
		}
		if t, ok := sm["type"].(string); ok && t == "" {
			sm["status"] = "empty"
		}
	}

	return info
}

// DefaultInventorySlot returns a single default inventory slot.
func DefaultInventorySlot() map[string]interface{} {
	return map[string]interface{}{
		"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0,
	}
}

// InitializeInventory returns the inventory loaded from savedVars["ace_inventory"],
// or a default 4-slot empty inventory if not available.
func InitializeInventory(savedVars map[string]interface{}) []map[string]interface{} {
	var savedInventory []interface{}
	if val, ok := savedVars["ace_inventory"]; ok && val != nil {
		if invSlice, ok := val.([]interface{}); ok {
			savedInventory = invSlice
		}
	}

	if savedInventory != nil {
		result := make([]map[string]interface{}, 0, len(savedInventory))
		for _, inv := range savedInventory {
			result = append(result, inv.(map[string]interface{}))
		}
		return result
	}

	return []map[string]interface{}{
		DefaultInventorySlot(),
		DefaultInventorySlot(),
		DefaultInventorySlot(),
		DefaultInventorySlot(),
	}
}

// EnsureDefaultVariables ensures the standard ACE variables exist in the given map,
// setting defaults for any missing keys.
func EnsureDefaultVariables(variables map[string]interface{}) {
	if _, ok := variables["ace_current_index"]; !ok {
		variables["ace_current_index"] = int64(0)
	}
	if _, ok := variables["ace_filament_pos"]; !ok {
		variables["ace_filament_pos"] = "bowden"
	}
	if _, ok := variables["ace_endless_spool_enabled"]; !ok {
		variables["ace_endless_spool_enabled"] = false
	}
	if _, ok := variables["ace_inventory"]; !ok {
		variables["ace_inventory"] = []interface{}{
			DefaultInventorySlot(),
			DefaultInventorySlot(),
			DefaultInventorySlot(),
			DefaultInventorySlot(),
		}
	}
}
