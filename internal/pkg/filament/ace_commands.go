package filament

import "fmt"

const aceSlotCount = 4

// ACECommandPlan describes a validated ACE request together with the dwell and
// success response behavior expected by the legacy project wrapper.
type ACECommandPlan struct {
	Request        map[string]interface{}
	Delay          float64
	SuccessMessage string
}

// ACEFeedAssistPlan extends ACECommandPlan with the runtime feed-assist index
// that should be stored after a successful command.
type ACEFeedAssistPlan struct {
	ACECommandPlan
	ResultIndex int
}

// ACEUICommandPlan extends ACECommandPlan with the effective slot index used
// after native UI defaults and fallback rules have been applied.
type ACEUICommandPlan struct {
	ACECommandPlan
	EffectiveIndex int
}

// ACECommandError converts an ACE response payload into an error when the
// response indicates a non-zero code.
func ACECommandError(response map[string]interface{}) error {
	if response == nil {
		return nil
	}
	code, ok := aceResponseCode(response["code"])
	if !ok || code == 0 {
		return nil
	}
	return fmt.Errorf("ACE Error: %v", response["msg"])
}

func aceResponseCode(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

// BuildACERequest returns a standard ACE request payload with only a method.
func BuildACERequest(method string) map[string]interface{} {
	return map[string]interface{}{"method": method}
}

// BuildACERequestWithParams returns a standard ACE request payload with a
// params object.
func BuildACERequestWithParams(method string, params map[string]interface{}) map[string]interface{} {
	request := BuildACERequest(method)
	request["params"] = params
	return request
}

// BuildDryingPlan validates dryer settings and returns the ACE command plan
// used by ACE_START_DRYING.
func BuildDryingPlan(temperature, duration, maxTemperature int) (ACECommandPlan, error) {
	if duration <= 0 {
		return ACECommandPlan{}, fmt.Errorf("Wrong duration")
	}
	if temperature <= 0 || temperature > maxTemperature {
		return ACECommandPlan{}, fmt.Errorf("Wrong temperature")
	}
	return ACECommandPlan{
		Request: BuildACERequestWithParams("drying", map[string]interface{}{
			"temp":      temperature,
			"fan_speed": 7000,
			"duration":  duration,
		}),
	}, nil
}

// BuildStopDryingPlan returns the ACE command plan used by ACE_STOP_DRYING.
func BuildStopDryingPlan() ACECommandPlan {
	return ACECommandPlan{Request: BuildACERequest("drying_stop")}
}

// BuildFeedAssistPlan validates a slot index and returns the feed-assist plan
// used by ACE feed-assist flows.
func BuildFeedAssistPlan(index int, enable bool) (ACEFeedAssistPlan, error) {
	if err := validateACESlotIndex(index); err != nil {
		return ACEFeedAssistPlan{}, err
	}
	plan := ACEFeedAssistPlan{
		ACECommandPlan: ACECommandPlan{
			Request:        BuildFeedAssistRequest(index, enable),
			SuccessMessage: "ACE: Enabled ACE feed assist",
		},
		ResultIndex: index,
	}
	if enable {
		plan.Delay = 0.7
		return plan, nil
	}
	plan.Delay = 0.3
	plan.SuccessMessage = "ACE: Disabled ACE feed assist"
	plan.ResultIndex = -1
	return plan, nil
}

// BuildFeedAssistRequest returns the ACE request payload for toggling feed
// assist for a slot.
func BuildFeedAssistRequest(index int, enable bool) map[string]interface{} {
	method := "stop_feed_assist"
	if enable {
		method = "start_feed_assist"
	}
	return BuildACERequestWithParams(method, map[string]interface{}{"index": index})
}

// BuildFeedRequest returns the ACE request payload for feeding filament.
func BuildFeedRequest(index, length, speed int) map[string]interface{} {
	return buildFilamentMotionRequest("feed_filament", index, length, speed)
}

// BuildRetractRequest returns the ACE request payload for retracting filament.
func BuildRetractRequest(index, length, speed int) map[string]interface{} {
	return buildFilamentMotionRequest("unwind_filament", index, length, speed)
}

// BuildStrictFeedPlan validates ACE_FEED parameters and returns the resulting
// motion command plan.
func BuildStrictFeedPlan(index, length, speed int) (ACECommandPlan, error) {
	return buildStrictMotionPlan(BuildFeedRequest, index, length, speed)
}

// BuildStrictRetractPlan validates ACE_RETRACT parameters and returns the
// resulting motion command plan.
func BuildStrictRetractPlan(index, length, speed int) (ACECommandPlan, error) {
	return buildStrictMotionPlan(BuildRetractRequest, index, length, speed)
}

// BuildUIFeedPlan normalizes the native UI feed defaults while preserving the
// existing request semantics.
func BuildUIFeedPlan(index, length, speed, defaultLength, defaultSpeed int) ACECommandPlan {
	return BuildUIFeedCommandPlan(index, length, speed, defaultLength, defaultSpeed).ACECommandPlan
}

// BuildUIFeedCommandPlan normalizes the native UI feed defaults and returns
// the effective index used for the request.
func BuildUIFeedCommandPlan(index, length, speed, defaultLength, defaultSpeed int) ACEUICommandPlan {
	return buildUICommandPlan(index, length, speed, defaultLength, defaultSpeed, BuildFeedRequest)
}

// BuildUIUnwindPlan normalizes the native UI unwind defaults while preserving
// the existing request semantics.
func BuildUIUnwindPlan(index, length, speed, defaultLength, defaultSpeed int) ACECommandPlan {
	return BuildUIUnwindCommandPlan(index, -1, length, speed, defaultLength, defaultSpeed).ACECommandPlan
}

// BuildUIUnwindCommandPlan normalizes the native UI unwind defaults, applies
// the legacy feed-assist fallback index rules, and returns the effective index
// used for the request.
func BuildUIUnwindCommandPlan(index, feedAssistIndex, length, speed, defaultLength, defaultSpeed int) ACEUICommandPlan {
	if index < 0 {
		index = feedAssistIndex
		if index < 0 {
			index = 0
		}
	}
	return buildUICommandPlan(index, length, speed, defaultLength, defaultSpeed, BuildRetractRequest)
}

func buildFilamentMotionRequest(method string, index, length, speed int) map[string]interface{} {
	return BuildACERequestWithParams(method, map[string]interface{}{
		"index":  index,
		"length": length,
		"speed":  speed,
	})
}

// MotionCommandDelay preserves the existing ACE motion dwell timing behavior.
func MotionCommandDelay(length, speed int) float64 {
	if speed == 0 {
		return 0.1
	}
	return float64(length/speed) + 0.1
}

func buildStrictMotionPlan(builder func(int, int, int) map[string]interface{}, index, length, speed int) (ACECommandPlan, error) {
	if err := validateACESlotIndex(index); err != nil {
		return ACECommandPlan{}, err
	}
	if length <= 0 {
		return ACECommandPlan{}, fmt.Errorf("Wrong length")
	}
	if speed <= 0 {
		return ACECommandPlan{}, fmt.Errorf("Wrong speed")
	}
	return buildMotionPlan(builder(index, length, speed), length, speed), nil
}

func buildMotionPlan(request map[string]interface{}, length, speed int) ACECommandPlan {
	return ACECommandPlan{
		Request: request,
		Delay:   MotionCommandDelay(length, speed),
	}
}

func buildUICommandPlan(index, length, speed, defaultLength, defaultSpeed int, builder func(int, int, int) map[string]interface{}) ACEUICommandPlan {
	length = normalizePositive(length, defaultLength)
	speed = normalizePositive(speed, defaultSpeed)
	return ACEUICommandPlan{
		ACECommandPlan: buildMotionPlan(builder(index, length, speed), length, speed),
		EffectiveIndex: index,
	}
}

func normalizePositive(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func validateACESlotIndex(index int) error {
	if index < 0 || index >= aceSlotCount {
		return fmt.Errorf("Wrong index")
	}
	return nil
}
