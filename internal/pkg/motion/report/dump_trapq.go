package report

type DumpTrapQController struct {
	core    *TrapQDump
	apiDump *APIDumpHelper
}

func NewDumpTrapQController(core *TrapQDump, apiDump *APIDumpHelper) *DumpTrapQController {
	return &DumpTrapQController{core: core, apiDump: apiDump}
}

func (self *DumpTrapQController) Moves(startTime, endTime float64) []TrapQMove {
	return self.core.Moves(startTime, endTime)
}

func (self *DumpTrapQController) LogMessage(data []TrapQMove) string {
	return self.core.LogMessage(data)
}

func (self *DumpTrapQController) PositionAt(printTime float64) ([]float64, float64) {
	return self.core.PositionAt(printTime)
}

func (self *DumpTrapQController) APIUpdate(eventtime float64) map[string]interface{} {
	_ = eventtime
	return self.core.APIUpdate()
}

func (self *DumpTrapQController) AddClient(client APIDumpClient, template map[string]interface{}) {
	self.apiDump.AddClient(client, template)
}

func (self *DumpTrapQController) Header() map[string]interface{} {
	return map[string]interface{}{"header": []string{"time", "duration", "start_velocity", "acceleration", "start_position", "direction"}}
}
