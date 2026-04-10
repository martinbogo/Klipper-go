package homing

import "testing"

type fakeCompletion struct {
	result interface{}
}

func (self *fakeCompletion) Wait(waketime float64, waketimeResult interface{}) interface{} {
	_ = waketime
	if self.result == nil {
		return waketimeResult
	}
	return self.result
}

func (self *fakeCompletion) Complete(result interface{}) {
	self.result = result
}

type fakeReactor struct {
	callbacks int
}

func (self *fakeReactor) RegisterCallback(callback func(interface{}) interface{}, waketime float64) Completion {
	_ = callback
	_ = waketime
	self.callbacks++
	return &fakeCompletion{}
}

type fakeStepper struct {
	name              string
	currentMCUPos     int
	pastPositions     map[float64]int
	stepDist          float64
	commandedPosition float64
}

func (self *fakeStepper) GetName(short bool) string {
	_ = short
	return self.name
}

func (self *fakeStepper) GetMCUPosition() int {
	return self.currentMCUPos
}

func (self *fakeStepper) GetPastMCUPosition(printTime float64) int {
	if pos, ok := self.pastPositions[printTime]; ok {
		return pos
	}
	return self.currentMCUPos
}

func (self *fakeStepper) CalcPositionFromCoord(coord []float64) float64 {
	if len(coord) == 0 {
		return 0
	}
	return coord[0]
}

func (self *fakeStepper) GetStepDist() float64 {
	if self.stepDist == 0 {
		return 1
	}
	return self.stepDist
}

func (self *fakeStepper) GetCommandedPosition() float64 {
	return self.commandedPosition
}

type fakeKinematics struct {
	steppers []Stepper
}

func (self *fakeKinematics) GetSteppers() []Stepper {
	return append([]Stepper{}, self.steppers...)
}

func (self *fakeKinematics) CalcPosition(stepperPositions map[string]float64) []float64 {
	return []float64{stepperPositions["x"], stepperPositions["y"], stepperPositions["z"]}
}

type fakeDripToolhead struct {
	position         []float64
	kinematics       Kinematics
	lastMoveTime     float64
	flushCalls       int
	dwellCalls       []float64
	dripCalls        [][]float64
	setPositionCalls [][]float64
	moveCalls        [][]float64
	homingAxes       [][]int
	dripHook         func([]float64, float64)
}

func (self *fakeDripToolhead) GetPosition() []float64 {
	return append([]float64{}, self.position...)
}

func (self *fakeDripToolhead) GetKinematics() Kinematics {
	return self.kinematics
}

func (self *fakeDripToolhead) FlushStepGeneration() {
	self.flushCalls++
}

func (self *fakeDripToolhead) GetLastMoveTime() float64 {
	return self.lastMoveTime
}

func (self *fakeDripToolhead) Dwell(delay float64) {
	self.dwellCalls = append(self.dwellCalls, delay)
}

func (self *fakeDripToolhead) DripMove(newpos []float64, speed float64, dripCompletion Completion) error {
	_ = speed
	self.dripCalls = append(self.dripCalls, append([]float64{}, newpos...))
	if self.dripHook != nil {
		self.dripHook(newpos, speed)
	}
	self.position = append([]float64{}, newpos...)
	if dripCompletion != nil {
		dripCompletion.Complete(nil)
	}
	return nil
}

func (self *fakeDripToolhead) SetPosition(newpos []float64, homingAxes []int) {
	self.position = append([]float64{}, newpos...)
	self.setPositionCalls = append(self.setPositionCalls, append([]float64{}, newpos...))
	self.homingAxes = append(self.homingAxes, append([]int{}, homingAxes...))
}

func (self *fakeDripToolhead) Move(newpos []float64, speed float64) {
	_ = speed
	self.position = append([]float64{}, newpos...)
	self.moveCalls = append(self.moveCalls, append([]float64{}, newpos...))
}

type homeStartCall struct {
	printTime   float64
	sampleTime  float64
	sampleCount int64
	restTime    float64
	triggered   int64
}

type fakeEndstop struct {
	steppers       []Stepper
	triggerTime    float64
	homeStartCalls []homeStartCall
	completion     Completion
}

func (self *fakeEndstop) GetSteppers() []Stepper {
	return append([]Stepper{}, self.steppers...)
}

func (self *fakeEndstop) HomeStart(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64) Completion {
	self.homeStartCalls = append(self.homeStartCalls, homeStartCall{
		printTime:   printTime,
		sampleTime:  sampleTime,
		sampleCount: sampleCount,
		restTime:    restTime,
		triggered:   triggered,
	})
	if self.completion == nil {
		self.completion = &fakeCompletion{}
	}
	return self.completion
}

func (self *fakeEndstop) HomeWait(moveEndPrintTime float64) float64 {
	_ = moveEndPrintTime
	return self.triggerTime
}

type fakeRail struct {
	endstops []NamedEndstop
	info     *RailHomingInfo
}

func (self *fakeRail) GetEndstops() []NamedEndstop {
	return append([]NamedEndstop{}, self.endstops...)
}

func (self *fakeRail) GetHomingInfo() *RailHomingInfo {
	copied := *self.info
	return &copied
}

type fakeMoveExecutor struct {
	executeCalls    int
	triggerPos      []float64
	triggerTime     float64
	stepperPosition []*StepperPosition
	checkNoMovement string
}

func (self *fakeMoveExecutor) Execute(movepos []float64, speed float64, probePos bool, triggered bool, checkTriggered bool) ([]float64, float64, error) {
	_, _, _, _, _ = movepos, speed, probePos, triggered, checkTriggered
	self.executeCalls++
	return append([]float64{}, self.triggerPos...), self.triggerTime, nil
}

func (self *fakeMoveExecutor) CheckNoMovement() string {
	return self.checkNoMovement
}

func (self *fakeMoveExecutor) StepperPositions() []*StepperPosition {
	return append([]*StepperPosition{}, self.stepperPosition...)
}

func TestMoveExecuteReturnsTriggerPositionAndUpdatesToolhead(t *testing.T) {
	stepper := &fakeStepper{name: "x", stepDist: 1, pastPositions: map[float64]int{1.5: 10}}
	toolhead := &fakeDripToolhead{
		position:     []float64{0, 0, 0, 0},
		kinematics:   &fakeKinematics{steppers: []Stepper{stepper}},
		lastMoveTime: 2.0,
		dripHook: func(newpos []float64, speed float64) {
			_ = newpos
			_ = speed
			stepper.currentMCUPos = 10
			stepper.commandedPosition = 5
		},
	}
	endstop := &fakeEndstop{steppers: []Stepper{stepper}, triggerTime: 1.5}
	move := NewMove(&fakeReactor{}, toolhead, []NamedEndstop{{Endstop: endstop, Name: "x_endstop"}}, false)

	triggerPos, triggerTime, err := move.Execute([]float64{5, 0, 0, 0}, 25, false, true, true)
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if triggerTime != 1.5 {
		t.Fatalf("expected trigger time 1.5, got %.3f", triggerTime)
	}
	if len(triggerPos) != 4 || triggerPos[0] != 5 {
		t.Fatalf("unexpected trigger position %#v", triggerPos)
	}
	if len(toolhead.setPositionCalls) != 1 || toolhead.setPositionCalls[0][0] != 5 {
		t.Fatalf("expected final toolhead position to be set to the move target, got %#v", toolhead.setPositionCalls)
	}
	if len(endstop.homeStartCalls) != 1 {
		t.Fatalf("expected one home start call, got %d", len(endstop.homeStartCalls))
	}
	if endstop.homeStartCalls[0].sampleTime != EndstopSampleTime || endstop.homeStartCalls[0].sampleCount != EndstopSampleCount {
		t.Fatalf("unexpected endstop sampling config %#v", endstop.homeStartCalls[0])
	}
	if len(move.StepperPositions()) != 1 || move.StepperPositions()[0].TrigPos != 10 {
		t.Fatalf("unexpected stepper positions %#v", move.StepperPositions())
	}
}

func TestMoveCheckNoMovementHonorsDebugInput(t *testing.T) {
	newMoveWithDebug := func(isDebug bool) *Move {
		stepper := &fakeStepper{name: "x", stepDist: 1, pastPositions: map[float64]int{1.5: 0}}
		toolhead := &fakeDripToolhead{
			position:     []float64{0, 0, 0, 0},
			kinematics:   &fakeKinematics{steppers: []Stepper{stepper}},
			lastMoveTime: 2.0,
		}
		endstop := &fakeEndstop{steppers: []Stepper{stepper}, triggerTime: 1.5}
		return NewMove(&fakeReactor{}, toolhead, []NamedEndstop{{Endstop: endstop, Name: "x_endstop"}}, isDebug)
	}

	move := newMoveWithDebug(false)
	if _, _, err := move.Execute([]float64{5, 0, 0, 0}, 25, false, true, true); err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if got := move.CheckNoMovement(); got != "x_endstop" {
		t.Fatalf("expected no-movement endstop name, got %q", got)
	}

	moveDebug := newMoveWithDebug(true)
	if _, _, err := moveDebug.Execute([]float64{5, 0, 0, 0}, 25, false, true, true); err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if got := moveDebug.CheckNoMovement(); got != "" {
		t.Fatalf("expected debug mode to suppress no-movement error, got %q", got)
	}
}

func TestStateHomeRailsAppliesStepperAdjustmentsAfterCallback(t *testing.T) {
	stepper := &fakeStepper{name: "x", stepDist: 1, commandedPosition: 10}
	toolhead := &fakeDripToolhead{
		position:     []float64{0, 0, 0, 0},
		kinematics:   &fakeKinematics{steppers: []Stepper{stepper}},
		lastMoveTime: 2.0,
	}
	state := NewState(toolhead)
	rail := &fakeRail{
		endstops: []NamedEndstop{{Endstop: &fakeEndstop{}, Name: "x_endstop"}},
		info: &RailHomingInfo{
			Speed:             50,
			PositionEndstop:   10,
			RetractSpeed:      20,
			RetractDist:       0,
			PositiveDir:       false,
			SecondHomingSpeed: 25,
		},
	}
	move := &fakeMoveExecutor{
		triggerPos:  []float64{10, 0, 0, 0},
		triggerTime: 1.25,
		stepperPosition: []*StepperPosition{{
			Stepper:     stepper,
			EndstopName: "x_endstop",
			StepperName: "x",
			TrigPos:     42,
		}},
	}

	err := state.HomeRailsWithPositions(
		[]Rail{rail},
		[]interface{}{float64(-5), nil, nil, nil},
		[]interface{}{float64(10), nil, nil, nil},
		func(endstops []NamedEndstop) MoveExecutor {
			if len(endstops) != 1 || endstops[0].Name != "x_endstop" {
				t.Fatalf("unexpected endstops %#v", endstops)
			}
			return move
		},
		func() {
			state.SetStepperAdjustment("x", 2)
		},
	)
	if err != nil {
		t.Fatalf("unexpected home rails error: %v", err)
	}
	if move.executeCalls != 1 {
		t.Fatalf("expected one homing move, got %d", move.executeCalls)
	}
	if got := state.GetTriggerPosition("x"); got != 42 {
		t.Fatalf("expected trigger position 42, got %.1f", got)
	}
	if len(toolhead.setPositionCalls) != 3 {
		t.Fatalf("expected start, home, and adjusted positions, got %#v", toolhead.setPositionCalls)
	}
	if last := toolhead.setPositionCalls[len(toolhead.setPositionCalls)-1]; last[0] != 12 {
		t.Fatalf("expected adjusted X position 12, got %#v", last)
	}
}
