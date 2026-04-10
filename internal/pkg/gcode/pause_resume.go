package gcode

import (
	"errors"
	"fmt"
)

var ErrAlreadyPaused = errors.New("print already paused")
var ErrNotPaused = errors.New("print is not paused")

type PauseResumeGCode interface {
	Run_script(script string)
	Run_script_from_command(script string)
	Respond_info(msg string, log bool)
}

type PauseResumeSD interface {
	Is_active() bool
	Do_pause()
	Do_resume() error
	Do_cancel()
}

type PauseResume struct {
	gcode            PauseResumeGCode
	recoverVelocity  float64
	virtualSD        PauseResumeSD
	IsPaused         bool
	SdPaused         bool
	PauseCommandSent bool
}

func NewPauseResume(gcode PauseResumeGCode, recoverVelocity float64) *PauseResume {
	return &PauseResume{
		gcode:           gcode,
		recoverVelocity: recoverVelocity,
	}
}

func (self *PauseResume) SetVirtualSD(vsd PauseResumeSD) {
	self.virtualSD = vsd
}

func (self *PauseResume) Get_status(eventtime float64) map[string]interface{} {
	return map[string]interface{}{
		"is_paused": self.IsPaused,
	}
}

func (self *PauseResume) Is_sd_active() bool {
	return self.virtualSD != nil && self.virtualSD.Is_active()
}

func (self *PauseResume) Send_pause_command() {
	if !self.PauseCommandSent {
		if self.Is_sd_active() {
			self.SdPaused = true
			self.virtualSD.Do_pause()
		} else {
			self.SdPaused = false
			self.gcode.Respond_info("action:paused", true)
		}
		self.PauseCommandSent = true
	}
}

func (self *PauseResume) Pause() error {
	if self.IsPaused {
		return ErrAlreadyPaused
	}
	self.Send_pause_command()
	self.gcode.Run_script_from_command("SAVE_GCODE_STATE NAME=PAUSE_STATE")
	self.IsPaused = true
	return nil
}

func (self *PauseResume) Send_resume_command() {
	if self.SdPaused {
		self.virtualSD.Do_resume()
		self.SdPaused = false
	} else {
		self.gcode.Respond_info("action:resumed", true)
	}
	self.PauseCommandSent = false
}

func (self *PauseResume) Resume(velocity float64) error {
	if !self.IsPaused {
		return ErrNotPaused
	}
	self.gcode.Run_script_from_command(fmt.Sprintf("RESTORE_GCODE_STATE NAME=PAUSE_STATE MOVE=1 MOVE_SPEED=%.4f", velocity))
	self.Send_resume_command()
	self.IsPaused = false
	return nil
}

func (self *PauseResume) ClearPause() {
	self.IsPaused = false
	self.PauseCommandSent = false
	self.SdPaused = false
}

func (self *PauseResume) CancelPrint() bool {
	if self.Is_sd_active() || self.SdPaused {
		if self.virtualSD != nil {
			self.virtualSD.Do_cancel()
		}
		self.ClearPause()
		return false
	}
	self.ClearPause()
	return true
}
