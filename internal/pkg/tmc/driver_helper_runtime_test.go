package tmc

import (
	"fmt"
	"goklipper/common/constants"
	gcodepkg "goklipper/internal/pkg/gcode"
	"strings"
	"testing"
)

func resetVirtualPinHomingSequenceState() {
	virtualPinHomingSequenceLock.Lock()
	defer virtualPinHomingSequenceLock.Unlock()
	virtualPinHomingActiveCount = 0
}

type fakeDriverHelperConfig struct {
	name string
	data map[string]interface{}
}

func (self *fakeDriverHelperConfig) Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	_ = minval
	_ = maxval
	_ = above
	_ = below
	_ = noteValid
	if value, ok := self.data[option]; ok {
		return value.(float64)
	}
	if default1 == nil {
		return 0
	}
	return default1.(float64)
}

func (self *fakeDriverHelperConfig) Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int {
	_ = minval
	_ = maxval
	_ = noteValid
	if value, ok := self.data[option]; ok {
		return value.(int)
	}
	if default1 == nil {
		return 0
	}
	return default1.(int)
}

func (self *fakeDriverHelperConfig) Get(option string, default1 interface{}, noteValid bool) interface{} {
	_ = noteValid
	if value, ok := self.data[option]; ok {
		return value
	}
	return default1
}

func (self *fakeDriverHelperConfig) Getchoice(option string, choices map[interface{}]interface{}, default1 interface{}, noteValid bool) interface{} {
	_ = choices
	return self.Get(option, default1, noteValid)
}

func (self *fakeDriverHelperConfig) Getboolean(option string, default1 interface{}, noteValid bool) bool {
	_ = noteValid
	if value, ok := self.data[option]; ok {
		return value.(bool)
	}
	if default1 == nil {
		return false
	}
	return default1.(bool)
}

func (self *fakeDriverHelperConfig) Getint64(option string, default1 interface{}, minval, maxval int64, noteValid bool) int64 {
	_ = minval
	_ = maxval
	_ = noteValid
	if value, ok := self.data[option]; ok {
		return value.(int64)
	}
	if default1 == nil {
		return 0
	}
	return default1.(int64)
}

func (self *fakeDriverHelperConfig) Get_name() string {
	return self.name
}

func TestDriverCommandHelperRegistersCommandsAndAppliesCurrentUpdates(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: true}
	events := map[string]func([]interface{}) error{}
	commands := map[string]func(interface{}) error{}
	responses := []string{}
	config := &fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}

	helper := NewDriverCommandHelper(config, access, currentHelper, DriverCommandHelperOptions{
		StepperEnable: &fakeCommandRuntimeStepperEnable{line: enableLine},
		RegisterEvent: func(event string, callback func([]interface{}) error) {
			events[event] = callback
		},
		RegisterMuxCommand: func(cmd string, key string, value string, handler func(interface{}) error, desc string) {
			_ = key
			_ = value
			_ = desc
			commands[cmd] = handler
		},
		LookupToolhead: func() CommandToolhead {
			return &fakeCommandRuntimeToolhead{lastMoveTime: 19.5}
		},
		LookupStepper: func(name string) CommandStepper {
			return &fakeCommandRuntimeStepper{name: name, pulseBothEdge: true, positionScale: 0.1}
		},
		LookupMutex: func() CommandMutex {
			return &fakeCommandRuntimeMutex{}
		},
	})

	if helper == nil {
		t.Fatal("expected helper to be created")
	}
	if len(events) != 6 {
		t.Fatalf("expected six registered events, got %d", len(events))
	}
	for _, event := range []string{"stepper:sync_mcu_position", "stepper:set_dir_inverted", "project:mcu_identify", "project:connect", "homing:homing_move_begin", "homing:homing_move_end"} {
		if events[event] == nil {
			t.Fatalf("expected event %s to be registered", event)
		}
	}
	for _, cmd := range []string{"INIT_TMC", "SET_TMC_FIELD", "SET_TMC_CURRENT"} {
		if commands[cmd] == nil {
			t.Fatalf("expected command %s to be registered", cmd)
		}
	}

	cmd := gcodepkg.NewDispatchCommand(func(msg string, log bool) {
		_ = log
		responses = append(responses, msg)
	}, func(string) {}, "SET_TMC_CURRENT", "SET_TMC_CURRENT CURRENT=1.00 HOLDCURRENT=0.20", map[string]string{
		"CURRENT":     "1.00",
		"HOLDCURRENT": "0.20",
	}, false)
	if err := commands["SET_TMC_CURRENT"](cmd); err != nil {
		t.Fatalf("SET_TMC_CURRENT returned error: %v", err)
	}
	if currentHelper.setCalls != 1 {
		t.Fatalf("expected current helper update, got %d", currentHelper.setCalls)
	}
	if len(responses) != 1 || responses[0] != "Run Current: 1.00A Hold Current: 0.20A" {
		t.Fatalf("unexpected responses: %#v", responses)
	}

	if err := events["project:mcu_identify"](nil); err != nil {
		t.Fatalf("project:mcu_identify returned error: %v", err)
	}
	if err := events["project:connect"](nil); err != nil {
		t.Fatalf("project:connect returned error: %v", err)
	}
	if enableLine.callback == nil {
		t.Fatal("expected enable-line callback after connect")
	}
}

func TestDriverVirtualPinHelperRegistersChipAndDelegatesHomingEvents(t *testing.T) {
	resetVirtualPinHomingSequenceState()
	defer resetVirtualPinHomingSequenceState()

	fields := NewFieldHelper(map[string]map[string]int64{
		"GCONF": {
			"diag0_stall":    1 << 0,
			"en_spreadcycle": 1 << 1,
		},
		"TPWMTHRS": {
			"tpwmthrs": 0xfffff,
		},
		"TCOOLTHRS": {
			"tcoolthrs": 0xfffff,
		},
	}, nil, nil, nil)
	access := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}
	events := map[string]func([]interface{}) error{}
	chipNames := []string{}
	setupCalls := []string{}
	endstop := struct{ id string }{"virtual"}
	helper := NewDriverVirtualPinHelper(&fakeDriverHelperConfig{
		name: "tmc2209 stepper_x",
		data: map[string]interface{}{"diag0_pin": "PA1"},
	}, access, DriverVirtualPinHelperOptions{
		RegisterChip: func(name string, chip interface{}) {
			chipNames = append(chipNames, fmt.Sprintf("%s:%T", name, chip))
		},
		RegisterEvent: func(event string, callback func([]interface{}) error) {
			events[event] = callback
		},
		SetupPin: func(pinType string, pin string) interface{} {
			setupCalls = append(setupCalls, pinType+":"+pin)
			return endstop
		},
		ExtractHomingMoveEndstops: func(move interface{}) []interface{} {
			if move != "move" {
				t.Fatalf("unexpected move payload: %#v", move)
			}
			return []interface{}{endstop}
		},
	})

	if helper == nil {
		t.Fatal("expected virtual pin helper")
	}
	if len(chipNames) != 1 || chipNames[0] != "tmc2209_stepper_x:*tmc.DriverVirtualPinHelper" {
		t.Fatalf("unexpected chip registration: %#v", chipNames)
	}

	result := helper.Setup_pin("endstop", map[string]interface{}{
		"pin":    "virtual_endstop",
		"invert": false,
		"pullup": false,
	})
	if result != endstop {
		t.Fatalf("expected configured endstop, got %#v", result)
	}
	if len(setupCalls) != 1 || setupCalls[0] != "endstop:PA1" {
		t.Fatalf("unexpected setup-pin calls: %#v", setupCalls)
	}
	if len(events) != 2 {
		t.Fatalf("expected two homing move events, got %d", len(events))
	}

	if err := events["homing:homing_move_begin"]([]interface{}{"move"}); err != nil {
		t.Fatalf("homing_move_begin returned error: %v", err)
	}
	if err := events["homing:homing_move_end"]([]interface{}{"move"}); err != nil {
		t.Fatalf("homing_move_end returned error: %v", err)
	}
	if len(access.writes) == 0 {
		t.Fatal("expected homing handlers to write TMC registers")
	}
}

func TestDriverCommandHelperDumpTMCUARTReadErrorDoesNotPanic(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"GSTAT": {
			"reset": 1 << 0,
		},
	}, nil, nil, nil)
	access := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}
	access.getRegisterHook = func(regName string) (int64, error, bool) {
		if regName == "GSTAT" {
			return 0, fmt.Errorf("Unable to read tmc uart 'stepper_x' register GSTAT"), true
		}
		return 0, nil, false
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: true}
	helper := NewDriverCommandHelper(&fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}, access, currentHelper, DriverCommandHelperOptions{
		StepperEnable: &fakeCommandRuntimeStepperEnable{line: enableLine},
	})
	helper.SetupRegisterDump([]string{"GSTAT"}, nil)

	responses := []string{}
	cmd := gcodepkg.NewDispatchCommand(func(msg string, log bool) {
		_ = log
		responses = append(responses, msg)
	}, func(string) {}, "DUMP_TMC", "DUMP_TMC STEPPER=stepper_x REGISTER=GSTAT", map[string]string{
		"STEPPER":  "stepper_x",
		"REGISTER": "GSTAT",
	}, false)

	if err := helper.Cmd_DUMP_TMC(cmd); err != nil {
		t.Fatalf("Cmd_DUMP_TMC returned error: %v", err)
	}
	if len(responses) != 1 {
		t.Fatalf("expected one response message, got %#v", responses)
	}
	if !strings.Contains(responses[0], "DUMP_TMC stepper_x failed:") {
		t.Fatalf("unexpected response prefix: %q", responses[0])
	}
	if !strings.Contains(responses[0], "Unable to read tmc uart 'stepper_x' register GSTAT") {
		t.Fatalf("unexpected response body: %q", responses[0])
	}
}

func TestDriverVirtualPinHelperOnlyDelaysMatchingParticipants(t *testing.T) {
	resetVirtualPinHomingSequenceState()
	defer resetVirtualPinHomingSequenceState()

	fields := NewFieldHelper(map[string]map[string]int64{
		"GCONF": {
			"diag0_stall":    1 << 0,
			"en_spreadcycle": 1 << 1,
		},
		"TPWMTHRS": {
			"tpwmthrs": 0xfffff,
		},
		"TCOOLTHRS": {
			"tcoolthrs": 0xfffff,
		},
	}, nil, nil, nil)
	matchEndstop := struct{ id string }{"match"}
	nonMatchEndstop := struct{ id string }{"other"}
	move := struct{ id string }{"move"}

	matchingAccess := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}
	matchingPauses := []float64{}
	matchingHelper := NewDriverVirtualPinHelper(&fakeDriverHelperConfig{
		name: "tmc2209 stepper_x",
		data: map[string]interface{}{"diag0_pin": "PA1"},
	}, matchingAccess, DriverVirtualPinHelperOptions{
		SetupPin: func(pinType string, pin string) interface{} {
			_ = pinType
			_ = pin
			return matchEndstop
		},
		ExtractHomingMoveEndstops: func(payload interface{}) []interface{} {
			if payload != move {
				t.Fatalf("unexpected move payload: %#v", payload)
			}
			return []interface{}{matchEndstop}
		},
		ReactorPause: func(waketime float64) float64 {
			matchingPauses = append(matchingPauses, waketime)
			return waketime
		},
		ReactorMonotonic: func() float64 { return 10.0 },
	})
	nonMatchingAccess := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}
	nonMatchingPauses := []float64{}
	nonMatchingHelper := NewDriverVirtualPinHelper(&fakeDriverHelperConfig{
		name: "tmc2209 stepper_y",
		data: map[string]interface{}{"diag0_pin": "PA2"},
	}, nonMatchingAccess, DriverVirtualPinHelperOptions{
		SetupPin: func(pinType string, pin string) interface{} {
			_ = pinType
			_ = pin
			return nonMatchEndstop
		},
		ExtractHomingMoveEndstops: func(payload interface{}) []interface{} {
			if payload != move {
				t.Fatalf("unexpected move payload: %#v", payload)
			}
			return []interface{}{matchEndstop}
		},
		ReactorPause: func(waketime float64) float64 {
			nonMatchingPauses = append(nonMatchingPauses, waketime)
			return waketime
		},
		ReactorMonotonic: func() float64 { return 20.0 },
	})

	matchingHelper.Setup_pin("endstop", map[string]interface{}{"pin": "virtual_endstop", "invert": false, "pullup": false})
	nonMatchingHelper.Setup_pin("endstop", map[string]interface{}{"pin": "virtual_endstop", "invert": false, "pullup": false})

	if err := matchingHelper.handle_homing_move_begin([]interface{}{move}); err != nil {
		t.Fatalf("matching helper homing_move_begin returned error: %v", err)
	}
	if len(matchingPauses) != 0 {
		t.Fatalf("expected first matching helper to avoid delay, got %#v", matchingPauses)
	}
	if err := nonMatchingHelper.handle_homing_move_begin([]interface{}{move}); err != nil {
		t.Fatalf("non-matching helper homing_move_begin returned error: %v", err)
	}
	if len(nonMatchingPauses) != 0 {
		t.Fatalf("expected non-matching helper to avoid delay, got %#v", nonMatchingPauses)
	}
	if len(nonMatchingAccess.writes) != 0 {
		t.Fatalf("expected non-matching helper to skip UART writes, got %#v", nonMatchingAccess.writes)
	}
	if err := matchingHelper.handle_homing_move_end([]interface{}{move}); err != nil {
		t.Fatalf("matching helper homing_move_end returned error: %v", err)
	}
	if err := nonMatchingHelper.handle_homing_move_end([]interface{}{move}); err != nil {
		t.Fatalf("non-matching helper homing_move_end returned error: %v", err)
	}
}

func TestDriverVirtualPinHelperDelaysSecondMatchingParticipant(t *testing.T) {
	resetVirtualPinHomingSequenceState()
	defer resetVirtualPinHomingSequenceState()

	fields := NewFieldHelper(map[string]map[string]int64{
		"GCONF": {
			"diag0_stall":    1 << 0,
			"en_spreadcycle": 1 << 1,
		},
		"TPWMTHRS": {
			"tpwmthrs": 0xfffff,
		},
		"TCOOLTHRS": {
			"tcoolthrs": 0xfffff,
		},
	}, nil, nil, nil)
	endstopX := struct{ id string }{"x"}
	endstopY := struct{ id string }{"y"}
	move := struct{ id string }{"move"}
	makeHelper := func(name string, diagPin string, endstop interface{}, monotonic float64, pauses *[]float64) *DriverVirtualPinHelper {
		return NewDriverVirtualPinHelper(&fakeDriverHelperConfig{
			name: name,
			data: map[string]interface{}{"diag0_pin": diagPin},
		}, &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}, DriverVirtualPinHelperOptions{
			SetupPin: func(pinType string, pin string) interface{} {
				_ = pinType
				_ = pin
				return endstop
			},
			ExtractHomingMoveEndstops: func(payload interface{}) []interface{} {
				if payload != move {
					t.Fatalf("unexpected move payload: %#v", payload)
				}
				return []interface{}{endstopX, endstopY}
			},
			ReactorPause: func(waketime float64) float64 {
				*pauses = append(*pauses, waketime)
				return waketime
			},
			ReactorMonotonic: func() float64 { return monotonic },
		})
	}

	pausesX := []float64{}
	pausesY := []float64{}
	helperX := makeHelper("tmc2209 stepper_x", "PA1", endstopX, 5.0, &pausesX)
	helperY := makeHelper("tmc2209 stepper_y", "PA2", endstopY, 6.0, &pausesY)
	helperX.Setup_pin("endstop", map[string]interface{}{"pin": "virtual_endstop", "invert": false, "pullup": false})
	helperY.Setup_pin("endstop", map[string]interface{}{"pin": "virtual_endstop", "invert": false, "pullup": false})

	if err := helperX.handle_homing_move_begin([]interface{}{move}); err != nil {
		t.Fatalf("first matching helper homing_move_begin returned error: %v", err)
	}
	if len(pausesX) != 0 {
		t.Fatalf("expected first matching helper to avoid delay, got %#v", pausesX)
	}
	if err := helperY.handle_homing_move_begin([]interface{}{move}); err != nil {
		t.Fatalf("second matching helper homing_move_begin returned error: %v", err)
	}
	if len(pausesY) != 1 || pausesY[0] != 6.075 {
		t.Fatalf("expected second matching helper pause at 6.075, got %#v", pausesY)
	}
	if err := helperX.handle_homing_move_end([]interface{}{move}); err != nil {
		t.Fatalf("first matching helper homing_move_end returned error: %v", err)
	}
	if err := helperY.handle_homing_move_end([]interface{}{move}); err != nil {
		t.Fatalf("second matching helper homing_move_end returned error: %v", err)
	}
	if virtualPinHomingActiveCount != 0 {
		t.Fatalf("expected matching homing sequence counter reset, got %d", virtualPinHomingActiveCount)
	}
}

func TestDriverCommandHelperEnableSchedulesImmediatelyBeforeImmediateRegisterWrites(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{"MSCNT": {768}},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{startChecksResult: true}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: true}
	events := map[string]func([]interface{}) error{}
	toolhead := &fakeCommandRuntimeToolhead{
		lastMoveTime:        19.5,
		monotonic:           3.0,
		estimatedPrintTimes: []float64{9.5, 10.5},
	}
	mutex := &fakeCommandRuntimeMutex{}
	var scheduledEventtime float64
	var scheduledCallback func(interface{}) interface{}
	config := &fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}

	helper := NewDriverCommandHelper(config, access, currentHelper, DriverCommandHelperOptions{
		StatusChecker: statusChecker,
		StepperEnable: &fakeCommandRuntimeStepperEnable{line: enableLine},
		RegisterEvent: func(event string, callback func([]interface{}) error) {
			events[event] = callback
		},
		LookupToolhead: func() CommandToolhead {
			return toolhead
		},
		LookupStepper: func(name string) CommandStepper {
			return &fakeCommandRuntimeStepper{name: name, pulseBothEdge: true, positionScale: 0.1}
		},
		LookupMutex: func() CommandMutex {
			return mutex
		},
		ScheduleCallback: func(callback func(interface{}) interface{}, eventtime float64) {
			scheduledCallback = callback
			scheduledEventtime = eventtime
		},
	})

	if helper == nil {
		t.Fatal("expected helper to be created")
	}
	if err := events["project:mcu_identify"](nil); err != nil {
		t.Fatalf("project:mcu_identify returned error: %v", err)
	}
	if err := events["project:connect"](nil); err != nil {
		t.Fatalf("project:connect returned error: %v", err)
	}
	access.writes = nil
	access.writePrintTimes = nil
	statusChecker.startCalls = 0

	enableTime := 10.0
	enableLine.callback(enableTime, true)
	if scheduledCallback == nil {
		t.Fatal("expected enable handler to be scheduled")
	}
	if scheduledEventtime != constants.NOW {
		t.Fatalf("expected enable callback at NOW=%v, got %v", constants.NOW, scheduledEventtime)
	}

	toolhead.monotonic = scheduledEventtime
	scheduledCallback(nil)

	if len(toolhead.pauseCalls) != 1 {
		t.Fatalf("expected scheduled callback to wait once for the enable print time, got %#v", toolhead.pauseCalls)
	}
	if got, want := toolhead.pauseCalls[0], scheduledEventtime+driverCommandPrintTimePollInterval; got != want {
		t.Fatalf("expected enable wait pause at %v, got %v", want, got)
	}
	if toolhead.waitCalls != 1 {
		t.Fatalf("expected phase-sync Wait_moves only, got %d calls", toolhead.waitCalls)
	}
	if statusChecker.startCalls != 1 {
		t.Fatalf("expected StartChecks once, got %d", statusChecker.startCalls)
	}
	if mutex.locks != 1 || mutex.unlocks != 1 {
		t.Fatalf("expected balanced mutex use, got locks=%d unlocks=%d", mutex.locks, mutex.unlocks)
	}
	if len(access.writePrintTimes) == 0 {
		t.Fatal("expected enable-time register writes")
	}
	for idx, got := range access.writePrintTimes {
		if got != nil {
			t.Fatalf("expected enable write %d to be immediate after the print-time wait, got %v", idx, *got)
		}
	}
	phaseOffset, phases := helper.GetPhaseOffset()
	if phaseOffset == nil || *phaseOffset != 3 || phases != 4 {
		t.Fatalf("expected synchronized phase offset=3 phases=4, got offset=%v phases=%d", phaseOffset, phases)
	}
}

func TestDriverCommandHelperEnableUsesImmediateWritesWithoutSettleWait(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{"MSCNT": {768}},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{startChecksResult: true}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: true}
	events := map[string]func([]interface{}) error{}
	toolhead := &fakeCommandRuntimeToolhead{
		lastMoveTime:        19.5,
		monotonic:           3.0,
		estimatedPrintTimes: []float64{10.0, 10.05, 10.101},
	}
	config := &fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}

	helper := NewDriverCommandHelper(config, access, currentHelper, DriverCommandHelperOptions{
		StatusChecker: statusChecker,
		StepperEnable: &fakeCommandRuntimeStepperEnable{line: enableLine},
		RegisterEvent: func(event string, callback func([]interface{}) error) {
			events[event] = callback
		},
		LookupToolhead: func() CommandToolhead {
			return toolhead
		},
		LookupStepper: func(name string) CommandStepper {
			return &fakeCommandRuntimeStepper{name: name, pulseBothEdge: true, positionScale: 0.1}
		},
		LookupMutex: func() CommandMutex {
			return &fakeCommandRuntimeMutex{}
		},
	})

	if err := events["project:mcu_identify"](nil); err != nil {
		t.Fatalf("project:mcu_identify returned error: %v", err)
	}
	if err := events["project:connect"](nil); err != nil {
		t.Fatalf("project:connect returned error: %v", err)
	}
	access.writes = nil
	access.writePrintTimes = nil
	statusChecker.startCalls = 0

	enableTime := 10.0
	helper.do_enable(&enableTime)

	if got := len(toolhead.pauseCalls); got != 0 {
		t.Fatalf("expected no settle-delay pause polls, got %d", got)
	}
	if statusChecker.startCalls != 1 {
		t.Fatalf("expected StartChecks once, got %d", statusChecker.startCalls)
	}
	for idx, got := range access.writePrintTimes {
		if got != nil {
			t.Fatalf("expected enable write %d to be immediate after settle wait, got %v", idx, *got)
		}
	}
}

func TestDriverCommandHelperDoEnableNormalizesShutdownReason(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{},
		setRegisterHook: func(regName string, value int64, printTime *float64) error {
			_ = regName
			_ = value
			_ = printTime
			return fmt.Errorf("write failed")
		},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	toolhead := &fakeCommandRuntimeToolhead{lastMoveTime: 19.5}
	shutdownReasons := []interface{}{}
	config := &fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}

	helper := NewDriverCommandHelper(config, access, currentHelper, DriverCommandHelperOptions{
		LookupToolhead: func() CommandToolhead {
			return toolhead
		},
		LookupStepper: func(name string) CommandStepper {
			return &fakeCommandRuntimeStepper{name: name, positionScale: 0.1}
		},
		StepperEnable: &fakeCommandRuntimeStepperEnable{line: &fakeCommandRuntimeEnableLine{dedicated: true}},
		Shutdown: func(reason interface{}) interface{} {
			shutdownReasons = append(shutdownReasons, reason)
			return nil
		},
	})

	helper.runtime.stepper = &fakeCommandRuntimeStepper{name: "stepper_x", positionScale: 0.1}
	enableTime := 8.0
	helper.do_enable(&enableTime)

	if len(shutdownReasons) != 1 {
		t.Fatalf("expected one shutdown reason, got %#v", shutdownReasons)
	}
	if reason, ok := shutdownReasons[0].(string); !ok || reason != "write failed" {
		t.Fatalf("expected normalized string shutdown reason, got %#v", shutdownReasons[0])
	}
}

func TestDriverCommandHelperDoEnableRetriesTransientUARTWriteFailure(t *testing.T) {
	fields := newCommandRuntimeFields()
	attempts := 0
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{"MSCNT": {768}},
		setRegisterHook: func(regName string, value int64, printTime *float64) error {
			_ = value
			_ = printTime
			if regName != "CHOPCONF" {
				return nil
			}
			attempts++
			if attempts == 1 {
				return fmt.Errorf("Unable to write tmc uart 'stepper_x' register CHOPCONF")
			}
			return nil
		},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{startChecksResult: true}
	toolhead := &fakeCommandRuntimeToolhead{
		lastMoveTime:        19.5,
		monotonic:           3.0,
		estimatedPrintTimes: []float64{10.2, 10.2},
	}
	shutdownReasons := []interface{}{}
	config := &fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}

	helper := NewDriverCommandHelper(config, access, currentHelper, DriverCommandHelperOptions{
		StatusChecker: statusChecker,
		LookupToolhead: func() CommandToolhead {
			return toolhead
		},
		LookupStepper: func(name string) CommandStepper {
			return &fakeCommandRuntimeStepper{name: name, positionScale: 0.1}
		},
		LookupMutex: func() CommandMutex {
			return &fakeCommandRuntimeMutex{}
		},
		StepperEnable: &fakeCommandRuntimeStepperEnable{line: &fakeCommandRuntimeEnableLine{dedicated: true}},
		Shutdown: func(reason interface{}) interface{} {
			shutdownReasons = append(shutdownReasons, reason)
			return nil
		},
	})

	helper.runtime.stepper = &fakeCommandRuntimeStepper{name: "stepper_x", positionScale: 0.1}
	enableTime := 10.0
	helper.do_enable(&enableTime)

	if attempts != 2 {
		t.Fatalf("expected transient UART write to be retried once, got %d attempts", attempts)
	}
	if len(shutdownReasons) != 0 {
		t.Fatalf("expected no shutdown after successful retry, got %#v", shutdownReasons)
	}
	if got := len(toolhead.pauseCalls); got != 1 {
		t.Fatalf("expected one retry pause, got %#v", toolhead.pauseCalls)
	}
	if toolhead.pauseCalls[0] != 3.05 {
		t.Fatalf("expected retry pause wake time 3.05, got %#v", toolhead.pauseCalls)
	}
	if statusChecker.startCalls != 1 {
		t.Fatalf("expected StartChecks once after successful retry, got %d", statusChecker.startCalls)
	}
	if len(access.writePrintTimes) == 0 {
		t.Fatal("expected successful enable-time register write after retry")
	}
	for idx, got := range access.writePrintTimes {
		if got != nil {
			t.Fatalf("expected retry enable write %d to be immediate, got %v", idx, *got)
		}
	}
}

func TestDriverCommandHelperDoEnableUsesExtendedRetriesDuringHoming(t *testing.T) {
	fields := newCommandRuntimeFields()
	attempts := 0
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{"MSCNT": {768}},
		setRegisterHook: func(regName string, value int64, printTime *float64) error {
			_ = value
			_ = printTime
			if regName != "CHOPCONF" {
				return nil
			}
			attempts++
			if attempts <= driverCommandEnableRetryCount+1 {
				return fmt.Errorf("Unable to write tmc uart 'stepper_x' register CHOPCONF")
			}
			return nil
		},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{startChecksResult: true}
	toolhead := &fakeCommandRuntimeToolhead{
		lastMoveTime:        19.5,
		monotonic:           3.0,
		estimatedPrintTimes: []float64{10.2, 10.2},
	}
	shutdownReasons := []interface{}{}
	config := &fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}

	helper := NewDriverCommandHelper(config, access, currentHelper, DriverCommandHelperOptions{
		StatusChecker: statusChecker,
		LookupToolhead: func() CommandToolhead {
			return toolhead
		},
		LookupStepper: func(name string) CommandStepper {
			return &fakeCommandRuntimeStepper{name: name, positionScale: 0.1}
		},
		LookupMutex: func() CommandMutex {
			return &fakeCommandRuntimeMutex{}
		},
		StepperEnable: &fakeCommandRuntimeStepperEnable{line: &fakeCommandRuntimeEnableLine{dedicated: true}},
		Shutdown: func(reason interface{}) interface{} {
			shutdownReasons = append(shutdownReasons, reason)
			return nil
		},
	})

	helper.runtime.stepper = &fakeCommandRuntimeStepper{name: "stepper_x", positionScale: 0.1}
	helper.runtime.SetHomingActive(true)
	enableTime := 10.0
	helper.do_enable(&enableTime)

	if attempts != driverCommandEnableRetryCount+2 {
		t.Fatalf("expected extended homing retries to recover after %d attempts, got %d", driverCommandEnableRetryCount+2, attempts)
	}
	if len(shutdownReasons) != 0 {
		t.Fatalf("expected no shutdown after homing retry recovery, got %#v", shutdownReasons)
	}
	if got := len(toolhead.pauseCalls); got != driverCommandEnableRetryCount+1 {
		t.Fatalf("expected %d retry pauses during homing, got %#v", driverCommandEnableRetryCount+1, toolhead.pauseCalls)
	}
	if statusChecker.startCalls != 1 {
		t.Fatalf("expected StartChecks once after successful homing retries, got %d", statusChecker.startCalls)
	}
}

func TestDriverCommandHelperSharedUARTHomingStressKeepsRetriesBoundedAndPhaseStable(t *testing.T) {
	type driverCase struct {
		name      string
		failCount int
	}
	drivers := []driverCase{
		{name: "stepper_x", failCount: 2},
		{name: "stepper_y", failCount: 3},
		{name: "stepper_z", failCount: 1},
	}

	type runtimeCase struct {
		helper          *DriverCommandHelperRuntime
		toolhead        *fakeCommandRuntimeToolhead
		statusChecker   *fakeCommandRuntimeStatusChecker
		shutdownReasons *[]interface{}
		attempts        *int
	}
	cases := make([]runtimeCase, 0, len(drivers))

	for _, driver := range drivers {
		fields := newCommandRuntimeFields()
		attempts := 0
		access := &fakeCommandRuntimeRegisterAccess{
			fields: fields,
			reads:  map[string][]int64{"MSCNT": {768}},
			setRegisterHook: func(regName string, value int64, printTime *float64) error {
				_ = value
				_ = printTime
				if regName != "CHOPCONF" {
					return nil
				}
				attempts++
				if attempts <= driver.failCount {
					return fmt.Errorf("Unable to write tmc uart '%s' register CHOPCONF", driver.name)
				}
				return nil
			},
		}
		currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
		statusChecker := &fakeCommandRuntimeStatusChecker{startChecksResult: true}
		toolhead := &fakeCommandRuntimeToolhead{
			lastMoveTime:        19.5,
			monotonic:           3.0,
			estimatedPrintTimes: []float64{10.2, 10.2},
		}
		shutdownReasons := []interface{}{}
		helper := NewDriverCommandHelper(&fakeDriverHelperConfig{name: "tmc2209 " + driver.name, data: map[string]interface{}{}}, access, currentHelper, DriverCommandHelperOptions{
			StatusChecker: statusChecker,
			LookupToolhead: func() CommandToolhead {
				return toolhead
			},
			LookupStepper: func(name string) CommandStepper {
				return &fakeCommandRuntimeStepper{name: name, positionScale: 0.1}
			},
			LookupMutex: func() CommandMutex {
				return &fakeCommandRuntimeMutex{}
			},
			StepperEnable: &fakeCommandRuntimeStepperEnable{line: &fakeCommandRuntimeEnableLine{dedicated: true}},
			Shutdown: func(reason interface{}) interface{} {
				shutdownReasons = append(shutdownReasons, reason)
				return nil
			},
		})
		helper.runtime.stepper = &fakeCommandRuntimeStepper{name: driver.name, positionScale: 0.1}
		cases = append(cases, runtimeCase{
			helper:          helper,
			toolhead:        toolhead,
			statusChecker:   statusChecker,
			shutdownReasons: &shutdownReasons,
			attempts:        &attempts,
		})
	}

	enableTime := 10.0
	for i, driver := range drivers {
		helper := cases[i].helper
		if err := helper.handle_homing_move_begin(nil); err != nil {
			t.Fatalf("%s homing begin failed: %v", driver.name, err)
		}
		helper.do_enable(&enableTime)
		if err := helper.handle_homing_move_end(nil); err != nil {
			t.Fatalf("%s homing end failed: %v", driver.name, err)
		}

		if helper.runtime.IsHomingActive() {
			t.Fatalf("%s expected homing-active false after end", driver.name)
		}
		if got := cases[i].statusChecker.startCalls; got != 1 {
			t.Fatalf("%s expected StartChecks once, got %d", driver.name, got)
		}
		if len(*cases[i].shutdownReasons) != 0 {
			t.Fatalf("%s expected no shutdown reasons, got %#v", driver.name, *cases[i].shutdownReasons)
		}
		if got := len(cases[i].toolhead.pauseCalls); got != driver.failCount {
			t.Fatalf("%s expected %d retry pauses, got %#v", driver.name, driver.failCount, cases[i].toolhead.pauseCalls)
		}
		if got := *cases[i].attempts; got != driver.failCount+1 {
			t.Fatalf("%s expected %d UART enable attempts, got %d", driver.name, driver.failCount+1, got)
		}
		if driver.failCount+1 > driverCommandEnableRetryCountHoming {
			t.Fatalf("%s fixture invalid: fail count exceeds homing retry bound", driver.name)
		}
		phaseOffset, phases := helper.GetPhaseOffset()
		if phaseOffset != nil || phases != 4 {
			t.Fatalf("%s expected homing fast-path to defer phase offset (nil) with phases=4, got offset=%v phases=%d", driver.name, phaseOffset, phases)
		}
	}
}

func TestDriverCommandHelperHomingEventsReferenceCountSuppressionScope(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	events := map[string]func([]interface{}) error{}
	config := &fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}

	helper := NewDriverCommandHelper(config, access, currentHelper, DriverCommandHelperOptions{
		RegisterEvent: func(event string, callback func([]interface{}) error) {
			events[event] = callback
		},
		LookupToolhead: func() CommandToolhead {
			return &fakeCommandRuntimeToolhead{lastMoveTime: 19.5}
		},
		LookupStepper: func(name string) CommandStepper {
			return &fakeCommandRuntimeStepper{name: name, pulseBothEdge: true, positionScale: 0.1}
		},
		LookupMutex: func() CommandMutex {
			return &fakeCommandRuntimeMutex{}
		},
	})

	if helper.runtime.IsHomingActive() {
		t.Fatal("expected homing suppression to start inactive")
	}
	if err := events["homing:homing_move_begin"](nil); err != nil {
		t.Fatalf("first homing_move_begin returned error: %v", err)
	}
	if !helper.runtime.IsHomingActive() {
		t.Fatal("expected first homing_move_begin to activate homing suppression")
	}
	if err := events["homing:homing_move_begin"](nil); err != nil {
		t.Fatalf("second homing_move_begin returned error: %v", err)
	}
	if !helper.runtime.IsHomingActive() {
		t.Fatal("expected nested homing_move_begin to keep homing suppression active")
	}
	if err := events["homing:homing_move_end"](nil); err != nil {
		t.Fatalf("first homing_move_end returned error: %v", err)
	}
	if !helper.runtime.IsHomingActive() {
		t.Fatal("expected homing suppression to stay active until the last end event")
	}
	if err := events["homing:homing_move_end"](nil); err != nil {
		t.Fatalf("second homing_move_end returned error: %v", err)
	}
	if helper.runtime.IsHomingActive() {
		t.Fatal("expected final homing_move_end to clear homing suppression")
	}
	if err := events["homing:homing_move_end"](nil); err != nil {
		t.Fatalf("extra homing_move_end returned error: %v", err)
	}
	if helper.runtime.IsHomingActive() {
		t.Fatal("expected extra homing_move_end to leave homing suppression inactive")
	}
}

func TestDriverCommandHelperSyncEventRetriesTransientPhaseReadFailure(t *testing.T) {
	fields := newCommandRuntimeFields()
	readAttempts := 0
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{},
		getRegisterHook: func(regName string) (int64, error, bool) {
			if regName != "MSCNT" {
				return 0, nil, false
			}
			readAttempts++
			if readAttempts < 3 {
				return 0, fmt.Errorf("Unable to read tmc uart 'stepper_x' register MSCNT"), true
			}
			return 768, nil, true
		},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: true, enabled: true}
	events := map[string]func([]interface{}) error{}
	toolhead := &fakeCommandRuntimeToolhead{lastMoveTime: 19.5, monotonic: 5.0}
	stepper := &fakeCommandRuntimeStepper{name: "stepper_x", mcuPos: 0, positionScale: 0.1}
	config := &fakeDriverHelperConfig{name: "tmc2209 stepper_x", data: map[string]interface{}{}}

	helper := NewDriverCommandHelper(config, access, currentHelper, DriverCommandHelperOptions{
		StepperEnable: &fakeCommandRuntimeStepperEnable{line: enableLine},
		RegisterEvent: func(event string, callback func([]interface{}) error) {
			events[event] = callback
		},
		LookupToolhead: func() CommandToolhead {
			return toolhead
		},
		LookupStepper: func(name string) CommandStepper {
			if name != "stepper_x" {
				t.Fatalf("unexpected stepper lookup %s", name)
			}
			return stepper
		},
		LookupMutex: func() CommandMutex {
			return &fakeCommandRuntimeMutex{}
		},
	})

	if helper == nil {
		t.Fatal("expected helper to be created")
	}
	if err := events["project:mcu_identify"](nil); err != nil {
		t.Fatalf("project:mcu_identify returned error: %v", err)
	}
	if err := events["stepper:sync_mcu_position"]([]interface{}{stepper}); err != nil {
		t.Fatalf("stepper:sync_mcu_position returned error: %v", err)
	}
	if readAttempts != 3 {
		t.Fatalf("expected 3 phase read attempts, got %d", readAttempts)
	}
	if len(toolhead.pauseCalls) != 2 {
		t.Fatalf("expected 2 retry pauses, got %#v", toolhead.pauseCalls)
	}
	if toolhead.pauseCalls[0] != 5.05 || toolhead.pauseCalls[1] != 5.1 {
		t.Fatalf("unexpected retry pause schedule: %#v", toolhead.pauseCalls)
	}
	phaseOffset, phases := helper.GetPhaseOffset()
	if phaseOffset == nil || *phaseOffset != 3 || phases != 4 {
		t.Fatalf("expected synchronized phase offset=3 phases=4, got offset=%v phases=%d", phaseOffset, phases)
	}
}
