package util

import printerpkg "goklipper/internal/pkg/printer"

type QueryADCModule struct {
	Printer printerpkg.ModulePrinter
	Adc     *ADCQueryRegistry
}

func NewQueryADCModule(config printerpkg.ModuleConfig) *QueryADCModule {
	self := &QueryADCModule{}
	self.Printer = config.Printer()
	self.Adc = NewADCQueryRegistry()
	self.Printer.GCode().RegisterCommand("QUERY_ADC", self.Cmd_QUERY_ADC,
		false, "Report the last value of an analog pin")
	return self
}

func (self *QueryADCModule) RegisterADC(name string, adc ADCReader) {
	self.Adc.RegisterADC(name, adc)
}

func (self *QueryADCModule) Cmd_QUERY_ADC(gcmd printerpkg.Command) error {
	name := gcmd.String("NAME", "")
	if name == "" {
		gcmd.RespondInfo(self.Adc.FormatAvailableMessage(), true)
		return nil
	}

	pullup := gcmd.Float("PULLUP", 0.)
	msg, ok := self.Adc.BuildValueMessage(name, pullup)
	if !ok {
		gcmd.RespondInfo(self.Adc.FormatAvailableMessage(), true)
		return nil
	}
	gcmd.RespondInfo(msg, true)
	return nil
}

func LoadConfigQueryADC(config printerpkg.ModuleConfig) interface{} {
	return NewQueryADCModule(config)
}
