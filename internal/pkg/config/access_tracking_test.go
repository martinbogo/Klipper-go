package config

import "testing"

func TestEnsureAccessTrackingInitializesNilMap(t *testing.T) {
	tracking := EnsureAccessTracking(nil)
	if tracking == nil {
		t.Fatal("expected initialized access-tracking map")
	}
	tracking["printer:max_velocity"] = 250.0
	if got := tracking["printer:max_velocity"]; got != 250.0 {
		t.Fatalf("expected stored value, got %#v", got)
	}
}

func TestNoteAccessNormalizesKeyAndSharesMap(t *testing.T) {
	tracking := map[string]interface{}{}
	updated := NoteAccess(tracking, "Extruder", "Rotation_Distance", 22.5)
	if updated == nil {
		t.Fatal("expected updated tracking map")
	}
	if got := updated["extruder:rotation_distance"]; got != 22.5 {
		t.Fatalf("expected normalized tracked value, got %#v", got)
	}
	if len(tracking) != 1 {
		t.Fatalf("expected original tracking map to be reused, got %#v", tracking)
	}
}

func TestMaybeNoteAccessStoresValueWhenRequested(t *testing.T) {
	tracking := MaybeNoteAccess(nil, "printer", "max_velocity", 250.0, true)
	if got := tracking["printer:max_velocity"]; got != 250.0 {
		t.Fatalf("expected tracked value, got %#v", got)
	}
}

func TestMaybeNoteAccessSkipsDisabledOrNilValues(t *testing.T) {
	tracking := MaybeNoteAccess(nil, "printer", "max_velocity", 250.0, false)
	if len(tracking) != 0 {
		t.Fatalf("expected no tracked values when disabled, got %#v", tracking)
	}
	tracking = MaybeNoteAccess(tracking, "printer", "square_corner_velocity", nil, true)
	if len(tracking) != 0 {
		t.Fatalf("expected nil values to be skipped, got %#v", tracking)
	}
	if tracking == nil {
		t.Fatal("expected initialized tracking map even when nothing was recorded")
	}
}
