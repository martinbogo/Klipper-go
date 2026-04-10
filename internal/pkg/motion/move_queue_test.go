package motion

import (
	"math"
	"reflect"
	"testing"
)

type fakeMoveJunctionCalculator struct {
	result float64
}

func (self *fakeMoveJunctionCalculator) Calc_junction(prev_move, move *Move) float64 {
	_, _ = prev_move, move
	return self.result
}

type fakeMoveQueueProcessor struct {
	batches [][]*Move
}

func (self *fakeMoveQueueProcessor) Process_moves(moves []*Move) {
	copied := append([]*Move(nil), moves...)
	self.batches = append(self.batches, copied)
}

func testMoveConfig() MoveConfig {
	return MoveConfig{
		Max_accel:          100,
		Junction_deviation: 0.05,
		Max_velocity:       50,
		Max_accel_to_decel: 40,
	}
}

func TestNewMoveCalculatesKinematicDistanceAndLimitsVelocity(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{3, 4, 0, 0}, 75)
	if !move.Is_kinematic_move {
		t.Fatal("expected kinematic move")
	}
	if move.Move_d != 5 {
		t.Fatalf("expected move distance 5, got %v", move.Move_d)
	}
	if move.Max_cruise_v2 != 2500 {
		t.Fatalf("expected velocity cap of 50mm/s, got cruise v2 %v", move.Max_cruise_v2)
	}
	if math.Abs(move.Min_move_t-0.1) > 1e-9 {
		t.Fatalf("expected minimum move time 0.1, got %v", move.Min_move_t)
	}
	if !reflect.DeepEqual(move.Axes_d, []float64{3, 4, 0, 0}) {
		t.Fatalf("unexpected axes delta %#v", move.Axes_d)
	}
}

func TestNewMoveHandlesExtrudeOnlyMoves(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{1, 2, 3, 0}, []float64{1, 2, 3, 4}, 2)
	if move.Is_kinematic_move {
		t.Fatal("expected extrude-only move to be non-kinematic")
	}
	if !reflect.DeepEqual(move.End_pos, []float64{1, 2, 3, 4}) {
		t.Fatalf("unexpected end position %#v", move.End_pos)
	}
	if !reflect.DeepEqual(move.Axes_d, []float64{0, 0, 0, 4}) {
		t.Fatalf("unexpected extrude-only delta %#v", move.Axes_d)
	}
	if move.Move_d != 4 {
		t.Fatalf("expected extrude-only move distance 4, got %v", move.Move_d)
	}
	if move.Axes_r[3] != 1 {
		t.Fatalf("expected unit extruder ratio, got %#v", move.Axes_r)
	}
	if move.Accel < 99999999 {
		t.Fatalf("expected extrude-only accel override, got %v", move.Accel)
	}
}

func TestMoveCalcJunctionHonorsExtruderLimit(t *testing.T) {
	config := testMoveConfig()
	prevMove := NewMove(config, []float64{0, 0, 0, 0}, []float64{10, 0, 0, 0}, 50)
	move := NewMove(config, []float64{10, 0, 0, 0}, []float64{20, 0, 0, 0}, 50)
	move.Calc_junction(prevMove, &fakeMoveJunctionCalculator{result: 81})
	if move.Max_start_v2 != 81 {
		t.Fatalf("expected max start v2 81, got %v", move.Max_start_v2)
	}
	if move.Max_smoothed_v2 != 81 {
		t.Fatalf("expected smoothed v2 81, got %v", move.Max_smoothed_v2)
	}
}

func TestLookAheadQueueFlushProcessesAndClearsMoves(t *testing.T) {
	processor := &fakeMoveQueueProcessor{}
	queue := NewMoveQueue(processor)
	calc := &fakeMoveJunctionCalculator{result: 100}
	config := testMoveConfig()
	queue.Add_move(NewMove(config, []float64{0, 0, 0, 0}, []float64{5, 0, 0, 0}, 25), calc)
	queue.Add_move(NewMove(config, []float64{5, 0, 0, 0}, []float64{10, 0, 0, 0}, 25), calc)

	queue.Flush(false)

	if len(processor.batches) != 1 {
		t.Fatalf("expected one processed batch, got %d", len(processor.batches))
	}
	if len(processor.batches[0]) != 2 {
		t.Fatalf("expected two moves to flush, got %d", len(processor.batches[0]))
	}
	if queue.Queue_len() != 0 {
		t.Fatalf("expected empty queue after flush, got %d", queue.Queue_len())
	}
	if queue.Get_last() != nil {
		t.Fatalf("expected no last move after flush, got %#v", queue.Get_last())
	}
}
