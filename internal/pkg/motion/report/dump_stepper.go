package report

type DumpStepperController struct {
	core    *StepperDump
	apiDump *APIDumpHelper
}

func NewDumpStepperController(core *StepperDump, apiDump *APIDumpHelper) *DumpStepperController {
	return &DumpStepperController{core: core, apiDump: apiDump}
}

func (self *DumpStepperController) StepQueue(startClock, endClock uint64) []StepQueueEntry {
	return self.core.StepQueue(startClock, endClock)
}

func (self *DumpStepperController) LogMessage(data []StepQueueEntry) string {
	return self.core.LogMessage(data)
}

func (self *DumpStepperController) APIUpdate(eventtime float64) map[string]interface{} {
	_ = eventtime
	return self.core.APIUpdate()
}

func (self *DumpStepperController) AddClient(client APIDumpClient, template map[string]interface{}) {
	self.apiDump.AddClient(client, template)
}

func (self *DumpStepperController) Header() map[string]interface{} {
	return map[string]interface{}{"header": []string{"interval", "count", "add"}}
}
