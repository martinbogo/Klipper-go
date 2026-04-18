package motion

import "math"

type ToolheadCoreState struct {
	CommandedPos          []float64
	MaxVelocity           float64
	MaxAccel              float64
	RequestedAccelToDecel float64
	MaxAccelToDecel       float64
	SquareCornerVelocity  float64
	JunctionDeviation     float64
	PrintTime             float64
	CheckStallTime        float64
	SpecialQueuingState   string
	NeedCheckPause        float64
	PrintStall            float64
	KinFlushDelay         float64
	KinFlushTimes         []float64
	LastStepGenTime       float64
	LastFlushTime         float64
	MinRestartTime        float64
	NeedFlushTime         float64
	StepGenTime           float64
	ClearHistoryTime      float64
	DoKickFlushTimer      bool
	CanPause              bool
}

func NewToolheadCoreState(canPause bool) ToolheadCoreState {
	return ToolheadCoreState{
		CommandedPos:        []float64{0.0, 0.0, 0.0, 0.0},
		SpecialQueuingState: "NeedPrime",
		NeedCheckPause:      -1.0,
		KinFlushDelay:       DefaultSdsCheckTime,
		KinFlushTimes:       []float64{},
		DoKickFlushTimer:    true,
		CanPause:            canPause,
	}
}

func (self ToolheadCoreState) CommandedPosition() []float64 {
	return append([]float64{}, self.CommandedPos...)
}

func (self *ToolheadCoreState) SetCommandedPosition(position []float64) {
	self.CommandedPos = append([]float64{}, position...)
}

func (self ToolheadCoreState) VelocitySettings() ToolheadVelocitySettings {
	return ToolheadVelocitySettings{
		MaxVelocity:           self.MaxVelocity,
		MaxAccel:              self.MaxAccel,
		RequestedAccelToDecel: self.RequestedAccelToDecel,
		SquareCornerVelocity:  self.SquareCornerVelocity,
	}
}

func (self *ToolheadCoreState) ApplyVelocityLimitResult(result ToolheadVelocityLimitResult) {
	self.MaxVelocity = result.Settings.MaxVelocity
	self.MaxAccel = result.Settings.MaxAccel
	self.RequestedAccelToDecel = result.Settings.RequestedAccelToDecel
	self.SquareCornerVelocity = result.Settings.SquareCornerVelocity
	self.JunctionDeviation = result.JunctionDeviation
	self.MaxAccelToDecel = result.MaxAccelToDecel
}

func (self ToolheadCoreState) MoveConfig() MoveConfig {
	return MoveConfig{
		Max_accel:          self.MaxAccel,
		Junction_deviation: self.JunctionDeviation,
		Max_velocity:       self.MaxVelocity,
		Max_accel_to_decel: self.MaxAccelToDecel,
	}
}

func (self ToolheadCoreState) WaitMovesState() ToolheadWaitMovesState {
	return ToolheadWaitMovesState{
		SpecialQueuingState: self.SpecialQueuingState,
		PrintTime:           self.PrintTime,
		CanPause:            self.CanPause,
	}
}

func (self ToolheadCoreState) PauseState() ToolheadPauseState {
	return ToolheadPauseState{
		PrintTime:           self.PrintTime,
		CheckStallTime:      self.CheckStallTime,
		PrintStall:          self.PrintStall,
		SpecialQueuingState: self.SpecialQueuingState,
		NeedCheckPause:      self.NeedCheckPause,
		CanPause:            self.CanPause,
	}
}

func (self *ToolheadCoreState) ApplyPauseState(state ToolheadPauseState) {
	self.PrintTime = state.PrintTime
	self.CheckStallTime = state.CheckStallTime
	self.PrintStall = state.PrintStall
	self.SpecialQueuingState = state.SpecialQueuingState
	self.NeedCheckPause = state.NeedCheckPause
	self.CanPause = state.CanPause
}

func (self ToolheadCoreState) FlushHandlerState() ToolheadFlushHandlerState {
	return ToolheadFlushHandlerState{
		PrintTime:           self.PrintTime,
		LastFlushTime:       self.LastFlushTime,
		LastStepGenTime:     self.LastStepGenTime,
		NeedFlushTime:       self.NeedFlushTime,
		NeedStepGenTime:     self.StepGenTime,
		SpecialQueuingState: self.SpecialQueuingState,
		KinFlushDelay:       self.KinFlushDelay,
	}
}

func (self ToolheadCoreState) TimingState() ToolheadTimingState {
	return ToolheadTimingState{
		PrintTime:        self.PrintTime,
		LastFlushTime:    self.LastFlushTime,
		LastStepGenTime:  self.LastStepGenTime,
		MinRestartTime:   self.MinRestartTime,
		NeedFlushTime:    self.NeedFlushTime,
		StepGenTime:      self.StepGenTime,
		ClearHistoryTime: self.ClearHistoryTime,
		KinFlushDelay:    self.KinFlushDelay,
		KinFlushTimes:    append([]float64{}, self.KinFlushTimes...),
		DoKickFlushTimer: self.DoKickFlushTimer,
		CanPause:         self.CanPause,
	}
}

func (self *ToolheadCoreState) ApplyTimingState(state ToolheadTimingState) {
	self.PrintTime = state.PrintTime
	self.LastFlushTime = state.LastFlushTime
	self.LastStepGenTime = state.LastStepGenTime
	self.MinRestartTime = state.MinRestartTime
	self.NeedFlushTime = state.NeedFlushTime
	self.StepGenTime = state.StepGenTime
	self.ClearHistoryTime = state.ClearHistoryTime
	self.KinFlushDelay = state.KinFlushDelay
	self.KinFlushTimes = append([]float64{}, state.KinFlushTimes...)
	self.DoKickFlushTimer = state.DoKickFlushTimer
	self.CanPause = state.CanPause
}

func (self *ToolheadCoreState) AdvanceFlushTime(flushTime float64, config ToolheadTimingConfig, actions FlushActions) {
	state := self.TimingState()
	state.AdvanceFlushTime(flushTime, config, actions)
	self.ApplyTimingState(state)
}

func (self *ToolheadCoreState) AdvanceMoveTime(nextPrintTime float64, config ToolheadTimingConfig, actions FlushActions) {
	state := self.TimingState()
	state.AdvanceMoveTime(nextPrintTime, config, actions)
	self.ApplyTimingState(state)
}

func (self *ToolheadCoreState) CalcPrintTime(config ToolheadTimingConfig, source PrintTimeSource, notifier SyncPrintTimeNotifier) {
	state := self.TimingState()
	state.CalcPrintTime(config, source, notifier)
	self.ApplyTimingState(state)
}

func (self *ToolheadCoreState) UpdateStepGenerationScanDelay(delay float64, oldDelay float64, config ToolheadTimingConfig) {
	state := self.TimingState()
	state.UpdateStepGenerationScanDelay(delay, oldDelay, config)
	self.ApplyTimingState(state)
}

func (self *ToolheadCoreState) NoteMovequeueActivity(mqTime float64, setStepGenTime bool) bool {
	state := self.TimingState()
	kickFlushTimer := state.NoteMovequeueActivity(mqTime, setStepGenTime)
	self.ApplyTimingState(state)
	return kickFlushTimer
}

func (self *ToolheadCoreState) ResetAfterLookaheadFlush(lookaheadFlushTime float64) ToolheadFlushReset {
	reset := BuildToolheadFlushReset(lookaheadFlushTime)
	self.SpecialQueuingState = reset.SpecialQueuingState
	self.NeedCheckPause = reset.NeedCheckPause
	self.CheckStallTime = reset.CheckStallTime
	return reset
}

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
	LastStepGenTime  float64
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
	self.LastStepGenTime = sgFlushTime
	self.MinRestartTime = math.Max(self.MinRestartTime, sgFlushTime)
	clearHistoryTime := self.ClearHistoryTime
	freeTime := sgFlushTime - self.KinFlushDelay
	if !self.CanPause {
		clearHistoryTime = math.Max(0.0, freeTime-config.MoveHistoryExpire)
	}
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
