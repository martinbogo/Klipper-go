package motion

import "testing"

type fakeToolheadMoveRuntime struct {
	commandedPos   []float64
	config         MoveConfig
	kinChecks      []*Move
	extraChecks    []*Move
	queuedMoves    []*Move
	printTime      float64
	needCheckPause float64
	checkPauseCnt  int
}

func (self *fakeToolheadMoveRuntime) CommandedPosition() []float64 {
	return append([]float64{}, self.commandedPos...)
}

func (self *fakeToolheadMoveRuntime) MoveConfig() MoveConfig {
	return self.config
}

func (self *fakeToolheadMoveRuntime) SetCommandedPosition(position []float64) {
	self.commandedPos = append([]float64{}, position...)
}

func (self *fakeToolheadMoveRuntime) CheckKinematicMove(move *Move) {
	self.kinChecks = append(self.kinChecks, move)
}

func (self *fakeToolheadMoveRuntime) CheckExtruderMove(move *Move) {
	self.extraChecks = append(self.extraChecks, move)
}

func (self *fakeToolheadMoveRuntime) QueueMove(move *Move) {
	self.queuedMoves = append(self.queuedMoves, move)
}

func (self *fakeToolheadMoveRuntime) PrintTime() float64 {
	return self.printTime
}

func (self *fakeToolheadMoveRuntime) NeedCheckPause() float64 {
	return self.needCheckPause
}

func (self *fakeToolheadMoveRuntime) CheckPause() {
	self.checkPauseCnt++
}

type fakeToolheadDwellRuntime struct {
	lastMoveTime     float64
	advanceMoveTimes []float64
	checkPauseCnt    int
}

type fakeToolheadSetPositionRuntime struct {
	flushCount     int
	printTime      float64
	commandedPos   []float64
	kinematicPos   []float64
	homingAxes     []int
	emitCount      int
	trapqPrintTime float64
	trapqPosition  []float64
}

func (self *fakeToolheadSetPositionRuntime) FlushStepGeneration() {
	self.flushCount++
}

func (self *fakeToolheadSetPositionRuntime) PrintTime() float64 {
	return self.printTime
}

func (self *fakeToolheadSetPositionRuntime) SetCommandedPosition(position []float64) {
	self.commandedPos = append([]float64{}, position...)
}

func (self *fakeToolheadSetPositionRuntime) SetKinematicPosition(newpos []float64, homingAxes []int) {
	self.kinematicPos = append([]float64{}, newpos...)
	self.homingAxes = append([]int{}, homingAxes...)
}

func (self *fakeToolheadSetPositionRuntime) EmitSetPositionEvent() {
	self.emitCount++
}

type fakeToolheadManualMoveRuntime struct {
	commandedPos []float64
	submittedPos []float64
	submittedSpd float64
	emitCount    int
}

func (self *fakeToolheadManualMoveRuntime) CommandedPosition() []float64 {
	return append([]float64{}, self.commandedPos...)
}

func (self *fakeToolheadManualMoveRuntime) SubmitMove(newpos []float64, speed float64) {
	self.submittedPos = append([]float64{}, newpos...)
	self.submittedSpd = speed
}

func (self *fakeToolheadManualMoveRuntime) EmitManualMoveEvent() {
	self.emitCount++
}

func (self *fakeToolheadDwellRuntime) GetLastMoveTime() float64 {
	return self.lastMoveTime
}

func (self *fakeToolheadDwellRuntime) AdvanceMoveTime(nextPrintTime float64) {
	self.advanceMoveTimes = append(self.advanceMoveTimes, nextPrintTime)
}

func (self *fakeToolheadDwellRuntime) CheckPause() {
	self.checkPauseCnt++
}

func testToolheadMoveConfig() MoveConfig {
	return MoveConfig{
		Max_accel:          100.0,
		Junction_deviation: 0.1,
		Max_velocity:       50.0,
		Max_accel_to_decel: 25.0,
	}
}

func TestRunToolheadMoveQueuesMoveAndChecksPause(t *testing.T) {
	runtime := &fakeToolheadMoveRuntime{
		commandedPos:   []float64{0, 0, 0, 0},
		config:         testToolheadMoveConfig(),
		printTime:      10.0,
		needCheckPause: 5.0,
	}

	moved := RunToolheadMove(runtime, []float64{3, 4, 0, 1}, 20.0)

	if !moved {
		t.Fatal("expected move to be queued")
	}
	if len(runtime.queuedMoves) != 1 || len(runtime.kinChecks) != 1 || len(runtime.extraChecks) != 1 {
		t.Fatalf("unexpected move callbacks queued=%d kin=%d extruder=%d", len(runtime.queuedMoves), len(runtime.kinChecks), len(runtime.extraChecks))
	}
	if runtime.checkPauseCnt != 1 {
		t.Fatalf("expected pause check, got %d", runtime.checkPauseCnt)
	}
	if !almostEqualFloat64(runtime.commandedPos[0], 3.0) || !almostEqualFloat64(runtime.commandedPos[1], 4.0) || !almostEqualFloat64(runtime.commandedPos[3], 1.0) {
		t.Fatalf("unexpected commanded position %#v", runtime.commandedPos)
	}
}

func TestRunToolheadMoveSkipsZeroDistance(t *testing.T) {
	runtime := &fakeToolheadMoveRuntime{commandedPos: []float64{1, 2, 3, 4}, config: testToolheadMoveConfig()}

	moved := RunToolheadMove(runtime, []float64{1, 2, 3, 4}, 20.0)

	if moved {
		t.Fatal("expected zero-distance move to be ignored")
	}
	if len(runtime.queuedMoves) != 0 || len(runtime.kinChecks) != 0 || len(runtime.extraChecks) != 0 || runtime.checkPauseCnt != 0 {
		t.Fatalf("unexpected side effects %#v", runtime)
	}
}

func TestBuildToolheadManualMoveTargetAppliesOverrides(t *testing.T) {
	target := BuildToolheadManualMoveTarget([]float64{1, 2, 3, 4}, []interface{}{nil, 8.0, nil, 9.0, 10.0})

	if len(target) != 5 {
		t.Fatalf("unexpected target length %d", len(target))
	}
	if !almostEqualFloat64(target[0], 1.0) || !almostEqualFloat64(target[1], 8.0) || !almostEqualFloat64(target[2], 3.0) || !almostEqualFloat64(target[3], 9.0) || !almostEqualFloat64(target[4], 10.0) {
		t.Fatalf("unexpected manual move target %#v", target)
	}
}

func TestApplyToolheadSetPositionFlushesUpdatesAndNotifies(t *testing.T) {
	runtime := &fakeToolheadSetPositionRuntime{printTime: 12.5}

	ApplyToolheadSetPosition(runtime, []float64{1, 2, 3, 4}, []int{0, 2}, func(printTime float64, newpos []float64) {
		runtime.trapqPrintTime = printTime
		runtime.trapqPosition = append([]float64{}, newpos...)
	})

	if runtime.flushCount != 1 {
		t.Fatalf("expected one flush, got %d", runtime.flushCount)
	}
	if !almostEqualFloat64(runtime.trapqPrintTime, 12.5) {
		t.Fatalf("unexpected trapq print time %v", runtime.trapqPrintTime)
	}
	if len(runtime.trapqPosition) != 4 || !almostEqualFloat64(runtime.trapqPosition[2], 3.0) {
		t.Fatalf("unexpected trapq position %#v", runtime.trapqPosition)
	}
	if len(runtime.commandedPos) != 4 || !almostEqualFloat64(runtime.commandedPos[3], 4.0) {
		t.Fatalf("unexpected commanded position %#v", runtime.commandedPos)
	}
	if len(runtime.kinematicPos) != 4 || len(runtime.homingAxes) != 2 || runtime.homingAxes[1] != 2 {
		t.Fatalf("unexpected kinematic update pos=%#v axes=%#v", runtime.kinematicPos, runtime.homingAxes)
	}
	if runtime.emitCount != 1 {
		t.Fatalf("expected one set-position event, got %d", runtime.emitCount)
	}
}

func TestRunToolheadManualMoveBuildsTargetAndEmitsEvent(t *testing.T) {
	runtime := &fakeToolheadManualMoveRuntime{commandedPos: []float64{1, 2, 3, 4}}

	RunToolheadManualMove(runtime, []interface{}{nil, 8.0, nil, 9.0}, 42.0)

	if len(runtime.submittedPos) != 4 {
		t.Fatalf("unexpected submitted position %#v", runtime.submittedPos)
	}
	if !almostEqualFloat64(runtime.submittedPos[0], 1.0) || !almostEqualFloat64(runtime.submittedPos[1], 8.0) || !almostEqualFloat64(runtime.submittedPos[2], 3.0) || !almostEqualFloat64(runtime.submittedPos[3], 9.0) {
		t.Fatalf("unexpected submitted position %#v", runtime.submittedPos)
	}
	if !almostEqualFloat64(runtime.submittedSpd, 42.0) {
		t.Fatalf("unexpected submitted speed %v", runtime.submittedSpd)
	}
	if runtime.emitCount != 1 {
		t.Fatalf("expected one manual-move event, got %d", runtime.emitCount)
	}
}

func TestRunToolheadDwellClampsNegativeDelay(t *testing.T) {
	runtime := &fakeToolheadDwellRuntime{lastMoveTime: 7.5}

	RunToolheadDwell(runtime, -2.0)

	if len(runtime.advanceMoveTimes) != 1 || !almostEqualFloat64(runtime.advanceMoveTimes[0], 7.5) {
		t.Fatalf("unexpected advance times %#v", runtime.advanceMoveTimes)
	}
	if runtime.checkPauseCnt != 1 {
		t.Fatalf("expected pause check, got %d", runtime.checkPauseCnt)
	}
}

func TestApplyToolheadVelocityLimitUpdate(t *testing.T) {
	maxVelocity := 150.0
	requestedAccelToDecel := 35.0
	result := ApplyToolheadVelocityLimitUpdate(ToolheadVelocitySettings{
		MaxVelocity:           100.0,
		MaxAccel:              40.0,
		RequestedAccelToDecel: 20.0,
		SquareCornerVelocity:  5.0,
	}, ToolheadVelocityLimitUpdate{
		MaxVelocity:           &maxVelocity,
		RequestedAccelToDecel: &requestedAccelToDecel,
	})

	if !almostEqualFloat64(result.Settings.MaxVelocity, 150.0) || !almostEqualFloat64(result.Settings.MaxAccel, 40.0) {
		t.Fatalf("unexpected velocity settings %#v", result.Settings)
	}
	if !almostEqualFloat64(result.MaxAccelToDecel, 35.0) {
		t.Fatalf("unexpected max accel to decel %v", result.MaxAccelToDecel)
	}
	if result.Summary == "" {
		t.Fatal("expected summary string")
	}
}
