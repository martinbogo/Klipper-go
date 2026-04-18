package mcu

import "testing"

type fakeRailEndstopAdder struct {
	steppers []interface{}
}

func (self *fakeRailEndstopAdder) AddStepper(stepper interface{}) {
	self.steppers = append(self.steppers, stepper)
}

type fakeLegacyRailEndstopAdder struct {
	steppers []interface{}
}

func (self *fakeLegacyRailEndstopAdder) Add_stepper(stepper interface{}) {
	self.steppers = append(self.steppers, stepper)
}

func TestResolveLegacyRailEndstopCreatesAndRegistersNewEndstop(t *testing.T) {
	createdCount := 0
	registered := []struct {
		endstop interface{}
		name    string
	}{}
	createdEndstop := &struct{ id string }{id: "new"}
	result, err := ResolveLegacyRailEndstop(
		map[string]RailEndstopEntry{},
		"mcu",
		"PA1",
		0,
		1,
		"stepper_x",
		func() interface{} {
			createdCount++
			return createdEndstop
		},
		func(endstop interface{}, name string) {
			registered = append(registered, struct {
				endstop interface{}
				name    string
			}{endstop: endstop, name: name})
		},
	)
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if !result.Created || result.PinName != "mcu:PA1" || result.Endstop != createdEndstop {
		t.Fatalf("unexpected resolve result %#v", result)
	}
	if result.Entry.Endstop != createdEndstop || result.Entry.Invert != 0 || result.Entry.Pullup != 1 {
		t.Fatalf("unexpected result entry %#v", result.Entry)
	}
	if createdCount != 1 {
		t.Fatalf("expected createEndstop to be called once, got %d", createdCount)
	}
	if len(registered) != 1 || registered[0].endstop != createdEndstop || registered[0].name != "stepper_x" {
		t.Fatalf("unexpected registration calls %#v", registered)
	}
}

func TestResolveLegacyRailEndstopReusesExistingEndstop(t *testing.T) {
	existing := &struct{ id string }{id: "existing"}
	createCalled := false
	registerCalled := false
	result, err := ResolveLegacyRailEndstop(
		map[string]RailEndstopEntry{"mcu:PA1": {Endstop: existing, Invert: 0, Pullup: 1}},
		"mcu",
		"PA1",
		0,
		1,
		"stepper_y",
		func() interface{} {
			createCalled = true
			return nil
		},
		func(endstop interface{}, name string) {
			_, _ = endstop, name
			registerCalled = true
		},
	)
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if result.Created {
		t.Fatalf("expected existing endstop to be reused, got %#v", result)
	}
	if result.Endstop != existing || result.Entry.Endstop != existing {
		t.Fatalf("unexpected reused endstop %#v", result)
	}
	if createCalled || registerCalled {
		t.Fatalf("expected no create/register calls, got create=%v register=%v", createCalled, registerCalled)
	}
}

func TestResolveLegacyRailEndstopDetectsSharedSettingsConflict(t *testing.T) {
	_, err := ResolveLegacyRailEndstop(
		map[string]RailEndstopEntry{"mcu:PA1": {Endstop: "endstop", Invert: 0, Pullup: 1}},
		"mcu",
		"PA1",
		1,
		1,
		"stepper_z",
		func() interface{} { return nil },
		nil,
	)
	if err == nil {
		t.Fatalf("expected shared-settings conflict")
	}
	if err.Error() != "shared endstop pin mcu:PA1 must specify the same pullup/invert settings" {
		t.Fatalf("unexpected error %q", err.Error())
	}
}

func TestLegacyRailEndstopEntriesFromRawMapCopiesProjectShape(t *testing.T) {
	raw := map[string]interface{}{
		"mcu:PA1": map[string]interface{}{"endstop": "endstop0", "invert": 1, "pullup": 0},
	}

	entries := LegacyRailEndstopEntriesFromRawMap(raw)
	entry := entries["mcu:PA1"]
	if entry.Endstop != "endstop0" || entry.Invert != 1 || entry.Pullup != 0 {
		t.Fatalf("unexpected converted entry %#v", entry)
	}

	raw["mcu:PA1"].(map[string]interface{})["invert"] = 0
	if entries["mcu:PA1"].Invert != 1 {
		t.Fatalf("expected converted map to be independent, got %#v", entries["mcu:PA1"])
	}
}

func TestRawLegacyRailEndstopEntryBuildsProjectShape(t *testing.T) {
	raw := RawLegacyRailEndstopEntry(RailEndstopEntry{Endstop: "endstop1", Invert: 0, Pullup: 1})
	if raw["endstop"] != "endstop1" || raw["invert"] != 0 || raw["pullup"] != 1 {
		t.Fatalf("unexpected raw entry %#v", raw)
	}
}

func TestAttachStepperToLegacyRailEndstopUsesAdderInterface(t *testing.T) {
	endstop := &fakeRailEndstopAdder{}
	stepper := "stepper_x"
	if !AttachStepperToLegacyRailEndstop(endstop, stepper) {
		t.Fatal("expected stepper attachment to succeed")
	}
	if len(endstop.steppers) != 1 || endstop.steppers[0] != stepper {
		t.Fatalf("unexpected attached steppers %#v", endstop.steppers)
	}
	if AttachStepperToLegacyRailEndstop("not-an-endstop", stepper) {
		t.Fatal("expected unsupported endstop attachment to report false")
	}
}

func TestAttachStepperToLegacyRailEndstopUsesLegacyAdderInterface(t *testing.T) {
	endstop := &fakeLegacyRailEndstopAdder{}
	stepper := "stepper_y"
	if !AttachStepperToLegacyRailEndstop(endstop, stepper) {
		t.Fatal("expected legacy stepper attachment to succeed")
	}
	if len(endstop.steppers) != 1 || endstop.steppers[0] != stepper {
		t.Fatalf("unexpected attached legacy steppers %#v", endstop.steppers)
	}
}
