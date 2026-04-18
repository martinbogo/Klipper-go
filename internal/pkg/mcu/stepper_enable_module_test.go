package mcu

import (
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeStepperEnableCommand struct {
	strings       map[string]string
	ints          map[string]int
	infoResponses []string
	rawResponses  []string
}

func (self *fakeStepperEnableCommand) String(name string, defaultValue string) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeStepperEnableCommand) Float(_ string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeStepperEnableCommand) Int(name string, defaultValue int, _ *int, _ *int) int {
	if value, ok := self.ints[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeStepperEnableCommand) Parameters() map[string]string {
	params := map[string]string{}
	for key, value := range self.strings {
		params[key] = value
	}
	return params
}

func (self *fakeStepperEnableCommand) RespondInfo(msg string, log bool) {
	self.infoResponses = append(self.infoResponses, msg)
	_ = log
}

func (self *fakeStepperEnableCommand) RespondRaw(msg string) {
	self.rawResponses = append(self.rawResponses, msg)
}

type fakeStepperEnableGCode struct {
	commands map[string]func(printerpkg.Command) error
	mutex    printerpkg.Mutex
}

func (self *fakeStepperEnableGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
	_, _ = whenNotReady, desc
}

func (self *fakeStepperEnableGCode) IsTraditionalGCode(cmd string) bool { return false }

func (self *fakeStepperEnableGCode) RunScriptFromCommand(script string) {}

func (self *fakeStepperEnableGCode) RunScript(script string) {}

func (self *fakeStepperEnableGCode) IsBusy() bool { return false }

func (self *fakeStepperEnableGCode) Mutex() printerpkg.Mutex { return self.mutex }

func (self *fakeStepperEnableGCode) RespondInfo(msg string, log bool) {}

func (self *fakeStepperEnableGCode) ReplaceCommand(_ string, _ func(printerpkg.Command) error, _ bool, _ string) func(printerpkg.Command) error {
	return nil
}

type fakeStepperEnableDigitalOut struct {
	maxDuration float64
	setCalls    [][2]float64
}

func (self *fakeStepperEnableDigitalOut) MCU() printerpkg.PrintTimeEstimator { return nil }

func (self *fakeStepperEnableDigitalOut) SetupMaxDuration(maxDuration float64) {
	self.maxDuration = maxDuration
}

func (self *fakeStepperEnableDigitalOut) SetDigital(printTime float64, value int) {
	self.setCalls = append(self.setCalls, [2]float64{printTime, float64(value)})
}

type fakeStepperEnableChip struct {
	pins map[string]*fakeStepperEnableDigitalOut
}

func (self *fakeStepperEnableChip) Setup_pin(pinType string, pinParams map[string]interface{}) interface{} {
	if self.pins == nil {
		self.pins = map[string]*fakeStepperEnableDigitalOut{}
	}
	pin := pinParams["pin"].(string)
	digitalOut := self.pins[pin]
	if digitalOut == nil {
		digitalOut = &fakeStepperEnableDigitalOut{}
		self.pins[pin] = digitalOut
	}
	_ = pinType
	return digitalOut
}

type fakeStepperEnablePins struct {
	chip       *fakeStepperEnableChip
	activePins map[string]map[string]interface{}
}

func (self *fakeStepperEnablePins) LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{} {
	if self.activePins == nil {
		self.activePins = map[string]map[string]interface{}{}
	}
	if existing, ok := self.activePins[pinDesc]; ok {
		return existing
	}
	params := map[string]interface{}{
		"chip":       self.chip,
		"chip_name":  "mcu",
		"pin":        pinDesc,
		"invert":     0,
		"pullup":     0,
		"share_type": shareType,
	}
	self.activePins[pinDesc] = params
	_, _ = canInvert, canPullup
	return params
}

type fakeStepperEnableToolhead struct {
	lastMoveTime float64
	dwellCalls   []float64
}

func (self *fakeStepperEnableToolhead) Dwell(delay float64) {
	self.dwellCalls = append(self.dwellCalls, delay)
}

func (self *fakeStepperEnableToolhead) Get_last_move_time() float64 {
	return self.lastMoveTime
}

type fakeStepperEnablePrinter struct {
	gcode         *fakeStepperEnableGCode
	objects       map[string]interface{}
	eventHandlers map[string]func([]interface{}) error
	sentEvents    []string
}

func (self *fakeStepperEnablePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.objects[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeStepperEnablePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeStepperEnablePrinter) SendEvent(event string, params []interface{}) {
	self.sentEvents = append(self.sentEvents, event)
	_ = params
}

func (self *fakeStepperEnablePrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeStepperEnablePrinter) AddObject(name string, obj interface{}) error { return nil }

func (self *fakeStepperEnablePrinter) LookupObjects(module string) []interface{} { return nil }

func (self *fakeStepperEnablePrinter) HasStartArg(name string) bool { return false }

func (self *fakeStepperEnablePrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeStepperEnablePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeStepperEnablePrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }

func (self *fakeStepperEnablePrinter) InvokeShutdown(msg string) {}

func (self *fakeStepperEnablePrinter) IsShutdown() bool { return false }

func (self *fakeStepperEnablePrinter) Reactor() printerpkg.ModuleReactor { return nil }

func (self *fakeStepperEnablePrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeStepperEnablePrinter) GCode() printerpkg.GCodeRuntime { return self.gcode }

func (self *fakeStepperEnablePrinter) GCodeMove() printerpkg.MoveTransformController { return nil }

func (self *fakeStepperEnablePrinter) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeStepperEnableConfig struct {
	printer      *fakeStepperEnablePrinter
	stringValues map[string]string
}

func (self *fakeStepperEnableConfig) Name() string { return "stepper_enable" }

func (self *fakeStepperEnableConfig) String(option string, defaultValue string, _ bool) string {
	if value, ok := self.stringValues[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeStepperEnableConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeStepperEnableConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeStepperEnableConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeStepperEnableConfig) LoadObject(section string) interface{} { return nil }

func (self *fakeStepperEnableConfig) LoadTemplate(_ string, _ string, _ string) printerpkg.Template {
	return nil
}

func (self *fakeStepperEnableConfig) LoadRequiredTemplate(_ string, _ string) printerpkg.Template {
	return nil
}

func (self *fakeStepperEnableConfig) Printer() printerpkg.ModulePrinter { return self.printer }

type fakeStepperEnableStepper struct {
	name      string
	callbacks []func(float64)
}

func (self *fakeStepperEnableStepper) Get_name(_ bool) string {
	return self.name
}

func (self *fakeStepperEnableStepper) Add_active_callback(callback func(float64)) {
	self.callbacks = append(self.callbacks, callback)
}

func (self *fakeStepperEnableStepper) TriggerActive(printTime float64) {
	callbacks := append([]func(float64){}, self.callbacks...)
	self.callbacks = nil
	for _, callback := range callbacks {
		callback(printTime)
	}
}

func newFakeStepperEnableModule() (*PrinterStepperEnableModule, *fakeStepperEnablePrinter, *fakeStepperEnablePins, *fakeStepperEnableToolhead) {
	pins := &fakeStepperEnablePins{chip: &fakeStepperEnableChip{}}
	toolhead := &fakeStepperEnableToolhead{lastMoveTime: 12.5}
	printer := &fakeStepperEnablePrinter{
		gcode:   &fakeStepperEnableGCode{},
		objects: map[string]interface{}{"pins": pins, "toolhead": toolhead},
	}
	module := LoadConfigStepperEnable(&fakeStepperEnableConfig{printer: printer}).(*PrinterStepperEnableModule)
	return module, printer, pins, toolhead
}

func TestLoadConfigStepperEnableRegistersCommandsAndRestartHandler(t *testing.T) {
	module, printer, _, _ := newFakeStepperEnableModule()
	if module == nil {
		t.Fatalf("expected stepper enable module instance")
	}
	if _, ok := printer.eventHandlers["gcode:request_restart"]; !ok {
		t.Fatalf("expected gcode:request_restart handler to be registered")
	}
	for _, command := range []string{"M18", "M84", "SET_STEPPER_ENABLE"} {
		if _, ok := printer.gcode.commands[command]; !ok {
			t.Fatalf("expected %s command to be registered", command)
		}
	}
}

func TestStepperEnableRegisterStepperSharesEnablePinsAndTracksState(t *testing.T) {
	module, _, pins, _ := newFakeStepperEnableModule()
	stepperX := &fakeStepperEnableStepper{name: "stepper_x"}
	stepperY := &fakeStepperEnableStepper{name: "stepper_y"}
	module.Register_stepper(&fakeStepperEnableConfig{stringValues: map[string]string{"enable_pin": "PA1"}}, stepperX)
	module.Register_stepper(&fakeStepperEnableConfig{stringValues: map[string]string{"enable_pin": "PA1"}}, stepperY)

	lineX, err := module.Lookup_enable("stepper_x")
	if err != nil {
		t.Fatalf("Lookup_enable(stepper_x) returned error: %v", err)
	}
	lineY, err := module.Lookup_enable("stepper_y")
	if err != nil {
		t.Fatalf("Lookup_enable(stepper_y) returned error: %v", err)
	}

	stepperX.TriggerActive(3.5)
	stepperY.TriggerActive(4.5)
	digitalOut := pins.chip.pins["PA1"]
	if digitalOut == nil {
		t.Fatalf("expected shared enable pin to be created")
	}
	if got, want := digitalOut.setCalls, [][2]float64{{3.5, 1}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable pin calls after shared enable = %v, want %v", got, want)
	}
	if lineX.Has_dedicated_enable() || lineY.Has_dedicated_enable() {
		t.Fatalf("shared enable lines should not report dedicated ownership once the enable pin is shared")
	}

	lineX.Motor_disable(5.5)
	if got, want := digitalOut.setCalls, [][2]float64{{3.5, 1}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable pin calls after disabling one shared line = %v, want %v", got, want)
	}
	lineY.Motor_disable(6.5)
	if got, want := digitalOut.setCalls, [][2]float64{{3.5, 1}, {6.5, 0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable pin calls after disabling shared lines = %v, want %v", got, want)
	}
	if lineX.Is_motor_enabled() || lineY.Is_motor_enabled() {
		t.Fatalf("expected shared lines to be disabled")
	}
}

func TestStepperEnableCmdSetStepperEnableAndMotorOff(t *testing.T) {
	module, printer, pins, toolhead := newFakeStepperEnableModule()
	stepperZ := &fakeStepperEnableStepper{name: "stepper_z"}
	module.Register_stepper(&fakeStepperEnableConfig{stringValues: map[string]string{"enable_pin": "PB2"}}, stepperZ)

	cmd := &fakeStepperEnableCommand{strings: map[string]string{"STEPPER": "stepper_z"}, ints: map[string]int{"ENABLE": 1}}
	if err := module.cmdSetStepperEnable(cmd); err != nil {
		t.Fatalf("cmdSetStepperEnable(enable) returned error: %v", err)
	}
	digitalOut := pins.chip.pins["PB2"]
	if got, want := digitalOut.setCalls, [][2]float64{{12.5, 1}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable pin calls after manual enable = %v, want %v", got, want)
	}
	if got, want := toolhead.dwellCalls, []float64{disableStallTime, disableStallTime}; !reflect.DeepEqual(got, want) {
		t.Fatalf("toolhead dwells after manual enable = %v, want %v", got, want)
	}

	cmd.ints["ENABLE"] = 0
	if err := module.cmdSetStepperEnable(cmd); err != nil {
		t.Fatalf("cmdSetStepperEnable(disable) returned error: %v", err)
	}
	if got, want := digitalOut.setCalls, [][2]float64{{12.5, 1}, {12.5, 0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable pin calls after manual disable = %v, want %v", got, want)
	}

	stepperZ.TriggerActive(14.0)
	module.Motor_off()
	if got, want := digitalOut.setCalls, [][2]float64{{12.5, 1}, {12.5, 0}, {14.0, 1}, {12.5, 0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable pin calls after Motor_off = %v, want %v", got, want)
	}
	if got, want := printer.sentEvents, []string{"stepper_enable:motor_off"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sent events = %v, want %v", got, want)
	}
	invalid := &fakeStepperEnableCommand{strings: map[string]string{"STEPPER": "missing"}, ints: map[string]int{"ENABLE": 1}}
	if err := module.cmdSetStepperEnable(invalid); err != nil {
		t.Fatalf("cmdSetStepperEnable(invalid) returned error: %v", err)
	}
	if got, want := invalid.infoResponses, []string{"set_stepper_enable: Invalid stepper missing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid stepper responses = %v, want %v", got, want)
	}
}

func TestStepperEnableCallbackRunsAfterEnablePinSet(t *testing.T) {
	module, _, pins, _ := newFakeStepperEnableModule()
	stepperX := &fakeStepperEnableStepper{name: "stepper_x"}
	module.Register_stepper(&fakeStepperEnableConfig{stringValues: map[string]string{"enable_pin": "PC3"}}, stepperX)

	line, err := module.Lookup_enable("stepper_x")
	if err != nil {
		t.Fatalf("Lookup_enable(stepper_x) returned error: %v", err)
	}
	digitalOut := pins.chip.pins["PC3"]
	if digitalOut == nil {
		t.Fatalf("expected dedicated enable pin to be created")
	}

	callbackPrintTimes := []float64{}
	callbackEnableFlags := []bool{}
	callbackStateSnapshots := []bool{}
	callbackPinCallCounts := []int{}
	line.Register_state_callback(func(printTime float64, isEnable bool) {
		callbackPrintTimes = append(callbackPrintTimes, printTime)
		callbackEnableFlags = append(callbackEnableFlags, isEnable)
		callbackStateSnapshots = append(callbackStateSnapshots, line.Is_motor_enabled())
		callbackPinCallCounts = append(callbackPinCallCounts, len(digitalOut.setCalls))
	})

	stepperX.TriggerActive(7.25)

	if got, want := digitalOut.setCalls, [][2]float64{{7.25, 1}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable pin calls after active trigger = %v, want %v", got, want)
	}
	if got, want := callbackPrintTimes, []float64{7.25}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callback print times = %v, want %v", got, want)
	}
	if got, want := callbackEnableFlags, []bool{true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callback enable flags = %v, want %v", got, want)
	}
	if got, want := callbackStateSnapshots, []bool{true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callback state snapshots = %v, want %v", got, want)
	}
	if got, want := callbackPinCallCounts, []int{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callback pin call counts = %v, want %v", got, want)
	}
}
