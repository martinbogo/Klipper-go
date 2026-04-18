package homing

import (
	"fmt"
	"goklipper/common/logger"
	"math"
)

type State struct {
	toolhead      Toolhead
	changedAxes   []int
	triggerMCUPos map[string]float64
	adjustPos     map[string]float64
}

func NewState(toolhead Toolhead) *State {
	return &State{
		toolhead:      toolhead,
		changedAxes:   []int{},
		triggerMCUPos: map[string]float64{},
		adjustPos:     map[string]float64{},
	}
}

func (self *State) SetAxes(axes []int) {
	self.changedAxes = append([]int{}, axes...)
}

func (self *State) GetAxes() []int {
	return append([]int{}, self.changedAxes...)
}

func (self *State) GetTriggerPosition(stepperName string) float64 {
	return self.triggerMCUPos[stepperName]
}

func (self *State) SetStepperAdjustment(stepperName string, adjustment float64) {
	self.adjustPos[stepperName] = adjustment
}

func (self *State) TriggerPositions() map[string]float64 {
	positions := make(map[string]float64, len(self.triggerMCUPos))
	for key, value := range self.triggerMCUPos {
		positions[key] = value
	}
	return positions
}

func (self *State) Adjustments() map[string]float64 {
	adjustments := make(map[string]float64, len(self.adjustPos))
	for key, value := range self.adjustPos {
		adjustments[key] = value
	}
	return adjustments
}

func (self *State) FillCoord(coord []interface{}) []float64 {
	position := self.toolhead.GetPosition()
	filled := make([]float64, len(position))
	copy(filled, position)
	for i, value := range coord {
		if value != nil {
			filled[i] = value.(float64)
		}
	}
	return filled
}

func (self *State) SetHomedPosition(pos []float64) {
	self.toolhead.SetPosition(append([]float64{}, pos...), []int{})
}

func (self *State) HomeRailsWithPositions(rails []Rail, forcepos []interface{}, movepos []interface{}, newMove func([]NamedEndstop) MoveExecutor, afterHome func() error) error {
	if len(rails) == 0 {
		return nil
	}
	homingAxes := []int{}
	for axis := 0; axis < 3; axis++ {
		if forcepos[axis] != nil {
			homingAxes = append(homingAxes, axis)
		}
	}
	startpos := self.FillCoord(forcepos)
	homepos := self.FillCoord(movepos)
	self.toolhead.SetPosition(startpos, homingAxes)
	endstops := []NamedEndstop{}
	for _, rail := range rails {
		endstops = append(endstops, rail.GetEndstops()...)
	}
	hi := rails[0].GetHomingInfo()
	hmove := newMove(endstops)
	if _, _, err := hmove.Execute(homepos, hi.Speed, false, true, true); err != nil {
		return err
	}
	if hi.RetractDist > 0 {
		homingRetryCount := 1
		triggerTimes := make([]float64, 3)
		if forcepos[2] != nil {
			homingRetryCount = 10
		}
		for i := 0; i < homingRetryCount; i++ {
			startpos = self.FillCoord(forcepos)
			homepos = self.FillCoord(movepos)
			axesD := make([]float64, len(homepos))
			for j, hp := range homepos {
				axesD[j] = hp - startpos[j]
			}
			sum := 0.0
			for _, delta := range axesD[:3] {
				sum += delta * delta
			}
			moveD := math.Sqrt(sum)
			retractR := math.Min(1., hi.RetractDist/moveD)
			retractpos := make([]float64, len(homepos))
			for j, hp := range homepos {
				retractpos[j] = hp - axesD[j]*retractR
			}
			self.toolhead.Move(retractpos, hi.RetractSpeed)
			self.toolhead.SetPosition(startpos, []int{})
			hmove = newMove(endstops)
			_, triggerTime, err := hmove.Execute(homepos, hi.SecondHomingSpeed, false, true, true)
			if err != nil {
				return err
			}
			if endstopName := hmove.CheckNoMovement(); endstopName != "" {
				return fmt.Errorf("endstop %s still triggered after retract", endstopName)
			}
			if forcepos[2] != nil {
				triggerTimes[i%3] = triggerTime
				if i > 0 && (i+1)%3 == 0 {
					logger.Debugf("diff1:%.3f diff2:%.3f", triggerTimes[1]-triggerTimes[0], triggerTimes[2]-triggerTimes[1])
					if math.Abs((triggerTimes[1]-triggerTimes[0])-(triggerTimes[2]-triggerTimes[1])) <= 0.2 {
						break
					}
					triggerTimes = make([]float64, 3)
				}
				if i == 9 {
					logger.Error("Homing probe reached the maximum retry limit.")
				}
			}
		}
	}
	self.toolhead.FlushStepGeneration()
	for _, position := range hmove.StepperPositions() {
		self.triggerMCUPos[position.StepperName] = float64(position.TrigPos)
	}
	self.adjustPos = map[string]float64{}
	if afterHome != nil {
		if err := afterHome(); err != nil {
			return err
		}
	}
	hasAdjustments := false
	for _, adjustment := range self.adjustPos {
		if adjustment != 0 {
			hasAdjustments = true
			break
		}
	}
	if hasAdjustments {
		kin := self.toolhead.GetKinematics()
		homepos = self.toolhead.GetPosition()
		kinSPos := map[string]float64{}
		for _, stepper := range kin.GetSteppers() {
			kinSPos[stepper.GetName(false)] = stepper.GetCommandedPosition() + self.adjustPos[stepper.GetName(false)]
		}
		newpos := kin.CalcPosition(kinSPos)
		for _, axis := range homingAxes {
			homepos[axis] = newpos[axis]
		}
		self.toolhead.SetPosition(homepos, []int{})
	}
	return nil
}
