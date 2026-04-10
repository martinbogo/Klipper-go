package addon

import (
	"path/filepath"
	"testing"
)

func TestParsePythonLiteral(t *testing.T) {
	val, err := ParsePythonLiteral("[1, 2, True, None, 'x']")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	list, ok := val.([]interface{})
	if !ok {
		t.Fatalf("expected slice result, got %#v", val)
	}
	if len(list) != 5 {
		t.Fatalf("unexpected parsed list length: %#v", list)
	}
	if list[2].(bool) != true || list[3] != nil || list[4].(string) != "x" {
		t.Fatalf("unexpected parsed values: %#v", list)
	}
}

func TestSaveVariablesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "vars.cfg")
	store := NewSaveVariables(filename)

	if err := store.EnsureFile(); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	if err := store.LoadVariables(); err != nil {
		t.Fatalf("failed to load initial file: %v", err)
	}

	if err := store.SaveVariable("answer", "42"); err != nil {
		t.Fatalf("failed to save int value: %v", err)
	}
	if err := store.SaveVariable("name", `"klipper"`); err != nil {
		t.Fatalf("failed to save string value: %v", err)
	}
	if err := store.SaveVariable("flags", "[True, False, None]"); err != nil {
		t.Fatalf("failed to save composite value: %v", err)
	}

	store = NewSaveVariables(filename)
	if err := store.LoadVariables(); err != nil {
		t.Fatalf("failed to reload file: %v", err)
	}

	vars := store.Variables()
	if vars["answer"].(int64) != 42 {
		t.Fatalf("unexpected int value: %#v", vars["answer"])
	}
	if vars["name"].(string) != "klipper" {
		t.Fatalf("unexpected string value: %#v", vars["name"])
	}
	flags, ok := vars["flags"].([]interface{})
	if !ok || len(flags) != 3 {
		t.Fatalf("unexpected composite value: %#v", vars["flags"])
	}
	if flags[0].(bool) != true || flags[1].(bool) != false || flags[2] != nil {
		t.Fatalf("unexpected composite elements: %#v", flags)
	}
}