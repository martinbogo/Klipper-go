package heater

import (
	"strings"
	"testing"

	"goklipper/common/constants"
	printerpkg "goklipper/internal/pkg/printer"
)

type fakeVerifyTimer struct {
	updated []float64
}

func (self *fakeVerifyTimer) Update(waketime float64) {
	self.updated = append(self.updated, waketime)
}

type fakeVerifyReactor struct {
	waketime float64
	timer    *fakeVerifyTimer
}

func (self *fakeVerifyReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	self.waketime = waketime
	return self.timer
}

func (self *fakeVerifyReactor) Monotonic() float64 {
	return 0
}

type fakeVerifyHeater struct {
	temp   float64
	target float64
	reads  []float64
}

func (self *fakeVerifyHeater) GetTemperature(eventtime float64) (float64, float64) {
	self.reads = append(self.reads, eventtime)
	return self.temp, self.target
}

type fakeVerifyPrinter struct {
	reactor       printerpkg.ModuleReactor
	heater        printerpkg.HeaterRuntime
	startArgs     map[string]bool
	eventHandlers map[string]func([]interface{}) error
	lookupName    string
	shutdownMsg   string
}

func (self *fakeVerifyPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	return defaultValue
}

func (self *fakeVerifyPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if self.eventHandlers == nil {
		self.eventHandlers = map[string]func([]interface{}) error{}
	}
	self.eventHandlers[event] = callback
}

func (self *fakeVerifyPrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeVerifyPrinter) CurrentExtruderName() string {
	return "extruder"
}

func (self *fakeVerifyPrinter) AddObject(name string, obj interface{}) error {
	return nil
}

func (self *fakeVerifyPrinter) LookupObjects(module string) []interface{} {
	return nil
}

func (self *fakeVerifyPrinter) HasStartArg(name string) bool {
	return self.startArgs[name]
}

func (self *fakeVerifyPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	self.lookupName = name
	return self.heater
}

func (self *fakeVerifyPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeVerifyPrinter) LookupMCU(name string) printerpkg.MCURuntime {
	return nil
}

func (self *fakeVerifyPrinter) InvokeShutdown(msg string) {
	self.shutdownMsg = msg
}

func (self *fakeVerifyPrinter) IsShutdown() bool {
	return false
}

func (self *fakeVerifyPrinter) Reactor() printerpkg.ModuleReactor {
	return self.reactor
}

func (self *fakeVerifyPrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return nil
}

func (self *fakeVerifyPrinter) GCode() printerpkg.GCodeRuntime {
	return nil
}

func (self *fakeVerifyPrinter) GCodeMove() printerpkg.MoveTransformController {
	return nil
}

func (self *fakeVerifyPrinter) Webhooks() printerpkg.WebhookRegistry {
	return nil
}

type fakeVerifyConfig struct {
	printer printerpkg.ModulePrinter
	name    string
	values  map[string]float64
}

func (self *fakeVerifyConfig) Name() string {
	return self.name
}

func (self *fakeVerifyConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeVerifyConfig) Bool(option string, defaultValue bool) bool {
	return defaultValue
}

func (self *fakeVerifyConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.values[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeVerifyConfig) OptionalFloat(option string) *float64 {
	return nil
}

func (self *fakeVerifyConfig) LoadObject(section string) interface{} {
	return nil
}

func (self *fakeVerifyConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeVerifyConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeVerifyConfig) Printer() printerpkg.ModulePrinter {
	return self.printer
}

func TestLoadConfigVerifyHeaterRegistersHandlersAndDefaults(t *testing.T) {
	printer := &fakeVerifyPrinter{reactor: &fakeVerifyReactor{timer: &fakeVerifyTimer{}}, heater: &fakeVerifyHeater{temp: 25, target: 0}}
	module := LoadConfigVerifyHeater(&fakeVerifyConfig{printer: printer, name: "verify_heater heater_bed"}).(*VerifyHeaterModule)
	if module.core.checkGainTime != 60.0 {
		t.Fatalf("expected heater_bed default gain time of 60, got %v", module.core.checkGainTime)
	}
	if _, ok := printer.eventHandlers["project:connect"]; !ok {
		t.Fatalf("expected connect handler to be registered")
	}
	if _, ok := printer.eventHandlers["project:shutdown"]; !ok {
		t.Fatalf("expected shutdown handler to be registered")
	}
	if err := module.handleConnect(nil); err != nil {
		t.Fatalf("handleConnect returned error: %v", err)
	}
	if printer.lookupName != "heater_bed" {
		t.Fatalf("expected heater lookup for heater_bed, got %q", printer.lookupName)
	}
	if module.checkTimer == nil {
		t.Fatalf("expected timer to be registered")
	}
	if reactor := printer.reactor.(*fakeVerifyReactor); reactor.waketime != constants.NOW {
		t.Fatalf("expected timer waketime NOW, got %v", reactor.waketime)
	}
	if err := module.handleShutdown(nil); err != nil {
		t.Fatalf("handleShutdown returned error: %v", err)
	}
	if updates := printer.reactor.(*fakeVerifyReactor).timer.updated; len(updates) != 1 || updates[0] != constants.NEVER {
		t.Fatalf("expected timer to be disabled on shutdown, got %#v", updates)
	}
}

func TestVerifyHeaterDebugOutputSkipsTimer(t *testing.T) {
	printer := &fakeVerifyPrinter{
		reactor:   &fakeVerifyReactor{timer: &fakeVerifyTimer{}},
		heater:    &fakeVerifyHeater{temp: 25, target: 0},
		startArgs: map[string]bool{"debugoutput": true},
	}
	module := LoadConfigVerifyHeater(&fakeVerifyConfig{printer: printer, name: "verify_heater extruder"}).(*VerifyHeaterModule)
	if err := module.handleConnect(nil); err != nil {
		t.Fatalf("handleConnect returned error: %v", err)
	}
	if module.checkTimer != nil {
		t.Fatalf("expected no timer when debugoutput is active")
	}
	if printer.lookupName != "" {
		t.Fatalf("expected no heater lookup in debugoutput mode, got %q", printer.lookupName)
	}
}

func TestVerifyHeaterFaultTriggersShutdown(t *testing.T) {
	printer := &fakeVerifyPrinter{reactor: &fakeVerifyReactor{timer: &fakeVerifyTimer{}}, heater: &fakeVerifyHeater{temp: 0, target: 50}}
	module := LoadConfigVerifyHeater(&fakeVerifyConfig{
		printer: printer,
		name:    "verify_heater extruder",
		values: map[string]float64{
			"max_error":       1,
			"hysteresis":      0,
			"heating_gain":    2,
			"check_gain_time": 5,
		},
	}).(*VerifyHeaterModule)
	if err := module.handleConnect(nil); err != nil {
		t.Fatalf("handleConnect returned error: %v", err)
	}
	module.core.lastTarget = 50
	module.core.errorValue = 1
	next := module.checkEvent(10)
	if next != constants.NEVER {
		t.Fatalf("expected fault path to stop timer, got %v", next)
	}
	if !strings.Contains(printer.shutdownMsg, "Heater extruder not heating at expected rate") {
		t.Fatalf("unexpected shutdown message: %q", printer.shutdownMsg)
	}
	if !strings.Contains(printer.shutdownMsg, HintThermal) {
		t.Fatalf("expected thermal hint in shutdown message: %q", printer.shutdownMsg)
	}
	if len(printer.heater.(*fakeVerifyHeater).reads) != 1 || printer.heater.(*fakeVerifyHeater).reads[0] != 10 {
		t.Fatalf("expected heater read at eventtime 10, got %#v", printer.heater.(*fakeVerifyHeater).reads)
	}
}
