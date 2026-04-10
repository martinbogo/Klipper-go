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

type Stepper interface {
	GetName(short bool) string
	GetMCUPosition() int
	GetPastMCUPosition(printTime float64) int
	CalcPositionFromCoord(coord []float64) float64
	GetStepDist() float64
	GetCommandedPosition() float64
}

type Kinematics interface {
	GetSteppers() []Stepper
	CalcPosition(stepperPositions map[string]float64) []float64
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

type Endstop interface {
	GetSteppers() []Stepper
	HomeStart(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64) Completion
	HomeWait(moveEndPrintTime float64) float64
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

type MoveExecutor interface {
	Execute(movepos []float64, speed float64, probePos bool, triggered bool, checkTriggered bool) ([]float64, float64, error)
	CheckNoMovement() string
	StepperPositions() []*StepperPosition
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
