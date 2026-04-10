package heater

import (
	"strings"

	"goklipper/common/constants"
	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

type VerifyHeaterModule struct {
	printer    printerpkg.ModulePrinter
	heaterName string
	heater     printerpkg.HeaterRuntime
	core       *VerifyHeater
	checkTimer printerpkg.TimerHandle
}

func LoadConfigVerifyHeater(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	heaterName := strings.Split(config.Name(), " ")[1]
	defaultGainTime := 20.0
	if heaterName == "heater_bed" {
		defaultGainTime = 60.0
	}
	self := &VerifyHeaterModule{
		printer:    printer,
		heaterName: heaterName,
		heater:     nil,
		core: NewVerifyHeater(
			heaterName,
			config.Float("hysteresis", 5.0),
			config.Float("max_error", 120.0),
			config.Float("heating_gain", 2.0),
			config.Float("check_gain_time", defaultGainTime),
		),
		checkTimer: nil,
	}
	printer.RegisterEventHandler("project:connect", self.handleConnect)
	printer.RegisterEventHandler("project:shutdown", self.handleShutdown)
	return self
}

func (self *VerifyHeaterModule) handleConnect([]interface{}) error {
	if self.printer.HasStartArg("debugoutput") {
		return nil
	}
	self.heater = self.printer.LookupHeater(self.heaterName)
	logger.Infof("Starting heater checks for %s", self.heaterName)
	self.checkTimer = self.printer.Reactor().RegisterTimer(self.checkEvent, constants.NOW)
	return nil
}

func (self *VerifyHeaterModule) handleShutdown([]interface{}) error {
	if self.checkTimer != nil {
		self.checkTimer.Update(constants.NEVER)
	}
	return nil
}

func (self *VerifyHeaterModule) checkEvent(eventtime float64) float64 {
	temp, target := self.heater.GetTemperature(eventtime)
	next, faultMsg := self.core.Check(eventtime, temp, target)
	if faultMsg != "" {
		return self.heaterFault(faultMsg)
	}
	return next
}

func (self *VerifyHeaterModule) heaterFault(msg string) float64 {
	logger.Error(msg)
	self.printer.InvokeShutdown(msg + HintThermal)
	return constants.NEVER
}