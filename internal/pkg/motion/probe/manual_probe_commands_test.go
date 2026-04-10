package probe

import (
	"strings"
	"testing"
)

type fakeManualProbeModuleContext struct {
	startCount    int
	finalize      func([]float64)
	zEndstop      float64
	aEndstop      float64
	bEndstop      float64
	cEndstop      float64
	homingOriginZ float64
	configWrites  []string
	responseLog   []string
}

func (self *fakeManualProbeModuleContext) StartManualProbe(command ProbeCommand, finalize func([]float64)) {
	_ = command
	self.startCount++
	self.finalize = finalize
}

func (self *fakeManualProbeModuleContext) ZPositionEndstop() float64 {
	return self.zEndstop
}

func (self *fakeManualProbeModuleContext) DeltaPositionEndstops() (float64, float64, float64) {
	return self.aEndstop, self.bEndstop, self.cEndstop
}

func (self *fakeManualProbeModuleContext) HomingOriginZ() float64 {
	return self.homingOriginZ
}

func (self *fakeManualProbeModuleContext) SetConfig(section string, option string, value string) {
	self.configWrites = append(self.configWrites, section+"."+option+"="+value)
}

func (self *fakeManualProbeModuleContext) RespondInfo(msg string, log bool) {
	_ = log
	self.responseLog = append(self.responseLog, msg)
}

func TestHandleManualProbeCommandStartsHelper(t *testing.T) {
	ctx := &fakeManualProbeModuleContext{}
	cmd := &fakeProbeCommand{params: map[string]string{}}

	if err := HandleManualProbeCommand(ctx, cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.startCount != 1 || ctx.finalize == nil {
		t.Fatalf("expected manual probe helper to start, got count=%d finalizeSet=%t", ctx.startCount, ctx.finalize != nil)
	}
	ctx.finalize([]float64{0, 0, 1.25})
	if len(ctx.responseLog) != 1 || ctx.responseLog[0] != "Z position is 1.250" {
		t.Fatalf("unexpected responses %#v", ctx.responseLog)
	}
}

func TestHandleZEndstopCalibrateCommandWritesConfig(t *testing.T) {
	ctx := &fakeManualProbeModuleContext{zEndstop: 2.0}
	cmd := &fakeProbeCommand{params: map[string]string{}}

	if err := HandleZEndstopCalibrateCommand(ctx, cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx.finalize([]float64{0, 0, 0.5})
	if len(ctx.configWrites) != 1 || ctx.configWrites[0] != "stepper_z.Position_endstop=1.500" {
		t.Fatalf("unexpected config writes %#v", ctx.configWrites)
	}
	if len(ctx.responseLog) != 1 || !strings.Contains(ctx.responseLog[0], "stepper_z: Position_endstop: 1.500") {
		t.Fatalf("unexpected responses %#v", ctx.responseLog)
	}
}

func TestHandleZOffsetApplyEndstopCommandWritesConfig(t *testing.T) {
	ctx := &fakeManualProbeModuleContext{zEndstop: 2.0, homingOriginZ: 0.25}

	if err := HandleZOffsetApplyEndstopCommand(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.configWrites) != 1 || ctx.configWrites[0] != "stepper_z.Position_endstop=1.750" {
		t.Fatalf("unexpected config writes %#v", ctx.configWrites)
	}
	if len(ctx.responseLog) != 1 || !strings.Contains(ctx.responseLog[0], "stepper_z: Position_endstop: 1.750") {
		t.Fatalf("unexpected responses %#v", ctx.responseLog)
	}
}

func TestHandleZOffsetApplyDeltaEndstopsCommandWritesAllConfigs(t *testing.T) {
	ctx := &fakeManualProbeModuleContext{aEndstop: 3.0, bEndstop: 4.0, cEndstop: 5.0, homingOriginZ: 0.5}

	if err := HandleZOffsetApplyDeltaEndstopsCommand(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.configWrites) != 3 {
		t.Fatalf("expected three config writes, got %#v", ctx.configWrites)
	}
	if ctx.configWrites[0] != "stepper_a.Position_endstop=2.500" || ctx.configWrites[1] != "stepper_b.Position_endstop=3.500" || ctx.configWrites[2] != "stepper_c.Position_endstop=4.500" {
		t.Fatalf("unexpected config writes %#v", ctx.configWrites)
	}
	if len(ctx.responseLog) != 1 || !strings.Contains(ctx.responseLog[0], "stepper_a: position_endstop: 2.500") {
		t.Fatalf("unexpected responses %#v", ctx.responseLog)
	}
}
