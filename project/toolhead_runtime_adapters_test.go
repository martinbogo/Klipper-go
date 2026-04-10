package project

import "testing"

type fakeToolheadQueueExtruder struct {
	moveCalls []fakeToolheadQueueExtruderCall
}

type fakeToolheadQueueExtruderCall struct {
	printTime float64
	move      *Move
}

func (self *fakeToolheadQueueExtruder) Update_move_time(flush_time float64, clear_history_time float64) {
}

func (self *fakeToolheadQueueExtruder) Check_move(move *Move) error { return nil }

func (self *fakeToolheadQueueExtruder) Find_past_position(print_time float64) float64 { return 0 }

func (self *fakeToolheadQueueExtruder) Calc_junction(prev_move, move *Move) float64 { return 0 }

func (self *fakeToolheadQueueExtruder) Move(print_time float64, move *Move) {
	self.moveCalls = append(self.moveCalls, fakeToolheadQueueExtruderCall{printTime: print_time, move: move})
}

func (self *fakeToolheadQueueExtruder) Get_name() string { return "fake" }

func (self *fakeToolheadQueueExtruder) Get_heater() interface{} { return nil }

func (self *fakeToolheadQueueExtruder) Get_trapq() interface{} { return nil }

func TestToolheadMoveBatchSinkQueuesExtruderMoveViaInterface(t *testing.T) {
	fakeExtruder := &fakeToolheadQueueExtruder{}
	sink := &toolheadMoveBatchSink{toolhead: &Toolhead{Extruder: fakeExtruder}}
	move := &Move{}

	sink.QueueExtruderMove(4.25, move)

	if len(fakeExtruder.moveCalls) != 1 {
		t.Fatalf("expected one extruder move call, got %d", len(fakeExtruder.moveCalls))
	}
	if fakeExtruder.moveCalls[0].printTime != 4.25 {
		t.Fatalf("expected print time 4.25, got %v", fakeExtruder.moveCalls[0].printTime)
	}
	if fakeExtruder.moveCalls[0].move != move {
		t.Fatalf("expected original move pointer to be forwarded")
	}
}
