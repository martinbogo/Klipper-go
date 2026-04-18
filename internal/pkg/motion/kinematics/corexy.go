package kinematics

import (
	"goklipper/common/utils/collections"
	"goklipper/common/utils/maths"
	"math"
	"strings"
)

type CoreXYKinematics struct {
	printer      Printer
	toolhead     Toolhead
	rails        []Rail
	maxZVelocity float64
	maxZAccel    float64
	limits       [][]float64
	axesMin      []float64
	axesMax      []float64
}

func NewCoreXY(config CoreXYConfig) *CoreXYKinematics {
	self := &CoreXYKinematics{
		printer:      config.Printer,
		toolhead:     config.Toolhead,
		rails:        append([]Rail{}, config.Rails...),
		maxZVelocity: config.MaxZVelocity,
		maxZAccel:    config.MaxZAccel,
		limits:       [][]float64{{1.0, -1.0}, {1.0, -1.0}, {1.0, -1.0}},
		axesMin:      []float64{0., 0., 0., 0.},
		axesMax:      []float64{0., 0., 0., 0.},
	}

	if len(self.rails) >= 2 {
		if endstop := self.rails[0].Primary_endstop(); endstop != nil {
			for _, stepper := range self.rails[1].Get_steppers() {
				endstop.Add_stepper(stepper)
			}
		}
		if endstop := self.rails[1].Primary_endstop(); endstop != nil {
			for _, stepper := range self.rails[0].Get_steppers() {
				endstop.Add_stepper(stepper)
			}
		}
	}

	axisNames := []string{"+", "-", "z"}
	for i, rail := range self.rails {
		if i < 2 {
			rail.Setup_itersolve("corexy_stepper_alloc", []byte(axisNames[i])[0])
		} else {
			rail.Setup_itersolve("cartesian_stepper_alloc", []byte(axisNames[i])[0])
		}
	}

	for _, stepper := range self.GetSteppers() {
		stepper.Set_trapq(self.toolhead.Get_trapq())
		self.toolhead.Register_step_generator(stepper.Generate_steps)
	}
	if self.printer != nil {
		self.printer.Register_event_handler("stepper_enable:motor_off", self.MotorOff)
	}

	for i, rail := range self.rails {
		positionMin, positionMax := rail.Get_range()
		self.axesMin[i] = positionMin
		self.axesMax[i] = positionMax
	}
	return self
}

func (self *CoreXYKinematics) GetSteppers() []Stepper {
	steppers := []Stepper{}
	for _, rail := range self.rails {
		steppers = append(steppers, rail.Get_steppers()...)
	}
	return steppers
}

func (self *CoreXYKinematics) Rails() []Rail {
	return append([]Rail{}, self.rails...)
}

func (self *CoreXYKinematics) CalcPosition(stepperPositions map[string]float64) []float64 {
	pos := make([]float64, 0, len(self.rails))
	for _, rail := range self.rails {
		pos = append(pos, stepperPositions[rail.Get_name(false)])
	}
	return []float64{0.5 * (pos[0] + pos[1]), 0.5 * (pos[0] - pos[1]), pos[2]}
}

func (self *CoreXYKinematics) SetPosition(newpos []float64, homingAxes []int) {
	for i, rail := range self.rails {
		rail.Set_position(newpos)
		if collections.InInt(i, homingAxes) {
			self.limits[i][0], self.limits[i][1] = rail.Get_range()
		}
	}
}

func (self *CoreXYKinematics) NoteZNotHomed() {
	self.limits[2] = []float64{1.0, -1.0}
}

func (self *CoreXYKinematics) Home(homingState HomingState) {
	axes := homingState.GetAxes()
	if len(axes) > 1 {
		xIndex := -1
		yIndex := -1
		for i, axis := range axes {
			switch axis {
			case 0:
				if xIndex == -1 {
					xIndex = i
				}
			case 1:
				if yIndex == -1 {
					yIndex = i
				}
			}
		}
		if xIndex != -1 && yIndex != -1 && xIndex < yIndex {
			axes[xIndex], axes[yIndex] = axes[yIndex], axes[xIndex]
		}
	}
	for _, axis := range axes {
		rail := self.rails[axis]
		positionMin, positionMax := rail.Get_range()
		hi := rail.Get_homing_info()
		homepos := []interface{}{nil, nil, nil, nil}
		homepos[axis] = hi.PositionEndstop
		forcepos := make([]interface{}, len(homepos))
		copy(forcepos, homepos)
		if hi.PositiveDir {
			forcepos[axis] = forcepos[axis].(float64) - 1.5*(hi.PositionEndstop-positionMin)
		} else {
			forcepos[axis] = forcepos[axis].(float64) + 1.5*(positionMax-hi.PositionEndstop)
		}
		homingState.HomeRails([]Rail{rail}, forcepos, homepos)
	}
}

func (self *CoreXYKinematics) MotorOff([]interface{}) error {
	self.limits = [][]float64{{1.0, -1.0}, {1.0, -1.0}, {1.0, -1.0}}
	return nil
}

func (self *CoreXYKinematics) CheckEndstops(move Move) error {
	endPos := move.EndPos()
	axesD := move.AxesD()
	for i := 0; i < 3; i++ {
		if axesD[i] != 0.0 && (maths.Check_below_limit(endPos[i], self.limits[i][0]) || maths.Check_above_limit(endPos[i], self.limits[i][1])) {
			if self.limits[i][0] > self.limits[i][1] {
				return move.MoveError("Must home axis first")
			}
			return move.MoveError("Move out of range")
		}
	}
	return nil
}

func (self *CoreXYKinematics) CheckMove(move Move) {
	endPos := move.EndPos()
	if maths.Check_below_limit(endPos[0], self.limits[0][0]) ||
		maths.Check_above_limit(endPos[0], self.limits[0][1]) ||
		maths.Check_below_limit(endPos[1], self.limits[1][0]) ||
		maths.Check_above_limit(endPos[1], self.limits[1][1]) {
		if err := self.CheckEndstops(move); err != nil {
			panic(err)
		}
	}
	if move.AxesD()[2] == 0 {
		return
	}
	if err := self.CheckEndstops(move); err != nil {
		panic(err)
	}
	zRatio := move.MoveD() / math.Abs(move.AxesD()[2])
	move.LimitSpeed(self.maxZVelocity*zRatio, self.maxZAccel*zRatio)
}

func (self *CoreXYKinematics) Status(eventtime float64) map[string]interface{} {
	_ = eventtime
	axes := []string{}
	for i, axis := range []string{"x", "y", "z"} {
		if self.limits[i][0] <= self.limits[i][1] {
			axes = append(axes, axis)
		}
	}
	return map[string]interface{}{
		"homed_axes":   strings.Join(axes, ""),
		"axis_minimum": append([]float64{}, self.axesMin...),
		"axis_maximum": append([]float64{}, self.axesMax...),
	}
}

func (self *CoreXYKinematics) GetAxisRange(axis int) (float64, float64) {
	return self.rails[axis].Get_range()
}
