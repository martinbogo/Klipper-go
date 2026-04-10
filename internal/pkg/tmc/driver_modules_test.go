package tmc

import "testing"

type fakeDriverConfig struct {
	values map[string]interface{}
}

func (self *fakeDriverConfig) Get(option string, default1 interface{}, noteValid bool) interface{} {
	if value, ok := self.values[option]; ok {
		return value
	}
	return default1
}

func (self *fakeDriverConfig) Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	if value, ok := self.values[option]; ok {
		switch typed := value.(type) {
		case float64:
			return typed
		case int:
			return float64(typed)
		case int64:
			return float64(typed)
		}
	}
	switch typed := default1.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func (self *fakeDriverConfig) Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int {
	if value, ok := self.values[option]; ok {
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		}
	}
	switch typed := default1.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (self *fakeDriverConfig) Getboolean(option string, default1 interface{}, noteValid bool) bool {
	if value, ok := self.values[option]; ok {
		if typed, ok := value.(bool); ok {
			return typed
		}
	}
	if typed, ok := default1.(bool); ok {
		return typed
	}
	return false
}

func (self *fakeDriverConfig) Getint64(option string, default1 interface{}, minval, maxval int64, noteValid bool) int64 {
	if value, ok := self.values[option]; ok {
		switch typed := value.(type) {
		case int:
			return int64(typed)
		case int64:
			return typed
		case float64:
			return int64(typed)
		}
	}
	switch typed := default1.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

type fakeRegisterAccess struct {
	fields *FieldHelper
}

func (self *fakeRegisterAccess) Get_fields() *FieldHelper                   { return self.fields }
func (self *fakeRegisterAccess) Get_register(string) (int64, error)         { return 0, nil }
func (self *fakeRegisterAccess) Set_register(string, int64, *float64) error { return nil }

type fakeCommandHelper struct {
	translate func(string, int64) (string, int64)
	status    map[string]interface{}
}

func (self *fakeCommandHelper) SetupRegisterDump(readRegisters []string, readTranslate func(string, int64) (string, int64)) {
	self.translate = readTranslate
}

func (self *fakeCommandHelper) GetPhaseOffset() (*int, int)              { return nil, 0 }
func (self *fakeCommandHelper) GetStatus(float64) map[string]interface{} { return self.status }

type fakeDriverAdapter struct {
	uartCalls        int
	spiCalls         int
	spi2660Calls     int
	virtualPinCalls  int
	stealthchopCalls int
	current2660Calls int
	lastFields       *FieldHelper
	commandHelper    *fakeCommandHelper
}

func newFakeDriverAdapter() *fakeDriverAdapter {
	return &fakeDriverAdapter{commandHelper: &fakeCommandHelper{status: map[string]interface{}{"ok": true}}}
}

func (self *fakeDriverAdapter) NewUART(config DriverConfig, nameToReg map[string]int64, fields *FieldHelper, maxAddr int64, tmcFrequency float64) RegisterAccess {
	self.uartCalls++
	self.lastFields = fields
	return &fakeRegisterAccess{fields: fields}
}

func (self *fakeDriverAdapter) NewSPI(config DriverConfig, nameToReg map[string]int64, fields *FieldHelper) RegisterAccess {
	self.spiCalls++
	self.lastFields = fields
	return &fakeRegisterAccess{fields: fields}
}

func (self *fakeDriverAdapter) NewTMC2660SPI(config DriverConfig, nameToReg map[string]int64, fields *FieldHelper) RegisterAccess {
	self.spi2660Calls++
	self.lastFields = fields
	return &fakeRegisterAccess{fields: fields}
}

func (self *fakeDriverAdapter) AttachVirtualPin(config DriverConfig, mcuTMC RegisterAccess) {
	self.virtualPinCalls++
}

func (self *fakeDriverAdapter) NewCommandHelper(config DriverConfig, mcuTMC RegisterAccess, currentHelper CurrentControl) DriverCommandHelper {
	return self.commandHelper
}

func (self *fakeDriverAdapter) ApplyStealthchop(config DriverConfig, mcuTMC RegisterAccess, tmcFrequency float64) {
	self.stealthchopCalls++
}

func (self *fakeDriverAdapter) NewTMC2660CurrentHelper(config DriverConfig, mcuTMC RegisterAccess) CurrentControl {
	self.current2660Calls++
	return NewTMC2660CurrentHelper(config, mcuTMC, nil, nil)
}

func TestNewTMC2240ChoosesUARTWhenConfigured(t *testing.T) {
	config := &fakeDriverConfig{values: map[string]interface{}{
		"uart_pin":       "PA1",
		"run_current":    0.8,
		"hold_current":   0.5,
		"sense_resistor": 0.11,
		"rref":           12000.0,
	}}
	adapter := newFakeDriverAdapter()
	module := NewTMC2240(config, adapter)

	if adapter.uartCalls != 1 || adapter.spiCalls != 0 {
		t.Fatalf("expected UART transport for TMC2240, got uart=%d spi=%d", adapter.uartCalls, adapter.spiCalls)
	}
	if adapter.virtualPinCalls != 1 {
		t.Fatalf("expected virtual pin helper for TMC2240, got %d", adapter.virtualPinCalls)
	}
	if adapter.stealthchopCalls != 1 {
		t.Fatalf("expected stealthchop helper for TMC2240, got %d", adapter.stealthchopCalls)
	}
	if module.Get_status(0)["ok"] != true {
		t.Fatalf("expected shared driver module to expose command-helper status")
	}
}

func TestNewTMC2240ChoosesSPIWithoutUART(t *testing.T) {
	config := &fakeDriverConfig{values: map[string]interface{}{
		"run_current":    0.8,
		"hold_current":   0.5,
		"sense_resistor": 0.11,
		"rref":           12000.0,
	}}
	adapter := newFakeDriverAdapter()
	NewTMC2240(config, adapter)

	if adapter.spiCalls != 1 || adapter.uartCalls != 0 {
		t.Fatalf("expected SPI transport for TMC2240, got uart=%d spi=%d", adapter.uartCalls, adapter.spiCalls)
	}
}

func TestNewTMC2208RegistersReadTranslate(t *testing.T) {
	config := &fakeDriverConfig{values: map[string]interface{}{
		"run_current":    0.8,
		"hold_current":   0.5,
		"sense_resistor": 0.11,
	}}
	adapter := newFakeDriverAdapter()
	NewTMC2208(config, adapter)

	if adapter.commandHelper.translate == nil {
		t.Fatalf("expected TMC2208 dump translator to be registered")
	}
	val220x := int64(1 << 8)
	regName, translated := adapter.commandHelper.translate("IOIN", val220x)
	if regName != "IOIN@TMC220x" || translated != val220x {
		t.Fatalf("expected TMC220x translation, got %s %#x", regName, translated)
	}
	val222x := int64(0)
	regName, translated = adapter.commandHelper.translate("IOIN", val222x)
	if regName != "IOIN@TMC222x" || translated != val222x {
		t.Fatalf("expected TMC222x translation, got %s %#x", regName, translated)
	}
}

func TestNewTMC2660UsesSpecialCurrentHelper(t *testing.T) {
	config := &fakeDriverConfig{values: map[string]interface{}{
		"run_current":          0.8,
		"sense_resistor":       0.1,
		"idle_current_percent": 50,
	}}
	adapter := newFakeDriverAdapter()
	module := NewTMC2660(config, adapter)

	if adapter.spi2660Calls != 1 {
		t.Fatalf("expected dedicated 2660 SPI transport, got %d", adapter.spi2660Calls)
	}
	if adapter.current2660Calls != 1 {
		t.Fatalf("expected dedicated 2660 current helper, got %d", adapter.current2660Calls)
	}
	if adapter.virtualPinCalls != 0 || adapter.stealthchopCalls != 0 {
		t.Fatalf("expected no virtual pin or stealthchop hooks for 2660, got virtual=%d stealth=%d", adapter.virtualPinCalls, adapter.stealthchopCalls)
	}
	if module.Get_status(0)["ok"] != true {
		t.Fatalf("expected shared driver module to expose command-helper status")
	}
}
