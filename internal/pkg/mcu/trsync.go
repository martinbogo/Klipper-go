package mcu

const (
	ReasonEndstopHit   int64 = 1
	ReasonHostRequest  int64 = 2
	ReasonPastEndTime  int64 = 3
	ReasonCommsTimeout int64 = 4
)

type Completion interface {
	Complete(result interface{})
}

type TrsyncRuntimeState struct {
	TriggerCompletion Completion
	HomeEndClock      *int64
}

type TrsyncStartPlan struct {
	Clock          int64
	ExpireTicks    int64
	ExpireClock    int64
	ReportTicks    int64
	ReportClock    int64
	MinExtendTicks int64
}

func (self *TrsyncRuntimeState) Shutdown() {
	if self.TriggerCompletion == nil {
		return
	}
	completion := self.TriggerCompletion
	self.TriggerCompletion = nil
	completion.Complete(false)
}

func (self *TrsyncRuntimeState) HandleState(params map[string]interface{}, clock32ToClock64 func(int64) int64, asyncComplete func(result map[string]interface{}), triggerPastEnd func()) bool {
	if canTrigger, ok := params["can_trigger"].(int64); ok && canTrigger != 1 {
		if self.TriggerCompletion != nil {
			reason, _ := params["trigger_reason"].(int64)
			self.TriggerCompletion = nil
			asyncComplete(map[string]interface{}{"aa": reason >= ReasonCommsTimeout})
		}
		return true
	}
	if self.HomeEndClock == nil {
		return false
	}
	clock := clock32ToClock64(params["clock"].(int64))
	if clock < *self.HomeEndClock {
		return false
	}
	self.HomeEndClock = nil
	triggerPastEnd()
	return true
}

func (self *TrsyncRuntimeState) SetHomeEndTime(homeEndTime float64, printTimeToClock func(float64) int64) {
	homeEndClock := printTimeToClock(homeEndTime)
	self.HomeEndClock = &homeEndClock
}

func BuildTrsyncStartPlan(printTime float64, reportOffset float64, expireTimeout float64, printTimeToClock func(float64) int64, secondsToClock func(float64) int64) TrsyncStartPlan {
	clock := printTimeToClock(printTime)
	expireTicks := secondsToClock(expireTimeout)
	reportTicks := secondsToClock(expireTimeout * 0.3)
	return TrsyncStartPlan{
		Clock:          clock,
		ExpireTicks:    expireTicks,
		ExpireClock:    clock + expireTicks,
		ReportTicks:    reportTicks,
		ReportClock:    clock + int64(float64(reportTicks)*reportOffset+0.5),
		MinExtendTicks: int64(float64(reportTicks)*0.8 + 0.5),
	}
}
