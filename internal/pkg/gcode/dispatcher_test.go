package gcode

import (
	"reflect"
	"testing"
)

type fakeDispatcherHost struct {
	startArgs map[string]interface{}
	events    map[string]func([]interface{}) error
	objects   map[string]interface{}
	shutdowns []string
	sent      []dispatcherEvent
	exits     []string
	stateMsg  string
	state     string
}

type dispatcherEvent struct {
	name   string
	params []interface{}
}

func (h *fakeDispatcherHost) StartArgs() map[string]interface{} {
	return h.startArgs
}

func (h *fakeDispatcherHost) RegisterEventHandler(event string, callback func([]interface{}) error) {
	if h.events == nil {
		h.events = map[string]func([]interface{}) error{}
	}
	h.events[event] = callback
}

func (h *fakeDispatcherHost) LookupObject(name string, defaultValue interface{}) interface{} {
	if h.objects != nil {
		if value, ok := h.objects[name]; ok {
			return value
		}
	}
	return defaultValue
}

func (h *fakeDispatcherHost) InvokeShutdown(msg string) {
	h.shutdowns = append(h.shutdowns, msg)
}

func (h *fakeDispatcherHost) SendEvent(event string, params []interface{}) {
	h.sent = append(h.sent, dispatcherEvent{name: event, params: params})
}

func (h *fakeDispatcherHost) RequestExit(result string) {
	h.exits = append(h.exits, result)
}

func (h *fakeDispatcherHost) StateMessage() (string, string) {
	return h.stateMsg, h.state
}

type fakeDispatcherToolhead struct {
	lastMoveTime float64
	dwells       []float64
	waits        int
}

func (t *fakeDispatcherToolhead) Get_last_move_time() float64 {
	return t.lastMoveTime
}

func (t *fakeDispatcherToolhead) Dwell(delay float64) {
	t.dwells = append(t.dwells, delay)
}

func (t *fakeDispatcherToolhead) Wait_moves() {
	t.waits++
}

func TestDispatcherRegistersLifecycleHandlersAndReadyResponse(t *testing.T) {
	host := &fakeDispatcherHost{startArgs: map[string]interface{}{}, stateMsg: "booting", state: "startup"}
	dispatcher := NewDispatcher(DispatcherOptions{Host: host})
	responses := []string{}
	dispatcher.Register_output_handler(func(msg string) {
		responses = append(responses, msg)
	})

	for _, event := range []string{"project:ready", "project:shutdown", "project:disconnect"} {
		if host.events[event] == nil {
			t.Fatalf("expected lifecycle handler for %s", event)
		}
	}
	if err := host.events["project:ready"](nil); err != nil {
		t.Fatalf("ready handler returned error: %v", err)
	}
	if !dispatcher.isPrinterReady {
		t.Fatalf("expected dispatcher to become ready")
	}
	if !reflect.DeepEqual(responses, []string{"project state: Ready"}) {
		t.Fatalf("unexpected ready response payloads: %#v", responses)
	}
	if dispatcher.Get_command_help()["RESTART"] != "Reload config file and restart host software" {
		t.Fatalf("expected restart help to be registered")
	}
}

func TestDispatcherRunScriptLocksAndEmitsCancelEvents(t *testing.T) {
	host := &fakeDispatcherHost{startArgs: map[string]interface{}{}}
	lockCount := 0
	unlockCount := 0
	dispatcher := NewDispatcher(DispatcherOptions{
		Host:   host,
		Lock:   func() { lockCount++ },
		Unlock: func() { unlockCount++ },
	})

	dispatcher.Run_script("CANCEL_PRINT")

	if lockCount != 1 || unlockCount != 1 {
		t.Fatalf("expected one lock/unlock, got %d/%d", lockCount, unlockCount)
	}
	want := []dispatcherEvent{{name: "project:pre_cancel", params: nil}, {name: "project:post_cancel", params: nil}}
	if !reflect.DeepEqual(host.sent, want) {
		t.Fatalf("unexpected cancel events: got %#v want %#v", host.sent, want)
	}
}

func TestDispatcherRequestRestartUsesToolheadAndExit(t *testing.T) {
	toolhead := &fakeDispatcherToolhead{lastMoveTime: 12.5}
	host := &fakeDispatcherHost{
		startArgs: map[string]interface{}{},
		objects:   map[string]interface{}{"toolhead": toolhead},
	}
	dispatcher := NewDispatcher(DispatcherOptions{Host: host})
	dispatcher.isPrinterReady = true

	dispatcher.Request_restart("restart")

	wantEvents := []dispatcherEvent{{name: "gcode:request_restart", params: []interface{}{12.5}}}
	if !reflect.DeepEqual(host.sent, wantEvents) {
		t.Fatalf("unexpected restart events: got %#v want %#v", host.sent, wantEvents)
	}
	if !reflect.DeepEqual(toolhead.dwells, []float64{0.5}) {
		t.Fatalf("unexpected dwell values: %#v", toolhead.dwells)
	}
	if toolhead.waits != 1 {
		t.Fatalf("expected one wait_moves call, got %d", toolhead.waits)
	}
	if !reflect.DeepEqual(host.exits, []string{"restart"}) {
		t.Fatalf("unexpected exit requests: %#v", host.exits)
	}
}

func TestDispatcherRespondErrorRequestsExitForFileInput(t *testing.T) {
	host := &fakeDispatcherHost{startArgs: map[string]interface{}{"debuginput": "commands.gcode"}}
	dispatcher := NewDispatcher(DispatcherOptions{Host: host})
	responses := []string{}
	dispatcher.Register_output_handler(func(msg string) {
		responses = append(responses, msg)
	})

	dispatcher.Respond_error("boom\nmore detail")

	if len(responses) != 2 {
		t.Fatalf("expected two response callbacks, got %#v", responses)
	}
	if responses[1] != "!! boom" {
		t.Fatalf("expected primary error response, got %#v", responses)
	}
	if !reflect.DeepEqual(host.exits, []string{"error_exit"}) {
		t.Fatalf("unexpected exit requests: %#v", host.exits)
	}
}

func TestDispatcherHostFuncsDelegateToCallbacks(t *testing.T) {
	host := &fakeDispatcherHost{
		startArgs: map[string]interface{}{"mode": "test"},
		objects:   map[string]interface{}{"toolhead": "ok"},
		stateMsg:  "warming",
		state:     "startup",
	}
	adapter := &DispatcherHostFuncs{
		StartArgsFunc:            host.StartArgs,
		RegisterEventHandlerFunc: host.RegisterEventHandler,
		LookupObjectFunc:         host.LookupObject,
		InvokeShutdownFunc:       host.InvokeShutdown,
		SendEventFunc:            host.SendEvent,
		RequestExitFunc:          host.RequestExit,
		StateMessageFunc:         host.StateMessage,
	}

	if got := adapter.StartArgs()["mode"]; got != "test" {
		t.Fatalf("unexpected start args %#v", adapter.StartArgs())
	}
	adapter.RegisterEventHandler("project:test", func([]interface{}) error { return nil })
	if host.events["project:test"] == nil {
		t.Fatalf("expected delegated event registration")
	}
	if got := adapter.LookupObject("toolhead", nil); got != "ok" {
		t.Fatalf("unexpected lookup value %#v", got)
	}
	adapter.InvokeShutdown("boom")
	adapter.SendEvent("project:event", []interface{}{1})
	adapter.RequestExit("restart")
	msg, state := adapter.StateMessage()
	if msg != "warming" || state != "startup" {
		t.Fatalf("unexpected state message %q/%q", msg, state)
	}
	if !reflect.DeepEqual(host.shutdowns, []string{"boom"}) {
		t.Fatalf("unexpected shutdowns %#v", host.shutdowns)
	}
	if !reflect.DeepEqual(host.sent, []dispatcherEvent{{name: "project:event", params: []interface{}{1}}}) {
		t.Fatalf("unexpected sent events %#v", host.sent)
	}
	if !reflect.DeepEqual(host.exits, []string{"restart"}) {
		t.Fatalf("unexpected exits %#v", host.exits)
	}
}
