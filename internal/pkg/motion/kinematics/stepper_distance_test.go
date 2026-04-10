package kinematics

import (
	"math"
	"testing"
)

type fakeStepperDistanceConfig struct {
	name   string
	values map[string]interface{}
}

func (self *fakeStepperDistanceConfig) Get(option string, default1 interface{}, noteValid bool) interface{} {
	_ = noteValid
	if value, ok := self.values[option]; ok {
		return value
	}
	return default1
}

func (self *fakeStepperDistanceConfig) Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	_ = default1
	_ = minval
	_ = maxval
	_ = above
	_ = below
	_ = noteValid
	return self.values[option].(float64)
}

func (self *fakeStepperDistanceConfig) Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int {
	_ = default1
	_ = minval
	_ = maxval
	_ = noteValid
	return self.values[option].(int)
}

func (self *fakeStepperDistanceConfig) Get_name() string {
	return self.name
}

func TestParseStepperDistanceWithRotationDistance(t *testing.T) {
	config := &fakeStepperDistanceConfig{
		name: "stepper_x",
		values: map[string]interface{}{
			"rotation_distance":       40.0,
			"microsteps":              16,
			"full_steps_per_rotation": 200,
			"gear_ratio":              "2:1,3:2",
		},
	}
	rotationDist, stepsPerRotation := ParseStepperDistance(config, false, true)
	if rotationDist != 40.0 {
		t.Fatalf("expected rotation distance 40, got %v", rotationDist)
	}
	if stepsPerRotation != 9600 {
		t.Fatalf("expected 9600 steps per rotation, got %d", stepsPerRotation)
	}
}

func TestParseStepperDistanceInfersRadiansFromGearRatio(t *testing.T) {
	config := &fakeStepperDistanceConfig{
		name: "stepper_a",
		values: map[string]interface{}{
			"microsteps":              8,
			"full_steps_per_rotation": 200,
			"gear_ratio":              "3:1",
		},
	}
	rotationDist, stepsPerRotation := ParseStepperDistance(config, nil, true)
	if math.Abs(rotationDist-2.0*math.Pi) > 1e-9 {
		t.Fatalf("expected radians rotation distance, got %v", rotationDist)
	}
	if stepsPerRotation != 4800 {
		t.Fatalf("expected 4800 steps per rotation, got %d", stepsPerRotation)
	}
}

func TestParseStepperDistanceSupportsStructuredGearRatio(t *testing.T) {
	config := &fakeStepperDistanceConfig{
		name: "stepper_z",
		values: map[string]interface{}{
			"rotation_distance":       8.0,
			"microsteps":              4,
			"full_steps_per_rotation": 200,
			"gear_ratio": []interface{}{
				[]float64{5, 2},
				[]interface{}{4.0, 1.0},
			},
		},
	}
	rotationDist, stepsPerRotation := ParseStepperDistance(config, false, true)
	if rotationDist != 8.0 {
		t.Fatalf("expected rotation distance 8, got %v", rotationDist)
	}
	if stepsPerRotation != 8000 {
		t.Fatalf("expected 8000 steps per rotation, got %d", stepsPerRotation)
	}
}

func TestParseStepperDistanceRejectsInvalidFullSteps(t *testing.T) {
	config := &fakeStepperDistanceConfig{
		name: "stepper_bad",
		values: map[string]interface{}{
			"rotation_distance":       40.0,
			"microsteps":              16,
			"full_steps_per_rotation": 201,
			"gear_ratio":              "1:1",
		},
	}
	defer func() {
		if recover() == nil {
			t.Fatalf("expected invalid full_steps_per_rotation to panic")
		}
	}()
	ParseStepperDistance(config, false, true)
}
