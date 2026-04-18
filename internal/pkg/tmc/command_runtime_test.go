package tmc

import (
	"fmt"
	"testing"
)

type fakeCommandRuntimeRegisterAccess struct {
	fields          *FieldHelper
	reads           map[string][]int64
	writes          []string
	writePrintTimes []*float64
	getRegisterHook func(regName string) (int64, error, bool)
	setRegisterHook func(regName string, value int64, printTime *float64) error
}

func (self *fakeCommandRuntimeRegisterAccess) Get_fields() *FieldHelper {
	return self.fields
}

func (self *fakeCommandRuntimeRegisterAccess) Get_register(regName string) (int64, error) {
	if self.getRegisterHook != nil {
		if value, err, handled := self.getRegisterHook(regName); handled {
			return value, err
		}
	}
	queue := self.reads[regName]
	if len(queue) == 0 {
		return 0, nil
	}
	value := queue[0]
	self.reads[regName] = queue[1:]
	return value, nil
}

func (self *fakeCommandRuntimeRegisterAccess) Set_register(regName string, value int64, printTime *float64) error {
	if self.setRegisterHook != nil {
		if err := self.setRegisterHook(regName, value, printTime); err != nil {
			return err
		}
	}
	_ = value
	self.writes = append(self.writes, regName)
	if printTime == nil {
		self.writePrintTimes = append(self.writePrintTimes, nil)
		return nil
	}
	printTimeCopy := *printTime
	self.writePrintTimes = append(self.writePrintTimes, &printTimeCopy)
	return nil
}

type fakeCommandRuntimeCurrentHelper struct {
	current  []float64
	setCalls int
	lastRun  float64
	lastHold float64
	lastTime float64
}

func (self *fakeCommandRuntimeCurrentHelper) Get_current() []float64 {
	return append([]float64(nil), self.current...)
}

func (self *fakeCommandRuntimeCurrentHelper) Set_current(runCurrent, holdCurrent, printTime float64) {
	self.setCalls++
	self.lastRun = runCurrent
	self.lastHold = holdCurrent
	self.lastTime = printTime
	self.current[0] = runCurrent
	self.current[1] = holdCurrent
	self.current[2] = holdCurrent
}

type fakeCommandRuntimeStatusChecker struct {
	startChecksResult bool
	startCalls        int
	stopCalls         int
	status            map[string]interface{}
}

func (self *fakeCommandRuntimeStatusChecker) StartChecks() bool {
	self.startCalls++
	return self.startChecksResult
}

func (self *fakeCommandRuntimeStatusChecker) StopChecks() {
	self.stopCalls++
}

func (self *fakeCommandRuntimeStatusChecker) GetStatus(eventtime float64) map[string]interface{} {
	_ = eventtime
	if self.status == nil {
		return map[string]interface{}{"drv_status": nil}
	}
	return self.status
}

type fakeCommandRuntimeEnableLine struct {
	callback  func(float64, bool)
	dedicated bool
	enabled   bool
}

func (self *fakeCommandRuntimeEnableLine) Register_state_callback(callback func(float64, bool)) {
	self.callback = callback
}

func (self *fakeCommandRuntimeEnableLine) Is_motor_enabled() bool {
	return self.enabled
}

func (self *fakeCommandRuntimeEnableLine) Has_dedicated_enable() bool {
	return self.dedicated
}

type fakeCommandRuntimeStepperEnable struct {
	line *fakeCommandRuntimeEnableLine
}

func (self *fakeCommandRuntimeStepperEnable) Lookup_enable(name string) (CommandEnableLine, error) {
	_ = name
	return self.line, nil
}

type fakeCommandRuntimeStepper struct {
	name          string
	setupCalls    int
	setupDuration interface{}
	setupBothEdge bool
	pulseBothEdge bool
	dirInverted   uint32
	mcuPos        int
	positionScale float64
}

func (self *fakeCommandRuntimeStepper) Get_name(short bool) string {
	_ = short
	return self.name
}

func (self *fakeCommandRuntimeStepper) Setup_default_pulse_duration(pulseduration interface{}, step_both_edge bool) {
	self.setupCalls++
	self.setupDuration = pulseduration
	self.setupBothEdge = step_both_edge
}

func (self *fakeCommandRuntimeStepper) Get_pulse_duration() (interface{}, bool) {
	return self.setupDuration, self.pulseBothEdge
}

func (self *fakeCommandRuntimeStepper) Mcu_to_commanded_position(mcuPos int) float64 {
	return float64(mcuPos) * self.positionScale
}

func (self *fakeCommandRuntimeStepper) Get_dir_inverted() (uint32, uint32) {
	return self.dirInverted, 0
}

func (self *fakeCommandRuntimeStepper) Get_mcu_position() int {
	return self.mcuPos
}

type fakeCommandRuntimeToolhead struct {
	lastMoveTime        float64
	waitCalls           int
	monotonic           float64
	pauseCalls          []float64
	estimatedPrintTimes []float64
	estimatedPrintIndex int
}

func (self *fakeCommandRuntimeToolhead) Get_last_move_time() float64 {
	return self.lastMoveTime
}

func (self *fakeCommandRuntimeToolhead) Wait_moves() {
	self.waitCalls++
}

func (self *fakeCommandRuntimeToolhead) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeCommandRuntimeToolhead) EstimatedPrintTime(eventtime float64) float64 {
	_ = eventtime
	if len(self.estimatedPrintTimes) == 0 {
		return self.lastMoveTime
	}
	idx := self.estimatedPrintIndex
	if idx >= len(self.estimatedPrintTimes) {
		return self.estimatedPrintTimes[len(self.estimatedPrintTimes)-1]
	}
	value := self.estimatedPrintTimes[idx]
	if idx < len(self.estimatedPrintTimes)-1 {
		self.estimatedPrintIndex++
	}
	return value
}

func (self *fakeCommandRuntimeToolhead) Pause(waketime float64) float64 {
	self.pauseCalls = append(self.pauseCalls, waketime)
	self.monotonic = waketime
	return waketime
}

type fakeCommandRuntimeMutex struct {
	locks   int
	unlocks int
}

func (self *fakeCommandRuntimeMutex) Lock() {
	self.locks++
}

func (self *fakeCommandRuntimeMutex) Unlock() {
	self.unlocks++
}

func newCommandRuntimeFields() *FieldHelper {
	fields := NewFieldHelper(map[string]map[string]int64{
		"CHOPCONF": {
			"mres":  0x0f,
			"dedge": 1 << 4,
			"toff":  0x1f << 8,
		},
		"MSCNT": {
			"mscnt": 0x3ff,
		},
	}, nil, nil, nil)
	fields.Set_field("mres", 8, nil, nil)
	fields.Set_field("toff", 3, nil, nil)
	return fields
}

func TestCommandRuntimeHandleMCUIdentifyAndConnect(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: false}
	runtime := NewCommandRuntime("stepper_x", access, currentHelper, statusChecker, &fakeCommandRuntimeStepperEnable{line: enableLine})
	stepper := &fakeCommandRuntimeStepper{name: "stepper_x", pulseBothEdge: true, positionScale: 0.1}

	if err := runtime.HandleMCUIdentify(func(name string) CommandStepper {
		if name != "stepper_x" {
			t.Fatalf("unexpected stepper lookup %s", name)
		}
		return stepper
	}); err != nil {
		t.Fatalf("HandleMCUIdentify returned error: %v", err)
	}
	if stepper.setupCalls != 1 || stepper.setupDuration != .000000100 || !stepper.setupBothEdge {
		t.Fatalf("expected default pulse duration setup, got calls=%d duration=%v bothEdge=%v", stepper.setupCalls, stepper.setupDuration, stepper.setupBothEdge)
	}

	if err := runtime.HandleConnect(func(float64, bool) {}); err != nil {
		t.Fatalf("HandleConnect returned error: %v", err)
	}
	if enableLine.callback == nil {
		t.Fatal("expected enable-line callback registration")
	}
	if fields.Get_field("dedge", nil, nil) != 1 {
		t.Fatalf("expected dedge to be enabled, got %d", fields.Get_field("dedge", nil, nil))
	}
	if fields.Get_field("toff", nil, nil) != 0 {
		t.Fatalf("expected virtual enable to zero toff, got %d", fields.Get_field("toff", nil, nil))
	}
	if len(access.writes) == 0 || access.writes[0] != "CHOPCONF" {
		t.Fatalf("expected CHOPCONF init write, got %#v", access.writes)
	}
}

func TestCommandRuntimeHandleEnableWaitsAndSynchronizesPhase(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{"MSCNT": {768}},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{startChecksResult: true}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: true}
	runtime := NewCommandRuntime("stepper_x", access, currentHelper, statusChecker, &fakeCommandRuntimeStepperEnable{line: enableLine})
	stepper := &fakeCommandRuntimeStepper{name: "stepper_x", mcuPos: 0, positionScale: 0.1}
	toolhead := &fakeCommandRuntimeToolhead{lastMoveTime: 12.5}
	mutex := &fakeCommandRuntimeMutex{}

	if err := runtime.HandleMCUIdentify(func(name string) CommandStepper {
		_ = name
		return stepper
	}); err != nil {
		t.Fatalf("HandleMCUIdentify returned error: %v", err)
	}
	enableTime := 19.25
	if err := runtime.HandleEnable(toolhead, mutex, &enableTime); err != nil {
		t.Fatalf("HandleEnable returned error: %v", err)
	}
	if statusChecker.startCalls != 1 {
		t.Fatalf("expected StartChecks once, got %d", statusChecker.startCalls)
	}
	if toolhead.waitCalls != 1 {
		t.Fatalf("expected Wait_moves once, got %d", toolhead.waitCalls)
	}
	if mutex.locks != 1 || mutex.unlocks != 1 {
		t.Fatalf("expected balanced mutex use, got locks=%d unlocks=%d", mutex.locks, mutex.unlocks)
	}
	phaseOffset, phases := runtime.GetPhaseOffset()
	if phaseOffset == nil || *phaseOffset != 3 || phases != 4 {
		t.Fatalf("expected synchronized phase offset=3 phases=4, got offset=%v phases=%d", phaseOffset, phases)
	}
	if len(access.writePrintTimes) == 0 {
		t.Fatal("expected enable-time register writes to be recorded")
	}
	for idx, got := range access.writePrintTimes {
		if got == nil || *got != enableTime {
			t.Fatalf("expected write %d to use enable print time %v, got %v", idx, enableTime, got)
		}
	}
}

func TestCommandRuntimeHandleEnableSkipsPhaseSyncWhileHoming(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{"MSCNT": {768}},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{startChecksResult: true}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: true}
	runtime := NewCommandRuntime("stepper_x", access, currentHelper, statusChecker, &fakeCommandRuntimeStepperEnable{line: enableLine})
	stepper := &fakeCommandRuntimeStepper{name: "stepper_x", mcuPos: 0, positionScale: 0.1}
	toolhead := &fakeCommandRuntimeToolhead{lastMoveTime: 12.5}
	mutex := &fakeCommandRuntimeMutex{}

	if err := runtime.HandleMCUIdentify(func(name string) CommandStepper {
		_ = name
		return stepper
	}); err != nil {
		t.Fatalf("HandleMCUIdentify returned error: %v", err)
	}
	runtime.SetHomingActive(true)
	enableTime := 19.25
	if err := runtime.HandleEnable(toolhead, mutex, &enableTime); err != nil {
		t.Fatalf("HandleEnable returned error: %v", err)
	}
	if statusChecker.startCalls != 1 {
		t.Fatalf("expected StartChecks once, got %d", statusChecker.startCalls)
	}
	if toolhead.waitCalls != 0 {
		t.Fatalf("expected homing fast-path to skip Wait_moves, got %d", toolhead.waitCalls)
	}
	if mutex.locks != 0 || mutex.unlocks != 0 {
		t.Fatalf("expected homing fast-path to skip phase-sync mutex, got locks=%d unlocks=%d", mutex.locks, mutex.unlocks)
	}
	phaseOffset, _ := runtime.GetPhaseOffset()
	if phaseOffset != nil {
		t.Fatalf("expected phase offset to remain unset during homing fast-path, got %v", *phaseOffset)
	}
	if len(access.writePrintTimes) == 0 {
		t.Fatal("expected enable-time register writes to be recorded")
	}
	for idx, got := range access.writePrintTimes {
		if got == nil || *got != enableTime {
			t.Fatalf("expected write %d to use enable print time %v, got %v", idx, enableTime, got)
		}
	}
}

func TestCommandRuntimeInitRegistersUsesFieldTouchOrder(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"REG_A": {"a": 1 << 0},
		"REG_B": {"b": 1 << 0},
		"REG_C": {"c": 1 << 0},
	}, nil, nil, nil)
	fields.Set_field("b", 1, nil, nil)
	fields.Set_field("a", 1, nil, nil)
	fields.Set_field("c", 1, nil, nil)

	access := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{}}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	runtime := NewCommandRuntime("stepper_x", access, currentHelper, nil, nil)

	if err := runtime.InitRegisters(nil); err != nil {
		t.Fatalf("InitRegisters returned error: %v", err)
	}

	want := []string{"REG_B", "REG_A", "REG_C"}
	if len(access.writes) != len(want) {
		t.Fatalf("InitRegisters wrote %d registers, want %d (%#v)", len(access.writes), len(want), access.writes)
	}
	for i := range want {
		if access.writes[i] != want[i] {
			t.Fatalf("InitRegisters write %d = %q, want %q (%#v)", i, access.writes[i], want[i], access.writes)
		}
	}
}

func TestCommandRuntimeHandleEnableRestoresVirtualEnableToff(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{fields: fields, reads: map[string][]int64{"MSCNT": {768}}}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.8, 0.4, 0.4, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{startChecksResult: true}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: false}
	runtime := NewCommandRuntime("stepper_x", access, currentHelper, statusChecker, &fakeCommandRuntimeStepperEnable{line: enableLine})
	stepper := &fakeCommandRuntimeStepper{name: "stepper_x", mcuPos: 0, positionScale: 0.1}
	toolhead := &fakeCommandRuntimeToolhead{lastMoveTime: 12.5}
	mutex := &fakeCommandRuntimeMutex{}

	if err := runtime.HandleMCUIdentify(func(name string) CommandStepper {
		_ = name
		return stepper
	}); err != nil {
		t.Fatalf("HandleMCUIdentify returned error: %v", err)
	}
	if err := runtime.HandleConnect(func(float64, bool) {}); err != nil {
		t.Fatalf("HandleConnect returned error: %v", err)
	}
	if got := fields.Get_field("toff", nil, nil); got != 0 {
		t.Fatalf("expected virtual enable to clear toff during connect, got %d", got)
	}

	enableTime := 19.25
	if err := runtime.HandleEnable(toolhead, mutex, &enableTime); err != nil {
		t.Fatalf("HandleEnable returned error: %v", err)
	}
	if got := fields.Get_field("toff", nil, nil); got != 3 {
		t.Fatalf("expected virtual enable to restore configured toff=3, got %d", got)
	}
}

func TestCommandRuntimeGetStatusMergesCurrentAndErrorState(t *testing.T) {
	fields := newCommandRuntimeFields()
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{"MSCNT": {512}},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.9, 0.5, 0.5, 2.0}}
	statusChecker := &fakeCommandRuntimeStatusChecker{status: map[string]interface{}{
		"drv_status":  map[string]interface{}{"drv_status": map[string]int64{"otpw": 1}},
		"temperature": 12.3,
	}}
	runtime := NewCommandRuntime("stepper_x", access, currentHelper, statusChecker, &fakeCommandRuntimeStepperEnable{line: &fakeCommandRuntimeEnableLine{dedicated: true}})
	stepper := &fakeCommandRuntimeStepper{name: "stepper_x", mcuPos: 1, positionScale: 0.25}

	if err := runtime.HandleMCUIdentify(func(name string) CommandStepper {
		_ = name
		return stepper
	}); err != nil {
		t.Fatalf("HandleMCUIdentify returned error: %v", err)
	}
	if err := runtime.HandleSyncMCUPos(stepper); err != nil {
		t.Fatalf("HandleSyncMCUPos returned error: %v", err)
	}
	status := runtime.GetStatus(0)
	phaseOffset, ok := status["mcu_phase_offset"].(*int)
	if !ok || phaseOffset == nil || *phaseOffset != 1 {
		t.Fatalf("expected phase offset 1, got %#v", status["mcu_phase_offset"])
	}
	if status["phase_offset_position"].(float64) != 0.25 {
		t.Fatalf("expected phase offset position 0.25, got %#v", status["phase_offset_position"])
	}
	if status["run_current"].(float64) != 0.9 || status["hold_current"].(float64) != 0.5 {
		t.Fatalf("expected current values to be preserved, got %#v", status)
	}
	if status["temperature"].(float64) != 12.3 {
		t.Fatalf("expected merged temperature, got %#v", status["temperature"])
	}
	if status["drv_status"].(map[string]interface{})["drv_status"].(map[string]int64)["otpw"] != 1 {
		t.Fatalf("expected merged drv_status, got %#v", status["drv_status"])
	}
}

func TestCommandRuntimeHandleSyncMCUPosRetriesTransientPhaseReadFailure(t *testing.T) {
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
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.9, 0.5, 0.5, 2.0}}
	runtime := NewCommandRuntime(
		"stepper_x",
		access,
		currentHelper,
		nil,
		&fakeCommandRuntimeStepperEnable{line: &fakeCommandRuntimeEnableLine{dedicated: true, enabled: true}},
	)
	stepper := &fakeCommandRuntimeStepper{name: "stepper_x", mcuPos: 0, positionScale: 0.25}
	pauseCalls := 0

	if err := runtime.handleSyncMCUPos(stepper, func() { pauseCalls++ }); err != nil {
		t.Fatalf("handleSyncMCUPos returned error: %v", err)
	}
	if readAttempts != 3 {
		t.Fatalf("expected 3 phase read attempts, got %d", readAttempts)
	}
	if pauseCalls != 2 {
		t.Fatalf("expected 2 retry pauses, got %d", pauseCalls)
	}
	phaseOffset, phases := runtime.GetPhaseOffset()
	if phaseOffset == nil || *phaseOffset != 3 || phases != 4 {
		t.Fatalf("expected synchronized phase offset=3 phases=4, got offset=%v phases=%d", phaseOffset, phases)
	}
}

func TestCommandRuntimeHandleSyncMCUPosSuppressesTransientFailureDuringHoming(t *testing.T) {
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
			return 0, fmt.Errorf("Unable to read tmc uart 'stepper_z' register MSCNT"), true
		},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.9, 0.5, 0.5, 2.0}}
	enableLine := &fakeCommandRuntimeEnableLine{dedicated: true, enabled: true}
	runtime := NewCommandRuntime(
		"stepper_z",
		access,
		currentHelper,
		nil,
		&fakeCommandRuntimeStepperEnable{line: enableLine},
	)
	stepper := &fakeCommandRuntimeStepper{name: "stepper_z", mcuPos: 0, positionScale: 0.25}
	runtime.SetHomingActive(true)

	panicRaised := false
	func() {
		defer func() {
			if recover() != nil {
				panicRaised = true
			}
		}()
		if err := runtime.HandleSyncMCUPos(stepper); err != nil {
			t.Fatalf("HandleSyncMCUPos returned unexpected error: %v", err)
		}
	}()

	if panicRaised {
		t.Fatal("expected transient homing phase-sync failure to be suppressed, but panic propagated")
	}
	if readAttempts != 0 {
		t.Fatalf("expected homing fast-path to skip MSCNT reads, got %d attempts", readAttempts)
	}
	phaseOffset, _ := runtime.GetPhaseOffset()
	if phaseOffset != nil {
		t.Fatalf("expected phase offset to be cleared on suppressed homing failure, got %v", *phaseOffset)
	}
}

func TestCommandRuntimeHandleSyncMCUPosSkipsPhaseReadsDuringHoming(t *testing.T) {
	fields := newCommandRuntimeFields()
	readAttempts := 0
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{"MSCNT": {768}},
		getRegisterHook: func(regName string) (int64, error, bool) {
			if regName != "MSCNT" {
				return 0, nil, false
			}
			readAttempts++
			return 768, nil, true
		},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.9, 0.5, 0.5, 2.0}}
	runtime := NewCommandRuntime(
		"stepper_x",
		access,
		currentHelper,
		nil,
		&fakeCommandRuntimeStepperEnable{line: &fakeCommandRuntimeEnableLine{dedicated: true, enabled: true}},
	)
	stepper := &fakeCommandRuntimeStepper{name: "stepper_x", mcuPos: 0, positionScale: 0.25}
	runtime.SetHomingActive(true)

	if err := runtime.HandleSyncMCUPos(stepper); err != nil {
		t.Fatalf("HandleSyncMCUPos returned unexpected error: %v", err)
	}
	if readAttempts != 0 {
		t.Fatalf("expected homing fast-path to skip MSCNT reads, got %d attempts", readAttempts)
	}
	phaseOffset, _ := runtime.GetPhaseOffset()
	if phaseOffset != nil {
		t.Fatalf("expected phase offset to remain unset while homing, got %v", *phaseOffset)
	}
}

func TestCommandRuntimeDumpRegisterReturnsUnderlyingUARTError(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"GCONF": {
			"pdn_disable": 1 << 6,
		},
	}, nil, nil, nil)
	access := &fakeCommandRuntimeRegisterAccess{
		fields: fields,
		reads:  map[string][]int64{},
		getRegisterHook: func(regName string) (int64, error, bool) {
			if regName != "GCONF" {
				return 0, nil, false
			}
			return 0, fmt.Errorf("Unable to read tmc uart 'stepper_x' register GCONF"), true
		},
	}
	currentHelper := &fakeCommandRuntimeCurrentHelper{current: []float64{0.9, 0.5, 0.5, 2.0}}
	runtime := NewCommandRuntime("stepper_x", access, currentHelper, nil, nil)
	runtime.SetupRegisterDump([]string{"GCONF"}, nil)

	line, err := runtime.DumpRegister("GCONF")
	if err == nil {
		t.Fatal("expected dump register to return the UART read error")
	}
	if line != "" {
		t.Fatalf("expected no formatted line on read failure, got %q", line)
	}
	if got, want := err.Error(), "Unable to read tmc uart 'stepper_x' register GCONF"; got != want {
		t.Fatalf("expected underlying UART error %q, got %q", want, got)
	}
}
