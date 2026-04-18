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

func TestMoveCompatibilityMethodsExposeLegacyFields(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{1, 2, 3, 4}, []float64{6, 8, 3, 9}, 25)

	if !reflect.DeepEqual(move.EndPos(), move.End_pos) {
		t.Fatalf("expected EndPos to mirror End_pos, got %#v want %#v", move.EndPos(), move.End_pos)
	}
	if !reflect.DeepEqual(move.AxesD(), move.Axes_d) {
		t.Fatalf("expected AxesD to mirror Axes_d, got %#v want %#v", move.AxesD(), move.Axes_d)
	}
	if move.MoveD() != move.Move_d {
		t.Fatalf("expected MoveD to mirror Move_d, got %v want %v", move.MoveD(), move.Move_d)
	}

	move.LimitSpeed(10, 20)
	if move.Max_cruise_v2 != 100 {
		t.Fatalf("expected LimitSpeed to reuse Limit_speed, got cruise v2 %v", move.Max_cruise_v2)
	}
	if move.Accel != 20 {
		t.Fatalf("expected LimitSpeed to clamp accel to 20, got %v", move.Accel)
	}

	err := move.MoveError("boom")
	if err == nil || err.Error() != "boom: 6.000 8.000 3.000 [9.000]" {
		t.Fatalf("expected MoveError formatting to match Move_error, got %v", err)
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
	if move.Max_mcr_start_v2 != 81 {
		t.Fatalf("expected mcr start v2 81, got %v", move.Max_mcr_start_v2)
	}
}

func TestMoveLimitNextJunctionSpeedCapsFollowingMove(t *testing.T) {
	config := testMoveConfig()
	prevMove := NewMove(config, []float64{0, 0, 0, 0}, []float64{10, 0, 0, 0}, 50)
	move := NewMove(config, []float64{10, 0, 0, 0}, []float64{20, 0, 0, 0}, 50)
	prevMove.LimitNextJunctionSpeed(6.0)

	move.Calc_junction(prevMove, &fakeMoveJunctionCalculator{result: 2500})

	if move.Max_start_v2 != 36.0 {
		t.Fatalf("expected next-junction cap of 36, got %v", move.Max_start_v2)
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

func TestLookAheadQueueAddMoveSignalsLazyFlushWithoutAutoProcessing(t *testing.T) {
	processor := &fakeMoveQueueProcessor{}
	queue := NewMoveQueue(processor)
	calc := &fakeMoveJunctionCalculator{result: 100}
	config := testMoveConfig()
	if queue.Add_move(NewMove(config, []float64{0, 0, 0, 0}, []float64{5, 0, 0, 0}, 25), calc) {
		t.Fatal("did not expect first move to request a lazy flush")
	}
	if !queue.Add_move(NewMove(config, []float64{5, 0, 0, 0}, []float64{10, 0, 0, 0}, 25), calc) {
		t.Fatal("expected second move to request a lazy flush")
	}

	if len(processor.batches) != 0 {
		t.Fatalf("expected Add_move to avoid auto-processing, got %d batches", len(processor.batches))
	}
	if queue.Queue_len() != 2 {
		t.Fatalf("expected queue to remain intact until lazy flush is run, got %d", queue.Queue_len())
	}

	queue.Flush(true)

	if len(processor.batches) != 1 || len(processor.batches[0]) != 1 {
		t.Fatalf("expected lazy flush to process the boundary-safe prefix, got %#v", processor.batches)
	}
	if queue.Queue_len() != 1 {
		t.Fatalf("expected one deferred move to remain queued after lazy flush, got %d", queue.Queue_len())
	}
}
