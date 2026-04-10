package mcu

type WaitableCompletion interface {
	Complete(result interface{})
	Wait(waketime float64, waketimeResult interface{}) interface{}
}

type EndstopHomeRuntimeState struct {
	RestTicks         int64
	TriggerCompletion WaitableCompletion
}

type EndstopHomeStartPlan struct {
	Clock         int64
	SampleTicks   int64
	SampleCount   int64
	RestTicks     int64
	PinValue      int64
	TrsyncOID     int64
	TriggerReason int64
	ExpireTimeout float64
	ReportOffsets []float64
}

func (self *EndstopHomeRuntimeState) BuildHomeStartPlan(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64, invert int, trsyncCount int, trsyncOID int, triggerCompletion WaitableCompletion, printTimeToClock func(float64) int64, secondsToClock func(float64) int64, trsyncTimeout float64, trsyncSingleMCUTimeout float64, reasonEndstopHit int64) EndstopHomeStartPlan {
	clock := printTimeToClock(printTime)
	restTicks := printTimeToClock(printTime+restTime) - clock
	self.RestTicks = restTicks
	self.TriggerCompletion = triggerCompletion
	expireTimeout := trsyncTimeout
	if trsyncCount == 1 {
		expireTimeout = trsyncSingleMCUTimeout
	}
	var reportOffsets []float64
	if trsyncCount > 0 {
		reportOffsets = make([]float64, trsyncCount)
		for i := range reportOffsets {
			reportOffsets[i] = float64(i) / float64(trsyncCount)
		}
	}
	return EndstopHomeStartPlan{
		Clock:         clock,
		SampleTicks:   secondsToClock(sampleTime),
		SampleCount:   sampleCount,
		RestTicks:     restTicks,
		PinValue:      triggered ^ int64(invert),
		TrsyncOID:     int64(trsyncOID),
		TriggerReason: reasonEndstopHit,
		ExpireTimeout: expireTimeout,
		ReportOffsets: reportOffsets,
	}
}

func (self *EndstopHomeRuntimeState) WaitForTrigger(isFileoutput bool, waketime float64, waketimeResult interface{}) interface{} {
	if self.TriggerCompletion == nil {
		return waketimeResult
	}
	if isFileoutput {
		self.TriggerCompletion.Complete(true)
	}
	return self.TriggerCompletion.Wait(waketime, waketimeResult)
}

type EndstopHomeWaitDecision struct {
	Result     float64
	NeedsQuery bool
}

func EvaluateEndstopHomeWait(homeEndTime float64, stopReasons []int64, isFileoutput bool, reasonEndstopHit int64, reasonCommsTimeout int64) EndstopHomeWaitDecision {
	if len(stopReasons) == 0 {
		return EndstopHomeWaitDecision{}
	}
	timeoutCount := 0
	for _, reason := range stopReasons {
		if reason == reasonCommsTimeout {
			timeoutCount += 1
		}
	}
	if timeoutCount == len(stopReasons) {
		return EndstopHomeWaitDecision{Result: -1.}
	}
	if stopReasons[0] != reasonEndstopHit {
		return EndstopHomeWaitDecision{Result: 0.}
	}
	if isFileoutput {
		return EndstopHomeWaitDecision{Result: homeEndTime}
	}
	return EndstopHomeWaitDecision{NeedsQuery: true}
}

func (self *EndstopHomeRuntimeState) HomeEndTimeFromNextClock(nextClock32 int64, clock32ToClock64 func(int64) int64, clockToPrintTime func(int64) float64) float64 {
	nextClock := clock32ToClock64(nextClock32)
	return clockToPrintTime(nextClock - self.RestTicks)
}

func QueryEndstop(printTime float64, invert int, isFileoutput bool, printTimeToClock func(float64) int64, queryPinValue func(clock int64) int64) int {
	if isFileoutput {
		return 0
	}
	clock := printTimeToClock(printTime)
	return int(queryPinValue(clock) ^ int64(invert))
}
