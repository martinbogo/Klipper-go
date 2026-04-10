package mcu

import (
	"errors"
	"math"
	"testing"
)

type fakeStepperPositionQuerySender struct {
	lastData     []int64
	lastMinclock int64
	lastReqclock int64
	response     map[string]interface{}
}

func (self *fakeStepperPositionQuerySender) Send(data interface{}, minclock, reqclock int64) interface{} {
	self.lastData = append([]int64{}, data.([]int64)...)
	self.lastMinclock = minclock
	self.lastReqclock = reqclock
	return self.response
}

func TestApplyStepperHomingResetQueuesResetMessage(t *testing.T) {
	queue := &fakeStepcompressQueue{}
	if err := ApplyStepperHomingReset(queue, 9, 12); err != nil {
		t.Fatalf("unexpected homing reset error %v", err)
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

func TestExecuteStepperPositionQuerySyncsOffsetFromResponse(t *testing.T) {
	query := &fakeStepperPositionQuerySender{response: map[string]interface{}{"#receive_time": 4.0, "pos": int64(7)}}
	queue := &fakeStepcompressQueue{}
	state := &StepperPositionState{StepDist: 0.2, InvertDir: 1}
	lastPos, err := ExecuteStepperPositionQuery(query, 12, 1.3, state, queue, func(value interface{}) float64 {
		return value.(float64)
	}, func(receiveTime float64) float64 {
		return receiveTime + 2.0
	}, func(printTime float64) int64 {
		return int64(printTime * 1000)
	})
	if err != nil {
		t.Fatalf("unexpected query error %v", err)
	}
	if len(query.lastData) != 1 || query.lastData[0] != 12 {
		t.Fatalf("unexpected query payload %#v", query.lastData)
	}
	if query.lastMinclock != 0 || query.lastReqclock != 0 {
		t.Fatalf("unexpected query clocks %d %d", query.lastMinclock, query.lastReqclock)
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

func TestExecuteStepperPositionQueryReturnsStepcompressError(t *testing.T) {
	query := &fakeStepperPositionQuerySender{response: map[string]interface{}{"#receive_time": 4.0, "pos": int64(7)}}
	queue := &fakeStepcompressQueue{setLastPositionRet: 1}
	state := &StepperPositionState{StepDist: 0.2}
	_, err := ExecuteStepperPositionQuery(query, 12, 1.3, state, queue, func(value interface{}) float64 {
		return value.(float64)
	}, func(receiveTime float64) float64 {
		return receiveTime + 2.0
	}, func(printTime float64) int64 {
		return int64(printTime * 1000)
	})
	if !errors.Is(err, ErrStepcompress) {
		t.Fatalf("expected stepcompress error, got %v", err)
	}
}