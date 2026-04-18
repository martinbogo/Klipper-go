package motion

type LegacyRailStepper interface {
	Setup_itersolve(alloc_func string, params interface{})
	Generate_steps(flush_time float64)
	Set_trapq(tq interface{}) interface{}
	Set_position(coord []float64)
	Get_name(short bool) string
	Get_commanded_position() float64
}

type LegacyRailRuntime struct {
	steppers []LegacyRailStepper
}

func NewLegacyRailRuntime() *LegacyRailRuntime {
	return &LegacyRailRuntime{steppers: []LegacyRailStepper{}}
}

func (self *LegacyRailRuntime) AddStepper(stepper LegacyRailStepper) {
	self.steppers = append(self.steppers, stepper)
}

func (self *LegacyRailRuntime) Steppers() []LegacyRailStepper {
	steppers := make([]LegacyRailStepper, len(self.steppers))
	copy(steppers, self.steppers)
	return steppers
}

func (self *LegacyRailRuntime) SetupItersolve(allocFunc string, params ...interface{}) {
	for _, stepper := range self.steppers {
		stepper.Setup_itersolve(allocFunc, params)
	}
}

func (self *LegacyRailRuntime) Setup_itersolve(allocFunc string, params ...interface{}) {
	self.SetupItersolve(allocFunc, params...)
}

func (self *LegacyRailRuntime) GenerateSteps(flushTime float64) {
	for _, stepper := range self.steppers {
		stepper.Generate_steps(flushTime)
	}
}

func (self *LegacyRailRuntime) Generate_steps(flushTime float64) {
	self.GenerateSteps(flushTime)
}

func (self *LegacyRailRuntime) SetTrapq(trapq interface{}) {
	for _, stepper := range self.steppers {
		stepper.Set_trapq(trapq)
	}
}

func (self *LegacyRailRuntime) Set_trapq(trapq interface{}) {
	self.SetTrapq(trapq)
}

func (self *LegacyRailRuntime) SetPosition(coord []float64) {
	for _, stepper := range self.steppers {
		stepper.Set_position(coord)
	}
}

func (self *LegacyRailRuntime) Set_position(coord []float64) {
	self.SetPosition(coord)
}

func (self *LegacyRailRuntime) GetName(short bool) string {
	if len(self.steppers) == 0 {
		return ""
	}
	return self.steppers[0].Get_name(short)
}

func (self *LegacyRailRuntime) Get_name(short bool) string {
	return self.GetName(short)
}

func (self *LegacyRailRuntime) GetCommandedPosition() float64 {
	if len(self.steppers) == 0 {
		return 0.
	}
	return self.steppers[0].Get_commanded_position()
}

func (self *LegacyRailRuntime) Get_commanded_position() float64 {
	return self.GetCommandedPosition()
}
