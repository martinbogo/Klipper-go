package mcu

import "testing"

type fakeWaitableCompletion struct {
	results    []interface{}
	waitCalls  []fakeWaitCall
	waitResult interface{}
}

type fakeWaitCall struct {
	waketime       float64
	waketimeResult interface{}
}

func (self *fakeWaitableCompletion) Complete(result interface{}) {
	self.results = append(self.results, result)
}

func (self *fakeWaitableCompletion) Wait(waketime float64, waketimeResult interface{}) interface{} {
	self.waitCalls = append(self.waitCalls, fakeWaitCall{waketime: waketime, waketimeResult: waketimeResult})
	return self.waitResult
}

func TestEndstopHomeRuntimeStateBuildHomeStartPlanSingleTrsync(t *testing.T) {
	completion := &fakeWaitableCompletion{}
	state := &EndstopHomeRuntimeState{}
	plan := state.BuildHomeStartPlan(10.0, 0.015, 4, 0.25, 1, 1, 1, 7, completion, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, func(seconds float64) int64 {
		return int64(seconds * 1000)
	}, 0.025, 0.25, ReasonEndstopHit)
	if state.RestTicks != 250 {
		t.Fatalf("unexpected rest ticks %d", state.RestTicks)
	}
	if state.TriggerCompletion != completion {
		t.Fatal("expected trigger completion to be stored")
	}
	if plan.Clock != 10000 || plan.SampleTicks != 15 || plan.SampleCount != 4 || plan.RestTicks != 250 || plan.PinValue != 0 || plan.TrsyncOID != 7 || plan.TriggerReason != ReasonEndstopHit || plan.ExpireTimeout != 0.25 {
		t.Fatalf("unexpected home start plan %#v", plan)
	}
	if len(plan.ReportOffsets) != 1 || plan.ReportOffsets[0] != 0 {
		t.Fatalf("unexpected report offsets %#v", plan.ReportOffsets)
	}
}

func TestEndstopHomeRuntimeStateBuildHomeStartPlanMultiTrsync(t *testing.T) {
	state := &EndstopHomeRuntimeState{}
	plan := state.BuildHomeStartPlan(5.0, 0.01, 3, 0.5, 0, 0, 2, 11, &fakeWaitableCompletion{}, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, func(seconds float64) int64 {
		return int64(seconds * 1000)
	}, 0.025, 0.25, ReasonEndstopHit)
	if plan.ExpireTimeout != 0.025 {
		t.Fatalf("expected multi-mcu timeout, got %f", plan.ExpireTimeout)
	}
	if len(plan.ReportOffsets) != 2 || plan.ReportOffsets[0] != 0 || plan.ReportOffsets[1] != 0.5 {
		t.Fatalf("unexpected report offsets %#v", plan.ReportOffsets)
	}
}

func TestEndstopHomeRuntimeStateWaitForTriggerCompletesFileoutput(t *testing.T) {
	completion := &fakeWaitableCompletion{waitResult: "done"}
	state := &EndstopHomeRuntimeState{TriggerCompletion: completion}
	result := state.WaitForTrigger(true, 123.0, "fallback")
	if len(completion.results) != 1 || completion.results[0] != true {
		t.Fatalf("unexpected completion results %#v", completion.results)
	}
	if len(completion.waitCalls) != 1 || completion.waitCalls[0].waketime != 123.0 || completion.waitCalls[0].waketimeResult != "fallback" {
		t.Fatalf("unexpected wait calls %#v", completion.waitCalls)
	}
	if result != "done" {
		t.Fatalf("unexpected wait result %#v", result)
	}
}

func TestEvaluateEndstopHomeWaitAllTimeouts(t *testing.T) {
	decision := EvaluateEndstopHomeWait(5.0, []int64{ReasonCommsTimeout, ReasonCommsTimeout}, false, ReasonEndstopHit, ReasonCommsTimeout)
	if decision.Result != -1.0 || decision.NeedsQuery {
		t.Fatalf("unexpected decision %#v", decision)
	}
}

func TestEvaluateEndstopHomeWaitNonHitReturnsZero(t *testing.T) {
	decision := EvaluateEndstopHomeWait(5.0, []int64{ReasonHostRequest}, false, ReasonEndstopHit, ReasonCommsTimeout)
	if decision.Result != 0.0 || decision.NeedsQuery {
		t.Fatalf("unexpected decision %#v", decision)
	}
}

func TestEvaluateEndstopHomeWaitFileoutputUsesHomeEndTime(t *testing.T) {
	decision := EvaluateEndstopHomeWait(7.5, []int64{ReasonEndstopHit}, true, ReasonEndstopHit, ReasonCommsTimeout)
	if decision.Result != 7.5 || decision.NeedsQuery {
		t.Fatalf("unexpected decision %#v", decision)
	}
}

func TestEvaluateEndstopHomeWaitRequestsQueryForTriggeredResult(t *testing.T) {
	decision := EvaluateEndstopHomeWait(7.5, []int64{ReasonEndstopHit, ReasonCommsTimeout}, false, ReasonEndstopHit, ReasonCommsTimeout)
	if decision.Result != 0.0 || !decision.NeedsQuery {
		t.Fatalf("unexpected decision %#v", decision)
	}
}

func TestEndstopHomeRuntimeStateHomeEndTimeFromNextClock(t *testing.T) {
	state := &EndstopHomeRuntimeState{RestTicks: 30}
	result := state.HomeEndTimeFromNextClock(250, func(clock int64) int64 {
		return clock + 1000
	}, func(clock int64) float64 {
		return float64(clock) / 100.0
	})
	if result != 12.2 {
		t.Fatalf("unexpected home end time %f", result)
	}
}

func TestQueryEndstopReturnsInvertedPinValue(t *testing.T) {
	queries := []int64{}
	result := QueryEndstop(2.5, 1, false, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, func(clock int64) int64 {
		queries = append(queries, clock)
		return 1
	})
	if len(queries) != 1 || queries[0] != 2500 {
		t.Fatalf("unexpected query clocks %#v", queries)
	}
	if result != 0 {
		t.Fatalf("unexpected query result %d", result)
	}
}

func TestQueryEndstopSkipsQueryForFileoutput(t *testing.T) {
	queried := false
	result := QueryEndstop(2.5, 0, true, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, func(clock int64) int64 {
		queried = true
		return clock
	})
	if queried {
		t.Fatal("did not expect query callback for fileoutput")
	}
	if result != 0 {
		t.Fatalf("unexpected query result %d", result)
	}
}
