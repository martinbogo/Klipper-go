package bedmesh

import (
	"math"
	"reflect"
	"testing"
)

func testMeshParams() map[string]interface{} {
	return map[string]interface{}{
		"min_x":      0.0,
		"max_x":      10.0,
		"min_y":      0.0,
		"max_y":      10.0,
		"x_count":    2,
		"y_count":    2,
		"mesh_x_pps": 0,
		"mesh_y_pps": 0,
		"algo":       "direct",
		"tension":    0.2,
	}
}

func TestZMeshCalcZInterpolatesAndUsesOffsets(t *testing.T) {
	mesh := NewZMesh(testMeshParams())
	mesh.Build_mesh([][]float64{{0, 10}, {20, 30}})

	if got, want := mesh.Avg_z, 15.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("Avg_z = %v, want %v", got, want)
	}
	if got, want := mesh.Calc_z(5, 5), 15.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("Calc_z(5,5) = %v, want %v", got, want)
	}
	mesh.Set_mesh_offsets([]float64{1, 2})
	if got, want := mesh.Calc_z(4, 3), 15.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("Calc_z(4,3) with offsets = %v, want %v", got, want)
	}
	minZ, maxZ := mesh.Get_z_range()
	if got, want := [2]float64{minZ, maxZ}, [2]float64{0, 30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Get_z_range() = %v, want %v", got, want)
	}
}

func TestMoveSplitterSplitFollowsMeshGradient(t *testing.T) {
	mesh := NewZMesh(testMeshParams())
	mesh.Build_mesh([][]float64{{0, 10}, {0, 10}})
	splitter := NewMoveSplitter(1.0, 2.0)
	splitter.Initialize(mesh, 0)
	splitter.Build_move([]float64{0, 0, 0, 0}, []float64{10, 0, 0, 0}, 1.0)

	var got [][]float64
	for !splitter.Traverse_complete {
		next := splitter.Split()
		if next == nil {
			t.Fatal("Split() returned nil before traversal completed")
		}
		got = append(got, append([]float64{}, next...))
	}

	want := [][]float64{
		{2, 0, 2, 0},
		{4, 0, 4, 0},
		{6, 0, 6, 0},
		{8, 0, 8, 0},
		{10, 0, 10, 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("split moves = %v, want %v", got, want)
	}
}

func TestResolveFadeTargetUsesMeshAverageWhenUnset(t *testing.T) {
	mesh := NewZMesh(testMeshParams())
	mesh.Build_mesh([][]float64{{0, 10}, {20, 30}})

	fadeTarget, logFadeComplete, err := ResolveFadeTarget(mesh, true, 50.0, 0.0)
	if err != nil {
		t.Fatalf("ResolveFadeTarget() error = %v", err)
	}
	if !logFadeComplete {
		t.Fatal("ResolveFadeTarget() did not request fade-complete logging")
	}
	if got, want := fadeTarget, 15.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("fadeTarget = %v, want %v", got, want)
	}
}

func TestResolveFadeTargetRejectsOutOfRangeConfiguredTarget(t *testing.T) {
	mesh := NewZMesh(testMeshParams())
	mesh.Build_mesh([][]float64{{0, 10}, {20, 30}})

	if _, _, err := ResolveFadeTarget(mesh, true, 50.0, 100.0); err == nil {
		t.Fatal("expected out-of-range fade target error")
	}
}

func TestCalculateUntransformedPosition(t *testing.T) {
	mesh := NewZMesh(testMeshParams())
	mesh.Build_mesh([][]float64{{0, 10}, {20, 30}})

	t.Run("without mesh subtracts fade target", func(t *testing.T) {
		got := CalculateUntransformedPosition([]float64{1, 2, 3, 4}, nil, 10, 20, 10, 1.5)
		want := []float64{1, 2, 1.5, 4}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("CalculateUntransformedPosition() = %v, want %v", got, want)
		}
	})

	t.Run("with mesh removes active z adjustment", func(t *testing.T) {
		got := CalculateUntransformedPosition([]float64{5, 5, 20, 1}, mesh, 10, 40, 30, 0)
		want := []float64{5, 5, 5, 1}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("CalculateUntransformedPosition() = %v, want %v", got, want)
		}
	})

	t.Run("with mesh after fade completion keeps z unchanged", func(t *testing.T) {
		got := CalculateUntransformedPosition([]float64{5, 5, 50, 1}, mesh, 10, 40, 30, 0)
		want := []float64{5, 5, 50, 1}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("CalculateUntransformedPosition() = %v, want %v", got, want)
		}
	})
}
