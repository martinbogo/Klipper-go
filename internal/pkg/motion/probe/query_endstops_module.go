package probe

import (
	printerpkg "goklipper/internal/pkg/printer"
)

const cmdQueryEndstopsHelp = "Report on the status of each endstop"

type QueryEndstopRuntime interface {
	Query_endstop(printTime float64) int
}

type queryEndstopAdapter struct {
	endstop QueryEndstopRuntime
}

func (self *queryEndstopAdapter) QueryEndstop(printTime float64) int {
	return self.endstop.Query_endstop(printTime)
}

type queryEndstopsToolhead interface {
	Get_last_move_time() float64
}

type QueryEndstopsModule struct {
	printer printerpkg.ModulePrinter
	gcode   printerpkg.GCodeRuntime
	core    *QueryEndstops
}

func LoadConfigQueryEndstops(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	self := &QueryEndstopsModule{
		printer: printer,
		gcode:   printer.GCode(),
		core:    NewQueryEndstops(),
	}
	_ = printer.Webhooks().RegisterEndpoint("query_endstops/status", self.handleStatusRequest)
	self.gcode.RegisterCommand("QUERY_ENDSTOPS", self.cmdQueryEndstops, false, cmdQueryEndstopsHelp)
	self.gcode.RegisterCommand("M119", self.cmdQueryEndstops, false, "")
	return self
}

func (self *QueryEndstopsModule) RegisterEndstop(endstop QueryEndstopRuntime, name string) {
	self.core.RegisterEndstop(&queryEndstopAdapter{endstop: endstop}, name)
}

func (self *QueryEndstopsModule) Register_endstop(endstop interface{}, name string) {
	queryEndstop, ok := endstop.(QueryEndstopRuntime)
	if !ok {
		panic("unsupported endstop type")
	}
	self.RegisterEndstop(queryEndstop, name)
}

func (self *QueryEndstopsModule) Get_status(eventtime interface{}) map[string]map[string]interface{} {
	return self.core.Get_status(eventtime)
}

func (self *QueryEndstopsModule) lookupToolhead() queryEndstopsToolhead {
	return self.printer.LookupObject("toolhead", nil).(queryEndstopsToolhead)
}

func (self *QueryEndstopsModule) queryStatuses() map[string]int {
	printTime := self.lookupToolhead().Get_last_move_time()
	return self.core.Query(printTime)
}

func (self *QueryEndstopsModule) handleStatusRequest() (interface{}, error) {
	mutex := self.gcode.Mutex()
	mutex.Lock()
	defer mutex.Unlock()
	return self.core.StatusTextMap(self.queryStatuses()), nil
}

func (self *QueryEndstopsModule) cmdQueryEndstops(gcmd printerpkg.Command) error {
	gcmd.RespondRaw(self.core.StatusMessage(self.queryStatuses()))
	return nil
}