package probe

import (
	"math"
	"strconv"
	"testing"
)

type fakeProbeRunCommand struct {
	params    map[string]string
	responses []string
}

func (self *fakeProbeRunCommand) Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
	_ = parser
	_ = minval
	_ = maxval
	_ = above
	_ = below
	if value, ok := self.params[name]; ok {
		return value
	}
	if text, ok := _default.(string); ok {
		return text
	}
	return ""
}

func (self *fakeProbeRunCommand) Get_int(name string, _default interface{}, minval *int, maxval *int) int {
	_ = minval
	_ = maxval
	if value, ok := self.params[name]; ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			panic(err)
		}
		return parsed
	}
	switch typed := _default.(type) {
	case int:
		return typed
	default:
		return 0
	}
}

func (self *fakeProbeRunCommand) Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	_ = minval
	_ = maxval
	_ = above
	_ = below
	if value, ok := self.params[name]; ok {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			panic(err)
		}
		return parsed
	}
	switch typed := _default.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func (self *fakeProbeRunCommand) RespondInfo(msg string, log bool) {
	_ = log
	self.responses = append(self.responses, msg)
}

type fakeProbeRunContext struct {
	core        *PrinterProbe
	toolheadPos []float64
	probePos    [][]float64
	probeIndex  int
	moves       [][]interface{}
	moveSpeeds  []float64
	beginCount  int
	endCount    int
	probeSpeeds []float64
}

func (self *fakeProbeRunContext) Core() *PrinterProbe {
	return self.core
}

func (self *fakeProbeRunContext) ToolheadPosition() []float64 {
	return append([]float64{}, self.toolheadPos...)
}

func (self *fakeProbeRunContext) Probe(speed float64) []float64 {
	self.probeSpeeds = append(self.probeSpeeds, speed)
	pos := append([]float64{}, self.probePos[self.probeIndex]...)
	self.probeIndex++
	return pos
}

func (self *fakeProbeRunContext) Move(coord []interface{}, speed float64) {
	copyCoord := make([]interface{}, len(coord))
	copy(copyCoord, coord)
	self.moves = append(self.moves, copyCoord)
	self.moveSpeeds = append(self.moveSpeeds, speed)
}

func (self *fakeProbeRunContext) BeginMultiProbe() {
	self.beginCount++
	self.core.BeginMultiProbe()
}

func (self *fakeProbeRunContext) EndMultiProbe() {
	self.endCount++
	self.core.EndMultiProbe()
}

func TestRunProbeSequenceUsesMedianWithoutNestedMultiProbeNotifications(t *testing.T) {
	ctx := &fakeProbeRunContext{
		core: &PrinterProbe{
			Speed:             5,
			LiftSpeed:         7,
			SampleCount:       2,
			SampleRetractDist: 1.5,
			SamplesResult:     "median",
			SamplesTolerance:  0.5,
			SamplesRetries:    0,
			MultiProbePending: true,
		},
		toolheadPos: []float64{10, 20, 3},
		probePos:    [][]float64{{10, 20, 1.0}, {10, 20, 1.4}},
	}
	cmd := &fakeProbeRunCommand{params: map[string]string{}}

	result := RunProbeSequence(ctx, cmd)

	if ctx.beginCount != 0 || ctx.endCount != 0 {
		t.Fatalf("begin/end counts = %d/%d, want 0/0", ctx.beginCount, ctx.endCount)
	}
	if len(ctx.moves) != 1 {
		t.Fatalf("moves = %#v, want one retract move", ctx.moves)
	}
	if got := ctx.moves[0]; len(got) != 3 || got[0] != 10.0 || got[1] != 20.0 || got[2] != 2.5 {
		t.Fatalf("retract move = %#v", got)
	}
	if len(ctx.moveSpeeds) != 1 || ctx.moveSpeeds[0] != 7 {
		t.Fatalf("move speeds = %v, want [7]", ctx.moveSpeeds)
	}
	if len(ctx.probeSpeeds) != 2 || ctx.probeSpeeds[0] != 5 || ctx.probeSpeeds[1] != 5 {
		t.Fatalf("probe speeds = %v, want [5 5]", ctx.probeSpeeds)
	}
	if math.Abs(result[2]-1.2) > 1e-9 {
		t.Fatalf("median result z = %v, want 1.2", result[2])
	}
	if len(cmd.responses) != 0 {
		t.Fatalf("responses = %#v, want none", cmd.responses)
	}
}

func TestRunProbeSequenceRetriesAfterToleranceFailure(t *testing.T) {
	ctx := &fakeProbeRunContext{
		core: &PrinterProbe{
			Speed:             5,
			LiftSpeed:         7,
			SampleCount:       2,
			SampleRetractDist: 1.5,
			SamplesResult:     "average",
			SamplesTolerance:  0.1,
			SamplesRetries:    1,
		},
		toolheadPos: []float64{10, 20, 3},
		probePos: [][]float64{
			{10, 20, 1.0},
			{10, 20, 1.4},
			{10, 20, 0.9},
			{10, 20, 0.95},
		},
	}
	cmd := &fakeProbeRunCommand{params: map[string]string{}}

	result := RunProbeSequence(ctx, cmd)

	if ctx.beginCount != 1 || ctx.endCount != 1 {
		t.Fatalf("begin/end counts = %d/%d, want 1/1", ctx.beginCount, ctx.endCount)
	}
	if len(cmd.responses) != 1 || cmd.responses[0] != "Probe samples exceed tolerance. Retrying..." {
		t.Fatalf("responses = %#v", cmd.responses)
	}
	if len(ctx.moves) != 3 {
		t.Fatalf("moves = %#v, want 3 retract moves", ctx.moves)
	}
	for i, move := range ctx.moves {
		if len(move) != 3 || move[0] != 10.0 || move[1] != 20.0 {
			t.Fatalf("move[%d] = %#v", i, move)
		}
	}
	if math.Abs(result[2]-0.925) > 1e-9 {
		t.Fatalf("average result z = %v, want 0.925", result[2])
	}
}

func TestRunProbeSequenceUsesCommandOverrides(t *testing.T) {
	ctx := &fakeProbeRunContext{
		core: &PrinterProbe{
			Speed:             5,
			LiftSpeed:         7,
			SampleCount:       1,
			SampleRetractDist: 1.5,
			SamplesResult:     "average",
			SamplesTolerance:  0.5,
			SamplesRetries:    0,
		},
		toolheadPos: []float64{10, 20, 3},
		probePos:    [][]float64{{10, 20, 1.3}, {10, 20, 1.5}, {10, 20, 1.7}},
	}
	cmd := &fakeProbeRunCommand{params: map[string]string{
		"PROBE_SPEED":               "11",
		"LIFT_SPEED":                "13",
		"SAMPLES":                   "3",
		"SAMPLE_RETRACT_DIST":       "2.5",
		"SAMPLES_TOLERANCE":         "1.0",
		"SAMPLES_TOLERANCE_RETRIES": "2",
		"SAMPLES_RESULT":            "median",
	}}

	result := RunProbeSequence(ctx, cmd)

	if len(ctx.probeSpeeds) != 3 || ctx.probeSpeeds[0] != 11 || ctx.probeSpeeds[1] != 11 || ctx.probeSpeeds[2] != 11 {
		t.Fatalf("probe speeds = %v, want all 11", ctx.probeSpeeds)
	}
	if len(ctx.moveSpeeds) != 2 || ctx.moveSpeeds[0] != 13 || ctx.moveSpeeds[1] != 13 {
		t.Fatalf("move speeds = %v, want [13 13]", ctx.moveSpeeds)
	}
	if math.Abs(result[2]-1.5) > 1e-9 {
		t.Fatalf("median result z = %v, want 1.5", result[2])
	}
	if ctx.beginCount != 1 || ctx.endCount != 1 {
		t.Fatalf("begin/end counts = %d/%d, want 1/1", ctx.beginCount, ctx.endCount)
	}
	if len(cmd.responses) != 0 {
		t.Fatalf("responses = %#v, want none", cmd.responses)
	}
}
