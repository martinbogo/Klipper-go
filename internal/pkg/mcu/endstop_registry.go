package mcu

import "strings"

type EndstopRegistryStepper interface {
	MCUKey() interface{}
	Name(short bool) string
	Raw() interface{}
}

type EndstopRegistryTrsync interface {
	MCUKey() interface{}
	Steppers() []EndstopRegistryStepper
}

type EndstopAddStepperPlan struct {
	TrsyncIndex        int
	NeedsNewTrsync     bool
	SharedAxisConflict bool
}

const SharedAxisConflictWarning = "Multi-mcu homing not supported on multi-mcu shared axis"

func (self EndstopAddStepperPlan) WarningMessage() string {
	if self.SharedAxisConflict {
		return SharedAxisConflictWarning
	}
	return ""
}

func BuildEndstopAddStepperPlan(trsyncs []EndstopRegistryTrsync, stepper EndstopRegistryStepper) EndstopAddStepperPlan {
	plan := EndstopAddStepperPlan{TrsyncIndex: len(trsyncs), NeedsNewTrsync: true}
	for i, trsync := range trsyncs {
		if trsync.MCUKey() == stepper.MCUKey() {
			plan.TrsyncIndex = i
			plan.NeedsNewTrsync = false
			break
		}
	}
	sname := stepper.Name(false)
	if !strings.HasPrefix(sname, "stepper_") {
		return plan
	}
	axisPrefix := sname[:9]
	for i, trsync := range trsyncs {
		if !plan.NeedsNewTrsync && i == plan.TrsyncIndex {
			continue
		}
		for _, other := range trsync.Steppers() {
			if strings.HasPrefix(other.Name(false), axisPrefix) {
				plan.SharedAxisConflict = true
				return plan
			}
		}
	}
	return plan
}

func CollectEndstopSteppers(trsyncs []EndstopRegistryTrsync) []interface{} {
	var steppers []interface{}
	for _, trsync := range trsyncs {
		for _, stepper := range trsync.Steppers() {
			steppers = append(steppers, stepper.Raw())
		}
	}
	return steppers
}
