package addon

import (
	"math"
	printerpkg "goklipper/internal/pkg/printer"
	"strings"
)

type homingOverrideToolhead interface {
	Get_position() []float64
	Set_position(newpos []float64, homingAxes []int)
}

type HomingOverrideModule struct {
	printer  printerpkg.ModulePrinter
	template printerpkg.Template
	gcode    printerpkg.GCodeRuntime
	prevG28  func(printerpkg.Command) error
	core     *HomingOverride
}

func LoadConfigHomingOverride(config printerpkg.ModuleConfig) interface{} {
	startPos := make([]float64, 0, 3)
	for _, axis := range "xyz" {
		value := config.OptionalFloat("set_position_" + string(axis))
		if value == nil {
			startPos = append(startPos, math.NaN())
			continue
		}
		startPos = append(startPos, *value)
	}

	printer := config.Printer()
	self := &HomingOverrideModule{
		printer:  printer,
		template: config.LoadRequiredTemplate("gcode_macro", "gcode"),
		gcode:    printer.GCode(),
		core:     NewHomingOverride(startPos, strings.ToUpper(config.String("axes", "XYZ", true))),
	}
	config.LoadObject("homing")
	self.prevG28 = self.gcode.ReplaceCommand("G28", self.cmdG28, false, "")
	return self
}

func (self *HomingOverrideModule) cmdG28(gcmd printerpkg.Command) error {
	if self.core.InScript {
		if self.prevG28 != nil {
			return self.prevG28(gcmd)
		}
		return nil
	}

	requestedAxes := map[string]bool{}
	for _, axis := range "XYZ" {
		if gcmd.String(string(axis), "") != "" {
			requestedAxes[string(axis)] = true
		}
	}

	if !self.core.ShouldOverride(requestedAxes) {
		if self.prevG28 != nil {
			return self.prevG28(gcmd)
		}
		return nil
	}

	toolhead := self.printer.LookupObject("toolhead", nil).(homingOverrideToolhead)
	pos, homingAxes := self.core.ApplyStartPosition(toolhead.Get_position())
	toolhead.Set_position(pos, homingAxes)

	context := self.template.CreateContext(nil)
	context["params"] = gcmd.Parameters()
	self.core.SetInScript(true)
	defer self.core.SetInScript(false)
	return self.template.RunGcodeFromCommand(context)
}