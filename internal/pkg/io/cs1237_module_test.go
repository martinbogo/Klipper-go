package io

import (
	"fmt"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeCS1237Command struct {
	sends [][3]interface{}
}

func (self *fakeCS1237Command) Send(data interface{}, minclock int64, reqclock int64) {
	args, ok := data.([]int64)
	if !ok {
		panic(fmt.Sprintf("unexpected command payload type %T", data))
	}
	copyArgs := append([]int64{}, args...)
	self.sends = append(self.sends, [3]interface{}{copyArgs, minclock, reqclock})
}

type fakeCS1237Completion struct {
	waits  []float64
	result interface{}
}

func (self *fakeCS1237Completion) Wait(waketime float64, waketimeResult interface{}) interface{} {
	self.waits = append(self.waits, waketime)
	if self.result == nil {
		return waketimeResult
	}
	return self.result
}

type fakeCS1237Reactor struct {
	monotonic      float64
	completions    []*fakeCS1237Completion
	nextWaitResult interface{}
	asyncResults   []map[string]interface{}
}

func (self *fakeCS1237Reactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return nil
}

func (self *fakeCS1237Reactor) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeCS1237Reactor) Completion() interface{} {
	completion := &fakeCS1237Completion{result: self.nextWaitResult}
	self.completions = append(self.completions, completion)
	self.nextWaitResult = nil
	return completion
}

func (self *fakeCS1237Reactor) AsyncComplete(completion interface{}, result map[string]interface{}) {
	self.asyncResults = append(self.asyncResults, result)
	completion.(*fakeCS1237Completion).result = result
}

type fakeCS1237PinRegistry struct {
	lookupCalls []string
	lookup      map[string]map[string]interface{}
}

func (self *fakeCS1237PinRegistry) LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{} {
	self.lookupCalls = append(self.lookupCalls, pinDesc)
	if value, ok := self.lookup[pinDesc]; ok {
		result := map[string]interface{}{}
		for key, item := range value {
			result[key] = item
		}
		return result
	}
	return map[string]interface{}{"chip_name": "mcu", "pin": pinDesc}
}

type fakeCS1237GCode struct {
	scripts []string
}

func (self *fakeCS1237GCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
}
func (self *fakeCS1237GCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakeCS1237GCode) RunScriptFromCommand(script string) {
	self.scripts = append(self.scripts, script)
}
func (self *fakeCS1237GCode) RunScript(script string)          {}
func (self *fakeCS1237GCode) IsBusy() bool                     { return false }
func (self *fakeCS1237GCode) Mutex() printerpkg.Mutex          { return nil }
func (self *fakeCS1237GCode) RespondInfo(msg string, log bool) {}
func (self *fakeCS1237GCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeCS1237Toolhead struct {
	dwells []float64
}

func (self *fakeCS1237Toolhead) Dwell(delay float64) {
	self.dwells = append(self.dwells, delay)
}

type fakeCS1237MCU struct {
	nextOID          int
	callbacks        []func()
	configCmds       []string
	lookupFormats    []string
	lookupQueues     []interface{}
	responses        []string
	responseOIDs     []interface{}
	commands         map[string]*fakeCS1237Command
	secondsToClockIn []float64
	commandQueue     interface{}
}

func (self *fakeCS1237MCU) CreateOID() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeCS1237MCU) RegisterConfigCallback(cb func()) {
	self.callbacks = append(self.callbacks, cb)
}

func (self *fakeCS1237MCU) AddConfigCmd(cmd string, isInit bool, onRestart bool) {
	self.configCmds = append(self.configCmds, cmd)
}

func (self *fakeCS1237MCU) GetQuerySlot(oid int) int64 { return 0 }

func (self *fakeCS1237MCU) SecondsToClock(time float64) int64 {
	self.secondsToClockIn = append(self.secondsToClockIn, time)
	return int64(time * 1000)
}

func (self *fakeCS1237MCU) RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{}) {
	self.responses = append(self.responses, msg)
	self.responseOIDs = append(self.responseOIDs, oid)
}

func (self *fakeCS1237MCU) ClockToPrintTime(clock int64) float64 { return float64(clock) / 1000.0 }

func (self *fakeCS1237MCU) Clock32ToClock64(clock32 int64) int64 { return clock32 }

func (self *fakeCS1237MCU) LookupCommand(msgformat string, cq interface{}) (interface{}, error) {
	self.lookupFormats = append(self.lookupFormats, msgformat)
	self.lookupQueues = append(self.lookupQueues, cq)
	if self.commands == nil {
		self.commands = map[string]*fakeCS1237Command{}
	}
	command, ok := self.commands[msgformat]
	if !ok {
		command = &fakeCS1237Command{}
		self.commands[msgformat] = command
	}
	return command, nil
}

func (self *fakeCS1237MCU) NewTrsyncCommandQueue() interface{} {
	if self.commandQueue == nil {
		self.commandQueue = "trsync-queue"
	}
	return self.commandQueue
}

type fakeCS1237Printer struct {
	lookup        map[string]interface{}
	mcus          map[string]printerpkg.MCURuntime
	reactor       printerpkg.ModuleReactor
	gcode         printerpkg.GCodeRuntime
	eventHandlers map[string]func([]interface{}) error
}

func (self *fakeCS1237Printer) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeCS1237Printer) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeCS1237Printer) SendEvent(event string, params []interface{})             {}
func (self *fakeCS1237Printer) CurrentExtruderName() string                              { return "extruder" }
func (self *fakeCS1237Printer) AddObject(name string, obj interface{}) error             { return nil }
func (self *fakeCS1237Printer) LookupObjects(module string) []interface{}                { return nil }
func (self *fakeCS1237Printer) HasStartArg(name string) bool                             { return false }
func (self *fakeCS1237Printer) LookupHeater(name string) printerpkg.HeaterRuntime        { return nil }
func (self *fakeCS1237Printer) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }
func (self *fakeCS1237Printer) InvokeShutdown(msg string)                                {}
func (self *fakeCS1237Printer) IsShutdown() bool                                         { return false }
func (self *fakeCS1237Printer) StepperEnable() printerpkg.StepperEnableRuntime           { return nil }
func (self *fakeCS1237Printer) GCodeMove() printerpkg.MoveTransformController            { return nil }
func (self *fakeCS1237Printer) Webhooks() printerpkg.WebhookRegistry                     { return nil }

func (self *fakeCS1237Printer) LookupMCU(name string) printerpkg.MCURuntime {
	if value, ok := self.mcus[name]; ok {
		return value
	}
	return nil
}

func (self *fakeCS1237Printer) Reactor() printerpkg.ModuleReactor {
	return self.reactor
}

func (self *fakeCS1237Printer) GCode() printerpkg.GCodeRuntime {
	return self.gcode
}

type fakeCS1237Config struct {
	printer printerpkg.ModulePrinter
	name    string
	strings map[string]string
}

func (self *fakeCS1237Config) Name() string { return self.name }
func (self *fakeCS1237Config) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}
func (self *fakeCS1237Config) Bool(option string, defaultValue bool) bool        { return defaultValue }
func (self *fakeCS1237Config) Float(option string, defaultValue float64) float64 { return defaultValue }
func (self *fakeCS1237Config) OptionalFloat(option string) *float64              { return nil }
func (self *fakeCS1237Config) LoadObject(section string) interface{}             { return nil }
func (self *fakeCS1237Config) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}
func (self *fakeCS1237Config) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}
func (self *fakeCS1237Config) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigCS1237RegistersCallbacksAndBuildsCommands(t *testing.T) {
	mcu := &fakeCS1237MCU{nextOID: 9}
	pins := &fakeCS1237PinRegistry{lookup: map[string]map[string]interface{}{
		"PB1": {"chip_name": "mcu", "pin": "gpio11"},
		"PB2": {"chip_name": "mcu", "pin": "gpio12"},
		"PB3": {"chip_name": "mcu", "pin": "gpio13"},
	}}
	printer := &fakeCS1237Printer{
		lookup:  map[string]interface{}{"pins": pins, "toolhead": &fakeCS1237Toolhead{}},
		mcus:    map[string]printerpkg.MCURuntime{"mcu": mcu},
		reactor: &fakeCS1237Reactor{monotonic: 5.0},
		gcode:   &fakeCS1237GCode{},
	}
	module := LoadConfigCS1237(&fakeCS1237Config{
		printer: printer,
		name:    "cs1237",
		strings: map[string]string{
			"register":    "3",
			"sensitivity": "8",
			"dout_pin":    "PB1",
			"sclk_pin":    "PB2",
			"level_pin":   "PB3",
		},
	}).(*CS1237Module)

	if module == nil {
		t.Fatalf("expected module instance")
	}
	if len(pins.lookupCalls) != 3 {
		t.Fatalf("unexpected pin lookups: %#v", pins.lookupCalls)
	}
	if len(mcu.callbacks) != 1 {
		t.Fatalf("expected one config callback, got %d", len(mcu.callbacks))
	}
	if printer.eventHandlers["homing:multi_probe_begin"] == nil || printer.eventHandlers["homing:multi_probe_end"] == nil {
		t.Fatalf("expected multi-probe event handlers registration")
	}

	mcu.callbacks[0]()
	if len(mcu.configCmds) != 1 {
		t.Fatalf("unexpected config commands: %#v", mcu.configCmds)
	}
	expectedConfig := "config_cs1237 oid=9 level_pin=gpio13 dout_pin=gpio11 sclk_pin=gpio12 register=3 sensitivity=8"
	if mcu.configCmds[0] != expectedConfig {
		t.Fatalf("unexpected config command: %q", mcu.configCmds[0])
	}
	expectedLookups := []string{
		"reset_cs1237 oid=%c count=%c",
		"start_cs1237_report oid=%c enable=%c ticks=%i print_state=%c sensitivity=%i",
		"enable_cs1237 oid=%c state=%c",
		"query_cs1237_diff oid=%c",
		"cs1237_calibration_phase oid=%c cali_state=%c speed_state=%c",
		"cs1237_calibration_DataProcess oid=%c",
	}
	if fmt.Sprint(mcu.lookupFormats) != fmt.Sprint(expectedLookups) {
		t.Fatalf("unexpected command lookups: %#v", mcu.lookupFormats)
	}
	for _, queue := range mcu.lookupQueues {
		if queue != "trsync-queue" {
			t.Fatalf("unexpected command queue: %#v", queue)
		}
	}
	if fmt.Sprint(mcu.responses) != fmt.Sprint([]string{"cs1237_state", "cs1237_diff", "cs1237_calibration_Val"}) {
		t.Fatalf("unexpected response registrations: %#v", mcu.responses)
	}
	for _, oid := range mcu.responseOIDs {
		if oid != 9 {
			t.Fatalf("unexpected response oid: %#v", oid)
		}
	}
}

func TestCS1237ModuleHandlesQueriesAndEvents(t *testing.T) {
	mcu := &fakeCS1237MCU{nextOID: 4}
	reactor := &fakeCS1237Reactor{monotonic: 7.0}
	gcode := &fakeCS1237GCode{}
	toolhead := &fakeCS1237Toolhead{}
	pins := &fakeCS1237PinRegistry{lookup: map[string]map[string]interface{}{
		"PB1": {"chip_name": "mcu", "pin": "gpio11"},
		"PB2": {"chip_name": "mcu", "pin": "gpio12"},
		"PB3": {"chip_name": "mcu", "pin": "gpio13"},
	}}
	printer := &fakeCS1237Printer{
		lookup:  map[string]interface{}{"pins": pins, "toolhead": toolhead},
		mcus:    map[string]printerpkg.MCURuntime{"mcu": mcu},
		reactor: reactor,
		gcode:   gcode,
	}
	module := LoadConfigCS1237(&fakeCS1237Config{
		printer: printer,
		name:    "cs1237",
		strings: map[string]string{
			"register":    "1",
			"sensitivity": "2",
			"dout_pin":    "PB1",
			"sclk_pin":    "PB2",
			"level_pin":   "PB3",
		},
	}).(*CS1237Module)

	mcu.callbacks[0]()
	if err := module.handleQueryState(map[string]interface{}{"adc": int64(11), "raw": int64(22), "state": int64(1)}); err != nil {
		t.Fatalf("query handler returned error: %v", err)
	}
	if ok, stats := module.Stats(0); !ok || stats != "cs1237:adc_value=11 raw_value=22 sensor_state=1" {
		t.Fatalf("unexpected stats response: %v %q", ok, stats)
	}
	if len(module.Get_status(0)) != 0 {
		t.Fatalf("expected empty status when report=false")
	}

	module.Reset_cs(2)
	resetSends := mcu.commands["reset_cs1237 oid=%c count=%c"].sends
	if len(resetSends) != 1 || fmt.Sprint(resetSends[0][0]) != fmt.Sprint([]int64{4, 2}) {
		t.Fatalf("unexpected reset sends: %#v", resetSends)
	}
	if fmt.Sprint(toolhead.dwells) != fmt.Sprint([]float64{0.1}) {
		t.Fatalf("unexpected toolhead dwells: %#v", toolhead.dwells)
	}

	if err := printer.eventHandlers["homing:multi_probe_begin"](nil); err != nil {
		t.Fatalf("enable handler returned error: %v", err)
	}
	if err := printer.eventHandlers["homing:multi_probe_begin"](nil); err != nil {
		t.Fatalf("second enable handler returned error: %v", err)
	}
	enableSends := mcu.commands["enable_cs1237 oid=%c state=%c"].sends
	if len(enableSends) != 1 || fmt.Sprint(enableSends[0][0]) != fmt.Sprint([]int64{4, 1}) {
		t.Fatalf("unexpected enable sends after two begins: %#v", enableSends)
	}
	if fmt.Sprint(gcode.scripts) != fmt.Sprint([]string{"G4 P500", "G4 P500"}) {
		t.Fatalf("unexpected gcode scripts: %#v", gcode.scripts)
	}
	if err := printer.eventHandlers["homing:multi_probe_end"](nil); err != nil {
		t.Fatalf("disable handler returned error: %v", err)
	}
	if err := printer.eventHandlers["homing:multi_probe_end"](nil); err != nil {
		t.Fatalf("second disable handler returned error: %v", err)
	}
	enableSends = mcu.commands["enable_cs1237 oid=%c state=%c"].sends
	if len(enableSends) != 2 || fmt.Sprint(enableSends[1][0]) != fmt.Sprint([]int64{4, 0}) {
		t.Fatalf("unexpected enable sends after disables: %#v", enableSends)
	}

	reactor.nextWaitResult = map[string]interface{}{"diff": int64(7), "raw": int64(8)}
	diff, raw, err := module.Diff()
	if err != nil || diff != 7 || raw != 8 {
		t.Fatalf("unexpected diff result: diff=%d raw=%d err=%v", diff, raw, err)
	}
	diffSends := mcu.commands["query_cs1237_diff oid=%c"].sends
	if len(diffSends) != 1 || fmt.Sprint(diffSends[0][0]) != fmt.Sprint([]int64{4}) {
		t.Fatalf("unexpected diff sends: %#v", diffSends)
	}

	reactor.nextWaitResult = map[string]interface{}{"BlockPreVal": int64(1), "TargetVal": int64(2), "RealVal": int64(3)}
	block, target, real := module.CalibrationVal()
	if block != 1 || target != 2 || real != 3 {
		t.Fatalf("unexpected calibration values: %d %d %d", block, target, real)
	}
	calibrationSends := mcu.commands["cs1237_calibration_DataProcess oid=%c"].sends
	if len(calibrationSends) != 1 || fmt.Sprint(calibrationSends[0][0]) != fmt.Sprint([]int64{4}) {
		t.Fatalf("unexpected calibration query sends: %#v", calibrationSends)
	}

	module.Calibration(5, 6)
	phaseSends := mcu.commands["cs1237_calibration_phase oid=%c cali_state=%c speed_state=%c"].sends
	if len(phaseSends) != 1 || fmt.Sprint(phaseSends[0][0]) != fmt.Sprint([]int64{4, 5, 6}) {
		t.Fatalf("unexpected calibration phase sends: %#v", phaseSends)
	}
	module.CheckStart(0.25, 3, 9, 0.1)
	reportSends := mcu.commands["start_cs1237_report oid=%c enable=%c ticks=%i print_state=%c sensitivity=%i"].sends
	if len(reportSends) != 1 || fmt.Sprint(reportSends[0][0]) != fmt.Sprint([]int64{4, 1, 250, 3, 9}) {
		t.Fatalf("unexpected check start sends: %#v", reportSends)
	}
	module.CheckStop(3)
	reportSends = mcu.commands["start_cs1237_report oid=%c enable=%c ticks=%i print_state=%c sensitivity=%i"].sends
	if len(reportSends) != 2 || fmt.Sprint(reportSends[1][0]) != fmt.Sprint([]int64{4, 0, 0, 3, 0}) {
		t.Fatalf("unexpected check stop sends: %#v", reportSends)
	}
}
