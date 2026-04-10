package addon

import "testing"

type fakeTuningTransform struct {
	position []float64
	moves    [][]float64
}

func (self *fakeTuningTransform) GetPosition() []float64 {
	pos := make([]float64, len(self.position))
	copy(pos, self.position)
	return pos
}

func (self *fakeTuningTransform) Move(newpos []float64, speed float64) {
	pos := make([]float64, len(newpos))
	copy(pos, newpos)
	self.position = pos
	self.moves = append(self.moves, pos)
}

func TestTuningTowerCalcValue(t *testing.T) {
	core := NewTuningTower()
	_, err := core.BeginTest(&fakeTuningTransform{position: []float64{0, 0, 0, 0}}, TuningConfig{
		CommandFmt: "M900 K%.9f",
		Start:      1,
		Factor:     2,
		Band:       4,
		Skip:       1,
	})
	if err != nil {
		t.Fatalf("unexpected begin error: %v", err)
	}
	if got := core.CalcValue(0.5); got != 5 {
		t.Fatalf("unexpected calc value before skip: %v", got)
	}
	if got := core.CalcValue(5); got != 13 {
		t.Fatalf("unexpected calc value with banding: %v", got)
	}
	core.EndTest()

	_, err = core.BeginTest(&fakeTuningTransform{position: []float64{0, 0, 0, 0}}, TuningConfig{
		CommandFmt: "SET X=%.9f",
		Start:      0,
		StepHeight: 2,
		StepDelta:  0.5,
	})
	if err != nil {
		t.Fatalf("unexpected stepped begin error: %v", err)
	}
	if got := core.CalcValue(4.9); got != 1 {
		t.Fatalf("unexpected stepped calc value: %v", got)
	}
}

func TestTuningTowerMoveEmitsCommandAndEndsOnLowerZ(t *testing.T) {
	transform := &fakeTuningTransform{position: []float64{0, 0, 0, 0}}
	core := NewTuningTower()
	_, err := core.BeginTest(transform, TuningConfig{
		CommandFmt: "CMD %.9f",
		Start:      10,
		Factor:     2,
	})
	if err != nil {
		t.Fatalf("unexpected begin error: %v", err)
	}

	command, end := core.Move([]float64{0, 0, 0.2, 0}, 100, 0.2)
	if end {
		t.Fatalf("did not expect test to end on z-only move")
	}
	if command != "" {
		t.Fatalf("did not expect command on z-only move, got %q", command)
	}

	command, end = core.Move([]float64{0, 0, 0.2, 1}, 100, 0.2)
	if end {
		t.Fatalf("did not expect test to end on first upward move")
	}
	if command != "CMD 10.400000000" {
		t.Fatalf("unexpected emitted command: %q", command)
	}
	if len(transform.moves) != 2 {
		t.Fatalf("expected forwarded move, got %d", len(transform.moves))
	}

	command, end = core.Move([]float64{0, 0, 0.4, 1}, 100, 0.4)
	if end {
		t.Fatalf("did not expect z-only move to end the test")
	}
	if command != "" {
		t.Fatalf("did not expect command on second z-only move, got %q", command)
	}

	command, end = core.Move([]float64{0, 0, 0.4, 2}, 100, 0.4)
	if end {
		t.Fatalf("did not expect second upward move to end the test")
	}
	if command != "CMD 10.800000000" {
		t.Fatalf("unexpected second emitted command: %q", command)
	}

	command, end = core.Move([]float64{0, 0, -2.0, 2}, 100, -2.0)
	if end {
		t.Fatalf("did not expect lower-z travel move to end the test")
	}
	if command != "" {
		t.Fatalf("did not expect command on lower-z travel move, got %q", command)
	}

	command, end = core.Move([]float64{0, 0, -2.0, 3}, 100, -2.0)
	if !end {
		t.Fatalf("expected lower-z extrusion move to end the test")
	}
	if command != "" {
		t.Fatalf("did not expect command when ending test, got %q", command)
	}
}