package filament

import "strings"

var aceEmptyColors = []interface{}{[]interface{}{0, 0, 0, 0}}

// SyncACEStatusSlots reconciles live ACE slot status with persisted custom slot
// metadata. It mutates both status and customSlots in place and returns the slot
// payloads that should be persisted back to ams_config.cfg for RFID-backed slots.
func SyncACEStatusSlots(status map[string]interface{}, customSlots []map[string]interface{}) map[int]map[string]interface{} {
	slots, _ := status["slots"].([]interface{})
	if len(slots) == 0 {
		return nil
	}

	persistedSlots := make(map[int]map[string]interface{})
	for i, s := range slots {
		slot, ok := s.(map[string]interface{})
		if !ok || i >= len(customSlots) {
			continue
		}

		customSlot := customSlots[i]
		customType, _ := customSlot["type"].(string)
		customType = strings.TrimSpace(customType)
		customColor, _ := customSlot["color"].([]interface{})
		customColors, hasCustomColors := customSlot["colors"].([]interface{})

		if HasValidACESlotRFID(slot) {
			if typ, ok := slot["type"].(string); ok {
				customSlot["type"] = strings.TrimSpace(typ)
			}
			if color, ok := slot["color"].([]interface{}); ok && len(color) == 3 {
				customSlot["color"] = color
				if colors, ok := slot["colors"].([]interface{}); ok && len(colors) > 0 {
					customSlot["colors"] = colors
				} else if generatedColors := BuildACEColorList(color); generatedColors != nil {
					customSlot["colors"] = generatedColors
					slot["colors"] = generatedColors
				}
			}
			customSlot["sku"] = slot["sku"]
			customSlot["rfid"] = slot["rfid"]
			customSlot["source"] = slot["source"]
			customSlot["icon_type"] = slot["icon_type"]
			persistedSlots[i] = map[string]interface{}{
				"type":      customSlot["type"],
				"color":     customSlot["color"],
				"colors":    customSlot["colors"],
				"sku":       customSlot["sku"],
				"rfid":      customSlot["rfid"],
				"source":    customSlot["source"],
				"icon_type": customSlot["icon_type"],
			}
			continue
		}

		sku, _ := slot["sku"].(string)
		if sku == "" || sku == "custom" {
			slot["status"] = "ready"
			slot["sku"] = ""
			slot["rfid"] = 1
			slot["source"] = 2
			slot["icon_type"] = 0
			slot["remain"] = 0
			slot["decorder"] = 0
			if customType != "" {
				slot["type"] = customType
			}
			if len(customColor) == 3 {
				slot["color"] = customColor
			}
			if hasCustomColors {
				slot["colors"] = customColors
			} else {
				slot["colors"] = aceEmptyColors
			}
			continue
		}

		if customType != "" && customType != "?" {
			slot["type"] = customType
		}
		if len(customColor) == 3 {
			slot["color"] = customColor
		}
		if hasCustomColors {
			slot["colors"] = customColors
		}
	}

	if len(persistedSlots) == 0 {
		return nil
	}
	return persistedSlots
}
