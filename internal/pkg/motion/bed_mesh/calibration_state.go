package bedmesh

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type CalibrationConfigSource interface {
	ConfigPairReader
	Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64
	Getfloatlist(option string, default1 interface{}, sep string, count int, noteValid bool) []float64
	Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int
}

type CalibrationCommandSource interface {
	Get(name string, defaultValue interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	Get_int(name string, defaultValue interface{}, minval *int, maxval *int) int
	Get_float(name string, defaultValue interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64
	Parameters() map[string]string
}

type CalibrationRefreshResult struct {
	AdjustedPoints         [][]float64
	ForcedLagrange         bool
	ZeroReferenceAttempted bool
	ZeroReferenceMatched   bool
}

type CalibrationState struct {
	Radius                 *float64
	Origin                 []float64
	MeshMin                []float64
	MeshMax                []float64
	RelativeReferenceIndex *int
	FaultyRegions          []FaultyRegion
	Substitutions          []PointSubstitution
	MeshConfig             map[string]interface{}
	Points                 [][]float64

	origRadius                 *float64
	origOrigin                 []float64
	origMeshMin                []float64
	origMeshMax                []float64
	origRelativeReferenceIndex *int
	origMeshConfig             map[string]interface{}
	origPoints                 [][]float64
	origSubstitutions          []PointSubstitution
}

func NewCalibrationState(config CalibrationConfigSource, relativeReferenceIndex *int) (*CalibrationState, error) {
	state := &CalibrationState{
		RelativeReferenceIndex: cloneIntPtr(relativeReferenceIndex),
		FaultyRegions:          []FaultyRegion{},
		MeshConfig:             map[string]interface{}{},
	}

	radius := config.Getfloat("mesh_radius", 0, 0, 0, 0., 0, true)
	minX, minY, maxX, maxY := 0.0, 0.0, 0.0, 0.0
	xCount, yCount := 0, 0
	if radius != 0 {
		roundedRadius := math.Floor(radius*10) / 10
		state.Radius = &roundedRadius
		state.Origin = cloneFloatSlice(config.Getfloatlist("mesh_radius", 0, ",", 2, true))
		xCount = config.Getint("round_probe_count", 5, 3, 0, true)
		yCount = xCount
		if xCount&1 == 0 {
			return nil, fmt.Errorf("bed_mesh: probe_count must be odd for round beds")
		}
		minX = -roundedRadius
		minY = -roundedRadius
		maxX = roundedRadius
		maxY = roundedRadius
	} else {
		pps, err := ParseConfigIntPair(config, "probe_count", 3, 3., 0.)
		if err != nil {
			return nil, err
		}
		xCount = pps[0]
		yCount = pps[1]
		meshMin := config.Getfloatlist("mesh_min", nil, ",", 2, true)
		meshMax := config.Getfloatlist("mesh_max", nil, ",", 2, true)
		minX, minY = meshMin[0], meshMin[1]
		maxX, maxY = meshMax[0], meshMax[1]
		if maxX <= minX || maxY <= minY {
			return nil, fmt.Errorf("bed_mesh: invalid min/max points")
		}
	}

	state.MeshMin = []float64{minX, minY}
	state.MeshMax = []float64{maxX, maxY}
	state.MeshConfig["x_count"] = xCount
	state.MeshConfig["y_count"] = yCount

	pps, err := ParseConfigIntPair(config, "mesh_pps", 2, 0., 0.)
	if err != nil {
		return nil, err
	}
	state.MeshConfig["mesh_x_pps"] = pps[0]
	state.MeshConfig["mesh_y_pps"] = pps[1]
	state.MeshConfig["algo"] = strings.ToLower(strings.TrimSpace(config.Get("algorithm", "lagrange", true).(string)))
	state.MeshConfig["tension"] = config.Getfloat("bicubic_tension", .2, 0., 2., 0, 0, true)

	for i := 1; i < 100; i++ {
		start := config.Getfloatlist(fmt.Sprintf("faulty_region_%d_min", i), nil, ",", 2, true)
		if len(start) == 0 {
			break
		}
		end := config.Getfloatlist(fmt.Sprintf("faulty_region_%d_max", i), nil, ",", 2, true)
		c1, c3, c2, c4, err := NormalizeFaultyRegionCorners(start, end)
		if err != nil {
			return nil, err
		}
		if err := ValidateFaultyRegionOverlap(c1, c3, c2, c4, faultyRegionsToSlices(state.FaultyRegions), i); err != nil {
			return nil, err
		}
		state.FaultyRegions = append(state.FaultyRegions, FaultyRegion{Min: c1, Max: c3})
	}

	normalizedAlgo, forced, err := NormalizeMeshAlgorithm(
		state.MeshConfig["algo"].(string),
		state.MeshConfig["mesh_x_pps"].(int),
		state.MeshConfig["mesh_y_pps"].(int),
		state.MeshConfig["x_count"].(int),
		state.MeshConfig["y_count"].(int),
	)
	if err != nil {
		return nil, err
	}
	state.MeshConfig["algo"] = normalizedAlgo
	_ = forced

	if err := state.generatePoints(); err != nil {
		return nil, err
	}
	state.captureOriginals()
	return state, nil
}

func (state *CalibrationState) IsRoundBed() bool {
	return state.origRadius != nil
}

func (state *CalibrationState) OriginalMeshMin() []float64 {
	return cloneFloatSlice(state.origMeshMin)
}

func (state *CalibrationState) OriginalMeshMax() []float64 {
	return cloneFloatSlice(state.origMeshMax)
}

func (state *CalibrationState) AdjustedPoints() [][]float64 {
	return GetAdjustedPoints(clonePointMatrix(state.Points), clonePointSubstitutions(state.Substitutions))
}

func (state *CalibrationState) ResetToOriginal() {
	state.Radius = cloneFloatPtr(state.origRadius)
	state.Origin = cloneFloatSlice(state.origOrigin)
	state.MeshMin = cloneFloatSlice(state.origMeshMin)
	state.MeshMax = cloneFloatSlice(state.origMeshMax)
	state.RelativeReferenceIndex = cloneIntPtr(state.origRelativeReferenceIndex)
	state.MeshConfig = cloneMeshParams(state.origMeshConfig)
	state.Points = clonePointMatrix(state.origPoints)
	state.Substitutions = clonePointSubstitutions(state.origSubstitutions)
	state.FaultyRegions = cloneFaultyRegions(state.FaultyRegions)
}

func (state *CalibrationState) ApplyCommandOverrides(cmd CalibrationCommandSource) (bool, error) {
	params := cmd.Parameters()
	needConfigUpdate := false

	if raw, ok := params["RELATIVE_REFERENCE_INDEX"]; ok {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			state.RelativeReferenceIndex = nil
		} else {
			val, _ := strconv.ParseInt(trimmed, 10, 32)
			if val < 0 {
				state.RelativeReferenceIndex = nil
			} else {
				relativeReferenceIndex := int(val)
				state.RelativeReferenceIndex = &relativeReferenceIndex
			}
		}
		needConfigUpdate = true
	}

	if state.IsRoundBed() {
		if _, ok := params["MESH_RADIUS"]; ok {
			radius := cmd.Get_float("MESH_RADIUS", nil, nil, nil, nil, nil)
			radius = math.Floor(radius*10) / 10
			state.Radius = &radius
			state.MeshMin = []float64{-radius, -radius}
			state.MeshMax = []float64{radius, radius}
			needConfigUpdate = true
		}
		if _, ok := params["MESH_ORIGIN"]; ok {
			v1, v2, err := ParseGCodeCoord(cmd, "MESH_ORIGIN")
			if err != nil {
				return false, err
			}
			state.Origin = []float64{v1, v2}
			needConfigUpdate = true
		}
		if _, ok := params["ROUND_PROBE_COUNT"]; ok {
			minVal := 3
			count := cmd.Get_int("ROUND_PROBE_COUNT", nil, &minVal, nil)
			state.MeshConfig["x_count"] = count
			state.MeshConfig["y_count"] = count
			needConfigUpdate = true
		}
	} else {
		if _, ok := params["MESH_MIN"]; ok {
			v1, v2, err := ParseGCodeCoord(cmd, "MESH_MIN")
			if err != nil {
				return false, err
			}
			state.MeshMin = []float64{v1, v2}
			needConfigUpdate = true
		}
		if _, ok := params["MESH_MAX"]; ok {
			v1, v2, err := ParseGCodeCoord(cmd, "MESH_MAX")
			if err != nil {
				return false, err
			}
			state.MeshMax = []float64{v1, v2}
			needConfigUpdate = true
		}
		if _, ok := params["PROBE_COUNT"]; ok {
			minVal := 3.0
			pair, err := ParseGCodeIntPair(cmd, "PROBE_COUNT", &minVal, nil)
			if err != nil {
				return false, err
			}
			state.MeshConfig["x_count"] = pair[0]
			state.MeshConfig["y_count"] = pair[1]
			needConfigUpdate = true
		}
	}

	if _, ok := params["ALGORITHM"]; ok {
		state.MeshConfig["algo"] = strings.ToLower(strings.TrimSpace(cmd.Get("ALGORITHM", nil, "", nil, nil, nil, nil)))
		needConfigUpdate = true
	}

	return needConfigUpdate, nil
}

func (state *CalibrationState) ApplyAdaptiveLayout(layout AdaptiveMeshLayout) {
	state.MeshMin = []float64{layout.MeshMin.X, layout.MeshMin.Y}
	state.MeshMax = []float64{layout.MeshMax.X, layout.MeshMax.Y}
	state.MeshConfig["x_count"] = layout.XCount
	state.MeshConfig["y_count"] = layout.YCount
	if layout.AdaptedRadius != nil {
		state.Radius = cloneFloatPtr(layout.AdaptedRadius)
	}
	if layout.AdaptedOrigin != nil {
		state.Origin = []float64{layout.AdaptedOrigin.X, layout.AdaptedOrigin.Y}
	}
}

func (state *CalibrationState) Refresh(zeroReferencePosition []float64) (CalibrationRefreshResult, error) {
	result := CalibrationRefreshResult{}
	normalizedAlgo, forced, err := NormalizeMeshAlgorithm(
		state.MeshConfig["algo"].(string),
		state.MeshConfig["mesh_x_pps"].(int),
		state.MeshConfig["mesh_y_pps"].(int),
		state.MeshConfig["x_count"].(int),
		state.MeshConfig["y_count"].(int),
	)
	if err != nil {
		return result, err
	}
	state.MeshConfig["algo"] = normalizedAlgo
	result.ForcedLagrange = forced && normalizedAlgo == "lagrange"

	if err := state.generatePoints(); err != nil {
		return result, err
	}

	if state.RelativeReferenceIndex == nil && len(zeroReferencePosition) == 2 {
		result.ZeroReferenceAttempted = true
		idx := FindProbePointIndex(state.Points, Vec2{X: zeroReferencePosition[0], Y: zeroReferencePosition[1]})
		if idx >= 0 {
			idxCopy := idx
			state.RelativeReferenceIndex = &idxCopy
			state.origRelativeReferenceIndex = cloneIntPtr(state.RelativeReferenceIndex)
			result.ZeroReferenceMatched = true
		}
	}

	result.AdjustedPoints = state.AdjustedPoints()
	return result, nil
}

func (state *CalibrationState) captureOriginals() {
	state.origRadius = cloneFloatPtr(state.Radius)
	state.origOrigin = cloneFloatSlice(state.Origin)
	state.origMeshMin = cloneFloatSlice(state.MeshMin)
	state.origMeshMax = cloneFloatSlice(state.MeshMax)
	state.origRelativeReferenceIndex = cloneIntPtr(state.RelativeReferenceIndex)
	state.origMeshConfig = cloneMeshParams(state.MeshConfig)
	state.origPoints = clonePointMatrix(state.Points)
	state.origSubstitutions = clonePointSubstitutions(state.Substitutions)
	state.FaultyRegions = cloneFaultyRegions(state.FaultyRegions)
}

func (state *CalibrationState) generatePoints() error {
	probeResult, err := GenerateProbePoints(
		cloneFloatPtr(state.Radius),
		cloneFloatSlice(state.Origin),
		cloneFloatSlice(state.MeshMin),
		cloneFloatSlice(state.MeshMax),
		state.MeshConfig["x_count"].(int),
		state.MeshConfig["y_count"].(int),
		cloneFaultyRegions(state.FaultyRegions),
	)
	if err != nil {
		return err
	}
	state.Points = probeResult.Points
	state.Substitutions = probeResult.Substitutions
	return nil
}

func faultyRegionsToSlices(regions []FaultyRegion) [][][]float64 {
	if len(regions) == 0 {
		return nil
	}
	converted := make([][][]float64, 0, len(regions))
	for _, region := range regions {
		converted = append(converted, [][]float64{cloneFloatSlice(region.Min), cloneFloatSlice(region.Max)})
	}
	return converted
}

func cloneFloatPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneFloatSlice(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]float64, len(values))
	copy(cloned, values)
	return cloned
}

func clonePointMatrix(points [][]float64) [][]float64 {
	if len(points) == 0 {
		return nil
	}
	cloned := make([][]float64, len(points))
	for i, point := range points {
		cloned[i] = cloneFloatSlice(point)
	}
	return cloned
}

func clonePointSubstitutions(substitutions []PointSubstitution) []PointSubstitution {
	if len(substitutions) == 0 {
		return nil
	}
	cloned := make([]PointSubstitution, len(substitutions))
	for i, substitution := range substitutions {
		cloned[i] = PointSubstitution{
			Index:  substitution.Index,
			Points: clonePointMatrix(substitution.Points),
		}
	}
	return cloned
}

func cloneFaultyRegions(regions []FaultyRegion) []FaultyRegion {
	if len(regions) == 0 {
		return nil
	}
	cloned := make([]FaultyRegion, len(regions))
	for i, region := range regions {
		cloned[i] = FaultyRegion{
			Min: cloneFloatSlice(region.Min),
			Max: cloneFloatSlice(region.Max),
		}
	}
	return cloned
}
