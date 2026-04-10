package heater

import (
	"fmt"

	printerpkg "goklipper/internal/pkg/printer"
)

type heaterBedHeater interface {
	Get_status(eventtime float64) map[string]float64
	Stats(eventtime float64) (bool, string)
}

type heaterBedManager interface {
	SetupHeater(config printerpkg.ModuleConfig, gcodeID string) interface{}
	Set_temperature(heater interface{}, temp float64, wait bool) error
}

type HeaterBedModule struct {
	*BedController
}

func LoadConfigHeaterBed(config printerpkg.ModuleConfig) interface{} {
	config.LoadObject("heaters")
	printer := config.Printer()
	heatersObj := printer.LookupObject("heaters", nil)
	heaters, ok := heatersObj.(heaterBedManager)
	if !ok {
		panic(fmt.Sprintf("heaters object does not implement heaterBedManager: %T", heatersObj))
	}

	heaterObj := heaters.SetupHeater(config, "B")
	heater, ok := heaterObj.(heaterBedHeater)
	if !ok || heater == nil {
		panic(fmt.Sprintf("heater_bed setup did not return a compatible heater: %T", heaterObj))
	}

	self := &HeaterBedModule{
		BedController: NewBedController(
			func(eventtime float64) map[string]float64 {
				return heater.Get_status(eventtime)
			},
			func(eventtime float64) (bool, string) {
				return heater.Stats(eventtime)
			},
			func(temp float64, wait bool) error {
				return heaters.Set_temperature(heaterObj, temp, wait)
			},
		),
	}

	gcode := printer.GCode()
	gcode.RegisterCommand("M140", self.cmdM140, false, "")
	gcode.RegisterCommand("M190", self.cmdM190, false, "")
	return self
}

func (self *HeaterBedModule) cmdM140(gcmd printerpkg.Command) error {
	return self.setTemperature(gcmd, false)
}

func (self *HeaterBedModule) cmdM190(gcmd printerpkg.Command) error {
	return self.setTemperature(gcmd, true)
}

func (self *HeaterBedModule) setTemperature(gcmd printerpkg.Command, wait bool) error {
	return self.BedController.SetTemperature(gcmd.Float("S", 0.0), wait)
}
