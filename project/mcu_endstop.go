package project

import (
	"goklipper/common/constants"
	"goklipper/common/utils/cast"
	"goklipper/internal/pkg/chelper"
	mcupkg "goklipper/internal/pkg/mcu"
	serialpkg "goklipper/internal/pkg/serialhdl"
	"log"
)

var TRSYNC_TIMEOUT = 0.025
var TRSYNC_SINGLE_MCU_TIMEOUT = 0.25

type MCU_endstop struct {
	Pin                interface{}
	Trdispatch         interface{}
	Home_cmd           *serialpkg.CommandWrapper
	Invert             int
	Trigger_completion *ReactorCompletion
	Mcu                *MCU
	Pullup             interface{}
	Trsyncs            []*MCU_trsync
	Oid                int
	Rest_ticks         int64
	Query_cmd          *serialpkg.CommandQueryWrapper
}

var RETRY_QUERY = 1.000

func NewMCU_endstop(mcu *MCU, pin_params map[string]interface{}) interface{} {
	self := MCU_endstop{}
	self.Mcu = mcu
	self.Pin = pin_params["pin"]
	self.Pullup = pin_params["pullup"]
	self.Invert = pin_params["invert"].(int)
	self.Oid = self.Mcu.Create_oid()
	self.Home_cmd = nil
	self.Query_cmd = nil
	self.Mcu.Register_config_callback(self.Build_config)
	self.Trigger_completion = nil
	self.Rest_ticks = 0
	self.Trdispatch = chelper.Trdispatch_alloc()
	//runtime.SetFinalizer(&self,self._MCU_endstop)
	self.Trsyncs = []*MCU_trsync{NewMCU_trsync(mcu, self.Trdispatch)}
	return &self
}
func (self *MCU_endstop) _MCU_endstop() {
	chelper.Free(self.Trdispatch)
}
func (self *MCU_endstop) Get_mcu() *MCU {
	return self.Mcu
}

func (self *MCU_endstop) runtimeState() mcupkg.EndstopHomeRuntimeState {
	return mcupkg.EndstopHomeRuntimeState{
		RestTicks:         self.Rest_ticks,
		TriggerCompletion: self.Trigger_completion,
	}
}

func (self *MCU_endstop) applyRuntimeState(state mcupkg.EndstopHomeRuntimeState) {
	self.Rest_ticks = state.RestTicks
	if state.TriggerCompletion == nil {
		self.Trigger_completion = nil
		return
	}
	self.Trigger_completion = state.TriggerCompletion.(*ReactorCompletion)
}

func (self *MCU_endstop) Add_stepper(_stepper interface{}) {
	stepper_ := _stepper.(*MCU_stepper)
	trsyncs := make([]mcupkg.EndstopRegistryTrsync, len(self.Trsyncs))
	for i, trsync := range self.Trsyncs {
		trsyncs[i] = trsync
	}
	plan := mcupkg.BuildEndstopAddStepperPlan(trsyncs, stepper_)
	trsyncIndex := plan.TrsyncIndex
	if plan.NeedsNewTrsync {
		newTrsync := NewMCU_trsync(stepper_.Get_mcu(), self.Trdispatch)
		self.Trsyncs = append(self.Trsyncs, newTrsync)
		trsyncIndex = len(self.Trsyncs) - 1
	}
	trsync := self.Trsyncs[trsyncIndex]
	trsync.Add_stepper(_stepper)
	if plan.SharedAxisConflict {
		log.Print("Multi-mcu homing not supported on" +
			" multi-mcu shared axis")
	}
}
func (self *MCU_endstop) Get_steppers() []interface{} {
	trsyncs := make([]mcupkg.EndstopRegistryTrsync, len(self.Trsyncs))
	for i, trsync := range self.Trsyncs {
		trsyncs[i] = trsync
	}
	return mcupkg.CollectEndstopSteppers(trsyncs)
}
func (self *MCU_endstop) Build_config() {
	plan := mcupkg.BuildEndstopConfigPlan(self.Oid, self.Pin, self.Pullup)
	for _, cmd := range plan.ConfigCmds {
		self.Mcu.Add_config_cmd(cmd, false, false)
	}
	for _, cmd := range plan.RestartCmds {
		self.Mcu.Add_config_cmd(cmd, false, true)
	}
	//Lookup commands
	cmd_queue := self.Trsyncs[0].Get_command_queue()
	self.Home_cmd, _ = self.Mcu.Lookup_command(plan.HomeLookupFormat, cmd_queue)
	self.Query_cmd = self.Mcu.Lookup_query_command(plan.QueryRequestFormat, plan.QueryResponseFormat, self.Oid, cmd_queue, false)
}
func (self *MCU_endstop) Home_start(print_time float64, sample_time float64, sample_count int64, rest_time float64, triggered int64) interface{} {
	state := self.runtimeState()
	reactor := self.Mcu.Get_printer().Get_reactor()
	plan := state.BuildHomeStartPlan(print_time, sample_time, sample_count, rest_time, triggered, self.Invert, len(self.Trsyncs), self.Trsyncs[0].Get_oid(), reactor.Completion(), self.Mcu.Print_time_to_clock, self.Mcu.Seconds_to_clock, TRSYNC_TIMEOUT, TRSYNC_SINGLE_MCU_TIMEOUT, REASON_ENDSTOP_HIT)
	self.applyRuntimeState(state)
	for i, trsync := range self.Trsyncs {
		trsync.Start(print_time, plan.ReportOffsets[i], self.Trigger_completion, plan.ExpireTimeout)
	}

	chelper.Trdispatch_start(self.Trdispatch, uint32(REASON_HOST_REQUEST))
	self.Home_cmd.Send([]int64{int64(self.Oid), plan.Clock, plan.SampleTicks,
		plan.SampleCount, plan.RestTicks, plan.PinValue,
		plan.TrsyncOID, plan.TriggerReason}, 0, plan.Clock)
	return self.Trigger_completion
}
func (self *MCU_endstop) Home_wait(home_end_time float64) float64 {
	state := self.runtimeState()
	etrsync := (self.Trsyncs)[0]
	etrsync.Set_home_end_time(home_end_time)
	state.WaitForTrigger(self.Mcu.Is_fileoutput(), constants.NEVER, nil)
	self.Home_cmd.Send([]int64{int64(self.Oid), 0, 0, 0, 0, 0, 0, 0}, 0, 0)
	chelper.Trdispatch_stop(self.Trdispatch)

	res := []int64{}
	for _, trsync := range self.Trsyncs {
		res = append(res, cast.ToInt64(trsync.Stop()))
	}
	decision := mcupkg.EvaluateEndstopHomeWait(home_end_time, res, self.Mcu.Is_fileoutput(), REASON_ENDSTOP_HIT, REASON_COMMS_TIMEOUT)
	if !decision.NeedsQuery {
		return decision.Result
	}
	params := self.Query_cmd.Send([]int64{int64(self.Oid)}, 0, 0).(map[string]interface{})
	return state.HomeEndTimeFromNextClock(params["next_clock"].(int64), self.Mcu.Clock32_to_clock64, self.Mcu.Clock_to_print_time)
}

func (self *MCU_endstop) Query_endstop(print_time float64) int {
	return mcupkg.QueryEndstop(print_time, self.Invert, self.Mcu.Is_fileoutput(), self.Mcu.Print_time_to_clock, func(clock int64) int64 {
		params := self.Query_cmd.Send([]int64{int64(self.Oid)}, clock, 0)
		return params.(map[string]interface{})["pin_value"].(int64)
	})
}
