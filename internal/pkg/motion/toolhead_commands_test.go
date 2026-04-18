package motion

import (
	"strings"
	"testing"
)

type fakeToolheadCommandContext struct {
	settings          ToolheadVelocitySettings
	junctionDeviation float64
	maxAccelToDecel   float64
	dwellCalls        []float64
	waitMovesCalls    int
	rolloverInfos     []string
}

func (self *fakeToolheadCommandContext) Dwell(delay float64) {
	self.dwellCalls = append(self.dwellCalls, delay)
}

func (self *fakeToolheadCommandContext) WaitMoves() {
	self.waitMovesCalls++
}

func (self *fakeToolheadCommandContext) VelocitySettings() ToolheadVelocitySettings {
	return self.settings
}

func (self *fakeToolheadCommandContext) ApplyVelocityLimitResult(result ToolheadVelocityLimitResult) {
	self.settings = result.Settings
	self.junctionDeviation = result.JunctionDeviation
	self.maxAccelToDecel = result.MaxAccelToDecel
}

func (self *fakeToolheadCommandContext) SetRolloverInfo(msg string) {
	self.rolloverInfos = append(self.rolloverInfos, msg)
}

type fakeToolheadCommand struct {
	params       map[string]float64
	commandline  string
	infoMessages []string
}

type fakeToolheadVelocityConfig struct {
	values map[string]float64
}

func (self *fakeToolheadVelocityConfig) Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	_, _, _, _, _, _ = minval, maxval, above, below, noteValid, option
	if value, ok := self.values[option]; ok {
		return value
	}
	if value, ok := default1.(float64); ok {
		return value
	}
	return 0.0
}

func (self *fakeToolheadCommand) Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	if value, ok := self.params[name]; ok {
		if minval != nil && value < *minval {
			return *minval
		}
		if maxval != nil && value > *maxval {
			return *maxval
		}
		if above != nil && value <= *above {
			return *above
		}
		if below != nil && value >= *below {
			return *below
		}
		return value
	}
	if _default != nil {
		if value, ok := _default.(float64); ok {
			return value
		}
	}
	return 0.0
}

func (self *fakeToolheadCommand) Get_commandline() string {
	return self.commandline
}

func (self *fakeToolheadCommand) RespondInfo(msg string, log bool) {
	self.infoMessages = append(self.infoMessages, msg)
}

func TestBuildToolheadInitialVelocityResultCalculatesDerivedFields(t *testing.T) {
	result := BuildToolheadInitialVelocityResult(ToolheadVelocitySettings{
		MaxVelocity:           100.0,
		MaxAccel:              40.0,
		RequestedAccelToDecel: 60.0,
		SquareCornerVelocity:  5.0,
	})

	if !almostEqualFloat64(result.Settings.MaxVelocity, 100.0) || !almostEqualFloat64(result.Settings.MaxAccel, 40.0) {
		t.Fatalf("unexpected settings %#v", result.Settings)
	}
	if !almostEqualFloat64(result.MaxAccelToDecel, 40.0) {
		t.Fatalf("expected max accel to decel clamp to 40.0, got %v", result.MaxAccelToDecel)
	}
	if result.JunctionDeviation == 0.0 {
		t.Fatal("expected junction deviation to be calculated")
	}
	if result.Summary == "" {
		t.Fatal("expected summary string")
	}
}

func TestReadToolheadVelocitySettingsUsesLegacyDefaults(t *testing.T) {
	settings := ReadToolheadVelocitySettings(&fakeToolheadVelocityConfig{values: map[string]float64{
		"max_velocity": 250.0,
		"max_accel":    5000.0,
	}})

	if !almostEqualFloat64(settings.MaxVelocity, 250.0) || !almostEqualFloat64(settings.MaxAccel, 5000.0) {
		t.Fatalf("unexpected toolhead settings %#v", settings)
	}
	if !almostEqualFloat64(settings.RequestedAccelToDecel, 2500.0) {
		t.Fatalf("expected default accel-to-decel to be half max accel, got %f", settings.RequestedAccelToDecel)
	}
	if !almostEqualFloat64(settings.SquareCornerVelocity, 5.0) {
		t.Fatalf("expected default square corner velocity 5.0, got %f", settings.SquareCornerVelocity)
	}
}

func TestReadToolheadVelocitySettingsUsesMinimumCruiseRatioWhenProvided(t *testing.T) {
	settings := ReadToolheadVelocitySettings(&fakeToolheadVelocityConfig{values: map[string]float64{
		"max_velocity":         250.0,
		"max_accel":            5000.0,
		"minimum_cruise_ratio": 0.2,
	}})

	if !almostEqualFloat64(settings.RequestedAccelToDecel, 4000.0) {
		t.Fatalf("expected accel-to-decel derived from minimum_cruise_ratio, got %f", settings.RequestedAccelToDecel)
	}
}

func TestDefaultToolheadSupportModulesReturnsCopy(t *testing.T) {
	modules := DefaultToolheadSupportModules()
	if got, want := strings.Join(modules, ","), "gcode_move,homing,statistics,idle_timeout,manual_probe,tuning_tower"; got != want {
		t.Fatalf("unexpected support modules %q", got)
	}
	modules[0] = "changed"
	if next := DefaultToolheadSupportModules()[0]; next != "gcode_move" {
		t.Fatalf("expected independent copy of support modules, got %q", next)
	}
}

func TestHandleToolheadG4CommandDwellsMilliseconds(t *testing.T) {
	context := &fakeToolheadCommandContext{}
	command := &fakeToolheadCommand{params: map[string]float64{"P": 1500.0}, commandline: "G4 P1500"}

	if err := HandleToolheadG4Command(context, command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(context.dwellCalls) != 1 || !almostEqualFloat64(context.dwellCalls[0], 1.5) {
		t.Fatalf("unexpected dwell calls %#v", context.dwellCalls)
	}
}

func TestHandleToolheadM400CommandWaitsMoves(t *testing.T) {
	context := &fakeToolheadCommandContext{}

	if err := HandleToolheadM400Command(context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if context.waitMovesCalls != 1 {
		t.Fatalf("expected one wait-moves call, got %d", context.waitMovesCalls)
	}
}

func TestHandleToolheadSetVelocityLimitCommandUpdatesState(t *testing.T) {
	context := &fakeToolheadCommandContext{settings: ToolheadVelocitySettings{
		MaxVelocity:           100.0,
		MaxAccel:              1000.0,
		RequestedAccelToDecel: 500.0,
		SquareCornerVelocity:  5.0,
	}}
	command := &fakeToolheadCommand{params: map[string]float64{
		"VELOCITY":               150.0,
		"ACCEL":                  2000.0,
		"SQUARE_CORNER_VELOCITY": 8.0,
		"ACCEL_TO_DECEL":         1200.0,
	}}

	result, queryOnly := HandleToolheadSetVelocityLimitCommand(context, command)

	if queryOnly {
		t.Fatal("expected update command, got query-only result")
	}
	if !almostEqualFloat64(context.settings.MaxVelocity, 150.0) || !almostEqualFloat64(context.settings.MaxAccel, 2000.0) || !almostEqualFloat64(context.settings.SquareCornerVelocity, 8.0) {
		t.Fatalf("unexpected updated settings %#v", context.settings)
	}
	if !almostEqualFloat64(context.settings.RequestedAccelToDecel, 1200.0) {
		t.Fatalf("unexpected accel-to-decel %.6f", context.settings.RequestedAccelToDecel)
	}
	if len(context.rolloverInfos) != 1 || context.rolloverInfos[0] == "" {
		t.Fatalf("expected rollover info, got %#v", context.rolloverInfos)
	}
	if result.Summary == "" {
		t.Fatal("expected summary string")
	}
}

func TestHandleToolheadSetVelocityLimitCommandAcceptsMinimumCruiseRatio(t *testing.T) {
	context := &fakeToolheadCommandContext{settings: ToolheadVelocitySettings{
		MaxVelocity:           100.0,
		MaxAccel:              1000.0,
		RequestedAccelToDecel: 500.0,
		SquareCornerVelocity:  5.0,
	}}
	command := &fakeToolheadCommand{params: map[string]float64{
		"ACCEL":                2000.0,
		"MINIMUM_CRUISE_RATIO": 0.25,
	}}

	_, queryOnly := HandleToolheadSetVelocityLimitCommand(context, command)

	if queryOnly {
		t.Fatal("expected update command, got query-only result")
	}
	if !almostEqualFloat64(context.settings.MaxAccel, 2000.0) {
		t.Fatalf("unexpected max accel %.6f", context.settings.MaxAccel)
	}
	if !almostEqualFloat64(context.settings.RequestedAccelToDecel, 1500.0) {
		t.Fatalf("expected accel-to-decel derived from minimum_cruise_ratio, got %.6f", context.settings.RequestedAccelToDecel)
	}
}

func TestHandleToolheadSetVelocityLimitCommandDetectsQueryOnly(t *testing.T) {
	context := &fakeToolheadCommandContext{settings: ToolheadVelocitySettings{
		MaxVelocity:           100.0,
		MaxAccel:              1000.0,
		RequestedAccelToDecel: 500.0,
		SquareCornerVelocity:  5.0,
	}}
	command := &fakeToolheadCommand{}

	_, queryOnly := HandleToolheadSetVelocityLimitCommand(context, command)

	if !queryOnly {
		t.Fatal("expected query-only detection when no parameters are supplied")
	}
	if len(context.rolloverInfos) != 1 {
		t.Fatalf("expected rollover info on query, got %#v", context.rolloverInfos)
	}
}

func TestHandleToolheadM204CommandUpdatesAcceleration(t *testing.T) {
	context := &fakeToolheadCommandContext{settings: ToolheadVelocitySettings{
		MaxVelocity:           100.0,
		MaxAccel:              1000.0,
		RequestedAccelToDecel: 500.0,
		SquareCornerVelocity:  5.0,
	}}
	command := &fakeToolheadCommand{params: map[string]float64{"P": 2000.0, "T": 1500.0}, commandline: "M204 P2000 T1500"}

	if err := HandleToolheadM204Command(context, command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !almostEqualFloat64(context.settings.MaxAccel, 1500.0) {
		t.Fatalf("expected max accel 1500.0, got %f", context.settings.MaxAccel)
	}
	if context.junctionDeviation == 0.0 || context.maxAccelToDecel == 0.0 {
		t.Fatalf("expected derived acceleration state to update, got jd=%f max=%f", context.junctionDeviation, context.maxAccelToDecel)
	}
}

func TestHandleToolheadM204CommandReportsInvalidSyntax(t *testing.T) {
	context := &fakeToolheadCommandContext{settings: ToolheadVelocitySettings{
		MaxVelocity:           100.0,
		MaxAccel:              1000.0,
		RequestedAccelToDecel: 500.0,
		SquareCornerVelocity:  5.0,
	}}
	command := &fakeToolheadCommand{commandline: "M204"}

	if err := HandleToolheadM204Command(context, command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(command.infoMessages) != 1 || !strings.Contains(command.infoMessages[0], "Invalid M204 command") {
		t.Fatalf("expected invalid syntax response, got %#v", command.infoMessages)
	}
}
