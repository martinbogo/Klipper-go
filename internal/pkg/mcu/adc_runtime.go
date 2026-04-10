package mcu

type ADCRuntimeState struct {
	LastValue   [2]float64
	InvMaxADC   float64
	ReportClock int64
}

func CalculateScaledADCValue(rawValue int64, invMaxADC float64) float64 {
	return float64(rawValue) * invMaxADC
}

func (self *ADCRuntimeState) ProcessAnalogInState(params map[string]interface{}, clock32ToClock64 func(int64) int64, clockToPrintTime func(int64) float64) [2]float64 {
	lastValue := CalculateScaledADCValue(params["value"].(int64), self.InvMaxADC)
	nextClock := clock32ToClock64(params["next_clock"].(int64))
	lastReadClock := nextClock - self.ReportClock
	lastReadTime := clockToPrintTime(lastReadClock)
	self.LastValue = [2]float64{lastValue, lastReadTime}
	return self.LastValue
}
