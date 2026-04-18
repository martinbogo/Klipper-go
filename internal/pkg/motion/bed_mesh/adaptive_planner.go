package bedmesh

import (
	"fmt"
	"math"
)

type AdaptiveMeshLayoutConfig struct {
	Margin            float64
	DefaultMin        Vec2
	DefaultMax        Vec2
	BaseXCount        int
	BaseYCount        int
	MinimumProbeCount int
	MinimumMeshSize   *Vec2
	Algorithm         string
	BedRadius         *float64
}

type AdaptiveMeshLayout struct {
	MeshMin       Vec2
	MeshMax       Vec2
	XCount        int
	YCount        int
	RatioX        float64
	RatioY        float64
	ProfileName   string
	AdaptedRadius *float64
	AdaptedOrigin *Vec2
}

func BuildAdaptiveMeshLayout(objectMin, objectMax Vec2, cfg AdaptiveMeshLayoutConfig) (AdaptiveMeshLayout, error) {
	result := AdaptiveMeshLayout{
		MeshMin: cfg.DefaultMin,
		MeshMax: cfg.DefaultMax,
		XCount:  cfg.BaseXCount,
		YCount:  cfg.BaseYCount,
	}
	if cfg.BaseXCount < 1 || cfg.BaseYCount < 1 {
		return result, fmt.Errorf("bed_mesh: invalid base probe count %d x %d", cfg.BaseXCount, cfg.BaseYCount)
	}
	defaultWidth := cfg.DefaultMax.X - cfg.DefaultMin.X
	defaultHeight := cfg.DefaultMax.Y - cfg.DefaultMin.Y
	if defaultWidth <= 0 || defaultHeight <= 0 {
		return result, fmt.Errorf("bed_mesh: invalid default mesh bounds")
	}

	adjustedMin, adjustedMax := ApplyMinMaxMargin(objectMin, objectMax, cfg.Margin)
	adjustedMin, adjustedMax = ApplyMinMaxLimit(adjustedMin, adjustedMax, cfg.DefaultMin, cfg.DefaultMax)
	adjustedWidth := adjustedMax.X - adjustedMin.X
	adjustedHeight := adjustedMax.Y - adjustedMin.Y
	if adjustedWidth <= 0 || adjustedHeight <= 0 {
		return result, fmt.Errorf("bed_mesh: invalid adaptive mesh bounds after applying limits")
	}

	result.RatioX = adjustedWidth / defaultWidth
	result.RatioY = adjustedHeight / defaultHeight

	xCount, yCount := ApplyProbePointLimits(
		int(math.Ceil(float64(cfg.BaseXCount)*result.RatioX)),
		int(math.Ceil(float64(cfg.BaseYCount)*result.RatioY)),
		ProbeGridConfig{MinCount: max(cfg.MinimumProbeCount, 1), Algorithm: cfg.Algorithm},
	)

	if cfg.BedRadius != nil && *cfg.BedRadius != 0 {
		adaptedRadius := math.Sqrt(adjustedWidth*adjustedWidth+adjustedHeight*adjustedHeight) / 2
		adaptedOrigin := Vec2{
			X: adjustedMin.X + adjustedWidth/2,
			Y: adjustedMin.Y + adjustedHeight/2,
		}
		if adaptedRadius+math.Hypot(adaptedOrigin.X, adaptedOrigin.Y) < *cfg.BedRadius {
			probeCount := max(xCount, yCount)
			if IsEven(probeCount) {
				probeCount++
			}
			result.MeshMin = Vec2{X: -adaptedRadius, Y: -adaptedRadius}
			result.MeshMax = Vec2{X: adaptedRadius, Y: adaptedRadius}
			result.XCount = probeCount
			result.YCount = probeCount
			result.AdaptedRadius = &adaptedRadius
			result.AdaptedOrigin = &adaptedOrigin
		}
	} else {
		if cfg.MinimumMeshSize != nil && adjustedWidth < cfg.MinimumMeshSize.X && adjustedHeight < cfg.MinimumMeshSize.Y {
			xCenter := (adjustedMin.X + adjustedMax.X) / 2
			yCenter := (adjustedMin.Y + adjustedMax.Y) / 2
			adjustedMin = Vec2{X: xCenter - cfg.MinimumMeshSize.X/2, Y: yCenter - cfg.MinimumMeshSize.Y/2}
			adjustedMax = Vec2{X: xCenter + cfg.MinimumMeshSize.X/2, Y: yCenter + cfg.MinimumMeshSize.Y/2}
			adjustedMin, adjustedMax = ApplyMinMaxLimit(adjustedMin, adjustedMax, cfg.DefaultMin, cfg.DefaultMax)
		}
		result.MeshMin = adjustedMin
		result.MeshMax = adjustedMax
		result.XCount = xCount
		result.YCount = yCount
	}

	if Isclose(result.RatioX, 1.0, 1e-4, 1e-9) && Isclose(result.RatioY, 1.0, 1e-4, 1e-9) {
		result.ProfileName = "default"
	} else {
		result.ProfileName = "adaptive"
	}

	return result, nil
}

type AdaptiveCalibrationPlanConfig struct {
	BoundsMin         Vec2
	BoundsMax         Vec2
	Margin            float64
	MaxHDist          float64
	MaxVDist          float64
	MinimumProbeCount int
	Algorithm         string
}

type AdaptiveCalibrationPlan struct {
	MeshMin                Vec2
	MeshMax                Vec2
	XCount                 int
	YCount                 int
	ProbePoints            []Vec2
	RelativeReferenceIndex int
	ZeroReference          Vec2
}

type AdaptiveCalibrationExecutionConfig struct {
	PlanConfig                    AdaptiveCalibrationPlanConfig
	IncludeRelativeReferenceIndex bool
}

type AdaptiveCalibrationExecution struct {
	Plan    AdaptiveCalibrationPlan
	Command string
}

type AdaptiveCalibrationCommandRequest struct {
	AreaInput       AdaptiveMeshAreaInput
	AreaConfig      AdaptiveMeshAreaConfig
	ExecutionConfig AdaptiveCalibrationExecutionConfig
}

type AdaptiveCalibrationModuleConfig struct {
	DefaultMin                    Vec2
	DefaultMax                    Vec2
	FadeEnd                       float64
	VirtualSDPath                 string
	ArcSegments                   int
	DisableSlicerBoundary         bool
	DisableExcludeBoundary        bool
	DisableGcodeBoundary          bool
	Margin                        float64
	MaxHDist                      float64
	MaxVDist                      float64
	MinimumProbeCount             int
	Algorithm                     string
	IncludeRelativeReferenceIndex bool
}

type AdaptiveCalibrationCommandResult struct {
	Area      AdaptiveMeshAreaResult
	Execution AdaptiveCalibrationExecution
}

type AdaptiveCalibrationModulePlan struct {
	Messages      []string
	MeshMin       Vec2
	MeshMax       Vec2
	Command       string
	ZeroReference *Vec2
}

type AdaptiveCalibrationModuleRuntime struct {
	Log              func(string)
	SetZeroReference func(*Vec2)
	RunCommand       func(string)
}

func PlanAdaptiveCalibration(meshMin, meshMax Vec2, cfg AdaptiveCalibrationPlanConfig) (AdaptiveCalibrationPlan, error) {
	minWithMargin, maxWithMargin := ApplyMinMaxMargin(meshMin, meshMax, cfg.Margin)
	limitedMin, limitedMax := ApplyMinMaxLimit(minWithMargin, maxWithMargin, cfg.BoundsMin, cfg.BoundsMax)
	if limitedMax.X <= limitedMin.X || limitedMax.Y <= limitedMin.Y {
		return AdaptiveCalibrationPlan{}, fmt.Errorf("invalid mesh bounds after applying margin and limits")
	}

	xCount, yCount, probePoints, relIdx := GenerateProbeGrid(limitedMin, limitedMax, ProbeGridConfig{
		MaxHDist:  cfg.MaxHDist,
		MaxVDist:  cfg.MaxVDist,
		MinCount:  max(cfg.MinimumProbeCount, 1),
		Algorithm: cfg.Algorithm,
	})
	if len(probePoints) == 0 {
		return AdaptiveCalibrationPlan{}, fmt.Errorf("unable to generate probe coordinates")
	}
	if relIdx < 0 || relIdx >= len(probePoints) {
		relIdx = len(probePoints) / 2
	}

	return AdaptiveCalibrationPlan{
		MeshMin:                limitedMin,
		MeshMax:                limitedMax,
		XCount:                 xCount,
		YCount:                 yCount,
		ProbePoints:            probePoints,
		RelativeReferenceIndex: relIdx,
		ZeroReference:          probePoints[relIdx],
	}, nil
}

func BuildAdaptiveCalibrationExecution(meshMin, meshMax Vec2, cfg AdaptiveCalibrationExecutionConfig) (AdaptiveCalibrationExecution, error) {
	plan, err := PlanAdaptiveCalibration(meshMin, meshMax, cfg.PlanConfig)
	if err != nil {
		return AdaptiveCalibrationExecution{}, err
	}
	command := fmt.Sprintf(
		"BED_MESH_CALIBRATE MESH_MIN=%.3f,%.3f MESH_MAX=%.3f,%.3f PROBE_COUNT=%d,%d",
		plan.MeshMin.X, plan.MeshMin.Y, plan.MeshMax.X, plan.MeshMax.Y, plan.XCount, plan.YCount,
	)
	if cfg.IncludeRelativeReferenceIndex {
		command += fmt.Sprintf(" RELATIVE_REFERENCE_INDEX=%d", plan.RelativeReferenceIndex)
	}
	return AdaptiveCalibrationExecution{Plan: plan, Command: command}, nil
}

func PlanAdaptiveCalibrationCommand(req AdaptiveCalibrationCommandRequest) (AdaptiveCalibrationCommandResult, error) {
	var result AdaptiveCalibrationCommandResult

	area, err := DetectAdaptiveMeshArea(req.AreaInput, req.AreaConfig)
	if err != nil {
		return result, err
	}
	execution, err := BuildAdaptiveCalibrationExecution(area.MeshMin, area.MeshMax, req.ExecutionConfig)
	if err != nil {
		return result, err
	}

	result.Area = area
	result.Execution = execution
	return result, nil
}

func PlanAdaptiveCalibrationModuleCommand(input AdaptiveMeshAreaInput, cfg AdaptiveCalibrationModuleConfig) (AdaptiveCalibrationCommandResult, error) {
	return PlanAdaptiveCalibrationCommand(AdaptiveCalibrationCommandRequest{
		AreaInput: input,
		AreaConfig: AdaptiveMeshAreaConfig{
			DefaultMin:             cfg.DefaultMin,
			DefaultMax:             cfg.DefaultMax,
			FadeEnd:                cfg.FadeEnd,
			VirtualSDPath:          cfg.VirtualSDPath,
			ArcSegments:            cfg.ArcSegments,
			DisableSlicerBoundary:  cfg.DisableSlicerBoundary,
			DisableExcludeBoundary: cfg.DisableExcludeBoundary,
			DisableGcodeBoundary:   cfg.DisableGcodeBoundary,
		},
		ExecutionConfig: AdaptiveCalibrationExecutionConfig{
			PlanConfig: AdaptiveCalibrationPlanConfig{
				BoundsMin:         cfg.DefaultMin,
				BoundsMax:         cfg.DefaultMax,
				Margin:            cfg.Margin,
				MaxHDist:          cfg.MaxHDist,
				MaxVDist:          cfg.MaxVDist,
				MinimumProbeCount: cfg.MinimumProbeCount,
				Algorithm:         cfg.Algorithm,
			},
			IncludeRelativeReferenceIndex: cfg.IncludeRelativeReferenceIndex,
		},
	})
}

func BuildAdaptiveCalibrationModulePlan(input AdaptiveMeshAreaInput, cfg AdaptiveCalibrationModuleConfig) (AdaptiveCalibrationModulePlan, error) {
	result, err := PlanAdaptiveCalibrationModuleCommand(input, cfg)
	if err != nil {
		return AdaptiveCalibrationModulePlan{}, err
	}
	plan := AdaptiveCalibrationModulePlan{
		Messages: append([]string(nil), result.Area.Messages...),
		MeshMin:  result.Execution.Plan.MeshMin,
		MeshMax:  result.Execution.Plan.MeshMax,
		Command:  result.Execution.Command,
	}
	if !cfg.IncludeRelativeReferenceIndex {
		zeroRef := result.Execution.Plan.ZeroReference
		plan.ZeroReference = &zeroRef
	}
	return plan, nil
}

func RunAdaptiveCalibrationModule(input AdaptiveMeshAreaInput, cfg AdaptiveCalibrationModuleConfig, runtime AdaptiveCalibrationModuleRuntime) error {
	plan, err := BuildAdaptiveCalibrationModulePlan(input, cfg)
	if err != nil {
		return err
	}
	ExecuteAdaptiveCalibrationModulePlan(plan, runtime)
	return nil
}

func ExecuteAdaptiveCalibrationModulePlan(plan AdaptiveCalibrationModulePlan, runtime AdaptiveCalibrationModuleRuntime) {
	for _, msg := range plan.Messages {
		if runtime.Log != nil {
			runtime.Log(msg)
		}
	}
	if runtime.Log != nil {
		runtime.Log(fmt.Sprintf("Selected mesh area: min(%.3f, %.3f) max(%.3f, %.3f)", plan.MeshMin.X, plan.MeshMin.Y, plan.MeshMax.X, plan.MeshMax.Y))
	}
	if runtime.SetZeroReference != nil {
		runtime.SetZeroReference(plan.ZeroReference)
	}
	if runtime.Log != nil {
		runtime.Log(plan.Command)
	}
	if runtime.RunCommand != nil {
		runtime.RunCommand(plan.Command)
	}
}
