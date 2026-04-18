package mcu

import "testing"

type fakeEndstopRegistryStepper struct {
	mcuKey interface{}
	name   string
	raw    interface{}
}

func (self *fakeEndstopRegistryStepper) MCUKey() interface{} { return self.mcuKey }
func (self *fakeEndstopRegistryStepper) Name(short bool) string {
	_ = short
	return self.name
}
func (self *fakeEndstopRegistryStepper) Raw() interface{} { return self.raw }

type fakeEndstopRegistryTrsync struct {
	mcuKey   interface{}
	steppers []EndstopRegistryStepper
}

func (self *fakeEndstopRegistryTrsync) MCUKey() interface{} { return self.mcuKey }
func (self *fakeEndstopRegistryTrsync) Steppers() []EndstopRegistryStepper {
	return append([]EndstopRegistryStepper(nil), self.steppers...)
}

func TestBuildEndstopAddStepperPlanReusesMatchingTrsync(t *testing.T) {
	mcu0 := "mcu0"
	plan := BuildEndstopAddStepperPlan([]EndstopRegistryTrsync{
		&fakeEndstopRegistryTrsync{mcuKey: mcu0},
	}, &fakeEndstopRegistryStepper{mcuKey: mcu0, name: "stepper_x"})
	if plan.TrsyncIndex != 0 || plan.NeedsNewTrsync || plan.SharedAxisConflict {
		t.Fatalf("unexpected plan %#v", plan)
	}
}

func TestBuildEndstopAddStepperPlanCreatesNewTrsync(t *testing.T) {
	plan := BuildEndstopAddStepperPlan([]EndstopRegistryTrsync{
		&fakeEndstopRegistryTrsync{mcuKey: "mcu0"},
	}, &fakeEndstopRegistryStepper{mcuKey: "mcu1", name: "extruder"})
	if plan.TrsyncIndex != 1 || !plan.NeedsNewTrsync || plan.SharedAxisConflict {
		t.Fatalf("unexpected plan %#v", plan)
	}
}

func TestBuildEndstopAddStepperPlanDetectsSharedAxisConflict(t *testing.T) {
	plan := BuildEndstopAddStepperPlan([]EndstopRegistryTrsync{
		&fakeEndstopRegistryTrsync{mcuKey: "mcu0", steppers: []EndstopRegistryStepper{
			&fakeEndstopRegistryStepper{mcuKey: "mcu0", name: "stepper_x"},
		}},
	}, &fakeEndstopRegistryStepper{mcuKey: "mcu1", name: "stepper_x1"})
	if !plan.NeedsNewTrsync || !plan.SharedAxisConflict {
		t.Fatalf("expected shared-axis conflict, got %#v", plan)
	}
	if got := plan.WarningMessage(); got != SharedAxisConflictWarning {
		t.Fatalf("unexpected warning message %q", got)
	}
}

func TestBuildEndstopAddStepperPlanWarningMessageEmptyWithoutConflict(t *testing.T) {
	plan := BuildEndstopAddStepperPlan([]EndstopRegistryTrsync{
		&fakeEndstopRegistryTrsync{mcuKey: "mcu0"},
	}, &fakeEndstopRegistryStepper{mcuKey: "mcu0", name: "stepper_x"})
	if got := plan.WarningMessage(); got != "" {
		t.Fatalf("expected empty warning message, got %q", got)
	}
}

func TestCollectEndstopSteppersFlattensTrsyncs(t *testing.T) {
	stepper0 := &fakeEndstopRegistryStepper{mcuKey: "mcu0", name: "stepper_x", raw: "x"}
	stepper1 := &fakeEndstopRegistryStepper{mcuKey: "mcu1", name: "stepper_y", raw: "y"}
	steppers := CollectEndstopSteppers([]EndstopRegistryTrsync{
		&fakeEndstopRegistryTrsync{mcuKey: "mcu0", steppers: []EndstopRegistryStepper{stepper0}},
		&fakeEndstopRegistryTrsync{mcuKey: "mcu1", steppers: []EndstopRegistryStepper{stepper1}},
	})
	if len(steppers) != 2 || steppers[0] != "x" || steppers[1] != "y" {
		t.Fatalf("unexpected flattened steppers %#v", steppers)
	}
}
