package config

import "strings"

// BuildAccessTrackingSettings converts a "section:option" → value tracking map
// into a nested map[section]map[option]value suitable for status reporting.
func BuildAccessTrackingSettings(accessTracking map[string]interface{}) map[string]interface{} {
	settings := make(map[string]interface{})
	for k, val := range accessTracking {
		section := strings.Split(k, ":")[0]
		option := strings.Split(k, ":")[1]
		item, ok := settings[section]
		if ok {
			item.(map[string]interface{})[option] = val
		} else {
			item = make(map[string]interface{})
			item.(map[string]interface{})[option] = val
			settings[section] = item
		}
	}
	return settings
}

// BuildDeprecationWarnings converts a "section:option:value" → message
// deprecation map into a slice of warning maps for status reporting.
func BuildDeprecationWarnings(deprecated map[string]interface{}) []interface{} {
	warnings := []interface{}{}
	for k, msg := range deprecated {
		parts := strings.Split(k, ":")
		section := parts[0]
		option := parts[1]
		val := parts[2]
		res := make(map[string]string)
		if val == "" {
			res["type"] = "deprecated_option"
		} else {
			res["type"] = "deprecated_value"
			res["value"] = "value"
		}
		res["message"] = msg.(string)
		res["section"] = section
		res["option"] = option
		warnings = append(warnings, res)
	}
	return warnings
}
