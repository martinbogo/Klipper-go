package io

import printerpkg "goklipper/internal/pkg/printer"

type moduleDigitalPinAdapter struct {
	pin printerpkg.DigitalOutPin
}

func (self *moduleDigitalPinAdapter) Get_mcu() MCUEstimator {
	return &moduleEstimatorAdapter{estimator: self.pin.MCU()}
}

func (self *moduleDigitalPinAdapter) Set_digital(printTime float64, value int) {
	self.pin.SetDigital(printTime, value)
}

type moduleEstimatorAdapter struct {
	estimator printerpkg.PrintTimeEstimator
}

func (self *moduleEstimatorAdapter) Estimated_print_time(eventtime float64) float64 {
	return self.estimator.EstimatedPrintTime(eventtime)
}

type DigitalOutputModule struct {
	*DigitalOutput
}

func NewDigitalOutputModule(config printerpkg.ModuleConfig) *DigitalOutputModule {
	printer := config.Printer()
	pins := printer.LookupObject("pins", nil).(printerpkg.PinRegistry)
	pin := pins.SetupDigitalOut(config.String("pin", "", true))
	pin.SetupMaxDuration(0.0)
	self := &DigitalOutputModule{
		DigitalOutput: NewDigitalOutput(
			&moduleDigitalPinAdapter{pin: pin},
			printer.Reactor(),
			int(config.Float("value", 0.0)),
			int(config.Float("shutdown_value", 0.0)),
		),
	}
	printer.RegisterEventHandler("project:ready", self.handleReady)
	printer.RegisterEventHandler("project:shutdown", self.handleShutdown)
	_ = printer.Webhooks().RegisterEndpointWithRequest("power/set_power_pin", self.setPowerPin)
	return self
}

func (self *DigitalOutputModule) Get_status(eventTime float64) map[string]int {
	return self.DigitalOutput.Get_status(eventTime)
}

func (self *DigitalOutputModule) setPowerPin(request printerpkg.WebhookRequest) (interface{}, error) {
	self.DigitalOutput.SetPowerPin(request.Int("S", 0))
	return nil, nil
}

func (self *DigitalOutputModule) handleReady([]interface{}) error {
	return self.DigitalOutput.HandleReady()
}

func (self *DigitalOutputModule) handleShutdown([]interface{}) error {
	return self.DigitalOutput.HandleShutdown()
}

func LoadConfigPrefixDigitalOut(config printerpkg.ModuleConfig) interface{} {
	return NewDigitalOutputModule(config)
}
