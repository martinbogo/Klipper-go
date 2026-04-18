package motion

import "testing"

type fakeLegacyRailStepper struct {
	name              string
	commandedPosition float64
	setupCalls        int
	generateCalls     int
	trapqs            []interface{}
	positions         [][]float64
}

func (self *fakeLegacyRailStepper) Setup_itersolve(alloc_func string, params interface{}) {
	self.setupCalls++
}
func (self *fakeLegacyRailStepper) Generate_steps(flush_time float64) {
	self.generateCalls++
}
func (self *fakeLegacyRailStepper) Set_trapq(tq interface{}) interface{} {
	self.trapqs = append(self.trapqs, tq)
	return nil
}
func (self *fakeLegacyRailStepper) Set_position(coord []float64) {
	copyCoord := append([]float64(nil), coord...)
	self.positions = append(self.positions, copyCoord)
}
func (self *fakeLegacyRailStepper) Get_name(short bool) string {
	if short {
		return self.name
	}
	return self.name
}
func (self *fakeLegacyRailStepper) Get_commanded_position() float64 { return self.commandedPosition }

func TestLegacyRailRuntimeCoordinatesStepperLoops(t *testing.T) {
	runtime := NewLegacyRailRuntime()
	first := &fakeLegacyRailStepper{name: "stepper_x", commandedPosition: 42.5}
	second := &fakeLegacyRailStepper{name: "stepper_y", commandedPosition: 7.0}
	runtime.AddStepper(first)
	runtime.AddStepper(second)

	runtime.SetupItersolve("cartesian_stepper_alloc", uint8('x'))
	runtime.GenerateSteps(1.0)
	runtime.SetTrapq("trapq")
	runtime.SetPosition([]float64{1, 2, 3})

	if first.setupCalls != 1 || second.setupCalls != 1 {
		t.Fatalf("expected setup on both steppers")
	}
	if first.generateCalls != 1 || second.generateCalls != 1 {
		t.Fatalf("expected generate on both steppers")
	}
	if len(first.trapqs) != 1 || len(second.trapqs) != 1 {
		t.Fatalf("expected trapq on both steppers")
	}
	if runtime.GetName(true) != "stepper_x" {
		t.Fatalf("unexpected primary name: %s", runtime.GetName(true))
	}
	if runtime.GetCommandedPosition() != 42.5 {
		t.Fatalf("unexpected commanded position: %v", runtime.GetCommandedPosition())
	}
	if runtime.Get_name(true) != "stepper_x" {
		t.Fatalf("unexpected legacy primary name: %s", runtime.Get_name(true))
	}
	if runtime.Get_commanded_position() != 42.5 {
		t.Fatalf("unexpected legacy commanded position: %v", runtime.Get_commanded_position())
	}
}
