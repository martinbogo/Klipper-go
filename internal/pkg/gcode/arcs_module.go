package gcode

import (
	"strconv"

	printerpkg "goklipper/internal/pkg/printer"
)

type arcCommandAdapter struct {
	cmd printerpkg.Command
}

func (self *arcCommandAdapter) GetFloat(name string, defaultValue float64) float64 {
	return self.cmd.Float(name, defaultValue)
}

func (self *arcCommandAdapter) GetFloatP(name string) *float64 {
	value, ok := self.cmd.Parameters()[name]
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		panic(err)
	}
	return &parsed
}

type ArcSupportModule struct {
	gcode     printerpkg.GCodeRuntime
	gcodeMove printerpkg.MoveTransformController
	core      *ArcSupport
}

func LoadConfigArcSupport(config printerpkg.ModuleConfig) interface{} {
	self := &ArcSupportModule{
		gcode:     config.Printer().GCode(),
		gcodeMove: config.Printer().GCodeMove(),
		core:      NewArcSupport(config.Float("resolution", 1.0)),
	}
	self.gcode.RegisterCommand("G2", self.cmdG2, false, "")
	self.gcode.RegisterCommand("G3", self.cmdG3, false, "")
	self.gcode.RegisterCommand("G17", self.cmdG17, false, "")
	self.gcode.RegisterCommand("G18", self.cmdG18, false, "")
	self.gcode.RegisterCommand("G19", self.cmdG19, false, "")
	self.core.SetPlaneXY()
	return self
}

func (self *ArcSupportModule) cmdG2(gcmd printerpkg.Command) error {
	return self.cmdInner(gcmd, true)
}

func (self *ArcSupportModule) cmdG3(gcmd printerpkg.Command) error {
	return self.cmdInner(gcmd, false)
}

func (self *ArcSupportModule) cmdG17(gcmd printerpkg.Command) error {
	self.core.SetPlaneXY()
	return nil
}

func (self *ArcSupportModule) cmdG18(gcmd printerpkg.Command) error {
	self.core.SetPlaneXZ()
	return nil
}

func (self *ArcSupportModule) cmdG19(gcmd printerpkg.Command) error {
	self.core.SetPlaneYZ()
	return nil
}

func (self *ArcSupportModule) cmdInner(gcmd printerpkg.Command, clockwise bool) error {
	state := self.gcodeMove.State()
	arcState := ArcState{
		GCodePosition:       state.GCodePosition,
		AbsoluteCoordinates: state.AbsoluteCoordinates,
		AbsoluteExtrude:     state.AbsoluteExtrude,
	}
	cmd := &arcCommandAdapter{cmd: gcmd}
	if clockwise {
		return self.core.CmdG2(cmd, arcState, self.gcodeMove.LinearMove)
	}
	return self.core.CmdG3(cmd, arcState, self.gcodeMove.LinearMove)
}
