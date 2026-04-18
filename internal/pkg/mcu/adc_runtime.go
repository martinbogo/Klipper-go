package mcu

import "encoding/binary"

type ADCRuntimeState struct {
	LastValue   [2]float64
	InvMaxADC   float64
	ReportClock int64
}

func CalculateScaledADCValue(rawValue int64, invMaxADC float64) float64 {
	return float64(rawValue) * invMaxADC
}

func (self *ADCRuntimeState) ProcessAnalogInState(params map[string]interface{}, clock32ToClock64 func(int64) int64, clockToPrintTime func(int64) float64) [2]float64 {
	var rawValue int64
	if values, ok := params["values"]; ok {
		data := values.([]int)
		if len(data) >= 2 {
			rawValue = int64(binary.LittleEndian.Uint16([]byte{byte(data[0]), byte(data[1])}))
		}
	} else {
		rawValue = params["value"].(int64)
	}
	lastValue := CalculateScaledADCValue(rawValue, self.InvMaxADC)
	nextClock := clock32ToClock64(params["next_clock"].(int64))
	lastReadClock := nextClock - self.ReportClock
	lastReadTime := clockToPrintTime(lastReadClock)
	self.LastValue = [2]float64{lastValue, lastReadTime}
	return self.LastValue
}
