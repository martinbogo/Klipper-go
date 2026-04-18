package motion

import "testing"

type fakeManualStepperToolhead struct {
	lastMoveTime float64
	dwells       []float64
	queueTimes   []float64
	queueFlags   []bool
}

func (self *fakeManualStepperToolhead) GetLastMoveTime() float64 {
	return self.lastMoveTime
}

func (self *fakeManualStepperToolhead) Dwell(delay float64) {
	self.dwells = append(self.dwells, delay)
}

func (self *fakeManualStepperToolhead) NoteMovequeueActivity(mqTime float64, setStepGenTime bool) {
	self.queueTimes = append(self.queueTimes, mqTime)
	self.queueFlags = append(self.queueFlags, setStepGenTime)
}

type fakeManualStepperMotorController struct {
	calls []manualStepperMotorCall
}

type manualStepperMotorCall struct {
	stepperName string
	printTime   float64
	enable      bool
}

func (self *fakeManualStepperMotorController) SetStepperEnabled(stepperName string, printTime float64, enable bool) {
	self.calls = append(self.calls, manualStepperMotorCall{
		stepperName: stepperName,
		printTime:   printTime,
		enable:      enable,
	})
}

type fakeManualStepperCommand struct {
	floats map[string]float64
	ints   map[string]int
}

func (self *fakeManualStepperCommand) Get_float(name string, defaultValue interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	_, _, _, _ = minval, maxval, above, below
	if value, ok := self.floats[name]; ok {
		return value
	}
	switch typed := defaultValue.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case nil:
		return 0.
	default:
		return 0.
	}
}

func (self *fakeManualStepperCommand) Get_int(name string, defaultValue interface{}, minval *int, maxval *int) int {
	_, _ = minval, maxval
	if value, ok := self.ints[name]; ok {
		return value
	}
	switch typed := defaultValue.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case nil:
		return 0
	default:
		return 0
	}
}

type fakeManualStepperCommandRuntime struct {
	velocity  float64
	accel     float64
	enabled   []bool
	positions []float64
	homeCalls []manualStepperHomeCall
	moveCalls []manualStepperMoveCall
	syncCount int
	homeErr   error
}

type manualStepperHomeCall struct {
	movepos      float64
	speed        float64
	accel        float64
	triggered    bool
	checkTrigger bool
}

type manualStepperMoveCall struct {
	movepos float64
	speed   float64
	accel   float64
	sync    bool
}

func (self *fakeManualStepperCommandRuntime) ManualStepperVelocity() float64 {
	return self.velocity
}

func (self *fakeManualStepperCommandRuntime) ManualStepperAccel() float64 {
	return self.accel
}

func (self *fakeManualStepperCommandRuntime) SetManualStepperEnabled(enable bool) {
	self.enabled = append(self.enabled, enable)
}

func (self *fakeManualStepperCommandRuntime) SetManualStepperPosition(setpos float64) {
	self.positions = append(self.positions, setpos)
}

func (self *fakeManualStepperCommandRuntime) HomeManualStepper(movepos float64, speed float64, accel float64, triggered bool, checkTrigger bool) error {
	self.homeCalls = append(self.homeCalls, manualStepperHomeCall{
		movepos:      movepos,
		speed:        speed,
		accel:        accel,
		triggered:    triggered,
		checkTrigger: checkTrigger,
	})
	return self.homeErr
}

func (self *fakeManualStepperCommandRuntime) MoveManualStepper(movepos float64, speed float64, accel float64, sync bool) {
	self.moveCalls = append(self.moveCalls, manualStepperMoveCall{
		movepos: movepos,
		speed:   speed,
		accel:   accel,
		sync:    sync,
	})
}

func (self *fakeManualStepperCommandRuntime) SyncManualStepper() {
	self.syncCount++
}

func TestManualStepperRuntimeCoordinatesStepperAndToolhead(t *testing.T) {
	stepperCore := NewLegacyRailRuntime()
	stepper := &fakeLegacyRailStepper{name: "manual_stepper", commandedPosition: 2.0}
	stepperCore.AddStepper(stepper)
	runtime := NewManualStepperRuntime(5.0, 2.5, stepperCore)
	toolhead := &fakeManualStepperToolhead{lastMoveTime: 10.0}
	motors := &fakeManualStepperMotorController{}

	if len(stepper.trapqs) != 1 || stepper.trapqs[0] == nil {
		t.Fatal("expected runtime construction to bind the trapq to the stepper core")
	}

	runtime.SetEnabled(toolhead, motors, []string{"manual_stepper"}, true)
	if len(motors.calls) != 1 {
		t.Fatalf("SetEnabled() calls = %d, want 1", len(motors.calls))
	}
	if !motors.calls[0].enable {
		t.Fatal("expected motor enable call")
	}
	if motors.calls[0].printTime != 10.0 {
		t.Fatalf("SetEnabled() printTime = %v, want 10.0", motors.calls[0].printTime)
	}

	endTime := runtime.Move(toolhead, 12.0, 6.0, 2.5, false)
	if endTime <= 10.0 {
		t.Fatalf("Move() endTime = %v, want > 10.0", endTime)
	}
	if stepper.generateCalls != 1 {
		t.Fatalf("GenerateSteps() calls = %d, want 1", stepper.generateCalls)
	}
	if len(toolhead.queueTimes) != 1 || toolhead.queueFlags[0] {
		t.Fatalf("NoteMovequeueActivity() = %#v / %#v, want one false entry", toolhead.queueTimes, toolhead.queueFlags)
	}
}

func TestManualStepperRuntimeTracksPositionsAndHomingAccel(t *testing.T) {
	stepperCore := NewLegacyRailRuntime()
	stepper := &fakeLegacyRailStepper{name: "manual_stepper", commandedPosition: 7.5}
	stepperCore.AddStepper(stepper)
	runtime := NewManualStepperRuntime(3.0, 1.0, stepperCore)
	toolhead := &fakeManualStepperToolhead{lastMoveTime: 4.0}

	runtime.SetPosition(9.25)
	if len(stepper.positions) != 1 || stepper.positions[0][0] != 9.25 {
		t.Fatalf("SetPosition() positions = %#v, want first axis 9.25", stepper.positions)
	}
	if got := runtime.Position(); got[0] != 7.5 || len(got) != 4 {
		t.Fatalf("Position() = %#v, want [7.5 0 0 0] shape", got)
	}
	if got := runtime.CalcPosition(map[string]float64{"manual_stepper": 11.5}); got[0] != 11.5 {
		t.Fatalf("CalcPosition() = %#v, want first axis 11.5", got)
	}

	runtime.SyncPrintTime(toolhead)
	runtime.Dwell(-2.0)
	runtime.Dwell(1.25)
	if got := runtime.LastMoveTime(toolhead); got != 5.25 {
		t.Fatalf("LastMoveTime() = %v, want 5.25", got)
	}
	if len(toolhead.dwells) != 1 || toolhead.dwells[0] != 1.25 {
		t.Fatalf("toolhead dwells = %#v, want [1.25]", toolhead.dwells)
	}

	runtime.UpdateHomingAccel(8.0)
	if err := runtime.DripMove(toolhead, []float64{20.0}, 5.0); err != nil {
		t.Fatalf("DripMove() error = %v", err)
	}
	if stepper.generateCalls != 1 {
		t.Fatalf("DripMove() generateCalls = %d, want 1", stepper.generateCalls)
	}
}

func TestHandleManualStepperCommandDispatchesMovePath(t *testing.T) {
	runtime := &fakeManualStepperCommandRuntime{velocity: 5.0, accel: 2.0}
	command := &fakeManualStepperCommand{
		ints: map[string]int{
			"ENABLE": 1,
			"SYNC":   0,
		},
		floats: map[string]float64{
			"SET_POSITION": 4.5,
			"MOVE":         9.0,
			"SPEED":        11.0,
			"ACCEL":        3.5,
		},
	}

	if err := HandleManualStepperCommand(runtime, command); err != nil {
		t.Fatalf("HandleManualStepperCommand() error = %v", err)
	}
	if len(runtime.enabled) != 1 || !runtime.enabled[0] {
		t.Fatalf("enabled calls = %#v, want [true]", runtime.enabled)
	}
	if len(runtime.positions) != 1 || runtime.positions[0] != 4.5 {
		t.Fatalf("positions = %#v, want [4.5]", runtime.positions)
	}
	if len(runtime.moveCalls) != 1 {
		t.Fatalf("move calls = %d, want 1", len(runtime.moveCalls))
	}
	moveCall := runtime.moveCalls[0]
	if moveCall.movepos != 9.0 || moveCall.speed != 11.0 || moveCall.accel != 3.5 || moveCall.sync {
		t.Fatalf("move call = %#v, want movepos=9 speed=11 accel=3.5 sync=false", moveCall)
	}
	if runtime.syncCount != 0 {
		t.Fatalf("sync count = %d, want 0", runtime.syncCount)
	}
}

func TestHandleManualStepperCommandDispatchesHomingAndSyncPath(t *testing.T) {
	runtime := &fakeManualStepperCommandRuntime{velocity: 5.0, accel: 2.0}
	homingCommand := &fakeManualStepperCommand{
		ints: map[string]int{
			"STOP_ON_ENDSTOP": 1,
		},
		floats: map[string]float64{
			"MOVE":  6.0,
			"SPEED": 7.0,
			"ACCEL": 1.5,
		},
	}

	if err := HandleManualStepperCommand(runtime, homingCommand); err != nil {
		t.Fatalf("HandleManualStepperCommand() homing error = %v", err)
	}
	if len(runtime.homeCalls) != 1 {
		t.Fatalf("home calls = %d, want 1", len(runtime.homeCalls))
	}
	homeCall := runtime.homeCalls[0]
	if homeCall.movepos != 6.0 || homeCall.speed != 7.0 || homeCall.accel != 1.5 || !homeCall.triggered || !homeCall.checkTrigger {
		t.Fatalf("home call = %#v, want triggered homing move", homeCall)
	}

	syncCommand := &fakeManualStepperCommand{ints: map[string]int{"SYNC": 1}}
	if err := HandleManualStepperCommand(runtime, syncCommand); err != nil {
		t.Fatalf("HandleManualStepperCommand() sync error = %v", err)
	}
	if runtime.syncCount != 1 {
		t.Fatalf("sync count = %d, want 1", runtime.syncCount)
	}
}
