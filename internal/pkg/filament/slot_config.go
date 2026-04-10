package filament

// MergeFilamentSlotConfig produces a merged slot configuration map by starting from
// existing (may be nil), applying all entries from updates, and enforcing defaults.
// Nil update values delete the corresponding key from the result.
func MergeFilamentSlotConfig(existing, updates map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range existing {
		result[k] = v
	}
	for k, v := range updates {
		if v != nil {
			result[k] = v
		} else {
			delete(result, k)
		}
	}
	if _, ok := result["icon_type"]; !ok {
		result["icon_type"] = 0
	}
	if color, ok := result["color"].([]interface{}); ok {
		if colors, ok := result["colors"].([]interface{}); !ok || len(colors) == 0 {
			if generatedColors := BuildACEColorList(color); generatedColors != nil {
				result["colors"] = generatedColors
			}
		}
	}
	return result
}
