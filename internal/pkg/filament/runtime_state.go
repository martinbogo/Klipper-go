package filament

import (
	"encoding/json"
	"fmt"
	"goklipper/common/utils/sys"
)

const EndlessSpoolModeImmediate = "Immediate switching on runout detection"

// EmptyInventorySlot returns the standard empty inventory slot payload.
func EmptyInventorySlot() map[string]interface{} {
	return DefaultInventorySlot()
}

// ReadyInventorySlot returns a ready inventory slot payload.
func ReadyInventorySlot(color []interface{}, material string, temp int) map[string]interface{} {
	return map[string]interface{}{
		"status":   "ready",
		"color":    color,
		"material": material,
		"temp":     temp,
	}
}

// SaveInventoryState updates the variables map and returns the JSON payload
// expected by SAVE_VARIABLE for ace_inventory.
func SaveInventoryState(variables map[string]interface{}, inventory []map[string]interface{}) (string, error) {
	if variables != nil {
		variables["ace_inventory"] = inventory
	}
	payload, err := json.Marshal(inventory)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// PrepareEndlessSpoolChange determines the next available slot and, when there
// is a current slot, marks it empty and returns the updated ace_inventory SAVE_VARIABLE payload.
func PrepareEndlessSpoolChange(currentTool int, inventory []map[string]interface{}, aceInfo map[string]interface{}, variables map[string]interface{}) (nextTool int, inventorySaveValue string, inventoryChanged bool, err error) {
	nextTool = FindNextAvailableSlot(currentTool, inventory, aceInfo)
	if nextTool == -1 {
		return -1, "", false, nil
	}
	if currentTool < 0 || currentTool >= len(inventory) {
		return nextTool, "", false, nil
	}
	inventory[currentTool] = EmptyInventorySlot()
	inventorySaveValue, err = SaveInventoryState(variables, inventory)
	if err != nil {
		return nextTool, "", true, err
	}
	return nextTool, inventorySaveValue, true, nil
}

// UpdateEndlessSpoolVariable updates the persisted endless spool enabled state
// and returns the literal SAVE_VARIABLE payload value.
func UpdateEndlessSpoolVariable(variables map[string]interface{}, enabled bool) string {
	if variables != nil {
		variables["ace_endless_spool_enabled"] = enabled
	}
	if enabled {
		return "true"
	}
	return "false"
}

// ShouldTriggerEndlessSpoolRunout reports whether the current sensor states
// represent a runout condition.
func ShouldTriggerEndlessSpoolRunout(runoutHelperPresent, endstopTriggered bool) bool {
	return !runoutHelperPresent && !endstopTriggered
}

// BuildEndlessSpoolStatus returns the standard endless spool status payload.
func BuildEndlessSpoolStatus(enabled, runoutDetected, inProgress bool) map[string]interface{} {
	return map[string]interface{}{
		"enabled":         enabled,
		"runout_detected": runoutDetected,
		"in_progress":     inProgress,
	}
}

// BuildEndlessSpoolStatusLines returns the gcode response lines for the
// ACE_ENDLESS_SPOOL_STATUS command.
func BuildEndlessSpoolStatusLines(enabled, savedEnabled, runoutDetected, inProgress bool) []string {
	return []string{
		"ACE: Endless spool status:",
		fmt.Sprintf("  - Currently enabled: %v", enabled),
		fmt.Sprintf("  - Saved enabled: %v", savedEnabled),
		fmt.Sprintf("  - Mode: %s", EndlessSpoolModeImmediate),
		fmt.Sprintf("  - Runout detected: %v", runoutDetected),
		fmt.Sprintf("  - In progress: %v", inProgress),
	}
}

// BuildRunoutSensorStatusLines returns the gcode response lines for the
// ACE_TEST_RUNOUT_SENSOR command.
func BuildRunoutSensorStatusLines(runoutHelperPresent, endstopTriggered, endlessSpoolEnabled bool, currentTool interface{}, runoutDetected bool) []string {
	return []string{
		"ACE: Extruder sensor states:",
		fmt.Sprintf("  - Runout helper filament present: %v", runoutHelperPresent),
		fmt.Sprintf("  - Endstop triggered: %v", endstopTriggered),
		fmt.Sprintf("  - Endless spool enabled: %v", endlessSpoolEnabled),
		fmt.Sprintf("  - Current tool: %v", currentTool),
		fmt.Sprintf("  - Runout detected: %v", runoutDetected),
		fmt.Sprintf("  - Would trigger runout: %v", ShouldTriggerEndlessSpoolRunout(runoutHelperPresent, endstopTriggered)),
	}
}

// BuildACEHubStatus composes the outward-facing ACE filament_hub status payload
// without mutating the live info map.
func BuildACEHubStatus(info map[string]interface{}, fwInfo map[string]interface{}, endlessSpoolEnabled, runoutDetected, inProgress bool) map[string]interface{} {
	status := sys.DeepCopyMap(info)
	if status == nil {
		status = map[string]interface{}{}
	}
	status["id"] = 0
	status["endless_spool"] = BuildEndlessSpoolStatus(endlessSpoolEnabled, runoutDetected, inProgress)
	status["fw_info"] = sys.DeepCopyMap(fwInfo)

	autoRefillVal := 0
	if endlessSpoolEnabled {
		autoRefillVal = 1
	}

	return map[string]interface{}{
		"auto_refill":              autoRefillVal,
		"current_filament":         "",
		"cutter_state":             0,
		"ext_spool":                1,
		"ext_spool_status":         "runout",
		"filament_hubs":            []interface{}{status},
		"filament_present":         0,
		"tracker_detection_length": 0,
		"tracker_filament_present": 0,
	}
}
