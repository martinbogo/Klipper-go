package addon

import (
	"fmt"
	"math"
	"strings"
)

const CancelZDelta = 2.0

type TuningTransform interface {
	GetPosition() []float64
	Move([]float64, float64)
}

type TuningConfig struct {
	CommandFmt string
	Start      float64
	Factor     float64
	Band       float64
	StepHeight float64
	StepDelta  float64
	Skip       float64
}

type TuningTower struct {
	normalTransform  TuningTransform
	lastPosition     []float64
	lastZ            float64
	start            float64
	factor           float64
	band             float64
	lastCommandValue *float64
	commandFmt       string
	stepHeight       float64
	stepDelta        float64
	skip             float64
}

func NewTuningTower() *TuningTower {
	zero := 0.
	return &TuningTower{
		lastPosition:     []float64{0., 0., 0., 0.},
		lastZ:            0.,
		start:            0.,
		factor:           0.,
		band:             0.,
		lastCommandValue: &zero,
		commandFmt:       "",
		stepHeight:       0.,
		stepDelta:        0.,
		skip:             0.,
	}
}

func (self *TuningTower) BeginTest(transform TuningTransform, config TuningConfig) (string, error) {
	if config.Factor != 0.0 && (config.StepHeight != 0.0 || config.StepDelta != 0.0) {
		return "", fmt.Errorf("must specify either FACTOR or both STEP_DELTA and STEP_HEIGHT")
	}
	if (config.StepDelta != 0.) != (config.StepHeight != 0.) {
		return "", fmt.Errorf("must specify both STEP_DELTA and STEP_HEIGHT")
	}

	self.normalTransform = transform
	self.start = config.Start
	self.factor = config.Factor
	self.band = config.Band
	self.stepDelta = config.StepDelta
	self.stepHeight = config.StepHeight
	self.skip = config.Skip
	self.commandFmt = config.CommandFmt
	self.lastZ = -99999999.9
	self.lastCommandValue = nil
	self.GetPosition()

	messageParts := []string{fmt.Sprintf("start=%.6f", self.start)}
	if self.factor != 0.0 {
		messageParts = append(messageParts, fmt.Sprintf("factor=%.6f", self.factor))
		if self.band != 0.0 {
			messageParts = append(messageParts, fmt.Sprintf("band=%.6f", self.band))
		}
	} else {
		messageParts = append(messageParts, fmt.Sprintf("step_delta=%.6f", self.stepDelta))
		messageParts = append(messageParts, fmt.Sprintf("step_height=%.6f", self.stepHeight))
	}
	if self.skip != 0.0 {
		messageParts = append(messageParts, fmt.Sprintf("skip=%.6f", self.skip))
	}
	return "Starting tuning test (" + strings.Join(messageParts, " ") + ")", nil
}

func (self *TuningTower) GetPosition() []float64 {
	if self.normalTransform == nil {
		return nil
	}
	pos := self.normalTransform.GetPosition()
	copy(self.lastPosition, pos)
	out := make([]float64, len(pos))
	copy(out, pos)
	return out
}

func (self *TuningTower) CalcValue(z float64) float64 {
	if self.skip != 0.0 {
		z = math.Max(0., z-self.skip)
	}
	if self.stepHeight != 0.0 {
		return self.start + self.stepDelta*math.Floor(z/self.stepHeight)
	}
	if self.band != 0.0 {
		z = (math.Floor(z/self.band) + .5) * self.band
	}
	return self.start + z*self.factor
}

func (self *TuningTower) Move(newpos []float64, speed float64, gcodeZ float64) (string, bool) {
	if self.normalTransform == nil {
		return "", false
	}

	isEq := true
	for i := 0; i < 3; i++ {
		if newpos[i] != self.lastPosition[i] {
			isEq = false
			break
		}
	}

	command := ""
	endTest := false
	if newpos[3] > self.lastPosition[3] && newpos[2] != self.lastZ && isEq {
		z := newpos[2]
		if z < self.lastZ-CancelZDelta {
			endTest = true
		} else {
			newval := self.CalcValue(gcodeZ)
			self.lastZ = z
			if self.lastCommandValue == nil || newval != *self.lastCommandValue {
				self.lastCommandValue = &newval
				command = fmt.Sprintf(self.commandFmt, newval)
			}
		}
	}

	copy(self.lastPosition, newpos)
	self.normalTransform.Move(newpos, speed)
	return command, endTest
}

func (self *TuningTower) EndTest() {
	self.normalTransform = nil
	self.lastCommandValue = nil
	self.commandFmt = ""
	self.lastZ = 0.
	copy(self.lastPosition, []float64{0., 0., 0., 0.})
}

func (self *TuningTower) IsActive() bool {
	return self.normalTransform != nil
}