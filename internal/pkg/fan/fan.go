package fan

import "math"

type PWMOutput interface {
	Setup_max_duration(maxDuration float64)
	Setup_cycle_time(cycleTime float64, hardwarePWM bool)
	Setup_start_value(startValue float64, shutdownValue float64)
	Set_pwm(printTime float64, value float64)
}

type DigitalOutput interface {
	Setup_max_duration(maxDuration float64)
	Set_digital(printTime float64, value int)
}

type RequestQueue interface {
	SendAsyncRequest(value float64, printTime interface{})
	QueueGCodeRequest(value float64)
}

type Tachometer interface {
	Get_status(eventtime float64) map[string]float64
}

type SpeedController interface {
	Get_status(eventtime float64) map[string]float64
	SetSpeed(value float64, printTime interface{})
}

type CommandSpeedController interface {
	Get_status(eventtime float64) map[string]float64
	SetSpeedFromCommand(value float64)
}

type Heater interface {
	Get_temp(eventtime float64) (float64, float64)
}

type MotorEnable interface {
	Is_motor_enabled() bool
}

type Fan struct {
	lastFanValue  float64
	lastReqValue  float64
	maxPower      float64
	kickStartTime float64
	offBelow      float64
	mcuFan        PWMOutput
	enablePin     DigitalOutput
	tachometer    Tachometer
	requestQueue  RequestQueue
}

func NewFan(maxPower float64, kickStartTime float64, offBelow float64,
	mcuFan PWMOutput, enablePin DigitalOutput, tachometer Tachometer,
	requestQueue RequestQueue) *Fan {
	self := Fan{}
	self.lastFanValue = 0.0
	self.lastReqValue = 0.0
	self.maxPower = maxPower
	self.kickStartTime = kickStartTime
	self.offBelow = offBelow
	self.mcuFan = mcuFan
	self.enablePin = enablePin
	self.tachometer = tachometer
	self.requestQueue = requestQueue
	return &self
}

func (self *Fan) ApplySpeed(printTime float64, value float64) (string, float64) {
	if value < self.offBelow {
		value = 0.0
	}
	value = math.Max(0.0, math.Min(self.maxPower, value*self.maxPower))
	if value == self.lastFanValue {
		return "discard", 0.0
	}

	if self.enablePin != nil {
		if value > 0 && self.lastFanValue == 0 {
			self.enablePin.Set_digital(printTime, 1)
		} else if value == 0 && self.lastFanValue > 0 {
			self.enablePin.Set_digital(printTime, 0)
		}
	}
	if value != 0.0 && self.kickStartTime != 0.0 &&
		(self.lastFanValue == 0.0 || value-self.lastFanValue > 0.5) {
		self.lastReqValue = value
		self.lastFanValue = self.maxPower
		self.mcuFan.Set_pwm(printTime, self.maxPower)
		return "delay", self.kickStartTime
	}
	self.lastReqValue = value
	self.lastFanValue = value
	self.mcuFan.Set_pwm(printTime, value)
	return "", 0.0
}

func (self *Fan) SetSpeed(value float64, printTime interface{}) {
	self.requestQueue.SendAsyncRequest(value, printTime)
}

func (self *Fan) SetSpeedFromCommand(value float64) {
	self.requestQueue.QueueGCodeRequest(value)
}

func (self *Fan) HandleRequestRestart(printTime float64) {
	self.SetSpeed(0.0, printTime)
}

func (self *Fan) Get_status(eventtime float64) map[string]float64 {
	tachometerStatus := map[string]float64{"rpm": 0}
	if self.tachometer != nil {
		tachometerStatus = self.tachometer.Get_status(eventtime)
	}
	return map[string]float64{
		"speed": self.lastReqValue,
		"rpm":   tachometerStatus["rpm"],
	}
}
