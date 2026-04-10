package printer

type ModuleConfig interface {
	Name() string
	String(option string, defaultValue string, noteValid bool) string
	Bool(option string, defaultValue bool) bool
	Float(option string, defaultValue float64) float64
	OptionalFloat(option string) *float64
	LoadObject(section string) interface{}
	LoadTemplate(module string, option string, defaultValue string) Template
	LoadRequiredTemplate(module string, option string) Template
	Printer() ModulePrinter
}

type ModulePrinter interface {
	LookupObject(name string, defaultValue interface{}) interface{}
	RegisterEventHandler(event string, callback func([]interface{}) error)
	SendEvent(event string, params []interface{})
	CurrentExtruderName() string
	AddObject(name string, obj interface{}) error
	LookupObjects(module string) []interface{}
	HasStartArg(name string) bool
	LookupHeater(name string) HeaterRuntime
	TemperatureSensors() TemperatureSensorRegistry
	LookupMCU(name string) MCURuntime
	InvokeShutdown(msg string)
	IsShutdown() bool
	Reactor() ModuleReactor
	StepperEnable() StepperEnableRuntime
	GCode() GCodeRuntime
	GCodeMove() MoveTransformController
	Webhooks() WebhookRegistry
}

type HeaterRuntime interface {
	GetTemperature(eventtime float64) (float64, float64)
}

type TemperatureSensor interface {
	SetupMinMax(minTemp float64, maxTemp float64)
	SetupCallback(callback func(float64, float64))
	GetReportTimeDelta() float64
}

type TemperatureSensorFactory func(ModuleConfig) TemperatureSensor

type TemperatureSensorRegistry interface {
	AddSensorFactory(sensorType string, factory TemperatureSensorFactory)
}

type ADCQueryReader interface {
	GetLastValue() [2]float64
}

type ADCQueryRegistry interface {
	RegisterADC(name string, adc ADCQueryReader)
}

type MCURuntime interface {
	CreateOID() int
	RegisterConfigCallback(cb func())
	AddConfigCmd(cmd string, isInit bool, onRestart bool)
	GetQuerySlot(oid int) int64
	SecondsToClock(time float64) int64
	RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{})
	ClockToPrintTime(clock int64) float64
	Clock32ToClock64(clock32 int64) int64
}

type ModuleReactor interface {
	RegisterTimer(callback func(float64) float64, waketime float64) TimerHandle
	Monotonic() float64
}

type TimerHandle interface {
	Update(waketime float64)
}

type Mutex interface {
	Lock()
	Unlock()
}

type Command interface {
	String(name string, defaultValue string) string
	Float(name string, defaultValue float64) float64
	Int(name string, defaultValue int, minValue *int, maxValue *int) int
	Parameters() map[string]string
	RespondInfo(msg string, log bool)
	RespondRaw(msg string)
}

type CommandRegistry interface {
	RegisterCommand(cmd string, handler func(Command) error, whenNotReady bool, desc string)
}

type GCodeRuntime interface {
	CommandRegistry
	IsTraditionalGCode(cmd string) bool
	RunScriptFromCommand(script string)
	RunScript(script string)
	IsBusy() bool
	Mutex() Mutex
	RespondInfo(msg string, log bool)
	ReplaceCommand(cmd string, handler func(Command) error, whenNotReady bool, desc string) func(Command) error
}

type MoveTransform interface {
	GetPosition() []float64
	Move([]float64, float64)
}

type MoveTransformController interface {
	SetMoveTransform(transform MoveTransform, force bool) MoveTransform
	GCodePositionZ() float64
	State() GCodeMoveState
	LinearMove(params map[string]string) error
	ResetLastPosition()
}

type GCodeMoveState struct {
	GCodePosition       []float64
	AbsoluteCoordinates bool
	AbsoluteExtrude     bool
}

type StepperEnableLine interface {
	MotorEnable(printTime float64)
	MotorDisable(printTime float64)
	IsMotorEnabled() bool
}

type StepperEnableRuntime interface {
	LookupEnable(name string) (StepperEnableLine, error)
	StepperNames() []string
}

type PrintTimeEstimator interface {
	EstimatedPrintTime(eventtime float64) float64
}

type DigitalOutPin interface {
	MCU() PrintTimeEstimator
	SetupMaxDuration(maxDuration float64)
	SetDigital(printTime float64, value int)
}

type ADCPin interface {
	ADCQueryReader
	SetupCallback(reportTime float64, callback func(float64, float64))
	SetupMinMax(sampleTime float64, sampleCount int, minval float64, maxval float64, rangeCheckCount int)
}

type PinRegistry interface {
	SetupDigitalOut(pin string) DigitalOutPin
	SetupADC(pin string) ADCPin
}

type WebhookRequest interface {
	String(name string, defaultValue string) string
	Float(name string, defaultValue float64) float64
	Int(name string, defaultValue int) int
}

type WebhookRegistry interface {
	RegisterEndpoint(path string, handler func() (interface{}, error)) error
	RegisterEndpointWithRequest(path string, handler func(WebhookRequest) (interface{}, error)) error
}

type Template interface {
	CreateContext(eventtime interface{}) map[string]interface{}
	Render(context map[string]interface{}) (string, error)
	RunGcodeFromCommand(context map[string]interface{}) error
}
