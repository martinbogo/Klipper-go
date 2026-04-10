package report

import (
	"reflect"
	"testing"
)

func TestDumpStepperControllerDelegatesToCore(t *testing.T) {
	queueSource := &fakeStepperSource{
		batches:       [][]StepQueueEntry{{{FirstClock: 20, LastClock: 25, StartPosition: 2}}},
		name:          "stepper_x",
		mcuName:       "mcu",
		clockToTime:   func(clock int64) float64 { return float64(clock) / 10.0 },
		startPosition: 1.25,
		stepDistance:  0.01,
	}
	reactor := &fakeAPIDumpReactor{now: 3.0}
	controller := NewDumpStepperController(NewStepperDump(queueSource), NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		_ = eventtime
		return map[string]interface{}{"sample": true}
	}, nil, 0.5))

	steps := controller.StepQueue(0, uint64(1)<<63)
	if len(steps) != 1 || steps[0].FirstClock != 20 {
		t.Fatalf("unexpected step queue %#v", steps)
	}
	msg := controller.LogMessage(steps)
	if msg == "" {
		t.Fatal("expected non-empty log message")
	}
	apiSource := &fakeStepperSource{
		batches:       [][]StepQueueEntry{{{FirstClock: 20, LastClock: 25, StartPosition: 2, Interval: 6, StepCount: 7, Add: 8}}},
		name:          "stepper_x",
		mcuName:       "mcu",
		clockToTime:   func(clock int64) float64 { return float64(clock) / 10.0 },
		startPosition: 1.25,
		stepDistance:  0.01,
	}
	apiController := NewDumpStepperController(NewStepperDump(apiSource), NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		_ = eventtime
		return nil
	}, nil, 0.5))
	update := apiController.APIUpdate(3.5)
	if update["start_position"].(float64) != 1.25 {
		t.Fatalf("unexpected api update %#v", update)
	}
	header := apiController.Header()
	if !reflect.DeepEqual(header["header"], []string{"interval", "count", "add"}) {
		t.Fatalf("unexpected header %#v", header)
	}
}

func TestDumpStepperControllerAddsClient(t *testing.T) {
	reactor := &fakeAPIDumpReactor{now: 5.0}
	controller := NewDumpStepperController(NewStepperDump(&fakeStepperSource{
		name:         "stepper_y",
		mcuName:      "mcu",
		clockToTime:  func(clock int64) float64 { return float64(clock) },
		stepDistance: 0.02,
	}), NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		return map[string]interface{}{"time": eventtime}
	}, nil, 0.25))
	client := &fakeAPIDumpClient{}
	controller.AddClient(client, map[string]interface{}{"meta": "x"})
	if reactor.callback == nil {
		t.Fatal("expected timer callback to be registered")
	}
	reactor.callback(5.5)
	if len(client.sent) != 1 {
		t.Fatalf("expected one sent payload, got %d", len(client.sent))
	}
	if client.sent[0]["meta"].(string) != "x" {
		t.Fatalf("unexpected sent payload %#v", client.sent[0])
	}
}
