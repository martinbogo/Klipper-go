package io

const OUTPIN_MIN_TIME = 0.100

type Reactor interface {
	Monotonic() float64
}

type MCUEstimator interface {
	Estimated_print_time(eventtime float64) float64
}

type DigitalPin interface {
	Get_mcu() MCUEstimator
	Set_digital(printTime float64, value int)
}

type DigitalOutput struct {
	pin           DigitalPin
	lastValue     int
	shutdownValue int
	reactor       Reactor
}

func NewDigitalOutput(pin DigitalPin, reactor Reactor, lastValue int, shutdownValue int) *DigitalOutput {
	self := &DigitalOutput{}
	self.pin = pin
	self.lastValue = lastValue
	self.shutdownValue = shutdownValue
	self.reactor = reactor
	return self
}

func (self *DigitalOutput) Get_status(eventTime float64) map[string]int {
	return map[string]int{"value": self.lastValue}
}

func (self *DigitalOutput) SetPowerPin(value int) {
	self.SetPin(value)
	self.lastValue = value
}

func (self *DigitalOutput) SetPin(value int) {
	curtime := self.reactor.Monotonic()
	estTime := self.pin.Get_mcu().Estimated_print_time(curtime)
	nextCmdTime := estTime + OUTPIN_MIN_TIME
	self.pin.Set_digital(nextCmdTime, value)
}

func (self *DigitalOutput) HandleReady() error {
	self.SetPin(self.lastValue)
	return nil
}

func (self *DigitalOutput) HandleShutdown() error {
	self.SetPin(self.shutdownValue)
	return nil
}