package mcu

import (
	"errors"
	"math"
)

var ErrStepcompress = errors.New("internal error in stepcompress")

type QuerySlotSource interface {
	SecondsToClock(time float64) int64
	EstimatedPrintTime(eventtime float64) float64
	Monotonic() float64
	PrintTimeToClock(printTime float64) int64
}

func QuerySlot(oid int, source QuerySlotSource) int64 {
	slot := source.SecondsToClock(float64(oid) * 0.01)
	t := source.EstimatedPrintTime(source.Monotonic()) + 1.5
	return source.PrintTimeToClock(t) + slot
}

type StepperSync interface {
	SetTime(offset float64, freq float64)
	Flush(clock uint64, clearHistoryClock uint64) int
}

type MoveQueueTimingSource interface {
	PrintTimeToClock(printTime float64) int64
	CalibrateClock(printTime float64, eventtime float64) []float64
	ClockSyncActive() bool
	IsFileoutput() bool
}

func FlushMoves(printTime float64, clearHistoryTime float64, source MoveQueueTimingSource, sync StepperSync, callbacks []func(float64, int64)) error {
	if sync == nil {
		return nil
	}
	clock := source.PrintTimeToClock(printTime)
	if clock < 0 {
		return nil
	}
	for _, cb := range callbacks {
		cb(printTime, clock)
	}
	clearHistoryClock := math.Max(0, float64(source.PrintTimeToClock(clearHistoryTime)))
	if sync.Flush(uint64(clock), uint64(clearHistoryClock)) != 0 {
		return ErrStepcompress
	}
	return nil
}

type MoveQueueTimingState struct {
	IsTimeout bool
}

func (self *MoveQueueTimingState) CheckActive(printTime float64, eventtime float64, source MoveQueueTimingSource, sync StepperSync) bool {
	if sync == nil {
		return false
	}
	clock := source.CalibrateClock(printTime, eventtime)
	sync.SetTime(clock[0], clock[1])
	if source.ClockSyncActive() || source.IsFileoutput() || self.IsTimeout {
		return false
	}
	self.IsTimeout = true
	return true
}

type StepGenerationOps interface {
	CheckActive(flushTime float64) float64
	Generate(flushTime float64) int32
}

type StepGenerationState struct {
	ActiveCallbacks []func(float64)
}

func (self *StepGenerationState) GenerateSteps(flushTime float64, ops StepGenerationOps) error {
	if len(self.ActiveCallbacks) > 0 {
		if activeTime := ops.CheckActive(flushTime); activeTime > 0 {
			callbacks := self.ActiveCallbacks
			self.ActiveCallbacks = nil
			for _, cb := range callbacks {
				cb(activeTime)
			}
		}
	}
	if ops.Generate(flushTime) > 0 {
		return ErrStepcompress
	}
	return nil
}
