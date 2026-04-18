package gcode

import (
	"reflect"
	"testing"
)

func TestDispatchCommandAckAndHelpers(t *testing.T) {
	rawResponses := []string{}
	infoResponses := []string{}
	command := NewDispatchCommand(
		func(msg string, log bool) {
			infoResponses = append(infoResponses, msg)
		},
		func(msg string) {
			rawResponses = append(rawResponses, msg)
		},
		"SET_TEST",
		"SET_TEST VALUE=42 SPEED=2.5",
		map[string]string{"VALUE": "42", "SPEED": "2.5"},
		true,
	)

	if got := command.String("VALUE", ""); got != "42" {
		t.Fatalf("expected VALUE 42, got %q", got)
	}
	if got := command.Int("VALUE", 0, nil, nil); got != 42 {
		t.Fatalf("expected int VALUE 42, got %d", got)
	}
	if got := command.Float("SPEED", 0); got != 2.5 {
		t.Fatalf("expected SPEED 2.5, got %v", got)
	}
	if got := command.RawParameters(); got != "VALUE=42 SPEED=2.5" {
		t.Fatalf("expected raw parameters, got %q", got)
	}

	if !command.Ack("queued") {
		t.Fatal("expected first Ack to send response")
	}
	if command.Ack("") {
		t.Fatal("expected second Ack to be ignored")
	}
	if !reflect.DeepEqual(rawResponses, []string{"ok queued"}) {
		t.Fatalf("unexpected raw responses %#v", rawResponses)
	}

	command.RespondInfo("status", true)
	command.RespondRaw("raw")
	if !reflect.DeepEqual(infoResponses, []string{"status"}) {
		t.Fatalf("unexpected info responses %#v", infoResponses)
	}
	if !reflect.DeepEqual(rawResponses, []string{"ok queued", "raw"}) {
		t.Fatalf("unexpected raw responses after RespondRaw %#v", rawResponses)
	}
	if got := command.Get_commandline(); got != "SET_TEST VALUE=42 SPEED=2.5" {
		t.Fatalf("unexpected commandline %q", got)
	}
	if got := command.Get_command_parameters(); !reflect.DeepEqual(got, map[string]string{"VALUE": "42", "SPEED": "2.5"}) {
		t.Fatalf("unexpected params %#v", got)
	}
}