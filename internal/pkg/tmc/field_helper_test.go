package tmc

import (
	"goklipper/common/utils/cast"
	"testing"
)

type fakeFieldConfig struct {
	bools  map[string]bool
	ints   map[string]int
	int64s map[string]int64
}

func (self *fakeFieldConfig) Getboolean(option string, default1 interface{}, noteValid bool) bool {
	_ = noteValid
	if value, ok := self.bools[option]; ok {
		return value
	}
	return cast.ToBool(default1)
}

func (self *fakeFieldConfig) Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int {
	_ = minval
	_ = maxval
	_ = noteValid
	if value, ok := self.ints[option]; ok {
		return value
	}
	return cast.ToInt(default1)
}

func (self *fakeFieldConfig) Getint64(option string, default1 interface{}, minval, maxval int64, noteValid bool) int64 {
	_ = minval
	_ = maxval
	_ = noteValid
	if value, ok := self.int64s[option]; ok {
		return value
	}
	return cast.ToInt64(default1)
}

func TestFieldHelperGetSetAndLookup(t *testing.T) {
	helper := NewFieldHelper(
		map[string]map[string]int64{"REG": {"flag": 1 << 0, "bits": 0x7 << 4}},
		nil,
		nil,
		nil,
	)

	helper.Set_field("flag", true, nil, nil)
	helper.Set_field("bits", 5, nil, nil)

	if got := helper.Lookup_register("flag", nil); got != "REG" {
		t.Fatalf("expected field lookup to resolve REG, got %#v", got)
	}
	if got := helper.Get_field("flag", nil, nil); got != 1 {
		t.Fatalf("expected flag bit to be set, got %d", got)
	}
	if got := helper.Get_field("bits", nil, nil); got != 5 {
		t.Fatalf("expected packed bits to round-trip, got %d", got)
	}
}

func TestFieldHelperSetConfigFieldUsesTypedAccessors(t *testing.T) {
	helper := NewFieldHelper(
		map[string]map[string]int64{"REG": {"bool_field": 1 << 0, "int_field": 0xf << 4, "long_field": 0xff << 8}},
		nil,
		nil,
		nil,
	)
	config := &fakeFieldConfig{
		bools:  map[string]bool{"driver_BOOL_FIELD": true},
		ints:   map[string]int{"driver_INT_FIELD": 7},
		int64s: map[string]int64{"driver_LONG_FIELD": 12},
	}

	helper.Set_config_field(config, "bool_field", false)
	helper.Set_config_field(config, "int_field", 0)
	helper.Set_config_field(config, "long_field", int64(0))

	if got := helper.Get_field("bool_field", nil, nil); got != 1 {
		t.Fatalf("expected bool config field to be applied, got %d", got)
	}
	if got := helper.Get_field("int_field", nil, nil); got != 7 {
		t.Fatalf("expected int config field to be applied, got %d", got)
	}
	if got := helper.Get_field("long_field", nil, nil); got != 12 {
		t.Fatalf("expected int64 config field to be applied, got %d", got)
	}
}

func TestFieldHelperPrettyFormatAndAccessors(t *testing.T) {
	helper := NewFieldHelper(
		map[string]map[string]int64{"REG": {"flag": 1 << 0}},
		nil,
		map[string]func(interface{}) string{"flag": func(v interface{}) string { return "1(enabled)" }},
		nil,
	)
	helper.Set_field("flag", 1, nil, nil)

	formatted := helper.Pretty_format("REG", helper.Registers()["REG"])
	if formatted == "" || helper.All_fields()["REG"]["flag"] == 0 {
		t.Fatalf("expected formatted output and field metadata, got %q", formatted)
	}
	if fields := helper.Get_reg_fields("REG", helper.Registers()["REG"]); fields["flag"] != 1 {
		t.Fatalf("expected register field snapshot to include enabled flag, got %#v", fields)
	}
}

func TestFieldHelperTracksRegisterFirstTouchOrder(t *testing.T) {
	helper := NewFieldHelper(
		map[string]map[string]int64{
			"REG_A": {"a": 1 << 0},
			"REG_B": {"b": 1 << 0},
			"REG_C": {"c": 1 << 0},
		},
		nil,
		nil,
		nil,
	)

	helper.Set_field("b", 1, nil, nil)
	helper.Set_field("a", 1, nil, nil)
	helper.Set_field("b", 0, nil, nil)
	helper.Set_field("c", 1, nil, nil)

	got := helper.orderedRegisterNames()
	want := []string{"REG_B", "REG_A", "REG_C"}
	if len(got) != len(want) {
		t.Fatalf("orderedRegisterNames length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("orderedRegisterNames[%d] = %q, want %q (%#v)", i, got[i], want[i], got)
		}
	}
}
