package mcu

import "fmt"

type LegacyRailEndstopRegistrar func(endstop interface{}, name string)

type LegacyRailEndstopResult struct {
	PinName string
	Endstop interface{}
	Entry   RailEndstopEntry
	Created bool
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
