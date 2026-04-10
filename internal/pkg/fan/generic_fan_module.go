package fan

import (
	"fmt"
	printerpkg "goklipper/internal/pkg/printer"
	"strings"
)

const cmdSetFanSpeedHelp = "Sets the speed of a fan"

type genericFanGCodeRuntime interface {
	printerpkg.GCodeRuntime
	RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string)
}

type PrinterFanGenericModule struct {
	core *GenericFan
	fan  *Fan
}

func LoadConfigGenericFan(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	nameParts := strings.Split(config.Name(), " ")
	fanName := nameParts[len(nameParts)-1]
	coreFan := newConfiguredFan(config)
	self := &PrinterFanGenericModule{
		core: NewGenericFan(fanName, coreFan),
		fan:  coreFan,
	}
	gcodeObj := printer.GCode()
	gcode, ok := gcodeObj.(genericFanGCodeRuntime)
	if !ok {
		panic(fmt.Sprintf("gcode runtime does not implement genericFanGCodeRuntime: %T", gcodeObj))
	}
	gcode.RegisterMuxCommand("SET_FAN_SPEED", "FAN", fanName, self.cmdSetFanSpeed, cmdSetFanSpeedHelp)
	printer.RegisterEventHandler("gcode:request_restart", self.handleRequestRestart)
	return self
}

func (self *PrinterFanGenericModule) Get_status(eventtime float64) map[string]float64 {
	return self.core.Get_status(eventtime)
}

func (self *PrinterFanGenericModule) cmdSetFanSpeed(gcmd printerpkg.Command) error {
	self.core.SetSpeedFromCommand(gcmd.Float("SPEED", 0.0))
	return nil
}

func (self *PrinterFanGenericModule) handleRequestRestart(args []interface{}) error {
	if len(args) == 0 {
		panic("missing generic fan restart print time")
	}
	printTime, ok := args[0].(float64)
	if !ok {
		panic(fmt.Sprintf("unexpected generic fan restart print time type: %T", args[0]))
	}
	self.fan.HandleRequestRestart(printTime)
	return nil
}