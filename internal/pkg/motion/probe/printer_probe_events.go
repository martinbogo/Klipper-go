package probe

type ProbeEventRuntime interface {
	Core() *PrinterProbe
	MatchesHomingMoveEndstop(endstop interface{}) bool
	MatchesHomeRailEndstop(endstop interface{}) bool
	PrepareProbe(move interface{})
	FinishProbe(move interface{})
	BeginMCUMultiProbe()
	EndMCUMultiProbe()
	SendEvent(event string)
}

func BeginMultiProbe(runtime ProbeEventRuntime) {
	runtime.SendEvent("homing:multi_probe_begin")
	runtime.BeginMCUMultiProbe()
	runtime.Core().BeginMultiProbe()
}

func EndMultiProbe(runtime ProbeEventRuntime) {
	if runtime.Core().EndMultiProbe() {
		runtime.EndMCUMultiProbe()
	}
	runtime.SendEvent("homing:multi_probe_end")
}

func HandleHomingMoveBegin(runtime ProbeEventRuntime, move interface{}, endstops []interface{}) {
	if hasMatchingEndstop(endstops, runtime.MatchesHomingMoveEndstop) {
		runtime.PrepareProbe(move)
	}
}

func HandleHomingMoveEnd(runtime ProbeEventRuntime, move interface{}, endstops []interface{}) {
	if hasMatchingEndstop(endstops, runtime.MatchesHomingMoveEndstop) {
		runtime.FinishProbe(move)
	}
}

func HandleHomeRailsBegin(runtime ProbeEventRuntime, endstops []interface{}) {
	if hasMatchingEndstop(endstops, runtime.MatchesHomeRailEndstop) {
		BeginMultiProbe(runtime)
	}
}

func HandleHomeRailsEnd(runtime ProbeEventRuntime, endstops []interface{}) {
	if hasMatchingEndstop(endstops, runtime.MatchesHomeRailEndstop) {
		EndMultiProbe(runtime)
	}
}

func HandleCommandError(runtime ProbeEventRuntime) {
	EndMultiProbe(runtime)
}

type probeEndstopMatcher func(interface{}) bool

func hasMatchingEndstop(endstops []interface{}, matches probeEndstopMatcher) bool {
	for _, endstop := range endstops {
		if matches(endstop) {
			return true
		}
	}
	return false
}
