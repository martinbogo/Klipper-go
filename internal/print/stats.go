package print

import (
	"goklipper/common/utils/maths"
	"math"
)

type ExtrusionStatus struct {
	Position      float64
	ExtrudeFactor float64
}

type Stats struct {
	filamentUsed      float64
	lastEPos          float64
	filename          string
	printStartTime    float64
	lastPauseTime     float64
	prevPauseDuration float64
	state             string
	errorMessage      string
	totalDuration     float64
	initDuration      float64
	infoTotalLayer    int
	infoCurrentLayer  int
}

func NewStats() *Stats {
	self := &Stats{}
	self.Reset()
	return self
}

func (self *Stats) Filename() string {
	return self.filename
}

func (self *Stats) InfoTotalLayer() int {
	return self.infoTotalLayer
}

func (self *Stats) InfoCurrentLayer() int {
	return self.infoCurrentLayer
}

func (self *Stats) updateFilamentUsage(status ExtrusionStatus) {
	if status.ExtrudeFactor == 0 {
		self.lastEPos = status.Position
		return
	}
	self.filamentUsed += (status.Position - self.lastEPos) / status.ExtrudeFactor
	self.lastEPos = status.Position
}

func (self *Stats) SetCurrentFile(filename string) {
	self.Reset()
	self.filename = filename
}

func (self *Stats) NoteStart(eventtime float64, status ExtrusionStatus) {
	if self.printStartTime == 0 {
		self.printStartTime = eventtime
	} else if self.lastPauseTime != 0 {
		pauseDuration := eventtime - self.lastPauseTime
		self.prevPauseDuration += pauseDuration
		self.lastPauseTime = 0
	}

	self.lastEPos = status.Position
	self.state = "printing"
	self.errorMessage = ""
}

func (self *Stats) NotePause(eventtime float64, status ExtrusionStatus) {
	if self.lastPauseTime == 0 {
		self.lastPauseTime = eventtime
		self.updateFilamentUsage(status)
	}

	if self.state != "error" {
		self.state = "paused"
	}
}

func (self *Stats) NoteComplete(eventtime float64) {
	self.noteFinish("complete", "", eventtime)
}

func (self *Stats) NoteError(eventtime float64, message string) {
	self.noteFinish("error", message, eventtime)
}

func (self *Stats) NoteCancel(eventtime float64) {
	self.noteFinish("cancelled", "", eventtime)
}

func (self *Stats) noteFinish(state string, errorMessage string, eventtime float64) {
	if self.printStartTime == 0 {
		return
	}
	self.state = state
	self.errorMessage = errorMessage
	self.totalDuration = eventtime - self.printStartTime

	if self.filamentUsed < 0.0000001 {
		self.initDuration = self.totalDuration - self.prevPauseDuration
	}

	self.printStartTime = 0
}

func (self *Stats) SetInfo(totalLayer int, currentLayer int) {
	if totalLayer == 0 {
		self.infoTotalLayer = 0
		self.infoCurrentLayer = 0
	} else if totalLayer != self.infoTotalLayer {
		self.infoTotalLayer = totalLayer
		self.infoCurrentLayer = 0
	}

	if self.infoTotalLayer != 0 &&
		currentLayer != 0 &&
		currentLayer != self.infoCurrentLayer {
		self.infoCurrentLayer = maths.Min(currentLayer, self.infoTotalLayer)
	}
}

func (self *Stats) Reset() {
	self.filename, self.errorMessage = "", ""
	self.state = "standby"
	self.prevPauseDuration, self.lastEPos = 0., 0.
	self.filamentUsed, self.totalDuration = 0., 0.
	self.printStartTime, self.lastPauseTime = 0., 0.
	self.initDuration = 0.
	self.infoTotalLayer = 0
	self.infoCurrentLayer = 0
}

func (self *Stats) GetStatus(eventtime float64, status ExtrusionStatus) map[string]interface{} {
	timePaused := self.prevPauseDuration
	if self.printStartTime != 0 {
		if self.lastPauseTime != 0 {
			timePaused += eventtime - self.lastPauseTime
		} else {
			self.updateFilamentUsage(status)
		}
		self.totalDuration = eventtime - self.printStartTime
		if self.filamentUsed < 0.0000001 {
			self.initDuration = self.totalDuration - timePaused
		}
	}

	printDuration := self.totalDuration - self.initDuration - timePaused

	return map[string]interface{}{
		"filename":       self.filename,
		"total_duration": math.Round(self.totalDuration),
		"print_duration": math.Round(printDuration),
		"filament_used":  math.Round(self.filamentUsed),
		"state":          self.state,
		"message":        self.errorMessage,
		"info": map[string]int{
			"total_layer":   self.infoTotalLayer,
			"current_layer": self.infoCurrentLayer,
		},
	}
}
