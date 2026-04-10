package heater

import "math"

const (
	ScaledSampleTime       = 0.001
	ScaledSampleCount      = 8
	ScaledReportTime       = 0.300
	ScaledRangeCheckCount  = 4
)

type ADCPin interface {
	SetCallback(reportTime float64, callback func(float64, float64))
	SetMinMax(sampleTime float64, sampleCount int, minval float64, maxval float64, rangeCheckCount int)
	MCURef() interface{}
	Raw() interface{}
}

type ScaledADCReader struct {
	main       *ScaledADCChip
	lastState  [2]float64
	adcPin     ADCPin
	callback   func(float64, float64)
}

func NewScaledADCReader(main *ScaledADCChip, adcPin ADCPin) *ScaledADCReader {
	self := &ScaledADCReader{}
	self.main = main
	self.lastState = [2]float64{0, 0}
	self.adcPin = adcPin
	self.callback = nil
	return self
}

func (self *ScaledADCReader) HandleCallback(readTime float64, readValue float64) {
	maxADC := self.main.lastVref[1]
	minADC := self.main.lastVssa[1]
	scaledVal := (readValue - minADC) / (maxADC - minADC)
	self.lastState = [2]float64{scaledVal, readTime}
	if self.callback != nil {
		self.callback(readTime, scaledVal)
	}
}

func (self *ScaledADCReader) SetCallback(reportTime float64, callback func(float64, float64)) {
	self.callback = callback
	self.adcPin.SetCallback(reportTime, self.HandleCallback)
}

func (self *ScaledADCReader) GetLastValue() [2]float64 {
	return self.lastState
}

func (self *ScaledADCReader) SetMinMax(sampleTime float64, sampleCount int, minval float64, maxval float64, rangeCheckCount int) {
	self.adcPin.SetMinMax(sampleTime, sampleCount, minval, maxval, rangeCheckCount)
}

func (self *ScaledADCReader) MCURef() interface{} {
	return self.adcPin.MCURef()
}

func (self *ScaledADCReader) Raw() interface{} {
	return self.adcPin.Raw()
}

type ScaledADCChip struct {
	name            string
	lastVref        [2]float64
	lastVssa        [2]float64
	invSmoothTime   float64
	mcuRef          interface{}
	setupMCUADC     func(pinParams map[string]interface{}) ADCPin
	registerADC     func(name string, adc interface{})
}

func NewScaledADCChip(name string, smoothTime float64,
	setupNamedADC func(pinName string, callback func(float64, float64)) ADCPin,
	setupMCUADC func(pinParams map[string]interface{}) ADCPin,
	registerADC func(name string, adc interface{})) (*ScaledADCChip, error) {
	self := &ScaledADCChip{}
	self.name = name
	self.lastVref = [2]float64{0.0, 0.0}
	self.lastVssa = [2]float64{0.0, 0.0}
	self.invSmoothTime = 1.0 / smoothTime
	self.setupMCUADC = setupMCUADC
	self.registerADC = registerADC

	vrefPin := setupNamedADC("vref", self.VrefCallback)
	vssaPin := setupNamedADC("vssa", self.VssaCallback)
	self.mcuRef = vrefPin.MCURef()
	if self.mcuRef != vssaPin.MCURef() {
		return nil, ErrDifferentMCUs{}
	}
	return self, nil
}

type ErrDifferentMCUs struct{}

func (ErrDifferentMCUs) Error() string {
	return "vref and vssa must be on same mcu"
}

func (self *ScaledADCChip) MCURef() interface{} {
	return self.mcuRef
}

func (self *ScaledADCChip) SetupReader(pinParams map[string]interface{}) *ScaledADCReader {
	adcPin := self.setupMCUADC(pinParams)
	reader := NewScaledADCReader(self, adcPin)
	if self.registerADC != nil {
		self.registerADC(self.name+":"+pinParams["pin"].(string), adcPin.Raw())
	}
	return reader
}

func (self *ScaledADCChip) CalcSmooth(readTime float64, readValue float64, last [2]float64) [2]float64 {
	lastTime, lastValue := last[0], last[1]
	timeDiff := readTime - lastTime
	valueDiff := readValue - lastValue
	adjTime := math.Min(timeDiff*self.invSmoothTime, 1.0)
	smoothedValue := lastValue + valueDiff*adjTime
	return [2]float64{readTime, smoothedValue}
}

func (self *ScaledADCChip) VrefCallback(readTime float64, readValue float64) {
	self.lastVref = self.CalcSmooth(readTime, readValue, self.lastVref)
}

func (self *ScaledADCChip) VssaCallback(readTime float64, readValue float64) {
	self.lastVssa = self.CalcSmooth(readTime, readValue, self.lastVssa)
}