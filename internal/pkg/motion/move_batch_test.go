package motion

import (
	"math"
	"reflect"
	"testing"
)

type fakeMoveBatchSink struct {
	kinematicTimes []float64
	kinematicMoves []*Move
	extruderTimes  []float64
	extruderMoves  []*Move
}

func (self *fakeMoveBatchSink) QueueKinematicMove(printTime float64, move *Move) {
	self.kinematicTimes = append(self.kinematicTimes, printTime)
	self.kinematicMoves = append(self.kinematicMoves, move)
}

func (self *fakeMoveBatchSink) QueueExtruderMove(printTime float64, move *Move) {
	self.extruderTimes = append(self.extruderTimes, printTime)
	self.extruderMoves = append(self.extruderMoves, move)
}

func TestQueueMoveBatchSchedulesMovesAndCallbacks(t *testing.T) {
	sink := &fakeMoveBatchSink{}
	callbackTimes := []float64{}
	kinematic := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{3, 4, 0, 1}, 10)
	kinematic.Set_junction(0, 100, 0)
	kinematic.Timing_callbacks = append(kinematic.Timing_callbacks, func(printTime float64) {
		callbackTimes = append(callbackTimes, printTime)
	})
	extrudeOnly := NewMove(testMoveConfig(), []float64{3, 4, 0, 1}, []float64{3, 4, 0, 3}, 5)
	extrudeOnly.Set_junction(0, 25, 0)

	endTime := QueueMoveBatch(12.5, []*Move{kinematic, extrudeOnly}, sink)
	firstEndTime := 12.5 + kinematic.Accel_t + kinematic.Cruise_t + kinematic.Decel_t
	secondEndTime := firstEndTime + extrudeOnly.Accel_t + extrudeOnly.Cruise_t + extrudeOnly.Decel_t

	if !reflect.DeepEqual(sink.kinematicTimes, []float64{12.5}) {
		t.Fatalf("unexpected kinematic queue times %#v", sink.kinematicTimes)
	}
	if !reflect.DeepEqual(sink.extruderTimes, []float64{12.5, firstEndTime}) {
		t.Fatalf("unexpected extruder queue times %#v", sink.extruderTimes)
	}
	if !reflect.DeepEqual(callbackTimes, []float64{firstEndTime}) {
		t.Fatalf("unexpected callback times %#v", callbackTimes)
	}
	if math.Abs(endTime-secondEndTime) > 1e-9 {
		t.Fatalf("unexpected end time %v", endTime)
	}
	if len(sink.kinematicMoves) != 1 || sink.kinematicMoves[0] != kinematic {
		t.Fatalf("unexpected kinematic moves %#v", sink.kinematicMoves)
	}
	if len(sink.extruderMoves) != 2 || sink.extruderMoves[0] != kinematic || sink.extruderMoves[1] != extrudeOnly {
		t.Fatalf("unexpected extruder moves %#v", sink.extruderMoves)
	}
	if extrudeOnly.Is_kinematic_move {
		t.Fatal("expected second move to remain extrude-only")
	}
	if len(sink.kinematicMoves) != 1 {
		t.Fatalf("expected only one kinematic move, got %d", len(sink.kinematicMoves))
	}
	if len(sink.extruderMoves) != 2 {
		t.Fatalf("expected two extruder moves, got %d", len(sink.extruderMoves))
	}
}
