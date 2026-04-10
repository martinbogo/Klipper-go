package filament

import (
	"encoding/json"
	"fmt"
	"os"
)

// FilamentInfo holds the display and identity data for a single filament slot.
type FilamentInfo struct {
	Type     string
	Color    []interface{}
	Colors   interface{}
	IconType interface{}
	RFID     interface{}
	SKU      string
	Source   interface{}
}

// ParseFilamentFromConfig reads ams_config.cfg and returns the slot's stored
// FilamentInfo. Defaults are pre-populated so the result is always usable even
// when the config file is absent or the slot is not present.
func ParseFilamentFromConfig(index int, configPath string) FilamentInfo {
	info := FilamentInfo{
		Type:     "",
		Color:    []interface{}{0, 0, 0},
		Colors:   []interface{}{[]interface{}{0, 0, 0, 0}},
		IconType: 0,
		RFID:     1,
		SKU:      "",
		Source:   2,
	}
	amsBytes, err := os.ReadFile(configPath)
	if err != nil {
		return info
	}
	var configData map[string]interface{}
	if err := json.Unmarshal(amsBytes, &configData); err != nil {
		return info
	}
	filaments, ok := configData["filaments"].(map[string]interface{})
	if !ok {
		return info
	}
	fData, ok := filaments[fmt.Sprintf("%d", index)]
	if !ok {
		return info
	}
	fMap, ok := fData.(map[string]interface{})
	if !ok {
		return info
	}
	if typ, ok := fMap["type"].(string); ok {
		info.Type = typ
	}
	if color, ok := fMap["color"].([]interface{}); ok {
		info.Color = color
	}
	if colors, ok := fMap["colors"].([]interface{}); ok {
		info.Colors = colors
	}
	if iconType, ok := fMap["icon_type"]; ok && iconType != nil {
		info.IconType = iconType
	}
	if rfid, ok := fMap["rfid"]; ok && rfid != nil {
		info.RFID = rfid
	}
	if sku, ok := fMap["sku"].(string); ok {
		info.SKU = sku
	}
	if source, ok := fMap["source"]; ok && source != nil {
		info.Source = source
	}
	return info
}

// MergeSlotStatusIntoFilamentInfo updates a FilamentInfo with fields from a
// hardware slot status map (type, color, colors, icon_type, rfid, sku, source).
func MergeSlotStatusIntoFilamentInfo(info *FilamentInfo, slotMap map[string]interface{}) {
	if typ, ok := slotMap["type"].(string); ok {
		info.Type = typ
	}
	if color, ok := slotMap["color"].([]interface{}); ok {
		info.Color = color
	}
	if colors, ok := slotMap["colors"].([]interface{}); ok {
		info.Colors = colors
	}
	if iconType, ok := slotMap["icon_type"]; ok && iconType != nil {
		info.IconType = iconType
	}
	if rfid, ok := slotMap["rfid"]; ok && rfid != nil {
		info.RFID = rfid
	}
	if sku, ok := slotMap["sku"].(string); ok {
		info.SKU = sku
	}
	if source, ok := slotMap["source"]; ok && source != nil {
		info.Source = source
	}
}

// BuildFilamentInfoResponse constructs the JSON-serialisable response map for a
// filament_hub/filament_info request.
func BuildFilamentInfoResponse(index int, info FilamentInfo) map[string]interface{} {
	return map[string]interface{}{
		"brand":         "",
		"color":         info.Color,
		"colors":        info.Colors,
		"diameter":      0,
		"extruder_temp": map[string]interface{}{"max": 0, "min": 0},
		"hotbed_temp":   map[string]interface{}{"max": 0, "min": 0},
		"icon_type":     info.IconType,
		"index":         index,
		"remain":        0,
		"rfid":          info.RFID,
		"sku":           info.SKU,
		"source":        info.Source,
		"type":          info.Type,
	}
}

// ReadAutoRefillConfig reads ams_config.cfg and returns 1 if auto_refill is true,
// 0 otherwise (also on read/parse errors).
func ReadAutoRefillConfig(configPath string) int {
	amsBytes, err := os.ReadFile(configPath)
	if err != nil {
		return 0
	}
	var configData map[string]interface{}
	if err := json.Unmarshal(amsBytes, &configData); err != nil {
		return 0
	}
	if ar, ok := configData["auto_refill"].(bool); ok && ar {
		return 1
	}
	return 0
}

// ParseDryingParams extracts duration (seconds) and temperature (°C) from a
// filament_hub/start_drying request params map. Defaults are 240 s / 50 °C.
func ParseDryingParams(params map[string]interface{}) (duration int, temp int) {
	duration = 240
	temp = 50
	if params == nil {
		return
	}
	if d, ok := params["duration"].(float64); ok {
		duration = int(d)
	} else if d, ok := params["time"].(float64); ok {
		duration = int(d)
	}
	if t, ok := params["target_temp"].(float64); ok {
		temp = int(t)
	} else if t, ok := params["temp"].(float64); ok {
		temp = int(t)
	}
	return
}

// UpdateAutoRefillInConfig reads the AMS config file, sets the auto_refill
// key to the given value, and writes it back.
func UpdateAutoRefillInConfig(configPath string, value interface{}) {
	amsBytes, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var configData map[string]interface{}
	if err := json.Unmarshal(amsBytes, &configData); err != nil {
		return
	}
	configData["auto_refill"] = value
	if b, err := json.Marshal(configData); err == nil {
		os.WriteFile(configPath, b, 0644)
	}
}

// FormatConfigValue converts a bool, float64, or int value to the string
// representation expected by SAVE_VARIABLE. Returns ("", false) for
// unsupported types.
func FormatConfigValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case bool:
		if val {
			return "True", true
		}
		return "False", true
	case float64:
		return fmt.Sprintf("%f", val), true
	case int:
		return fmt.Sprintf("%d", val), true
	default:
		return "", false
	}
}

// IsAutoRefillEnabled determines whether a value represents "enabled":
// true for bool(true) or float64 > 0.
func IsAutoRefillEnabled(v interface{}) bool {
	if val, ok := v.(bool); ok {
		return val
	}
	if fval, ok := v.(float64); ok {
		return fval > 0
	}
	return false
}
