package print

import (
	"fmt"

	printerpkg "goklipper/internal/pkg/printer"
)

type extrusionStatusSource interface {
	Get_status(eventtime float64) map[string]interface{}
}

const CmdSetPrintStatsInfoHelp = "Pass slicer info like layer act and total to klipper"

type PrintStatsModule struct {
	gcodeMove extrusionStatusSource
	reactor   printerpkg.ModuleReactor
	core      *Stats
}

func NewPrintStatsModule(printer printerpkg.ModulePrinter, reactor printerpkg.ModuleReactor, gcodeMove extrusionStatusSource) *PrintStatsModule {
	self := &PrintStatsModule{
		gcodeMove: gcodeMove,
		reactor:   reactor,
		core:      NewStats(),
	}
	printer.GCode().RegisterCommand("SET_PRINT_STATS_INFO", self.cmdSetPrintStatsInfo, false, CmdSetPrintStatsInfoHelp)
	return self
}

func LoadConfigPrintStats(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	gcodeMoveObj := config.LoadObject("gcode_move")
	gcodeMove, ok := gcodeMoveObj.(extrusionStatusSource)
	if !ok {
		panic(fmt.Sprintf("print_stats requires gcode_move extrusion status source: %T", gcodeMoveObj))
	}
	return NewPrintStatsModule(printer, printer.Reactor(), gcodeMove)
}

func (self *PrintStatsModule) extrusionStatus(eventtime float64) ExtrusionStatus {
	gcStatus := self.gcodeMove.Get_status(eventtime)
	return ExtrusionStatus{
		Position:      gcStatus["position"].([]float64)[3],
		ExtrudeFactor: gcStatus["extrude_factor"].(float64),
	}
}

func (self *PrintStatsModule) Filename() string {
	return self.core.Filename()
}

func (self *PrintStatsModule) Set_current_file(filename string) {
	self.core.SetCurrentFile(filename)
}

func (self *PrintStatsModule) Note_start() {
	curtime := self.reactor.Monotonic()
	self.core.NoteStart(curtime, self.extrusionStatus(curtime))
}

func (self *PrintStatsModule) Note_pause() {
	curtime := self.reactor.Monotonic()
	self.core.NotePause(curtime, self.extrusionStatus(curtime))
}

func (self *PrintStatsModule) Note_complete() {
	self.core.NoteComplete(self.reactor.Monotonic())
}

func (self *PrintStatsModule) Note_error(message string) {
	self.core.NoteError(self.reactor.Monotonic(), message)
}

func (self *PrintStatsModule) Note_cancel() {
	self.core.NoteCancel(self.reactor.Monotonic())
}

func (self *PrintStatsModule) Reset() {
	self.core.Reset()
}

func (self *PrintStatsModule) Get_status(eventtime float64) map[string]interface{} {
	return self.core.GetStatus(eventtime, self.extrusionStatus(eventtime))
}

func (self *PrintStatsModule) cmdSetPrintStatsInfo(gcmd printerpkg.Command) error {
	totalLayer := gcmd.Int("TOTAL_LAYER", self.core.InfoTotalLayer(), intPtr(0), nil)
	currentLayer := gcmd.Int("CURRENT_LAYER", self.core.InfoCurrentLayer(), intPtr(0), nil)
	self.core.SetInfo(totalLayer, currentLayer)
	return nil
}

func intPtr(v int) *int {
	return &v
}
