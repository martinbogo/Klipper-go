package bedmesh

import (
	"fmt"
	"goklipper/internal/pkg/util"
	"strings"
)

type AdaptiveModuleConfigSource interface {
	HasSection(section string) bool
	HasOption(section string, option string) bool
	FloatSlice(section string, option string, fallback []float64, count int) []float64
	Float(section string, option string, fallback float64) float64
	Int(section string, option string, fallback int, minValue int) int
	Bool(section string, option string, fallback bool) bool
	String(section string, option string, fallback string) (string, error)
}

func BuildAdaptiveCalibrationModuleConfig(source AdaptiveModuleConfigSource) (AdaptiveCalibrationModuleConfig, error) {
	if !source.HasSection("bed_mesh") {
		return AdaptiveCalibrationModuleConfig{}, fmt.Errorf("[adaptive_bed_mesh] missing required [bed_mesh] section")
	}
	meshMin := source.FloatSlice("bed_mesh", "mesh_min", nil, 2)
	if len(meshMin) != 2 {
		return AdaptiveCalibrationModuleConfig{}, fmt.Errorf("[adaptive_bed_mesh] bed_mesh.mesh_min must contain two coordinates")
	}
	meshMax := source.FloatSlice("bed_mesh", "mesh_max", nil, 2)
	if len(meshMax) != 2 {
		return AdaptiveCalibrationModuleConfig{}, fmt.Errorf("[adaptive_bed_mesh] bed_mesh.mesh_max must contain two coordinates")
	}
	fadeEnd := source.Float("bed_mesh", "fade_end", 0)
	algorithm, err := source.String("bed_mesh", "algorithm", "lagrange")
	if err != nil {
		return AdaptiveCalibrationModuleConfig{}, err
	}

	if !source.HasSection("virtual_sdcard") {
		return AdaptiveCalibrationModuleConfig{}, fmt.Errorf("[adaptive_bed_mesh] missing required [virtual_sdcard] section")
	}
	if !source.HasOption("virtual_sdcard", "path") {
		return AdaptiveCalibrationModuleConfig{}, fmt.Errorf("[adaptive_bed_mesh] missing required virtual_sdcard.path option")
	}
	sdPath, err := source.String("virtual_sdcard", "path", "")
	if err != nil {
		return AdaptiveCalibrationModuleConfig{}, err
	}

	return AdaptiveCalibrationModuleConfig{
		DefaultMin:                    Vec2{X: meshMin[0], Y: meshMin[1]},
		DefaultMax:                    Vec2{X: meshMax[0], Y: meshMax[1]},
		FadeEnd:                       fadeEnd,
		VirtualSDPath:                 util.Normpath(util.ExpandUser(sdPath)),
		ArcSegments:                   source.Int("", "arc_segments", 80, 1),
		DisableSlicerBoundary:         source.Bool("", "disable_slicer_min_max_boundary_detection", false),
		DisableExcludeBoundary:        source.Bool("", "disable_exclude_object_boundary_detection", false),
		DisableGcodeBoundary:          source.Bool("", "disable_gcode_analysis_boundary_detection", false),
		Margin:                        source.Float("", "mesh_area_clearance", 5),
		MaxHDist:                      source.Float("", "max_probe_horizontal_distance", 50),
		MaxVDist:                      source.Float("", "max_probe_vertical_distance", 50),
		MinimumProbeCount:             3,
		Algorithm:                     strings.ToLower(strings.TrimSpace(algorithm)),
		IncludeRelativeReferenceIndex: source.Bool("", "use_relative_reference_index", false),
	}, nil
}
