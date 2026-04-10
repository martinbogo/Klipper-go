package tmc

import (
	"math"
	"testing"
)

type fakeCarrier struct {
	fields *FieldHelper
}

func (self *fakeCarrier) Get_fields() *FieldHelper {
	return self.fields
}

func TestApplyWaveTableDefaults(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"MSLUT0":     {"mslut0": 0xffffffff},
		"MSLUT1":     {"mslut1": 0xffffffff},
		"MSLUT2":     {"mslut2": 0xffffffff},
		"MSLUT3":     {"mslut3": 0xffffffff},
		"MSLUT4":     {"mslut4": 0xffffffff},
		"MSLUT5":     {"mslut5": 0xffffffff},
		"MSLUT6":     {"mslut6": 0xffffffff},
		"MSLUT7":     {"mslut7": 0xffffffff},
		"MSLUTSEL":   {"w0": 0x03 << 0, "w1": 0x03 << 2, "w2": 0x03 << 4, "w3": 0x03 << 6, "x1": 0xff << 8, "x2": 0xff << 16, "x3": 0xff << 24},
		"MSLUTSTART": {"start_sin": 0xff << 0, "start_sin90": 0xff << 16},
	}, nil, nil, nil)
	carrier := &fakeCarrier{fields: fields}
	config := &fakeFieldConfig{bools: map[string]bool{}, ints: map[string]int{}, int64s: map[string]int64{}}

	ApplyWaveTableDefaults(config, carrier)

	if got := fields.Get_field("mslut0", nil, nil); got != int64(0xAAAAB554) {
		t.Fatalf("expected default wavetable to populate mslut0, got %#x", got)
	}
	if got := fields.Get_field("start_sin90", nil, nil); got != 247 {
		t.Fatalf("expected default start_sin90 to be applied, got %d", got)
	}
}

func TestApplyMicrostepSettings(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{"REG": {"mres": 0xf << 0, "intpol": 1 << 8}}, nil, nil, nil)
	ApplyMicrostepSettings(fields, 4, true)
	if got := fields.Get_field("mres", nil, nil); got != 4 {
		t.Fatalf("expected mres to be set, got %d", got)
	}
	if got := fields.Get_field("intpol", nil, nil); got != 1 {
		t.Fatalf("expected intpol to be enabled, got %d", got)
	}
}

func TestTMCtstepHelperClampsAndStealthchopAppliesThreshold(t *testing.T) {
	if got := TMCtstepHelper(0.01, 4, 12000000, -1); got != 0xfffff {
		t.Fatalf("expected non-positive velocity to clamp to max threshold, got %d", got)
	}

	fields := NewFieldHelper(map[string]map[string]int64{"REG": {"mres": 0xf << 0, "tpwmthrs": 0xfffff << 4, "en_pwm_mode": 1 << 24}}, nil, nil, nil)
	fields.Set_field("mres", 4, nil, nil)
	ApplyStealthchop(fields, 12000000, 0.01, 5.0)

	if got := fields.Get_field("en_pwm_mode", nil, nil); got != 1 {
		t.Fatalf("expected stealthchop helper to enable pwm mode, got %d", got)
	}
	if got := fields.Get_field("tpwmthrs", nil, nil); got <= 0 || got >= int64(math.MaxInt32) {
		t.Fatalf("expected finite pwm threshold, got %d", got)
	}
}
