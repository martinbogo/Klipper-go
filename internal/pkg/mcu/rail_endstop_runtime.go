package mcu

import "fmt"

type LegacyRailEndstopRegistrar func(endstop interface{}, name string)

type LegacyRailEndstopStepperAdder interface {
	AddStepper(stepper interface{})
}

type LegacyRailEndstopLegacyStepperAdder interface {
	Add_stepper(stepper interface{})
}

type LegacyRailEndstopResult struct {
	PinName string
	Endstop interface{}
	Entry   RailEndstopEntry
	Created bool
}

func LegacyRailEndstopEntriesFromRawMap(raw map[string]interface{}) map[string]RailEndstopEntry {
	entries := make(map[string]RailEndstopEntry, len(raw))
	for pinName, rawEntry := range raw {
		entry := rawEntry.(map[string]interface{})
		entries[pinName] = RailEndstopEntry{
			Endstop: entry["endstop"],
			Invert:  entry["invert"],
			Pullup:  entry["pullup"],
		}
	}
	return entries
}

func RawLegacyRailEndstopEntry(entry RailEndstopEntry) map[string]interface{} {
	return map[string]interface{}{
		"endstop": entry.Endstop,
		"invert":  entry.Invert,
		"pullup":  entry.Pullup,
	}
}

func AttachStepperToLegacyRailEndstop(endstop interface{}, stepper interface{}) bool {
	adder, ok := endstop.(LegacyRailEndstopStepperAdder)
	if ok {
		adder.AddStepper(stepper)
		return true
	}
	legacyAdder, ok := endstop.(LegacyRailEndstopLegacyStepperAdder)
	if !ok {
		return false
	}
	legacyAdder.Add_stepper(stepper)
	return true
}

func ResolveLegacyRailEndstop(existing map[string]RailEndstopEntry, chipName string, pin interface{}, invert interface{}, pullup interface{}, stepperName string, createEndstop func() interface{}, registerEndstop LegacyRailEndstopRegistrar) (LegacyRailEndstopResult, error) {
	lookupPlan := BuildRailEndstopLookupPlan(chipName, pin, invert, pullup, existing)
	if lookupPlan.SharedSettingsConflict {
		return LegacyRailEndstopResult{}, fmt.Errorf("shared endstop pin %s must specify the same pullup/invert settings", lookupPlan.PinName)
	}
	endstop := lookupPlan.ExistingEndstop
	created := false
	if lookupPlan.NeedsNewEndstop {
		endstop = createEndstop()
		created = true
		if registerEndstop != nil {
			registerEndstop(endstop, stepperName)
		}
	}
	return LegacyRailEndstopResult{
		PinName: lookupPlan.PinName,
		Endstop: endstop,
		Entry: RailEndstopEntry{
			Endstop: endstop,
			Invert:  invert,
			Pullup:  pullup,
		},
		Created: created,
	}, nil
}
