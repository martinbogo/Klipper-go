package io

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeButtonsCommand struct {
	sends [][3]interface{}
}

func (self *fakeButtonsCommand) Send(data interface{}, minclock int64, reqclock int64) {
	args, ok := data.([]int64)
	if !ok {
		panic(fmt.Sprintf("unexpected command payload type %T", data))
	}
	copyArgs := append([]int64{}, args...)
	self.sends = append(self.sends, [3]interface{}{copyArgs, minclock, reqclock})
}

type fakeButtonsReactor struct {
	monotonic      float64
	asyncCallbacks int
	asyncTimes     []float64
}

func (self *fakeButtonsReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return nil
}

func (self *fakeButtonsReactor) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeButtonsReactor) RegisterAsyncCallback(callback func(float64)) {
	self.asyncCallbacks++
	self.asyncTimes = append(self.asyncTimes, self.monotonic)
	callback(self.monotonic)
}

type fakeButtonsMCU struct {
	nextOID          int
	callbacks        []func()
	configCmds       []string
	querySlot        int64
	secondsToClockIn []float64
	responseCallback func(map[string]interface{}) error
	responseMessage  string
	responseOID      interface{}
	lookupFormats    []string
	lookupQueues     []interface{}
	commandQueue     interface{}
	commands         map[string]*fakeButtonsCommand
}

func (self *fakeButtonsMCU) CreateOID() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeButtonsMCU) RegisterConfigCallback(cb func()) {
	self.callbacks = append(self.callbacks, cb)
}

func (self *fakeButtonsMCU) AddConfigCmd(cmd string, isInit bool, onRestart bool) {
	self.configCmds = append(self.configCmds, cmd)
}

func (self *fakeButtonsMCU) GetQuerySlot(oid int) int64 {
	return self.querySlot
}

func (self *fakeButtonsMCU) SecondsToClock(time float64) int64 {
	self.secondsToClockIn = append(self.secondsToClockIn, time)
	return int64(time * 1000)
}

func (self *fakeButtonsMCU) RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{}) {
	self.responseCallback = cb
	self.responseMessage = msg
	self.responseOID = oid
}

func (self *fakeButtonsMCU) ClockToPrintTime(clock int64) float64 { return float64(clock) / 1000.0 }

func (self *fakeButtonsMCU) Clock32ToClock64(clock32 int64) int64 { return clock32 }

func (self *fakeButtonsMCU) AllocCommandQueue() interface{} {
	if self.commandQueue == nil {
		self.commandQueue = "buttons-queue"
	}
	return self.commandQueue
}

func (self *fakeButtonsMCU) LookupCommand(msgformat string, cq interface{}) (interface{}, error) {
	self.lookupFormats = append(self.lookupFormats, msgformat)
	self.lookupQueues = append(self.lookupQueues, cq)
	if self.commands == nil {
		self.commands = map[string]*fakeButtonsCommand{}
	}
	command, ok := self.commands[msgformat]
	if !ok {
		command = &fakeButtonsCommand{}
		self.commands[msgformat] = command
	}
	return command, nil
}

type fakeButtonsADC struct {
	reportTimes []float64
	callback    func(float64, float64)
	minmaxCalls [][5]float64
}

func (self *fakeButtonsADC) SetupCallback(reportTime float64, callback func(float64, float64)) {
	self.reportTimes = append(self.reportTimes, reportTime)
	self.callback = callback
}

func (self *fakeButtonsADC) SetupMinMax(sampleTime float64, sampleCount int, minval float64, maxval float64, rangeCheckCount int) {
	self.minmaxCalls = append(self.minmaxCalls, [5]float64{sampleTime, float64(sampleCount), minval, maxval, float64(rangeCheckCount)})
}

func (self *fakeButtonsADC) GetLastValue() [2]float64 {
	return [2]float64{}
}

type fakeButtonsPinRegistry struct {
	lookupCalls  []string
	lookup       map[string]map[string]interface{}
	adcPins      map[string]*fakeButtonsADC
	adcSetupPins []string
}

func (self *fakeButtonsPinRegistry) SetupDigitalOut(pin string) printerpkg.DigitalOutPin {
	return nil
}

func (self *fakeButtonsPinRegistry) SetupADC(pin string) printerpkg.ADCPin {
	self.adcSetupPins = append(self.adcSetupPins, pin)
	if self.adcPins == nil {
		self.adcPins = map[string]*fakeButtonsADC{}
	}
	adc, ok := self.adcPins[pin]
	if !ok {
		adc = &fakeButtonsADC{}
		self.adcPins[pin] = adc
	}
	return adc
}

func (self *fakeButtonsPinRegistry) LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{} {
	self.lookupCalls = append(self.lookupCalls, pinDesc)
	if value, ok := self.lookup[pinDesc]; ok {
		result := map[string]interface{}{}
		for key, item := range value {
			result[key] = item
		}
		return result
	}
	return map[string]interface{}{"chip_name": "mcu", "pin": pinDesc, "invert": 0, "pullup": 0}
}

type fakeButtonsADCQuery struct {
	names []string
	adcs  []printerpkg.ADCQueryReader
}

func (self *fakeButtonsADCQuery) RegisterADC(name string, adc printerpkg.ADCQueryReader) {
	self.names = append(self.names, name)
	self.adcs = append(self.adcs, adc)
}

type fakeButtonsPrinter struct {
	lookup  map[string]interface{}
	mcus    map[string]printerpkg.MCURuntime
	reactor printerpkg.ModuleReactor
}

func (self *fakeButtonsPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeButtonsPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}
func (self *fakeButtonsPrinter) SendEvent(event string, params []interface{})             {}
func (self *fakeButtonsPrinter) CurrentExtruderName() string                              { return "extruder" }
func (self *fakeButtonsPrinter) AddObject(name string, obj interface{}) error             { return nil }
func (self *fakeButtonsPrinter) LookupObjects(module string) []interface{}                { return nil }
func (self *fakeButtonsPrinter) HasStartArg(name string) bool                             { return false }
func (self *fakeButtonsPrinter) LookupHeater(name string) printerpkg.HeaterRuntime        { return nil }
func (self *fakeButtonsPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }
func (self *fakeButtonsPrinter) InvokeShutdown(msg string)                                {}
func (self *fakeButtonsPrinter) IsShutdown() bool                                         { return false }
func (self *fakeButtonsPrinter) StepperEnable() printerpkg.StepperEnableRuntime           { return nil }
func (self *fakeButtonsPrinter) GCode() printerpkg.GCodeRuntime                           { return nil }
func (self *fakeButtonsPrinter) GCodeMove() printerpkg.MoveTransformController            { return nil }
func (self *fakeButtonsPrinter) Webhooks() printerpkg.WebhookRegistry                     { return nil }

func (self *fakeButtonsPrinter) LookupMCU(name string) printerpkg.MCURuntime {
	if value, ok := self.mcus[name]; ok {
		return value
	}
	return nil
}

func (self *fakeButtonsPrinter) Reactor() printerpkg.ModuleReactor {
	return self.reactor
}

type fakeButtonsConfig struct {
	printer       printerpkg.ModulePrinter
	loadedObjects []string
	objects       map[string]interface{}
}

func (self *fakeButtonsConfig) Name() string { return "buttons" }
func (self *fakeButtonsConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}
func (self *fakeButtonsConfig) Bool(option string, defaultValue bool) bool { return defaultValue }
func (self *fakeButtonsConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}
func (self *fakeButtonsConfig) OptionalFloat(option string) *float64 { return nil }
func (self *fakeButtonsConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	return self.objects[section]
}
func (self *fakeButtonsConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}
func (self *fakeButtonsConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}
func (self *fakeButtonsConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigButtonsRegistersDigitalButtonsAndBuildsMCUCommands(t *testing.T) {
	queryADC := &fakeButtonsADCQuery{}
	pins := &fakeButtonsPinRegistry{lookup: map[string]map[string]interface{}{
		"PA1": {"chip_name": "mcu", "pin": "gpio1", "invert": 1, "pullup": 1},
		"PA2": {"chip_name": "mcu", "pin": "gpio2", "invert": 0, "pullup": 1},
	}}
	mcu := &fakeButtonsMCU{nextOID: 7, querySlot: 120}
	reactor := &fakeButtonsReactor{monotonic: 2.0}
	printer := &fakeButtonsPrinter{
		lookup:  map[string]interface{}{"pins": pins},
		mcus:    map[string]printerpkg.MCURuntime{"mcu": mcu},
		reactor: reactor,
	}
	config := &fakeButtonsConfig{printer: printer, objects: map[string]interface{}{"query_adc": queryADC}}
	module := LoadConfigButtons(config).(*PrinterButtonsModule)
	if module == nil {
		t.Fatalf("expected buttons module instance")
	}
	if !reflect.DeepEqual(config.loadedObjects, []string{"query_adc"}) {
		t.Fatalf("unexpected load object calls: %#v", config.loadedObjects)
	}

	callbackStates := []int{}
	callbackTimes := []float64{}
	module.Register_buttons([]string{"PA1", "PA2"}, func(eventtime float64, state int) {
		callbackTimes = append(callbackTimes, eventtime)
		callbackStates = append(callbackStates, state)
	})
	if len(mcu.callbacks) != 1 {
		t.Fatalf("expected one mcu config callback, got %d", len(mcu.callbacks))
	}
	mcu.callbacks[0]()
	expectedConfig := []string{
		"config_buttons oid=7 button_count=2",
		"buttons_add oid=7 pos=0 pin=gpio1 pull_up=1",
		"buttons_add oid=7 pos=1 pin=gpio2 pull_up=1",
		"buttons_query oid=7 clock=120 rest_ticks=2 retransmit_count=50 invert=1",
	}
	if fmt.Sprint(mcu.configCmds) != fmt.Sprint(expectedConfig) {
		t.Fatalf("unexpected config commands: %#v", mcu.configCmds)
	}
	if fmt.Sprint(mcu.lookupFormats) != fmt.Sprint([]string{"buttons_ack oid=%c count=%c"}) {
		t.Fatalf("unexpected command lookups: %#v", mcu.lookupFormats)
	}
	if len(mcu.lookupQueues) != 1 || mcu.lookupQueues[0] != "buttons-queue" {
		t.Fatalf("unexpected command queue: %#v", mcu.lookupQueues)
	}
	if mcu.responseMessage != "buttons_state" || mcu.responseOID != 7 {
		t.Fatalf("unexpected response registration: %q %#v", mcu.responseMessage, mcu.responseOID)
	}

	if err := mcu.responseCallback(map[string]interface{}{"ack_count": int64(0), "state": []int{0}}); err != nil {
		t.Fatalf("buttons_state handler returned error: %v", err)
	}
	ackCommand := mcu.commands["buttons_ack oid=%c count=%c"]
	if ackCommand == nil || len(ackCommand.sends) != 1 {
		t.Fatalf("expected one ack command send, got %#v", ackCommand)
	}
	if got := ackCommand.sends[0][0].([]int64); !reflect.DeepEqual(got, []int64{7, 1}) {
		t.Fatalf("unexpected ack send args: %#v", got)
	}
	if !reflect.DeepEqual(callbackTimes, []float64{2.0}) || !reflect.DeepEqual(callbackStates, []int{1}) {
		t.Fatalf("unexpected button callbacks: times=%#v states=%#v", callbackTimes, callbackStates)
	}
	if reactor.asyncCallbacks != 1 {
		t.Fatalf("expected one async callback, got %d", reactor.asyncCallbacks)
	}
}

func TestButtonsRejectPinsOnDifferentMCUs(t *testing.T) {
	queryADC := &fakeButtonsADCQuery{}
	pins := &fakeButtonsPinRegistry{lookup: map[string]map[string]interface{}{
		"PA1": {"chip_name": "mcu0", "pin": "gpio1", "invert": 0, "pullup": 1},
		"PB1": {"chip_name": "mcu1", "pin": "gpio2", "invert": 0, "pullup": 1},
	}}
	printer := &fakeButtonsPrinter{
		lookup:  map[string]interface{}{"pins": pins},
		mcus:    map[string]printerpkg.MCURuntime{"mcu0": &fakeButtonsMCU{}, "mcu1": &fakeButtonsMCU{}},
		reactor: &fakeButtonsReactor{monotonic: 1.0},
	}
	module := LoadConfigButtons(&fakeButtonsConfig{printer: printer, objects: map[string]interface{}{"query_adc": queryADC}}).(*PrinterButtonsModule)
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic for cross-mcu button pins")
		}
		if !strings.Contains(fmt.Sprint(recovered), "same mcu") {
			t.Fatalf("unexpected panic: %v", recovered)
		}
	}()
	module.Register_buttons([]string{"PA1", "PB1"}, func(float64, int) {})
}

func TestButtonsRegisterADCAndDebounceCallbacks(t *testing.T) {
	queryADC := &fakeButtonsADCQuery{}
	pins := &fakeButtonsPinRegistry{}
	reactor := &fakeButtonsReactor{monotonic: 4.0}
	printer := &fakeButtonsPrinter{
		lookup:  map[string]interface{}{"pins": pins},
		mcus:    map[string]printerpkg.MCURuntime{},
		reactor: reactor,
	}
	module := LoadConfigButtons(&fakeButtonsConfig{printer: printer, objects: map[string]interface{}{"query_adc": queryADC}}).(*PrinterButtonsModule)
	events := []bool{}
	times := []float64{}
	module.Register_adc_button("ADC0", 900.0, 1100.0, 4700.0, func(eventtime float64, state bool) {
		times = append(times, eventtime)
		events = append(events, state)
	})

	adc := pins.adcPins["ADC0"]
	if adc == nil {
		t.Fatalf("expected adc pin setup for ADC0")
	}
	if !reflect.DeepEqual(pins.adcSetupPins, []string{"ADC0"}) {
		t.Fatalf("unexpected adc setup pins: %#v", pins.adcSetupPins)
	}
	if len(adc.minmaxCalls) != 1 || adc.minmaxCalls[0] != [5]float64{0.001, 6, 0, 1, 0} {
		t.Fatalf("unexpected adc minmax calls: %#v", adc.minmaxCalls)
	}
	if !reflect.DeepEqual(adc.reportTimes, []float64{0.015}) {
		t.Fatalf("unexpected adc report times: %#v", adc.reportTimes)
	}
	if !reflect.DeepEqual(queryADC.names, []string{"adc_button:ADC0"}) {
		t.Fatalf("unexpected query_adc registrations: %#v", queryADC.names)
	}

	pressed := 1000.0 / (4700.0 + 1000.0)
	adc.callback(0.0, pressed)
	adc.callback(0.03, pressed)
	adc.callback(0.06, 0.99)
	adc.callback(0.09, 0.99)
	if !reflect.DeepEqual(events, []bool{true, false}) {
		t.Fatalf("unexpected adc button events: %#v", events)
	}
	if !reflect.DeepEqual(times, []float64{4.0, 4.0}) {
		t.Fatalf("unexpected adc button event times: %#v", times)
	}
}
