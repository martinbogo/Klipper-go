package io

import "fmt"

type PinApplier interface {
	SetPWM(printTime float64, value float64)
	SetDigital(printTime float64, value int)
}

type OutputPinController struct {
	isPWM    bool
	scale    float64
	lastValue float64
}

func NewOutputPinController(isPWM bool, scale float64, lastValue float64) *OutputPinController {
	self := &OutputPinController{}
	self.isPWM = isPWM
	self.scale = scale
	self.lastValue = lastValue
	return self
}

func (self *OutputPinController) Get_status(eventTime float64) map[string]float64 {
	return map[string]float64{"value": self.lastValue}
}

func (self *OutputPinController) SetPin(printTime float64, value float64, applier PinApplier) (string, float64) {
	if value == self.lastValue {
		return "discard", 0.0
	}

	self.lastValue = value
	if self.isPWM {
		applier.SetPWM(printTime, value)
	} else {
		applier.SetDigital(printTime, int(value))
	}
	return "", 0.0
}

func (self *OutputPinController) NormalizeCommandValue(value float64) (float64, error) {
	value /= self.scale
	if !self.isPWM && value != 0.0 && value != 1.0 {
		return 0.0, fmt.Errorf("Invalid pin value")
	}
	return value, nil
}