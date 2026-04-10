package project

import (
	"goklipper/internal/pkg/chelper"
	mcupkg "goklipper/internal/pkg/mcu"
	serialpkg "goklipper/internal/pkg/serialhdl"
)

type MCU_trsync struct {
	Trdispatch             interface{}
	Trsync_start_cmd       *serialpkg.CommandWrapper
	Mcu                    *MCU
	Trsync_trigger_cmd     *serialpkg.CommandWrapper
	StepperList            []interface{}
	Stepper_stop_cmd       *serialpkg.CommandWrapper
	Home_end_clock         interface{}
	Trdispatch_mcu         interface{}
	Cmd_queue              interface{}
	Reactor                IReactor
	Oid                    int
	Trigger_completion     *ReactorCompletion
	Trsync_set_timeout_cmd *serialpkg.CommandWrapper
	Trsync_query_cmd       *serialpkg.CommandQueryWrapper
}

var REASON_ENDSTOP_HIT int64 = mcupkg.ReasonEndstopHit
var REASON_HOST_REQUEST int64 = mcupkg.ReasonHostRequest
var REASON_PAST_END_TIME int64 = mcupkg.ReasonPastEndTime
var REASON_COMMS_TIMEOUT int64 = mcupkg.ReasonCommsTimeout

func NewMCU_trsync(mcu *MCU, trdispatch interface{}) *MCU_trsync {
	self := MCU_trsync{}
	self.Mcu = mcu
	self.Trdispatch = trdispatch
	self.Reactor = mcu.Get_printer().Get_reactor()
	self.StepperList = []interface{}{}
	self.Trdispatch_mcu = nil
	self.Oid = mcu.Create_oid()
	self.Cmd_queue = mcu.Alloc_command_queue()
	self.Trsync_start_cmd = nil
	self.Trsync_set_timeout_cmd = self.Trsync_start_cmd
	self.Trsync_trigger_cmd = nil
	self.Trsync_query_cmd = nil
	self.Stepper_stop_cmd = nil
	self.Trigger_completion = nil
	self.Home_end_clock = nil
	mcu.Register_config_callback(self.Build_config)
	printer := mcu.Get_printer()
	printer.Register_event_handler("project:shutdown", self.Shutdown)
	return &self
}

func (self *MCU_trsync) Get_mcu() *MCU {

	return self.Mcu
}
func (self *MCU_trsync) Get_oid() int {

	return self.Oid
}
func (self *MCU_trsync) Get_command_queue() interface{} {

	return self.Cmd_queue
}
func (self *MCU_trsync) Add_stepper(stepper interface{}) {

	for _, o := range self.StepperList {
		if o == stepper {
			return
		}
	}
	self.StepperList = append(self.StepperList, stepper)

}
func (self *MCU_trsync) Get_steppers() []interface{} {
	steppers_back := make([]interface{}, len(self.StepperList))
	copy(steppers_back, self.StepperList)
	return steppers_back
}
func (self *MCU_trsync) Build_config() {

	mcu := self.Mcu
	plan := mcupkg.BuildTrsyncConfigPlan(self.Oid)
	for _, cmd := range plan.ConfigCmds {
		mcu.Add_config_cmd(cmd, false, false)
	}
	for _, cmd := range plan.RestartCmds {
		mcu.Add_config_cmd(cmd, false, true)
	}
	//Lookup commands
	self.Trsync_start_cmd, _ = mcu.Lookup_command(plan.StartLookupFormat, self.Cmd_queue)
	self.Trsync_set_timeout_cmd, _ = mcu.Lookup_command(plan.SetTimeoutFormat, self.Cmd_queue)
	self.Trsync_trigger_cmd, _ = mcu.Lookup_command(plan.TriggerFormat, self.Cmd_queue)
	self.Trsync_query_cmd = mcu.Lookup_query_command(plan.QueryRequestFormat, plan.QueryResponseFormat, self.Oid, self.Cmd_queue, false)
	self.Stepper_stop_cmd, _ = mcu.Lookup_command(plan.StepperStopFormat, self.Cmd_queue)
	// Create trdispatch_mcu object
	set_timeout_tag := mcu.Lookup_command_tag(plan.SetTimeoutTagFormat)
	trigger_tag := mcu.Lookup_command_tag(plan.TriggerTagFormat)
	state_tag := mcu.Lookup_command_tag(plan.StateTagFormat)

	self.Trdispatch_mcu = chelper.Trdispatch_mcu_alloc(self.Trdispatch, mcu.Serial.Serialqueue,
		self.Cmd_queue, self.Oid, uint32(set_timeout_tag.(int)), uint32(trigger_tag.(int)),
		uint32(state_tag.(int)))
}
func (self *MCU_trsync) _MCU_trsync() {
	chelper.Free(self.Trdispatch_mcu)
}

func (self *MCU_trsync) runtimeState() mcupkg.TrsyncRuntimeState {
	state := mcupkg.TrsyncRuntimeState{TriggerCompletion: self.Trigger_completion}
	if self.Home_end_clock != nil {
		clock := self.Home_end_clock.(int64)
		state.HomeEndClock = &clock
	}
	return state
}

func (self *MCU_trsync) applyRuntimeState(state mcupkg.TrsyncRuntimeState) {
	if state.TriggerCompletion == nil {
		self.Trigger_completion = nil
	} else {
		self.Trigger_completion = state.TriggerCompletion.(*ReactorCompletion)
	}
	if state.HomeEndClock == nil {
		self.Home_end_clock = nil
		return
	}
	self.Home_end_clock = *state.HomeEndClock
}

func (self *MCU_trsync) Shutdown([]interface{}) error {
	state := self.runtimeState()
	state.Shutdown()
	self.applyRuntimeState(state)
	return nil
}
func (self *MCU_trsync) Handle_trsync_state(params map[string]interface{}) error {
	state := self.runtimeState()
	currentCompletion := self.Trigger_completion
	handled := state.HandleState(params, self.Mcu.Clock32_to_clock64, func(result map[string]interface{}) {
		if currentCompletion != nil {
			self.Reactor.Async_complete(currentCompletion, result)
		}
	}, func() {
		self.Trsync_trigger_cmd.Send([]int64{int64(self.Oid), REASON_PAST_END_TIME}, 0, 0)
	})
	self.applyRuntimeState(state)
	if !handled {
		return nil
	}
	return nil
}
func (self *MCU_trsync) Start(print_time float64, report_offset float64, trigger_completion *ReactorCompletion, expire_timeout float64) {
	state := self.runtimeState()
	state.TriggerCompletion = trigger_completion
	state.HomeEndClock = nil
	self.applyRuntimeState(state)
	plan := mcupkg.BuildTrsyncStartPlan(print_time, report_offset, expire_timeout, self.Mcu.Print_time_to_clock, self.Mcu.Seconds_to_clock)
	chelper.Trdispatch_mcu_setup(self.Trdispatch_mcu, uint64(plan.Clock), uint64(plan.ExpireClock), uint64(plan.ExpireTicks), uint64(plan.MinExtendTicks))
	self.Mcu.Register_response(self.Handle_trsync_state, "trsync_state", self.Oid)
	self.Trsync_start_cmd.Send([]int64{int64(self.Oid), plan.ReportClock, plan.ReportTicks,
		REASON_COMMS_TIMEOUT}, 0, plan.ReportClock)
	for _, s := range self.StepperList {
		self.Stepper_stop_cmd.Send([]int64{int64(s.(*MCU_stepper).Get_oid()), int64(self.Oid)}, 0, 0)
	}
	self.Trsync_set_timeout_cmd.Send([]int64{int64(self.Oid), plan.ExpireClock}, 0, plan.ExpireClock)
}
func (self *MCU_trsync) Set_home_end_time(home_end_time float64) {
	state := self.runtimeState()
	state.SetHomeEndTime(home_end_time, self.Mcu.Print_time_to_clock)
	self.applyRuntimeState(state)
}
func (self *MCU_trsync) Stop() interface{} {

	self.Mcu.Register_response(nil, "trsync_state", self.Oid)
	self.Trigger_completion = nil
	if self.Mcu.Is_fileoutput() {
		return REASON_ENDSTOP_HIT
	}
	params := self.Trsync_query_cmd.Send([]int64{int64(self.Oid), REASON_HOST_REQUEST}, 0, 0)
	for _, s := range self.StepperList {
		s.(*MCU_stepper).Note_homing_end()
	}
	return params.(map[string]interface{})["trigger_reason"]
}

func (self *MCU_trsync) MCUKey() interface{} {
	return self.Get_mcu()
}

func (self *MCU_trsync) Steppers() []mcupkg.EndstopRegistryStepper {
	rawSteppers := self.Get_steppers()
	adapted := make([]mcupkg.EndstopRegistryStepper, len(rawSteppers))
	for i, stepper := range rawSteppers {
		adapted[i] = stepper.(*MCU_stepper)
	}
	return adapted
}

var _ mcupkg.EndstopRegistryTrsync = (*MCU_trsync)(nil)
