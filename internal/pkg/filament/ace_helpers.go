package filament

import (
	"fmt"
	"math"
	"strings"
)

// ParseRGB parses a comma-separated "r,g,b" string into a float64 slice.
func ParseRGB(s string) []interface{} {
	var r, g, b float64
	fmt.Sscanf(s, "%f,%f,%f", &r, &g, &b)
	return []interface{}{r, g, b}
}

// NormalizeACEFilamentType returns requestedType trimmed, falling back to
// fallbackType when requestedType is empty or "?".
func NormalizeACEFilamentType(requestedType, fallbackType string) string {
	trimmedType := strings.TrimSpace(requestedType)
	if trimmedType == "" || trimmedType == "?" {
		fallbackType = strings.TrimSpace(fallbackType)
		if fallbackType != "" {
			return fallbackType
		}
	}
	return trimmedType
}

// HasValidACESlotRFID reports whether the given slot map has a non-empty,
// non-"custom" SKU field.
func HasValidACESlotRFID(slot map[string]interface{}) bool {
	sku, _ := slot["sku"].(string)
	sku = strings.TrimSpace(sku)
	return sku != "" && sku != "custom"
}

// BuildACEColorList converts a 3-element [r, g, b] slice into the ACE
// colour-list format: [[r, g, b, 255]].  Returns nil if color is not length 3.
func BuildACEColorList(color []interface{}) []interface{} {
	if len(color) != 3 {
		return nil
	}
	return []interface{}{
		[]interface{}{color[0], color[1], color[2], 255},
	}
}

// CalcReconnectTimeout returns the delay (seconds) before the nth reconnection attempt.
func CalcReconnectTimeout(attempt int) float64 {
	return 0.8*float64(attempt) + math.Cos(float64(attempt))*0.5
}

// ParseColorFromMap extracts R, G, B float64 values from a color parameter map
// (keys "R", "G", "B") and returns them as an []interface{}{int, int, int} slice.
func ParseColorFromMap(colorMap map[string]interface{}) []interface{} {
	var r, g, b float64
	if v, ok := colorMap["R"].(float64); ok {
		r = v
	}
	if v, ok := colorMap["G"].(float64); ok {
		g = v
	}
	if v, ok := colorMap["B"].(float64); ok {
		b = v
	}
	return []interface{}{int(r), int(g), int(b)}
}
