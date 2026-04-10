package probe

import (
	"reflect"
	"testing"
)

type fakeManualProbeCommand struct {
	params    map[string]string
	responses []string
}

func (self *fakeManualProbeCommand) Parameters() map[string]string {
	copied := make(map[string]string, len(self.params))
	for key, value := range self.params {
		copied[key] = value
	}
	return copied
}

func (self *fakeManualProbeCommand) RespondInfo(msg string, log bool) {
	_ = log
	self.responses = append(self.responses, msg)
}

type fakeManualProbeGCode struct {
	handlers  map[string]func(ManualProbeCommand) error
	responses []string
	cleared   []string
}

func (self *fakeManualProbeGCode) RegisterCommand(cmd string, handler func(ManualProbeCommand) error, desc string) {
	_ = desc
	if self.handlers == nil {
		self.handlers = map[string]func(ManualProbeCommand) error{}
	}
	self.handlers[cmd] = handler
}

func (self *fakeManualProbeGCode) ClearCommand(cmd string) {
	delete(self.handlers, cmd)
	self.cleared = append(self.cleared, cmd)
}

func (self *fakeManualProbeGCode) RespondInfo(msg string, log bool) {
	_ = log
	self.responses = append(self.responses, msg)
}

type fakeManualProbeRuntime struct {
	status        map[string]interface{}
	toolheadPos   []float64
	kinematicsPos []float64
	moves         [][]interface{}
	speeds        []float64
}

func (self *fakeManualProbeRuntime) ResetStatus() {
	self.status = map[string]interface{}{
		"is_active":        false,
		"z_position":       nil,
		"z_position_lower": nil,
		"z_position_upper": nil,
	}
}

func (self *fakeManualProbeRuntime) SetStatus(status map[string]interface{}) {
	self.status = status
}

func (self *fakeManualProbeRuntime) ToolheadPosition() []float64 {
	return append([]float64{}, self.toolheadPos...)
}

func (self *fakeManualProbeRuntime) KinematicsPosition() []float64 {
	return append([]float64{}, self.kinematicsPos...)
}

func (self *fakeManualProbeRuntime) ManualMove(coord []interface{}, speed float64) {
	copied := append([]interface{}{}, coord...)
	self.moves = append(self.moves, copied)
	self.speeds = append(self.speeds, speed)
	if len(coord) > 2 && coord[2] != nil {
		z := coord[2].(float64)
		self.toolheadPos[2] = z
		self.kinematicsPos[2] = z
	}
}

func TestNewManualProbeSessionRegistersCommands(t *testing.T) {
	gcode := &fakeManualProbeGCode{}
	runtime := &fakeManualProbeRuntime{toolheadPos: []float64{0, 0, 5, 0}, kinematicsPos: []float64{0, 0, 5, 0}}
	runtime.ResetStatus()

	session := NewManualProbeSession(gcode, runtime, 4.5, func([]float64) {})
	if session == nil {
		t.Fatal("expected session")
	}
	for _, name := range []string{"ACCEPT", "NEXT", "ABORT", "TESTZ"} {
		if gcode.handlers[name] == nil {
			t.Fatalf("expected command %s to be registered", name)
		}
	}
	if len(gcode.responses) != 1 {
		t.Fatalf("expected startup response, got %#v", gcode.responses)
	}
}

func TestManualProbeSessionTestZInsertsHistoryAndMoves(t *testing.T) {
	gcode := &fakeManualProbeGCode{}
	runtime := &fakeManualProbeRuntime{toolheadPos: []float64{0, 0, 1, 0}, kinematicsPos: []float64{0, 0, 1, 0}}
	runtime.ResetStatus()
	session := NewManualProbeSession(gcode, runtime, 5, func([]float64) {})

	command := &fakeManualProbeCommand{params: map[string]string{"Z": "-0.100"}}
	if err := gcode.handlers["TESTZ"](command); err != nil {
		t.Fatalf("unexpected TESTZ error: %v", err)
	}
	if !reflect.DeepEqual(session.pastPositions, []float64{1}) {
		t.Fatalf("expected past position history [1], got %#v", session.pastPositions)
	}
	if len(runtime.moves) != 2 {
		t.Fatalf("expected bob move and target move, got %#v", runtime.moves)
	}
	if runtime.toolheadPos[2] != 0.9 {
		t.Fatalf("expected final Z 0.9, got %v", runtime.toolheadPos[2])
	}
}

func TestManualProbeSessionAcceptFinalizesAndClearsCommands(t *testing.T) {
	gcode := &fakeManualProbeGCode{}
	runtime := &fakeManualProbeRuntime{toolheadPos: []float64{0, 0, 4, 0}, kinematicsPos: []float64{0, 0, 3.5, 0}}
	runtime.ResetStatus()
	finalized := []float64(nil)
	NewManualProbeSession(gcode, runtime, 5, func(position []float64) {
		finalized = append([]float64{}, position...)
	})
	runtime.toolheadPos[2] = 3.5

	command := &fakeManualProbeCommand{params: map[string]string{}}
	if err := gcode.handlers["ACCEPT"](command); err != nil {
		t.Fatalf("unexpected ACCEPT error: %v", err)
	}
	if !reflect.DeepEqual(finalized, []float64{0, 0, 3.5, 0}) {
		t.Fatalf("unexpected finalized position %#v", finalized)
	}
	if len(gcode.handlers) != 0 {
		t.Fatalf("expected commands to be cleared, got %#v", gcode.handlers)
	}
	if runtime.status["is_active"] != false {
		t.Fatalf("expected reset status, got %#v", runtime.status)
	}
}

func TestManualProbeSessionReportZStatusSetsCurrentWindow(t *testing.T) {
	gcode := &fakeManualProbeGCode{}
	runtime := &fakeManualProbeRuntime{toolheadPos: []float64{0, 0, 1, 0}, kinematicsPos: []float64{0, 0, 1, 0}}
	runtime.ResetStatus()
	session := NewManualProbeSession(gcode, runtime, 5, func([]float64) {})
	session.pastPositions = []float64{0.8, 1.0, 1.2}

	session.ReportZStatus(true, 1.0)
	if runtime.status["zPositionLower"] != 0.8 {
		t.Fatalf("expected lower bound 0.8, got %#v", runtime.status)
	}
	if runtime.status["zPositionUpper"] != 1.2 {
		t.Fatalf("expected upper bound 1.2, got %#v", runtime.status)
	}
}
