package fan

import (
	"math"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeFanQueueMCU struct {
	flushCallbacks []func(float64, int64)
	estimatedCalls []float64
	estimatedValue float64
}

func (self *fakeFanQueueMCU) RegisterFlushCallback(callback func(float64, int64)) {
	self.flushCallbacks = append(self.flushCallbacks, callback)
}

func (self *fakeFanQueueMCU) EstimatedPrintTime(eventtime float64) float64 {
	self.estimatedCalls = append(self.estimatedCalls, eventtime)
	return self.estimatedValue
}

type fakeFanPWMPin struct {
	mcu          interface{}
	maxDurations []float64
	cycleCalls   []struct {
		cycleTime   float64
		hardwarePWM bool
	}
	startCalls [][2]float64
	setCalls   [][2]float64
}

func (self *fakeFanPWMPin) SetupMaxDuration(maxDuration float64) {
	self.maxDurations = append(self.maxDurations, maxDuration)
}

func (self *fakeFanPWMPin) SetupCycleTime(cycleTime float64, hardwarePWM bool) {
	self.cycleCalls = append(self.cycleCalls, struct {
		cycleTime   float64
		hardwarePWM bool
	}{cycleTime: cycleTime, hardwarePWM: hardwarePWM})
}

func (self *fakeFanPWMPin) SetupStartValue(startValue float64, shutdownValue float64) {
	self.startCalls = append(self.startCalls, [2]float64{startValue, shutdownValue})
}

func (self *fakeFanPWMPin) SetPWM(printTime float64, value float64) {
	self.setCalls = append(self.setCalls, [2]float64{printTime, value})
}

func (self *fakeFanPWMPin) MCU() interface{} {
	return self.mcu
}

type fakeFanPins struct {
	pwm         interface{}
	pwmPins     []string
	digitalPins []string
}

func (self *fakeFanPins) SetupPWM(pin string) interface{} {
	self.pwmPins = append(self.pwmPins, pin)
	return self.pwm
}

func (self *fakeFanPins) SetupDigitalOut(pin string) printerpkg.DigitalOutPin {
	self.digitalPins = append(self.digitalPins, pin)
	return nil
}

type fakeFanToolhead struct {
	nextPrintTime float64
	callbacks     []func(float64)
	notedTimes    []float64
	notedFlags    []bool
}

func (self *fakeFanToolhead) RegisterLookaheadCallback(callback func(float64)) {
	self.callbacks = append(self.callbacks, callback)
	callback(self.nextPrintTime)
}

func (self *fakeFanToolhead) NoteMCUMovequeueActivity(mqTime float64, setStepGenTime bool) {
	self.notedTimes = append(self.notedTimes, mqTime)
	self.notedFlags = append(self.notedFlags, setStepGenTime)
}

type fakeFanGCode struct {
	handlers map[string]func(printerpkg.Command) error
}

func (self *fakeFanGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.handlers == nil {
		self.handlers = map[string]func(printerpkg.Command) error{}
	}
	self.handlers[cmd] = handler
}

func (self *fakeFanGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakeFanGCode) RunScriptFromCommand(script string) {}
func (self *fakeFanGCode) RunScript(script string)            {}
func (self *fakeFanGCode) IsBusy() bool                       { return false }
func (self *fakeFanGCode) Mutex() printerpkg.Mutex            { return nil }
func (self *fakeFanGCode) RespondInfo(msg string, log bool)   {}
func (self *fakeFanGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeFanWebhookRegistry struct {
	requestHandlers map[string]func(printerpkg.WebhookRequest) (interface{}, error)
}

func (self *fakeFanWebhookRegistry) RegisterEndpoint(path string, handler func() (interface{}, error)) error {
	return nil
}

func (self *fakeFanWebhookRegistry) RegisterEndpointWithRequest(path string, handler func(printerpkg.WebhookRequest) (interface{}, error)) error {
	if self.requestHandlers == nil {
		self.requestHandlers = map[string]func(printerpkg.WebhookRequest) (interface{}, error){}
	}
	self.requestHandlers[path] = handler
	return nil
}

type fakeFanReactor struct {
	monotonic float64
}

func (self *fakeFanReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return nil
}

func (self *fakeFanReactor) Monotonic() float64 {
	return self.monotonic
}

type fakeFanPrinter struct {
	lookup        map[string]interface{}
	reactor       printerpkg.ModuleReactor
	gcode         printerpkg.GCodeRuntime
	webhooks      printerpkg.WebhookRegistry
	eventHandlers map[string]func([]interface{}) error
}

func (self *fakeFanPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFanPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeFanPrinter) SendEvent(event string, params []interface{}) {}
func (self *fakeFanPrinter) CurrentExtruderName() string                  { return "extruder" }
func (self *fakeFanPrinter) AddObject(name string, obj interface{}) error { return nil }
func (self *fakeFanPrinter) LookupObjects(module string) []interface{}    { return nil }
func (self *fakeFanPrinter) HasStartArg(name string) bool                 { return false }
func (self *fakeFanPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}
func (self *fakeFanPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }
func (self *fakeFanPrinter) LookupMCU(name string) printerpkg.MCURuntime              { return nil }
func (self *fakeFanPrinter) InvokeShutdown(msg string)                                {}
func (self *fakeFanPrinter) IsShutdown() bool                                         { return false }
func (self *fakeFanPrinter) Reactor() printerpkg.ModuleReactor                        { return self.reactor }
func (self *fakeFanPrinter) StepperEnable() printerpkg.StepperEnableRuntime           { return nil }
func (self *fakeFanPrinter) GCode() printerpkg.GCodeRuntime                           { return self.gcode }
func (self *fakeFanPrinter) GCodeMove() printerpkg.MoveTransformController            { return nil }
func (self *fakeFanPrinter) Webhooks() printerpkg.WebhookRegistry                     { return self.webhooks }

type fakeFanConfig struct {
	printer printerpkg.ModulePrinter
	name    string
	strings map[string]string
	floats  map[string]float64
	bools   map[string]bool
}

func (self *fakeFanConfig) Name() string { return self.name }

func (self *fakeFanConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFanConfig) Bool(option string, defaultValue bool) bool {
	if value, ok := self.bools[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFanConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFanConfig) OptionalFloat(option string) *float64  { return nil }
func (self *fakeFanConfig) LoadObject(section string) interface{} { return nil }
func (self *fakeFanConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}
func (self *fakeFanConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}
func (self *fakeFanConfig) Printer() printerpkg.ModulePrinter { return self.printer }

type fakeFanCommand struct {
	floats map[string]float64
}

func (self *fakeFanCommand) String(name string, defaultValue string) string { return defaultValue }

func (self *fakeFanCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFanCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeFanCommand) Parameters() map[string]string    { return nil }
func (self *fakeFanCommand) RespondInfo(msg string, log bool) {}
func (self *fakeFanCommand) RespondRaw(msg string)            {}

type fakeFanWebhookRequest struct {
	floats map[string]float64
}

func (self *fakeFanWebhookRequest) String(name string, defaultValue string) string {
	return defaultValue
}

func (self *fakeFanWebhookRequest) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeFanWebhookRequest) Int(name string, defaultValue int) int { return defaultValue }

func (self *fakeFanWebhookRequest) GetParams() map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range self.floats {
		m[k] = v
	}
	return m
}

func assertApprox(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("unexpected value: got %v want %v", got, want)
	}
}

func TestLoadConfigFanRegistersCommandsAndQueuesSpeedChanges(t *testing.T) {
	queueMCU := &fakeFanQueueMCU{estimatedValue: 11.0}
	pwmPin := &fakeFanPWMPin{mcu: queueMCU}
	pins := &fakeFanPins{pwm: pwmPin}
	toolhead := &fakeFanToolhead{nextPrintTime: 5.0}
	gcode := &fakeFanGCode{}
	webhooks := &fakeFanWebhookRegistry{}
	printer := &fakeFanPrinter{
		lookup: map[string]interface{}{
			"pins":     pins,
			"toolhead": toolhead,
		},
		reactor:  &fakeFanReactor{monotonic: 2.0},
		gcode:    gcode,
		webhooks: webhooks,
	}
	module := LoadConfigFan(&fakeFanConfig{
		printer: printer,
		name:    "fan",
		strings: map[string]string{"pin": "PA0"},
		floats: map[string]float64{
			"kick_start_time": 0.0,
		},
		bools: map[string]bool{},
	}).(*PrinterFanModule)

	if len(pins.pwmPins) != 1 || pins.pwmPins[0] != "PA0" {
		t.Fatalf("unexpected pwm pin setup: %#v", pins.pwmPins)
	}
	if len(pwmPin.maxDurations) != 1 || pwmPin.maxDurations[0] != 0.0 {
		t.Fatalf("unexpected pwm max duration setup: %#v", pwmPin.maxDurations)
	}
	if len(pwmPin.cycleCalls) != 1 || pwmPin.cycleCalls[0].cycleTime != 0.010 || pwmPin.cycleCalls[0].hardwarePWM {
		t.Fatalf("unexpected cycle setup: %#v", pwmPin.cycleCalls)
	}
	if len(pwmPin.startCalls) != 1 || pwmPin.startCalls[0] != [2]float64{0.0, 0.0} {
		t.Fatalf("unexpected start values: %#v", pwmPin.startCalls)
	}
	if gcode.handlers["M106"] == nil || gcode.handlers["M107"] == nil {
		t.Fatalf("expected M106 and M107 handlers to be registered")
	}
	if webhooks.requestHandlers["fan/set_fan"] == nil {
		t.Fatalf("expected fan/set_fan webhook registration")
	}
	if printer.eventHandlers["project:connect"] == nil || printer.eventHandlers["gcode:request_restart"] == nil {
		t.Fatalf("expected connect and restart handlers to be registered")
	}
	if len(queueMCU.flushCallbacks) != 1 {
		t.Fatalf("expected one flush callback, got %#v", queueMCU.flushCallbacks)
	}

	if err := printer.eventHandlers["project:connect"](nil); err != nil {
		t.Fatalf("connect handler returned error: %v", err)
	}
	if err := gcode.handlers["M106"](&fakeFanCommand{floats: map[string]float64{"S": 128.0}}); err != nil {
		t.Fatalf("M106 returned error: %v", err)
	}
	queueMCU.flushCallbacks[0](5.0, 0)
	if len(pwmPin.setCalls) != 1 {
		t.Fatalf("expected one pwm set call after M106, got %#v", pwmPin.setCalls)
	}
	assertApprox(t, pwmPin.setCalls[0][0], 5.0)
	assertApprox(t, pwmPin.setCalls[0][1], 128.0/255.0)

	status := module.Get_status(9.0)
	assertApprox(t, status["speed"], 128.0/255.0)
	assertApprox(t, status["rpm"], 0.0)

	if _, err := webhooks.requestHandlers["fan/set_fan"](&fakeFanWebhookRequest{floats: map[string]float64{"speed": 25.0}}); err != nil {
		t.Fatalf("fan webhook returned error: %v", err)
	}
	queueMCU.flushCallbacks[0](6.0, 0)
	if len(pwmPin.setCalls) != 2 {
		t.Fatalf("expected second pwm set call after webhook, got %#v", pwmPin.setCalls)
	}
	if pwmPin.setCalls[1][0] <= 5.0 {
		t.Fatalf("expected queued webhook change after first request time, got %#v", pwmPin.setCalls[1])
	}
	assertApprox(t, pwmPin.setCalls[1][1], 0.25)

	if err := printer.eventHandlers["gcode:request_restart"]([]interface{}{7.0}); err != nil {
		t.Fatalf("restart handler returned error: %v", err)
	}
	if len(pwmPin.setCalls) != 3 {
		t.Fatalf("expected third pwm set call after restart, got %#v", pwmPin.setCalls)
	}
	assertApprox(t, pwmPin.setCalls[2][0], 7.0)
	assertApprox(t, pwmPin.setCalls[2][1], 0.0)
	if len(toolhead.notedTimes) == 0 {
		t.Fatalf("expected toolhead activity notifications, got none")
	}
}
