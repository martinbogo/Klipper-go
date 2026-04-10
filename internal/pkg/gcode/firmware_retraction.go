package gcode

import "fmt"

type ScriptRunner interface {
	RunScriptFromCommand(script string)
}

type FirmwareRetraction struct {
	retractLength         float64
	retractSpeed          float64
	unretractExtraLength float64
	unretractSpeed        float64
	unretractLength       float64
	isRetracted           bool
	runner                ScriptRunner
}

func NewFirmwareRetraction(retractLength float64, retractSpeed float64,
	unretractExtraLength float64, unretractSpeed float64,
	runner ScriptRunner) *FirmwareRetraction {
	self := &FirmwareRetraction{}
	self.retractLength = retractLength
	self.retractSpeed = retractSpeed
	self.unretractExtraLength = unretractExtraLength
	self.unretractSpeed = unretractSpeed
	self.unretractLength = retractLength + unretractExtraLength
	self.isRetracted = false
	self.runner = runner
	return self
}

func (self *FirmwareRetraction) Get_status(eventtime float64) map[string]float64 {
	return map[string]float64{
		"retract_length":         self.retractLength,
		"retract_speed":          self.retractSpeed,
		"unretract_extra_length": self.unretractExtraLength,
		"unretract_speed":        self.unretractSpeed,
	}
}

func (self *FirmwareRetraction) UpdateParameters(retractLength float64, retractSpeed float64,
	unretractExtraLength float64, unretractSpeed float64) {
	self.retractLength = retractLength
	self.retractSpeed = retractSpeed
	self.unretractExtraLength = unretractExtraLength
	self.unretractSpeed = unretractSpeed
	self.unretractLength = self.retractLength + self.unretractExtraLength
	self.isRetracted = false
}

func (self *FirmwareRetraction) RetractLength() float64 {
	return self.retractLength
}

func (self *FirmwareRetraction) RetractSpeed() float64 {
	return self.retractSpeed
}

func (self *FirmwareRetraction) UnretractExtraLength() float64 {
	return self.unretractExtraLength
}

func (self *FirmwareRetraction) UnretractSpeed() float64 {
	return self.unretractSpeed
}

func (self *FirmwareRetraction) RetractionMessage() string {
	return fmt.Sprintf(
		"RETRACT_LENGTH=%.5f RETRACT_SPEED=%.5f UNRETRACT_EXTRA_LENGTH=%.5f UNRETRACT_SPEED=%.5f",
		self.retractLength,
		self.retractSpeed,
		self.unretractExtraLength,
		self.unretractSpeed,
	)
}

func (self *FirmwareRetraction) Retract() {
	if self.isRetracted || self.runner == nil {
		return
	}
	self.runner.RunScriptFromCommand(fmt.Sprintf(
		"SAVE_GCODE_STATE NAME=_retract_state\n"+
			"G91\n"+
			"G1 E-%.5f F%d\n"+
			"RESTORE_GCODE_STATE NAME=_retract_state",
		self.retractLength, int64(self.retractSpeed*60)))
	self.isRetracted = true
}

func (self *FirmwareRetraction) Unretract() {
	if !self.isRetracted || self.runner == nil {
		return
	}
	self.runner.RunScriptFromCommand(fmt.Sprintf(
		"SAVE_GCODE_STATE NAME=_retract_state\n"+
			"G91\n"+
			"G1 E%.5f F%d\n"+
			"RESTORE_GCODE_STATE NAME=_retract_state",
		self.unretractLength, int64(self.unretractSpeed*60)))
	self.isRetracted = false
}