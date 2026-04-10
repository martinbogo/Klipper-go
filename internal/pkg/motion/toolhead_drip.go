package motion

import (
	"errors"
	"math"
)

var ErrDripModeEnd = errors.New("drip mode ended")

type DripCompletion interface {
	Test() bool
	Wait(waketime float64, waketimeResult interface{}) interface{}
}

type DripTimeSource interface {
	Monotonic() float64
	EstimatedPrintTime(eventtime float64) float64
}

type DripRuntime interface {
	PrintTime() float64
	KinFlushDelay() float64
	CanPause() bool
	NoteMovequeueActivity(mqTime float64, setStepGenTime bool)
	AdvanceMoveTime(nextPrintTime float64)
}

type ToolheadDripConfig struct {
	DripTime             float64
	StepcompressFlushTime float64
	DripSegmentTime      float64
}

func UpdateToolheadDripTime(runtime DripRuntime, source DripTimeSource, completion DripCompletion, nextPrintTime float64, config ToolheadDripConfig) error {
	flushDelay := config.DripTime + config.StepcompressFlushTime + runtime.KinFlushDelay()
	for runtime.PrintTime() < nextPrintTime {
		if completion.Test() {
			return ErrDripModeEnd
		}
		curTime := source.Monotonic()
		estPrintTime := source.EstimatedPrintTime(curTime)
		waitTime := runtime.PrintTime() - estPrintTime - flushDelay
		if waitTime > 0.0 && runtime.CanPause() {
			completion.Wait(curTime+waitTime, nil)
			continue
		}
		nextSegmentPrintTime := math.Min(runtime.PrintTime()+config.DripSegmentTime, nextPrintTime)
		runtime.NoteMovequeueActivity(nextSegmentPrintTime+runtime.KinFlushDelay(), true)
		runtime.AdvanceMoveTime(nextSegmentPrintTime)
	}
	return nil
}