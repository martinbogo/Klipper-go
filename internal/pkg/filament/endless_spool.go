package filament

// FindNextAvailableSlot returns the index (0–3) of the next slot with
// filament "ready" in both the local inventory and the ACE hub info map,
// starting the search from currentSlot+1 and wrapping around.
// Returns -1 if no available slot is found.
func FindNextAvailableSlot(currentSlot int, inventory []map[string]interface{}, aceInfo map[string]interface{}) int {
	slots, _ := aceInfo["slots"].([]interface{})
	for i := 0; i < 4; i++ {
		next := (currentSlot + 1 + i) % 4
		if next == currentSlot {
			continue
		}
		if next >= len(inventory) || inventory[next]["status"] != "ready" {
			continue
		}
		if slots == nil || next >= len(slots) {
			continue
		}
		slotInfo, ok := slots[next].(map[string]interface{})
		if !ok || slotInfo["status"] != "ready" {
			continue
		}
		return next
	}
	return -1
}
