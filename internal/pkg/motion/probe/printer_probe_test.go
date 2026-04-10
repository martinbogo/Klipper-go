package probe

import "testing"

func TestPrinterProbeStateAndStatus(t *testing.T) {
	core := NewPrinterProbe(5, 6, 1, 2, 3, 4, -1, 2, 0.5, "median", 0.1, 3)

	x, y, z := core.GetOffsets()
	if x != 1 || y != 2 || z != 3 {
		t.Fatalf("GetOffsets() = (%f, %f, %f)", x, y, z)
	}

	core.BeginMultiProbe()
	if !core.MultiProbePending {
		t.Fatalf("BeginMultiProbe() did not mark pending")
	}
	if !core.EndMultiProbe() || core.MultiProbePending {
		t.Fatalf("EndMultiProbe() did not clear pending correctly")
	}

	core.RecordLastState(true)
	core.RecordLastZResult(0.25)
	status := core.Status()
	if status["last_query"] != true || status["last_z_result"] != 0.25 {
		t.Fatalf("Status() = %v", status)
	}
}

func TestPrinterProbeCalibratedOffset(t *testing.T) {
	core := NewPrinterProbe(0, 0, 0, 0, 0, 0, 0, 1, 0, "average", 0, 0)
	core.SetProbeCalibrateZ(5.5)
	if got := core.CalibratedOffset([]float64{0, 0, 1.25}); got != 4.25 {
		t.Fatalf("CalibratedOffset() = %f, want 4.25", got)
	}
}
