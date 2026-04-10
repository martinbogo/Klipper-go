package bedmesh

import (
	"reflect"
	"testing"
)

func TestGenerateProbePointsRectangularBedSerpentine(t *testing.T) {
	result, err := GenerateProbePoints(nil, nil, []float64{0, 0}, []float64{20, 10}, 3, 2, nil)
	if err != nil {
		t.Fatalf("GenerateProbePoints() error = %v", err)
	}
	want := [][]float64{{0, 0}, {10, 0}, {20, 0}, {20, 10}, {10, 10}, {0, 10}}
	if !reflect.DeepEqual(result.Points, want) {
		t.Fatalf("points = %#v, want %#v", result.Points, want)
	}
	if len(result.Substitutions) != 0 {
		t.Fatalf("expected no substitutions, got %#v", result.Substitutions)
	}
}

func TestGenerateProbePointsFaultyRegionProducesOrderedSubstitution(t *testing.T) {
	result, err := GenerateProbePoints(nil, nil, []float64{0, 0}, []float64{20, 20}, 3, 3, []FaultyRegion{{Min: []float64{9, 9}, Max: []float64{11, 11}}})
	if err != nil {
		t.Fatalf("GenerateProbePoints() error = %v", err)
	}
	if len(result.Substitutions) != 1 {
		t.Fatalf("expected 1 substitution, got %#v", result.Substitutions)
	}
	sub := result.Substitutions[0]
	if sub.Index != 4 {
		t.Fatalf("substitution index = %d, want 4", sub.Index)
	}
	wantPoints := [][]float64{{11, 10}, {10, 9}, {10, 11}, {9, 10}}
	if !reflect.DeepEqual(sub.Points, wantPoints) {
		t.Fatalf("substitution points = %#v, want %#v", sub.Points, wantPoints)
	}
}

func TestNormalizeMeshAlgorithm(t *testing.T) {
	t.Run("forces direct when interpolation disabled", func(t *testing.T) {
		got, forced, err := NormalizeMeshAlgorithm("bicubic", 0, 0, 5, 5)
		if err != nil {
			t.Fatalf("NormalizeMeshAlgorithm() error = %v", err)
		}
		if got != "direct" || !forced {
			t.Fatalf("NormalizeMeshAlgorithm() = (%q, %v), want (direct, true)", got, forced)
		}
	})

	t.Run("forces lagrange for small bicubic grids", func(t *testing.T) {
		got, forced, err := NormalizeMeshAlgorithm("bicubic", 2, 2, 3, 4)
		if err != nil {
			t.Fatalf("NormalizeMeshAlgorithm() error = %v", err)
		}
		if got != "lagrange" || !forced {
			t.Fatalf("NormalizeMeshAlgorithm() = (%q, %v), want (lagrange, true)", got, forced)
		}
	})

	t.Run("rejects excessive lagrange probe counts", func(t *testing.T) {
		_, _, err := NormalizeMeshAlgorithm("lagrange", 1, 1, 7, 6)
		if err == nil {
			t.Fatal("expected error for oversized lagrange probe count")
		}
	})
}
