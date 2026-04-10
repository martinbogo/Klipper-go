package probe

type PrinterProbe struct {
	Speed             float64
	LiftSpeed         float64
	XOffset           float64
	YOffset           float64
	ZOffset           float64
	FinalSpeed        float64
	ProbeCalibrateZ   float64
	MultiProbePending bool
	LastState         bool
	LastZResult       float64
	SampleCount       int
	SampleRetractDist float64
	SamplesResult     interface{}
	SamplesTolerance  float64
	SamplesRetries    int
	ZPosition         float64
}

func NewPrinterProbe(speed, liftSpeed, xOffset, yOffset, zOffset, finalSpeed, zPosition float64,
	sampleCount int, sampleRetractDist float64, samplesResult interface{}, samplesTolerance float64,
	samplesRetries int) *PrinterProbe {
	return &PrinterProbe{
		Speed:             speed,
		LiftSpeed:         liftSpeed,
		XOffset:           xOffset,
		YOffset:           yOffset,
		ZOffset:           zOffset,
		FinalSpeed:        finalSpeed,
		ProbeCalibrateZ:   0,
		MultiProbePending: false,
		LastState:         false,
		LastZResult:       0,
		SampleCount:       sampleCount,
		SampleRetractDist: sampleRetractDist,
		SamplesResult:     samplesResult,
		SamplesTolerance:  samplesTolerance,
		SamplesRetries:    samplesRetries,
		ZPosition:         zPosition,
	}
}

func (self *PrinterProbe) GetOffsets() (float64, float64, float64) {
	return self.XOffset, self.YOffset, self.ZOffset
}

func (self *PrinterProbe) BeginMultiProbe() {
	self.MultiProbePending = true
}

func (self *PrinterProbe) EndMultiProbe() bool {
	wasPending := self.MultiProbePending
	self.MultiProbePending = false
	return wasPending
}

func (self *PrinterProbe) SetProbeCalibrateZ(z float64) {
	self.ProbeCalibrateZ = z
}

func (self *PrinterProbe) CalibratedOffset(kinPos []float64) float64 {
	if len(kinPos) == 0 {
		return 0
	}
	return self.ProbeCalibrateZ - kinPos[2]
}

func (self *PrinterProbe) RecordLastZResult(z float64) {
	self.LastZResult = z
}

func (self *PrinterProbe) RecordLastState(triggered bool) {
	self.LastState = triggered
}

func (self *PrinterProbe) Status() map[string]interface{} {
	return map[string]interface{}{
		"last_query":    self.LastState,
		"last_z_result": self.LastZResult,
	}
}
