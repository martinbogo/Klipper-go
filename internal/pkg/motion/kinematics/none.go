package kinematics

type NoneKinematics struct {
	axesMinMax []string
}

func NewNone(config NoneConfig) *NoneKinematics {
	axesMinMax := config.AxesMinMax
	if axesMinMax == nil {
		axesMinMax = []string{"0.", "0.", "0.", "0."}
	}
	copyAxes := make([]string, len(axesMinMax))
	copy(copyAxes, axesMinMax)
	return &NoneKinematics{axesMinMax: copyAxes}
}

func (self *NoneKinematics) GetSteppers() []Stepper {
	return []Stepper{}
}

func (self *NoneKinematics) CalcPosition(stepperPositions map[string]float64) []float64 {
	_ = stepperPositions
	return []float64{0, 0, 0}
}

func (self *NoneKinematics) SetPosition(newpos []float64, homingAxes []int) {
	_, _ = newpos, homingAxes
}

func (self *NoneKinematics) NoteZNotHomed() {
}

func (self *NoneKinematics) Home(homingState HomingState) {
	_ = homingState
}

func (self *NoneKinematics) CheckMove(move Move) {
	_ = move
}

func (self *NoneKinematics) Status(eventtime float64) map[string]interface{} {
	_ = eventtime
	return map[string]interface{}{
		"homed_axes":   "",
		"axis_minimum": append([]string{}, self.axesMinMax...),
		"axis_maximum": append([]string{}, self.axesMinMax...),
	}
}

func (self *NoneKinematics) CheckEndstops(move Move) error {
	_ = move
	return nil
}

func (self *NoneKinematics) MotorOff([]interface{}) error {
	return nil
}
