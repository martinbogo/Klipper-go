package fan

type FrequencyReader interface {
	Get_frequency() float64
}

type FanTachometer struct {
	freqCounter FrequencyReader
	ppr         int
}

func NewFanTachometer(freqCounter FrequencyReader, ppr int) *FanTachometer {
	self := FanTachometer{}
	self.freqCounter = freqCounter
	self.ppr = ppr
	return &self
}

func (self *FanTachometer) Get_status(eventtime float64) map[string]float64 {
	rpm := 0.0
	if self.freqCounter != nil && self.ppr > 0 {
		rpm = self.freqCounter.Get_frequency() * 30.0 / float64(self.ppr)
	}
	return map[string]float64{"rpm": rpm}
}
