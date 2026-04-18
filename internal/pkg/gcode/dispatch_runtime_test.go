package gcode

import (
	"reflect"
	"testing"
)

func TestDispatchRuntimeTracksReadyHandlers(t *testing.T) {
	runtime := NewDispatchRuntime(DispatchRuntimeOptions{})
	runtime.RegisterCommand("M115", func(interface{}) error { return nil }, true, "firmware info")
	runtime.RegisterCommand("SET_TEST_MODE", func(interface{}) error { return nil }, false, "test")

	if runtime.ActiveHandlers()["M115"] == nil {
		t.Fatalf("expected base handler to be active before ready")
	}
	if runtime.ActiveHandlers()["SET_TEST_MODE"] != nil {
		t.Fatalf("expected ready-only handler to stay inactive before ready")
	}
	if runtime.Help["SET_TEST_MODE"] != "test" {
		t.Fatalf("expected command help to be recorded")
	}

	runtime.SetReady(true)
	if !runtime.IsPrinterReady() {
		t.Fatalf("expected ready state to be tracked")
	}
	if runtime.ActiveHandlers()["SET_TEST_MODE"] == nil {
		t.Fatalf("expected ready-only handler to become active")
	}

	runtime.SetReady(false)
	if runtime.ActiveHandlers()["SET_TEST_MODE"] != nil {
		t.Fatalf("expected ready-only handler to be removed after shutdown")
	}
}

func TestDispatchRuntimeExtendsNonTraditionalCommands(t *testing.T) {
	type payload struct {
		extended bool
	}

	runtime := NewDispatchRuntime(DispatchRuntimeOptions{
		ExtendParams: func(arg interface{}) interface{} {
			return &payload{extended: true}
		},
	})

	called := false
	runtime.RegisterCommand("SET_TEST_MODE", func(arg interface{}) error {
		got := arg.(*payload)
		if !got.extended {
			t.Fatalf("expected extended params to be applied")
		}
		called = true
		return nil
	}, false, "")

	handler, ok := runtime.ReadyHandlers["SET_TEST_MODE"].(func(interface{}) error)
	if !ok {
		t.Fatalf("expected wrapped handler function")
	}
	if err := handler(&payload{}); err != nil {
		t.Fatalf("wrapped handler returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected wrapped handler to execute")
	}
}

func TestParseDispatchCommandLineParsesCommandAndParams(t *testing.T) {
	parsed := ParseDispatchCommandLine(" G1 X1.50 Y2.25 ; move comment ")

	if parsed.OriginalLine != "G1 X1.50 Y2.25 ; move comment" {
		t.Fatalf("unexpected original line: %q", parsed.OriginalLine)
	}
	if parsed.Command != "G1" {
		t.Fatalf("unexpected command: %q", parsed.Command)
	}
	if parsed.Params["X"] != "1.50" {
		t.Fatalf("unexpected X param: %q", parsed.Params["X"])
	}
	if parsed.Params["Y"] != "2.25" {
		t.Fatalf("unexpected Y param: %q", parsed.Params["Y"])
	}
	if parsed.Params["G"] != "1" {
		t.Fatalf("expected command token to be preserved in params")
	}
}

func TestDispatchRuntimeProcessCommandsUsesRegisteredAndDefaultHandlers(t *testing.T) {
	runtime := NewDispatchRuntime(DispatchRuntimeOptions{})
	seen := []string{}
	runtime.RegisterCommand("M115", func(arg interface{}) error {
		seen = append(seen, arg.(*DispatchCommand).Command)
		return nil
	}, true, "")

	acks := []string{}
	runtime.ProcessCommands([]string{"M115", "M117 Hello"}, true, DispatchProcessOptions{
		NewCommand: func(parsed ParsedDispatchCommand, needAck bool) *DispatchCommand {
			return NewDispatchCommand(
				func(string, bool) {},
				func(msg string) { acks = append(acks, msg) },
				parsed.Command,
				parsed.OriginalLine,
				parsed.Params,
				needAck,
			)
		},
		DefaultHandler: func(gcmd *DispatchCommand) error {
			seen = append(seen, "default:"+gcmd.Command)
			return nil
		},
	})

	if !reflect.DeepEqual(seen, []string{"M115", "default:M117"}) {
		t.Fatalf("unexpected handler sequence: %#v", seen)
	}
	if !reflect.DeepEqual(acks, []string{"ok", "ok"}) {
		t.Fatalf("unexpected ack sequence: %#v", acks)
	}
}

func TestDispatchRuntimeProcessCommandsDelegatesErrors(t *testing.T) {
	runtime := NewDispatchRuntime(DispatchRuntimeOptions{})
	runtime.RegisterCommand("M115", func(interface{}) error {
		return assertErr("boom")
	}, true, "")

	var handled ParsedDispatchCommand
	var gotErr error
	runtime.ProcessCommands([]string{"M115"}, true, DispatchProcessOptions{
		NewCommand: func(parsed ParsedDispatchCommand, needAck bool) *DispatchCommand {
			return NewDispatchCommand(func(string, bool) {}, func(string) {}, parsed.Command, parsed.OriginalLine, parsed.Params, needAck)
		},
		HandleError: func(parsed ParsedDispatchCommand, err error) {
			handled = parsed
			gotErr = err
		},
	})

	if handled.Command != "M115" {
		t.Fatalf("expected handled command M115, got %#v", handled)
	}
	if gotErr == nil || gotErr.Error() != "boom" {
		t.Fatalf("expected handled error boom, got %v", gotErr)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
