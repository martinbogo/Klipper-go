package probe

import (
	"reflect"
	"strings"
	"testing"
)

type fakeEndstopRuntime struct {
	position          []float64
	activateNextPos   []float64
	deactivateNextPos []float64
	activateCount     int
	deactivateCount   int
}

type fakeEndstopIdentifyRuntime struct {
	steppers   []interface{}
	activeAxes map[interface{}]rune
	added      []interface{}
}

func (self *fakeEndstopRuntime) ToolheadPosition() []float64 {
	return append([]float64{}, self.position...)
}

func (self *fakeEndstopRuntime) RunActivateGCode() {
	self.activateCount++
	if self.activateNextPos != nil {
		self.position = append([]float64{}, self.activateNextPos...)
	}
}

func (self *fakeEndstopRuntime) RunDeactivateGCode() {
	self.deactivateCount++
	if self.deactivateNextPos != nil {
		self.position = append([]float64{}, self.deactivateNextPos...)
	}
}

func (self *fakeEndstopIdentifyRuntime) KinematicsSteppers() []interface{} {
	return append([]interface{}{}, self.steppers...)
}

func (self *fakeEndstopIdentifyRuntime) StepperIsActiveAxis(stepper interface{}, axis rune) bool {
	return self.activeAxes[stepper] == axis
}

func (self *fakeEndstopIdentifyRuntime) AddStepper(stepper interface{}) {
	self.added = append(self.added, stepper)
}

func expectPanicMessage(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic %q", want)
		}
		got := recovered.(string)
		if got != want {
			t.Fatalf("panic = %q, want %q", got, want)
		}
	}()
	fn()
}

func TestEndstopWrapperRaiseProbeValidatesNoMovement(t *testing.T) {
	core := NewEndstopWrapper(1.5, false)
	runtime := &fakeEndstopRuntime{position: []float64{1, 2, 3}}

	core.RaiseProbe(runtime)

	if runtime.deactivateCount != 1 || runtime.activateCount != 0 {
		t.Fatalf("script counts activate/deactivate = %d/%d, want 0/1", runtime.activateCount, runtime.deactivateCount)
	}

	runtime = &fakeEndstopRuntime{
		position:          []float64{1, 2, 3},
		deactivateNextPos: []float64{1, 2, 4},
	}
	expectPanicMessage(t, "project.Toolhead moved during probe activate_gcode script", func() {
		core.RaiseProbe(runtime)
	})
}

func TestEndstopWrapperLowerProbeValidatesNoMovement(t *testing.T) {
	core := NewEndstopWrapper(1.5, false)
	runtime := &fakeEndstopRuntime{position: []float64{1, 2, 3}}

	core.LowerProbe(runtime)

	if runtime.activateCount != 1 || runtime.deactivateCount != 0 {
		t.Fatalf("script counts activate/deactivate = %d/%d, want 1/0", runtime.activateCount, runtime.deactivateCount)
	}

	runtime = &fakeEndstopRuntime{
		position:        []float64{1, 2, 3},
		activateNextPos: []float64{1, 2, 4},
	}
	expectPanicMessage(t, "project.Toolhead moved during probe deactivate_gcode script", func() {
		core.LowerProbe(runtime)
	})
}

func TestEndstopWrapperRuntimeHandlersFollowStateMachine(t *testing.T) {
	core := NewEndstopWrapper(1.5, false)
	runtime := &fakeEndstopRuntime{position: []float64{1, 2, 3}}

	core.BeginMultiProbe()
	core.HandleProbePrepare(runtime)
	if runtime.activateCount != 1 {
		t.Fatalf("activateCount after first prepare = %d, want 1", runtime.activateCount)
	}
	if core.Multi != MultiOn {
		t.Fatalf("Multi after prepare = %q, want %q", core.Multi, MultiOn)
	}
	core.HandleProbePrepare(runtime)
	if runtime.activateCount != 1 {
		t.Fatalf("activateCount after second prepare = %d, want 1", runtime.activateCount)
	}
	core.HandleProbeFinish(runtime)
	if runtime.deactivateCount != 0 {
		t.Fatalf("deactivateCount after finish while multi on = %d, want 0", runtime.deactivateCount)
	}
	core.HandleMultiProbeEnd(runtime)
	if runtime.deactivateCount != 1 {
		t.Fatalf("deactivateCount after multi end = %d, want 1", runtime.deactivateCount)
	}
	if core.Multi != MultiOff {
		t.Fatalf("Multi after multi end = %q, want %q", core.Multi, MultiOff)
	}
}

func TestEndstopWrapperFinishRaisesWhenStowingEachSample(t *testing.T) {
	core := NewEndstopWrapper(2.0, true)
	runtime := &fakeEndstopRuntime{position: []float64{1, 2, 3}}

	core.HandleProbePrepare(runtime)
	if runtime.activateCount != 1 {
		t.Fatalf("activateCount after prepare = %d, want 1", runtime.activateCount)
	}
	core.HandleProbeFinish(runtime)
	if runtime.deactivateCount != 1 {
		t.Fatalf("deactivateCount after finish = %d, want 1", runtime.deactivateCount)
	}
	core.HandleMultiProbeEnd(runtime)
	if runtime.deactivateCount != 1 {
		t.Fatalf("deactivateCount after multi end = %d, want 1", runtime.deactivateCount)
	}
	if !strings.EqualFold(core.Multi, MultiOff) {
		t.Fatalf("Multi after multi end = %q, want %q", core.Multi, MultiOff)
	}
}

func TestEndstopWrapperHandleMCUIdentifyAddsOnlyZSteppers(t *testing.T) {
	core := NewEndstopWrapper(1.5, false)
	runtime := &fakeEndstopIdentifyRuntime{
		steppers: []interface{}{"x", "z0", "z1"},
		activeAxes: map[interface{}]rune{
			"x":  'x',
			"z0": 'z',
			"z1": 'z',
		},
	}

	core.HandleMCUIdentify(runtime)

	if got, want := runtime.added, []interface{}{"z0", "z1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("added = %v, want %v", got, want)
	}
}

func TestValidateVirtualEndstopPin(t *testing.T) {
	ValidateVirtualEndstopPin("endstop", "z_virtual_endstop", false, false)

	expectPanicMessage(t, "Probe virtual endstop only useful as endstop pin", func() {
		ValidateVirtualEndstopPin("adc", "z_virtual_endstop", false, false)
	})
	expectPanicMessage(t, "Can not pullup/invert probe virtual endstop", func() {
		ValidateVirtualEndstopPin("endstop", "z_virtual_endstop", true, false)
	})
}
