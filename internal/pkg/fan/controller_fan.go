package fan

import (
	"fmt"
	"strings"
)

type ControllerFan struct {
	stepperNames []string
	heaters      []Heater
	fan          SpeedController
	fanSpeed     float64
	idleSpeed    float64
	idleTimeout  int
	lastOn       int
	lastSpeed    float64
}

func NewControllerFan(stepperNames []string, fan SpeedController, fanSpeed float64,
	idleSpeed float64, idleTimeout int) *ControllerFan {
	self := &ControllerFan{}
	self.stepperNames = append([]string{}, stepperNames...)
	self.heaters = make([]Heater, 0)
	self.fan = fan
	self.fanSpeed = fanSpeed
	self.idleSpeed = idleSpeed
	self.idleTimeout = idleTimeout
	self.lastOn = idleTimeout
	self.lastSpeed = 0.0
	return self
}

func (self *ControllerFan) SetHeaters(heaters []Heater) {
	self.heaters = append([]Heater{}, heaters...)
}

func (self *ControllerFan) ResolveSteppers(allSteppers []string) error {
	if len(self.stepperNames) == 0 {
		self.stepperNames = append([]string{}, allSteppers...)
		return nil
	}
	for _, v := range self.stepperNames {
		if !containsStepper(allSteppers, v) {
			return fmt.Errorf("One or more of these steppers are unknown: %s (valid steppers are: %s)",
				self.stepperNames, strings.Join(allSteppers, ", "))
		}
	}
	return nil
}

func (self *ControllerFan) Get_status(eventtime float64) map[string]float64 {
	return self.fan.Get_status(eventtime)
}

func (self *ControllerFan) Callback(eventtime float64,
	lookupEnable func(string) (MotorEnable, error),
	logError func(string, error)) float64 {
	speed := 0.0
	active := false

	for _, name := range self.stepperNames {
		et, err := lookupEnable(name)
		if err == nil {
			if et.Is_motor_enabled() {
				active = true
			}
		} else if logError != nil {
			logError(name, err)
		}
	}

	for _, heater := range self.heaters {
		_, targetTemp := heater.Get_temp(eventtime)
		if targetTemp > 0 {
			active = true
		}
	}

	if active {
		self.lastOn = 0
		speed = self.fanSpeed
	} else if self.lastOn < self.idleTimeout {
		speed = self.idleSpeed
		self.lastOn += 1
	}

	if speed != self.lastSpeed {
		self.lastSpeed = speed
		self.fan.SetSpeed(speed, nil)
	}

	return eventtime + 1.0
}

func containsStepper(steppers []string, name string) bool {
	for _, v := range steppers {
		if v == name {
			return true
		}
	}
	return false
}