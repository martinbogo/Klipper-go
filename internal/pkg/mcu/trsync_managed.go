package mcu

import "fmt"

type ManagedTrsyncRuntimeHost interface {
	CreateOID() int
	AllocCommandQueue() interface{}
	AddConfigCmd(cmd string, isInit bool, onRestart bool)
	LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error)
	LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{}
	LookupCommandTag(msgformat string) interface{}
	RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{})
	Clock32ToClock64(int64) int64
	PrintTimeToClock(float64) int64
	SecondsToClock(float64) int64
	IsFileoutput() bool
	RegisterConfigCallback(func())
}

type ManagedTrsyncRuntimeFactoryHooks struct {
	RegisterShutdownHandler func(func([]interface{}) error)
	CreateDispatch          func(tags TrsyncDispatchTags) interface{}
	SetupDispatch           func(handle interface{}, plan TrsyncStartPlan)
	AsyncComplete           func(completion Completion, result map[string]interface{})
}

type ManagedTrsyncBindings struct {
	AddConfigCmd       func(cmd string, isInit bool, onRestart bool)
	LookupCommand      func(msgformat string, cq interface{}) (TrsyncCommandSender, error)
	LookupQueryCommand func(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) TrsyncQuerySender
	LookupCommandTag   func(msgformat string) interface{}
	SetStateResponse   func(handler func(map[string]interface{}) error)
	CreateDispatch     func(tags TrsyncDispatchTags) interface{}
	SetupDispatch      func(handle interface{}, plan TrsyncStartPlan)
	AsyncComplete      func(completion Completion, result map[string]interface{})
	Clock32ToClock64   func(int64) int64
	PrintTimeToClock   func(float64) int64
	SecondsToClock     func(float64) int64
	IsFileoutput       func() bool
}

type ManagedTrsyncRuntimeHooks struct {
	MCUKey                  interface{}
	CreateOID               func() int
	AllocCommandQueue       func() interface{}
	BuildBindings           func(oid int, cmdQueue interface{}) ManagedTrsyncBindings
	RegisterConfigCallback  func(func())
	RegisterShutdownHandler func(func([]interface{}) error)
}

type TrsyncDispatchTags struct {
	CmdQueue      interface{}
	OID           int
	SetTimeoutTag uint32
	TriggerTag    uint32
	StateTag      uint32
}

type ManagedTrsync struct {
	mcuKey         interface{}
	oid            int
	cmdQueue       interface{}
	bindings       ManagedTrsyncBindings
	core           *TrsyncController
	commands       *TrsyncCommandRuntime
	dispatchHandle interface{}
}

func NewManagedTrsync(mcuKey interface{}, oid int, cmdQueue interface{}, bindings ManagedTrsyncBindings) *ManagedTrsync {
	return &ManagedTrsync{
		mcuKey:   mcuKey,
		oid:      oid,
		cmdQueue: cmdQueue,
		bindings: bindings,
		core:     NewTrsyncController(),
		commands: NewTrsyncCommandRuntime(oid),
	}
}

func NewManagedTrsyncRuntime(hooks ManagedTrsyncRuntimeHooks) *ManagedTrsync {
	oid := hooks.CreateOID()
	cmdQueue := hooks.AllocCommandQueue()
	runtime := NewManagedTrsync(hooks.MCUKey, oid, cmdQueue, hooks.BuildBindings(oid, cmdQueue))
	if hooks.RegisterConfigCallback != nil {
		hooks.RegisterConfigCallback(runtime.Build_config)
	}
	if hooks.RegisterShutdownHandler != nil {
		hooks.RegisterShutdownHandler(runtime.Shutdown)
	}
	return runtime
}

func NewManagedTrsyncRuntimeForHost(host ManagedTrsyncRuntimeHost, hooks ManagedTrsyncRuntimeFactoryHooks) *ManagedTrsync {
	return NewManagedTrsyncRuntime(ManagedTrsyncRuntimeHooks{
		MCUKey:            host,
		CreateOID:         host.CreateOID,
		AllocCommandQueue: host.AllocCommandQueue,
		BuildBindings: func(oid int, cmdQueue interface{}) ManagedTrsyncBindings {
			return ManagedTrsyncBindings{
				AddConfigCmd: host.AddConfigCmd,
				LookupCommand: func(msgformat string, cq interface{}) (TrsyncCommandSender, error) {
					command, err := host.LookupCommandRaw(msgformat, cq)
					if err != nil {
						return nil, err
					}
					sender, ok := command.(TrsyncCommandSender)
					if !ok {
						return nil, fmt.Errorf("mcu: unexpected trsync command sender %T", command)
					}
					return sender, nil
				},
				LookupQueryCommand: func(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) TrsyncQuerySender {
					query, _ := host.LookupQueryCommand(msgformat, respformat, oid, cq, isAsync).(TrsyncQuerySender)
					return query
				},
				LookupCommandTag: host.LookupCommandTag,
				SetStateResponse: func(handler func(map[string]interface{}) error) {
					host.RegisterResponse(handler, "trsync_state", oid)
				},
				CreateDispatch: hooks.CreateDispatch,
				SetupDispatch:  hooks.SetupDispatch,
				AsyncComplete: func(completion Completion, result map[string]interface{}) {
					if hooks.AsyncComplete != nil {
						hooks.AsyncComplete(completion, result)
						return
					}
					completion.Complete(result)
				},
				Clock32ToClock64: host.Clock32ToClock64,
				PrintTimeToClock: host.PrintTimeToClock,
				SecondsToClock:   host.SecondsToClock,
				IsFileoutput:     host.IsFileoutput,
			}
		},
		RegisterConfigCallback: host.RegisterConfigCallback,
		RegisterShutdownHandler: func(handler func([]interface{}) error) {
			if hooks.RegisterShutdownHandler != nil {
				hooks.RegisterShutdownHandler(handler)
			}
		},
	})
}

func (self *ManagedTrsync) Get_oid() int {
	return self.oid
}

func (self *ManagedTrsync) Get_command_queue() interface{} {
	return self.cmdQueue
}

func (self *ManagedTrsync) Add_stepper(stepper interface{}) {
	self.core.AddStepper(stepper.(TrsyncManagedStepper))
}

func (self *ManagedTrsync) Get_steppers() []interface{} {
	return self.core.RawSteppers()
}

func (self *ManagedTrsync) Build_config() {
	plan := BuildTrsyncConfigPlan(self.oid)
	for _, cmd := range plan.ConfigCmds {
		self.bindings.AddConfigCmd(cmd, false, false)
	}
	for _, cmd := range plan.RestartCmds {
		self.bindings.AddConfigCmd(cmd, false, true)
	}
	startCmd, _ := self.bindings.LookupCommand(plan.StartLookupFormat, self.cmdQueue)
	setTimeoutCmd, _ := self.bindings.LookupCommand(plan.SetTimeoutFormat, self.cmdQueue)
	triggerCmd, _ := self.bindings.LookupCommand(plan.TriggerFormat, self.cmdQueue)
	queryCmd := self.bindings.LookupQueryCommand(plan.QueryRequestFormat, plan.QueryResponseFormat, self.oid, self.cmdQueue, false)
	stepperStopCmd, _ := self.bindings.LookupCommand(plan.StepperStopFormat, self.cmdQueue)
	self.dispatchHandle = self.bindings.CreateDispatch(TrsyncDispatchTags{
		CmdQueue:      self.cmdQueue,
		OID:           self.oid,
		SetTimeoutTag: commandTagUint32(self.bindings.LookupCommandTag(plan.SetTimeoutTagFormat)),
		TriggerTag:    commandTagUint32(self.bindings.LookupCommandTag(plan.TriggerTagFormat)),
		StateTag:      commandTagUint32(self.bindings.LookupCommandTag(plan.StateTagFormat)),
	})
	self.commands.Configure(func(plan TrsyncStartPlan) {
		self.bindings.SetupDispatch(self.dispatchHandle, plan)
	}, func() {
		self.bindings.SetStateResponse(self.Handle_trsync_state)
	}, func() {
		self.bindings.SetStateResponse(nil)
	}, startCmd, setTimeoutCmd, triggerCmd, stepperStopCmd, queryCmd)
}

func (self *ManagedTrsync) Shutdown([]interface{}) error {
	self.core.Shutdown()
	return nil
}

func (self *ManagedTrsync) Handle_trsync_state(params map[string]interface{}) error {
	currentCompletion := self.core.CurrentCompletion()
	self.core.HandleState(params, self.bindings.Clock32ToClock64, func(result map[string]interface{}) {
		if currentCompletion != nil {
			self.bindings.AsyncComplete(currentCompletion, result)
		}
	}, self.commands.TriggerPastEnd)
	return nil
}

func (self *ManagedTrsync) Start(print_time float64, report_offset float64, trigger_completion Completion, expire_timeout float64) {
	self.core.Start(self.oid, print_time, report_offset, trigger_completion, expire_timeout, self.bindings.PrintTimeToClock, self.bindings.SecondsToClock, self.commands.SetupDispatch, self.commands.RegisterStateResponse, self.commands.SendStart, self.commands.SendStepperStop, self.commands.SendTimeout)
}

func (self *ManagedTrsync) Set_home_end_time(home_end_time float64) {
	self.core.SetHomeEndTime(home_end_time, self.bindings.PrintTimeToClock)
}

func (self *ManagedTrsync) Stop() interface{} {
	return self.core.Stop(self.oid, self.bindings.IsFileoutput(), self.commands.UnregisterStateResponse, func(oid int, hostReason int64) int64 {
		_ = oid
		return self.commands.QueryTriggerReason(hostReason)
	})
}

func (self *ManagedTrsync) MCUKey() interface{} {
	return self.mcuKey
}

func (self *ManagedTrsync) Steppers() []EndstopRegistryStepper {
	return self.core.RegistrySteppers()
}

func commandTagUint32(value interface{}) uint32 {
	switch tag := value.(type) {
	case int:
		return uint32(tag)
	case int32:
		return uint32(tag)
	case int64:
		return uint32(tag)
	case uint32:
		return tag
	case uint64:
		return uint32(tag)
	default:
		return 0
	}
}

var _ EndstopManagedTrsync = (*ManagedTrsync)(nil)
