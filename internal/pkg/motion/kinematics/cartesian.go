package kinematics

import (
	"goklipper/common/utils/collections"
	"goklipper/common/utils/maths"
	"math"
	"strings"
)

type CartesianKinematics struct {
	printer           Printer
	toolhead          Toolhead
	dualCarriageAxis  *int
	dualCarriageRails []Rail
	rails             []Rail
	maxZVelocity      float64
	maxZAccel         float64
	limits            [][]float64
	axesMin           []float64
	axesMax           []float64
}

func NewCartesian(config CartesianConfig) *CartesianKinematics {
	self := &CartesianKinematics{
		printer:      config.Printer,
		toolhead:     config.Toolhead,
		rails:        append([]Rail{}, config.Rails...),
		maxZVelocity: config.MaxZVelocity,
		maxZAccel:    config.MaxZAccel,
		limits:       [][]float64{{1.0, -1.0}, {1.0, -1.0}, {1.0, -1.0}},
		axesMin:      []float64{0., 0., 0., 0.},
		axesMax:      []float64{0., 0., 0., 0.},
	}

	axisNames := []string{"x", "y", "z"}
	for i, rail := range self.rails {
		rail.Setup_itersolve("cartesian_stepper_alloc", []byte(axisNames[i])[0])
	}

	if config.DualCarriage != nil {
		axis := config.DualCarriage.Axis
		self.dualCarriageAxis = &axis
		self.dualCarriageRails = append([]Rail{}, config.DualCarriage.Rails...)
		if len(self.dualCarriageRails) > 1 {
			self.dualCarriageRails[1].Setup_itersolve("cartesian_stepper_alloc", []byte(config.DualCarriage.AxisName)[0])
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

func (self *CartesianKinematics) GetSteppers() []Stepper {
	rails := []Rail{}
	if self.dualCarriageAxis != nil && len(self.dualCarriageRails) == 2 {
		axis := *self.dualCarriageAxis
		rails = append(rails, self.rails[:axis]...)
		rails = append(rails, self.dualCarriageRails...)
		rails = append(rails, self.rails[axis+1:]...)
	} else {
		rails = append(rails, self.rails...)
	}

	steppers := []Stepper{}
	for _, rail := range rails {
		steppers = append(steppers, rail.Get_steppers()...)
	}
	return steppers
}

func (self *CartesianKinematics) CalcPosition(stepperPositions map[string]float64) []float64 {
	position := make([]float64, 0, len(self.rails))
	for _, rail := range self.rails {
		position = append(position, stepperPositions[rail.Get_name(false)])
	}
	return position
}

func (self *CartesianKinematics) SetPosition(newpos []float64, homingAxes []int) {
	for i, rail := range self.rails {
		rail.Set_position(newpos)
		if collections.InInt(i, homingAxes) {
			self.limits[i][0], self.limits[i][1] = rail.Get_range()
		}
	}
}

func (self *CartesianKinematics) NoteZNotHomed() {
	self.limits[2] = []float64{1.0, -1.0}
}

func (self *CartesianKinematics) HomeAxis(homingState HomingState, axis int, rail Rail) {
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

func (self *CartesianKinematics) Home(homingState HomingState) {
	for _, axis := range homingState.GetAxes() {
		if self.dualCarriageAxis != nil && axis == *self.dualCarriageAxis && len(self.dualCarriageRails) == 2 {
			dc1 := self.dualCarriageRails[0]
			dc2 := self.dualCarriageRails[1]
			activeCarriage := 0
			if self.rails[axis] == dc2 {
				activeCarriage = 1
			}
			self.ActivateCarriage(0)
			self.HomeAxis(homingState, axis, dc1)
			self.ActivateCarriage(1)
			self.HomeAxis(homingState, axis, dc2)
			self.ActivateCarriage(activeCarriage)
			continue
		}
		self.HomeAxis(homingState, axis, self.rails[axis])
	}
}

func (self *CartesianKinematics) MotorOff([]interface{}) error {
	self.limits = [][]float64{{1.0, -1.0}, {1.0, -1.0}, {1.0, -1.0}}
	return nil
}

func (self *CartesianKinematics) CheckEndstops(move Move) error {
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

func (self *CartesianKinematics) CheckMove(move Move) {
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

func (self *CartesianKinematics) Status(eventtime float64) map[string]interface{} {
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

func (self *CartesianKinematics) ActivateCarriage(carriage int) {
	if self.dualCarriageAxis == nil || len(self.dualCarriageRails) != 2 {
		return
	}
	axis := *self.dualCarriageAxis
	self.toolhead.Flush_step_generation()
	dcRail := self.dualCarriageRails[carriage]
	self.rails[axis].Set_trapq(nil)
	dcRail.Set_trapq(self.toolhead.Get_trapq())
	self.rails[axis] = dcRail
	pos := self.toolhead.Get_position()
	pos[axis] = dcRail.Get_commanded_position()
	self.toolhead.Set_position(pos, []int{})
	if self.limits[axis][0] <= self.limits[axis][1] {
		self.limits[axis][0], self.limits[axis][1] = dcRail.Get_range()
	}
}
