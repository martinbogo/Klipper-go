package vibration

import (
	"math"
	"reflect"
	"testing"
)

type fakeAccelToolhead struct {
	lastMoveTime float64
	waitCount    int
}

func (self *fakeAccelToolhead) Get_last_move_time() float64 {
	return self.lastMoveTime
}

func (self *fakeAccelToolhead) Wait_moves() {
	self.waitCount += 1
}

type fakeAccelDumpConnection struct {
	msgs      []map[string]map[string]interface{}
	finalized bool
}

func (self *fakeAccelDumpConnection) Finalize() {
	self.finalized = true
}

func (self *fakeAccelDumpConnection) Get_messages() []map[string]map[string]interface{} {
	return self.msgs
}

type fakeClockSyncMCU struct{}

func (self *fakeClockSyncMCU) Clock_to_print_time(clock int64) float64 {
	return float64(clock) / 100.
}

func TestAccelQueryHelperFiltersSamplesToRequestWindow(t *testing.T) {
	toolhead := &fakeAccelToolhead{lastMoveTime: 1.0}
	conn := &fakeAccelDumpConnection{msgs: []map[string]map[string]interface{}{
		{"params": {"data": [][]float64{{0.5, 1, 2, 3}, {1.5, 4, 5, 6}, {2.5, 7, 8, 9}}}},
	}}
	helper := NewAccelQueryHelper(toolhead, conn)
	helper.request_end_time = 2.0

	samples := helper.Get_samples()
	expected := [][]float64{{1.5, 4, 5, 6}}
	if !reflect.DeepEqual(samples, expected) {
		t.Fatalf("unexpected samples %#v", samples)
	}
	helper.Finish_measurements()
	if !conn.finalized || toolhead.waitCount != 1 {
		t.Fatalf("expected finalize/wait to run, finalized=%v waits=%d", conn.finalized, toolhead.waitCount)
	}
}

func TestClockSyncRegressionGetTimeTranslation(t *testing.T) {
	regression := NewClockSyncRegression(&fakeClockSyncMCU{}, 10, 0.5)
	regression.Reset(100, 200)
	regression.Update(120, 220)

	baseTime, baseChip, invFreq := regression.Get_time_translation()
	if math.Abs(baseTime-1.1) > 1e-9 || baseChip != 210 || math.Abs(invFreq-0.01) > 1e-9 {
		t.Fatalf("unexpected translation baseTime=%v baseChip=%v invFreq=%v", baseTime, baseChip, invFreq)
	}
}