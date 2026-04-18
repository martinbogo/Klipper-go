package bedmesh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AdaptiveMeshAreaConfig struct {
	DefaultMin             Vec2
	DefaultMax             Vec2
	FadeEnd                float64
	VirtualSDPath          string
	ArcSegments            int
	DisableSlicerBoundary  bool
	DisableExcludeBoundary bool
	DisableGcodeBoundary   bool
}

type AdaptiveMeshAreaInput struct {
	AreaStart      string
	AreaEnd        string
	ExcludeObjects []map[string]interface{}
	GCodeFilePath  string
	ActiveFilename string
}

type AdaptiveMeshAreaResult struct {
	MeshMin  Vec2
	MeshMax  Vec2
	Source   string
	Messages []string
}

func DetectAdaptiveMeshArea(input AdaptiveMeshAreaInput, cfg AdaptiveMeshAreaConfig) (AdaptiveMeshAreaResult, error) {
	result := AdaptiveMeshAreaResult{
		MeshMin: cfg.DefaultMin,
		MeshMax: cfg.DefaultMax,
		Source:  "default",
	}
	if cfg.DefaultMax.X <= cfg.DefaultMin.X || cfg.DefaultMax.Y <= cfg.DefaultMin.Y {
		return result, fmt.Errorf("invalid default bed mesh bounds")
	}

	if !cfg.DisableSlicerBoundary {
		result.Messages = append(result.Messages, "Attempting to detect boundary by slicer min max")
		areaStart := strings.TrimSpace(input.AreaStart)
		areaEnd := strings.TrimSpace(input.AreaEnd)
		if areaStart != "" && areaEnd != "" {
			start, errStart := ParseVec2CSV(areaStart)
			end, errEnd := ParseVec2CSV(areaEnd)
			if errStart == nil && errEnd == nil {
				result.Messages = append(result.Messages, "Use min max boundary detection")
				result.MeshMin = start
				result.MeshMax = end
				result.Source = "slicer"
				return result, nil
			}
			result.Messages = append(result.Messages, fmt.Sprintf("Failed to parse slicer AREA_* parameters: %v %v", errStart, errEnd))
		} else {
			result.Messages = append(result.Messages, "Failed to run slicer min max: No information available")
		}
	}

	if !cfg.DisableExcludeBoundary {
		result.Messages = append(result.Messages, "Attempting to detect boundary by exclude boundary")
		if len(input.ExcludeObjects) > 0 {
			minPt, maxPt, err := ExtractExcludeObjectBounds(input.ExcludeObjects)
			if err != nil {
				result.Messages = append(result.Messages, fmt.Sprintf("Failed to run exclude object analysis: %v", err))
			} else {
				result.Messages = append(result.Messages, "Use exclude object boundary detection")
				result.MeshMin = minPt
				result.MeshMax = maxPt
				result.Source = "exclude_object"
				return result, nil
			}
		} else {
			result.Messages = append(result.Messages, "Failed to run exclude object analysis: No exclude object information available")
		}
	}

	if !cfg.DisableGcodeBoundary {
		result.Messages = append(result.Messages, "Attempting to detect boundary by Gcode analysis")
		minPt, maxPt, err := DetectAdaptiveMeshAreaFromGCode(input.GCodeFilePath, input.ActiveFilename, cfg.VirtualSDPath, cfg.ArcSegments, cfg.FadeEnd)
		if err != nil {
			result.Messages = append(result.Messages, fmt.Sprintf("Failed to run Gcode analysis: %v", err))
		} else {
			result.Messages = append(result.Messages, "Use Gcode analysis boundary detection")
			result.MeshMin = minPt
			result.MeshMax = maxPt
			result.Source = "gcode_analysis"
			return result, nil
		}
	}

	result.Messages = append(result.Messages, "Fallback to default bed mesh")
	return result, nil
}

func DetectAdaptiveMeshAreaFromGCode(gcodePath, activeFilename, virtualSDPath string, arcSegments int, fadeEnd float64) (Vec2, Vec2, error) {
	resolvedPath, err := resolveAdaptiveMeshGCodePath(gcodePath, activeFilename, virtualSDPath)
	if err != nil {
		return Vec2{}, Vec2{}, err
	}
	if _, err := os.Stat(resolvedPath); err != nil {
		return Vec2{}, Vec2{}, fmt.Errorf("unable to access gcode file: %w", err)
	}
	layers, err := GetLayerVertices(resolvedPath, arcSegments)
	if err != nil {
		return Vec2{}, Vec2{}, err
	}
	return GetLayerMinMaxBeforeFade(layers, fadeEnd)
}

func resolveAdaptiveMeshGCodePath(gcodePath, activeFilename, virtualSDPath string) (string, error) {
	resolvedPath := strings.TrimSpace(gcodePath)
	if resolvedPath == "" {
		activeFilename = strings.TrimSpace(activeFilename)
		if activeFilename == "" {
			return "", fmt.Errorf("no gcode filepath provided and no active print file")
		}
		resolvedPath = activeFilename
	}
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(virtualSDPath, resolvedPath)
	}
	return resolvedPath, nil
}
