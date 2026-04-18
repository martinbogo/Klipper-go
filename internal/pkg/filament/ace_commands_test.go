package filament

import "testing"

func TestACECommandErrorHandlesNumericCodes(t *testing.T) {
	if err := ACECommandError(map[string]interface{}{"code": float64(0)}); err != nil {
		t.Fatalf("expected nil error for zero code, got %v", err)
	}

	err := ACECommandError(map[string]interface{}{"code": int64(12), "msg": "bad request"})
	if err == nil {
		t.Fatalf("expected error for non-zero code")
	}
	if got := err.Error(); got != "ACE Error: bad request" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestBuildFeedAssistRequest(t *testing.T) {
	request := BuildFeedAssistRequest(2, true)
	if request["method"] != "start_feed_assist" {
		t.Fatalf("unexpected method: %v", request["method"])
	}
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected params map, got %#v", request["params"])
	}
	if params["index"] != 2 {
		t.Fatalf("unexpected index: %v", params["index"])
	}

	request = BuildFeedAssistRequest(1, false)
	if request["method"] != "stop_feed_assist" {
		t.Fatalf("unexpected disable method: %v", request["method"])
	}
}

func TestBuildDryingPlanValidatesInputsAndPayload(t *testing.T) {
	if _, err := BuildDryingPlan(45, 0, 55); err == nil || err.Error() != "Wrong duration" {
		t.Fatalf("expected duration validation error, got %v", err)
	}
	if _, err := BuildDryingPlan(60, 120, 55); err == nil || err.Error() != "Wrong temperature" {
		t.Fatalf("expected temperature validation error, got %v", err)
	}

	plan, err := BuildDryingPlan(45, 120, 55)
	if err != nil {
		t.Fatalf("unexpected error building drying plan: %v", err)
	}
	if plan.Delay != 0 {
		t.Fatalf("expected zero delay, got %v", plan.Delay)
	}
	if plan.SuccessMessage != "" {
		t.Fatalf("expected empty success message, got %q", plan.SuccessMessage)
	}
	if plan.Request["method"] != "drying" {
		t.Fatalf("unexpected method: %v", plan.Request["method"])
	}
	params, ok := plan.Request["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected params map, got %#v", plan.Request["params"])
	}
	if params["temp"] != 45 || params["duration"] != 120 || params["fan_speed"] != 7000 {
		t.Fatalf("unexpected drying params: %#v", params)
	}
}

func TestBuildFeedAssistPlanTracksDelayMessageAndState(t *testing.T) {
	if _, err := BuildFeedAssistPlan(-1, true); err == nil || err.Error() != "Wrong index" {
		t.Fatalf("expected index validation error, got %v", err)
	}

	enablePlan, err := BuildFeedAssistPlan(2, true)
	if err != nil {
		t.Fatalf("unexpected enable error: %v", err)
	}
	if enablePlan.Delay != 0.7 || enablePlan.ResultIndex != 2 {
		t.Fatalf("unexpected enable plan: %#v", enablePlan)
	}
	if enablePlan.SuccessMessage != "ACE: Enabled ACE feed assist" {
		t.Fatalf("unexpected enable message: %q", enablePlan.SuccessMessage)
	}

	disablePlan, err := BuildFeedAssistPlan(2, false)
	if err != nil {
		t.Fatalf("unexpected disable error: %v", err)
	}
	if disablePlan.Delay != 0.3 || disablePlan.ResultIndex != -1 {
		t.Fatalf("unexpected disable plan: %#v", disablePlan)
	}
	if disablePlan.SuccessMessage != "ACE: Disabled ACE feed assist" {
		t.Fatalf("unexpected disable message: %q", disablePlan.SuccessMessage)
	}
}

func TestBuildMotionRequestsAndDelay(t *testing.T) {
	feed := BuildFeedRequest(1, 25, 5)
	if feed["method"] != "feed_filament" {
		t.Fatalf("unexpected feed method: %v", feed["method"])
	}
	feedParams := feed["params"].(map[string]interface{})
	if feedParams["length"] != 25 || feedParams["speed"] != 5 {
		t.Fatalf("unexpected feed params: %#v", feedParams)
	}

	retract := BuildRetractRequest(3, 40, 8)
	if retract["method"] != "unwind_filament" {
		t.Fatalf("unexpected retract method: %v", retract["method"])
	}

	if delay := MotionCommandDelay(11, 5); delay != 2.1 {
		t.Fatalf("expected legacy integer-division delay of 2.1, got %v", delay)
	}
}

func TestBuildStrictMotionPlansValidateAndNormalize(t *testing.T) {
	if _, err := BuildStrictFeedPlan(4, 20, 5); err == nil || err.Error() != "Wrong index" {
		t.Fatalf("expected index validation error, got %v", err)
	}
	if _, err := BuildStrictFeedPlan(1, 0, 5); err == nil || err.Error() != "Wrong length" {
		t.Fatalf("expected length validation error, got %v", err)
	}
	if _, err := BuildStrictRetractPlan(1, 20, 0); err == nil || err.Error() != "Wrong speed" {
		t.Fatalf("expected speed validation error, got %v", err)
	}

	plan, err := BuildStrictFeedPlan(1, 25, 5)
	if err != nil {
		t.Fatalf("unexpected strict feed error: %v", err)
	}
	if plan.Delay != 5.1 {
		t.Fatalf("unexpected strict feed delay: %v", plan.Delay)
	}
}

func TestBuildUIPlansFallbackToDefaults(t *testing.T) {
	feedPlan := BuildUIFeedPlan(3, 0, -2, 100, 50)
	feedParams := feedPlan.Request["params"].(map[string]interface{})
	if feedParams["length"] != 100 || feedParams["speed"] != 50 {
		t.Fatalf("expected feed defaults, got %#v", feedParams)
	}
	if feedPlan.Delay != 2.1 {
		t.Fatalf("expected normalized feed delay, got %v", feedPlan.Delay)
	}

	unwindPlan := BuildUIUnwindPlan(1, -1, 0, 100, 50)
	unwindParams := unwindPlan.Request["params"].(map[string]interface{})
	if unwindParams["length"] != 100 || unwindParams["speed"] != 50 {
		t.Fatalf("expected unwind defaults, got %#v", unwindParams)
	}
}

func TestBuildUICommandPlansTrackEffectiveIndex(t *testing.T) {
	feedPlan := BuildUIFeedCommandPlan(2, 40, 20, 100, 50)
	if feedPlan.EffectiveIndex != 2 {
		t.Fatalf("expected feed effective index 2, got %d", feedPlan.EffectiveIndex)
	}

	unwindPlan := BuildUIUnwindCommandPlan(-1, 3, 0, 0, 100, 50)
	if unwindPlan.EffectiveIndex != 3 {
		t.Fatalf("expected unwind to fall back to feed assist index, got %d", unwindPlan.EffectiveIndex)
	}
	unwindParams := unwindPlan.Request["params"].(map[string]interface{})
	if unwindParams["index"] != 3 {
		t.Fatalf("expected unwind request to use fallback index, got %#v", unwindParams)
	}

	defaultPlan := BuildUIUnwindCommandPlan(-1, -1, 0, 0, 100, 50)
	if defaultPlan.EffectiveIndex != 0 {
		t.Fatalf("expected unwind to fall back to slot 0, got %d", defaultPlan.EffectiveIndex)
	}
}
