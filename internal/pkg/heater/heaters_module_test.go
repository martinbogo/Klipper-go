package heater

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeHeaterSensor struct {
	reportDelta float64
	minTemp     float64
	maxTemp     float64
	callback    func(float64, float64)
}

func (self *fakeHeaterSensor) SetupMinMax(minTemp float64, maxTemp float64) {
	self.minTemp = minTemp
	self.maxTemp = maxTemp
}

func (self *fakeHeaterSensor) SetupCallback(callback func(float64, float64)) {
	self.callback = callback
}

func (self *fakeHeaterSensor) GetReportTimeDelta() float64 {
	return self.reportDelta
}

type fakeHeaterMCU struct {
	estimatedPrintTime float64
}

func (self *fakeHeaterMCU) EstimatedPrintTime(eventtime float64) float64 {
	return self.estimatedPrintTime
}

type fakeHeaterPWMPin struct {
	mcu          *fakeHeaterMCU
	maxDurations []float64
	cycleCalls   []struct {
		cycleTime   float64
		hardwarePWM bool
	}
	pwmCalls []struct {
		printTime float64
		value     float64
	}
}

func (self *fakeHeaterPWMPin) MCU() interface{} {
	return self.mcu
}

func (self *fakeHeaterPWMPin) SetupMaxDuration(maxDuration float64) {
	self.maxDurations = append(self.maxDurations, maxDuration)
}

func (self *fakeHeaterPWMPin) SetupCycleTime(cycleTime float64, hardwarePWM bool) {
	self.cycleCalls = append(self.cycleCalls, struct {
		cycleTime   float64
		hardwarePWM bool
	}{cycleTime: cycleTime, hardwarePWM: hardwarePWM})
}

func (self *fakeHeaterPWMPin) SetPWM(printTime float64, value float64) {
	self.pwmCalls = append(self.pwmCalls, struct {
		printTime float64
		value     float64
	}{printTime: printTime, value: value})
}

type fakeHeaterPins struct {
	pwm       *fakeHeaterPWMPin
	setupPins []string
}

func (self *fakeHeaterPins) SetupPWM(pin string) interface{} {
	self.setupPins = append(self.setupPins, pin)
	return self.pwm
}

type fakeHeaterToolhead struct {
	lookaheadCallbacks int
	lastMoveCalls      int
}

func (self *fakeHeaterToolhead) Get_last_move_time() float64 {
	self.lastMoveCalls++
	return 0.
}

func (self *fakeHeaterToolhead) RegisterLookaheadCallback(callback func(float64)) {
	self.lookaheadCallbacks++
}

type fakeHeaterReactor struct {
	now        float64
	pauseCalls []float64
}

func (self *fakeHeaterReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return nil
}

func (self *fakeHeaterReactor) Monotonic() float64 {
	return self.now
}

func (self *fakeHeaterReactor) Pause(waketime float64) float64 {
	self.pauseCalls = append(self.pauseCalls, waketime)
	self.now = waketime
	return waketime
}

type fakeHeaterGCode struct {
	handlers     map[string]func(printerpkg.Command) error
	muxHandlers  map[string]map[string]func(printerpkg.Command) error
	rawResponses []string
	infoMessages []string
}

func (self *fakeHeaterGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.handlers == nil {
		self.handlers = map[string]func(printerpkg.Command) error{}
	}
	self.handlers[cmd] = handler
}

func (self *fakeHeaterGCode) RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string) {
	if self.muxHandlers == nil {
		self.muxHandlers = map[string]map[string]func(printerpkg.Command) error{}
	}
	if self.muxHandlers[cmd] == nil {
		self.muxHandlers[cmd] = map[string]func(printerpkg.Command) error{}
	}
	self.muxHandlers[cmd][value] = handler
}

func (self *fakeHeaterGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakeHeaterGCode) RunScriptFromCommand(script string) {}
func (self *fakeHeaterGCode) RunScript(script string)            {}
func (self *fakeHeaterGCode) IsBusy() bool                       { return false }
func (self *fakeHeaterGCode) Mutex() printerpkg.Mutex            { return nil }
func (self *fakeHeaterGCode) RespondInfo(msg string, log bool) {
	self.infoMessages = append(self.infoMessages, msg)
}
func (self *fakeHeaterGCode) RespondRaw(msg string) {
	self.rawResponses = append(self.rawResponses, msg)
}
func (self *fakeHeaterGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeHeaterPrinter struct {
	lookup        map[string]interface{}
	gcode         *fakeHeaterGCode
	reactor       *fakeHeaterReactor
	eventHandlers map[string]func([]interface{}) error
	startArgs     map[string]bool
	shutdown      bool
	shutdownMsg   string
}

func (self *fakeHeaterPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeHeaterPrinter) SendEvent(event string, params []interface{}) {}
func (self *fakeHeaterPrinter) CurrentExtruderName() string                  { return "extruder" }
func (self *fakeHeaterPrinter) AddObject(name string, obj interface{}) error {
	if self.lookup == nil {
		self.lookup = map[string]interface{}{}
	}
	self.lookup[name] = obj
	return nil
}
func (self *fakeHeaterPrinter) LookupObjects(module string) []interface{}         { return nil }
func (self *fakeHeaterPrinter) HasStartArg(name string) bool                      { return self.startArgs[name] }
func (self *fakeHeaterPrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }
func (self *fakeHeaterPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}
func (self *fakeHeaterPrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }
func (self *fakeHeaterPrinter) InvokeShutdown(msg string) {
	self.shutdown = true
	self.shutdownMsg = msg
}
func (self *fakeHeaterPrinter) IsShutdown() bool                               { return self.shutdown }
func (self *fakeHeaterPrinter) Reactor() printerpkg.ModuleReactor              { return self.reactor }
func (self *fakeHeaterPrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }
func (self *fakeHeaterPrinter) GCode() printerpkg.GCodeRuntime                 { return self.gcode }
func (self *fakeHeaterPrinter) GCodeMove() printerpkg.MoveTransformController  { return nil }
func (self *fakeHeaterPrinter) Webhooks() printerpkg.WebhookRegistry           { return nil }

type fakeHeaterConfig struct {
	printer             *fakeHeaterPrinter
	name                string
	strings             map[string]string
	floats              map[string]float64
	hasOptions          map[string]bool
	loadObjectCalls     []string
	supportConfigCalls  []string
	supportConfigErrors map[string]error
	deprecated          []string
}

func (self *fakeHeaterConfig) Name() string { return self.name }

func (self *fakeHeaterConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeHeaterConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterConfig) OptionalFloat(option string) *float64 {
	if value, ok := self.floats[option]; ok {
		valueCopy := value
		return &valueCopy
	}
	return nil
}

func (self *fakeHeaterConfig) LoadObject(section string) interface{} {
	self.loadObjectCalls = append(self.loadObjectCalls, section)
	return nil
}

func (self *fakeHeaterConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeHeaterConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeHeaterConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func (self *fakeHeaterConfig) HasOption(option string) bool {
	return self.hasOptions[option]
}

func (self *fakeHeaterConfig) LoadSupportConfig(filename string) error {
	self.supportConfigCalls = append(self.supportConfigCalls, filename)
	if err, ok := self.supportConfigErrors[filename]; ok {
		return err
	}
	return nil
}

func (self *fakeHeaterConfig) Deprecate(option string, value string) {
	self.deprecated = append(self.deprecated, option+":"+value)
}

type fakeHeaterCommand struct {
	strings     map[string]string
	floats      map[string]float64
	parameters  map[string]string
	ackMessages []string
	rawMessages []string
}

func (self *fakeHeaterCommand) String(name string, defaultValue string) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeHeaterCommand) Parameters() map[string]string {
	return self.parameters
}

func (self *fakeHeaterCommand) RespondInfo(msg string, log bool) {}

func (self *fakeHeaterCommand) RespondRaw(msg string) {
	self.rawMessages = append(self.rawMessages, msg)
}

func (self *fakeHeaterCommand) Ack(msg string) bool {
	self.ackMessages = append(self.ackMessages, msg)
	return true
}

type fakeBusyControl struct {
	responses []bool
	calls     int
}

func (self *fakeBusyControl) Check_busy(eventtime float64, smoothed_temp float64, target_temp float64) bool {
	if self.calls >= len(self.responses) {
		return false
	}
	response := self.responses[self.calls]
	self.calls++
	return response
}

func newHeaterSectionConfig(printer *fakeHeaterPrinter, name string) *fakeHeaterConfig {
	return &fakeHeaterConfig{
		printer: printer,
		name:    name,
		strings: map[string]string{
			"sensor_type": "test-sensor",
			"heater_pin":  "PA0",
			"control":     "watermark",
		},
		floats: map[string]float64{
			"min_temp":         10.,
			"max_temp":         300.,
			"min_extrude_temp": 170.,
			"max_power":        0.8,
			"smooth_time":      1.0,
			"pwm_cycle_time":   0.1,
			"max_delta":        2.0,
		},
		hasOptions: map[string]bool{
			"sensor_type":      true,
			"heater_pin":       true,
			"control":          true,
			"min_temp":         true,
			"max_temp":         true,
			"min_extrude_temp": true,
			"max_power":        true,
			"smooth_time":      true,
			"pwm_cycle_time":   true,
			"max_delta":        true,
		},
	}
}

func TestLoadConfigHeatersRegistersCommandsAndLoadsSupportConfigOnce(t *testing.T) {
	gcode := &fakeHeaterGCode{}
	reactor := &fakeHeaterReactor{}
	printer := &fakeHeaterPrinter{
		lookup:  map[string]interface{}{},
		gcode:   gcode,
		reactor: reactor,
	}
	baseConfig := &fakeHeaterConfig{
		printer:    printer,
		name:       "heaters",
		hasOptions: map[string]bool{},
		supportConfigErrors: map[string]error{
			"temperature_sensors.cfg":                          os.ErrNotExist,
			filepath.Join("config", "temperature_sensors.cfg"): nil,
		},
	}
	module := LoadConfigHeaters(baseConfig).(*PrinterHeaters)
	if module == nil {
		t.Fatalf("expected heaters module instance")
	}
	if gcode.handlers["TURN_OFF_HEATERS"] == nil || gcode.handlers["M105"] == nil || gcode.handlers["TEMPERATURE_WAIT"] == nil {
		t.Fatalf("expected heater commands to be registered: %#v", gcode.handlers)
	}
	if printer.eventHandlers["project:ready"] == nil || printer.eventHandlers["gcode:request_restart"] == nil {
		t.Fatalf("expected ready and restart handlers to be registered: %#v", printer.eventHandlers)
	}

	module.Add_sensor_factory("test-sensor", printerpkg.TemperatureSensorFactory(func(config printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
		return &fakeHeaterSensor{reportDelta: 0.25}
	}))
	sectionConfig := newHeaterSectionConfig(printer, "extruder")
	module.Setup_sensor(sectionConfig)
	module.Setup_sensor(sectionConfig)

	expectedSupportCalls := []string{"temperature_sensors.cfg", filepath.Join("config", "temperature_sensors.cfg")}
	if !reflect.DeepEqual(baseConfig.supportConfigCalls, expectedSupportCalls) {
		t.Fatalf("unexpected support config load calls: %#v", baseConfig.supportConfigCalls)
	}
}

func TestPrinterHeatersSetupHeaterAndWaitForTemperature(t *testing.T) {
	gcode := &fakeHeaterGCode{}
	reactor := &fakeHeaterReactor{now: 10.0}
	pwm := &fakeHeaterPWMPin{mcu: &fakeHeaterMCU{estimatedPrintTime: 10.0}}
	pins := &fakeHeaterPins{pwm: pwm}
	toolhead := &fakeHeaterToolhead{}
	printer := &fakeHeaterPrinter{
		lookup: map[string]interface{}{
			"pins":     pins,
			"toolhead": toolhead,
		},
		gcode:   gcode,
		reactor: reactor,
	}
	baseConfig := &fakeHeaterConfig{
		printer:    printer,
		name:       "heaters",
		hasOptions: map[string]bool{},
		supportConfigErrors: map[string]error{
			"temperature_sensors.cfg": nil,
		},
	}
	module := NewPrinterHeaters(baseConfig)
	printer.lookup["heaters"] = module
	module.Add_sensor_factory("test-sensor", printerpkg.TemperatureSensorFactory(func(config printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
		return &fakeHeaterSensor{reportDelta: 0.25}
	}))

	sectionConfig := newHeaterSectionConfig(printer, "extruder")
	heater := module.Setup_heater(sectionConfig, "T0")
	if heater == nil {
		t.Fatalf("expected heater instance")
	}
	if module.Lookup_heater("extruder") != heater {
		t.Fatalf("expected lookup to return registered heater")
	}
	if !reflect.DeepEqual(sectionConfig.loadObjectCalls, []string{"verify_heater extruder", "pid_calibrate"}) {
		t.Fatalf("unexpected dependent object load calls: %#v", sectionConfig.loadObjectCalls)
	}
	if !reflect.DeepEqual(pins.setupPins, []string{"PA0"}) {
		t.Fatalf("unexpected PWM setup calls: %#v", pins.setupPins)
	}
	if len(pwm.maxDurations) != 1 || pwm.maxDurations[0] != MAX_HEAT_TIME {
		t.Fatalf("unexpected max duration calls: %#v", pwm.maxDurations)
	}
	if len(pwm.cycleCalls) != 1 || pwm.cycleCalls[0].cycleTime != 0.1 || pwm.cycleCalls[0].hardwarePWM {
		t.Fatalf("unexpected cycle time calls: %#v", pwm.cycleCalls)
	}
	if gcode.muxHandlers["SET_HEATER_TEMPERATURE"]["extruder"] == nil {
		t.Fatalf("expected SET_HEATER_TEMPERATURE mux handler to be registered: %#v", gcode.muxHandlers)
	}
	if !reflect.DeepEqual(module.Available_heaters, []string{"extruder"}) {
		t.Fatalf("unexpected available heaters: %#v", module.Available_heaters)
	}
	if module.Gcode_id_to_sensor["T0"] != heater {
		t.Fatalf("expected gcode sensor registration for T0")
	}

	if err := module.Handle_ready(nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}
	heater.Last_temp_time = 100.
	heater.Smoothed_temp = 215.
	heater.Target_temp = 220.
	m105 := &fakeHeaterCommand{}
	if err := module.Cmd_M105(m105); err != nil {
		t.Fatalf("M105 returned error: %v", err)
	}
	if !reflect.DeepEqual(m105.ackMessages, []string{"T0:215.0 /220.0"}) {
		t.Fatalf("unexpected M105 ack messages: %#v", m105.ackMessages)
	}
	if !reflect.DeepEqual(m105.rawMessages, []string{"T0:215.0 /220.0"}) {
		t.Fatalf("unexpected M105 raw messages: %#v", m105.rawMessages)
	}

	setTempCommand := &fakeHeaterCommand{floats: map[string]float64{"TARGET": 205.0}}
	if err := gcode.muxHandlers["SET_HEATER_TEMPERATURE"]["extruder"](setTempCommand); err != nil {
		t.Fatalf("SET_HEATER_TEMPERATURE returned error: %v", err)
	}
	if heater.Target_temp != 205.0 {
		t.Fatalf("expected target temperature to be updated, got %v", heater.Target_temp)
	}
	if toolhead.lookaheadCallbacks != 1 {
		t.Fatalf("expected one lookahead registration from heater command, got %d", toolhead.lookaheadCallbacks)
	}

	gcode.rawResponses = nil
	toolhead.lastMoveCalls = 0
	reactor.now = 10.0
	reactor.pauseCalls = nil
	heater.Last_temp_time = 100.
	heater.Smoothed_temp = 209.
	heater.Target_temp = 210.
	heater.Control = &fakeBusyControl{responses: []bool{true, false}}
	if err := module.Set_temperature(heater, 210.0, true); err != nil {
		t.Fatalf("Set_temperature returned error: %v", err)
	}
	if toolhead.lookaheadCallbacks != 2 {
		t.Fatalf("expected second lookahead registration from wait path, got %d", toolhead.lookaheadCallbacks)
	}
	if toolhead.lastMoveCalls != 1 {
		t.Fatalf("expected a single toolhead move-time query, got %d", toolhead.lastMoveCalls)
	}
	if !reflect.DeepEqual(reactor.pauseCalls, []float64{11.0}) {
		t.Fatalf("unexpected reactor pause calls: %#v", reactor.pauseCalls)
	}
	if !reflect.DeepEqual(gcode.rawResponses, []string{"T0:209.0 /210.0"}) {
		t.Fatalf("unexpected wait status output: %#v", gcode.rawResponses)
	}
}

func TestPrinterHeatersSetupBedHeaterDefaultsMinExtrudeTempToZero(t *testing.T) {
	gcode := &fakeHeaterGCode{}
	reactor := &fakeHeaterReactor{now: 10.0}
	pwm := &fakeHeaterPWMPin{mcu: &fakeHeaterMCU{estimatedPrintTime: 10.0}}
	pins := &fakeHeaterPins{pwm: pwm}
	printer := &fakeHeaterPrinter{
		lookup: map[string]interface{}{
			"pins":     pins,
			"toolhead": &fakeHeaterToolhead{},
		},
		gcode:   gcode,
		reactor: reactor,
	}
	baseConfig := &fakeHeaterConfig{
		printer:    printer,
		name:       "heaters",
		hasOptions: map[string]bool{},
		supportConfigErrors: map[string]error{
			"temperature_sensors.cfg": nil,
		},
	}
	module := NewPrinterHeaters(baseConfig)
	printer.lookup["heaters"] = module
	module.Add_sensor_factory("test-sensor", printerpkg.TemperatureSensorFactory(func(config printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
		return &fakeHeaterSensor{reportDelta: 0.25}
	}))

	sectionConfig := newHeaterSectionConfig(printer, "heater_bed")
	sectionConfig.floats["max_temp"] = 140.
	delete(sectionConfig.floats, "min_extrude_temp")
	delete(sectionConfig.hasOptions, "min_extrude_temp")

	heater := module.Setup_heater(sectionConfig, "B")
	if heater == nil {
		t.Fatalf("expected heater instance")
	}
	if got := heater.Min_extrude_temp; got != 0 {
		t.Fatalf("bed heater min_extrude_temp = %v, want 0", got)
	}
	if !heater.Can_extrude {
		t.Fatalf("expected non-extruder heater to be immediately extrudable-safe")
	}
	if module.Gcode_id_to_sensor["B"] != heater {
		t.Fatalf("expected bed heater gcode registration")
	}
	if gcode.muxHandlers["SET_HEATER_TEMPERATURE"]["heater_bed"] == nil {
		t.Fatalf("expected SET_HEATER_TEMPERATURE mux handler for heater_bed")
	}
	if !reflect.DeepEqual(sectionConfig.loadObjectCalls, []string{"verify_heater heater_bed", "pid_calibrate"}) {
		t.Fatalf("unexpected dependent object load calls: %#v", sectionConfig.loadObjectCalls)
	}
	if !reflect.DeepEqual(pins.setupPins, []string{"PA0"}) {
		t.Fatalf("unexpected PWM setup calls: %#v", pins.setupPins)
	}
	if len(pwm.cycleCalls) != 1 || pwm.cycleCalls[0].cycleTime != 0.1 {
		t.Fatalf("unexpected cycle time calls: %#v", pwm.cycleCalls)
	}
	if len(pwm.maxDurations) != 1 || pwm.maxDurations[0] != MAX_HEAT_TIME {
		t.Fatalf("unexpected max duration calls: %#v", pwm.maxDurations)
	}
	if !reflect.DeepEqual(module.Available_heaters, []string{"heater_bed"}) {
		t.Fatalf("unexpected available heaters: %#v", module.Available_heaters)
	}
	if module.Lookup_heater("heater_bed") != heater {
		t.Fatalf("expected lookup to return registered bed heater")
	}
	if !reflect.DeepEqual(module.Available_sensors, []string{"heater_bed"}) {
		t.Fatalf("unexpected available sensors: %#v", module.Available_sensors)
	}
	if heater.Max_temp != 140. {
		t.Fatalf("unexpected bed heater max_temp: %v", heater.Max_temp)
	}
	if heater.Min_temp != 10. {
		t.Fatalf("unexpected bed heater min_temp: %v", heater.Min_temp)
	}
	if got := heater.Get_max_power(); got != 0.8 {
		t.Fatalf("unexpected bed heater max power: %v", got)
	}
	if got := heater.Get_smooth_time(); got != 1.0 {
		t.Fatalf("unexpected bed heater smooth time: %v", got)
	}
	if got := heater.Name; got != "heater_bed" {
		t.Fatalf("unexpected heater name: %q", got)
	}
	if got := heater.Sensor.(*fakeHeaterSensor).maxTemp; got != 140. {
		t.Fatalf("sensor max temp = %v, want 140", got)
	}
}
