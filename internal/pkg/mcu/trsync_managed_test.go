package mcu

import "testing"

func TestNewManagedTrsyncRuntimeRegistersCallbacks(t *testing.T) {
	var registeredBuildConfig func()
	var registeredShutdown func([]interface{}) error
	buildBindingsCalls := 0

	runtime := NewManagedTrsyncRuntime(ManagedTrsyncRuntimeHooks{
		MCUKey:            "mcu-a",
		CreateOID:         func() int { return 41 },
		AllocCommandQueue: func() interface{} { return "cmdq" },
		BuildBindings: func(oid int, cmdQueue interface{}) ManagedTrsyncBindings {
			buildBindingsCalls++
			if oid != 41 {
				t.Fatalf("unexpected oid %d", oid)
			}
			if cmdQueue != "cmdq" {
				t.Fatalf("unexpected command queue %#v", cmdQueue)
			}
			return ManagedTrsyncBindings{}
		},
		RegisterConfigCallback: func(cb func()) {
			registeredBuildConfig = cb
		},
		RegisterShutdownHandler: func(cb func([]interface{}) error) {
			registeredShutdown = cb
		},
	})

	if buildBindingsCalls != 1 {
		t.Fatalf("expected one bindings build, got %d", buildBindingsCalls)
	}
	if runtime.Get_oid() != 41 {
		t.Fatalf("unexpected runtime oid %d", runtime.Get_oid())
	}
	if runtime.Get_command_queue() != "cmdq" {
		t.Fatalf("unexpected runtime command queue %#v", runtime.Get_command_queue())
	}
	if registeredBuildConfig == nil {
		t.Fatal("expected build-config callback to be registered")
	}
	if registeredShutdown == nil {
		t.Fatal("expected shutdown handler to be registered")
	}
}

type fakeManagedTrsyncRuntimeHost struct {
	createdOID          int
	allocatedQueue      interface{}
	registeredBuild     func()
	lookupCommandValue  interface{}
	lookupQueryValue    interface{}
	registeredResponses []struct {
		msg string
		oid interface{}
	}
}

func (self *fakeManagedTrsyncRuntimeHost) CreateOID() int                  { return self.createdOID }
func (self *fakeManagedTrsyncRuntimeHost) AllocCommandQueue() interface{}  { return self.allocatedQueue }
func (self *fakeManagedTrsyncRuntimeHost) AddConfigCmd(string, bool, bool) {}
func (self *fakeManagedTrsyncRuntimeHost) LookupCommandRaw(string, interface{}) (interface{}, error) {
	return self.lookupCommandValue, nil
}
func (self *fakeManagedTrsyncRuntimeHost) LookupQueryCommand(string, string, int, interface{}, bool) interface{} {
	return self.lookupQueryValue
}
func (self *fakeManagedTrsyncRuntimeHost) LookupCommandTag(string) interface{} { return 0 }
func (self *fakeManagedTrsyncRuntimeHost) RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{}) {
	_ = cb
	self.registeredResponses = append(self.registeredResponses, struct {
		msg string
		oid interface{}
	}{msg: msg, oid: oid})
}
func (self *fakeManagedTrsyncRuntimeHost) Clock32ToClock64(clock int64) int64 { return clock }
func (self *fakeManagedTrsyncRuntimeHost) PrintTimeToClock(printTime float64) int64 {
	return int64(printTime * 1000)
}
func (self *fakeManagedTrsyncRuntimeHost) SecondsToClock(seconds float64) int64 {
	return int64(seconds * 1000)
}
func (self *fakeManagedTrsyncRuntimeHost) IsFileoutput() bool { return false }
func (self *fakeManagedTrsyncRuntimeHost) RegisterConfigCallback(cb func()) {
	self.registeredBuild = cb
}

func TestNewManagedTrsyncRuntimeForHostAdaptsHostMethods(t *testing.T) {
	host := &fakeManagedTrsyncRuntimeHost{
		createdOID:         23,
		allocatedQueue:     "cmdq",
		lookupCommandValue: &fakeTrsyncCommandSender{},
		lookupQueryValue:   &fakeTrsyncQuerySender{},
	}
	var registeredShutdown func([]interface{}) error
	asyncCalls := 0

	runtime := NewManagedTrsyncRuntimeForHost(host, ManagedTrsyncRuntimeFactoryHooks{
		RegisterShutdownHandler: func(handler func([]interface{}) error) {
			registeredShutdown = handler
		},
		CreateDispatch: func(tags TrsyncDispatchTags) interface{} {
			if tags.OID != 23 || tags.CmdQueue != "cmdq" {
				t.Fatalf("unexpected dispatch tags %#v", tags)
			}
			return "dispatch"
		},
		SetupDispatch: func(handle interface{}, plan TrsyncStartPlan) {
			_ = plan
			if handle != "dispatch" {
				t.Fatalf("unexpected dispatch handle %#v", handle)
			}
		},
		AsyncComplete: func(completion Completion, result map[string]interface{}) {
			asyncCalls++
			completion.Complete(result)
		},
	})

	if runtime.Get_oid() != 23 {
		t.Fatalf("unexpected runtime oid %d", runtime.Get_oid())
	}
	if runtime.Get_command_queue() != "cmdq" {
		t.Fatalf("unexpected runtime command queue %#v", runtime.Get_command_queue())
	}
	if host.registeredBuild == nil {
		t.Fatal("expected config callback registration")
	}
	if registeredShutdown == nil {
		t.Fatal("expected shutdown handler registration")
	}
	runtime.Build_config()
	if len(host.registeredResponses) != 0 {
		t.Fatalf("expected no response registration during build, got %#v", host.registeredResponses)
	}
	if asyncCalls != 0 {
		t.Fatalf("unexpected async completion count %d", asyncCalls)
	}
}

func TestManagedTrsyncBuildConfigStartHandleAndStop(t *testing.T) {
	start := &fakeTrsyncCommandSender{}
	timeout := &fakeTrsyncCommandSender{}
	trigger := &fakeTrsyncCommandSender{}
	stepperStop := &fakeTrsyncCommandSender{}
	query := &fakeTrsyncQuerySender{response: map[string]interface{}{"trigger_reason": int64(ReasonEndstopHit)}}
	lookupTags := map[string]interface{}{
		"trsync_set_timeout oid=%c clock=%u":                            int(12),
		"trsync_trigger oid=%c reason=%c":                               int(13),
		"trsync_state oid=%c can_trigger=%c trigger_reason=%c clock=%u": int(14),
	}
	lookupCommands := map[string]*fakeTrsyncCommandSender{
		"trsync_start oid=%c report_clock=%u report_ticks=%u expire_reason=%c": start,
		"trsync_set_timeout oid=%c clock=%u":                                   timeout,
		"trsync_trigger oid=%c reason=%c":                                      trigger,
		"stepper_stop_on_trigger oid=%c trsync_oid=%c":                         stepperStop,
	}
	type configCall struct {
		cmd       string
		isInit    bool
		onRestart bool
	}
	var configCalls []configCall
	var responseHandler func(map[string]interface{}) error
	var createdTags TrsyncDispatchTags
	var setupHandle interface{}
	var setupPlan TrsyncStartPlan
	var asyncCompletion Completion
	var asyncResult map[string]interface{}

	runtime := NewManagedTrsync("mcu-a", 11, "cmdq", ManagedTrsyncBindings{
		AddConfigCmd: func(cmd string, isInit bool, onRestart bool) {
			configCalls = append(configCalls, configCall{cmd: cmd, isInit: isInit, onRestart: onRestart})
		},
		LookupCommand: func(msgformat string, cq interface{}) (TrsyncCommandSender, error) {
			if cq != "cmdq" {
				t.Fatalf("unexpected command queue %#v", cq)
			}
			return lookupCommands[msgformat], nil
		},
		LookupQueryCommand: func(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) TrsyncQuerySender {
			if msgformat == "" || respformat == "" || oid != 11 || cq != "cmdq" || isAsync {
				t.Fatalf("unexpected query lookup args msg=%q resp=%q oid=%d cq=%#v async=%v", msgformat, respformat, oid, cq, isAsync)
			}
			return query
		},
		LookupCommandTag: func(msgformat string) interface{} {
			return lookupTags[msgformat]
		},
		SetStateResponse: func(handler func(map[string]interface{}) error) {
			responseHandler = handler
		},
		CreateDispatch: func(tags TrsyncDispatchTags) interface{} {
			createdTags = tags
			return "dispatch-handle"
		},
		SetupDispatch: func(handle interface{}, plan TrsyncStartPlan) {
			setupHandle = handle
			setupPlan = plan
		},
		AsyncComplete: func(completion Completion, result map[string]interface{}) {
			asyncCompletion = completion
			asyncResult = result
		},
		Clock32ToClock64: func(clock int64) int64 { return clock },
		PrintTimeToClock: func(printTime float64) int64 { return int64(printTime * 1000) },
		SecondsToClock:   func(seconds float64) int64 { return int64(seconds * 1000) },
		IsFileoutput:     func() bool { return false },
	})
	stepper := &fakeTrsyncManagedStepper{name: "stepper_x", oid: 7}
	runtime.Add_stepper(stepper)
	runtime.Build_config()

	if len(configCalls) != 2 {
		t.Fatalf("expected 2 config calls, got %#v", configCalls)
	}
	if configCalls[0].cmd != "config_trsync oid=11" || configCalls[0].onRestart {
		t.Fatalf("unexpected config call %#v", configCalls[0])
	}
	if configCalls[1].cmd != "trsync_start oid=11 report_clock=0 report_ticks=0 expire_reason=0" || !configCalls[1].onRestart {
		t.Fatalf("unexpected restart call %#v", configCalls[1])
	}
	if createdTags.OID != 11 || createdTags.CmdQueue != "cmdq" || createdTags.SetTimeoutTag != 12 || createdTags.TriggerTag != 13 || createdTags.StateTag != 14 {
		t.Fatalf("unexpected dispatch tags %#v", createdTags)
	}

	completion := &fakeTrsyncCompletion{}
	runtime.Start(5.0, 0.25, completion, 0.1)
	if responseHandler == nil {
		t.Fatal("expected state response handler to be registered")
	}
	if setupHandle != "dispatch-handle" {
		t.Fatalf("unexpected dispatch handle %#v", setupHandle)
	}
	if setupPlan.Clock != 5000 || setupPlan.ExpireClock != 5100 || setupPlan.ReportClock != 5008 {
		t.Fatalf("unexpected dispatch setup plan %#v", setupPlan)
	}
	if len(start.calls) != 1 || len(timeout.calls) != 1 || len(stepperStop.calls) != 1 {
		t.Fatalf("unexpected command call counts start=%d timeout=%d stop=%d", len(start.calls), len(timeout.calls), len(stepperStop.calls))
	}

	if err := responseHandler(map[string]interface{}{"can_trigger": int64(0), "trigger_reason": int64(ReasonCommsTimeout)}); err != nil {
		t.Fatalf("unexpected state handler error: %v", err)
	}
	if asyncCompletion != completion {
		t.Fatalf("expected async completion %#v, got %#v", completion, asyncCompletion)
	}
	if asyncResult == nil || asyncResult["aa"] != true {
		t.Fatalf("unexpected async result %#v", asyncResult)
	}

	reason := runtime.Stop().(int64)
	if reason != ReasonEndstopHit {
		t.Fatalf("unexpected stop reason %d", reason)
	}
	if responseHandler != nil {
		t.Fatal("expected state response handler to be cleared on stop")
	}
	if stepper.noteHomingCount != 1 {
		t.Fatalf("expected one homing notification, got %d", stepper.noteHomingCount)
	}
	if got := stepperStop.calls[0].data.([]int64); len(got) != 2 || got[0] != 7 || got[1] != 11 {
		t.Fatalf("unexpected stepper stop payload %#v", got)
	}
	if got := query.lastData.([]int64); len(got) != 2 || got[0] != 11 || got[1] != ReasonHostRequest {
		t.Fatalf("unexpected stop query payload %#v", got)
	}
}
