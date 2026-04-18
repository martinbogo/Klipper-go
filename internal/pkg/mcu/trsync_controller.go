package mcu

import "time"

type TrsyncManagedStepper interface {
	EndstopRegistryStepper
	Get_oid() int
	Note_homing_end()
}

type TrsyncController struct {
	steppers []TrsyncManagedStepper
	state    TrsyncRuntimeState
}

func NewTrsyncController() *TrsyncController {
	return &TrsyncController{steppers: []TrsyncManagedStepper{}}
}

func (self *TrsyncController) AddStepper(stepper TrsyncManagedStepper) {
	for _, existing := range self.steppers {
		if existing == stepper {
			return
		}
	}
	self.steppers = append(self.steppers, stepper)
}

func (self *TrsyncController) Steppers() []TrsyncManagedStepper {
	steppers := make([]TrsyncManagedStepper, len(self.steppers))
	copy(steppers, self.steppers)
	return steppers
}

func (self *TrsyncController) RawSteppers() []interface{} {
	raw := make([]interface{}, len(self.steppers))
	for i, stepper := range self.steppers {
		raw[i] = stepper.Raw()
	}
	return raw
}

func (self *TrsyncController) RegistrySteppers() []EndstopRegistryStepper {
	adapted := make([]EndstopRegistryStepper, len(self.steppers))
	for i, stepper := range self.steppers {
		adapted[i] = stepper
	}
	return adapted
}

func (self *TrsyncController) CurrentCompletion() Completion {
	return self.state.TriggerCompletion
}

func (self *TrsyncController) Shutdown() {
	self.state.Shutdown()
}

func (self *TrsyncController) HandleState(params map[string]interface{}, clock32ToClock64 func(int64) int64, asyncComplete func(result map[string]interface{}), triggerPastEnd func()) bool {
	handled := self.state.HandleState(params, clock32ToClock64, asyncComplete, triggerPastEnd)
	return handled
}

func (self *TrsyncController) Start(oid int, printTime float64, reportOffset float64, triggerCompletion Completion, expireTimeout float64, printTimeToClock func(float64) int64, secondsToClock func(float64) int64, setupDispatch func(TrsyncStartPlan), registerStateResponse func(), sendStart func([]int64, int64, int64), sendStepperStop func(int), sendTimeout func([]int64, int64, int64)) {
	self.state.TriggerCompletion = triggerCompletion
	self.state.HomeEndClock = nil
	plan := BuildTrsyncStartPlan(printTime, reportOffset, expireTimeout, printTimeToClock, secondsToClock)
	if setupDispatch != nil {
		setupDispatch(plan)
	}
	if registerStateResponse != nil {
		registerStateResponse()
	}
	if sendStart != nil {
		sendStart([]int64{int64(oid), plan.ReportClock, plan.ReportTicks, ReasonCommsTimeout}, 0, plan.ReportClock)
	}
	if sendStepperStop != nil {
		for _, stepper := range self.steppers {
			sendStepperStop(stepper.Get_oid())
		}
	}
	if sendTimeout != nil {
		sendTimeout([]int64{int64(oid), plan.ExpireClock}, 0, plan.ExpireClock)
	}
}

func (self *TrsyncController) SetHomeEndTime(homeEndTime float64, printTimeToClock func(float64) int64) {
	self.state.SetHomeEndTime(homeEndTime, printTimeToClock)
}

func (self *TrsyncController) Stop(oid int, isFileoutput bool, unregisterStateResponse func(), queryTriggerReason func(int, int64) int64) int64 {
	if unregisterStateResponse != nil {
		unregisterStateResponse()
	}
	self.state.TriggerCompletion = nil
	if isFileoutput {
		return ReasonEndstopHit
	}
	result := ReasonHostRequest
	if queryTriggerReason != nil {
		result = queryTriggerReason(oid, ReasonHostRequest)
	}
	// Phase sync each stepper with delays to avoid thundering herd problem.
	// When multiple drivers share a single UART, simultaneous phase-sync
	// attempts cause UART framing errors. Add 100ms delay between syncs
	// to serialize writes to the shared bitbang UART.
	// Upstream Klipper avoids this by doing phase-sync during per-stepper enable,
	// not all-at-once during trsync stop. Future: migrate to that pattern.
	for i, stepper := range self.steppers {
		stepper.Note_homing_end()
		if i < len(self.steppers)-1 {
			// Sleep to ensure inter-stepper UART transaction spacing.
			// This prevents response framing errors from rapid writes.
			time.Sleep(100 * time.Millisecond)
		}
	}
	return result
}
