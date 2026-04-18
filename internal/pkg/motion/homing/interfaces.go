package homing

import "goklipper/common/constants"

const (
	HomingStartDelay   = 0.001
	EndstopSampleTime  = .000015
	EndstopSampleCount = 4
)

type Completion interface {
	Wait(waketime float64, waketimeResult interface{}) interface{}
	Complete(result interface{})
}

type Reactor interface {
	RegisterCallback(callback func(interface{}) interface{}, waketime float64) Completion
}

type ReactorFuncs struct {
	RegisterCallbackFunc func(callback func(interface{}) interface{}, waketime float64) Completion
}

func (self *ReactorFuncs) RegisterCallback(callback func(interface{}) interface{}, waketime float64) Completion {
	return self.RegisterCallbackFunc(callback, waketime)
}

type Stepper interface {
	GetName(short bool) string
	GetMCUPosition() int
	GetPastMCUPosition(printTime float64) int
	CalcPositionFromCoord(coord []float64) float64
	GetStepDist() float64
	GetCommandedPosition() float64
}

type StepperFuncs struct {
	GetNameFunc               func(short bool) string
	GetMCUPositionFunc        func() int
	GetPastMCUPositionFunc    func(printTime float64) int
	CalcPositionFromCoordFunc func(coord []float64) float64
	GetStepDistFunc           func() float64
	GetCommandedPositionFunc  func() float64
}

func (self *StepperFuncs) GetName(short bool) string {
	return self.GetNameFunc(short)
}

func (self *StepperFuncs) GetMCUPosition() int {
	return self.GetMCUPositionFunc()
}

func (self *StepperFuncs) GetPastMCUPosition(printTime float64) int {
	return self.GetPastMCUPositionFunc(printTime)
}

func (self *StepperFuncs) CalcPositionFromCoord(coord []float64) float64 {
	return self.CalcPositionFromCoordFunc(coord)
}

func (self *StepperFuncs) GetStepDist() float64 {
	return self.GetStepDistFunc()
}

func (self *StepperFuncs) GetCommandedPosition() float64 {
	return self.GetCommandedPositionFunc()
}

type Kinematics interface {
	GetSteppers() []Stepper
	CalcPosition(stepperPositions map[string]float64) []float64
}

type KinematicsFuncs struct {
	GetSteppersFunc  func() []Stepper
	CalcPositionFunc func(stepperPositions map[string]float64) []float64
}

func (self *KinematicsFuncs) GetSteppers() []Stepper {
	return self.GetSteppersFunc()
}

func (self *KinematicsFuncs) CalcPosition(stepperPositions map[string]float64) []float64 {
	return self.CalcPositionFunc(stepperPositions)
}

type DripToolhead interface {
	GetPosition() []float64
	GetKinematics() Kinematics
	FlushStepGeneration()
	GetLastMoveTime() float64
	Dwell(delay float64)
	DripMove(newpos []float64, speed float64, dripCompletion Completion) error
	SetPosition(newpos []float64, homingAxes []int)
}

type Toolhead interface {
	DripToolhead
	Move(newpos []float64, speed float64)
}

type ToolheadFuncs struct {
	GetPositionFunc         func() []float64
	GetKinematicsFunc       func() Kinematics
	FlushStepGenerationFunc func()
	GetLastMoveTimeFunc     func() float64
	DwellFunc               func(delay float64)
	DripMoveFunc            func(newpos []float64, speed float64, dripCompletion Completion) error
	MoveFunc                func(newpos []float64, speed float64)
	SetPositionFunc         func(newpos []float64, homingAxes []int)
}

func (self *ToolheadFuncs) GetPosition() []float64 {
	return self.GetPositionFunc()
}

func (self *ToolheadFuncs) GetKinematics() Kinematics {
	return self.GetKinematicsFunc()
}

func (self *ToolheadFuncs) FlushStepGeneration() {
	self.FlushStepGenerationFunc()
}

func (self *ToolheadFuncs) GetLastMoveTime() float64 {
	return self.GetLastMoveTimeFunc()
}

func (self *ToolheadFuncs) Dwell(delay float64) {
	self.DwellFunc(delay)
}

func (self *ToolheadFuncs) DripMove(newpos []float64, speed float64, dripCompletion Completion) error {
	return self.DripMoveFunc(newpos, speed, dripCompletion)
}

func (self *ToolheadFuncs) Move(newpos []float64, speed float64) {
	self.MoveFunc(newpos, speed)
}

func (self *ToolheadFuncs) SetPosition(newpos []float64, homingAxes []int) {
	self.SetPositionFunc(newpos, homingAxes)
}

type Endstop interface {
	GetSteppers() []Stepper
	HomeStart(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64) Completion
	HomeWait(moveEndPrintTime float64) float64
}

type EndstopFuncs struct {
	GetSteppersFunc func() []Stepper
	HomeStartFunc   func(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64) Completion
	HomeWaitFunc    func(moveEndPrintTime float64) float64
}

func (self *EndstopFuncs) GetSteppers() []Stepper {
	return self.GetSteppersFunc()
}

func (self *EndstopFuncs) HomeStart(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64) Completion {
	return self.HomeStartFunc(printTime, sampleTime, sampleCount, restTime, triggered)
}

func (self *EndstopFuncs) HomeWait(moveEndPrintTime float64) float64 {
	return self.HomeWaitFunc(moveEndPrintTime)
}

type NamedEndstop struct {
	Endstop Endstop
	Name    string
}

type RailHomingInfo struct {
	Speed             float64
	PositionEndstop   float64
	RetractSpeed      float64
	RetractDist       float64
	PositiveDir       bool
	SecondHomingSpeed float64
}

type Rail interface {
	GetEndstops() []NamedEndstop
	GetHomingInfo() *RailHomingInfo
}

type RailFuncs struct {
	GetEndstopsFunc   func() []NamedEndstop
	GetHomingInfoFunc func() *RailHomingInfo
}

func (self *RailFuncs) GetEndstops() []NamedEndstop {
	return self.GetEndstopsFunc()
}

func (self *RailFuncs) GetHomingInfo() *RailHomingInfo {
	return self.GetHomingInfoFunc()
}

type MoveExecutor interface {
	Execute(movepos []float64, speed float64, probePos bool, triggered bool, checkTriggered bool) ([]float64, float64, error)
	CheckNoMovement() string
	StepperPositions() []*StepperPosition
}

type MoveExecutorFuncs struct {
	ExecuteFunc          func(movepos []float64, speed float64, probePos bool, triggered bool, checkTriggered bool) ([]float64, float64, error)
	CheckNoMovementFunc  func() string
	StepperPositionsFunc func() []*StepperPosition
}

func (self *MoveExecutorFuncs) Execute(movepos []float64, speed float64, probePos bool, triggered bool, checkTriggered bool) ([]float64, float64, error) {
	return self.ExecuteFunc(movepos, speed, probePos, triggered, checkTriggered)
}

func (self *MoveExecutorFuncs) CheckNoMovement() string {
	return self.CheckNoMovementFunc()
}

func (self *MoveExecutorFuncs) StepperPositions() []*StepperPosition {
	return self.StepperPositionsFunc()
}

func MultiComplete(reactor Reactor, completions []Completion) Completion {
	if len(completions) == 1 {
		return completions[0]
	}
	cp := reactor.RegisterCallback(func(interface{}) interface{} {
		for _, completion := range completions {
			completion.Wait(constants.NEVER, nil)
		}
		return nil
	}, constants.NOW)
	for _, completion := range completions {
		completion := completion
		reactor.RegisterCallback(func(interface{}) interface{} {
			if completion.Wait(constants.NEVER, nil) != nil {
				cp.Complete(1)
			}
			return nil
		}, constants.NOW)
	}
	return cp
}
