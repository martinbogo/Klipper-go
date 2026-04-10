package tmc

import (
	"errors"
	"goklipper/common/constants"
	"strings"
	"testing"
)

type fakeErrorCheckReactor struct {
	now              float64
	registeredWake   float64
	registeredCount  int
	unregistered     int
	paused           []float64
	callback         func(float64) float64
	registeredHandle interface{}
}

func (self *fakeErrorCheckReactor) Monotonic() float64 {
	return self.now
}

func (self *fakeErrorCheckReactor) Pause(waketime float64) float64 {
	self.paused = append(self.paused, waketime)
	return waketime
}

func (self *fakeErrorCheckReactor) RegisterTimer(callback func(float64) float64, waketime float64) interface{} {
	self.registeredCount++
	self.registeredWake = waketime
	self.callback = callback
	self.registeredHandle = self.registeredCount
	return self.registeredHandle
}

func (self *fakeErrorCheckReactor) UnregisterTimer(timer interface{}) {
	_ = timer
	self.unregistered++
}

type errorCheckReadResult struct {
	value int64
	err   error
}

type errorCheckWrite struct {
	regName string
	value   int64
}

type fakeErrorCheckRegisterAccess struct {
	fields *FieldHelper
	reads  map[string][]errorCheckReadResult
	writes []errorCheckWrite
}

func (self *fakeErrorCheckRegisterAccess) Get_fields() *FieldHelper {
	return self.fields
}

func (self *fakeErrorCheckRegisterAccess) Get_register(regName string) (int64, error) {
	queue := self.reads[regName]
	if len(queue) == 0 {
		return 0, nil
	}
	result := queue[0]
	self.reads[regName] = queue[1:]
	return result.value, result.err
}

func (self *fakeErrorCheckRegisterAccess) Set_register(regName string, value int64, printTime *float64) error {
	_ = printTime
	self.writes = append(self.writes, errorCheckWrite{regName: regName, value: value})
	return nil
}

func newErrorCheckFields() *FieldHelper {
	return NewFieldHelper(map[string]map[string]int64{
		"DRV_STATUS": {
			"ot":    1 << 0,
			"s2ga":  1 << 1,
			"s2gb":  1 << 2,
			"s2vsa": 1 << 3,
			"s2vsb": 1 << 4,
			"otpw":  1 << 5,
		},
		"GSTAT": {
			"drv_err": 1 << 0,
			"reset":   1 << 1,
		},
		"ADC_STAT": {
			"adc_temp": 0xff,
		},
	}, nil, nil, nil)
}

func TestErrorCheckRuntimeStartChecksSchedulesAndClearsReset(t *testing.T) {
	fields := newErrorCheckFields()
	access := &fakeErrorCheckRegisterAccess{
		fields: fields,
		reads: map[string][]errorCheckReadResult{
			"DRV_STATUS": {{value: 0}},
			"GSTAT":      {{value: 1 << 1}, {value: 0}},
		},
	}
	reactor := &fakeErrorCheckReactor{now: 5.0}
	shutdowns := []string{}
	monitorCalls := 0
	helper := NewErrorCheckRuntime("tmc2209", "stepper_x", access, reactor,
		func(msg string) {
			shutdowns = append(shutdowns, msg)
		},
		func() {
			monitorCalls++
		},
	)

	if reactor.registeredWake != constants.NEVER {
		t.Fatalf("expected constructor timer at NEVER, got %v", reactor.registeredWake)
	}
	if monitorCalls != 1 {
		t.Fatalf("expected adc monitor registration, got %d", monitorCalls)
	}
	if !helper.StartChecks() {
		t.Fatal("expected GSTAT reset flag to report driver reset")
	}
	if reactor.unregistered != 1 {
		t.Fatalf("expected initial timer to be unregistered once, got %d", reactor.unregistered)
	}
	if reactor.registeredWake != 6.0 {
		t.Fatalf("expected periodic timer at current time + 1, got %v", reactor.registeredWake)
	}
	if len(access.writes) != 1 || access.writes[0].regName != "GSTAT" || access.writes[0].value != (1<<1) {
		t.Fatalf("expected GSTAT clear write, got %#v", access.writes)
	}
	if len(shutdowns) != 0 {
		t.Fatalf("expected no shutdowns, got %#v", shutdowns)
	}
}

func TestErrorCheckRuntimeRetriesUARTReadsAndFormatsStatus(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"DRV_STATUS": {
			"ot":    1 << 0,
			"s2ga":  1 << 1,
			"s2gb":  1 << 2,
			"s2vsa": 1 << 3,
			"s2vsb": 1 << 4,
			"otpw":  1 << 5,
		},
	}, nil, nil, nil)
	access := &fakeErrorCheckRegisterAccess{
		fields: fields,
		reads: map[string][]errorCheckReadResult{
			"DRV_STATUS": {
				{err: errors.New("Unable to read tmc uart temporary 1")},
				{err: errors.New("Unable to read tmc uart temporary 2")},
				{value: 1 << 5},
			},
		},
	}
	reactor := &fakeErrorCheckReactor{now: 2.0}
	helper := NewErrorCheckRuntime("tmc2209", "stepper_y", access, reactor, nil, nil)

	if helper.StartChecks() {
		t.Fatal("did not expect reset flag when no GSTAT register exists")
	}
	if len(reactor.paused) != 2 {
		t.Fatalf("expected two UART retry pauses, got %d", len(reactor.paused))
	}
	for _, wake := range reactor.paused {
		if wake != 2.05 {
			t.Fatalf("expected UART retry pause at 2.05, got %v", wake)
		}
	}
	status := helper.GetStatus(0)
	drvStatus := status["drv_status"].(map[string]interface{})["drv_status"].(map[string]int64)
	if drvStatus["otpw"] != 1 {
		t.Fatalf("expected warning field to be exposed, got %#v", drvStatus)
	}
}

func TestErrorCheckRuntimePeriodicCheckInvokesShutdownOnDriverError(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"DRV_STATUS": {
			"ot":    1 << 0,
			"s2ga":  1 << 1,
			"s2gb":  1 << 2,
			"s2vsa": 1 << 3,
			"s2vsb": 1 << 4,
		},
	}, nil, nil, nil)
	access := &fakeErrorCheckRegisterAccess{
		fields: fields,
		reads: map[string][]errorCheckReadResult{
			"DRV_STATUS": {
				{value: 0},
				{value: 1 << 0},
				{value: 1 << 0},
				{value: 1 << 0},
			},
		},
	}
	reactor := &fakeErrorCheckReactor{now: 3.0}
	shutdowns := []string{}
	helper := NewErrorCheckRuntime("tmc2209", "stepper_z", access, reactor, func(msg string) {
		shutdowns = append(shutdowns, msg)
	}, nil)
	helper.StartChecks()

	next := reactor.callback(4.0)
	if next != constants.NEVER {
		t.Fatalf("expected periodic shutdown to stop scheduling, got %v", next)
	}
	if len(shutdowns) != 1 {
		t.Fatalf("expected one shutdown, got %#v", shutdowns)
	}
	if !strings.Contains(shutdowns[0], "stepper_z") {
		t.Fatalf("expected shutdown to mention stepper, got %q", shutdowns[0])
	}
}
