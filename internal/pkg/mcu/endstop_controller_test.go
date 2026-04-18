package mcu

import (
	"reflect"
	"testing"
)

type fakeEndstopWaitableCompletion struct {
	completed []interface{}
	waits     []float64
	result    interface{}
}

func (self *fakeEndstopWaitableCompletion) Complete(result interface{}) {
	self.completed = append(self.completed, result)
}

func (self *fakeEndstopWaitableCompletion) Wait(waketime float64, waketimeResult interface{}) interface{} {
	self.waits = append(self.waits, waketime)
	if self.result != nil {
		return self.result
	}
	return waketimeResult
}

type fakeEndstopTrsync struct {
	mcuKey       interface{}
	oid          int
	steppers     []EndstopRegistryStepper
	stepperAdds  []interface{}
	startCalls   int
	homeEndTimes []float64
	stopReason   int64
	commandQueue interface{}
}

func (self *fakeEndstopTrsync) MCUKey() interface{} { return self.mcuKey }
func (self *fakeEndstopTrsync) Steppers() []EndstopRegistryStepper {
	steppers := make([]EndstopRegistryStepper, len(self.steppers))
	copy(steppers, self.steppers)
	return steppers
}
func (self *fakeEndstopTrsync) Add_stepper(stepper interface{}) {
	self.stepperAdds = append(self.stepperAdds, stepper)
	if adapted, ok := stepper.(EndstopRegistryStepper); ok {
		self.steppers = append(self.steppers, adapted)
	}
}
func (self *fakeEndstopTrsync) Get_oid() int                   { return self.oid }
func (self *fakeEndstopTrsync) Get_command_queue() interface{} { return self.commandQueue }
func (self *fakeEndstopTrsync) Start(print_time float64, report_offset float64, trigger_completion Completion, expire_timeout float64) {
	self.startCalls++
}
func (self *fakeEndstopTrsync) Set_home_end_time(home_end_time float64) {
	self.homeEndTimes = append(self.homeEndTimes, home_end_time)
}
func (self *fakeEndstopTrsync) Stop() interface{} { return self.stopReason }

type fakeEndstopControllerStepper struct {
	name   string
	mcuKey interface{}
}

func (self *fakeEndstopControllerStepper) MCUKey() interface{} { return self.mcuKey }
func (self *fakeEndstopControllerStepper) Name(short bool) string {
	if short {
		return self.name
	}
	return self.name
}
func (self *fakeEndstopControllerStepper) Raw() interface{} { return self }

type fakeManagedLegacyEndstopController struct {
	nextOID                 int
	registeredConfigActions []interface{}
}

func (self *fakeManagedLegacyEndstopController) Create_oid() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeManagedLegacyEndstopController) Register_config_callback(cb interface{}) {
	self.registeredConfigActions = append(self.registeredConfigActions, cb)
}

func (self *fakeManagedLegacyEndstopController) Add_config_cmd(cmd string, is_init bool, on_restart bool) {
	_, _, _ = cmd, is_init, on_restart
}

func (self *fakeManagedLegacyEndstopController) LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error) {
	_, _ = msgformat, cq
	return nil, nil
}

func (self *fakeManagedLegacyEndstopController) LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{} {
	_, _, _, _, _ = msgformat, respformat, oid, cq, isAsync
	return nil
}

func (self *fakeManagedLegacyEndstopController) Is_fileoutput() bool { return false }

func (self *fakeManagedLegacyEndstopController) Print_time_to_clock(print_time float64) int64 {
	return int64(print_time * 1000)
}

func (self *fakeManagedLegacyEndstopController) Seconds_to_clock(time float64) int64 {
	return int64(time * 1000)
}

func (self *fakeManagedLegacyEndstopController) Clock32_to_clock64(clock32 int64) int64 {
	return clock32
}

func (self *fakeManagedLegacyEndstopController) Clock_to_print_time(clock int64) float64 {
	return float64(clock) / 1000.0
}

func TestEndstopControllerAddStepperCreatesNewTrsyncForNewMCU(t *testing.T) {
	primary := &fakeEndstopTrsync{mcuKey: "mcu0", oid: 3}
	controller := NewEndstopController(11, "PA1", 1, 0, primary)
	stepper := &fakeEndstopControllerStepper{name: "stepper_y", mcuKey: "mcu1"}

	created := 0
	plan := controller.AddStepper(stepper, func() EndstopManagedTrsync {
		created++
		return &fakeEndstopTrsync{mcuKey: "mcu1", oid: 4}
	})

	if !plan.NeedsNewTrsync {
		t.Fatalf("expected new trsync plan")
	}
	if created != 1 {
		t.Fatalf("expected one trsync creation, got %d", created)
	}
	if got := len(controller.Trsyncs()); got != 2 {
		t.Fatalf("expected 2 trsyncs, got %d", got)
	}
}

func TestEndstopControllerHomeWaitQueriesNextClockWhenNeeded(t *testing.T) {
	primary := &fakeEndstopTrsync{mcuKey: "mcu0", oid: 3, stopReason: ReasonEndstopHit}
	controller := NewEndstopController(11, "PA1", 1, 0, primary)
	completion := &fakeEndstopWaitableCompletion{}
	reasons := EndstopHomeReasonCodes{EndstopHit: ReasonEndstopHit, HostRequest: ReasonHostRequest, CommsTimeout: ReasonCommsTimeout}

	controller.HomeStart(1.0, 0.000015, 4, 0.1, 1, DefaultEndstopHomeTimeoutValues(), reasons, EndstopHomeStartOps{
		TriggerCompletion: completion,
		PrintTimeToClock: func(printTime float64) int64 {
			return int64(printTime * 1000)
		},
		SecondsToClock: func(seconds float64) int64 {
			return int64(seconds * 1000)
		},
	})

	result := controller.HomeWait(2.0, reasons, EndstopHomeWaitOps{
		IsFileoutput: false,
		StopTrsync: func(trsync EndstopManagedTrsync) int64 {
			return trsync.Stop().(int64)
		},
		QueryNextClock: func() int64 {
			return 2105
		},
		Clock32ToClock64: func(clock32 int64) int64 {
			return clock32
		},
		ClockToPrintTime: func(clock int64) float64 {
			return float64(clock) / 1000.0
		},
	})

	if result != 2.005 {
		t.Fatalf("expected queried home end time, got %v", result)
	}
	if len(primary.homeEndTimes) != 1 || primary.homeEndTimes[0] != 2.0 {
		t.Fatalf("expected home end time to be recorded, got %#v", primary.homeEndTimes)
	}
}

func TestNewManagedLegacyEndstopUsesStepperMCUKeyForNewTrsync(t *testing.T) {
	controller := &fakeManagedLegacyEndstopController{}
	collectedKeys := []interface{}{}
	endstop := NewManagedLegacyEndstop(
		controller,
		map[string]interface{}{"pin": "PA1", "invert": 0, "pullup": 1},
		"dispatch0",
		func(mcuKey interface{}, trdispatch interface{}) EndstopManagedTrsync {
			collectedKeys = append(collectedKeys, mcuKey)
			return &fakeEndstopTrsync{mcuKey: mcuKey, oid: len(collectedKeys), commandQueue: trdispatch}
		},
		func() WaitableCompletion {
			return &fakeEndstopWaitableCompletion{}
		},
		nil,
		nil,
	)
	stepper := &fakeEndstopControllerStepper{name: "stepper_y", mcuKey: "mcu1"}

	endstop.Add_stepper(stepper)

	if len(controller.registeredConfigActions) != 1 {
		t.Fatalf("expected one config callback registration, got %d", len(controller.registeredConfigActions))
	}
	if got, want := collectedKeys, []interface{}{controller, "mcu1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected managed trsync builder keys %#v, want %#v", got, want)
	}
	if got := len(endstop.core.Trsyncs()); got != 2 {
		t.Fatalf("expected second trsync to be created, got %d", got)
	}
}
