package mcu

import "testing"

type fakeCompletion struct {
	results []interface{}
}

func (self *fakeCompletion) Complete(result interface{}) {
	self.results = append(self.results, result)
}

func TestTrsyncRuntimeStateShutdownCompletesPendingTrigger(t *testing.T) {
	completion := &fakeCompletion{}
	state := &TrsyncRuntimeState{TriggerCompletion: completion}
	state.Shutdown()
	if len(completion.results) != 1 || completion.results[0] != false {
		t.Fatalf("unexpected completion results %#v", completion.results)
	}
	if state.TriggerCompletion != nil {
		t.Fatal("expected trigger completion to be cleared")
	}
}

func TestTrsyncRuntimeStateHandleStateAsyncCompletesOnTriggerReason(t *testing.T) {
	state := &TrsyncRuntimeState{TriggerCompletion: &fakeCompletion{}}
	results := []map[string]interface{}{}
	handled := state.HandleState(map[string]interface{}{"can_trigger": int64(0), "trigger_reason": int64(ReasonCommsTimeout)}, func(clock int64) int64 {
		return clock
	}, func(result map[string]interface{}) {
		results = append(results, result)
	}, func() {
		t.Fatal("unexpected past-end trigger")
	})
	if !handled {
		t.Fatal("expected state to be handled")
	}
	if state.TriggerCompletion != nil {
		t.Fatal("expected trigger completion to be cleared")
	}
	if len(results) != 1 || results[0]["aa"] != true {
		t.Fatalf("unexpected async-complete results %#v", results)
	}
}

func TestTrsyncRuntimeStateHandleStateTriggersPastEnd(t *testing.T) {
	homeEndClock := int64(200)
	state := &TrsyncRuntimeState{HomeEndClock: &homeEndClock}
	triggered := false
	handled := state.HandleState(map[string]interface{}{"can_trigger": int64(1), "clock": int64(250)}, func(clock int64) int64 {
		return clock
	}, func(result map[string]interface{}) {
		_ = result
		t.Fatal("unexpected async complete")
	}, func() {
		triggered = true
	})
	if !handled {
		t.Fatal("expected state to be handled")
	}
	if !triggered {
		t.Fatal("expected past-end trigger to fire")
	}
	if state.HomeEndClock != nil {
		t.Fatal("expected home end clock to be cleared")
	}
}

func TestTrsyncRuntimeStateHandleStateReturnsFalseWhenNoActionNeeded(t *testing.T) {
	homeEndClock := int64(300)
	state := &TrsyncRuntimeState{HomeEndClock: &homeEndClock}
	handled := state.HandleState(map[string]interface{}{"can_trigger": int64(1), "clock": int64(250)}, func(clock int64) int64 {
		return clock
	}, func(result map[string]interface{}) {
		_ = result
		t.Fatal("unexpected async complete")
	}, func() {
		t.Fatal("unexpected past-end trigger")
	})
	if handled {
		t.Fatal("expected state to remain unhandled")
	}
	if state.HomeEndClock == nil || *state.HomeEndClock != 300 {
		t.Fatalf("unexpected home end clock %#v", state.HomeEndClock)
	}
}

func TestTrsyncRuntimeStateSetHomeEndTime(t *testing.T) {
	state := &TrsyncRuntimeState{}
	state.SetHomeEndTime(2.5, func(printTime float64) int64 {
		return int64(printTime * 1000)
	})
	if state.HomeEndClock == nil || *state.HomeEndClock != 2500 {
		t.Fatalf("unexpected home end clock %#v", state.HomeEndClock)
	}
}

func TestBuildTrsyncStartPlan(t *testing.T) {
	plan := BuildTrsyncStartPlan(10.0, 0.5, 0.25, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, func(seconds float64) int64 {
		return int64(seconds * 1000)
	})
	if plan.Clock != 10000 || plan.ExpireTicks != 250 || plan.ExpireClock != 10250 || plan.ReportTicks != 75 || plan.ReportClock != 10038 || plan.MinExtendTicks != 60 {
		t.Fatalf("unexpected trsync start plan %#v", plan)
	}
}
