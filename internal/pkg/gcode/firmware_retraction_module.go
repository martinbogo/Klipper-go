package gcode

import (
	"fmt"
	"strconv"

	printerpkg "goklipper/internal/pkg/printer"
)

const (
	cmdSetRetractionHelp = "Set firmware retraction parameters"
	cmdGetRetractionHelp = "Report firmware retraction paramters"
)

type firmwareRetractionRunner struct {
	gcode printerpkg.GCodeRuntime
}

func (self *firmwareRetractionRunner) RunScriptFromCommand(script string) {
	self.gcode.RunScriptFromCommand(script)
}

type FirmwareRetractionModule struct {
	core *FirmwareRetraction
}

func LoadConfigFirmwareRetraction(config printerpkg.ModuleConfig) interface{} {
	retractLength := config.Float("retract_length", 0)
	retractSpeed := config.Float("retract_speed", 20.0)
	unretractExtraLength := config.Float("unretract_extra_length", 0.0)
	unretractSpeed := config.Float("unretract_speed", 10.0)
	gcode := config.Printer().GCode()
	self := &FirmwareRetractionModule{
		core: NewFirmwareRetraction(
			retractLength,
			retractSpeed,
			unretractExtraLength,
			unretractSpeed,
			&firmwareRetractionRunner{gcode: gcode},
		),
	}
	gcode.RegisterCommand("SET_RETRACTION", self.cmdSetRetraction, false, cmdSetRetractionHelp)
	gcode.RegisterCommand("GET_RETRACTION", self.cmdGetRetraction, false, cmdGetRetractionHelp)
	gcode.RegisterCommand("G10", self.cmdG10, false, "")
	gcode.RegisterCommand("G11", self.cmdG11, false, "")
	return self
}

func (self *FirmwareRetractionModule) Get_status(eventtime float64) map[string]float64 {
	return self.core.Get_status(eventtime)
}

func (self *FirmwareRetractionModule) cmdSetRetraction(gcmd printerpkg.Command) error {
	retractLength := commandFloatWithMin(gcmd, "RETRACT_LENGTH", self.core.RetractLength(), 0.0)
	retractSpeed := commandFloatWithMin(gcmd, "RETRACT_SPEED", self.core.RetractSpeed(), 1.0)
	unretractExtraLength := commandFloatWithMin(gcmd, "UNRETRACT_EXTRA_LENGTH", self.core.UnretractExtraLength(), 0.0)
	unretractSpeed := commandFloatWithMin(gcmd, "UNRETRACT_SPEED", self.core.UnretractSpeed(), 1.0)
	self.core.UpdateParameters(retractLength, retractSpeed, unretractExtraLength, unretractSpeed)
	return nil
}

func (self *FirmwareRetractionModule) cmdGetRetraction(gcmd printerpkg.Command) error {
	gcmd.RespondInfo(self.core.RetractionMessage(), true)
	return nil
}

func (self *FirmwareRetractionModule) cmdG10(gcmd printerpkg.Command) error {
	self.core.Retract()
	return nil
}

func (self *FirmwareRetractionModule) cmdG11(gcmd printerpkg.Command) error {
	self.core.Unretract()
	return nil
}

func commandFloatWithMin(gcmd printerpkg.Command, name string, defaultValue float64, minValue float64) float64 {
	value, ok := gcmd.Parameters()[name]
	if !ok {
		return defaultValue
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		panic(err)
	}
	if parsed < minValue {
		panic(fmt.Sprintf("%s must be >= %.3f", name, minValue))
	}
	return parsed
}
