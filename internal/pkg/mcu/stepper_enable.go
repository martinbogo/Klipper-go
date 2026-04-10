package mcu

type DigitalOut interface {
	SetDigital(printTime float64, value int)
}

type ActiveStepper interface {
	AddActiveCallback(func(float64))
}

type StepperEnablePin struct {
	digitalOut   DigitalOut
	enableCount  int
	isDedicated  bool
}

func NewStepperEnablePin(digitalOut DigitalOut, enableCount int) *StepperEnablePin {
	self := &StepperEnablePin{}
	self.digitalOut = digitalOut
	self.enableCount = enableCount
	self.isDedicated = true
	return self
}

func (self *StepperEnablePin) SetEnable(printTime float64) {
	if self.enableCount == 0 && self.digitalOut != nil {
		self.digitalOut.SetDigital(printTime, 1)
	}
	self.enableCount++
}

func (self *StepperEnablePin) Set_enable(printTime float64) {
	self.SetEnable(printTime)
}

func (self *StepperEnablePin) SetDisable(printTime float64) {
	self.enableCount--
	if self.enableCount == 0 && self.digitalOut != nil {
		self.digitalOut.SetDigital(printTime, 0)
	}
}

func (self *StepperEnablePin) Set_disable(printTime float64) {
	self.SetDisable(printTime)
}

func (self *StepperEnablePin) SetDedicated(isDedicated bool) {
	self.isDedicated = isDedicated
}

func (self *StepperEnablePin) IsDedicated() bool {
	return self.isDedicated
}

type EnableTracking struct {
	stepper    ActiveStepper
	enable     *StepperEnablePin
	callbacks  []func(float64, bool)
	isEnabled  bool
}

func NewEnableTracking(stepper ActiveStepper, enable *StepperEnablePin) *EnableTracking {
	self := &EnableTracking{}
	self.stepper = stepper
	self.enable = enable
	self.callbacks = []func(float64, bool){}
	self.isEnabled = false
	self.stepper.AddActiveCallback(self.MotorEnable)
	return self
}

func (self *EnableTracking) RegisterStateCallback(callback func(float64, bool)) {
	self.callbacks = append(self.callbacks, callback)
}

func (self *EnableTracking) Register_state_callback(callback func(float64, bool)) {
	self.RegisterStateCallback(callback)
}

func (self *EnableTracking) MotorEnable(printTime float64) {
	if !self.isEnabled {
		for _, cb := range self.callbacks {
			cb(printTime, true)
		}
		self.enable.SetEnable(printTime)
		self.isEnabled = true
	}
}

func (self *EnableTracking) Motor_enable(printTime float64) {
	self.MotorEnable(printTime)
}

func (self *EnableTracking) MotorDisable(printTime float64) {
	if self.isEnabled {
		for _, cb := range self.callbacks {
			cb(printTime, false)
		}
		self.enable.SetDisable(printTime)
		self.isEnabled = false
		self.stepper.AddActiveCallback(self.MotorEnable)
	}
}

func (self *EnableTracking) Motor_disable(printTime float64) {
	self.MotorDisable(printTime)
}

func (self *EnableTracking) IsMotorEnabled() bool {
	return self.isEnabled
}

func (self *EnableTracking) Is_motor_enabled() bool {
	return self.IsMotorEnabled()
}

func (self *EnableTracking) HasDedicatedEnable() bool {
	return self.enable.IsDedicated()
}

func (self *EnableTracking) Has_dedicated_enable() bool {
	return self.HasDedicatedEnable()
}