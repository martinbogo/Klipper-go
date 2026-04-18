package probe

import (
	"reflect"
	"testing"
)

type fakeProbeEventRuntime struct {
	core                  *PrinterProbe
	matchingHomingEndstop interface{}
	matchingRailEndstop   interface{}
	prepareMoves          []interface{}
	finishMoves           []interface{}
	beginMCUCount         int
	endMCUCount           int
	events                []string
	callLog               []string
}

func (self *fakeProbeEventRuntime) Core() *PrinterProbe {
	return self.core
}

func (self *fakeProbeEventRuntime) MatchesHomingMoveEndstop(endstop interface{}) bool {
	return endstop == self.matchingHomingEndstop
}

func (self *fakeProbeEventRuntime) MatchesHomeRailEndstop(endstop interface{}) bool {
	return endstop == self.matchingRailEndstop
}

func (self *fakeProbeEventRuntime) PrepareProbe(move interface{}) {
	self.prepareMoves = append(self.prepareMoves, move)
	self.callLog = append(self.callLog, "prepare")
}

func (self *fakeProbeEventRuntime) FinishProbe(move interface{}) {
	self.finishMoves = append(self.finishMoves, move)
	self.callLog = append(self.callLog, "finish")
}

func (self *fakeProbeEventRuntime) BeginMCUMultiProbe() {
	self.beginMCUCount++
	self.callLog = append(self.callLog, "begin-mcu")
}

func (self *fakeProbeEventRuntime) EndMCUMultiProbe() {
	self.endMCUCount++
	self.callLog = append(self.callLog, "end-mcu")
}

func (self *fakeProbeEventRuntime) SendEvent(event string) {
	self.events = append(self.events, event)
	self.callLog = append(self.callLog, "event:"+event)
}

func TestHandleHomingMoveBeginPreparesOnceForMatchingWrapper(t *testing.T) {
	match := &struct{}{}
	runtime := &fakeProbeEventRuntime{
		core:                  NewPrinterProbe(0, 0, 0, 0, 0, 0, 0, 1, 0, "average", 0, 0),
		matchingHomingEndstop: match,
	}

	HandleHomingMoveBegin(runtime, "move", []interface{}{1, match, match})

	if got, want := runtime.prepareMoves, []interface{}{"move"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("prepareMoves = %v, want %v", got, want)
	}
	if runtime.beginMCUCount != 0 || runtime.endMCUCount != 0 {
		t.Fatalf("unexpected multi-probe activity: begin=%d end=%d", runtime.beginMCUCount, runtime.endMCUCount)
	}
	if got, want := runtime.callLog, []string{"prepare"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callLog = %v, want %v", got, want)
	}
}

func TestHandleHomingMoveEndFinishesOnceForMatchingWrapper(t *testing.T) {
	match := &struct{}{}
	runtime := &fakeProbeEventRuntime{
		core:                  NewPrinterProbe(0, 0, 0, 0, 0, 0, 0, 1, 0, "average", 0, 0),
		matchingHomingEndstop: match,
	}

	HandleHomingMoveEnd(runtime, "move", []interface{}{match, match})

	if got, want := runtime.finishMoves, []interface{}{"move"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("finishMoves = %v, want %v", got, want)
	}
	if got, want := runtime.callLog, []string{"finish"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callLog = %v, want %v", got, want)
	}
}

func TestHandleHomeRailsBeginStartsMultiProbeLifecycle(t *testing.T) {
	match := &struct{}{}
	core := NewPrinterProbe(0, 0, 0, 0, 0, 0, 0, 1, 0, "average", 0, 0)
	runtime := &fakeProbeEventRuntime{
		core:                core,
		matchingRailEndstop: match,
	}

	HandleHomeRailsBegin(runtime, []interface{}{"other", match, match})

	if !core.MultiProbePending {
		t.Fatal("core.MultiProbePending = false, want true")
	}
	if runtime.beginMCUCount != 1 {
		t.Fatalf("beginMCUCount = %d, want 1", runtime.beginMCUCount)
	}
	if got, want := runtime.callLog, []string{"event:homing:multi_probe_begin", "begin-mcu"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callLog = %v, want %v", got, want)
	}
}

func TestHandleHomeRailsEndEndsPendingMultiProbeBeforeEvent(t *testing.T) {
	match := &struct{}{}
	core := NewPrinterProbe(0, 0, 0, 0, 0, 0, 0, 1, 0, "average", 0, 0)
	core.BeginMultiProbe()
	runtime := &fakeProbeEventRuntime{
		core:                core,
		matchingRailEndstop: match,
	}

	HandleHomeRailsEnd(runtime, []interface{}{match, match})

	if core.MultiProbePending {
		t.Fatal("core.MultiProbePending = true, want false")
	}
	if runtime.endMCUCount != 1 {
		t.Fatalf("endMCUCount = %d, want 1", runtime.endMCUCount)
	}
	if got, want := runtime.callLog, []string{"end-mcu", "event:homing:multi_probe_end"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callLog = %v, want %v", got, want)
	}
}

func TestHandleCommandErrorAlwaysEmitsMultiProbeEndEvent(t *testing.T) {
	runtime := &fakeProbeEventRuntime{
		core: NewPrinterProbe(0, 0, 0, 0, 0, 0, 0, 1, 0, "average", 0, 0),
	}

	HandleCommandError(runtime)

	if runtime.endMCUCount != 0 {
		t.Fatalf("endMCUCount = %d, want 0", runtime.endMCUCount)
	}
	if got, want := runtime.events, []string{"homing:multi_probe_end"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if got, want := runtime.callLog, []string{"event:homing:multi_probe_end"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("callLog = %v, want %v", got, want)
	}
}
