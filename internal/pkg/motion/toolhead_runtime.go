package motion

import "math"

type MoveQueueFlusher interface {
	FlushMoves(flushTime float64, clearHistoryTime float64)
}

type FlushActions struct {
	StepGenerators         []func(float64)
	FinalizeMoves          func(freeTime float64, clearHistoryTime float64)
	UpdateExtruderMoveTime func(freeTime float64, clearHistoryTime float64)
	FlushDrivers           []MoveQueueFlusher
}

type PrintTimeSource interface {
	Monotonic() float64
	EstimatedPrintTime(eventtime float64) float64
}

type SyncPrintTimeNotifier interface {
	SyncPrintTime(curTime float64, estPrintTime float64, printTime float64)
}

type ToolheadTimingConfig struct {
	BufferTimeStart       float64
	MinKinTime            float64
	MoveBatchTime         float64
	MoveHistoryExpire     float64
	ScanTimeOffset        float64
	StepcompressFlushTime float64
}

type ToolheadTimingState struct {
	PrintTime        float64
	LastFlushTime    float64
	MinRestartTime   float64
	NeedFlushTime    float64
	StepGenTime      float64
	ClearHistoryTime float64
	KinFlushDelay    float64
	KinFlushTimes    []float64
	DoKickFlushTimer bool
	CanPause         bool
}

func (self *ToolheadTimingState) AdvanceFlushTime(flushTime float64, config ToolheadTimingConfig, actions FlushActions) {
	flushTime = math.Max(flushTime, self.LastFlushTime)
	sgFlushWant := math.Min(flushTime+config.StepcompressFlushTime, self.PrintTime-self.KinFlushDelay)
	sgFlushTime := math.Max(sgFlushWant, flushTime)
	for _, sg := range actions.StepGenerators {
		sg(sgFlushTime)
	}
	self.MinRestartTime = math.Max(self.MinRestartTime, sgFlushTime)
	clearHistoryTime := self.ClearHistoryTime
	if !self.CanPause {
		clearHistoryTime = flushTime - config.MoveHistoryExpire
	}
	freeTime := sgFlushTime - self.KinFlushDelay
	if actions.FinalizeMoves != nil {
		actions.FinalizeMoves(freeTime, clearHistoryTime)
	}
	if actions.UpdateExtruderMoveTime != nil {
		actions.UpdateExtruderMoveTime(freeTime, clearHistoryTime)
	}
	for _, flusher := range actions.FlushDrivers {
		flusher.FlushMoves(flushTime, clearHistoryTime)
	}
	self.LastFlushTime = flushTime
}

func (self *ToolheadTimingState) AdvanceMoveTime(nextPrintTime float64, config ToolheadTimingConfig, actions FlushActions) {
	ptDelay := self.KinFlushDelay + config.StepcompressFlushTime
	flushTime := math.Max(self.LastFlushTime, self.PrintTime-ptDelay)
	self.PrintTime = math.Max(self.PrintTime, nextPrintTime)
	wantFlushTime := math.Max(flushTime, self.PrintTime-ptDelay)
	for {
		flushTime = math.Min(flushTime+config.MoveBatchTime, wantFlushTime)
		self.AdvanceFlushTime(flushTime, config, actions)
		if flushTime >= wantFlushTime {
			break
		}
	}
}

func (self *ToolheadTimingState) CalcPrintTime(config ToolheadTimingConfig, source PrintTimeSource, notifier SyncPrintTimeNotifier) {
	curTime := source.Monotonic()
	estPrintTime := source.EstimatedPrintTime(curTime)
	kinTime := math.Max(estPrintTime+config.MinKinTime, self.MinRestartTime)
	kinTime += self.KinFlushDelay
	minPrintTime := math.Max(estPrintTime+config.BufferTimeStart, kinTime)
	if minPrintTime <= self.PrintTime {
		return
	}
	self.PrintTime = minPrintTime
	if notifier != nil {
		notifier.SyncPrintTime(curTime, estPrintTime, self.PrintTime)
	}
}

func (self *ToolheadTimingState) UpdateStepGenerationScanDelay(delay float64, oldDelay float64, config ToolheadTimingConfig) {
	if oldDelay != 0.0 {
		for i, value := range self.KinFlushTimes {
			if value != oldDelay {
				continue
			}
			self.KinFlushTimes = append(self.KinFlushTimes[:i], self.KinFlushTimes[i+1:]...)
			break
		}
	}
	if delay != 0.0 {
		self.KinFlushTimes = append(self.KinFlushTimes, delay)
	}
	newDelay := 0.0
	for _, value := range self.KinFlushTimes {
		value += config.ScanTimeOffset
		if value > newDelay {
			newDelay = value
		}
	}
	self.KinFlushDelay = newDelay
}

func (self *ToolheadTimingState) NoteMovequeueActivity(mqTime float64, setStepGenTime bool) bool {
	self.NeedFlushTime = math.Max(self.NeedFlushTime, mqTime)
	if setStepGenTime {
		self.StepGenTime = math.Max(self.StepGenTime, mqTime)
	}
	if !self.DoKickFlushTimer {
		return false
	}
	self.DoKickFlushTimer = false
	return true
}
