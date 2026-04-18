package config

import "testing"

func TestValidateIntRange(t *testing.T) {
	if err := ValidateIntRange("max_velocity", "printer", 250, 200, 300); err != nil {
		t.Fatalf("expected valid int range, got %v", err)
	}
	if err := ValidateIntRange("max_velocity", "printer", 150, 200, 300); err == nil {
		t.Fatal("expected minimum violation")
	}
	if err := ValidateIntRange("max_velocity", "printer", 350, 200, 300); err == nil {
		t.Fatal("expected maximum violation")
	}
}

func TestValidateInt64Range(t *testing.T) {
	if err := ValidateInt64Range("buffer", "mcu", 64, 32, 128); err != nil {
		t.Fatalf("expected valid int64 range, got %v", err)
	}
	if err := ValidateInt64Range("buffer", "mcu", 16, 32, 128); err == nil {
		t.Fatal("expected minimum violation")
	}
	if err := ValidateInt64Range("buffer", "mcu", 256, 32, 128); err == nil {
		t.Fatal("expected maximum violation")
	}
}

func TestValidateFloatRange(t *testing.T) {
	if err := ValidateFloatRange("speed", "printer", 5.5, 1.0, 10.0, 0.0, 0.0); err != nil {
		t.Fatalf("expected valid float range, got %v", err)
	}
	if err := ValidateFloatRange("speed", "printer", 0.5, 1.0, 10.0, 0.0, 0.0); err == nil {
		t.Fatal("expected minimum violation")
	}
	if err := ValidateFloatRange("speed", "printer", 10.5, 1.0, 10.0, 0.0, 0.0); err == nil {
		t.Fatal("expected maximum violation")
	}
	if err := ValidateFloatRange("speed", "printer", 1.0, 0.0, 0.0, 1.0, 0.0); err == nil {
		t.Fatal("expected above violation")
	}
	if err := ValidateFloatRange("speed", "printer", 5.0, 0.0, 0.0, 0.0, 5.0); err == nil {
		t.Fatal("expected below violation")
	}
}
