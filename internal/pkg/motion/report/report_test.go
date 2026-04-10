package report

import (
	"reflect"
	"strings"
	"testing"
)

type fakeStepperSource struct {
	batches       [][]StepQueueEntry
	callIndex     int
	name          string
	mcuName       string
	clockToTime   func(int64) float64
	startPosition float64
	stepDistance  float64
}

func (self *fakeStepperSource) Name() string    { return self.name }
func (self *fakeStepperSource) MCUName() string { return self.mcuName }
func (self *fakeStepperSource) DumpSteps(count int, startClock uint64, endClock uint64) ([]StepQueueEntry, int) {
	_, _, _ = count, startClock, endClock
	if self.callIndex >= len(self.batches) {
		return nil, 0
	}
	batch := self.batches[self.callIndex]
	self.callIndex++
	return append([]StepQueueEntry(nil), batch...), len(batch)
}
func (self *fakeStepperSource) ClockToPrintTime(clock int64) float64 { return self.clockToTime(clock) }
func (self *fakeStepperSource) MCUToCommandedPosition(mcuPos int) float64 {
	_ = mcuPos
	return self.startPosition
}
func (self *fakeStepperSource) StepDistance() float64 { return self.stepDistance }

type fakeTrapQSource struct {
	batches   [][]TrapQMove
	callIndex int
}

func (self *fakeTrapQSource) ExtractMoves(limit int, startTime float64, endTime float64) ([]TrapQMove, int) {
	_, _, _ = limit, startTime, endTime
	if self.callIndex >= len(self.batches) {
		return nil, 0
	}
	batch := self.batches[self.callIndex]
	self.callIndex++
	return append([]TrapQMove(nil), batch...), len(batch)
}

func TestStepperDumpStepQueueFlattensBatchesInChronologicalOrder(t *testing.T) {
	source := &fakeStepperSource{
		batches: [][]StepQueueEntry{
			{{FirstClock: 90, LastClock: 95, StartPosition: 9}, {FirstClock: 70, LastClock: 75, StartPosition: 7}},
			{{FirstClock: 50, LastClock: 55, StartPosition: 5}, {FirstClock: 30, LastClock: 35, StartPosition: 3}},
		},
		name:         "stepper_x",
		mcuName:      "mcu",
		clockToTime:  func(clock int64) float64 { return float64(clock) / 1000.0 },
		stepDistance: 0.01,
	}
	dump := NewStepperDump(source)
	got := dump.StepQueue(0, uint64(1)<<63)
	want := []StepQueueEntry{{FirstClock: 30, LastClock: 35, StartPosition: 3}, {FirstClock: 50, LastClock: 55, StartPosition: 5}, {FirstClock: 70, LastClock: 75, StartPosition: 7}, {FirstClock: 90, LastClock: 95, StartPosition: 9}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StepQueue() = %#v, want %#v", got, want)
	}
}

func TestStepperDumpAPIUpdateIncludesClockAndStepMetadata(t *testing.T) {
	source := &fakeStepperSource{
		batches: [][]StepQueueEntry{{
			{FirstClock: 120, LastClock: 130, StartPosition: 45, Interval: 6, StepCount: 7, Add: 8},
			{FirstClock: 100, LastClock: 110, StartPosition: 42, Interval: 3, StepCount: 4, Add: 5},
		}},
		name:          "stepper_y",
		mcuName:       "mcu",
		clockToTime:   func(clock int64) float64 { return float64(clock) / 10.0 },
		startPosition: 12.34,
		stepDistance:  -0.02,
	}
	dump := NewStepperDump(source)
	msg := dump.APIUpdate()
	if got := msg["start_position"].(float64); got != 12.34 {
		t.Fatalf("start_position = %v, want 12.34", got)
	}
	if got := msg["step_distance"].(float64); got != -0.02 {
		t.Fatalf("step_distance = %v, want -0.02", got)
	}
	if got := msg["first_step_time"].(float64); got != 10.0 {
		t.Fatalf("first_step_time = %v, want 10", got)
	}
	if got := msg["last_step_time"].(float64); got != 13.0 {
		t.Fatalf("last_step_time = %v, want 13", got)
	}
	if log := dump.LogMessage([]StepQueueEntry{{FirstClock: 100, StartPosition: 42}}); !strings.Contains(log, "queue_step 0") {
		t.Fatalf("expected log output to mention queue_step 0, got %q", log)
	}
}

func TestTrapQDumpMovesAndPositionAt(t *testing.T) {
	source := &fakeTrapQSource{
		batches: [][]TrapQMove{
			{{PrintTime: 3.0, MoveTime: 1.0, StartVelocity: 4, Acceleration: 2, StartPosition: [3]float64{3, 0, 0}, Direction: [3]float64{1, 0, 0}}, {PrintTime: 2.0, MoveTime: 1.0, StartVelocity: 3, Acceleration: 0, StartPosition: [3]float64{2, 0, 0}, Direction: [3]float64{1, 0, 0}}},
			{{PrintTime: 1.0, MoveTime: 1.0, StartVelocity: 2, Acceleration: 0, StartPosition: [3]float64{1, 0, 0}, Direction: [3]float64{1, 0, 0}}, {PrintTime: 0.0, MoveTime: 1.0, StartVelocity: 1, Acceleration: 0, StartPosition: [3]float64{0, 0, 0}, Direction: [3]float64{1, 0, 0}}},
		},
	}
	dump := NewTrapQDump("toolhead", source)
	moves := dump.Moves(0.0, NeverTime)
	if got, want := len(moves), 4; got != want {
		t.Fatalf("len(Moves()) = %d, want %d", got, want)
	}
	if moves[0].PrintTime != 0.0 || moves[3].PrintTime != 3.0 {
		t.Fatalf("Moves() order = %#v", moves)
	}

	positionSource := &fakeTrapQSource{batches: [][]TrapQMove{{{PrintTime: 10.0, MoveTime: 4.0, StartVelocity: 2.0, Acceleration: 1.0, StartPosition: [3]float64{1, 2, 3}, Direction: [3]float64{1, 0, 0}}}}}
	positionDump := NewTrapQDump("toolhead", positionSource)
	pos, velocity := positionDump.PositionAt(12.0)
	if !reflect.DeepEqual(pos, []float64{7, 2, 3}) {
		t.Fatalf("PositionAt() = %#v, want [7 2 3]", pos)
	}
	if velocity != 4.0 {
		t.Fatalf("velocity = %v, want 4", velocity)
	}
	if log := positionDump.LogMessage([]TrapQMove{{PrintTime: 1.0}}); !strings.Contains(log, "Dumping trapq 'toolhead'") {
		t.Fatalf("expected trapq log header, got %q", log)
	}
}
