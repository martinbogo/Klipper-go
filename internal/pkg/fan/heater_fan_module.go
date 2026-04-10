package fan

import (
	"strings"

	printerpkg "goklipper/internal/pkg/printer"
)

const heaterFanPinMinTime = 0.100

type moduleHeaterAdapter struct {
	heater printerpkg.HeaterRuntime
}

func (self *moduleHeaterAdapter) Get_temp(eventtime float64) (float64, float64) {
	return self.heater.GetTemperature(eventtime)
}

type HeaterFanModule struct {
	printer     printerpkg.ModulePrinter
	heaterNames []string
	core        *PrinterHeaterFan
	fan         *Fan
	lastSpeed   float64
}

func LoadConfigHeaterFan(config printerpkg.ModuleConfig) interface{} {
	return NewHeaterFanModule(config)
}

func NewHeaterFanModule(config printerpkg.ModuleConfig) *HeaterFanModule {
	_ = config.LoadObject("heaters")
	printer := config.Printer()
	fan := newConfiguredFan(config)
	self := &HeaterFanModule{
		printer:     printer,
		heaterNames: parseConfigList(config.String("heater", "extruder", true)),
		core:        NewPrinterHeaterFan(fan, config.Float("heater_temp", 50.0), config.Float("fan_speed", 1.0)),
		fan:         fan,
		lastSpeed:   0.0,
	}
	printer.RegisterEventHandler("project:ready", self.handleReady)
	printer.RegisterEventHandler("gcode:request_restart", self.handleRequestRestart)
	return self
}

func parseConfigList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func (self *HeaterFanModule) handleReady([]interface{}) error {
	coreHeaters := make([]Heater, 0, len(self.heaterNames))
	for _, name := range self.heaterNames {
		coreHeaters = append(coreHeaters, &moduleHeaterAdapter{heater: self.printer.LookupHeater(name)})
	}
	self.core.SetHeaters(coreHeaters)
	reactor := self.printer.Reactor()
	reactor.RegisterTimer(self.callback, reactor.Monotonic()+heaterFanPinMinTime)
	return nil
}

func (self *HeaterFanModule) handleRequestRestart(args []interface{}) error {
	if len(args) == 0 {
		panic("missing heater_fan restart print time")
	}
	printTime, ok := args[0].(float64)
	if !ok {
		panic("unexpected heater_fan restart print time type")
	}
	self.fan.HandleRequestRestart(printTime)
	return nil
}

func (self *HeaterFanModule) Get_status(eventtime float64) map[string]float64 {
	return self.core.Get_status(eventtime)
}

func (self *HeaterFanModule) callback(eventtime float64) float64 {
	return self.core.Callback(eventtime)
}