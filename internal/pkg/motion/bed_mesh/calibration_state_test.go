package bedmesh

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type stubCalibrationConfigSource struct {
	floatValues map[string]float64
	floatLists  map[string][]float64
	intValues   map[string]int
	intLists    map[string][]int
	values      map[string]interface{}
}

func (s stubCalibrationConfigSource) Getfloat(option string, default1 interface{}, _ float64, _ float64, _ float64, _ float64, _ bool) float64 {
	if value, ok := s.floatValues[option]; ok {
		return value
	}
	switch typed := default1.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func (s stubCalibrationConfigSource) Getfloatlist(option string, default1 interface{}, _ string, _ int, _ bool) []float64 {
	if values, ok := s.floatLists[option]; ok {
		cloned := make([]float64, len(values))
		copy(cloned, values)
		return cloned
	}
	if values, ok := default1.([]float64); ok {
		cloned := make([]float64, len(values))
		copy(cloned, values)
		return cloned
	}
	return nil
}

func (s stubCalibrationConfigSource) Getint(option string, default1 interface{}, _ int, _ int, _ bool) int {
	if value, ok := s.intValues[option]; ok {
		return value
	}
	if value, ok := default1.(int); ok {
		return value
	}
	return 0
}

func (s stubCalibrationConfigSource) Getintlist(option string, _ interface{}, _ string, _ int, _ bool) []int {
	values := s.intLists[option]
	cloned := make([]int, len(values))
	copy(cloned, values)
	return cloned
}

func (s stubCalibrationConfigSource) Get(option string, default1 interface{}, _ bool) interface{} {
	if value, ok := s.values[option]; ok {
		return value
	}
	return default1
}

type stubCalibrationCommandSource struct {
	params map[string]string
}

func (s stubCalibrationCommandSource) Get(name string, defaultValue interface{}, _ interface{}, _ *float64, _ *float64, _ *float64, _ *float64) string {
	if value, ok := s.params[name]; ok {
		return value
	}
	if value, ok := defaultValue.(string); ok {
		return value
	}
	return ""
}

func (s stubCalibrationCommandSource) Get_int(name string, defaultValue interface{}, _ *int, _ *int) int {
	if value, ok := s.params[name]; ok {
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	}
	if value, ok := defaultValue.(int); ok {
		return value
	}
	return 0
}

func (s stubCalibrationCommandSource) Get_float(name string, defaultValue interface{}, _ *float64, _ *float64, _ *float64, _ *float64) float64 {
	if value, ok := s.params[name]; ok {
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return parsed
	}
	if value, ok := defaultValue.(float64); ok {
		return value
	}
	return 0
}

func (s stubCalibrationCommandSource) Parameters() map[string]string {
	cloned := make(map[string]string, len(s.params))
	for key, value := range s.params {
		cloned[key] = value
	}
	return cloned
}

func rectangularCalibrationConfig() stubCalibrationConfigSource {
	return stubCalibrationConfigSource{
		floatValues: map[string]float64{
			"mesh_radius":     0,
			"bicubic_tension": 0.2,
		},
		floatLists: map[string][]float64{
			"mesh_min": {0, 0},
			"mesh_max": {20, 20},
		},
		intLists: map[string][]int{
			"probe_count": {3, 3},
			"mesh_pps":    {2, 2},
		},
		values: map[string]interface{}{
			"algorithm": "lagrange",
		},
	}
}

func TestCalibrationStateRectangularOverridesUseRectangularBranch(t *testing.T) {
	state, err := NewCalibrationState(rectangularCalibrationConfig(), nil)
	if err != nil {
		t.Fatalf("NewCalibrationState() error = %v", err)
	}
	if state.Radius != nil {
		t.Fatalf("Radius = %v, want nil for rectangular beds", *state.Radius)
	}

	changed, err := state.ApplyCommandOverrides(stubCalibrationCommandSource{params: map[string]string{
		"MESH_MIN":    "1,2",
		"MESH_MAX":    "11,12",
		"PROBE_COUNT": "4,5",
	}})
	if err != nil {
		t.Fatalf("ApplyCommandOverrides() error = %v", err)
	}
	if !changed {
		t.Fatal("ApplyCommandOverrides() = false, want true")
	}
	if got, want := state.MeshMin, []float64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("MeshMin = %#v, want %#v", got, want)
	}
	if got, want := state.MeshMax, []float64{11, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("MeshMax = %#v, want %#v", got, want)
	}
	if got, want := state.MeshConfig["x_count"], 4; got != want {
		t.Fatalf("x_count = %v, want %d", got, want)
	}
	if got, want := state.MeshConfig["y_count"], 5; got != want {
		t.Fatalf("y_count = %v, want %d", got, want)
	}
}

func TestCalibrationStateResetRestoresOriginalPointsAndSubstitutions(t *testing.T) {
	cfg := rectangularCalibrationConfig()
	cfg.floatLists["faulty_region_1_min"] = []float64{9, 9}
	cfg.floatLists["faulty_region_1_max"] = []float64{11, 11}
	state, err := NewCalibrationState(cfg, nil)
	if err != nil {
		t.Fatalf("NewCalibrationState() error = %v", err)
	}
	if len(state.Substitutions) == 0 {
		t.Fatal("expected original substitutions for faulty region")
	}

	wantPoints := state.AdjustedPoints()
	wantSubstitutions := clonePointSubstitutions(state.Substitutions)

	state.MeshMin = []float64{0, 0}
	state.MeshMax = []float64{40, 20}
	state.MeshConfig["x_count"] = 5
	state.MeshConfig["y_count"] = 3
	if _, err := state.Refresh(nil); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	state.ResetToOriginal()
	if got := state.AdjustedPoints(); !reflect.DeepEqual(got, wantPoints) {
		t.Fatalf("AdjustedPoints after reset = %#v, want %#v", got, wantPoints)
	}
	if got := state.Substitutions; !reflect.DeepEqual(got, wantSubstitutions) {
		t.Fatalf("Substitutions after reset = %#v, want %#v", got, wantSubstitutions)
	}
}

func TestCalibrationStateRefreshPersistsResolvedZeroReference(t *testing.T) {
	state, err := NewCalibrationState(rectangularCalibrationConfig(), nil)
	if err != nil {
		t.Fatalf("NewCalibrationState() error = %v", err)
	}

	refresh, err := state.Refresh([]float64{10, 10})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if !refresh.ZeroReferenceAttempted || !refresh.ZeroReferenceMatched {
		t.Fatalf("zero reference result = %+v, want attempted and matched", refresh)
	}
	if state.RelativeReferenceIndex == nil || *state.RelativeReferenceIndex != 4 {
		t.Fatalf("RelativeReferenceIndex = %#v, want 4", state.RelativeReferenceIndex)
	}

	state.ResetToOriginal()
	if state.RelativeReferenceIndex == nil || *state.RelativeReferenceIndex != 4 {
		t.Fatalf("RelativeReferenceIndex after reset = %#v, want persisted 4", state.RelativeReferenceIndex)
	}
}
