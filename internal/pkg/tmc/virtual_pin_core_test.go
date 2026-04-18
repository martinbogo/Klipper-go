package tmc

import "testing"

type fakeVirtualPinConfig struct {
	name string
	data map[string]interface{}
}

func (self *fakeVirtualPinConfig) Get_name() string {
	return self.name
}

func (self *fakeVirtualPinConfig) Get(option string, default1 interface{}, noteValid bool) interface{} {
	_ = noteValid
	if value, ok := self.data[option]; ok {
		return value
	}
	return default1
}

type fakeVirtualPinWrite struct {
	regName   string
	value     int64
	printTime *float64
}

type fakeVirtualPinRegisterAccess struct {
	fields *FieldHelper
	writes []fakeVirtualPinWrite
}

func (self *fakeVirtualPinRegisterAccess) Get_fields() *FieldHelper {
	return self.fields
}

func (self *fakeVirtualPinRegisterAccess) Get_register(string) (int64, error) {
	return 0, nil
}

func (self *fakeVirtualPinRegisterAccess) Set_register(regName string, value int64, printTime *float64) error {
	var copiedPrintTime *float64
	if printTime != nil {
		valueCopy := *printTime
		copiedPrintTime = &valueCopy
	}
	self.writes = append(self.writes, fakeVirtualPinWrite{regName: regName, value: value, printTime: copiedPrintTime})
	return nil
}

func TestVirtualPinHelperCoreBeginAndEndHomingRestoreSpreadcycleState(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"GCONF":     {"en_spreadcycle": 1 << 0},
		"TPWMTHRS":  {"tpwmthrs": 0xfffff},
		"TCOOLTHRS": {"tcoolthrs": 0xfffff},
	}, nil, nil, nil)
	fields.Set_field("en_spreadcycle", 1, nil, nil)
	fields.Set_field("tpwmthrs", 321, nil, nil)
	fields.Set_field("tcoolthrs", 0, nil, nil)
	access := &fakeVirtualPinRegisterAccess{fields: fields}
	core := NewVirtualPinHelperCore(&fakeVirtualPinConfig{
		name: "tmc2208 stepper_x",
		data: map[string]interface{}{"diag_pin": "PA1"},
	}, access)

	if err := core.BeginHoming(access, 500); err != nil {
		t.Fatalf("BeginHoming returned error: %v", err)
	}
	if len(access.writes) != 3 {
		t.Fatalf("expected 3 begin writes, got %#v", access.writes)
	}
	if access.writes[0].regName != "TPWMTHRS" || access.writes[0].value != 0 {
		t.Fatalf("unexpected TPWMTHRS begin write %#v", access.writes[0])
	}
	if access.writes[1].regName != "GCONF" || access.writes[1].value != 0 {
		t.Fatalf("unexpected GCONF begin write %#v", access.writes[1])
	}
	if access.writes[2].regName != "TCOOLTHRS" || access.writes[2].value != 500 {
		t.Fatalf("unexpected TCOOLTHRS begin write %#v", access.writes[2])
	}
	if got := fields.Get_field("en_spreadcycle", nil, nil); got != 0 {
		t.Fatalf("expected en_spreadcycle disabled during homing, got %d", got)
	}
	if got := fields.Get_field("tpwmthrs", nil, nil); got != 0 {
		t.Fatalf("expected tpwmthrs cleared during homing, got %d", got)
	}
	if got := fields.Get_field("tcoolthrs", nil, nil); got != 500 {
		t.Fatalf("expected tcoolthrs set to homing threshold, got %d", got)
	}

	if err := core.EndHoming(access, nil); err != nil {
		t.Fatalf("EndHoming returned error: %v", err)
	}
	if len(access.writes) != 6 {
		t.Fatalf("expected 6 total writes after restore, got %#v", access.writes)
	}
	if access.writes[3].regName != "TPWMTHRS" || access.writes[3].value != 321 {
		t.Fatalf("unexpected TPWMTHRS restore write %#v", access.writes[3])
	}
	if access.writes[4].regName != "GCONF" || access.writes[4].value != 1 {
		t.Fatalf("unexpected GCONF restore write %#v", access.writes[4])
	}
	if access.writes[5].regName != "TCOOLTHRS" || access.writes[5].value != 0 {
		t.Fatalf("unexpected TCOOLTHRS restore write %#v", access.writes[5])
	}
	if got := fields.Get_field("en_spreadcycle", nil, nil); got != 1 {
		t.Fatalf("expected en_spreadcycle restored to 1, got %d", got)
	}
	if got := fields.Get_field("tpwmthrs", nil, nil); got != 321 {
		t.Fatalf("expected tpwmthrs restored to 321, got %d", got)
	}
	if got := fields.Get_field("tcoolthrs", nil, nil); got != 0 {
		t.Fatalf("expected tcoolthrs restored to 0, got %d", got)
	}
}

func TestVirtualPinHelperCoreBeginMoveHomingRestoresOriginalThresholds(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"GCONF":     {"en_pwm_mode": 1 << 0, "diag0_stall": 1 << 1},
		"TPWMTHRS":  {"tpwmthrs": 0xfffff},
		"TCOOLTHRS": {"tcoolthrs": 0xfffff},
		"THIGH":     {"thigh": 0xfffff},
	}, nil, nil, nil)
	fields.Set_field("en_pwm_mode", 1, nil, nil)
	fields.Set_field("diag0_stall", 0, nil, nil)
	fields.Set_field("tpwmthrs", 320, nil, nil)
	fields.Set_field("tcoolthrs", 654, nil, nil)
	fields.Set_field("thigh", 111, nil, nil)
	access := &fakeVirtualPinRegisterAccess{fields: fields}
	core := NewVirtualPinHelperCore(&fakeVirtualPinConfig{
		name: "tmc2209 stepper_y",
		data: map[string]interface{}{"diag0_pin": "PA1"},
	}, access)

	if err := core.BeginMoveHoming(access); err != nil {
		t.Fatalf("BeginMoveHoming returned error: %v", err)
	}
	if len(access.writes) != 4 {
		t.Fatalf("expected 4 begin-move writes, got %#v", access.writes)
	}
	wantBegin := []fakeVirtualPinWrite{
		{regName: "TCOOLTHRS", value: 0},
		{regName: "GCONF", value: 2},
		{regName: "TCOOLTHRS", value: 500},
		{regName: "THIGH", value: 0},
	}
	for i, want := range wantBegin {
		got := access.writes[i]
		if got.regName != want.regName || got.value != want.value {
			t.Fatalf("begin-move write %d = %#v, want %#v", i, got, want)
		}
	}
	if got := fields.Get_field("en_pwm_mode", nil, nil); got != 0 {
		t.Fatalf("expected en_pwm_mode disabled during homing, got %d", got)
	}
	if got := fields.Get_field("diag0_stall", nil, nil); got != 1 {
		t.Fatalf("expected diag0_stall enabled during homing, got %d", got)
	}
	if got := fields.Get_field("tcoolthrs", nil, nil); got != 500 {
		t.Fatalf("expected tcoolthrs set to 500 during move homing, got %d", got)
	}
	if got := fields.Get_field("thigh", nil, nil); got != 0 {
		t.Fatalf("expected thigh cleared during homing, got %d", got)
	}

	if err := core.EndHoming(access, nil); err != nil {
		t.Fatalf("EndHoming returned error: %v", err)
	}
	if len(access.writes) != 7 {
		t.Fatalf("expected 7 total writes after restore, got %#v", access.writes)
	}
	wantRestore := []fakeVirtualPinWrite{
		{regName: "TCOOLTHRS", value: 654},
		{regName: "GCONF", value: 1},
		{regName: "THIGH", value: 111},
	}
	for i, want := range wantRestore {
		got := access.writes[4+i]
		if got.regName != want.regName || got.value != want.value {
			t.Fatalf("restore write %d = %#v, want %#v", i, got, want)
		}
	}
	if got := fields.Get_field("en_pwm_mode", nil, nil); got != 1 {
		t.Fatalf("expected en_pwm_mode restored to 1, got %d", got)
	}
	if got := fields.Get_field("diag0_stall", nil, nil); got != 0 {
		t.Fatalf("expected diag0_stall restored to 0, got %d", got)
	}
	if got := fields.Get_field("tcoolthrs", nil, nil); got != 654 {
		t.Fatalf("expected tcoolthrs restored to 654, got %d", got)
	}
	if got := fields.Get_field("thigh", nil, nil); got != 111 {
		t.Fatalf("expected thigh restored to 111, got %d", got)
	}
}

func TestVirtualPinRuntimeEndMoveHomingUsesImmediateWrites(t *testing.T) {
	fields := NewFieldHelper(map[string]map[string]int64{
		"GCONF":     {"en_spreadcycle": 1 << 0},
		"TPWMTHRS":  {"tpwmthrs": 0xfffff},
		"TCOOLTHRS": {"tcoolthrs": 0xfffff},
	}, nil, nil, nil)
	fields.Set_field("en_spreadcycle", 1, nil, nil)
	fields.Set_field("tpwmthrs", 321, nil, nil)
	fields.Set_field("tcoolthrs", 0, nil, nil)
	access := &fakeVirtualPinRegisterAccess{fields: fields}
	runtime := NewVirtualPinRuntime(&fakeVirtualPinConfig{
		name: "tmc2208 stepper_x",
		data: map[string]interface{}{"diag_pin": "PA1"},
	}, access)

	if err := runtime.BeginHoming(); err != nil {
		t.Fatalf("BeginHoming returned error: %v", err)
	}
	access.writes = nil
	if err := runtime.EndMoveHoming(); err != nil {
		t.Fatalf("EndMoveHoming returned error: %v", err)
	}
	if len(access.writes) == 0 {
		t.Fatal("expected restore writes on EndMoveHoming")
	}
	for i, write := range access.writes {
		if write.printTime != nil {
			t.Fatalf("expected immediate restore write %d, got print time %v", i, *write.printTime)
		}
	}
}
