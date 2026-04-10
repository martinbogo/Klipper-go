package print

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestLeviQ3PersistedStateRecordJSONRoundTripPreservesOffsetsAndMesh(t *testing.T) {
	mesh := NewBedMesh(XY{X: 15, Y: 15}, XY{X: 45, Y: 45}, XYCount{X: 2, Y: 2}, "project-json")
	mesh.Set(0, 0, 0.11)
	mesh.Set(1, 0, 0.12)
	mesh.Set(0, 1, 0.13)
	mesh.Set(1, 1, 0.14)

	state := LeviQ3PersistentState{
		SavedZOffset:           0.27,
		LastHotbedTemp:         60,
		LastCompensationTemp:   60,
		LastTempCompensation:   0.02,
		LastAutoZOffsetRaw:     0.04,
		LastAutoZOffset:        0.31,
		LastAutoZOffsetSamples: []float64{0.04, 0.05},
		LastAppliedOffsets:     XYZ{X: 1.5, Y: 2.5, Z: 0.27},
		LastCancelReason:       "cancelled for test",
		LastRun:                time.Unix(1712345678, 0).UTC(),
		SavedMesh:              mesh,
	}

	persisted := NewLeviQ3PersistedStateRecord(0.27, state)
	payload, err := json.Marshal(persisted)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded LeviQ3PersistedStateRecord
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	restored := decoded.PersistentState()
	if restored.LastAppliedOffsets != state.LastAppliedOffsets {
		t.Fatalf("LastAppliedOffsets = %#v, want %#v", restored.LastAppliedOffsets, state.LastAppliedOffsets)
	}
	if restored.SavedMesh == nil {
		t.Fatal("expected saved mesh to round-trip")
	}
	if !reflect.DeepEqual(restored.SavedMesh.CloneMatrix(), state.SavedMesh.CloneMatrix()) {
		t.Fatalf("saved mesh matrix = %#v, want %#v", restored.SavedMesh.CloneMatrix(), state.SavedMesh.CloneMatrix())
	}
	if restored.LastCancelReason != state.LastCancelReason {
		t.Fatalf("LastCancelReason = %q, want %q", restored.LastCancelReason, state.LastCancelReason)
	}
	if !restored.LastRun.Equal(state.LastRun) {
		t.Fatalf("LastRun = %v, want %v", restored.LastRun, state.LastRun)
	}
}