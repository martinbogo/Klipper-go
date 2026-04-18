package homing

import (
	"fmt"
	"math"
	"reflect"
)

type StepperPosition struct {
	Stepper     Stepper
	EndstopName string
	StepperName string
	StartPos    int
	HaltPos     int
	TrigPos     int
}

func NewStepperPosition(stepper Stepper, endstopName string) *StepperPosition {
	return &StepperPosition{
		Stepper:     stepper,
		EndstopName: endstopName,
		StepperName: stepper.GetName(false),
		StartPos:    stepper.GetMCUPosition(),
	}
}

func (self *StepperPosition) NoteHomeEnd(triggerTime float64) {
	self.HaltPos = self.Stepper.GetMCUPosition()
	self.TrigPos = self.Stepper.GetPastMCUPosition(triggerTime)
}

type Move struct {
	reactor          Reactor
	isDebugInput     bool
	endstops         []NamedEndstop
	toolhead         DripToolhead
	stepperPositions []*StepperPosition
}

func NewMove(reactor Reactor, toolhead DripToolhead, endstops []NamedEndstop, isDebugInput bool) *Move {
	return &Move{
		reactor:          reactor,
		isDebugInput:     isDebugInput,
		endstops:         append([]NamedEndstop{}, endstops...),
		toolhead:         toolhead,
		stepperPositions: []*StepperPosition{},
	}
}

func (self *Move) StepperPositions() []*StepperPosition {
	return self.stepperPositions
}

func (self *Move) CalcEndstopRate(endstop Endstop, movepos []float64, speed float64) float64 {
	startpos := self.toolhead.GetPosition()
	axesD := make([]float64, len(startpos))
	for i, value := range startpos {
		axesD[i] = movepos[i] - value
	}
	sum := 0.0
	for _, delta := range axesD[:3] {
		sum += delta * delta
	}
	moveD := math.Sqrt(sum)
	moveT := moveD / speed
	maxSteps := 0.0
	for _, stepper := range endstop.GetSteppers() {
		stepCount := math.Abs(stepper.CalcPositionFromCoord(startpos)-stepper.CalcPositionFromCoord(movepos)) / stepper.GetStepDist()
		maxSteps = math.Max(stepCount, maxSteps)
	}
	if maxSteps <= 0 {
		return .001
	}
	return moveT / maxSteps
}

func (self *Move) CalcToolheadPos(kinSPosInput map[string]float64, offsets map[string]float64) []float64 {
	kinSPos := make(map[string]float64, len(kinSPosInput))
	for key, value := range kinSPosInput {
		kinSPos[key] = value
	}
	kin := self.toolhead.GetKinematics()
	for _, stepper := range kin.GetSteppers() {
		name := stepper.GetName(false)
		kinSPos[name] += offsets[name] * stepper.GetStepDist()
	}
	toolheadPos := self.toolhead.GetPosition()
	position := append([]float64{}, kin.CalcPosition(kinSPos)[:3]...)
	position = append(position, toolheadPos[3:]...)
	return position
}

func (self *Move) Execute(movepos []float64, speed float64, probePos bool, triggered bool, checkTriggered bool) ([]float64, float64, error) {
	self.stepperPositions = []*StepperPosition{}
	self.toolhead.FlushStepGeneration()
	kin := self.toolhead.GetKinematics()
	kinSPos := map[string]float64{}
	for _, stepper := range kin.GetSteppers() {
		kinSPos[stepper.GetName(false)] = stepper.GetCommandedPosition()
	}
	for _, namedEndstop := range self.endstops {
		for _, stepper := range namedEndstop.Endstop.GetSteppers() {
			self.stepperPositions = append(self.stepperPositions, NewStepperPosition(stepper, namedEndstop.Name))
		}
	}
	printTime := self.toolhead.GetLastMoveTime()
	endstopTriggers := make([]Completion, 0, len(self.endstops))
	triggeredValue := int64(0)
	if triggered {
		triggeredValue = 1
	}
	for _, namedEndstop := range self.endstops {
		restTime := self.CalcEndstopRate(namedEndstop.Endstop, movepos, speed)
		endstopTriggers = append(endstopTriggers, namedEndstop.Endstop.HomeStart(printTime, EndstopSampleTime, EndstopSampleCount, restTime, triggeredValue))
	}
	allEndstopTrigger := MultiComplete(self.reactor, endstopTriggers)
	self.toolhead.Dwell(HomingStartDelay)
	if err := self.toolhead.DripMove(movepos, speed, allEndstopTrigger); err != nil {
		return nil, 0, fmt.Errorf("error during homing move: %w", err)
	}
	triggerTimes := map[string]float64{}
	triggerTime := 0.0
	moveEndPrintTime := self.toolhead.GetLastMoveTime()
	var executeErr error
	for _, namedEndstop := range self.endstops {
		triggerTime = namedEndstop.Endstop.HomeWait(moveEndPrintTime)
		if triggerTime > 0 {
			triggerTimes[namedEndstop.Name] = triggerTime
		} else if triggerTime < 0 && executeErr == nil {
			executeErr = fmt.Errorf("communication timeout during homing %s", namedEndstop.Name)
		} else if checkTriggered && executeErr == nil {
			executeErr = fmt.Errorf("no trigger on %s after full movement", namedEndstop.Name)
		}
	}
	self.toolhead.FlushStepGeneration()
	for _, position := range self.stepperPositions {
		timestamp := moveEndPrintTime
		if triggerTimestamp, ok := triggerTimes[position.EndstopName]; ok {
			timestamp = triggerTimestamp
		}
		position.NoteHomeEnd(timestamp)
	}
	haltPos := []float64{}
	triggerPos := []float64{}
	if probePos {
		haltSteps := map[string]float64{}
		triggerSteps := map[string]float64{}
		for _, position := range self.stepperPositions {
			haltSteps[position.StepperName] = float64(position.HaltPos - position.StartPos)
			triggerSteps[position.StepperName] = float64(position.TrigPos - position.StartPos)
		}
		haltPos = self.CalcToolheadPos(kinSPos, triggerSteps)
		triggerPos = haltPos
		if !reflect.DeepEqual(triggerSteps, haltSteps) {
			haltPos = self.CalcToolheadPos(kinSPos, haltSteps)
		}
	} else {
		haltPos = append([]float64{}, movepos...)
		triggerPos = append([]float64{}, movepos...)
		overSteps := map[string]float64{}
		for _, position := range self.stepperPositions {
			overSteps[position.StepperName] = float64(position.HaltPos - position.TrigPos)
		}
		hasOffsets := false
		for _, value := range overSteps {
			if value != 0 {
				hasOffsets = true
				break
			}
		}
		if hasOffsets {
			self.toolhead.SetPosition(movepos, []int{})
			haltKinSPos := map[string]float64{}
			for _, stepper := range kin.GetSteppers() {
				haltKinSPos[stepper.GetName(false)] = stepper.GetCommandedPosition()
			}
			haltPos = self.CalcToolheadPos(haltKinSPos, overSteps)
		}
	}
	self.toolhead.SetPosition(haltPos, []int{})
	return triggerPos, triggerTime, executeErr
}

func (self *Move) CheckNoMovement() string {
	if self.isDebugInput {
		return ""
	}
	for _, position := range self.stepperPositions {
		if position.StartPos == position.TrigPos {
			return position.EndstopName
		}
	}
	return ""
}
