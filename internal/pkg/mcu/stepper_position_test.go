package mcu

import (
	"errors"
	"math"
	"testing"
)

type fakeStepcompressQueue struct {
	lastFindClock        uint64
	findPastPosition     int64
	resetCalls           []uint64
	queueMessages        [][]uint32
	setLastPositionCalls [][2]int64
	resetRet             int
	queueRet             int
	setLastPositionRet   int
}

func (self *fakeStepcompressQueue) FindPastPosition(clock uint64) int64 {
	self.lastFindClock = clock
	return self.findPastPosition
}

func (self *fakeStepcompressQueue) Reset(lastStepClock uint64) int {
	self.resetCalls = append(self.resetCalls, lastStepClock)
	return self.resetRet
}

func (self *fakeStepcompressQueue) QueueMessage(data []uint32) int {
	self.queueMessages = append(self.queueMessages, append([]uint32(nil), data...))
	return self.queueRet
}

func (self *fakeStepcompressQueue) SetLastPosition(clock uint64, lastPosition int64) int {
	self.setLastPositionCalls = append(self.setLastPositionCalls, [2]int64{int64(clock), lastPosition})
	return self.setLastPositionRet
}

func TestStepperPositionStateGetAndSetMCUPosition(t *testing.T) {
	state := &StepperPositionState{StepDist: 0.2}
	if got := state.GetMCUPosition(1.11); got != 6 {
		t.Fatalf("unexpected MCU position %d", got)
	}
	state.SetMCUPosition(6, 1.11)
	if math.Abs(state.MCUPositionOffset-0.09) > 1e-9 {
		t.Fatalf("unexpected MCU position offset %v", state.MCUPositionOffset)
	}
	if got := state.MCUToCommandedPosition(6); math.Abs(got-1.11) > 1e-9 {
		t.Fatalf("unexpected commanded position %v", got)
	}
}

func TestStepperPositionStateRoundsNegativeMCUPosition(t *testing.T) {
	state := &StepperPositionState{StepDist: 0.2}
	if got := state.GetMCUPosition(-1.11); got != -6 {
		t.Fatalf("unexpected MCU position %d", got)
	}
}

func TestStepperPositionStatePastMCUPosition(t *testing.T) {
	queue := &fakeStepcompressQueue{findPastPosition: 17}
	state := &StepperPositionState{}
	got := state.PastMCUPosition(3.25, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, queue)
	if got != 17 {
		t.Fatalf("unexpected past position %d", got)
	}
	if queue.lastFindClock != 3250 {
		t.Fatalf("unexpected queried clock %d", queue.lastFindClock)
	}
}

func TestStepperPositionStateNoteHomingEndQueuesResetMessage(t *testing.T) {
	queue := &fakeStepcompressQueue{}
	state := &StepperPositionState{}
	if err := state.NoteHomingEnd(queue, 9, 12); err != nil {
		t.Fatalf("unexpected homing end error %v", err)
	}
	if len(queue.resetCalls) != 1 || queue.resetCalls[0] != 0 {
		t.Fatalf("unexpected reset calls %#v", queue.resetCalls)
	}
	if len(queue.queueMessages) != 1 {
		t.Fatalf("unexpected queued messages %#v", queue.queueMessages)
	}
	want := []uint32{9, 12, 0}
	got := queue.queueMessages[0]
	if len(got) != len(want) {
		t.Fatalf("unexpected queued message %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected queued message %#v", got)
		}
	}
}

func TestStepperPositionStateNoteHomingEndReturnsStepcompressError(t *testing.T) {
	state := &StepperPositionState{}
	if err := state.NoteHomingEnd(&fakeStepcompressQueue{resetRet: 1}, 9, 12); !errors.Is(err, ErrStepcompress) {
		t.Fatalf("expected stepcompress error, got %v", err)
	}
	if err := state.NoteHomingEnd(&fakeStepcompressQueue{queueRet: 1}, 9, 12); !errors.Is(err, ErrStepcompress) {
		t.Fatalf("expected stepcompress error, got %v", err)
	}
}

func TestStepperPositionStateSyncFromQueryResponse(t *testing.T) {
	queue := &fakeStepcompressQueue{}
	state := &StepperPositionState{StepDist: 0.2, InvertDir: 1}
	lastPos, err := state.SyncFromQueryResponse(7, 4.0, 1.3, func(receiveTime float64) float64 {
		return receiveTime + 2.0
	}, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, queue)
	if err != nil {
		t.Fatalf("unexpected sync error %v", err)
	}
	if lastPos != -7 {
		t.Fatalf("unexpected last position %d", lastPos)
	}
	if len(queue.setLastPositionCalls) != 1 || queue.setLastPositionCalls[0] != [2]int64{6000, -7} {
		t.Fatalf("unexpected set last position calls %#v", queue.setLastPositionCalls)
	}
	if math.Abs(state.MCUPositionOffset-(-2.7)) > 1e-9 {
		t.Fatalf("unexpected MCU position offset %v", state.MCUPositionOffset)
	}
}

func TestStepperPositionStateSyncFromQueryResponseReturnsStepcompressError(t *testing.T) {
	queue := &fakeStepcompressQueue{setLastPositionRet: 1}
	state := &StepperPositionState{StepDist: 0.2}
	_, err := state.SyncFromQueryResponse(7, 4.0, 1.3, func(receiveTime float64) float64 {
		return receiveTime + 2.0
	}, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, queue)
	if !errors.Is(err, ErrStepcompress) {
		t.Fatalf("expected stepcompress error, got %v", err)
	}
}
