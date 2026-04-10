package mcu

import "math"

type ADCConfigPlan struct {
	QueryClock      int64
	SampleTicks     int64
	SampleCount     int
	ReportClock     int64
	MinSample       int
	MaxSample       int
	RangeCheckCount int
}

func (self *ADCRuntimeState) BuildConfigPlan(oid int, sampleTime float64, reportTime float64, minSample float64, maxSample float64, sampleCount int, rangeCheckCount int, getQuerySlot func(int) int64, secondsToClock func(float64) int64, mcuADCMax float64) ADCConfigPlan {
	maxADC := float64(sampleCount) * mcuADCMax
	self.InvMaxADC = 1.0 / maxADC
	self.ReportClock = secondsToClock(reportTime)
	return ADCConfigPlan{
		QueryClock:      getQuerySlot(oid),
		SampleTicks:     secondsToClock(sampleTime),
		SampleCount:     sampleCount,
		ReportClock:     self.ReportClock,
		MinSample:       int(math.Max(0, math.Min(65535, float64(int(minSample*maxADC))))),
		MaxSample:       int(math.Max(0, math.Min(65535, float64(int(math.Ceil(maxSample*maxADC)))))),
		RangeCheckCount: rangeCheckCount,
	}
}
