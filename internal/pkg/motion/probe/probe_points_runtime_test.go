package probe

import (
	"reflect"
	"testing"
)

type fakeProbePointsCommand struct {
	params    map[string]string
	responses []string
}

func (self *fakeProbePointsCommand) Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
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

func (self *fakeProbePointsCommand) Get_int(name string, _default interface{}, minval *int, maxval *int) int {
	_ = name
	_ = _default
	_ = minval
	_ = maxval
	return 0
}

func (self *fakeProbePointsCommand) Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	_ = name
	_ = _default
	_ = minval
	_ = maxval
	_ = above
	_ = below
	return 0
}

func (self *fakeProbePointsCommand) RespondInfo(msg string, log bool) {
	_ = log
	self.responses = append(self.responses, msg)
}

type fakeProbePointsAutomaticProbe struct {
	liftSpeed  float64
	offsets    []float64
	results    [][]float64
	runIndex   int
	beginCount int
	endCount   int
}

func (self *fakeProbePointsAutomaticProbe) GetLiftSpeed(command ProbeCommand) float64 {
	_ = command
	return self.liftSpeed
}

func (self *fakeProbePointsAutomaticProbe) GetOffsets() []float64 {
	return append([]float64{}, self.offsets...)
}

func (self *fakeProbePointsAutomaticProbe) BeginMultiProbe() {
	self.beginCount++
}

func (self *fakeProbePointsAutomaticProbe) EndMultiProbe() {
	self.endCount++
}

func (self *fakeProbePointsAutomaticProbe) RunProbe(command ProbeCommand) []float64 {
	_ = command
	result := append([]float64{}, self.results[self.runIndex]...)
	self.runIndex++
	return result
}

type fakeProbePointsRuntime struct {
	automaticProbe   ProbePointsAutomaticProbe
	moves            [][]interface{}
	speeds           []float64
	touchCount       int
	manualStartCount int
	ensureCount      int
	manualFinalize   func([]float64)
}

func (self *fakeProbePointsRuntime) EnsureNoManualProbe() {
	self.ensureCount++
}

func (self *fakeProbePointsRuntime) LookupAutomaticProbe() ProbePointsAutomaticProbe {
	return self.automaticProbe
}

func (self *fakeProbePointsRuntime) Move(coord interface{}, speed float64) {
	typed := coord.([]interface{})
	copyCoord := make([]interface{}, len(typed))
	copy(copyCoord, typed)
	self.moves = append(self.moves, copyCoord)
	self.speeds = append(self.speeds, speed)
}

func (self *fakeProbePointsRuntime) TouchLastMoveTime() {
	self.touchCount++
}

func (self *fakeProbePointsRuntime) StartManualProbe(finalize func([]float64)) {
	self.manualStartCount++
	self.manualFinalize = finalize
}

func TestProbePointsHelperStartProbeAutomaticFlow(t *testing.T) {
	var finalizeOffsets []float64
	var finalizeResults [][]float64
	finalizeCalls := 0
	helper := NewProbePointsHelper("bed_mesh", func(offsets []float64, results [][]float64) string {
		finalizeCalls++
		finalizeOffsets = append([]float64{}, offsets...)
		finalizeResults = append(finalizeResults[:0], results...)
		return "done"
	}, [][]float64{{10, 20}, {30, 40}}, 5, 50)
	helper.UseXYOffsets(true)
	runtime := &fakeProbePointsRuntime{
		automaticProbe: &fakeProbePointsAutomaticProbe{
			liftSpeed: 12,
			offsets:   []float64{1, 2, 3},
			results:   [][]float64{{9, 18, 1.10}, {29, 38, 1.20}},
		},
	}
	command := &fakeProbePointsCommand{params: map[string]string{"METHOD": "automatic"}}

	helper.StartProbe(runtime, command)

	probe := runtime.automaticProbe.(*fakeProbePointsAutomaticProbe)
	if runtime.ensureCount != 1 {
		t.Fatalf("EnsureNoManualProbe() calls = %d, want 1", runtime.ensureCount)
	}
	if probe.beginCount != 1 || probe.endCount != 1 {
		t.Fatalf("multi-probe begin/end = %d/%d, want 1/1", probe.beginCount, probe.endCount)
	}
	if runtime.touchCount != 1 {
		t.Fatalf("TouchLastMoveTime() calls = %d, want 1", runtime.touchCount)
	}
	expectedMoves := [][]interface{}{
		{nil, nil, 5.0},
		{9.0, 18.0},
		{nil, nil, 5.0},
		{29.0, 38.0},
		{nil, nil, 5.0},
	}
	if !reflect.DeepEqual(runtime.moves, expectedMoves) {
		t.Fatalf("moves = %#v, want %#v", runtime.moves, expectedMoves)
	}
	expectedSpeeds := []float64{12, 50, 50, 50, 50}
	if !reflect.DeepEqual(runtime.speeds, expectedSpeeds) {
		t.Fatalf("speeds = %v, want %v", runtime.speeds, expectedSpeeds)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", finalizeCalls)
	}
	if got, want := finalizeOffsets, []float64{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("finalize offsets = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(finalizeResults, [][]float64{{9, 18, 1.10}, {29, 38, 1.20}}) {
		t.Fatalf("finalize results = %v", finalizeResults)
	}
	if helper.ResultCount() != 2 {
		t.Fatalf("ResultCount() = %d, want 2", helper.ResultCount())
	}
}

func TestProbePointsHelperManualFlow(t *testing.T) {
	var finalizeOffsets []float64
	var finalizeResults [][]float64
	finalizeCalls := 0
	helper := NewProbePointsHelper("bed_mesh", func(offsets []float64, results [][]float64) string {
		finalizeCalls++
		finalizeOffsets = append([]float64{}, offsets...)
		finalizeResults = append(finalizeResults[:0], results...)
		return "done"
	}, [][]float64{{10, 20}, {30, 40}}, 5, 50)
	runtime := &fakeProbePointsRuntime{}
	command := &fakeProbePointsCommand{params: map[string]string{"METHOD": "manual"}}

	helper.StartProbe(runtime, command)
	if runtime.manualStartCount != 1 {
		t.Fatalf("manual probe starts after StartProbe() = %d, want 1", runtime.manualStartCount)
	}
	if runtime.touchCount != 0 {
		t.Fatalf("TouchLastMoveTime() before results = %d, want 0", runtime.touchCount)
	}
	runtime.manualFinalize([]float64{10, 20, 1.50})
	if runtime.manualStartCount != 2 {
		t.Fatalf("manual probe starts after first callback = %d, want 2", runtime.manualStartCount)
	}
	if runtime.touchCount != 0 {
		t.Fatalf("TouchLastMoveTime() before completion = %d, want 0", runtime.touchCount)
	}
	runtime.manualFinalize([]float64{30, 40, 1.25})

	if runtime.ensureCount != 1 {
		t.Fatalf("EnsureNoManualProbe() calls = %d, want 1", runtime.ensureCount)
	}
	if runtime.touchCount != 1 {
		t.Fatalf("TouchLastMoveTime() calls = %d, want 1", runtime.touchCount)
	}
	expectedMoves := [][]interface{}{
		{nil, nil, 5.0},
		{10.0, 20.0},
		{nil, nil, 5.0},
		{30.0, 40.0},
		{nil, nil, 5.0},
	}
	if !reflect.DeepEqual(runtime.moves, expectedMoves) {
		t.Fatalf("moves = %#v, want %#v", runtime.moves, expectedMoves)
	}
	expectedSpeeds := []float64{50, 50, 50, 50, 50}
	if !reflect.DeepEqual(runtime.speeds, expectedSpeeds) {
		t.Fatalf("speeds = %v, want %v", runtime.speeds, expectedSpeeds)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", finalizeCalls)
	}
	if got, want := finalizeOffsets, []float64{0, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("finalize offsets = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(finalizeResults, [][]float64{{10, 20, 1.50}, {30, 40, 1.25}}) {
		t.Fatalf("finalize results = %v", finalizeResults)
	}
}

func TestProbePointsHelperAutomaticRetryFlow(t *testing.T) {
	finalizeCalls := 0
	helper := NewProbePointsHelper("bed_mesh", func(offsets []float64, results [][]float64) string {
		_ = offsets
		_ = results
		finalizeCalls++
		if finalizeCalls == 1 {
			return "retry"
		}
		return "done"
	}, [][]float64{{10, 20}}, 5, 50)
	helper.UseXYOffsets(true)
	runtime := &fakeProbePointsRuntime{
		automaticProbe: &fakeProbePointsAutomaticProbe{
			liftSpeed: 12,
			offsets:   []float64{1, 2, 3},
			results:   [][]float64{{9, 18, 1.10}, {9, 18, 1.05}},
		},
	}
	command := &fakeProbePointsCommand{params: map[string]string{"METHOD": "automatic"}}

	helper.StartProbe(runtime, command)

	probe := runtime.automaticProbe.(*fakeProbePointsAutomaticProbe)
	if finalizeCalls != 2 {
		t.Fatalf("finalize calls = %d, want 2", finalizeCalls)
	}
	if probe.beginCount != 1 || probe.endCount != 1 {
		t.Fatalf("multi-probe begin/end = %d/%d, want 1/1", probe.beginCount, probe.endCount)
	}
	if runtime.touchCount != 2 {
		t.Fatalf("TouchLastMoveTime() calls = %d, want 2", runtime.touchCount)
	}
	expectedMoves := [][]interface{}{
		{nil, nil, 5.0},
		{9.0, 18.0},
		{nil, nil, 5.0},
		{9.0, 18.0},
		{nil, nil, 5.0},
	}
	if !reflect.DeepEqual(runtime.moves, expectedMoves) {
		t.Fatalf("moves = %#v, want %#v", runtime.moves, expectedMoves)
	}
	expectedSpeeds := []float64{12, 50, 50, 50, 50}
	if !reflect.DeepEqual(runtime.speeds, expectedSpeeds) {
		t.Fatalf("speeds = %v, want %v", runtime.speeds, expectedSpeeds)
	}
}
