package mcu

import "fmt"

type RailEndstopEntry struct {
	Endstop interface{}
	Invert  interface{}
	Pullup  interface{}
}

type RailEndstopLookupPlan struct {
	PinName                string
	ExistingEndstop        interface{}
	NeedsNewEndstop        bool
	SharedSettingsConflict bool
}

func BuildRailEndstopLookupPlan(chipName string, pin interface{}, invert interface{}, pullup interface{}, existing map[string]RailEndstopEntry) RailEndstopLookupPlan {
	pinName := fmt.Sprintf("%s:%s", chipName, pin)
	entry, ok := existing[pinName]
	if !ok {
		return RailEndstopLookupPlan{PinName: pinName, NeedsNewEndstop: true}
	}
	return RailEndstopLookupPlan{
		PinName:                pinName,
		ExistingEndstop:        entry.Endstop,
		NeedsNewEndstop:        false,
		SharedSettingsConflict: entry.Invert != invert || entry.Pullup != pullup,
	}
}
