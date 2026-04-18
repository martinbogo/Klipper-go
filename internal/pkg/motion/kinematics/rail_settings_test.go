package kinematics

import (
	"fmt"
	"testing"
)

type fakeRailConfig struct {
	name        string
	floatValues map[string]float64
	boolValues  map[string]bool
	hasOptions  map[string]bool
	boolCalls   []struct {
		option       string
		defaultValue bool
	}
}

func (self *fakeRailConfig) Get_name() string {
	return self.name
}

func (self *fakeRailConfig) HasOption(option string) bool {
	if self.hasOptions == nil {
		return false
	}
	return self.hasOptions[option]
}

func (self *fakeRailConfig) Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	_, _, _, _, _, _ = minval, maxval, above, below, noteValid, default1
	if value, ok := self.floatValues[option]; ok {
		return value
	}
	if value, ok := default1.(float64); ok {
		return value
	}
	return 0
}

func (self *fakeRailConfig) Getboolean(option string, default1 interface{}, noteValid bool) bool {
	_ = noteValid
	if value, ok := self.boolValues[option]; ok {
		return value
	}
	defaultValue, _ := default1.(bool)
	self.boolCalls = append(self.boolCalls, struct {
		option       string
		defaultValue bool
	}{option: option, defaultValue: defaultValue})
	return defaultValue
}

type fakeRailEndstopProvider struct {
	positionEndstop float64
}

func (self *fakeRailEndstopProvider) Get_position_endstop() float64 {
	return self.positionEndstop
}

func TestBuildLegacyRailSettingsUsesEndstopProvider(t *testing.T) {
	config := &fakeRailConfig{
		name: "stepper_z",
		floatValues: map[string]float64{
			"homing_speed": 8,
		},
		boolValues: map[string]bool{},
	}
	settings := BuildLegacyRailSettings(config, &fakeRailEndstopProvider{positionEndstop: 9}, false, (*float64)(nil))
	if settings.PositionEndstop != 9 {
		t.Fatalf("expected provider position_endstop, got %#v", settings)
	}
	if settings.PositionMin != 0 || settings.PositionMax != 9 {
		t.Fatalf("unexpected rail range %#v", settings)
	}
	if !settings.HomingPositiveDir {
		t.Fatalf("expected homing direction to infer positive, got %#v", settings)
	}
	if settings.HomingSpeed != 8 || settings.SecondHomingSpeed != 4 {
		t.Fatalf("unexpected homing speeds %#v", settings)
	}
	if len(config.boolCalls) != 1 || config.boolCalls[0].option != "homing_positive_dir" || !config.boolCalls[0].defaultValue {
		t.Fatalf("expected inferred homing_positive_dir to be fed back to config, got %#v", config.boolCalls)
	}
}

func TestBuildLegacyRailSettingsReadsConfiguredPositionEndstop(t *testing.T) {
	config := &fakeRailConfig{
		name: "stepper_x",
		floatValues: map[string]float64{
			"position_endstop": 2,
			"position_min":     0,
			"position_max":     10,
		},
		boolValues: map[string]bool{},
	}
	settings := BuildLegacyRailSettings(config, struct{}{}, true, (*float64)(nil))
	if settings.PositionEndstop != 2 {
		t.Fatalf("expected config position_endstop, got %#v", settings)
	}
	if settings.PositionMin != 0 || settings.PositionMax != 10 {
		t.Fatalf("unexpected explicit rail range %#v", settings)
	}
	if settings.HomingPositiveDir {
		t.Fatalf("expected homing direction to infer negative near minimum, got %#v", settings)
	}
	if len(config.boolCalls) != 1 || config.boolCalls[0].defaultValue {
		t.Fatalf("expected inferred negative homing_positive_dir callback, got %#v", config.boolCalls)
	}
}

func TestBuildLegacyRailSettingsRespectsExplicitHomingDirection(t *testing.T) {
	config := &fakeRailConfig{
		name: "stepper_x",
		floatValues: map[string]float64{
			"position_endstop": 1,
			"position_min":     0,
			"position_max":     10,
		},
		boolValues: map[string]bool{
			"homing_positive_dir": false,
		},
		hasOptions: map[string]bool{
			"homing_positive_dir": true,
		},
	}

	settings := BuildLegacyRailSettings(config, struct{}{}, true, (*float64)(nil))
	if settings.HomingPositiveDir {
		t.Fatalf("expected explicit homing_positive_dir=false to be preserved")
	}
	if len(config.boolCalls) != 0 {
		t.Fatalf("expected no inferred homing_positive_dir callback when explicitly configured, got %#v", config.boolCalls)
	}
}

func TestBuildLegacyRailSettingsWrapsOutOfRangeError(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected wrapped out-of-range panic")
		}
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("expected error panic, got %T", recovered)
		}
		want := "Position_endstop '11.000000' in section 'stepper_z' must be between position_min and position_max"
		if err.Error() != want {
			t.Fatalf("unexpected error %q", err.Error())
		}
	}()
	config := &fakeRailConfig{
		name: "stepper_z",
		floatValues: map[string]float64{
			"position_endstop": 11,
			"position_min":     0,
			"position_max":     10,
		},
		boolValues: map[string]bool{},
	}
	BuildLegacyRailSettings(config, struct{}{}, true, (*float64)(nil))
}

func TestBuildLegacyRailSettingsWrapsAmbiguousDirectionError(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected wrapped ambiguous-direction panic")
		}
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("expected error panic, got %T", recovered)
		}
		want := fmt.Sprintf("Unable to infer homing_positive_dir in section '%s'", "stepper_y")
		if err.Error() != want {
			t.Fatalf("unexpected error %q", err.Error())
		}
	}()
	config := &fakeRailConfig{
		name: "stepper_y",
		floatValues: map[string]float64{
			"position_endstop": 5,
			"position_min":     0,
			"position_max":     10,
		},
		boolValues: map[string]bool{},
	}
	BuildLegacyRailSettings(config, struct{}{}, true, (*float64)(nil))
}

func TestBuildLegacyRailSettingsWrapsExplicitDirectionMismatchError(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected wrapped explicit-direction mismatch panic")
		}
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("expected error panic, got %T", recovered)
		}
		want := fmt.Sprintf("Invalid homing_positive_dir / Position_endstop in '%s'", "stepper_y")
		if err.Error() != want {
			t.Fatalf("unexpected error %q", err.Error())
		}
	}()
	config := &fakeRailConfig{
		name: "stepper_y",
		floatValues: map[string]float64{
			"position_endstop": 10,
			"position_min":     0,
			"position_max":     10,
		},
		boolValues: map[string]bool{
			"homing_positive_dir": false,
		},
		hasOptions: map[string]bool{
			"homing_positive_dir": true,
		},
	}
	BuildLegacyRailSettings(config, struct{}{}, true, (*float64)(nil))
}
