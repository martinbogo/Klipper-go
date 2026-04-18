package print

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestFuncConfigSourceUsesCallbacksAndFallbacks(t *testing.T) {
	source := FuncConfigSource{
		Float64Func: func(key string, fallback float64) float64 {
			if key == "speed" {
				return fallback + 1.5
			}
			return fallback
		},
		IntFunc: func(key string, fallback int) int {
			if key == "count" {
				return fallback + 2
			}
			return fallback
		},
		BoolFunc: func(key string, fallback bool) bool {
			if key == "enabled" {
				return !fallback
			}
			return fallback
		},
		Float64SliceFunc: func(key string, fallback []float64) []float64 {
			if key == "points" {
				return []float64{1, 2, 3}
			}
			return append([]float64(nil), fallback...)
		},
		StringFunc: func(key string, fallback string) string {
			if key == "profile" {
				return fallback + "-custom"
			}
			return fallback
		},
	}

	if got := source.Float64("speed", 2.5); math.Abs(got-4.0) > 1e-9 {
		t.Fatalf("Float64() = %v, want 4.0", got)
	}
	if got := source.Int("count", 3); got != 5 {
		t.Fatalf("Int() = %d, want 5", got)
	}
	if got := source.Bool("enabled", true); got {
		t.Fatal("Bool() = true, want false")
	}
	if got := source.String("profile", "leviq3"); got != "leviq3-custom" {
		t.Fatalf("String() = %q, want %q", got, "leviq3-custom")
	}
	if got := source.Float64Slice("points", []float64{9, 9}); !reflect.DeepEqual(got, []float64{1, 2, 3}) {
		t.Fatalf("Float64Slice() = %#v, want %#v", got, []float64{1.0, 2.0, 3.0})
	}

	var fallbackSource FuncConfigSource
	fallback := []float64{4, 5}
	gotFallback := fallbackSource.Float64Slice("missing", fallback)
	if !reflect.DeepEqual(gotFallback, fallback) {
		t.Fatalf("fallback Float64Slice() = %#v, want %#v", gotFallback, fallback)
	}
	gotFallback[0] = 99
	if fallback[0] != 4 {
		t.Fatalf("fallback slice was not cloned: %#v", fallback)
	}
	if got := fallbackSource.String("missing", "default"); got != "default" {
		t.Fatalf("fallback String() = %q, want %q", got, "default")
	}
}

func TestFuncStatusSinkInvokesCallbacks(t *testing.T) {
	var infoCalls, errorCalls int
	var infoMsg, errorMsg string
	sink := FuncStatusSink{
		InfofFunc: func(format string, args ...any) {
			infoCalls++
			infoMsg = format
		},
		ErrorfFunc: func(format string, args ...any) {
			errorCalls++
			errorMsg = format
		},
	}

	sink.Infof("info %s", "message")
	sink.Errorf("error %s", "message")

	if infoCalls != 1 || infoMsg != "info %s" {
		t.Fatalf("Infof callback = (%d, %q), want (1, %q)", infoCalls, infoMsg, "info %s")
	}
	if errorCalls != 1 || errorMsg != "error %s" {
		t.Fatalf("Errorf callback = (%d, %q), want (1, %q)", errorCalls, errorMsg, "error %s")
	}

	var nilSink FuncStatusSink
	nilSink.Infof("ignored")
	nilSink.Errorf("ignored")
}

type fakeLeviQ3Motion struct {
	temperature float64
	samples     []float64
	sampleIndex int
}

func (self *fakeLeviQ3Motion) Is_printer_ready(ctx context.Context) bool {
	_ = ctx
	return true
}

func (self *fakeLeviQ3Motion) Clear_homing_state(ctx context.Context) error {
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Home_axis(ctx context.Context, axis Axis, params HomingParams) error {
	_, _, _ = axis, params, ctx
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Home_rails(ctx context.Context, axes []Axis, params HomingParams) error {
	_, _, _ = axes, params, ctx
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Homing_move(ctx context.Context, axis Axis, target float64, speed float64) error {
	_, _, _ = axis, target, speed
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Set_bed_temperature(ctx context.Context, target float64) error {
	_ = target
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Wait_for_temperature(ctx context.Context, target float64, timeout time.Duration) error {
	_, _ = target, timeout
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) GetHotbedTemp(ctx context.Context) (float64, error) {
	return self.temperature, ctx.Err()
}

func (self *fakeLeviQ3Motion) Lower_probe(ctx context.Context) error {
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Raise_probe(ctx context.Context) error {
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Run_probe(ctx context.Context, position XY) (float64, error) {
	_ = position
	return 0, ctx.Err()
}

func (self *fakeLeviQ3Motion) Wipe_nozzle(ctx context.Context, position XYZ) error {
	_ = position
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Set_gcode_offset(ctx context.Context, z float64) error {
	_ = z
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Current_z_offset(ctx context.Context) float64 {
	_ = ctx
	return 0
}

func (self *fakeLeviQ3Motion) Save_mesh(ctx context.Context, mesh *BedMesh) error {
	_ = mesh
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) Sleep(ctx context.Context, d time.Duration) error {
	_ = d
	return ctx.Err()
}

func (self *fakeLeviQ3Motion) GetAutoZOffsetTemperature(ctx context.Context) (float64, error) {
	return self.temperature, ctx.Err()
}

func (self *fakeLeviQ3Motion) MeasureAutoZOffset(ctx context.Context, helper *LeviQ3Helper) (float64, error) {
	_ = helper
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if self.sampleIndex >= len(self.samples) {
		return 0, nil
	}
	value := self.samples[self.sampleIndex]
	self.sampleIndex++
	return value, nil
}

func newTestLeviQ3Helper(t *testing.T, motion MotionController, config MapConfigSource) *LeviQ3Helper {
	t.Helper()
	helper, err := NewLeviQ3Helper(config, motion, NullStatus{})
	if err != nil {
		t.Fatalf("NewLeviQ3Helper() error = %v", err)
	}
	return helper
}

func TestComputeLeviQ3TemperatureCompensation(t *testing.T) {
	got := ComputeLeviQ3TemperatureCompensation(180.0, 0.16)
	want := (180.0 - 140.0) * 0.16 / 80.0
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("ComputeLeviQ3TemperatureCompensation() = %v, want %v", got, want)
	}
	if got := ComputeLeviQ3TemperatureCompensation(0, 0.16); got != 0 {
		t.Fatalf("expected zero compensation for non-positive temperature, got %v", got)
	}
}

func TestValidateAutoZOffsetSamplesRetriesAndFlagsNoise(t *testing.T) {
	motion := &fakeLeviQ3Motion{samples: []float64{0.1000, 0.1500, 0.1510}}
	helper := newTestLeviQ3Helper(t, motion, MapConfigSource{
		"auto_zoffset_retry_count": 2,
		"max_diff":                 0.0401,
		"noise_diff":               0.0401,
	})

	candidate, samples, err := helper.validate_auto_zoffset_samples(context.Background())
	if err != nil {
		t.Fatalf("validate_auto_zoffset_samples() error = %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
	want := (0.1500 + 0.1510) / 2.0
	if math.Abs(candidate-want) > 1e-9 {
		t.Fatalf("validate_auto_zoffset_samples() = %v, want %v", candidate, want)
	}
	if !helper.is_scratch_notice {
		t.Fatalf("expected scratch notice to be raised when sample range exceeds noise threshold")
	}
}

func TestClampAutoZOffsetUsesRecoveredStepLimit(t *testing.T) {
	helper := newTestLeviQ3Helper(t, nil, MapConfigSource{})
	helper.current_zoffset = 1.25
	got := helper.clamp_auto_zoffset(10.0)
	want := 1.25 + default_auto_zoffset_step_limit
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("clamp_auto_zoffset() = %v, want %v", got, want)
	}
}

func TestPersistentStateRoundTripPreservesCurrentZOffset(t *testing.T) {
	helper := newTestLeviQ3Helper(t, nil, MapConfigSource{})
	mesh := NewBedMesh(XY{X: 15, Y: 15}, XY{X: 45, Y: 45}, XYCount{X: 2, Y: 2}, "persist-test")
	mesh.Set(0, 0, 0.11)
	mesh.Set(1, 0, 0.12)
	mesh.Set(0, 1, 0.13)
	mesh.Set(1, 1, 0.14)
	mesh.points = []ProbePoint{
		{Index: 0, Position: XY{X: 15, Y: 15}, MeasuredZ: 0.11, Valid: true},
		{Index: 1, Position: XY{X: 45, Y: 15}, MeasuredZ: 0.12, Valid: true},
	}
	state := LeviQ3PersistentState{
		SavedZOffset:           0.27,
		LastAutoZOffset:        0.31,
		LastAutoZOffsetRaw:     0.04,
		LastAutoZOffsetSamples: []float64{0.04, 0.05},
		LastAppliedOffsets:     XYZ{X: 1.5, Y: 2.5, Z: 0.27},
		SavedMesh:              mesh,
	}
	helper.RestorePersistentState(state)
	if math.Abs(helper.CurrentZOffset()-0.27) > 1e-9 {
		t.Fatalf("CurrentZOffset() = %v, want 0.27", helper.CurrentZOffset())
	}
	roundTrip := helper.PersistentState()
	if len(roundTrip.LastAutoZOffsetSamples) != 2 {
		t.Fatalf("expected persisted samples to round-trip, got %#v", roundTrip.LastAutoZOffsetSamples)
	}
	if roundTrip.LastAppliedOffsets != state.LastAppliedOffsets {
		t.Fatalf("LastAppliedOffsets = %#v, want %#v", roundTrip.LastAppliedOffsets, state.LastAppliedOffsets)
	}
	if roundTrip.SavedMesh == nil {
		t.Fatal("expected saved mesh to round-trip")
	}
	if !reflect.DeepEqual(roundTrip.SavedMesh.CloneMatrix(), mesh.CloneMatrix()) {
		t.Fatalf("saved mesh matrix = %#v, want %#v", roundTrip.SavedMesh.CloneMatrix(), mesh.CloneMatrix())
	}
	if !reflect.DeepEqual(roundTrip.SavedMesh.points, mesh.points) {
		t.Fatalf("saved mesh points = %#v, want %#v", roundTrip.SavedMesh.points, mesh.points)
	}
}

func TestPersistentStateJSONRoundTripPreservesCurrentZOffset(t *testing.T) {
	helper := newTestLeviQ3Helper(t, nil, MapConfigSource{})
	mesh := NewBedMesh(XY{X: 5, Y: 5}, XY{X: 25, Y: 25}, XYCount{X: 2, Y: 2}, "json-round-trip")
	mesh.Set(0, 0, 0.31)
	mesh.Set(1, 0, 0.32)
	mesh.Set(0, 1, 0.33)
	mesh.Set(1, 1, 0.34)
	helper.RestorePersistentState(LeviQ3PersistentState{
		SavedZOffset:           0.42,
		LastAutoZOffset:        0.43,
		LastAutoZOffsetRaw:     0.04,
		LastAutoZOffsetSamples: []float64{0.04, 0.05, 0.06},
		LastAppliedOffsets:     XYZ{X: 1, Y: 2, Z: 0.42},
		SavedMesh:              mesh,
	})

	payload, err := helper.PersistentStateJSON()
	if err != nil {
		t.Fatalf("PersistentStateJSON() error = %v", err)
	}

	restored := newTestLeviQ3Helper(t, nil, MapConfigSource{})
	if err := restored.RestorePersistentStateFromJSON(payload); err != nil {
		t.Fatalf("RestorePersistentStateFromJSON() error = %v", err)
	}
	if math.Abs(restored.CurrentZOffset()-0.42) > 1e-9 {
		t.Fatalf("CurrentZOffset() = %v, want 0.42", restored.CurrentZOffset())
	}
	roundTrip := restored.PersistentState()
	if roundTrip.LastAppliedOffsets != (XYZ{X: 1, Y: 2, Z: 0.42}) {
		t.Fatalf("LastAppliedOffsets = %#v", roundTrip.LastAppliedOffsets)
	}
	if roundTrip.SavedMesh == nil {
		t.Fatal("expected saved mesh after JSON restore")
	}
	if !reflect.DeepEqual(roundTrip.SavedMesh.CloneMatrix(), mesh.CloneMatrix()) {
		t.Fatalf("saved mesh matrix = %#v, want %#v", roundTrip.SavedMesh.CloneMatrix(), mesh.CloneMatrix())
	}
}

func TestRestorePersistentStateValueRestoresDecodedSaveVariablePayload(t *testing.T) {
	helper := newTestLeviQ3Helper(t, nil, MapConfigSource{})
	encoded, err := json.Marshal(NewLeviQ3PersistedStateRecord(0.55, LeviQ3PersistentState{
		SavedZOffset:           0.55,
		LastAutoZOffset:        0.56,
		LastAutoZOffsetSamples: []float64{0.1, 0.2},
		LastAppliedOffsets:     XYZ{X: 1, Y: 2, Z: 0.55},
	}))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var raw any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if err := helper.RestorePersistentStateValue(raw); err != nil {
		t.Fatalf("RestorePersistentStateValue() error = %v", err)
	}
	if got := helper.CurrentZOffset(); math.Abs(got-0.55) > 1e-9 {
		t.Fatalf("CurrentZOffset() = %v, want 0.55", got)
	}
	if got := helper.PersistentState().LastAppliedOffsets; got != (XYZ{X: 1, Y: 2, Z: 0.55}) {
		t.Fatalf("LastAppliedOffsets = %#v", got)
	}
}

func TestPersistentStateSaveVariableValueReturnsJSONPayload(t *testing.T) {
	helper := newTestLeviQ3Helper(t, nil, MapConfigSource{})
	helper.RestorePersistentState(LeviQ3PersistentState{SavedZOffset: 0.61})

	value, err := helper.PersistentStateSaveVariableValue()
	if err != nil {
		t.Fatalf("PersistentStateSaveVariableValue() error = %v", err)
	}
	if value == "" {
		t.Fatal("expected non-empty save-variable payload")
	}
	var decoded LeviQ3PersistedStateRecord
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if math.Abs(decoded.CurrentZOffset-0.61) > 1e-9 {
		t.Fatalf("CurrentZOffset = %v, want 0.61", decoded.CurrentZOffset)
	}
}

func TestBedMeshSnapshotRoundTripPreservesMatrixAndPoints(t *testing.T) {
	mesh := NewBedMesh(XY{X: 10, Y: 20}, XY{X: 30, Y: 40}, XYCount{X: 2, Y: 2}, "snapshot-test")
	mesh.Set(0, 0, 0.21)
	mesh.Set(1, 0, 0.22)
	mesh.Set(0, 1, 0.23)
	mesh.Set(1, 1, 0.24)
	mesh.points = []ProbePoint{
		{Index: 0, Position: XY{X: 10, Y: 20}, MeasuredZ: 0.21, Valid: true},
		{Index: 1, Position: XY{X: 30, Y: 20}, MeasuredZ: 0.22, Valid: true},
	}

	snapshot := mesh.Snapshot()
	restored := NewBedMeshFromSnapshot(snapshot)
	if restored == nil {
		t.Fatal("expected restored mesh")
	}
	if !reflect.DeepEqual(restored.CloneMatrix(), mesh.CloneMatrix()) {
		t.Fatalf("restored matrix = %#v, want %#v", restored.CloneMatrix(), mesh.CloneMatrix())
	}
	if !reflect.DeepEqual(restored.points, mesh.points) {
		t.Fatalf("restored points = %#v, want %#v", restored.points, mesh.points)
	}
	if restored.source != mesh.source {
		t.Fatalf("restored source = %q, want %q", restored.source, mesh.source)
	}
}
