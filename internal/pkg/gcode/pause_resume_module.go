package gcode

import printerpkg "goklipper/internal/pkg/printer"

const CmdPauseHelp = "Pauses the current print"
const CmdResumeHelp = "Resumes the print from a pause"
const CmdClearPauseHelp = "Clears the current paused state without resuming the print"
const CmdCancelPrintHelp = "Cancel the current print"

type PauseResumeModule struct {
	printer          printerpkg.ModulePrinter
	gcode            PauseResumeGCode
	recoverVelocity  float64
	core             *PauseResume
}

func NewPauseResumeModule(printer printerpkg.ModulePrinter, gcode PauseResumeGCode, recoverVelocity float64, webhooks printerpkg.WebhookRegistry) *PauseResumeModule {
	self := &PauseResumeModule{
		printer:         printer,
		gcode:           gcode,
		recoverVelocity: recoverVelocity,
	}
	self.core = NewPauseResume(gcode, recoverVelocity)

	self.printer.RegisterEventHandler("project:connect", self.Handle_connect)
	self.printer.GCode().RegisterCommand("PAUSE", self.cmdPause, false, CmdPauseHelp)
	self.printer.GCode().RegisterCommand("RESUME", self.cmdResume, false, CmdResumeHelp)
	self.printer.GCode().RegisterCommand("CLEAR_PAUSE", self.cmdClearPause, false, CmdClearPauseHelp)
	self.printer.GCode().RegisterCommand("CANCEL_PRINT", self.cmdCancelPrint, false, CmdCancelPrintHelp)

	_ = webhooks.RegisterEndpoint("pause_resume/cancel", self.handleCancelRequest)
	_ = webhooks.RegisterEndpoint("pause_resume/pause", self.handlePauseRequest)
	_ = webhooks.RegisterEndpoint("pause_resume/resume", self.handleResumeRequest)
	return self
}

func LoadConfigPauseResume(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	gcode := printer.LookupObject("gcode", nil).(PauseResumeGCode)
	return NewPauseResumeModule(printer, gcode, config.Float("recover_velocity", 50.), printer.Webhooks())
}

func (self *PauseResumeModule) Handle_connect(args []interface{}) error {
	obj := self.printer.LookupObject("virtual_sdcard", nil)
	if obj == nil {
		self.core.SetVirtualSD(nil)
		return nil
	}
	if vsd, ok := obj.(PauseResumeSD); ok {
		self.core.SetVirtualSD(vsd)
	}
	return nil
}

func (self *PauseResumeModule) handleCancelRequest() (interface{}, error) {
	self.gcode.Run_script("CANCEL_PRINT")
	return nil, nil
}

func (self *PauseResumeModule) handlePauseRequest() (interface{}, error) {
	self.gcode.Run_script("PAUSE")
	return nil, nil
}

func (self *PauseResumeModule) handleResumeRequest() (interface{}, error) {
	self.gcode.Run_script("RESUME")
	return nil, nil
}

func (self *PauseResumeModule) Get_status(eventtime float64) map[string]interface{} {
	return self.core.Get_status(eventtime)
}

func (self *PauseResumeModule) Is_sd_active() bool {
	return self.core.Is_sd_active()
}

func (self *PauseResumeModule) Send_pause_command() {
	self.core.Send_pause_command()
}

func (self *PauseResumeModule) Send_resume_command() {
	self.core.Send_resume_command()
}

func (self *PauseResumeModule) cmdPause(gcmd printerpkg.Command) error {
	if err := self.core.Pause(); err != nil {
		gcmd.RespondInfo("Print already paused", true)
	}
	return nil
}

func (self *PauseResumeModule) cmdResume(gcmd printerpkg.Command) error {
	if !self.core.IsPaused {
		gcmd.RespondInfo("Print is not paused, resume aborted", true)
		return nil
	}
	if err := self.core.Resume(gcmd.Float("VELOCITY", self.recoverVelocity)); err != nil {
		gcmd.RespondInfo("Print is not paused, resume aborted", true)
	}
	return nil
}

func (self *PauseResumeModule) cmdClearPause(gcmd printerpkg.Command) error {
	self.core.ClearPause()
	return nil
}

func (self *PauseResumeModule) cmdCancelPrint(gcmd printerpkg.Command) error {
	if self.core.CancelPrint() {
		gcmd.RespondInfo("action:cancel", true)
	}
	return nil
}