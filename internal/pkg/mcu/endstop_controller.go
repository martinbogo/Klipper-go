package mcu

type EndstopManagedTrsync interface {
	EndstopRegistryTrsync
	Add_stepper(stepper interface{})
	Get_oid() int
	Get_command_queue() interface{}
	Start(print_time float64, report_offset float64, trigger_completion Completion, expire_timeout float64)
	Set_home_end_time(home_end_time float64)
	Stop() interface{}
}

const (
	DefaultEndstopTrsyncTimeout          = 0.025
	DefaultEndstopSingleMCUTrsyncTimeout = 0.25
)

type EndstopHomeTimeouts struct {
	Trsync    float64
	SingleMCU float64
}

func DefaultEndstopHomeTimeoutValues() EndstopHomeTimeouts {
	return EndstopHomeTimeouts{
		Trsync:    DefaultEndstopTrsyncTimeout,
		SingleMCU: DefaultEndstopSingleMCUTrsyncTimeout,
	}
}

type EndstopHomeReasonCodes struct {
	EndstopHit   int64
	HostRequest  int64
	CommsTimeout int64
}

type EndstopHomeStartOps struct {
	TriggerCompletion WaitableCompletion
	PrintTimeToClock  func(float64) int64
	SecondsToClock    func(float64) int64
	StartDispatch     func(hostReason int64)
	SendHome          func([]int64, int64, int64)
}

type EndstopHomeWaitOps struct {
	IsFileoutput     bool
	Waketime         float64
	WaketimeResult   interface{}
	CancelHome       func()
	StopTrsync       func(EndstopManagedTrsync) int64
	QueryNextClock   func() int64
	Clock32ToClock64 func(int64) int64
	ClockToPrintTime func(int64) float64
}

type EndstopController struct {
	oid     int
	pin     interface{}
	pullup  interface{}
	invert  int
	state   EndstopHomeRuntimeState
	trsyncs []EndstopManagedTrsync
}

func NewEndstopController(oid int, pin interface{}, pullup interface{}, invert int, initialTrsync EndstopManagedTrsync) *EndstopController {
	controller := &EndstopController{
		oid:     oid,
		pin:     pin,
		pullup:  pullup,
		invert:  invert,
		trsyncs: []EndstopManagedTrsync{},
	}
	if initialTrsync != nil {
		controller.trsyncs = append(controller.trsyncs, initialTrsync)
	}
	return controller
}

func (self *EndstopController) ConfigPlan() EndstopConfigPlan {
	return BuildEndstopConfigPlan(self.oid, self.pin, self.pullup)
}

func (self *EndstopController) PrimaryTrsync() EndstopManagedTrsync {
	if len(self.trsyncs) == 0 {
		return nil
	}
	return self.trsyncs[0]
}

func (self *EndstopController) Trsyncs() []EndstopManagedTrsync {
	trsyncs := make([]EndstopManagedTrsync, len(self.trsyncs))
	copy(trsyncs, self.trsyncs)
	return trsyncs
}

func (self *EndstopController) registryTrsyncs() []EndstopRegistryTrsync {
	adapted := make([]EndstopRegistryTrsync, len(self.trsyncs))
	for i, trsync := range self.trsyncs {
		adapted[i] = trsync
	}
	return adapted
}

func (self *EndstopController) AddStepper(stepper EndstopRegistryStepper, newTrsync func() EndstopManagedTrsync) EndstopAddStepperPlan {
	plan := BuildEndstopAddStepperPlan(self.registryTrsyncs(), stepper)
	trsyncIndex := plan.TrsyncIndex
	if plan.NeedsNewTrsync && newTrsync != nil {
		created := newTrsync()
		if created != nil {
			self.trsyncs = append(self.trsyncs, created)
			trsyncIndex = len(self.trsyncs) - 1
		}
	}
	if trsyncIndex < len(self.trsyncs) {
		self.trsyncs[trsyncIndex].Add_stepper(stepper.Raw())
	}
	return plan
}

func (self *EndstopController) Steppers() []interface{} {
	return CollectEndstopSteppers(self.registryTrsyncs())
}

func (self *EndstopController) HomeStart(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64, timeouts EndstopHomeTimeouts, reasons EndstopHomeReasonCodes, ops EndstopHomeStartOps) WaitableCompletion {
	primary := self.PrimaryTrsync()
	if primary == nil {
		return nil
	}
	plan := self.state.BuildHomeStartPlan(printTime, sampleTime, sampleCount, restTime, triggered, self.invert, len(self.trsyncs), primary.Get_oid(), ops.TriggerCompletion, ops.PrintTimeToClock, ops.SecondsToClock, timeouts.Trsync, timeouts.SingleMCU, reasons.EndstopHit)
	for i, trsync := range self.trsyncs {
		trsync.Start(printTime, plan.ReportOffsets[i], self.state.TriggerCompletion, plan.ExpireTimeout)
	}
	if ops.StartDispatch != nil {
		ops.StartDispatch(reasons.HostRequest)
	}
	if ops.SendHome != nil {
		ops.SendHome([]int64{int64(self.oid), plan.Clock, plan.SampleTicks, plan.SampleCount, plan.RestTicks, plan.PinValue, plan.TrsyncOID, plan.TriggerReason}, 0, plan.Clock)
	}
	return self.state.TriggerCompletion
}

func (self *EndstopController) HomeWait(homeEndTime float64, reasons EndstopHomeReasonCodes, ops EndstopHomeWaitOps) float64 {
	primary := self.PrimaryTrsync()
	if primary == nil {
		return 0.
	}
	primary.Set_home_end_time(homeEndTime)
	self.state.WaitForTrigger(ops.IsFileoutput, ops.Waketime, ops.WaketimeResult)
	if ops.CancelHome != nil {
		ops.CancelHome()
	}
	stopReasons := make([]int64, 0, len(self.trsyncs))
	for _, trsync := range self.trsyncs {
		stopReasons = append(stopReasons, ops.StopTrsync(trsync))
	}
	decision := EvaluateEndstopHomeWait(homeEndTime, stopReasons, ops.IsFileoutput, reasons.EndstopHit, reasons.CommsTimeout)
	if !decision.NeedsQuery {
		return decision.Result
	}
	return self.state.HomeEndTimeFromNextClock(ops.QueryNextClock(), ops.Clock32ToClock64, ops.ClockToPrintTime)
}
