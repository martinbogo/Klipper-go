package heater

import (
	"fmt"
	"strings"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeDS18SensorRegistry struct {
	sensorType string
	factory    printerpkg.TemperatureSensorFactory
}

func (self *fakeDS18SensorRegistry) AddSensorFactory(sensorType string, factory printerpkg.TemperatureSensorFactory) {
	self.sensorType = sensorType
	self.factory = factory
}

type fakeDS18MCU struct {
	nextOID          int
	configCallback   func()
	responseCallback func(map[string]interface{}) error
	responseMsg      string
	responseOID      interface{}
	configCmds       []string
	querySlot        int64
	clockScale       int64
	clockOffset      int64
}

func (self *fakeDS18MCU) CreateOID() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeDS18MCU) RegisterConfigCallback(cb func()) {
	self.configCallback = cb
}

func (self *fakeDS18MCU) AddConfigCmd(cmd string, isInit bool, onRestart bool) {
	self.configCmds = append(self.configCmds, cmd)
}

func (self *fakeDS18MCU) GetQuerySlot(oid int) int64 {
	return self.querySlot
}

func (self *fakeDS18MCU) SecondsToClock(time float64) int64 {
	return int64(time * float64(self.clockScale))
}

func (self *fakeDS18MCU) RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{}) {
	self.responseCallback = cb
	self.responseMsg = msg
	self.responseOID = oid
}

func (self *fakeDS18MCU) ClockToPrintTime(clock int64) float64 {
	return float64(clock) / 100.0
}

func (self *fakeDS18MCU) Clock32ToClock64(clock32 int64) int64 {
	return clock32 + self.clockOffset
}

type fakeDS18Printer struct {
	registry      printerpkg.TemperatureSensorRegistry
	mcu           printerpkg.MCURuntime
	lookupMCUName string
}

func (self *fakeDS18Printer) LookupObject(name string, defaultValue interface{}) interface{} {
	return defaultValue
}

func (self *fakeDS18Printer) RegisterEventHandler(event string, callback func([]interface{}) error) {}

func (self *fakeDS18Printer) SendEvent(event string, params []interface{}) {}

func (self *fakeDS18Printer) CurrentExtruderName() string { return "extruder" }

func (self *fakeDS18Printer) AddObject(name string, obj interface{}) error { return nil }

func (self *fakeDS18Printer) LookupObjects(module string) []interface{} { return nil }

func (self *fakeDS18Printer) HasStartArg(name string) bool { return false }

func (self *fakeDS18Printer) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeDS18Printer) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return self.registry
}

func (self *fakeDS18Printer) LookupMCU(name string) printerpkg.MCURuntime {
	self.lookupMCUName = name
	return self.mcu
}

func (self *fakeDS18Printer) InvokeShutdown(msg string) {}

func (self *fakeDS18Printer) IsShutdown() bool { return false }

func (self *fakeDS18Printer) Reactor() printerpkg.ModuleReactor { return nil }

func (self *fakeDS18Printer) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeDS18Printer) GCode() printerpkg.GCodeRuntime { return nil }

func (self *fakeDS18Printer) GCodeMove() printerpkg.MoveTransformController { return nil }

func (self *fakeDS18Printer) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeDS18Config struct {
	printer printerpkg.ModulePrinter
	name    string
	strings map[string]string
	floats  map[string]float64
}

func (self *fakeDS18Config) Name() string { return self.name }

func (self *fakeDS18Config) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeDS18Config) Bool(option string, defaultValue bool) bool { return defaultValue }

func (self *fakeDS18Config) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeDS18Config) OptionalFloat(option string) *float64 { return nil }

func (self *fakeDS18Config) LoadObject(section string) interface{} { return nil }

func (self *fakeDS18Config) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeDS18Config) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeDS18Config) Printer() printerpkg.ModulePrinter { return self.printer }

func TestLoadConfigDS18B20RegistersFactoryAndBuildsSensor(t *testing.T) {
	registry := &fakeDS18SensorRegistry{}
	mcu := &fakeDS18MCU{querySlot: 321, clockScale: 1000, clockOffset: 1000}
	printer := &fakeDS18Printer{registry: registry, mcu: mcu}

	if got := LoadConfigDS18B20(&fakeDS18Config{printer: printer, name: "ds18b20"}); got != nil {
		t.Fatalf("expected nil module load result, got %#v", got)
	}
	if registry.sensorType != "DS18B20" {
		t.Fatalf("expected DS18B20 factory registration, got %q", registry.sensorType)
	}
	if registry.factory == nil {
		t.Fatalf("expected sensor factory to be registered")
	}

	sensor := registry.factory(&fakeDS18Config{
		printer: printer,
		name:    "heater_bed",
		strings: map[string]string{"serial_no": "abc123", "sensor_mcu": "tool"},
		floats:  map[string]float64{"ds18_report_time": 5.0},
	})
	ds18, ok := sensor.(*DS18B20Sensor)
	if !ok {
		t.Fatalf("expected DS18B20Sensor, got %T", sensor)
	}
	if printer.lookupMCUName != "tool" {
		t.Fatalf("expected MCU lookup for tool, got %q", printer.lookupMCUName)
	}
	if mcu.responseMsg != "ds18b20_result" {
		t.Fatalf("expected ds18b20_result response registration, got %q", mcu.responseMsg)
	}
	if mcu.responseOID != 0 {
		t.Fatalf("expected response registration on oid 0, got %#v", mcu.responseOID)
	}
	if mcu.configCallback == nil {
		t.Fatalf("expected config callback to be registered")
	}

	ds18.SetupMinMax(10.0, 20.0)
	var callbackTime float64
	var callbackTemp float64
	ds18.SetupCallback(func(readTime float64, temp float64) {
		callbackTime = readTime
		callbackTemp = temp
	})

	mcu.configCallback()
	if len(mcu.configCmds) != 2 {
		t.Fatalf("expected two config commands, got %#v", mcu.configCmds)
	}
	if want := "config_ds18b20 oid=0 serial=616263313233 max_error_count=4"; mcu.configCmds[0] != want {
		t.Fatalf("unexpected config command: %q", mcu.configCmds[0])
	}
	if want := "query_ds18b20 oid=0 clock=321 rest_ticks=5000 min_value=10000 max_value=20000"; mcu.configCmds[1] != want {
		t.Fatalf("unexpected query command: %q", mcu.configCmds[1])
	}

	if err := mcu.responseCallback(map[string]interface{}{"value": 25000.0, "next_clock": int64(6000)}); err != nil {
		t.Fatalf("response callback returned error: %v", err)
	}
	if callbackTemp != 25.0 {
		t.Fatalf("expected callback temp 25.0, got %v", callbackTemp)
	}
	if callbackTime != 20.0 {
		t.Fatalf("expected callback time 20.0, got %v", callbackTime)
	}

	if err := mcu.responseCallback(map[string]interface{}{"value": 12000.0, "next_clock": int64(7000), "fault": 1}); err != nil {
		t.Fatalf("fault response returned error: %v", err)
	}
	if callbackTemp != 25.0 || callbackTime != 20.0 {
		t.Fatalf("fault response should not invoke callback again, got time=%v temp=%v", callbackTime, callbackTemp)
	}
	if ds18.temp != 25.0 {
		t.Fatalf("expected stored sensor temp of 25.0, got %v", ds18.temp)
	}
}

func TestNewDS18B20SensorRejectsShortReportTime(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected short report time to panic")
		}
		msg := fmt.Sprint(recovered)
		if !strings.Contains(msg, "ds18_report_time") {
			t.Fatalf("unexpected panic message: %q", msg)
		}
	}()

	printer := &fakeDS18Printer{registry: &fakeDS18SensorRegistry{}, mcu: &fakeDS18MCU{clockScale: 1000}}
	NewDS18B20Sensor(&fakeDS18Config{
		printer: printer,
		name:    "heater_bed",
		strings: map[string]string{"serial_no": "abc123"},
		floats:  map[string]float64{"ds18_report_time": 0.5},
	})
}
