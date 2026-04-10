package tmc

import "testing"

type fakeVirtualPinEventRuntime struct {
	matches        map[interface{}]bool
	beginMoveCalls int
	endMoveCalls   int
	beginCalls     int
	endCalls       int
	beginMoveErr   error
	endMoveErr     error
	beginErr       error
	endErr         error
}

func (self *fakeVirtualPinEventRuntime) MatchesHomingMoveEndstop(endstop interface{}) bool {
	return self.matches[endstop]
}

func (self *fakeVirtualPinEventRuntime) BeginMoveHoming() error {
	self.beginMoveCalls++
	return self.beginMoveErr
}

func (self *fakeVirtualPinEventRuntime) EndMoveHoming() error {
	self.endMoveCalls++
	return self.endMoveErr
}

func (self *fakeVirtualPinEventRuntime) BeginHoming() error {
	self.beginCalls++
	return self.beginErr
}

func (self *fakeVirtualPinEventRuntime) EndHoming() error {
	self.endCalls++
	return self.endErr
}

func TestHandleVirtualPinHomingMoveBeginOnlyOnMatch(t *testing.T) {
	match := struct{ id string }{"match"}
	runtime := &fakeVirtualPinEventRuntime{matches: map[interface{}]bool{match: true}}
	if err := HandleVirtualPinHomingMoveBegin(runtime, []interface{}{"other", match, match}); err != nil {
		t.Fatalf("HandleVirtualPinHomingMoveBegin returned error: %v", err)
	}
	if runtime.beginMoveCalls != 1 {
		t.Fatalf("expected one begin-move call, got %d", runtime.beginMoveCalls)
	}

	runtime = &fakeVirtualPinEventRuntime{matches: map[interface{}]bool{}}
	if err := HandleVirtualPinHomingMoveBegin(runtime, []interface{}{"other"}); err != nil {
		t.Fatalf("HandleVirtualPinHomingMoveBegin returned error on non-match: %v", err)
	}
	if runtime.beginMoveCalls != 0 {
		t.Fatalf("expected no begin-move call on non-match, got %d", runtime.beginMoveCalls)
	}
}

func TestHandleVirtualPinHomingMoveEndOnlyOnMatch(t *testing.T) {
	match := struct{ id string }{"match"}
	runtime := &fakeVirtualPinEventRuntime{matches: map[interface{}]bool{match: true}}
	if err := HandleVirtualPinHomingMoveEnd(runtime, []interface{}{match, "other"}); err != nil {
		t.Fatalf("HandleVirtualPinHomingMoveEnd returned error: %v", err)
	}
	if runtime.endMoveCalls != 1 {
		t.Fatalf("expected one end-move call, got %d", runtime.endMoveCalls)
	}

	runtime = &fakeVirtualPinEventRuntime{matches: map[interface{}]bool{}}
	if err := HandleVirtualPinHomingMoveEnd(runtime, []interface{}{"other"}); err != nil {
		t.Fatalf("HandleVirtualPinHomingMoveEnd returned error on non-match: %v", err)
	}
	if runtime.endMoveCalls != 0 {
		t.Fatalf("expected no end-move call on non-match, got %d", runtime.endMoveCalls)
	}
}

func TestHandleVirtualPinHomingBeginAndEndDelegate(t *testing.T) {
	runtime := &fakeVirtualPinEventRuntime{}
	if err := HandleVirtualPinHomingBegin(runtime); err != nil {
		t.Fatalf("HandleVirtualPinHomingBegin returned error: %v", err)
	}
	if err := HandleVirtualPinHomingEnd(runtime); err != nil {
		t.Fatalf("HandleVirtualPinHomingEnd returned error: %v", err)
	}
	if runtime.beginCalls != 1 || runtime.endCalls != 1 {
		t.Fatalf("expected begin/end delegation once each, got begin=%d end=%d", runtime.beginCalls, runtime.endCalls)
	}
}
