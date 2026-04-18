package bedmesh

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAdaptiveMeshLayoutRectangular(t *testing.T) {
	minSize := Vec2{X: 100, Y: 100}
	plan, err := BuildAdaptiveMeshLayout(
		Vec2{X: 40, Y: 55},
		Vec2{X: 60, Y: 65},
		AdaptiveMeshLayoutConfig{
			Margin:            5,
			DefaultMin:        Vec2{X: 0, Y: 0},
			DefaultMax:        Vec2{X: 200, Y: 200},
			BaseXCount:        7,
			BaseYCount:        7,
			MinimumProbeCount: 3,
			MinimumMeshSize:   &minSize,
			Algorithm:         "bicubic",
		},
	)
	if err != nil {
		t.Fatalf("BuildAdaptiveMeshLayout() error = %v", err)
	}
	if got, want := plan.MeshMin, (Vec2{X: 0, Y: 10}); math.Abs(got.X-want.X) > 1e-9 || math.Abs(got.Y-want.Y) > 1e-9 {
		t.Fatalf("MeshMin = %+v, want %+v", got, want)
	}
	if got, want := plan.MeshMax, (Vec2{X: 100, Y: 110}); math.Abs(got.X-want.X) > 1e-9 || math.Abs(got.Y-want.Y) > 1e-9 {
		t.Fatalf("MeshMax = %+v, want %+v", got, want)
	}
	if plan.XCount != 3 || plan.YCount != 3 {
		t.Fatalf("expected minimum probe counts of 3x3, got %dx%d", plan.XCount, plan.YCount)
	}
	if plan.ProfileName != "adaptive" {
		t.Fatalf("ProfileName = %q, want adaptive", plan.ProfileName)
	}
}

func TestBuildAdaptiveMeshLayoutRoundUsesOddProbeCount(t *testing.T) {
	radius := 120.0
	plan, err := BuildAdaptiveMeshLayout(
		Vec2{X: -30, Y: -40},
		Vec2{X: 30, Y: 40},
		AdaptiveMeshLayoutConfig{
			Margin:            10,
			DefaultMin:        Vec2{X: -120, Y: -120},
			DefaultMax:        Vec2{X: 120, Y: 120},
			BaseXCount:        7,
			BaseYCount:        7,
			MinimumProbeCount: 3,
			Algorithm:         "lagrange",
			BedRadius:         &radius,
		},
	)
	if err != nil {
		t.Fatalf("BuildAdaptiveMeshLayout() error = %v", err)
	}
	if plan.AdaptedRadius == nil || plan.AdaptedOrigin == nil {
		t.Fatal("expected round-bed adaptive radius and origin to be populated")
	}
	if plan.XCount != plan.YCount || plan.XCount%2 == 0 {
		t.Fatalf("expected odd square probe count, got %dx%d", plan.XCount, plan.YCount)
	}
}

func TestPlanAdaptiveCalibrationClampsBoundsAndReference(t *testing.T) {
	plan, err := PlanAdaptiveCalibration(
		Vec2{X: 5, Y: 10},
		Vec2{X: 105, Y: 90},
		AdaptiveCalibrationPlanConfig{
			BoundsMin:         Vec2{X: 0, Y: 0},
			BoundsMax:         Vec2{X: 100, Y: 100},
			Margin:            10,
			MaxHDist:          35,
			MaxVDist:          35,
			MinimumProbeCount: 3,
			Algorithm:         "lagrange",
		},
	)
	if err != nil {
		t.Fatalf("PlanAdaptiveCalibration() error = %v", err)
	}
	if plan.MeshMin != (Vec2{X: 0, Y: 0}) || plan.MeshMax != (Vec2{X: 100, Y: 100}) {
		t.Fatalf("unexpected clamped mesh bounds: min=%+v max=%+v", plan.MeshMin, plan.MeshMax)
	}
	if plan.RelativeReferenceIndex < 0 || plan.RelativeReferenceIndex >= len(plan.ProbePoints) {
		t.Fatalf("relative reference index %d out of range for %d probe points", plan.RelativeReferenceIndex, len(plan.ProbePoints))
	}
	zero := plan.ProbePoints[plan.RelativeReferenceIndex]
	if plan.ZeroReference != zero {
		t.Fatalf("zero reference = %+v, want %+v", plan.ZeroReference, zero)
	}
}

func TestBuildAdaptiveCalibrationExecutionIncludesRelativeReferenceIndexWhenRequested(t *testing.T) {
	execution, err := BuildAdaptiveCalibrationExecution(
		Vec2{X: 10, Y: 20},
		Vec2{X: 70, Y: 80},
		AdaptiveCalibrationExecutionConfig{
			PlanConfig: AdaptiveCalibrationPlanConfig{
				BoundsMin:         Vec2{X: 0, Y: 0},
				BoundsMax:         Vec2{X: 100, Y: 100},
				Margin:            5,
				MaxHDist:          20,
				MaxVDist:          20,
				MinimumProbeCount: 3,
				Algorithm:         "lagrange",
			},
			IncludeRelativeReferenceIndex: true,
		},
	)
	if err != nil {
		t.Fatalf("BuildAdaptiveCalibrationExecution() error = %v", err)
	}
	if !strings.Contains(execution.Command, "BED_MESH_CALIBRATE ") {
		t.Fatalf("command = %q, want BED_MESH_CALIBRATE prefix", execution.Command)
	}
	needle := "RELATIVE_REFERENCE_INDEX="
	if !strings.Contains(execution.Command, needle) {
		t.Fatalf("command = %q, want %s", execution.Command, needle)
	}
	if execution.Plan.ZeroReference != execution.Plan.ProbePoints[execution.Plan.RelativeReferenceIndex] {
		t.Fatalf("unexpected zero reference %+v for relative index %d", execution.Plan.ZeroReference, execution.Plan.RelativeReferenceIndex)
	}
}

func TestBuildAdaptiveCalibrationExecutionOmitsRelativeReferenceIndexWhenDisabled(t *testing.T) {
	execution, err := BuildAdaptiveCalibrationExecution(
		Vec2{X: 10, Y: 20},
		Vec2{X: 70, Y: 80},
		AdaptiveCalibrationExecutionConfig{
			PlanConfig: AdaptiveCalibrationPlanConfig{
				BoundsMin:         Vec2{X: 0, Y: 0},
				BoundsMax:         Vec2{X: 100, Y: 100},
				Margin:            5,
				MaxHDist:          20,
				MaxVDist:          20,
				MinimumProbeCount: 3,
				Algorithm:         "lagrange",
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildAdaptiveCalibrationExecution() error = %v", err)
	}
	if strings.Contains(execution.Command, "RELATIVE_REFERENCE_INDEX=") {
		t.Fatalf("command = %q, did not want relative reference index", execution.Command)
	}
}

func TestDetectAdaptiveMeshAreaPrefersSlicerBounds(t *testing.T) {
	result, err := DetectAdaptiveMeshArea(AdaptiveMeshAreaInput{
		AreaStart:      "10,20",
		AreaEnd:        "30,40",
		ExcludeObjects: []map[string]interface{}{{"polygon": [][]float64{{0, 0}, {100, 100}}}},
	}, AdaptiveMeshAreaConfig{
		DefaultMin: Vec2{X: 0, Y: 0},
		DefaultMax: Vec2{X: 200, Y: 200},
	})
	if err != nil {
		t.Fatalf("DetectAdaptiveMeshArea() error = %v", err)
	}
	if result.Source != "slicer" {
		t.Fatalf("Source = %q, want slicer", result.Source)
	}
	if result.MeshMin != (Vec2{X: 10, Y: 20}) || result.MeshMax != (Vec2{X: 30, Y: 40}) {
		t.Fatalf("unexpected slicer bounds min=%+v max=%+v", result.MeshMin, result.MeshMax)
	}
	if !strings.Contains(strings.Join(result.Messages, "\n"), "Use min max boundary detection") {
		t.Fatalf("messages = %#v, want slicer success log", result.Messages)
	}
}

func TestDetectAdaptiveMeshAreaFallsBackToExcludeObjects(t *testing.T) {
	result, err := DetectAdaptiveMeshArea(AdaptiveMeshAreaInput{
		ExcludeObjects: []map[string]interface{}{{
			"polygon": [][]float64{{20, 30}, {40, 60}, {10, 50}},
		}},
	}, AdaptiveMeshAreaConfig{
		DefaultMin:            Vec2{X: 0, Y: 0},
		DefaultMax:            Vec2{X: 200, Y: 200},
		DisableSlicerBoundary: true,
	})
	if err != nil {
		t.Fatalf("DetectAdaptiveMeshArea() error = %v", err)
	}
	if result.Source != "exclude_object" {
		t.Fatalf("Source = %q, want exclude_object", result.Source)
	}
	if result.MeshMin != (Vec2{X: 10, Y: 30}) || result.MeshMax != (Vec2{X: 40, Y: 60}) {
		t.Fatalf("unexpected exclude-object bounds min=%+v max=%+v", result.MeshMin, result.MeshMax)
	}
}

func TestDetectAdaptiveMeshAreaUsesRelativeGCodePath(t *testing.T) {
	tmpDir := t.TempDir()
	gcodeFile := filepath.Join(tmpDir, "print.gcode")
	gcode := strings.Join([]string{
		"G90",
		"G1 Z0.2 F600",
		"G1 X10 Y10 E1 F1500",
		"G1 X50 Y10 E2",
		"G1 X50 Y50 E3",
		"G1 Z0.6",
		"G1 X100 Y100 E4",
	}, "\n")
	if err := os.WriteFile(gcodeFile, []byte(gcode), 0600); err != nil {
		t.Fatalf("failed to write test gcode: %v", err)
	}

	result, err := DetectAdaptiveMeshArea(AdaptiveMeshAreaInput{
		GCodeFilePath: "print.gcode",
	}, AdaptiveMeshAreaConfig{
		DefaultMin:             Vec2{X: 0, Y: 0},
		DefaultMax:             Vec2{X: 200, Y: 200},
		VirtualSDPath:          tmpDir,
		ArcSegments:            8,
		FadeEnd:                0.5,
		DisableSlicerBoundary:  true,
		DisableExcludeBoundary: true,
	})
	if err != nil {
		t.Fatalf("DetectAdaptiveMeshArea() error = %v", err)
	}
	if result.Source != "gcode_analysis" {
		t.Fatalf("Source = %q, want gcode_analysis", result.Source)
	}
	if result.MeshMin != (Vec2{X: 10, Y: 10}) || result.MeshMax != (Vec2{X: 50, Y: 50}) {
		t.Fatalf("unexpected gcode bounds min=%+v max=%+v", result.MeshMin, result.MeshMax)
	}
	if !strings.Contains(strings.Join(result.Messages, "\n"), "Use Gcode analysis boundary detection") {
		t.Fatalf("messages = %#v, want gcode analysis success log", result.Messages)
	}
}

func TestDetectAdaptiveMeshAreaFallsBackToDefault(t *testing.T) {
	result, err := DetectAdaptiveMeshArea(AdaptiveMeshAreaInput{}, AdaptiveMeshAreaConfig{
		DefaultMin:             Vec2{X: 1, Y: 2},
		DefaultMax:             Vec2{X: 3, Y: 4},
		DisableSlicerBoundary:  true,
		DisableExcludeBoundary: true,
		DisableGcodeBoundary:   true,
	})
	if err != nil {
		t.Fatalf("DetectAdaptiveMeshArea() error = %v", err)
	}
	if result.Source != "default" {
		t.Fatalf("Source = %q, want default", result.Source)
	}
	if result.MeshMin != (Vec2{X: 1, Y: 2}) || result.MeshMax != (Vec2{X: 3, Y: 4}) {
		t.Fatalf("unexpected default bounds min=%+v max=%+v", result.MeshMin, result.MeshMax)
	}
	if !strings.Contains(strings.Join(result.Messages, "\n"), "Fallback to default bed mesh") {
		t.Fatalf("messages = %#v, want fallback log", result.Messages)
	}
}

func TestPlanAdaptiveCalibrationCommandCombinesDetectionAndExecution(t *testing.T) {
	result, err := PlanAdaptiveCalibrationCommand(AdaptiveCalibrationCommandRequest{
		AreaInput: AdaptiveMeshAreaInput{
			AreaStart: "10,20",
			AreaEnd:   "30,40",
		},
		AreaConfig: AdaptiveMeshAreaConfig{
			DefaultMin: Vec2{X: 0, Y: 0},
			DefaultMax: Vec2{X: 100, Y: 100},
		},
		ExecutionConfig: AdaptiveCalibrationExecutionConfig{
			PlanConfig: AdaptiveCalibrationPlanConfig{
				BoundsMin:         Vec2{X: 0, Y: 0},
				BoundsMax:         Vec2{X: 100, Y: 100},
				Margin:            5,
				MaxHDist:          20,
				MaxVDist:          20,
				MinimumProbeCount: 3,
				Algorithm:         "lagrange",
			},
			IncludeRelativeReferenceIndex: true,
		},
	})
	if err != nil {
		t.Fatalf("PlanAdaptiveCalibrationCommand() error = %v", err)
	}
	if result.Area.Source != "slicer" {
		t.Fatalf("Area.Source = %q, want slicer", result.Area.Source)
	}
	if result.Execution.Command == "" || !strings.Contains(result.Execution.Command, "BED_MESH_CALIBRATE") {
		t.Fatalf("Execution.Command = %q, want bed mesh calibration command", result.Execution.Command)
	}
	if !strings.Contains(result.Execution.Command, "RELATIVE_REFERENCE_INDEX=") {
		t.Fatalf("Execution.Command = %q, want relative reference index", result.Execution.Command)
	}
	if result.Execution.Plan.ZeroReference != result.Execution.Plan.ProbePoints[result.Execution.Plan.RelativeReferenceIndex] {
		t.Fatalf("unexpected zero reference %+v", result.Execution.Plan.ZeroReference)
	}
}
