package probe

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

type fakeProbeCommand struct {
	params     map[string]string
	responses  []string
	respondRaw []string
}

func (self *fakeProbeCommand) Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
	_ = parser
	_ = minval
	_ = maxval
	_ = above
	_ = below
	if value, ok := self.params[name]; ok {
		return value
	}
	if _default == nil {
		return ""
	}
	if value, ok := _default.(string); ok {
		return value
	}
	return fmt.Sprint(_default)
}

func (self *fakeProbeCommand) Get_int(name string, _default interface{}, minval *int, maxval *int) int {
	_ = minval
	_ = maxval
	if value, ok := self.params[name]; ok {
		parsed, _ := strconv.Atoi(value)
		return parsed
	}
	if _default == nil {
		return 0
	}
	return _default.(int)
}

func (self *fakeProbeCommand) Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	_ = minval
	_ = maxval
	_ = above
	_ = below
	if value, ok := self.params[name]; ok {
		parsed, _ := strconv.ParseFloat(value, 64)
		return parsed
	}
	if _default == nil {
		return 0
	}
	return _default.(float64)
}

func (self *fakeProbeCommand) RespondInfo(msg string, log bool) {
	_ = log
	self.responses = append(self.responses, msg)
}

type fakeProbeCommandContext struct {
	name           string
	core           *PrinterProbe
	toolheadPos    []float64
	lastMoveTime   float64
	queryResults   []int
	queryIndex     int
	probePositions [][]float64
	runProbePos    []float64
	probeIndex     int
	moves          [][]interface{}
	moveSpeeds     []float64
	beginCount     int
	endCount       int
	ensureCount    int
	manualCommand  ProbeCommand
	manualFinalize func([]float64)
	homingOriginZ  float64
	configWrites   []string
	responseLog    []string
}

func (self *fakeProbeCommandContext) Name() string {
	return self.name
}

func (self *fakeProbeCommandContext) Core() *PrinterProbe {
	return self.core
}

func (self *fakeProbeCommandContext) ToolheadPosition() []float64 {
	return append([]float64{}, self.toolheadPos...)
}

func (self *fakeProbeCommandContext) LastMoveTime() float64 {
	return self.lastMoveTime
}

func (self *fakeProbeCommandContext) QueryEndstop(printTime float64) int {
	_ = printTime
	result := self.queryResults[self.queryIndex]
	if self.queryIndex < len(self.queryResults)-1 {
		self.queryIndex++
	}
	return result
}

func (self *fakeProbeCommandContext) Probe(speed float64) []float64 {
	_ = speed
	position := append([]float64{}, self.probePositions[self.probeIndex]...)
	if self.probeIndex < len(self.probePositions)-1 {
		self.probeIndex++
	}
	return position
}

func (self *fakeProbeCommandContext) RunProbeCommand(command ProbeCommand) []float64 {
	_ = command
	return append([]float64{}, self.runProbePos...)
}

func (self *fakeProbeCommandContext) Move(coord []interface{}, speed float64) {
	copied := append([]interface{}{}, coord...)
	self.moves = append(self.moves, copied)
	self.moveSpeeds = append(self.moveSpeeds, speed)
}

func (self *fakeProbeCommandContext) BeginMultiProbe() {
	self.beginCount++
}

func (self *fakeProbeCommandContext) EndMultiProbe() {
	self.endCount++
}

func (self *fakeProbeCommandContext) EnsureNoManualProbe() {
	self.ensureCount++
}

func (self *fakeProbeCommandContext) StartManualProbe(command ProbeCommand, finalize func([]float64)) {
	self.manualCommand = command
	self.manualFinalize = finalize
}

func (self *fakeProbeCommandContext) SetConfig(section string, option string, value string) {
	self.configWrites = append(self.configWrites, fmt.Sprintf("%s.%s=%s", section, option, value))
}

func (self *fakeProbeCommandContext) HomingOriginZ() float64 {
	return self.homingOriginZ
}

func (self *fakeProbeCommandContext) RespondInfo(msg string, log bool) {
	_ = log
	self.responseLog = append(self.responseLog, msg)
}

func TestHandleProbeCommandRecordsLastResult(t *testing.T) {
	ctx := &fakeProbeCommandContext{
		name:        "probe",
		core:        NewPrinterProbe(5, 7, 1, 2, 3, 2, 0, 1, 2, "average", 0.1, 0),
		runProbePos: []float64{10, 20, 1.25},
	}
	cmd := &fakeProbeCommand{params: map[string]string{}}

	if err := HandleProbeCommand(ctx, cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.core.LastZResult != 1.25 {
		t.Fatalf("expected last z result 1.25, got %v", ctx.core.LastZResult)
	}
	if len(cmd.responses) != 1 || !strings.Contains(cmd.responses[0], "Result is z=1.250000") {
		t.Fatalf("unexpected responses %#v", cmd.responses)
	}
}

func TestHandleQueryProbeCommandTracksTriggeredState(t *testing.T) {
	ctx := &fakeProbeCommandContext{
		name:         "probe",
		core:         NewPrinterProbe(5, 7, 1, 2, 3, 2, 0, 1, 2, "average", 0.1, 0),
		lastMoveTime: 42,
		queryResults: []int{0, 1},
	}
	cmd := &fakeProbeCommand{params: map[string]string{"COUNT": "2"}}

	if err := HandleQueryProbeCommand(ctx, cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ctx.core.LastState {
		t.Fatalf("expected last state to be triggered")
	}
	if len(cmd.responses) != 2 || cmd.responses[0] != "probe: open" || cmd.responses[1] != "probe: TRIGGERED" {
		t.Fatalf("unexpected responses %#v", cmd.responses)
	}
}

func TestHandleProbeCalibrateCommandStartsManualProbeAndFinalizes(t *testing.T) {
	ctx := &fakeProbeCommandContext{
		name:        "probe",
		core:        NewPrinterProbe(5, 7, 1, 2, 2.75, 2, 0, 1, 2, "average", 0.1, 0),
		runProbePos: []float64{10, 20, 1.5},
	}
	cmd := &fakeProbeCommand{params: map[string]string{}}

	if err := HandleProbeCalibrateCommand(ctx, cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ensureCount != 1 {
		t.Fatalf("expected manual probe precheck, got %d", ctx.ensureCount)
	}
	if ctx.core.ProbeCalibrateZ != 1.5 {
		t.Fatalf("expected probe calibration z 1.5, got %v", ctx.core.ProbeCalibrateZ)
	}
	if len(ctx.moves) != 2 {
		t.Fatalf("expected two moves, got %#v", ctx.moves)
	}
	if ctx.moves[0][2] != 6.5 || ctx.moveSpeeds[0] != 7 {
		t.Fatalf("unexpected lift move %#v at %v", ctx.moves[0], ctx.moveSpeeds[0])
	}
	if ctx.moves[1][0] != 11.0 || ctx.moves[1][1] != 22.0 || ctx.moveSpeeds[1] != 5 {
		t.Fatalf("unexpected nozzle move %#v at %v", ctx.moves[1], ctx.moveSpeeds[1])
	}
	if ctx.manualFinalize == nil {
		t.Fatal("expected manual probe callback")
	}
	ctx.manualFinalize([]float64{0, 0, 1.0})
	if len(ctx.configWrites) != 1 || ctx.configWrites[0] != "probe.z_offset=0.500" {
		t.Fatalf("unexpected config writes %#v", ctx.configWrites)
	}
	if len(ctx.responseLog) != 1 || !strings.Contains(ctx.responseLog[0], "probe: z_offset: 0.500") {
		t.Fatalf("unexpected response log %#v", ctx.responseLog)
	}
}

func TestHandleProbeAccuracyCommandUsesMultiProbeFlow(t *testing.T) {
	ctx := &fakeProbeCommandContext{
		name:        "probe",
		core:        NewPrinterProbe(5, 7, 1, 2, 3, 2, 0, 1, 2, "average", 0.1, 0),
		toolheadPos: []float64{1, 2, 3},
		probePositions: [][]float64{
			{1, 2, 0.2},
			{1, 2, 0.3},
		},
	}
	cmd := &fakeProbeCommand{params: map[string]string{"SAMPLES": "2"}}

	if err := HandleProbeAccuracyCommand(ctx, cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.beginCount != 1 || ctx.endCount != 1 {
		t.Fatalf("expected begin/end multiprobe once, got %d/%d", ctx.beginCount, ctx.endCount)
	}
	if len(ctx.moves) != 2 {
		t.Fatalf("expected retract moves for both samples, got %#v", ctx.moves)
	}
	if len(cmd.responses) != 2 || !strings.Contains(cmd.responses[1], "probe accuracy results") {
		t.Fatalf("unexpected responses %#v", cmd.responses)
	}
}

func TestHandleZOffsetApplyProbeCommandWritesConfig(t *testing.T) {
	ctx := &fakeProbeCommandContext{
		name:          "probe",
		core:          NewPrinterProbe(5, 7, 1, 2, 2.75, 2, 0, 1, 2, "average", 0.1, 0),
		homingOriginZ: 0.25,
	}

	if err := HandleZOffsetApplyProbeCommand(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.configWrites) != 1 || ctx.configWrites[0] != "probe.z_offset=2.5000" {
		t.Fatalf("unexpected config writes %#v", ctx.configWrites)
	}
	if len(ctx.responseLog) != 1 || !strings.Contains(ctx.responseLog[0], "probe: z_offset: 2.500") {
		t.Fatalf("unexpected response log %#v", ctx.responseLog)
	}
}
