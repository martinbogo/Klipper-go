package kinematics

import (
	"testing"
)

type fakeDualCarriageCommand struct {
	intValues map[string]int
}

func (self *fakeDualCarriageCommand) Get_int(name string, _default interface{}, minval *int, maxval *int) int {
	if v, ok := self.intValues[name]; ok {
		return v
	}
	if _default != nil {
		return _default.(int)
	}
	return 0
}

func buildDualCarriageKin() (*CartesianKinematics, *fakeRail, *fakeRail) {
	toolhead := &fakeToolhead{trapq: "trapq", currentPosition: []float64{10, 20, 30, 40}}
	baseY := &fakeRail{
		name:              "stepper_y",
		steppers:          []Stepper{&fakeStepper{name: "y0"}},
		rangeMin:          0,
		rangeMax:          200,
		homingInfo:        &RailHomingInfo{},
		commandedPosition: 20,
	}
	altY := &fakeRail{
		name:              "dual_y",
		steppers:          []Stepper{&fakeStepper{name: "y1"}},
		rangeMin:          10,
		rangeMax:          210,
		homingInfo:        &RailHomingInfo{},
		commandedPosition: 55,
	}
	kin := NewCartesian(CartesianConfig{
		Toolhead: toolhead,
		Rails: []Rail{
			&fakeRail{name: "stepper_x", steppers: []Stepper{&fakeStepper{name: "x"}}, rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
			baseY,
			&fakeRail{name: "stepper_z", steppers: []Stepper{&fakeStepper{name: "z"}}, rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
		},
		MaxZVelocity: 20,
		MaxZAccel:    20,
		DualCarriage: &DualCarriageConfig{
			Axis:     1,
			AxisName: "y",
			Rails:    []Rail{baseY, altY},
		},
	})
	kin.SetPosition([]float64{0, 0, 0, 0}, []int{0, 1, 2})
	return kin, baseY, altY
}

func TestHandleSetDualCarriageCommandActivatesCarriage0(t *testing.T) {
	kin, baseY, _ := buildDualCarriageKin()
	// First activate carriage 1 so carriage 0 is not current.
	kin.ActivateCarriage(1)
	// Reset history relevant to the command call.
	baseY.trapqHistory = nil

	cmd := &fakeDualCarriageCommand{intValues: map[string]int{"CARRIAGE": 0}}
	if err := HandleSetDualCarriageCommand(kin, cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After activating carriage 0 the base rail should receive the trapq again.
	if len(baseY.trapqHistory) == 0 || baseY.trapqHistory[len(baseY.trapqHistory)-1] != "trapq" {
		t.Fatalf("expected base rail to get trapq after activating carriage 0, got %#v", baseY.trapqHistory)
	}
}

func TestHandleSetDualCarriageCommandActivatesCarriage1(t *testing.T) {
	kin, _, altY := buildDualCarriageKin()
	altY.trapqHistory = nil

	cmd := &fakeDualCarriageCommand{intValues: map[string]int{"CARRIAGE": 1}}
	if err := HandleSetDualCarriageCommand(kin, cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(altY.trapqHistory) == 0 || altY.trapqHistory[len(altY.trapqHistory)-1] != "trapq" {
		t.Fatalf("expected alt rail to get trapq after activating carriage 1, got %#v", altY.trapqHistory)
	}
}

func TestSetDualCarriageHelpIsNonEmpty(t *testing.T) {
	if SetDualCarriageHelp == "" {
		t.Fatal("SetDualCarriageHelp must not be empty")
	}
}
