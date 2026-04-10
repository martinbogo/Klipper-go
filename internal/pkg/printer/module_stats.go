package printer

import (
	"goklipper/common/constants"
	"goklipper/common/logger"
)

type PrinterSysStatsModule struct {
	core *SystemStats
}

func NewPrinterSysStatsModule(config ModuleConfig) *PrinterSysStatsModule {
	self := &PrinterSysStatsModule{
		core: NewSystemStats("/proc/meminfo"),
	}
	config.Printer().RegisterEventHandler("project:disconnect", self.handleDisconnect)
	return self
}

func (self *PrinterSysStatsModule) handleDisconnect(_ []interface{}) error {
	return self.core.Close()
}

func (self *PrinterSysStatsModule) Stats(eventtime float64) (bool, string) {
	return self.core.Stats(eventtime)
}

func (self *PrinterSysStatsModule) Get_status(eventtime float64) map[string]float64 {
	return self.core.GetStatus(eventtime)
}

type PrinterStatsModule struct {
	printer    ModulePrinter
	core       *StatsCollector
	statsTimer TimerHandle
}

func NewPrinterStatsModule(config ModuleConfig) *PrinterStatsModule {
	printer := config.Printer()
	self := &PrinterStatsModule{
		printer: printer,
		core:    NewStatsCollector(),
	}
	self.statsTimer = printer.Reactor().RegisterTimer(self.Generate_stats, constants.NEVER)
	printer.RegisterEventHandler("project:ready", self.Handle_ready)
	return self
}

func (self *PrinterStatsModule) Handle_ready(_ []interface{}) error {
	self.core.RegisterObjects(self.printer.LookupObjects(""))
	self.statsTimer.Update(constants.NOW)
	return nil
}

func (self *PrinterStatsModule) Generate_stats(eventtime float64) float64 {
	nextWake, msg, shouldLog := self.core.GenerateStats(eventtime, self.printer.IsShutdown())
	if shouldLog {
		logger.Infof(msg)
	}
	return nextWake
}

func LoadConfigStatsModule(config ModuleConfig) interface{} {
	_ = config.Printer().AddObject("system_stats", NewPrinterSysStatsModule(config))
	return NewPrinterStatsModule(config)
}
