package bedmesh

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

type stubConfigPairReader struct {
	intLists map[string][]int
	values   map[string]interface{}
	err      error
}

func (self *stubConfigPairReader) Getintlist(option string, _ interface{}, _ string, _ int, _ bool) []int {
	return append([]int(nil), self.intLists[option]...)
}

func (self *stubConfigPairReader) Get(option string, _ interface{}, _ bool) interface{} {
	if self.err != nil {
		panic(self.err)
	}
	return self.values[option]
}

type stubGCodeValueReader struct {
	values map[string]string
}

func (self *stubGCodeValueReader) Get(name string, _ interface{}, _ interface{}, _ *float64, _ *float64, _ *float64, _ *float64) string {
	return self.values[name]
}

func TestNormalizeIntPairDuplicatesSingleValue(t *testing.T) {
	min := 3.0
	pair, err := NormalizeIntPair([]int{5}, "probe_count", &min, nil)
	if err != nil {
		t.Fatalf("NormalizeIntPair() error = %v", err)
	}
	want := []int{5, 5}
	if !reflect.DeepEqual(pair, want) {
		t.Fatalf("NormalizeIntPair() = %#v, want %#v", pair, want)
	}
}

func TestParseIntPairRejectsMalformedInput(t *testing.T) {
	if _, err := ParseIntPair("3,4,5", "PROBE_COUNT", nil, nil); err == nil {
		t.Fatal("expected malformed pair error")
	}
}

func TestParseCoordPairParsesTrimmedValues(t *testing.T) {
	x, y, err := ParseCoordPair(" 12.5, -3.25 ")
	if err != nil {
		t.Fatalf("ParseCoordPair() error = %v", err)
	}
	if x != 12.5 || y != -3.25 {
		t.Fatalf("ParseCoordPair() = (%v, %v), want (12.5, -3.25)", x, y)
	}
}

func TestParseConfigIntPairFormatsBoundsErrors(t *testing.T) {
	config := &stubConfigPairReader{intLists: map[string][]int{"probe_count": {2, 4}}}
	_, err := ParseConfigIntPair(config, "probe_count", 3, 3, 0)
	if err == nil || err.Error() != "Option 'probe_count' in section bed_mesh must have a minimum of 3" {
		t.Fatalf("ParseConfigIntPair() error = %v", err)
	}
}

func TestParseConfigIntPairPreservesMissingOptionPanic(t *testing.T) {
	missing := errors.New("Option 'probe_count' in section 'bed_mesh' must be specified")
	config := &stubConfigPairReader{err: missing}
	defer func() {
		recovered := recover()
		if recovered != missing {
			t.Fatalf("recover() = %#v, want %#v", recovered, missing)
		}
	}()
	_, _ = ParseConfigIntPair(config, "probe_count", 3, 3, 0)
}

func TestParseGCodeIntPairFormatsErrors(t *testing.T) {
	gcmd := &stubGCodeValueReader{values: map[string]string{"PROBE_COUNT": "2,3"}}
	min := 3.0
	_, err := ParseGCodeIntPair(gcmd, "PROBE_COUNT", &min, nil)
	if err == nil || err.Error() != "Parameter 'PROBE_COUNT' must have a minimum of 3" {
		t.Fatalf("ParseGCodeIntPair() error = %v", err)
	}
}

func TestParseGCodeCoordFormatsErrors(t *testing.T) {
	gcmd := &stubGCodeValueReader{values: map[string]string{"MESH_MIN": "1,2,3"}}
	_, _, err := ParseGCodeCoord(gcmd, "MESH_MIN")
	if err == nil || err.Error() != "Unable to parse parameter 'MESH_MIN'" {
		t.Fatalf("ParseGCodeCoord() error = %v", err)
	}
}

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

func TestFinalizeCalibrationBuildsRoundedMatrixAndParams(t *testing.T) {
	result, err := FinalizeCalibration(
		[]float64{0.5, 1.5, 0.25},
		[][]float64{{0.004, 0.004, 1}, {10.004, 0.004, 2}, {10.004, 10.004, 4}, {0.004, 10.004, 3}},
		CalibrationFinalizeConfig{
			MeshConfig: map[string]interface{}{
				"x_count": 2,
				"y_count": 2,
				"algo":    "direct",
			},
		},
	)
	if err != nil {
		t.Fatalf("FinalizeCalibration() error = %v", err)
	}

	wantParams := map[string]float64{
		"min_x": 0.5,
		"max_x": 10.5,
		"min_y": 1.5,
		"max_y": 11.5,
	}
	for key, want := range wantParams {
		got, ok := result.MeshParams[key].(float64)
		if !ok || math.Abs(got-want) > 1e-9 {
			t.Fatalf("mesh param %s = %v, want %v", key, result.MeshParams[key], want)
		}
	}

	wantPositions := [][]float64{{0, 0, 1}, {10, 0, 2}, {10, 10, 4}, {0, 10, 3}}
	if !reflect.DeepEqual(result.CorrectedPositions, wantPositions) {
		t.Fatalf("CorrectedPositions = %#v, want %#v", result.CorrectedPositions, wantPositions)
	}

	wantMatrix := [][]float64{{0.75, 1.75}, {2.75, 3.75}}
	if !reflect.DeepEqual(result.ProbedMatrix, wantMatrix) {
		t.Fatalf("ProbedMatrix = %#v, want %#v", result.ProbedMatrix, wantMatrix)
	}
}

func TestFindProbePointIndexUsesBedMeshTolerance(t *testing.T) {
	points := [][]float64{{10, 20}, {30, 40}, {50, 60}}
	if got := FindProbePointIndex(points, Vec2{X: 30.00005, Y: 40.0000005}); got != 1 {
		t.Fatalf("FindProbePointIndex() = %d, want 1", got)
	}
	if got := FindProbePointIndex(points, Vec2{X: 31, Y: 40}); got != -1 {
		t.Fatalf("FindProbePointIndex() = %d, want -1", got)
	}
}
