package fan

import (
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeGenericFanGCode struct {
	muxHandlers map[string]func(printerpkg.Command) error
}

func (self *fakeGenericFanGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
}

func (self *fakeGenericFanGCode) RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string) {
	if self.muxHandlers == nil {
		self.muxHandlers = map[string]func(printerpkg.Command) error{}
	}
	self.muxHandlers[cmd+"\x00"+key+"\x00"+value] = handler
}

func (self *fakeGenericFanGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakeGenericFanGCode) RunScriptFromCommand(script string) {}
func (self *fakeGenericFanGCode) RunScript(script string)            {}
func (self *fakeGenericFanGCode) IsBusy() bool                       { return false }
func (self *fakeGenericFanGCode) Mutex() printerpkg.Mutex            { return nil }
func (self *fakeGenericFanGCode) RespondInfo(msg string, log bool)   {}
func (self *fakeGenericFanGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

func TestLoadConfigGenericFanRegistersMuxCommandAndRestartHandler(t *testing.T) {
	queueMCU := &fakeFanQueueMCU{estimatedValue: 9.0}
	pwmPin := &fakeFanPWMPin{mcu: queueMCU}
	pins := &fakeFanPins{pwm: pwmPin}
	toolhead := &fakeFanToolhead{nextPrintTime: 4.0}
	gcode := &fakeGenericFanGCode{}
	printer := &fakeFanPrinter{
		lookup: map[string]interface{}{
			"pins":     pins,
			"toolhead": toolhead,
		},
		reactor:  &fakeFanReactor{monotonic: 1.0},
		gcode:    gcode,
		webhooks: &fakeFanWebhookRegistry{},
	}
	module := LoadConfigGenericFan(&fakeFanConfig{
		printer: printer,
		name:    "fan_generic box_fan",
		strings: map[string]string{"pin": "PA1"},
		floats: map[string]float64{
			"kick_start_time": 0.0,
		},
		bools: map[string]bool{},
	}).(*PrinterFanGenericModule)

	if module == nil {
		t.Fatalf("expected generic fan module instance")
	}
	if len(pins.pwmPins) != 1 || pins.pwmPins[0] != "PA1" {
		t.Fatalf("unexpected pwm pin setup: %#v", pins.pwmPins)
	}
	if gcode.muxHandlers["SET_FAN_SPEED\x00FAN\x00box_fan"] == nil {
		t.Fatalf("expected SET_FAN_SPEED mux command registration")
	}
	if printer.eventHandlers["project:connect"] == nil || printer.eventHandlers["gcode:request_restart"] == nil {
		t.Fatalf("expected connect and restart handlers to be registered")
	}
	if len(queueMCU.flushCallbacks) != 1 {
		t.Fatalf("expected one flush callback, got %#v", queueMCU.flushCallbacks)
	}

	if err := printer.eventHandlers["project:connect"](nil); err != nil {
		t.Fatalf("connect handler returned error: %v", err)
	}
	if err := gcode.muxHandlers["SET_FAN_SPEED\x00FAN\x00box_fan"](&fakeFanCommand{floats: map[string]float64{"SPEED": 0.5}}); err != nil {
		t.Fatalf("SET_FAN_SPEED returned error: %v", err)
	}
	queueMCU.flushCallbacks[0](4.0, 0)
	if len(pwmPin.setCalls) != 1 {
		t.Fatalf("expected one pwm set call after SET_FAN_SPEED, got %#v", pwmPin.setCalls)
	}
	assertApprox(t, pwmPin.setCalls[0][0], 4.0)
	assertApprox(t, pwmPin.setCalls[0][1], 0.5)

	status := module.Get_status(7.0)
	assertApprox(t, status["speed"], 0.5)
	assertApprox(t, status["rpm"], 0.0)

	if err := printer.eventHandlers["gcode:request_restart"]([]interface{}{6.5}); err != nil {
		t.Fatalf("restart handler returned error: %v", err)
	}
	if len(pwmPin.setCalls) != 2 {
		t.Fatalf("expected second pwm set call after restart, got %#v", pwmPin.setCalls)
	}
	assertApprox(t, pwmPin.setCalls[1][0], 6.5)
	assertApprox(t, pwmPin.setCalls[1][1], 0.0)
	if len(toolhead.notedTimes) == 0 {
		t.Fatalf("expected toolhead activity notifications, got none")
	}
}