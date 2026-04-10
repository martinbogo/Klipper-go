package probe

import "testing"

func TestEndstopWrapperMultiProbeStateMachine(t *testing.T) {
	core := NewEndstopWrapper(1.5, false)

	core.BeginMultiProbe()
	if core.Multi != MultiFirst {
		t.Fatalf("BeginMultiProbe() state = %q", core.Multi)
	}
	if !core.PrepareForProbe() {
		t.Fatalf("PrepareForProbe() should request lowering on first probe")
	}
	if core.Multi != MultiOn {
		t.Fatalf("PrepareForProbe() state = %q", core.Multi)
	}
	if core.PrepareForProbe() {
		t.Fatalf("PrepareForProbe() should not request lowering again while multi is ON")
	}
	if core.FinishProbe() {
		t.Fatalf("FinishProbe() should not request raising while multi is ON")
	}
	if !core.EndMultiProbe() {
		t.Fatalf("EndMultiProbe() should request raise when stow-on-each-sample is disabled")
	}
	if core.Multi != MultiOff {
		t.Fatalf("EndMultiProbe() state = %q", core.Multi)
	}
}

func TestEndstopWrapperStowOnEachSample(t *testing.T) {
	core := NewEndstopWrapper(2.0, true)

	core.BeginMultiProbe()
	if core.Multi != MultiOff {
		t.Fatalf("BeginMultiProbe() should not change state when stow-on-each-sample is enabled")
	}
	if !core.PrepareForProbe() {
		t.Fatalf("PrepareForProbe() should lower probe when multi is OFF")
	}
	if !core.FinishProbe() {
		t.Fatalf("FinishProbe() should raise probe when multi is OFF")
	}
	if core.EndMultiProbe() {
		t.Fatalf("EndMultiProbe() should not request raise when stow-on-each-sample is enabled")
	}
}

func TestCoordinatesChanged(t *testing.T) {
	if CoordinatesChanged([]float64{1, 2, 3}, []float64{1, 2, 3}) {
		t.Fatalf("CoordinatesChanged() should report false for identical positions")
	}
	if !CoordinatesChanged([]float64{1, 2, 3}, []float64{1, 2, 4}) {
		t.Fatalf("CoordinatesChanged() should report true when coordinates differ")
	}
}
