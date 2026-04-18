package print

import (
	"strings"
	"testing"
)

func TestPersistentStateSaveVariableCommandBuildsSaveVariableScript(t *testing.T) {
	helper, err := NewLeviQ3Helper(MapConfigSource{}, nil, NullStatus{})
	if err != nil {
		t.Fatalf("NewLeviQ3Helper() error = %v", err)
	}
	helper.RestorePersistentState(LeviQ3PersistentState{SavedZOffset: 0.61})

	command, err := helper.PersistentStateSaveVariableCommand(LeviQ3PersistentStateVariable)
	if err != nil {
		t.Fatalf("PersistentStateSaveVariableCommand() error = %v", err)
	}
	if !strings.HasPrefix(command, "SAVE_VARIABLE VARIABLE=leviq3_state VALUE={") {
		t.Fatalf("command = %q, want SAVE_VARIABLE prefix with JSON payload", command)
	}
}

func TestRestorePersistentStateVariableLoadsConfiguredValue(t *testing.T) {
	helper, err := NewLeviQ3Helper(MapConfigSource{}, nil, NullStatus{})
	if err != nil {
		t.Fatalf("NewLeviQ3Helper() error = %v", err)
	}
	helper.RestorePersistentState(LeviQ3PersistentState{SavedZOffset: 0.61})
	encoded, err := helper.PersistentStateSaveVariableValue()
	if err != nil {
		t.Fatalf("PersistentStateSaveVariableValue() error = %v", err)
	}
	restored, err := NewLeviQ3Helper(MapConfigSource{}, nil, NullStatus{})
	if err != nil {
		t.Fatalf("NewLeviQ3Helper() error = %v", err)
	}
	if err := restored.RestorePersistentStateVariable(map[string]interface{}{
		LeviQ3PersistentStateVariable: encoded,
	}, LeviQ3PersistentStateVariable); err != nil {
		t.Fatalf("RestorePersistentStateVariable() error = %v", err)
	}
	if got := restored.CurrentZOffset(); got != helper.CurrentZOffset() {
		t.Fatalf("CurrentZOffset() = %v, want %v", got, helper.CurrentZOffset())
	}
}

func TestRestorePersistentStateVariableRejectsEmptyName(t *testing.T) {
	helper, err := NewLeviQ3Helper(MapConfigSource{}, nil, NullStatus{})
	if err != nil {
		t.Fatalf("NewLeviQ3Helper() error = %v", err)
	}
	if err := helper.RestorePersistentStateVariable(nil, "   "); err == nil {
		t.Fatal("expected empty variable name error")
	}
}
