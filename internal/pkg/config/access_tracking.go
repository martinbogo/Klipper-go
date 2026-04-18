package config

import "strings"

// EnsureAccessTracking normalizes nil tracking maps to an initialized map.
func EnsureAccessTracking(accessTracking map[string]interface{}) map[string]interface{} {
	if accessTracking == nil {
		return map[string]interface{}{}
	}
	return accessTracking
}

// AccessTrackingKey normalizes a tracked setting key to section:option.
func AccessTrackingKey(section, option string) string {
	return strings.ToLower(section) + ":" + strings.ToLower(option)
}

// NoteAccess records a tracked value and returns the active tracking map.
func NoteAccess(accessTracking map[string]interface{}, section, option string, value interface{}) map[string]interface{} {
	tracking := EnsureAccessTracking(accessTracking)
	tracking[AccessTrackingKey(section, option)] = value
	return tracking
}

// MaybeNoteAccess records a tracked value only when noteValid is true and the
// value is present, while still normalizing nil maps for callers that want a
// consistent tracking container.
func MaybeNoteAccess(accessTracking map[string]interface{}, section, option string, value interface{}, noteValid bool) map[string]interface{} {
	tracking := EnsureAccessTracking(accessTracking)
	if !noteValid || value == nil {
		return tracking
	}
	tracking[AccessTrackingKey(section, option)] = value
	return tracking
}
