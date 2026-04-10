package filament

import "encoding/json"

// UpdateACESlotInfo applies filament type/color assignment to a slot map and
// deep-updates the corresponding info map entry. It returns the updated info
// map and a persistedSlot map ready for storage.
//
// slot is modified in-place (it is a reference to one element of custom_slots).
func UpdateACESlotInfo(
	slot map[string]interface{},
	info map[string]interface{},
	index int,
	typ string,
	color []interface{},
) (updatedInfo map[string]interface{}, persistedSlot map[string]interface{}) {
	existingType, _ := slot["type"].(string)
	typ = NormalizeACEFilamentType(typ, existingType)
	slot["type"] = typ
	slot["color"] = color
	slot["sku"] = ""
	slot["rfid"] = 1
	slot["source"] = 2
	slot["icon_type"] = 0

	var colorsList []interface{}
	if len(color) == 3 {
		colorsList = BuildACEColorList(color)
		slot["colors"] = colorsList
	}

	updatedInfo = info
	if info != nil {
		encoded, _ := json.Marshal(info)
		var newInfo map[string]interface{}
		json.Unmarshal(encoded, &newInfo) //nolint:errcheck
		if slots, ok := newInfo["slots"].([]interface{}); ok && index < len(slots) {
			if slotMap, ok2 := slots[index].(map[string]interface{}); ok2 {
				slotMap["type"] = typ
				slotMap["status"] = "ready"
				slotMap["sku"] = ""
				slotMap["rfid"] = 1
				slotMap["source"] = 2
				slotMap["icon_type"] = 0
				slotMap["remain"] = 0
				slotMap["decorder"] = 0
				slotMap["color"] = color
				if colorsList != nil {
					slotMap["colors"] = colorsList
				} else {
					slotMap["colors"] = []interface{}{[]interface{}{0, 0, 0, 0}}
				}
			}
		}
		updatedInfo = newInfo
	}

	persistedSlot = map[string]interface{}{
		"type":      typ,
		"color":     color,
		"colors":    colorsList,
		"sku":       "",
		"rfid":      1,
		"source":    2,
		"icon_type": 0,
	}
	return updatedInfo, persistedSlot
}
