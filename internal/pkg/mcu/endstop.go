package mcu

import (
	"goklipper/common/constants"
	"goklipper/common/utils/cast"
	"log"
)

type EndstopControllerMCU interface {
	Create_oid() int
	Register_config_callback(cb interface{})
	Add_config_cmd(cmd string, is_init bool, on_restart bool)
	LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error)
	LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{}
	Is_fileoutput() bool
	Print_time_to_clock(print_time float64) int64
	Seconds_to_clock(time float64) int64
	Clock32_to_clock64(clock32 int64) int64
	Clock_to_print_time(clock int64) float64
}

type EndstopCommandSender interface {
	Send(data interface{}, minclock, reqclock int64)
}

type EndstopQuerySender interface {
	Send(data interface{}, minclock, reqclock int64) interface{}
}

type LegacyEndstop struct {
	Pin               interface{}
	Trdispatch        interface{}
	Home_cmd          EndstopCommandSender
	Invert            int
	controller        EndstopControllerMCU
	Pullup            interface{}
	Oid               int
	Query_cmd         EndstopQuerySender
	core              *EndstopController
	completionFactory func() WaitableCompletion
	newTrsync         func(stepper interface{}, trdispatch interface{}) EndstopManagedTrsync
	startDispatch     func(trdispatch interface{}, hostReason int64)
	stopDispatch      func(trdispatch interface{})
}

func NewLegacyEndstop(mcu EndstopControllerMCU, pinParams map[string]interface{}, trdispatch interface{}, initialTrsync EndstopManagedTrsync,
	completionFactory func() WaitableCompletion, newTrsync func(stepper interface{}, trdispatch interface{}) EndstopManagedTrsync,
	startDispatch func(trdispatch interface{}, hostReason int64), stopDispatch func(trdispatch interface{})) *LegacyEndstop {
	self := &LegacyEndstop{
		Pin:               pinParams["pin"],
		Trdispatch:        trdispatch,
		Home_cmd:          nil,
		Invert:            pinParams["invert"].(int),
		controller:        mcu,
		Pullup:            pinParams["pullup"],
		Oid:               mcu.Create_oid(),
		Query_cmd:         nil,
		completionFactory: completionFactory,
		newTrsync:         newTrsync,
		startDispatch:     startDispatch,
		stopDispatch:      stopDispatch,
	}
	mcu.Register_config_callback(self.Build_config)
	self.core = NewEndstopController(self.Oid, self.Pin, self.Pullup, self.Invert, initialTrsync)
	return self
}

type ManagedLegacyEndstopTrsyncBuilder func(mcuKey interface{}, trdispatch interface{}) EndstopManagedTrsync

func NewManagedLegacyEndstop(mcu EndstopControllerMCU, pinParams map[string]interface{}, trdispatch interface{}, buildManagedTrsync ManagedLegacyEndstopTrsyncBuilder,
	completionFactory func() WaitableCompletion, startDispatch func(trdispatch interface{}, hostReason int64), stopDispatch func(trdispatch interface{})) *LegacyEndstop {
	if buildManagedTrsync == nil {
		panic("managed legacy endstop requires trsync builder")
	}
	return NewLegacyEndstop(
		mcu,
		pinParams,
		trdispatch,
		buildManagedTrsync(mcu, trdispatch),
		completionFactory,
		func(stepper interface{}, trdispatch interface{}) EndstopManagedTrsync {
			registryStepper, ok := stepper.(EndstopRegistryStepper)
			if !ok {
				panic("endstop stepper does not implement registry interface")
			}
			return buildManagedTrsync(registryStepper.MCUKey(), trdispatch)
		},
		startDispatch,
		stopDispatch,
	)
}

func (self *LegacyEndstop) Add_stepper(stepper interface{}) {
	registryStepper, ok := stepper.(EndstopRegistryStepper)
	if !ok {
		panic("endstop stepper does not implement registry interface")
	}
	plan := self.core.AddStepper(registryStepper, func() EndstopManagedTrsync {
		if self.newTrsync == nil {
			return nil
		}
		return self.newTrsync(stepper, self.Trdispatch)
	})
	if warning := plan.WarningMessage(); warning != "" {
		log.Print(warning)
	}
}

func (self *LegacyEndstop) Get_steppers() []interface{} {
	return self.core.Steppers()
}

func (self *LegacyEndstop) MCUKey() interface{} {
	return self.controller
}

func (self *LegacyEndstop) Build_config() {
	plan := self.core.ConfigPlan()
	for _, cmd := range plan.ConfigCmds {
		self.controller.Add_config_cmd(cmd, false, false)
	}
	for _, cmd := range plan.RestartCmds {
		self.controller.Add_config_cmd(cmd, false, true)
	}
	cmdQueue := self.core.PrimaryTrsync().Get_command_queue()
	command, err := self.controller.LookupCommandRaw(plan.HomeLookupFormat, cmdQueue)
	if err != nil {
		panic(err)
	}
	homeCmd, ok := command.(EndstopCommandSender)
	if !ok {
		panic("endstop home command sender has unexpected type")
	}
	queryCmd, ok := self.controller.LookupQueryCommand(plan.QueryRequestFormat, plan.QueryResponseFormat, self.Oid, cmdQueue, false).(EndstopQuerySender)
	if !ok {
		panic("endstop query sender has unexpected type")
	}
	self.Home_cmd = homeCmd
	self.Query_cmd = queryCmd
}

func (self *LegacyEndstop) Home_start(print_time float64, sample_time float64, sample_count int64, rest_time float64, triggered int64) interface{} {
	return self.core.HomeStart(print_time, sample_time, sample_count, rest_time, triggered, DefaultEndstopHomeTimeoutValues(), EndstopHomeReasonCodes{EndstopHit: ReasonEndstopHit, HostRequest: ReasonHostRequest, CommsTimeout: ReasonCommsTimeout}, EndstopHomeStartOps{
		TriggerCompletion: self.completionFactory(),
		PrintTimeToClock:  self.controller.Print_time_to_clock,
		SecondsToClock:    self.controller.Seconds_to_clock,
		StartDispatch: func(hostReason int64) {
			if self.startDispatch != nil {
				self.startDispatch(self.Trdispatch, hostReason)
			}
		},
		SendHome: func(args []int64, minclock int64, reqclock int64) {
			self.Home_cmd.Send(args, minclock, reqclock)
		},
	})
}

func (self *LegacyEndstop) Home_wait(home_end_time float64) float64 {
	return self.core.HomeWait(home_end_time, EndstopHomeReasonCodes{EndstopHit: ReasonEndstopHit, CommsTimeout: ReasonCommsTimeout}, EndstopHomeWaitOps{
		IsFileoutput:   self.controller.Is_fileoutput(),
		Waketime:       constants.NEVER,
		WaketimeResult: nil,
		CancelHome: func() {
			self.Home_cmd.Send([]int64{int64(self.Oid), 0, 0, 0, 0, 0, 0, 0}, 0, 0)
			if self.stopDispatch != nil {
				self.stopDispatch(self.Trdispatch)
			}
		},
		StopTrsync: func(trsync EndstopManagedTrsync) int64 {
			return cast.ToInt64(trsync.Stop())
		},
		QueryNextClock: func() int64 {
			res := self.Query_cmd.Send([]int64{int64(self.Oid)}, 0, 0)
			if params, ok := res.(map[string]interface{}); ok {
				if next, ok := params["next_clock"].(int64); ok {
					return next
				}
			}
			return 0
		},
		Clock32ToClock64: self.controller.Clock32_to_clock64,
		ClockToPrintTime: self.controller.Clock_to_print_time,
	})
}

func (self *LegacyEndstop) Query_endstop(print_time float64) int {
	return QueryEndstop(print_time, self.Invert, self.controller.Is_fileoutput(), self.controller.Print_time_to_clock, func(clock int64) int64 {
		res := self.Query_cmd.Send([]int64{int64(self.Oid)}, clock, 0)
		if params, ok := res.(map[string]interface{}); ok {
			if pinVal, ok := params["pin_value"].(int64); ok {
				return pinVal
			}
		}
		return 0
	})
}
