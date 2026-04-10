package tmc

import "testing"

type fakeCurrentRegisterAccess struct {
	fields *FieldHelper
	writes []string
}

func (self *fakeCurrentRegisterAccess) Get_fields() *FieldHelper {
	return self.fields
}

func (self *fakeCurrentRegisterAccess) Get_register(string) (int64, error) {
	return 0, nil
}

func (self *fakeCurrentRegisterAccess) Set_register(regName string, val int64, printTime *float64) error {
	_ = val
	_ = printTime
	self.writes = append(self.writes, regName)
	return nil
}

type fakeCurrentConfig struct {
	floats map[string]float64
	ints   map[string]int
}

func (self *fakeCurrentConfig) Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	_ = minval
	_ = maxval
	_ = above
	_ = below
	_ = noteValid
	if value, ok := self.floats[option]; ok {
		return value
	}
	return default1.(float64)
}

func (self *fakeCurrentConfig) Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int {
	_ = minval
	_ = maxval
	_ = noteValid
	if value, ok := self.ints[option]; ok {
		return value
	}
	return default1.(int)
}

func TestTMC2660CurrentHelperRegistersIdleCallbacks(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"SGCSCONF": {"cs": 0x1f},
		"DRVCONF":  {"vsense": 0x01 << 6},
	}, nil, nil, nil)
	access := &fakeCurrentRegisterAccess{fields: fields}
	config := &fakeCurrentConfig{
		floats: map[string]float64{"run_current": 1.2, "sense_resistor": 0.1},
		ints:   map[string]int{"idle_current_percent": 50},
	}
	registered := map[string]func([]interface{}) error{}
	scheduled := 0
	helper := NewTMC2660CurrentHelper(config, access,
		func(event string, callback func([]interface{}) error) {
			registered[event] = callback
		},
		func(callback func(interface{}) interface{}, eventtime float64) {
			if eventtime != 0 {
				t.Fatalf("expected immediate scheduling, got %f", eventtime)
			}
			scheduled++
			callback(nil)
		})

	if helper.Get_current()[3] != TMC2660MaxCurrent {
		t.Fatalf("expected TMC2660 max current to be preserved, got %f", helper.Get_current()[3])
	}
	if registered["idle_timeout:printing"] == nil || registered["idle_timeout:ready"] == nil {
		t.Fatalf("expected idle timeout handlers to be registered, got %#v", registered)
	}
	if err := registered["idle_timeout:ready"]([]interface{}{1.0}); err != nil {
		t.Fatalf("ready callback returned error: %v", err)
	}
	if err := registered["idle_timeout:printing"]([]interface{}{1.0}); err != nil {
		t.Fatalf("printing callback returned error: %v", err)
	}
	if scheduled != 2 {
		t.Fatalf("expected two scheduled callbacks, got %d", scheduled)
	}
	if len(access.writes) == 0 {
		t.Fatalf("expected current update to write driver registers")
	}
}