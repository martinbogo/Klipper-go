package printer

type HeaterRuntimeAdapter struct {
	source         interface{}
	getTemperature func(float64) (float64, float64)
}

var _ HeaterRuntime = (*HeaterRuntimeAdapter)(nil)

func NewHeaterRuntimeAdapter(source interface{}, getTemperature func(float64) (float64, float64)) *HeaterRuntimeAdapter {
	return &HeaterRuntimeAdapter{source: source, getTemperature: getTemperature}
}

func (self *HeaterRuntimeAdapter) Source() interface{} {
	return self.source
}

func (self *HeaterRuntimeAdapter) GetTemperature(eventtime float64) (float64, float64) {
	return self.getTemperature(eventtime)
}

type HeaterManagerAdapterOptions struct {
	LookupHeater   func(string) interface{}
	SetupHeater    func(ModuleConfig, string) interface{}
	SetTemperature func(interface{}, float64, bool) error
}

type HeaterManagerAdapter struct {
	opts HeaterManagerAdapterOptions
}

func NewHeaterManagerAdapter(opts HeaterManagerAdapterOptions) *HeaterManagerAdapter {
	return &HeaterManagerAdapter{opts: opts}
}

func (self *HeaterManagerAdapter) LookupHeater(name string) interface{} {
	return self.opts.LookupHeater(name)
}

func (self *HeaterManagerAdapter) SetupHeater(config ModuleConfig, gcodeID string) interface{} {
	return self.opts.SetupHeater(config, gcodeID)
}

func (self *HeaterManagerAdapter) Set_temperature(heater interface{}, temp float64, wait bool) error {
	return self.opts.SetTemperature(heater, temp, wait)
}

type TemperatureSensorRegistryAdapter struct {
	addSensorFactory func(string, TemperatureSensorFactory)
}

var _ TemperatureSensorRegistry = (*TemperatureSensorRegistryAdapter)(nil)

func NewTemperatureSensorRegistryAdapter(addSensorFactory func(string, TemperatureSensorFactory)) *TemperatureSensorRegistryAdapter {
	return &TemperatureSensorRegistryAdapter{addSensorFactory: addSensorFactory}
}

func (self *TemperatureSensorRegistryAdapter) AddSensorFactory(sensorType string, factory TemperatureSensorFactory) {
	self.addSensorFactory(sensorType, factory)
}

type PinRegistryRuntimeAdapterOptions struct {
	RegisterChip    func(string, interface{})
	SetupPWM        func(string) interface{}
	SetupDigitalOut func(string) DigitalOutPin
	SetupADC        func(string) ADCPin
	LookupPin       func(string, bool, bool, interface{}) map[string]interface{}
}

type PinRegistryRuntimeAdapter struct {
	opts PinRegistryRuntimeAdapterOptions
}

var _ PinRegistry = (*PinRegistryRuntimeAdapter)(nil)

func NewPinRegistryRuntimeAdapter(opts PinRegistryRuntimeAdapterOptions) *PinRegistryRuntimeAdapter {
	return &PinRegistryRuntimeAdapter{opts: opts}
}

func (self *PinRegistryRuntimeAdapter) RegisterChip(name string, chip interface{}) {
	self.opts.RegisterChip(name, chip)
}

func (self *PinRegistryRuntimeAdapter) SetupPWM(pin string) interface{} {
	return self.opts.SetupPWM(pin)
}

func (self *PinRegistryRuntimeAdapter) SetupDigitalOut(pin string) DigitalOutPin {
	return self.opts.SetupDigitalOut(pin)
}

func (self *PinRegistryRuntimeAdapter) SetupADC(pin string) ADCPin {
	return self.opts.SetupADC(pin)
}

func (self *PinRegistryRuntimeAdapter) LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{} {
	return self.opts.LookupPin(pinDesc, canInvert, canPullup, shareType)
}