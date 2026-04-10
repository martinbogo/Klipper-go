package heater

import (
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeHeaterBedHeater struct {
	status      map[string]float64
	statsActive bool
	statsText   string
	statusCalls []float64
	statsCalls  []float64
}

func (self *fakeHeaterBedHeater) Get_status(eventtime float64) map[string]float64 {
	self.statusCalls = append(self.statusCalls, eventtime)
	return self.status
}

func (self *fakeHeaterBedHeater) Stats(eventtime float64) (bool, string) {
	self.statsCalls = append(self.statsCalls, eventtime)
	return self.statsActive, self.statsText
}

type fakeHeaterBedManager struct {
	heater     interface{}
	setupCalls []struct {
		name    string
		gcodeID string
	}
	setTempCalls []struct {
		heater interface{}
		temp   float64
		wait   bool
	}
}

func (self *fakeHeaterBedManager) SetupHeater(config printerpkg.ModuleConfig, gcodeID string) interface{} {
	self.setupCalls = append(self.setupCalls, struct {
		name    string
		gcodeID string
	}{name: config.Name(), gcodeID: gcodeID})
	return self.heater
}

func (self *fakeHeaterBedManager) Set_temperature(heater interface{}, temp float64, wait bool) error {
	self.setTempCalls = append(self.setTempCalls, struct {
		heater interface{}
		temp   float64
		wait   bool
	}{heater: heater, temp: temp, wait: wait})
	return nil
}

type fakeHeaterBedGCode struct {
	handlers map[string]func(printerpkg.Command) error
	descs    map[string]string
}

func (self *fakeHeaterBedGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.handlers == nil {
		self.handlers = map[string]func(printerpkg.Command) error{}
		self.descs = map[string]string{}
	}
	self.handlers[cmd] = handler
	self.descs[cmd] = desc
}

func (self *fakeHeaterBedGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakeHeaterBedGCode) RunScriptFromCommand(script string) {}
func (self *fakeHeaterBedGCode) RunScript(script string)            {}
func (self *fakeHeaterBedGCode) IsBusy() bool                       { return false }
func (self *fakeHeaterBedGCode) Mutex() printerpkg.Mutex            { return nil }
func (self *fakeHeaterBedGCode) RespondInfo(msg string, log bool)   {}
func (self *fakeHeaterBedGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeHeaterBedPrinter struct {
	lookup map[string]interface{}
	gcode  printerpkg.GCodeRuntime
}

func (self *fakeHeaterBedPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterBedPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}
func (self *fakeHeaterBedPrinter) SendEvent(event string, params []interface{})      {}
func (self *fakeHeaterBedPrinter) CurrentExtruderName() string                       { return "extruder" }
func (self *fakeHeaterBedPrinter) AddObject(name string, obj interface{}) error      { return nil }
func (self *fakeHeaterBedPrinter) LookupObjects(module string) []interface{}         { return nil }
func (self *fakeHeaterBedPrinter) HasStartArg(name string) bool                      { return false }
func (self *fakeHeaterBedPrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }
func (self *fakeHeaterBedPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}
func (self *fakeHeaterBedPrinter) LookupMCU(name string) printerpkg.MCURuntime    { return nil }
func (self *fakeHeaterBedPrinter) InvokeShutdown(msg string)                      {}
func (self *fakeHeaterBedPrinter) IsShutdown() bool                               { return false }
func (self *fakeHeaterBedPrinter) Reactor() printerpkg.ModuleReactor              { return nil }
func (self *fakeHeaterBedPrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }
func (self *fakeHeaterBedPrinter) GCode() printerpkg.GCodeRuntime                 { return self.gcode }
func (self *fakeHeaterBedPrinter) GCodeMove() printerpkg.MoveTransformController  { return nil }
func (self *fakeHeaterBedPrinter) Webhooks() printerpkg.WebhookRegistry           { return nil }

type fakeHeaterBedConfig struct {
	printer      printerpkg.ModulePrinter
	name         string
	loadedObject []string
}

func (self *fakeHeaterBedConfig) Name() string { return self.name }

func (self *fakeHeaterBedConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeHeaterBedConfig) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeHeaterBedConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeHeaterBedConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeHeaterBedConfig) LoadObject(section string) interface{} {
	self.loadedObject = append(self.loadedObject, section)
	return nil
}

func (self *fakeHeaterBedConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeHeaterBedConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeHeaterBedConfig) Printer() printerpkg.ModulePrinter { return self.printer }

type fakeHeaterBedCommand struct {
	floats map[string]float64
}

func (self *fakeHeaterBedCommand) String(name string, defaultValue string) string {
	return defaultValue
}

func (self *fakeHeaterBedCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeHeaterBedCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeHeaterBedCommand) Parameters() map[string]string    { return nil }
func (self *fakeHeaterBedCommand) RespondInfo(msg string, log bool) {}
func (self *fakeHeaterBedCommand) RespondRaw(msg string)            {}

func TestLoadConfigHeaterBedRegistersCommandsAndControlsTemperature(t *testing.T) {
	heater := &fakeHeaterBedHeater{
		status:      map[string]float64{"temperature": 60, "target": 70, "power": 0.5},
		statsActive: true,
		statsText:   "heater_bed: target=70 temp=60.0 pwm=0.500",
	}
	heaters := &fakeHeaterBedManager{heater: heater}
	gcode := &fakeHeaterBedGCode{}
	printer := &fakeHeaterBedPrinter{
		lookup: map[string]interface{}{"heaters": heaters},
		gcode:  gcode,
	}
	config := &fakeHeaterBedConfig{printer: printer, name: "heater_bed"}

	module := LoadConfigHeaterBed(config).(*HeaterBedModule)
	if module == nil {
		t.Fatalf("expected heater bed module instance")
	}
	if !reflect.DeepEqual(config.loadedObject, []string{"heaters"}) {
		t.Fatalf("unexpected load object calls: %#v", config.loadedObject)
	}
	if len(heaters.setupCalls) != 1 || heaters.setupCalls[0].name != "heater_bed" || heaters.setupCalls[0].gcodeID != "B" {
		t.Fatalf("unexpected setup heater calls: %#v", heaters.setupCalls)
	}
	if gcode.handlers["M140"] == nil || gcode.handlers["M190"] == nil {
		t.Fatalf("expected M140 and M190 handlers to be registered")
	}

	status := module.Get_status(12.5)
	if !reflect.DeepEqual(status, heater.status) {
		t.Fatalf("unexpected bed status: %#v", status)
	}
	active, stats := module.Stats(12.5)
	if !active || stats != heater.statsText {
		t.Fatalf("unexpected bed stats: active=%v stats=%q", active, stats)
	}

	if err := gcode.handlers["M140"](&fakeHeaterBedCommand{floats: map[string]float64{"S": 55}}); err != nil {
		t.Fatalf("M140 returned error: %v", err)
	}
	if err := gcode.handlers["M190"](&fakeHeaterBedCommand{floats: map[string]float64{"S": 65}}); err != nil {
		t.Fatalf("M190 returned error: %v", err)
	}

	if len(heaters.setTempCalls) != 2 {
		t.Fatalf("unexpected set temperature calls: %#v", heaters.setTempCalls)
	}
	if heaters.setTempCalls[0].heater != heater || heaters.setTempCalls[0].temp != 55 || heaters.setTempCalls[0].wait {
		t.Fatalf("unexpected M140 set call: %#v", heaters.setTempCalls[0])
	}
	if heaters.setTempCalls[1].heater != heater || heaters.setTempCalls[1].temp != 65 || !heaters.setTempCalls[1].wait {
		t.Fatalf("unexpected M190 set call: %#v", heaters.setTempCalls[1])
	}
	if !reflect.DeepEqual(heater.statusCalls, []float64{12.5}) {
		t.Fatalf("unexpected status calls: %#v", heater.statusCalls)
	}
	if !reflect.DeepEqual(heater.statsCalls, []float64{12.5}) {
		t.Fatalf("unexpected stats calls: %#v", heater.statsCalls)
	}
}
