package mcu

import "testing"

type fakeTrsyncCommandSender struct {
	calls []struct {
		data     interface{}
		minclock int64
		reqclock int64
	}
}

func (self *fakeTrsyncCommandSender) Send(data interface{}, minclock, reqclock int64) {
	self.calls = append(self.calls, struct {
		data     interface{}
		minclock int64
		reqclock int64
	}{data: data, minclock: minclock, reqclock: reqclock})
}

type fakeTrsyncQuerySender struct {
	response interface{}
	lastData interface{}
}

func (self *fakeTrsyncQuerySender) Send(data interface{}, minclock, reqclock int64) interface{} {
	_, _ = minclock, reqclock
	self.lastData = data
	return self.response
}

func TestTrsyncCommandRuntimeRoutesCommands(t *testing.T) {
	start := &fakeTrsyncCommandSender{}
	timeout := &fakeTrsyncCommandSender{}
	trigger := &fakeTrsyncCommandSender{}
	stop := &fakeTrsyncCommandSender{}
	query := &fakeTrsyncQuerySender{response: map[string]interface{}{"trigger_reason": int64(7)}}

	setupCount := 0
	registerCount := 0
	unregisterCount := 0
	runtime := NewTrsyncCommandRuntime(11)
	runtime.Configure(func(plan TrsyncStartPlan) {
		setupCount++
		if plan.Clock != 3 {
			t.Fatalf("unexpected plan clock %d", plan.Clock)
		}
	}, func() {
		registerCount++
	}, func() {
		unregisterCount++
	}, start, timeout, trigger, stop, query)

	runtime.SetupDispatch(TrsyncStartPlan{Clock: 3})
	runtime.RegisterStateResponse()
	runtime.SendStart([]int64{1, 2}, 4, 5)
	runtime.SendStepperStop(9)
	runtime.SendTimeout([]int64{6, 7}, 8, 9)
	runtime.TriggerPastEnd()
	gotReason := runtime.QueryTriggerReason(ReasonHostRequest)
	runtime.UnregisterStateResponse()

	if setupCount != 1 || registerCount != 1 || unregisterCount != 1 {
		t.Fatalf("unexpected lifecycle counts: setup=%d register=%d unregister=%d", setupCount, registerCount, unregisterCount)
	}
	if len(start.calls) != 1 || len(timeout.calls) != 1 || len(trigger.calls) != 1 || len(stop.calls) != 1 {
		t.Fatalf("unexpected command call counts: start=%d timeout=%d trigger=%d stop=%d", len(start.calls), len(timeout.calls), len(trigger.calls), len(stop.calls))
	}
	if gotReason != 7 {
		t.Fatalf("expected trigger reason 7, got %d", gotReason)
	}
	if got := stop.calls[0].data.([]int64); len(got) != 2 || got[0] != 9 || got[1] != 11 {
		t.Fatalf("unexpected stop payload: %#v", got)
	}
	if got := trigger.calls[0].data.([]int64); len(got) != 2 || got[0] != 11 || got[1] != ReasonPastEndTime {
		t.Fatalf("unexpected trigger payload: %#v", got)
	}
}
