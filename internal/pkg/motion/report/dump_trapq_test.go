package report

import (
	"reflect"
	"testing"
)

func TestDumpTrapQControllerDelegatesToCore(t *testing.T) {
	movesSource := &fakeTrapQSource{
		batches: [][]TrapQMove{{{PrintTime: 2.0, MoveTime: 1.0, StartVelocity: 3.0, Acceleration: 1.0, StartPosition: [3]float64{1, 2, 3}, Direction: [3]float64{1, 0, 0}}}},
	}
	reactor := &fakeAPIDumpReactor{now: 2.0}
	controller := NewDumpTrapQController(NewTrapQDump("toolhead", movesSource), NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		_ = eventtime
		return map[string]interface{}{"sample": true}
	}, nil, 0.25))
	moves := controller.Moves(0.0, NeverTime)
	if len(moves) != 1 || moves[0].PrintTime != 2.0 {
		t.Fatalf("unexpected trapq moves %#v", moves)
	}
	if controller.LogMessage(moves) == "" {
		t.Fatal("expected non-empty trapq log message")
	}
	positionSource := &fakeTrapQSource{batches: [][]TrapQMove{{{PrintTime: 10.0, MoveTime: 4.0, StartVelocity: 2.0, Acceleration: 1.0, StartPosition: [3]float64{1, 2, 3}, Direction: [3]float64{1, 0, 0}}}}}
	positionController := NewDumpTrapQController(NewTrapQDump("toolhead", positionSource), NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		_ = eventtime
		return nil
	}, nil, 0.25))
	pos, velocity := positionController.PositionAt(12.0)
	if !reflect.DeepEqual(pos, []float64{7, 2, 3}) || velocity != 4.0 {
		t.Fatalf("unexpected position result pos=%#v velocity=%v", pos, velocity)
	}
	apiSource := &fakeTrapQSource{batches: [][]TrapQMove{{{PrintTime: 1.0, MoveTime: 0.5, StartVelocity: 2.0, Acceleration: 0.0, StartPosition: [3]float64{0, 0, 0}, Direction: [3]float64{1, 0, 0}}}}}
	apiController := NewDumpTrapQController(NewTrapQDump("toolhead", apiSource), NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		_ = eventtime
		return nil
	}, nil, 0.25))
	update := apiController.APIUpdate(3.0)
	if _, ok := update["data"]; !ok {
		t.Fatalf("unexpected api update %#v", update)
	}
	header := apiController.Header()
	if !reflect.DeepEqual(header["header"], []string{"time", "duration", "start_velocity", "acceleration", "start_position", "direction"}) {
		t.Fatalf("unexpected header %#v", header)
	}
}

func TestDumpTrapQControllerAddsClient(t *testing.T) {
	reactor := &fakeAPIDumpReactor{now: 4.0}
	controller := NewDumpTrapQController(NewTrapQDump("toolhead", &fakeTrapQSource{}), NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		return map[string]interface{}{"time": eventtime}
	}, nil, 0.2))
	client := &fakeAPIDumpClient{}
	controller.AddClient(client, map[string]interface{}{"meta": "trapq"})
	reactor.callback(4.2)
	if len(client.sent) != 1 {
		t.Fatalf("expected one sent payload, got %d", len(client.sent))
	}
	if client.sent[0]["meta"].(string) != "trapq" {
		t.Fatalf("unexpected sent payload %#v", client.sent[0])
	}
}