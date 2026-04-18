package mcu

type TrsyncCommandSender interface {
	Send(data interface{}, minclock, reqclock int64)
}

type TrsyncQuerySender interface {
	Send(data interface{}, minclock, reqclock int64) interface{}
}

type TrsyncCommandRuntime struct {
	oid                     int
	setupDispatch           func(TrsyncStartPlan)
	registerStateResponse   func()
	unregisterStateResponse func()
	startSender             TrsyncCommandSender
	setTimeoutSender        TrsyncCommandSender
	triggerSender           TrsyncCommandSender
	stepperStopSender       TrsyncCommandSender
	querySender             TrsyncQuerySender
}

func NewTrsyncCommandRuntime(oid int) *TrsyncCommandRuntime {
	return &TrsyncCommandRuntime{oid: oid}
}

func (self *TrsyncCommandRuntime) Configure(setupDispatch func(TrsyncStartPlan), registerStateResponse func(), unregisterStateResponse func(), startSender TrsyncCommandSender, setTimeoutSender TrsyncCommandSender, triggerSender TrsyncCommandSender, stepperStopSender TrsyncCommandSender, querySender TrsyncQuerySender) {
	self.setupDispatch = setupDispatch
	self.registerStateResponse = registerStateResponse
	self.unregisterStateResponse = unregisterStateResponse
	self.startSender = startSender
	self.setTimeoutSender = setTimeoutSender
	self.triggerSender = triggerSender
	self.stepperStopSender = stepperStopSender
	self.querySender = querySender
}

func (self *TrsyncCommandRuntime) SetupDispatch(plan TrsyncStartPlan) {
	if self != nil && self.setupDispatch != nil {
		self.setupDispatch(plan)
	}
}

func (self *TrsyncCommandRuntime) RegisterStateResponse() {
	if self != nil && self.registerStateResponse != nil {
		self.registerStateResponse()
	}
}

func (self *TrsyncCommandRuntime) UnregisterStateResponse() {
	if self != nil && self.unregisterStateResponse != nil {
		self.unregisterStateResponse()
	}
}

func (self *TrsyncCommandRuntime) SendStart(args []int64, minclock int64, reqclock int64) {
	if self != nil && self.startSender != nil {
		self.startSender.Send(args, minclock, reqclock)
	}
}

func (self *TrsyncCommandRuntime) SendStepperStop(stepperOID int) {
	if self != nil && self.stepperStopSender != nil {
		self.stepperStopSender.Send([]int64{int64(stepperOID), int64(self.oid)}, 0, 0)
	}
}

func (self *TrsyncCommandRuntime) SendTimeout(args []int64, minclock int64, reqclock int64) {
	if self != nil && self.setTimeoutSender != nil {
		self.setTimeoutSender.Send(args, minclock, reqclock)
	}
}

func (self *TrsyncCommandRuntime) TriggerPastEnd() {
	if self != nil && self.triggerSender != nil {
		self.triggerSender.Send([]int64{int64(self.oid), ReasonPastEndTime}, 0, 0)
	}
}

func (self *TrsyncCommandRuntime) QueryTriggerReason(hostReason int64) int64 {
	if self == nil || self.querySender == nil {
		return hostReason
	}
	res := self.querySender.Send([]int64{int64(self.oid), hostReason}, 0, 0)
	if params, ok := res.(map[string]interface{}); ok {
		if val, ok := params["trigger_reason"].(int64); ok {
			return val
		}
	}
	return hostReason
}
