package bedmesh

import (
	"strings"
	"testing"
)

func TestBuildMeshSafelyBuildsMesh(t *testing.T) {
	var zMesh *ZMesh
	zMesh = &ZMesh{
		Sample: func(zMatrix [][]float64) {
			zMesh.Mesh_matrix = zMatrix
		},
	}

	probedMatrix := [][]float64{{0.1, 0.2}, {0.3, 0.4}}
	if err := BuildMeshSafely(zMesh, probedMatrix); err != nil {
		t.Fatalf("BuildMeshSafely() error = %v", err)
	}
	if len(zMesh.Probed_matrix) != 2 || len(zMesh.Mesh_matrix) != 2 {
		t.Fatalf("BuildMeshSafely() did not populate matrices: probed=%v mesh=%v", zMesh.Probed_matrix, zMesh.Mesh_matrix)
	}
	if zMesh.Avg_z != 0.25 {
		t.Fatalf("Avg_z = %.3f, want 0.250", zMesh.Avg_z)
	}
}

func TestBuildMeshSafelyConvertsPanicsToErrors(t *testing.T) {
	zMesh := &ZMesh{
		Sample: func(_ [][]float64) {
			panic("boom")
		},
	}

	err := BuildMeshSafely(zMesh, [][]float64{{0.1}})
	if err == nil {
		t.Fatal("BuildMeshSafely() error = nil, want panic conversion")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("BuildMeshSafely() error = %q, want panic text", err)
	}
}
