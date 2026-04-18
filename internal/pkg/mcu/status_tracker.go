package mcu

import "goklipper/common/utils/sys"

type MCUStatusTracker struct {
	statusInfo     map[string]interface{}
	statsSumsqBase float64
	tickAvg        float64
	tickStddev     float64
	tickAwake      float64
}

func NewMCUStatusTracker() *MCUStatusTracker {
	return &MCUStatusTracker{statusInfo: map[string]interface{}{}}
}

func (self *MCUStatusTracker) SetStatsSumsqBase(base float64) {
	self.statsSumsqBase = base
}

func (self *MCUStatusTracker) HandleMCUStats(params map[string]interface{}, mcuFreq float64) error {
	state := StatsState{TickAvg: self.tickAvg, TickStddev: self.tickStddev, TickAwake: self.tickAwake}
	state.HandleMCUStats(params, mcuFreq, self.statsSumsqBase)
	self.tickAvg = state.TickAvg
	self.tickStddev = state.TickStddev
	self.tickAwake = state.TickAwake
	return nil
}

func (self *MCUStatusTracker) SetStatusInfo(statusInfo map[string]interface{}) {
	if statusInfo == nil {
		self.statusInfo = map[string]interface{}{}
		return
	}
	self.statusInfo = statusInfo
}

func (self *MCUStatusTracker) StatusInfo() map[string]interface{} {
	if self.statusInfo == nil {
		self.statusInfo = map[string]interface{}{}
	}
	return self.statusInfo
}

func (self *MCUStatusTracker) GetStatus() map[string]interface{} {
	return sys.DeepCopyMap(self.StatusInfo())
}

func (self *MCUStatusTracker) Stats(mcuName string, serialStats string, clockSyncStats string) (bool, string) {
	state := StatsState{TickAvg: self.tickAvg, TickStddev: self.tickStddev, TickAwake: self.tickAwake}
	ok, summary, lastStats := state.BuildStatsSummary(mcuName, serialStats, clockSyncStats)
	self.StatusInfo()["last_stats"] = lastStats
	return ok, summary
}
