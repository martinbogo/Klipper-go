package print

import "time"

const LeviQ3PersistentStateVariable = "leviq3_state"

type LeviQ3PersistedStateRecord struct {
	CurrentZOffset        float64          `json:"current_z_offset"`
	LastHotbedTemp        float64          `json:"last_hotbed_temp,omitempty"`
	LastCompensationTemp  float64          `json:"last_compensation_temp,omitempty"`
	LastTempCompensation  float64          `json:"last_temp_compensation,omitempty"`
	LastAutoZOffsetRaw    float64          `json:"last_auto_zoffset_raw,omitempty"`
	LastAutoZOffset       float64          `json:"last_auto_zoffset,omitempty"`
	LastAutoZOffsetSample []float64        `json:"last_auto_zoffset_samples,omitempty"`
	LastAppliedOffsets    *XYZ             `json:"last_applied_offsets,omitempty"`
	SavedMesh             *BedMeshSnapshot `json:"saved_mesh,omitempty"`
	LastCancelReason      string           `json:"last_cancel_reason,omitempty"`
	LastRun               time.Time        `json:"last_run,omitempty"`
}

func NewLeviQ3PersistedStateRecord(currentZOffset float64, state LeviQ3PersistentState) LeviQ3PersistedStateRecord {
	persisted := LeviQ3PersistedStateRecord{
		CurrentZOffset:        currentZOffset,
		LastHotbedTemp:        state.LastHotbedTemp,
		LastCompensationTemp:  state.LastCompensationTemp,
		LastTempCompensation:  state.LastTempCompensation,
		LastAutoZOffsetRaw:    state.LastAutoZOffsetRaw,
		LastAutoZOffset:       state.LastAutoZOffset,
		LastAutoZOffsetSample: append([]float64(nil), state.LastAutoZOffsetSamples...),
		LastCancelReason:      state.LastCancelReason,
		LastRun:               state.LastRun,
	}
	if state.LastAppliedOffsets != (XYZ{}) {
		offsets := state.LastAppliedOffsets
		persisted.LastAppliedOffsets = &offsets
	}
	if state.SavedMesh != nil {
		persisted.SavedMesh = state.SavedMesh.Snapshot()
	}
	return persisted
}

func (state LeviQ3PersistedStateRecord) PersistentState() LeviQ3PersistentState {
	helperState := LeviQ3PersistentState{
		SavedZOffset:           state.CurrentZOffset,
		LastHotbedTemp:         state.LastHotbedTemp,
		LastCompensationTemp:   state.LastCompensationTemp,
		LastTempCompensation:   state.LastTempCompensation,
		LastAutoZOffsetRaw:     state.LastAutoZOffsetRaw,
		LastAutoZOffset:        state.LastAutoZOffset,
		LastAutoZOffsetSamples: append([]float64(nil), state.LastAutoZOffsetSample...),
		LastCancelReason:       state.LastCancelReason,
		LastRun:                state.LastRun,
	}
	if state.LastAppliedOffsets != nil {
		helperState.LastAppliedOffsets = *state.LastAppliedOffsets
	}
	if state.SavedMesh != nil {
		helperState.SavedMesh = NewBedMeshFromSnapshot(state.SavedMesh)
	}
	return helperState
}
