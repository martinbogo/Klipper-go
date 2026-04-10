package fan

import (
	"fmt"
	"math"

	iopkg "goklipper/internal/pkg/io"
	printerpkg "goklipper/internal/pkg/printer"
)

type fanPWMPin interface {
	SetupMaxDuration(maxDuration float64)
	SetupCycleTime(cycleTime float64, hardwarePWM bool)
	SetupStartValue(startValue float64, shutdownValue float64)
	SetPWM(printTime float64, value float64)
	MCU() interface{}
}

type fanPinRegistry interface {
	SetupPWM(pin string) interface{}
	SetupDigitalOut(pin string) printerpkg.DigitalOutPin
}

type fanPinLookup interface {
	LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{}
}

type fanQueueMCU interface {
	EstimatedPrintTime(eventtime float64) float64
	RegisterFlushCallback(callback func(float64, int64))
}

type fanToolhead interface {
	RegisterLookaheadCallback(callback func(float64))
	NoteMCUMovequeueActivity(mqTime float64, setStepGenTime bool)
}

type pwmOutputAdapter struct {
	pin fanPWMPin
}

func (self *pwmOutputAdapter) Setup_max_duration(maxDuration float64) {
	self.pin.SetupMaxDuration(maxDuration)
}

func (self *pwmOutputAdapter) Setup_cycle_time(cycleTime float64, hardwarePWM bool) {
	self.pin.SetupCycleTime(cycleTime, hardwarePWM)
}

func (self *pwmOutputAdapter) Setup_start_value(startValue float64, shutdownValue float64) {
	self.pin.SetupStartValue(startValue, shutdownValue)
}

func (self *pwmOutputAdapter) Set_pwm(printTime float64, value float64) {
	self.pin.SetPWM(printTime, value)
}

type digitalOutputAdapter struct {
	pin printerpkg.DigitalOutPin
}

func (self *digitalOutputAdapter) Setup_max_duration(maxDuration float64) {
	self.pin.SetupMaxDuration(maxDuration)
}

func (self *digitalOutputAdapter) Set_digital(printTime float64, value int) {
	self.pin.SetDigital(printTime, value)
}

type GCodeRequestQueue struct {
	printer  printerpkg.ModulePrinter
	mcu      fanQueueMCU
	core     *iopkg.RequestQueue
	toolhead fanToolhead
}

func NewGCodeRequestQueue(printer printerpkg.ModulePrinter, mcu fanQueueMCU,
	callback func(float64, float64) (string, float64)) *GCodeRequestQueue {
	self := &GCodeRequestQueue{
		printer:  printer,
		mcu:      mcu,
		core:     iopkg.NewRequestQueue(callback),
		toolhead: nil,
	}
	self.mcu.RegisterFlushCallback(self.flushNotification)
	self.printer.RegisterEventHandler("project:connect", self.handleConnect)
	return self
}

func (self *GCodeRequestQueue) handleConnect([]interface{}) error {
	self.ensureToolhead()
	return nil
}

func (self *GCodeRequestQueue) ensureToolhead() fanToolhead {
	if self.toolhead != nil {
		return self.toolhead
	}
	toolheadObj := self.printer.LookupObject("toolhead", nil)
	if toolheadObj == nil {
		return nil
	}
	toolhead, ok := toolheadObj.(fanToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement fanToolhead: %T", toolheadObj))
	}
	self.toolhead = toolhead
	return self.toolhead
}

func (self *GCodeRequestQueue) noteActivity(nextTime float64) {
	toolhead := self.ensureToolhead()
	if toolhead != nil {
		toolhead.NoteMCUMovequeueActivity(nextTime, true)
	}
}

func (self *GCodeRequestQueue) flushNotification(printTime float64, clock int64) {
	self.core.Flush(printTime, self.noteActivity)
}

func (self *GCodeRequestQueue) queueRequest(printTime float64, value float64) {
	self.core.QueueRequest(printTime, value, self.noteActivity)
}

func (self *GCodeRequestQueue) QueueGCodeRequest(value float64) {
	toolhead := self.ensureToolhead()
	if toolhead == nil {
		panic("toolhead not available for fan gcode request")
	}
	toolhead.RegisterLookaheadCallback(func(pt float64) {
		self.queueRequest(pt, value)
	})
}

func (self *GCodeRequestQueue) SendAsyncRequest(value float64, printTime interface{}) {
	requestTime := 0.0
	switch typed := printTime.(type) {
	case nil:
		systime := self.printer.Reactor().Monotonic()
		requestTime = self.mcu.EstimatedPrintTime(systime + 0.01)
	case float64:
		requestTime = typed
	default:
		panic(fmt.Sprintf("unsupported fan print time type: %T", printTime))
	}
	self.core.SendAsyncRequest(value, requestTime)
}

type PrinterFanModule struct {
	core *Fan
}

func LoadConfigFan(config printerpkg.ModuleConfig) interface{} {
	return NewPrinterFanModule(config)
}

func newConfiguredFan(config printerpkg.ModuleConfig) *Fan {
	printer := config.Printer()
	pinsObj := printer.LookupObject("pins", nil)
	pins, ok := pinsObj.(fanPinRegistry)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement fanPinRegistry: %T", pinsObj))
	}

	maxPower := config.Float("max_power", 1.0)
	kickStartTime := config.Float("kick_start_time", 0.1)
	offBelow := config.Float("off_below", 0.0)
	cycleTime := config.Float("cycle_time", 0.010)
	hardwarePWM := config.Bool("hardware_pwm", false)
	shutdownSpeed := config.Float("shutdown_speed", 0.0)

	pwmObj := pins.SetupPWM(config.String("pin", "", true))
	pwmPin, ok := pwmObj.(fanPWMPin)
	if !ok {
		panic(fmt.Sprintf("fan pin does not implement fanPWMPin: %T", pwmObj))
	}
	pwmPin.SetupMaxDuration(0.0)
	pwmPin.SetupCycleTime(cycleTime, hardwarePWM)
	shutdownPower := math.Max(0.0, math.Min(maxPower, shutdownSpeed))
	pwmPin.SetupStartValue(0.0, shutdownPower)

	var enablePin DigitalOutput
	enablePinName := config.String("enable_pin", "", true)
	if enablePinName != "" {
		pin := pins.SetupDigitalOut(enablePinName)
		pin.SetupMaxDuration(0.0)
		enablePin = &digitalOutputAdapter{pin: pin}
	}

	queueMCUObj := pwmPin.MCU()
	queueMCU, ok := queueMCUObj.(fanQueueMCU)
	if !ok {
		panic(fmt.Sprintf("fan MCU does not implement fanQueueMCU: %T", queueMCUObj))
	}

	tachometer := NewModuleFanTachometer(config)
	var coreFan *Fan
	requestQueue := NewGCodeRequestQueue(printer, queueMCU,
		func(printTime float64, value float64) (string, float64) {
			return coreFan.ApplySpeed(printTime, value)
		})
	coreFan = NewFan(maxPower, kickStartTime, offBelow,
		&pwmOutputAdapter{pin: pwmPin}, enablePin, tachometer, requestQueue)
	return coreFan
}

func NewPrinterFanModule(config printerpkg.ModuleConfig) *PrinterFanModule {
	printer := config.Printer()
	self := &PrinterFanModule{core: newConfiguredFan(config)}
	printer.RegisterEventHandler("gcode:request_restart", self.handleRequestRestart)
	printer.GCode().RegisterCommand("M106", self.cmdM106, false, "")
	printer.GCode().RegisterCommand("M107", self.cmdM107, false, "")
	_ = printer.Webhooks().RegisterEndpointWithRequest("fan/set_fan", self.setFan)
	return self
}

func NewModuleFanTachometer(config printerpkg.ModuleConfig) Tachometer {
	pin := config.String("tachometer_pin", "", true)
	if pin == "" {
		return nil
	}
	ppr := int(config.Float("tachometer_ppr", 2.0))
	if ppr < 1 {
		panic("tachometer_ppr must be at least 1")
	}
	pollTime := config.Float("tachometer_poll_interval", 0.0015)
	printer := config.Printer()
	pinsObj := printer.LookupObject("pins", nil)
	pinLookup, ok := pinsObj.(fanPinLookup)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement fanPinLookup: %T", pinsObj))
	}
	return NewFanTachometer(NewFrequencyCounter(printer, pinLookup, pin, 1.0, pollTime), ppr)
}

func (self *PrinterFanModule) Get_status(eventtime float64) map[string]float64 {
	return self.core.Get_status(eventtime)
}

func (self *PrinterFanModule) cmdM106(gcmd printerpkg.Command) error {
	self.core.SetSpeedFromCommand(gcmd.Float("S", 255.0) / 255.0)
	return nil
}

func (self *PrinterFanModule) cmdM107(gcmd printerpkg.Command) error {
	self.core.SetSpeedFromCommand(0.0)
	return nil
}

func (self *PrinterFanModule) setFan(request printerpkg.WebhookRequest) (interface{}, error) {
	self.core.SetSpeedFromCommand(request.Float("speed", 0.0) / 100.0)
	return nil, nil
}

func (self *PrinterFanModule) handleRequestRestart(args []interface{}) error {
	if len(args) == 0 {
		panic("missing fan restart print time")
	}
	printTime, ok := args[0].(float64)
	if !ok {
		panic(fmt.Sprintf("unexpected fan restart print time type: %T", args[0]))
	}
	self.core.HandleRequestRestart(printTime)
	return nil
}
