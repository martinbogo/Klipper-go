package vibration

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeResonanceCommand struct {
	strings       map[string]string
	floats        map[string]float64
	ints          map[string]int
	infoResponses []string
	rawResponses  []string
}

func (self *fakeResonanceCommand) String(name string, defaultValue string) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeResonanceCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeResonanceCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	if value, ok := self.ints[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeResonanceCommand) Parameters() map[string]string {
	params := map[string]string{}
	for key, value := range self.strings {
		params[key] = value
	}
	return params
}

func (self *fakeResonanceCommand) RespondInfo(msg string, log bool) {
	self.infoResponses = append(self.infoResponses, msg)
	_ = log
}

func (self *fakeResonanceCommand) RespondRaw(msg string) {
	self.rawResponses = append(self.rawResponses, msg)
}

func (self *fakeResonanceCommand) Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	if defaultString, ok := _default.(string); ok {
		return defaultString
	}
	return ""
}

func (self *fakeResonanceCommand) Get_int(name string, _default interface{}, minval *int, maxval *int) int {
	if value, ok := self.ints[name]; ok {
		return value
	}
	if defaultInt, ok := _default.(int); ok {
		return defaultInt
	}
	return 0
}

func (self *fakeResonanceCommand) Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	if defaultFloat, ok := _default.(float64); ok {
		return defaultFloat
	}
	return 0.
}

type fakeResonanceGCode struct {
	commands    map[string]func(printerpkg.Command) error
	scriptCalls []string
	infoCalls   []string
}

func (self *fakeResonanceGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
	_, _ = whenNotReady, desc
}

func (self *fakeResonanceGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakeResonanceGCode) RunScriptFromCommand(script string) {
	self.scriptCalls = append(self.scriptCalls, script)
}
func (self *fakeResonanceGCode) RunScript(script string) {
	self.scriptCalls = append(self.scriptCalls, script)
}
func (self *fakeResonanceGCode) IsBusy() bool            { return false }
func (self *fakeResonanceGCode) Mutex() printerpkg.Mutex { return nil }
func (self *fakeResonanceGCode) RespondInfo(msg string, log bool) {
	self.infoCalls = append(self.infoCalls, msg)
	_ = log
}
func (self *fakeResonanceGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeResonanceReactor struct {
	now float64
}

func (self *fakeResonanceReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return nil
}

func (self *fakeResonanceReactor) Monotonic() float64 {
	return self.now
}

func (self *fakeResonanceReactor) Pause(waketime float64) float64 {
	self.now = waketime
	return waketime
}

type fakeResonanceToolhead struct {
	position []float64
	status   map[string]interface{}
	moves    []struct {
		pos   []float64
		speed float64
	}
	manualMoves   [][]interface{}
	dwells        []float64
	waitCalls     int
	m204Calls     []float64
	flushCount    int
	scanTimeCalls [][2]float64
}

func (self *fakeResonanceToolhead) Get_position() []float64 {
	copied := make([]float64, len(self.position))
	copy(copied, self.position)
	return copied
}

func (self *fakeResonanceToolhead) Get_status(eventtime float64) map[string]interface{} {
	return self.status
}

func (self *fakeResonanceToolhead) Move(newpos []float64, speed float64) {
	copied := make([]float64, len(newpos))
	copy(copied, newpos)
	self.moves = append(self.moves, struct {
		pos   []float64
		speed float64
	}{pos: copied, speed: speed})
}

func (self *fakeResonanceToolhead) Manual_move(coord []interface{}, speed float64) {
	copied := append([]interface{}{}, coord...)
	self.manualMoves = append(self.manualMoves, copied)
	_ = speed
}

func (self *fakeResonanceToolhead) Wait_moves() {
	self.waitCalls++
}

func (self *fakeResonanceToolhead) Dwell(delay float64) {
	self.dwells = append(self.dwells, delay)
}

func (self *fakeResonanceToolhead) M204(accel float64) {
	self.m204Calls = append(self.m204Calls, accel)
}

func (self *fakeResonanceToolhead) Flush_step_generation() {
	self.flushCount++
}

func (self *fakeResonanceToolhead) Note_step_generation_scan_time(delay, oldDelay float64) {
	self.scanTimeCalls = append(self.scanTimeCalls, [2]float64{delay, oldDelay})
}

func (self *fakeResonanceToolhead) Get_kinematics() interface{} {
	return nil
}

type fakeResonanceConfigStore struct {
	values map[string]map[string]string
}

func (self *fakeResonanceConfigStore) Set(section string, option string, val string) {
	if self.values == nil {
		self.values = map[string]map[string]string{}
	}
	if self.values[section] == nil {
		self.values[section] = map[string]string{}
	}
	self.values[section][option] = val
}

type fakeResonancePrinter struct {
	lookup        map[string]interface{}
	gcode         *fakeResonanceGCode
	reactor       *fakeResonanceReactor
	eventHandlers map[string]func([]interface{}) error
}

func (self *fakeResonancePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeResonancePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeResonancePrinter) SendEvent(event string, params []interface{}) {}
func (self *fakeResonancePrinter) CurrentExtruderName() string                  { return "extruder" }
func (self *fakeResonancePrinter) AddObject(name string, obj interface{}) error { return nil }
func (self *fakeResonancePrinter) LookupObjects(module string) []interface{}    { return nil }
func (self *fakeResonancePrinter) HasStartArg(name string) bool                 { return false }
func (self *fakeResonancePrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}
func (self *fakeResonancePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}
func (self *fakeResonancePrinter) LookupMCU(name string) printerpkg.MCURuntime    { return nil }
func (self *fakeResonancePrinter) InvokeShutdown(msg string)                      {}
func (self *fakeResonancePrinter) IsShutdown() bool                               { return false }
func (self *fakeResonancePrinter) Reactor() printerpkg.ModuleReactor              { return self.reactor }
func (self *fakeResonancePrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }
func (self *fakeResonancePrinter) GCode() printerpkg.GCodeRuntime                 { return self.gcode }
func (self *fakeResonancePrinter) GCodeMove() printerpkg.MoveTransformController  { return nil }
func (self *fakeResonancePrinter) Webhooks() printerpkg.WebhookRegistry           { return nil }

type fakeResonanceConfig struct {
	printer  *fakeResonancePrinter
	name     string
	values   map[string]interface{}
	lists    map[string]interface{}
	sections map[string]bool
}

func (self *fakeResonanceConfig) Name() string { return self.name }

func (self *fakeResonanceConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.values[option]; ok {
		return value.(string)
	}
	return defaultValue
}

func (self *fakeResonanceConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeResonanceConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.values[option]; ok {
		return value.(float64)
	}
	return defaultValue
}

func (self *fakeResonanceConfig) OptionalFloat(option string) *float64  { return nil }
func (self *fakeResonanceConfig) LoadObject(section string) interface{} { return nil }
func (self *fakeResonanceConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}
func (self *fakeResonanceConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}
func (self *fakeResonanceConfig) Printer() printerpkg.ModulePrinter { return self.printer }
func (self *fakeResonanceConfig) HasOption(option string) bool {
	_, ok := self.values[option]
	return ok
}
func (self *fakeResonanceConfig) Get(option string, default1 interface{}, noteValid bool) interface{} {
	if value, ok := self.values[option]; ok {
		return value
	}
	return default1
}
func (self *fakeResonanceConfig) Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int {
	if value, ok := self.values[option]; ok {
		return value.(int)
	}
	if defaultInt, ok := default1.(int); ok {
		return defaultInt
	}
	return 0
}
func (self *fakeResonanceConfig) Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	if value, ok := self.values[option]; ok {
		return value.(float64)
	}
	if defaultFloat, ok := default1.(float64); ok {
		return defaultFloat
	}
	return 0
}
func (self *fakeResonanceConfig) Getlists(option string, default1 interface{}, seps []string, count int, kind reflect.Kind, noteValid bool) interface{} {
	if value, ok := self.lists[option]; ok {
		return value
	}
	return default1
}

type fakeAccelClient struct {
	samples     [][]float64
	finishCalls int
	files       []string
}

func (self *fakeAccelClient) Finish_measurements() {
	self.finishCalls++
}

func (self *fakeAccelClient) Write_to_file(file string) {
	self.files = append(self.files, file)
}

func (self *fakeAccelClient) Has_valid_samples() bool {
	return len(self.samples) > 0
}

func (self *fakeAccelClient) Get_samples() [][]float64 {
	return self.samples
}

type fakeAccelChip struct {
	name    string
	samples [][]float64
	started []*fakeAccelClient
}

func (self *fakeAccelChip) Start_internal_client() accelClient {
	client := &fakeAccelClient{samples: self.samples}
	self.started = append(self.started, client)
	return client
}

func (self *fakeAccelChip) Get_name() string {
	return self.name
}

func makeProbePoints() interface{} {
	return [][]interface{}{{1.0, 2.0, 3.0}}
}

func makeResonanceConfig(printer *fakeResonancePrinter) *fakeResonanceConfig {
	return &fakeResonanceConfig{
		printer: printer,
		name:    "resonance_tester",
		values: map[string]interface{}{
			"move_speed":    50.0,
			"min_freq":      5.0,
			"max_freq":      5.0,
			"accel_per_hz":  10.0,
			"hz_per_sec":    1.0,
			"accel_chip":    "chip0",
			"max_smoothing": 0.2,
		},
		lists: map[string]interface{}{
			"probe_points": makeProbePoints(),
		},
	}
}

func makeSineSamples() [][]float64 {
	samples := make([][]float64, 1024)
	for i := range samples {
		timeValue := float64(i) / 1000.0
		samples[i] = []float64{timeValue, math.Sin(2 * math.Pi * 40 * timeValue), 0.0, 0.0}
	}
	return samples
}

func TestToolheadMinimumCruiseRatioUsesCanonicalOrLegacyStatus(t *testing.T) {
	if ratio := toolheadMinimumCruiseRatio(map[string]interface{}{"minimum_cruise_ratio": 0.25, "max_accel": 1000.0, "max_accel_to_decel": 900.0}); math.Abs(ratio-0.25) > 1e-9 {
		t.Fatalf("expected canonical minimum_cruise_ratio to win, got %v", ratio)
	}
	if ratio := toolheadMinimumCruiseRatio(map[string]interface{}{"max_accel": 1000.0, "max_accel_to_decel": 900.0}); math.Abs(ratio-0.1) > 1e-9 {
		t.Fatalf("expected legacy accel-to-decel fallback ratio 0.1, got %v", ratio)
	}
	if ratio := toolheadMinimumCruiseRatio(map[string]interface{}{"max_accel": 0.0, "max_accel_to_decel": 900.0}); math.Abs(ratio-0.0) > 1e-9 {
		t.Fatalf("expected zero ratio fallback for invalid max_accel, got %v", ratio)
	}
}

func TestLoadConfigResonanceTesterRegistersCommandsAndConnectsChips(t *testing.T) {
	toolhead := &fakeResonanceToolhead{
		position: []float64{0, 0, 0, 0},
		status:   map[string]interface{}{"max_accel": 1000.0, "max_accel_to_decel": 1000.0},
	}
	chip := &fakeAccelChip{name: "chip0"}
	printer := &fakeResonancePrinter{
		lookup: map[string]interface{}{
			"toolhead": toolhead,
			"chip0":    chip,
		},
		gcode:   &fakeResonanceGCode{},
		reactor: &fakeResonanceReactor{},
	}
	module := LoadConfigResonanceTester(makeResonanceConfig(printer)).(*ResonanceTester)
	if module == nil {
		t.Fatalf("expected resonance tester module instance")
	}
	for _, command := range []string{"MEASURE_AXES_NOISE", "TEST_RESONANCES", "SHAPER_CALIBRATE"} {
		if printer.gcode.commands[command] == nil {
			t.Fatalf("expected %s command registration", command)
		}
	}
	if printer.eventHandlers["project:connect"] == nil {
		t.Fatalf("expected project:connect handler registration")
	}
	if err := module.connect(nil); err != nil {
		t.Fatalf("connect returned error: %v", err)
	}
	if len(module.accel_chips) != 1 {
		t.Fatalf("expected one resolved accelerometer chip, got %d", len(module.accel_chips))
	}
	if module.accel_chips[0].Chip_axis != "xy" || module.accel_chips[0].Chip.Get_name() != "chip0" {
		t.Fatalf("unexpected accelerometer mapping: %#v", module.accel_chips[0])
	}
}

func TestLoadConfigResonanceTesterAllowsMissingMaxSmoothing(t *testing.T) {
	toolhead := &fakeResonanceToolhead{
		position: []float64{0, 0, 0, 0},
		status:   map[string]interface{}{"max_accel": 1000.0, "max_accel_to_decel": 1000.0},
	}
	printer := &fakeResonancePrinter{
		lookup: map[string]interface{}{
			"toolhead": toolhead,
			"chip0":    &fakeAccelChip{name: "chip0"},
		},
		gcode:   &fakeResonanceGCode{},
		reactor: &fakeResonanceReactor{},
	}
	config := makeResonanceConfig(printer)
	delete(config.values, "max_smoothing")

	module := LoadConfigResonanceTester(config).(*ResonanceTester)
	if module.max_smoothing != 0 {
		t.Fatalf("expected missing max_smoothing to default to 0, got %v", module.max_smoothing)
	}
}

func TestResonanceTesterCmdTestResonancesWritesRawDataAndRestoresInputShaper(t *testing.T) {
	toolhead := &fakeResonanceToolhead{
		position: []float64{0, 0, 0, 0},
		status:   map[string]interface{}{"max_accel": 1000.0, "minimum_cruise_ratio": 0.1, "max_accel_to_decel": 900.0},
	}
	chip := &fakeAccelChip{name: "chip0", samples: makeSineSamples()}
	inputShaper := &InputShaper{
		toolhead: toolhead,
		shapers: []*AxisInputShaper{
			{axis: "x", n: 1, A: []float64{1}, T: []float64{0.1}},
			{axis: "y", n: 1, A: []float64{1}, T: []float64{0.1}},
		},
	}
	printer := &fakeResonancePrinter{
		lookup: map[string]interface{}{
			"toolhead":     toolhead,
			"chip0":        chip,
			"input_shaper": inputShaper,
		},
		gcode:   &fakeResonanceGCode{},
		reactor: &fakeResonanceReactor{},
	}
	module := NewResonanceTester(makeResonanceConfig(printer))
	if err := module.connect(nil); err != nil {
		t.Fatalf("connect returned error: %v", err)
	}
	cmd := &fakeResonanceCommand{strings: map[string]string{"AXIS": "x", "OUTPUT": "raw_data", "NAME": "testraw"}}
	if err := module.Cmd_TEST_RESONANCES(cmd); err != nil {
		t.Fatalf("Cmd_TEST_RESONANCES returned error: %v", err)
	}
	if len(chip.started) != 1 {
		t.Fatalf("expected one accelerometer client, got %d", len(chip.started))
	}
	client := chip.started[0]
	if client.finishCalls != 1 {
		t.Fatalf("expected Finish_measurements to be called once, got %d", client.finishCalls)
	}
	wantFile := filepath.Join("/tmp", "raw_data_x_chip0_testraw.csv")
	if !reflect.DeepEqual(client.files, []string{wantFile}) {
		t.Fatalf("unexpected raw-data file writes: %#v", client.files)
	}
	if len(printer.gcode.scriptCalls) != 2 {
		t.Fatalf("expected two SET_VELOCITY_LIMIT scripts, got %#v", printer.gcode.scriptCalls)
	}
	if printer.gcode.scriptCalls[0] != "SET_VELOCITY_LIMIT ACCEL=50.000 MINIMUM_CRUISE_RATIO=0.000" {
		t.Fatalf("unexpected override script %q", printer.gcode.scriptCalls[0])
	}
	if printer.gcode.scriptCalls[1] != "SET_VELOCITY_LIMIT ACCEL=1000.000 MINIMUM_CRUISE_RATIO=0.100" {
		t.Fatalf("unexpected restore script %q", printer.gcode.scriptCalls[1])
	}
	if len(toolhead.moves) != 2 {
		t.Fatalf("expected one resonance pulse iteration with two moves, got %d", len(toolhead.moves))
	}
	if toolhead.flushCount < 2 {
		t.Fatalf("expected input shaper disable/enable to flush toolhead, got %d", toolhead.flushCount)
	}
	if !containsString(cmd.infoResponses, "Re-enabled [input_shaper]") {
		t.Fatalf("expected input shaper re-enable message, got %#v", cmd.infoResponses)
	}
}

func TestResonanceTesterCmdShaperCalibrateSavesRecommendedSettings(t *testing.T) {
	toolhead := &fakeResonanceToolhead{
		position: []float64{0, 0, 0, 0},
		status:   map[string]interface{}{"max_accel": 1000.0, "max_accel_to_decel": 900.0},
	}
	chip := &fakeAccelChip{name: "chip0", samples: makeSineSamples()}
	configStore := &fakeResonanceConfigStore{}
	printer := &fakeResonancePrinter{
		lookup: map[string]interface{}{
			"toolhead":   toolhead,
			"chip0":      chip,
			"configfile": configStore,
		},
		gcode:   &fakeResonanceGCode{},
		reactor: &fakeResonanceReactor{},
	}
	module := NewResonanceTester(makeResonanceConfig(printer))
	if err := module.connect(nil); err != nil {
		t.Fatalf("connect returned error: %v", err)
	}
	cmd := &fakeResonanceCommand{strings: map[string]string{"AXIS": "x", "NAME": "testcal"}}
	outputFile := BuildFilename("calibration_data", "testcal", NewTestAxis("x", nil), nil, "")
	defer os.Remove(outputFile)
	if err := module.Cmd_SHAPER_CALIBRATE(cmd); err != nil {
		t.Fatalf("Cmd_SHAPER_CALIBRATE returned error: %v", err)
	}
	settings := configStore.values["input_shaper"]
	if settings == nil {
		t.Fatalf("expected input_shaper settings to be saved")
	}
	if settings["shaper_type_x"] == "" || settings["shaper_freq_x"] == "" {
		t.Fatalf("expected shaper parameters to be saved, got %#v", settings)
	}
	if !containsResponsePrefix(cmd.infoResponses, "Recommended shaper_type_x = ") {
		t.Fatalf("expected recommendation output, got %#v", cmd.infoResponses)
	}
	if _, err := os.Stat(outputFile); err != nil {
		t.Fatalf("expected calibration data file %q to exist: %v", outputFile, err)
	}
}

func TestResonanceTesterUsesLegacyStatusAliasAsFallback(t *testing.T) {
	toolhead := &fakeResonanceToolhead{
		position: []float64{0, 0, 0, 0},
		status:   map[string]interface{}{"max_accel": 1000.0, "max_accel_to_decel": 900.0},
	}
	chip := &fakeAccelChip{name: "chip0", samples: makeSineSamples()}
	printer := &fakeResonancePrinter{
		lookup: map[string]interface{}{
			"toolhead": toolhead,
			"chip0":    chip,
		},
		gcode:   &fakeResonanceGCode{},
		reactor: &fakeResonanceReactor{},
	}
	module := NewResonanceTester(makeResonanceConfig(printer))
	if err := module.connect(nil); err != nil {
		t.Fatalf("connect returned error: %v", err)
	}
	cmd := &fakeResonanceCommand{strings: map[string]string{"AXIS": "x", "OUTPUT": "raw_data", "NAME": "legacyfallback"}}
	if err := module.Cmd_TEST_RESONANCES(cmd); err != nil {
		t.Fatalf("Cmd_TEST_RESONANCES returned error: %v", err)
	}
	if len(printer.gcode.scriptCalls) != 2 {
		t.Fatalf("expected two SET_VELOCITY_LIMIT scripts, got %#v", printer.gcode.scriptCalls)
	}
	if printer.gcode.scriptCalls[1] != "SET_VELOCITY_LIMIT ACCEL=1000.000 MINIMUM_CRUISE_RATIO=0.100" {
		t.Fatalf("expected legacy fallback restore script, got %q", printer.gcode.scriptCalls[1])
	}
}

func containsResponsePrefix(messages []string, prefix string) bool {
	for _, message := range messages {
		if len(message) >= len(prefix) && message[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
