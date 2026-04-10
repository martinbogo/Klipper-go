package mcu

import "math"

type OutputCommandSender interface {
	Send(data interface{}, minclock, reqclock int64)
}

type DigitalOutRuntimeState struct {
	Invert    int
	LastClock int64
}

func normalizeBinaryValue(value float64) int {
	if value != 0 {
		return 1
	}
	return 0
}

func (self *DigitalOutRuntimeState) SetupStartValue(startValue float64, shutdownValue float64) (int, int) {
	startValueInt := normalizeBinaryValue(startValue)
	shutdownValueInt := normalizeBinaryValue(shutdownValue)
	return startValueInt ^ self.Invert, shutdownValueInt ^ self.Invert
}

func (self *DigitalOutRuntimeState) SetDigital(printTime float64, value int, printTimeToClock func(float64) int64, sender OutputCommandSender, oid int) {
	clock := printTimeToClock(printTime)
	var encodedValue int64
	if value != 0 {
		encodedValue = 1
	}
	sender.Send([]int64{int64(oid), clock, encodedValue ^ int64(self.Invert)}, self.LastClock, clock)
	self.LastClock = clock
}

type PWMRuntimeState struct {
	Invert    int
	PWMMax    float64
	LastClock int64
}

func clampUnitValue(value float64) float64 {
	return math.Max(0.0, math.Min(1.0, value))
}

func (self *PWMRuntimeState) SetupStartValue(startValue float64, shutdownValue float64) (float64, float64) {
	if self.Invert > 0 {
		startValue = 1.0 - startValue
		shutdownValue = 1.0 - shutdownValue
	}
	return clampUnitValue(startValue), clampUnitValue(shutdownValue)
}

func (self *PWMRuntimeState) SetPWM(printTime float64, value float64, printTimeToClock func(float64) int64, sender OutputCommandSender, oid int) {
	if self.Invert != 0 {
		value = 1.0 - value
	}
	encodedValue := int64(clampUnitValue(value)*self.PWMMax + 0.5)
	clock := printTimeToClock(printTime)
	sender.Send([]int64{int64(oid), clock, encodedValue}, self.LastClock, clock)
	self.LastClock = clock
}
