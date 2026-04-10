package print

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"
)

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
