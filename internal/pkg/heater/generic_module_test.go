package heater

import (
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeGenericHeaterManager struct {
	setupCalls []string
	returned   interface{}
}

func (self *fakeGenericHeaterManager) SetupHeater(config printerpkg.ModuleConfig, gcodeID string) interface{} {
	self.setupCalls = append(self.setupCalls, config.Name()+"|"+gcodeID)
	return self.returned
}

type fakeGenericHeaterPrinter struct {
	lookup map[string]interface{}
}

func (self *fakeGenericHeaterPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeGenericHeaterPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}

func (self *fakeGenericHeaterPrinter) SendEvent(event string, params []interface{}) {}
func (self *fakeGenericHeaterPrinter) CurrentExtruderName() string                  { return "extruder" }
func (self *fakeGenericHeaterPrinter) AddObject(name string, obj interface{}) error { return nil }
func (self *fakeGenericHeaterPrinter) LookupObjects(module string) []interface{}    { return nil }
func (self *fakeGenericHeaterPrinter) HasStartArg(name string) bool                 { return false }
func (self *fakeGenericHeaterPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}
func (self *fakeGenericHeaterPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}
func (self *fakeGenericHeaterPrinter) LookupMCU(name string) printerpkg.MCURuntime   { return nil }
func (self *fakeGenericHeaterPrinter) InvokeShutdown(msg string)                      {}
func (self *fakeGenericHeaterPrinter) IsShutdown() bool                               { return false }
func (self *fakeGenericHeaterPrinter) Reactor() printerpkg.ModuleReactor              { return nil }
func (self *fakeGenericHeaterPrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }
func (self *fakeGenericHeaterPrinter) GCode() printerpkg.GCodeRuntime                 { return nil }
func (self *fakeGenericHeaterPrinter) GCodeMove() printerpkg.MoveTransformController  { return nil }
func (self *fakeGenericHeaterPrinter) Webhooks() printerpkg.WebhookRegistry           { return nil }

type fakeGenericHeaterConfig struct {
	printer       printerpkg.ModulePrinter
	name          string
	loadedObjects []string
}

func (self *fakeGenericHeaterConfig) Name() string { return self.name }
func (self *fakeGenericHeaterConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}
func (self *fakeGenericHeaterConfig) Bool(option string, defaultValue bool) bool { return defaultValue }
func (self *fakeGenericHeaterConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}
func (self *fakeGenericHeaterConfig) OptionalFloat(option string) *float64 { return nil }
func (self *fakeGenericHeaterConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	return nil
}
func (self *fakeGenericHeaterConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}
func (self *fakeGenericHeaterConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}
func (self *fakeGenericHeaterConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigGenericHeaterUsesSharedHeaterManager(t *testing.T) {
	returned := struct{ name string }{name: "heater"}
	manager := &fakeGenericHeaterManager{returned: returned}
	config := &fakeGenericHeaterConfig{
		printer: &fakeGenericHeaterPrinter{lookup: map[string]interface{}{"heaters": manager}},
		name:    "heater_generic chamber",
	}

	result := LoadConfigGenericHeater(config)
	if result != returned {
		t.Fatalf("unexpected generic heater result: %#v", result)
	}
	if len(config.loadedObjects) != 1 || config.loadedObjects[0] != "heaters" {
		t.Fatalf("unexpected loaded objects: %#v", config.loadedObjects)
	}
	if len(manager.setupCalls) != 1 || manager.setupCalls[0] != "heater_generic chamber|" {
		t.Fatalf("unexpected heater setup calls: %#v", manager.setupCalls)
	}
}