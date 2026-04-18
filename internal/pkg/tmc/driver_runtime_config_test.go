package tmc

import "testing"

type fakeDriverSectionConfig struct {
	*fakeDriverConfig
	sections map[string]*fakeDriverConfig
}

func (self *fakeDriverSectionConfig) Has_section(section string) bool {
	_, ok := self.sections[section]
	return ok
}

func (self *fakeDriverSectionConfig) Getsection(section string) *fakeDriverConfig {
	return self.sections[section]
}

func TestLookupDriverSectionFromConfigReturnsNestedSection(t *testing.T) {
	stepper := &fakeDriverConfig{name: "stepper_x"}
	config := &fakeDriverSectionConfig{
		fakeDriverConfig: &fakeDriverConfig{name: "tmc2209 stepper_x"},
		sections:         map[string]*fakeDriverConfig{"stepper_x": stepper},
	}

	got := LookupDriverSectionFromConfig[*fakeDriverConfig](config, "stepper_x")
	if got != stepper {
		t.Fatalf("expected nested section to be returned, got %#v", got)
	}

	if got := LookupDriverSectionFromConfig[*fakeDriverConfig](config, "stepper_y"); got != nil {
		t.Fatalf("expected missing section lookup to return nil, got %#v", got)
	}
}

func TestApplyDriverMicrostepConfigUsesDriverFallback(t *testing.T) {
	driver := &fakeDriverConfig{name: "tmc2209 stepper_x", values: map[string]interface{}{
		"microsteps":  32,
		"interpolate": false,
	}}
	stepper := &fakeDriverConfig{name: "stepper_x", values: map[string]interface{}{}}
	access := &fakeRegisterAccess{fields: NewFieldHelper(TMC2209Fields, TMC2208SignedFields, TMC2209FieldFormatters, nil)}

	err := ApplyDriverMicrostepConfig(driver, func(section string) DriverConfig {
		if section == "stepper_x" {
			return stepper
		}
		return nil
	}, access)
	if err != nil {
		t.Fatalf("expected driver microstep config to resolve, got %v", err)
	}
	if got := access.fields.Get_field("mres", nil, nil); got != 3 {
		t.Fatalf("expected 32 microsteps to map to mres=3, got %d", got)
	}
	if got := access.fields.Get_field("intpol", nil, nil); got != 0 {
		t.Fatalf("expected interpolate=false to clear intpol, got %d", got)
	}
}

func TestApplyDriverMicrostepConfigRequiresStepperSection(t *testing.T) {
	driver := &fakeDriverConfig{name: "tmc2209 stepper_x", values: map[string]interface{}{"microsteps": 16}}
	access := &fakeRegisterAccess{fields: NewFieldHelper(TMC2209Fields, TMC2208SignedFields, TMC2209FieldFormatters, nil)}

	err := ApplyDriverMicrostepConfig(driver, func(string) DriverConfig { return nil }, access)
	if err == nil {
		t.Fatalf("expected missing stepper section error")
	}
}

func TestApplyDriverStealthchopConfigUsesStepperDistance(t *testing.T) {
	driver := &fakeDriverConfig{name: "tmc2209 stepper_x", values: map[string]interface{}{
		"stealthchop_threshold": 50.0,
	}}
	stepper := &fakeDriverConfig{name: "stepper_x", values: map[string]interface{}{
		"rotation_distance":       40.0,
		"microsteps":              16,
		"full_steps_per_rotation": 200,
	}}
	access := &fakeRegisterAccess{fields: NewFieldHelper(TMC2209Fields, TMC2208SignedFields, TMC2209FieldFormatters, nil)}
	access.fields.Set_field("mres", 4, nil, nil)

	err := ApplyDriverStealthchopConfig(driver, func(section string) DriverConfig {
		if section == "stepper_x" {
			return stepper
		}
		return nil
	}, access, TMC2209TMCFrequency)
	if err != nil {
		t.Fatalf("expected stealthchop config to resolve, got %v", err)
	}
	want := int64(TMCtstepHelper(40.0/float64(200*16), 4, TMC2209TMCFrequency, 50.0))
	if got := access.fields.Get_field("tpwmthrs", nil, nil); got != want {
		t.Fatalf("expected tpwmthrs=%d, got %d", want, got)
	}
}

func TestApplyDriverCoolstepThresholdConfigUsesStepperDistance(t *testing.T) {
	driver := &fakeDriverConfig{name: "tmc2209 stepper_x", values: map[string]interface{}{
		"coolstep_threshold": 50.0,
	}}
	stepper := &fakeDriverConfig{name: "stepper_x", values: map[string]interface{}{
		"rotation_distance":       40.0,
		"microsteps":              16,
		"full_steps_per_rotation": 200,
	}}
	access := &fakeRegisterAccess{fields: NewFieldHelper(TMC2209Fields, TMC2208SignedFields, TMC2209FieldFormatters, nil)}
	access.fields.Set_field("mres", 4, nil, nil)

	err := ApplyDriverCoolstepThresholdConfig(driver, func(section string) DriverConfig {
		if section == "stepper_x" {
			return stepper
		}
		return nil
	}, access, TMC2209TMCFrequency)
	if err != nil {
		t.Fatalf("expected coolstep threshold config to resolve, got %v", err)
	}
	want := int64(TMCtstepHelper(40.0/float64(200*16), 4, TMC2209TMCFrequency, 50.0))
	if got := access.fields.Get_field("tcoolthrs", nil, nil); got != want {
		t.Fatalf("expected tcoolthrs=%d, got %d", want, got)
	}
}

func TestApplyDriverHighVelocityThresholdConfigUsesStepperDistance(t *testing.T) {
	driver := &fakeDriverConfig{name: "tmc5160 stepper_x", values: map[string]interface{}{
		"high_velocity_threshold": 60.0,
	}}
	stepper := &fakeDriverConfig{name: "stepper_x", values: map[string]interface{}{
		"rotation_distance":       40.0,
		"microsteps":              16,
		"full_steps_per_rotation": 200,
	}}
	access := &fakeRegisterAccess{fields: NewFieldHelper(TMC5160Fields, TMC5160SignedFields, TMC5160FieldFormatters, nil)}
	access.fields.Set_field("mres", 4, nil, nil)

	err := ApplyDriverHighVelocityThresholdConfig(driver, func(section string) DriverConfig {
		if section == "stepper_x" {
			return stepper
		}
		return nil
	}, access, TMC5160TMCFrequency)
	if err != nil {
		t.Fatalf("expected high velocity threshold config to resolve, got %v", err)
	}
	want := int64(TMCtstepHelper(40.0/float64(200*16), 4, TMC5160TMCFrequency, 60.0))
	if got := access.fields.Get_field("thigh", nil, nil); got != want {
		t.Fatalf("expected thigh=%d, got %d", want, got)
	}
}
