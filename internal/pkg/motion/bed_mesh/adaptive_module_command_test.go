package bedmesh

import (
	"fmt"
	"reflect"
	"testing"
)

type stubAdaptiveModuleConfigSource struct {
	sections map[string]map[string]interface{}
}

func (self stubAdaptiveModuleConfigSource) section(name string) map[string]interface{} {
	if self.sections == nil {
		return nil
	}
	return self.sections[name]
}

func (self stubAdaptiveModuleConfigSource) HasSection(section string) bool {
	_, ok := self.sections[section]
	return ok
}

func (self stubAdaptiveModuleConfigSource) HasOption(section string, option string) bool {
	_, ok := self.section(section)[option]
	return ok
}

func (self stubAdaptiveModuleConfigSource) FloatSlice(section string, option string, fallback []float64, _ int) []float64 {
	if value, ok := self.section(section)[option].([]float64); ok {
		return append([]float64(nil), value...)
	}
	return append([]float64(nil), fallback...)
}

func (self stubAdaptiveModuleConfigSource) Float(section string, option string, fallback float64) float64 {
	if value, ok := self.section(section)[option].(float64); ok {
		return value
	}
	return fallback
}

func (self stubAdaptiveModuleConfigSource) Int(section string, option string, fallback int, _ int) int {
	if value, ok := self.section(section)[option].(int); ok {
		return value
	}
	return fallback
}

func (self stubAdaptiveModuleConfigSource) Bool(section string, option string, fallback bool) bool {
	if value, ok := self.section(section)[option].(bool); ok {
		return value
	}
	return fallback
}

func (self stubAdaptiveModuleConfigSource) String(section string, option string, fallback string) (string, error) {
	if value, ok := self.section(section)[option]; ok {
		str, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("invalid %s.%s type %T", section, option, value)
		}
		return str, nil
	}
	return fallback, nil
}

func TestBuildAdaptiveCalibrationModuleConfigReadsSectionsAndNormalizesValues(t *testing.T) {
	source := stubAdaptiveModuleConfigSource{sections: map[string]map[string]interface{}{
		"": {
			"arc_segments": 96,
			"disable_slicer_min_max_boundary_detection": true,
			"disable_exclude_object_boundary_detection": true,
			"disable_gcode_analysis_boundary_detection": false,
			"mesh_area_clearance":                       7.5,
			"max_probe_horizontal_distance":             33.0,
			"max_probe_vertical_distance":               44.0,
			"use_relative_reference_index":              true,
		},
		"bed_mesh": {
			"mesh_min":  []float64{15, 20},
			"mesh_max":  []float64{210, 220},
			"fade_end":  8.0,
			"algorithm": " Bicubic ",
		},
		"virtual_sdcard": {
			"path": "~/printer_data/gcodes",
		},
	}}

	cfg, err := BuildAdaptiveCalibrationModuleConfig(source)
	if err != nil {
		t.Fatalf("BuildAdaptiveCalibrationModuleConfig() error = %v", err)
	}
	if cfg.DefaultMin != (Vec2{X: 15, Y: 20}) {
		t.Fatalf("DefaultMin = %+v", cfg.DefaultMin)
	}
	if cfg.DefaultMax != (Vec2{X: 210, Y: 220}) {
		t.Fatalf("DefaultMax = %+v", cfg.DefaultMax)
	}
	if cfg.FadeEnd != 8.0 || cfg.ArcSegments != 96 || cfg.Margin != 7.5 || cfg.MaxHDist != 33.0 || cfg.MaxVDist != 44.0 {
		t.Fatalf("unexpected numeric config: %+v", cfg)
	}
	if cfg.Algorithm != "bicubic" {
		t.Fatalf("Algorithm = %q, want bicubic", cfg.Algorithm)
	}
	if !cfg.DisableSlicerBoundary || !cfg.DisableExcludeBoundary || cfg.DisableGcodeBoundary == true || !cfg.IncludeRelativeReferenceIndex {
		t.Fatalf("unexpected boolean config: %+v", cfg)
	}
	if cfg.VirtualSDPath == "~/printer_data/gcodes" || cfg.VirtualSDPath == "" {
		t.Fatalf("VirtualSDPath = %q, want normalized expanded path", cfg.VirtualSDPath)
	}
}

func TestBuildAdaptiveCalibrationModuleConfigRejectsMissingSections(t *testing.T) {
	_, err := BuildAdaptiveCalibrationModuleConfig(stubAdaptiveModuleConfigSource{sections: map[string]map[string]interface{}{
		"": {},
	}})
	if err == nil || err.Error() != "[adaptive_bed_mesh] missing required [bed_mesh] section" {
		t.Fatalf("BuildAdaptiveCalibrationModuleConfig() error = %v", err)
	}
}

func TestPlanAdaptiveCalibrationModuleCommandBuildsRequest(t *testing.T) {
	result, err := PlanAdaptiveCalibrationModuleCommand(
		AdaptiveMeshAreaInput{
			AreaStart: "10,20",
			AreaEnd:   "110,120",
		},
		AdaptiveCalibrationModuleConfig{
			DefaultMin:                    Vec2{X: 0, Y: 0},
			DefaultMax:                    Vec2{X: 200, Y: 200},
			Margin:                        5,
			MaxHDist:                      40,
			MaxVDist:                      40,
			MinimumProbeCount:             3,
			Algorithm:                     "lagrange",
			IncludeRelativeReferenceIndex: true,
		},
	)
	if err != nil {
		t.Fatalf("PlanAdaptiveCalibrationModuleCommand() error = %v", err)
	}
	if result.Area.Source != "slicer" {
		t.Fatalf("Area.Source = %q, want slicer", result.Area.Source)
	}
	if result.Execution.Plan.MeshMin.X != 5 || result.Execution.Plan.MeshMin.Y != 15 {
		t.Fatalf("Plan.MeshMin = %+v, want {5 15}", result.Execution.Plan.MeshMin)
	}
	if result.Execution.Plan.MeshMax.X != 115 || result.Execution.Plan.MeshMax.Y != 125 {
		t.Fatalf("Plan.MeshMax = %+v, want {115 125}", result.Execution.Plan.MeshMax)
	}
	if result.Execution.Command == "" {
		t.Fatal("Execution.Command is empty")
	}
	if result.Execution.Plan.RelativeReferenceIndex < 0 {
		t.Fatalf("RelativeReferenceIndex = %d, want non-negative", result.Execution.Plan.RelativeReferenceIndex)
	}
}

func TestBuildAdaptiveCalibrationModulePlanSetsZeroReferenceOnlyForLegacyZeroRefMode(t *testing.T) {
	input := AdaptiveMeshAreaInput{
		AreaStart: "10,20",
		AreaEnd:   "110,120",
	}
	cfg := AdaptiveCalibrationModuleConfig{
		DefaultMin:                    Vec2{X: 0, Y: 0},
		DefaultMax:                    Vec2{X: 200, Y: 200},
		Margin:                        5,
		MaxHDist:                      40,
		MaxVDist:                      40,
		MinimumProbeCount:             3,
		Algorithm:                     "lagrange",
		IncludeRelativeReferenceIndex: false,
	}
	plan, err := BuildAdaptiveCalibrationModulePlan(input, cfg)
	if err != nil {
		t.Fatalf("BuildAdaptiveCalibrationModulePlan() error = %v", err)
	}
	if plan.Command == "" {
		t.Fatal("expected generated adaptive calibration command")
	}
	if plan.ZeroReference == nil {
		t.Fatal("expected zero reference for legacy zero-ref mode")
	}
	if plan.ZeroReference.X < plan.MeshMin.X || plan.ZeroReference.X > plan.MeshMax.X ||
		plan.ZeroReference.Y < plan.MeshMin.Y || plan.ZeroReference.Y > plan.MeshMax.Y {
		t.Fatalf("zero reference %+v should lie within selected mesh area min=%+v max=%+v", *plan.ZeroReference, plan.MeshMin, plan.MeshMax)
	}

	cfg.IncludeRelativeReferenceIndex = true
	plan, err = BuildAdaptiveCalibrationModulePlan(input, cfg)
	if err != nil {
		t.Fatalf("BuildAdaptiveCalibrationModulePlan() with relative ref index error = %v", err)
	}
	if plan.ZeroReference != nil {
		t.Fatalf("expected nil zero reference when relative reference index mode is enabled, got %+v", *plan.ZeroReference)
	}
}

func TestExecuteAdaptiveCalibrationModulePlanAppliesMessagesZeroReferenceAndCommand(t *testing.T) {
	var logs []string
	var zeroRef *Vec2
	var executed []string
	plan := AdaptiveCalibrationModulePlan{
		Messages:      []string{"detected slicer bounds", "expanded by margin"},
		MeshMin:       Vec2{X: 5, Y: 10},
		MeshMax:       Vec2{X: 115, Y: 125},
		Command:       "BED_MESH_CALIBRATE MESH_MIN=5,10 MESH_MAX=115,125",
		ZeroReference: &Vec2{X: 60, Y: 70},
	}
	ExecuteAdaptiveCalibrationModulePlan(plan, AdaptiveCalibrationModuleRuntime{
		Log: func(msg string) {
			logs = append(logs, msg)
		},
		SetZeroReference: func(value *Vec2) {
			zeroRef = value
		},
		RunCommand: func(command string) {
			executed = append(executed, command)
		},
	})

	if zeroRef == nil || *zeroRef != (Vec2{X: 60, Y: 70}) {
		t.Fatalf("zero reference = %+v, want {60 70}", zeroRef)
	}
	wantLogs := []string{
		"detected slicer bounds",
		"expanded by margin",
		"Selected mesh area: min(5.000, 10.000) max(115.000, 125.000)",
		"BED_MESH_CALIBRATE MESH_MIN=5,10 MESH_MAX=115,125",
	}
	if !reflect.DeepEqual(logs, wantLogs) {
		t.Fatalf("logs = %#v, want %#v", logs, wantLogs)
	}
	if !reflect.DeepEqual(executed, []string{plan.Command}) {
		t.Fatalf("executed = %#v, want %#v", executed, []string{plan.Command})
	}

	logs = nil
	zeroRef = &Vec2{X: 1, Y: 2}
	executed = nil
	plan.ZeroReference = nil
	ExecuteAdaptiveCalibrationModulePlan(plan, AdaptiveCalibrationModuleRuntime{
		SetZeroReference: func(value *Vec2) {
			zeroRef = value
		},
	})
	if zeroRef != nil {
		t.Fatalf("zero reference should be cleared, got %+v", *zeroRef)
	}
}

func TestRunAdaptiveCalibrationModuleBuildsAndExecutesPlan(t *testing.T) {
	var logs []string
	var zeroRef *Vec2
	var executed []string
	err := RunAdaptiveCalibrationModule(
		AdaptiveMeshAreaInput{
			AreaStart: "10,20",
			AreaEnd:   "110,120",
		},
		AdaptiveCalibrationModuleConfig{
			DefaultMin:                    Vec2{X: 0, Y: 0},
			DefaultMax:                    Vec2{X: 200, Y: 200},
			Margin:                        5,
			MaxHDist:                      40,
			MaxVDist:                      40,
			MinimumProbeCount:             3,
			Algorithm:                     "lagrange",
			IncludeRelativeReferenceIndex: false,
		},
		AdaptiveCalibrationModuleRuntime{
			Log: func(msg string) {
				logs = append(logs, msg)
			},
			SetZeroReference: func(value *Vec2) {
				zeroRef = value
			},
			RunCommand: func(command string) {
				executed = append(executed, command)
			},
		},
	)
	if err != nil {
		t.Fatalf("RunAdaptiveCalibrationModule() error = %v", err)
	}
	if len(logs) < 3 {
		t.Fatalf("expected logs from module execution, got %#v", logs)
	}
	if zeroRef == nil {
		t.Fatal("expected zero reference to be forwarded")
	}
	if len(executed) != 1 || executed[0] == "" {
		t.Fatalf("executed = %#v, want one generated command", executed)
	}
}
