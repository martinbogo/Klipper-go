package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goklipper/common/logger"
	"goklipper/common/utils/object"
	addonpkg "goklipper/internal/addon"
	bedmeshpkg "goklipper/internal/pkg/motion/bed_mesh"
	"goklipper/internal/pkg/util"
	printpkg "goklipper/internal/print"
)

type AdaptiveBedMesh struct {
	printer                    *Printer
	gcode                      *GCodeDispatch
	excludeObject              *addonpkg.ExcludeObjectModule
	printStats                 *printpkg.PrintStatsModule
	bedMesh                    *BedMesh
	virtualSDPath              string
	arcSegments                int
	meshAreaClearance          float64
	maxProbeHorizontalDistance float64
	maxProbeVerticalDistance   float64
	useRelativeReferenceIndex  bool
	disableSlicerBoundary      bool
	disableExcludeBoundary     bool
	disableGcodeBoundary       bool
	debugMode                  bool
	minimumAxisProbeCounts     int
	bedMeshConfigMeshMin       []float64
	bedMeshConfigMeshMax       []float64
	bedMeshConfigFadeEnd       float64
	bedMeshConfigAlgorithm     string
}

type vec2 = bedmeshpkg.Vec2
type moveState = bedmeshpkg.MoveState
type moveCommand = bedmeshpkg.MoveCommand

func NewAdaptiveBedMesh(config *ConfigWrapper) *AdaptiveBedMesh {
	self := &AdaptiveBedMesh{}
	self.printer = config.Get_printer()
	self.arcSegments = config.Getint("arc_segments", 80, 1, 0, true)
	self.meshAreaClearance = config.Getfloat("mesh_area_clearance", 5., 0, 0, 0, 0, true)
	self.maxProbeHorizontalDistance = config.Getfloat("max_probe_horizontal_distance", 50., 0, 0, 0, 0, true)
	self.maxProbeVerticalDistance = config.Getfloat("max_probe_vertical_distance", 50., 0, 0, 0, 0, true)
	self.useRelativeReferenceIndex = config.Getboolean("use_relative_reference_index", false, true)

	self.disableSlicerBoundary = config.Getboolean("disable_slicer_min_max_boundary_detection", false, true)
	self.disableExcludeBoundary = config.Getboolean("disable_exclude_object_boundary_detection", false, true)
	self.disableGcodeBoundary = config.Getboolean("disable_gcode_analysis_boundary_detection", false, true)
	self.debugMode = config.Getboolean("debug_mode", false, true)
	self.minimumAxisProbeCounts = 3

	self.gcode = MustLookupGcode(self.printer)

	excludeObj, ok := self.printer.Lookup_object("exclude_object", object.Sentinel{}).(*addonpkg.ExcludeObjectModule)
	if !ok || excludeObj == nil {
		panic(fmt.Sprintf("[adaptive_bed_mesh] requires [exclude_object] to be configured before %s", config.Section))
	}
	self.excludeObject = excludeObj

	printStats, ok := self.printer.Lookup_object("print_stats", object.Sentinel{}).(*printpkg.PrintStatsModule)
	if !ok || printStats == nil {
		panic(fmt.Sprintf("[adaptive_bed_mesh] requires [print_stats] to be configured before %s", config.Section))
	}
	self.printStats = printStats

	bedMesh, ok := self.printer.Lookup_object("bed_mesh", object.Sentinel{}).(*BedMesh)
	if !ok || bedMesh == nil {
		panic(fmt.Sprintf("[adaptive_bed_mesh] requires [bed_mesh] to be configured before %s", config.Section))
	}
	self.bedMesh = bedMesh

	fileConfig := config.Fileconfig()
	if !fileConfig.Has_section("bed_mesh") {
		panic("[adaptive_bed_mesh] missing required [bed_mesh] section")
	}
	bedMeshSection := config.Getsection("bed_mesh")
	self.bedMeshConfigMeshMin = bedMeshSection.Getfloatlist("mesh_min", nil, ",", 2, true)
	self.bedMeshConfigMeshMax = bedMeshSection.Getfloatlist("mesh_max", nil, ",", 2, true)
	self.bedMeshConfigFadeEnd = bedMeshSection.Getfloat("fade_end", 0., 0, 0, 0, 0, true)
	alg := bedMeshSection.Get("algorithm", "lagrange", true).(string)
	self.bedMeshConfigAlgorithm = strings.ToLower(strings.TrimSpace(alg))

	if !fileConfig.Has_section("virtual_sdcard") {
		panic("[adaptive_bed_mesh] missing required [virtual_sdcard] section")
	}
	virtualSection := config.Getsection("virtual_sdcard")
	pathVal := virtualSection.Get("path", object.Sentinel{}, true)
	sdPath, ok := pathVal.(string)
	if !ok {
		panic("[adaptive_bed_mesh] virtual_sdcard.path must be a string")
	}
	self.virtualSDPath = util.Normpath(util.ExpandUser(sdPath))

	self.gcode.Register_command("ADAPTIVE_BED_MESH_CALIBRATE", self.Cmd_ADAPTIVE_BED_MESH_CALIBRATE, false,
		"Run adaptive bed mesh calibration using detected print area")

	return self
}

func (self *AdaptiveBedMesh) Cmd_ADAPTIVE_BED_MESH_CALIBRATE(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	if err := self.cmdAdaptiveBedMeshCalibrate(gcmd); err != nil {
		gcmd.Respond_info(fmt.Sprintf("AdaptiveBedMesh: %v", err), true)
		if !self.debugMode {
			return err
		}
	}
	return nil
}

func (self *AdaptiveBedMesh) cmdAdaptiveBedMeshCalibrate(gcmd *GCodeCommand) error {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic recovered: %v", r)
			self.logToGcode(gcmd, err.Error())
			if !self.debugMode {
				panic(err)
			}
		}
	}()

	meshMin, meshMax, err := self.detectMeshArea(gcmd)
	if err != nil {
		return err
	}

	self.logToGcode(gcmd, fmt.Sprintf("Selected mesh area: min(%.3f, %.3f) max(%.3f, %.3f)", meshMin.X, meshMin.Y, meshMax.X, meshMax.Y))

	return self.executeBedMeshCalibrate(meshMin, meshMax, gcmd)
}

func (self *AdaptiveBedMesh) detectMeshArea(gcmd *GCodeCommand) (vec2, vec2, error) {
	defaultMin := vec2{X: self.bedMeshConfigMeshMin[0], Y: self.bedMeshConfigMeshMin[1]}
	defaultMax := vec2{X: self.bedMeshConfigMeshMax[0], Y: self.bedMeshConfigMeshMax[1]}

	if !self.disableSlicerBoundary {
		self.logToGcode(gcmd, "Attempting to detect boundary by slicer min max")
		areaStart := strings.TrimSpace(gcmd.Params["AREA_START"])
		areaEnd := strings.TrimSpace(gcmd.Params["AREA_END"])
		if areaStart != "" && areaEnd != "" {
			start, errStart := bedmeshpkg.ParseVec2CSV(areaStart)
			end, errEnd := bedmeshpkg.ParseVec2CSV(areaEnd)
			if errStart == nil && errEnd == nil {
				self.logToGcode(gcmd, "Use min max boundary detection")
				return start, end, nil
			}
			self.logToGcode(gcmd, fmt.Sprintf("Failed to parse slicer AREA_* parameters: %v %v", errStart, errEnd))
		} else {
			self.logToGcode(gcmd, "Failed to run slicer min max: No information available")
		}
	}

	if !self.disableExcludeBoundary {
		self.logToGcode(gcmd, "Attempting to detect boundary by exclude boundary")
		excludeObjects := self.excludeObject.Objects()
		if self.debugMode {
			self.logToGcode(gcmd, fmt.Sprintf("Exclude objects count: %d", len(excludeObjects)))
		}
		if len(excludeObjects) > 0 {
			minPt, maxPt, err := self.generateMeshWithExcludeObject(excludeObjects)
			if err != nil {
				self.logToGcode(gcmd, fmt.Sprintf("Failed to run exclude object analysis: %v", err))
			} else {
				self.logToGcode(gcmd, "Use exclude object boundary detection")
				return minPt, maxPt, nil
			}
		} else {
			self.logToGcode(gcmd, "Failed to run exclude object analysis: No exclude object information available")
		}
	}

	if !self.disableGcodeBoundary {
		self.logToGcode(gcmd, "Attempting to detect boundary by Gcode analysis")
		gcodePath := strings.TrimSpace(gcmd.Params["GCODE_FILEPATH"])
		minPt, maxPt, err := self.generateMeshWithGcodeAnalysis(gcodePath)
		if err != nil {
			self.logToGcode(gcmd, fmt.Sprintf("Failed to run Gcode analysis: %v", err))
		} else {
			self.logToGcode(gcmd, "Use Gcode analysis boundary detection")
			return minPt, maxPt, nil
		}
	}

	self.logToGcode(gcmd, "Fallback to default bed mesh")
	return defaultMin, defaultMax, nil
}

func (self *AdaptiveBedMesh) generateMeshWithExcludeObject(objects []map[string]interface{}) (vec2, vec2, error) {
	return bedmeshpkg.ExtractExcludeObjectBounds(objects)
}

func (self *AdaptiveBedMesh) generateMeshWithGcodeAnalysis(gcodePath string) (vec2, vec2, error) {
	resolvedPath := gcodePath
	if resolvedPath == "" {
		if self.printStats.Filename() == "" {
			return vec2{}, vec2{}, fmt.Errorf("no gcode filepath provided and no active print file")
		}
		resolvedPath = filepath.Join(self.virtualSDPath, self.printStats.Filename())
	} else if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(self.virtualSDPath, resolvedPath)
	}

	if _, err := os.Stat(resolvedPath); err != nil {
		return vec2{}, vec2{}, fmt.Errorf("unable to access gcode file: %w", err)
	}

	layers, err := bedmeshpkg.GetLayerVertices(resolvedPath, self.arcSegments)
	if err != nil {
		return vec2{}, vec2{}, err
	}

	return bedmeshpkg.GetLayerMinMaxBeforeFade(layers, self.bedMeshConfigFadeEnd)
}

func (self *AdaptiveBedMesh) logToGcode(gcmd *GCodeCommand, msg string) {
	if gcmd != nil {
		gcmd.Respond_info("AdaptiveBedMesh: "+msg, true)
	} else {
		logger.Debug("AdaptiveBedMesh: " + msg)
	}
}

func (self *AdaptiveBedMesh) executeBedMeshCalibrate(meshMin, meshMax vec2, gcmd *GCodeCommand) error {
	minWithMargin, maxWithMargin := bedmeshpkg.ApplyMinMaxMargin(meshMin, meshMax, self.meshAreaClearance)
	limitedMin, limitedMax := self.applyMinMaxLimit(minWithMargin, maxWithMargin)
	if limitedMax.X <= limitedMin.X || limitedMax.Y <= limitedMin.Y {
		return fmt.Errorf("invalid mesh bounds after applying margin and limits")
	}

	xCount, yCount, probePoints, relIdx := self.getProbePoints(limitedMin, limitedMax)
	if len(probePoints) == 0 {
		return fmt.Errorf("unable to generate probe coordinates")
	}
	if relIdx < 0 || relIdx >= len(probePoints) {
		relIdx = len(probePoints) / 2
	}
	zeroRef := probePoints[relIdx]

	cmd := fmt.Sprintf(
		"BED_MESH_CALIBRATE MESH_MIN=%.3f,%.3f MESH_MAX=%.3f,%.3f PROBE_COUNT=%d,%d",
		limitedMin.X, limitedMin.Y, limitedMax.X, limitedMax.Y, xCount, yCount,
	)

	if self.useRelativeReferenceIndex {
		cmd += fmt.Sprintf(" RELATIVE_REFERENCE_INDEX=%d", relIdx)
		self.bedMesh.Zero_ref_pos = nil
	} else {
		self.bedMesh.Zero_ref_pos = []float64{zeroRef.X, zeroRef.Y}
	}

	self.logToGcode(gcmd, cmd)
	self.gcode.Run_script_from_command(cmd)
	return nil
}

func (self *AdaptiveBedMesh) applyMinMaxLimit(minPt, maxPt vec2) (vec2, vec2) {
	boundsMin := vec2{X: self.bedMeshConfigMeshMin[0], Y: self.bedMeshConfigMeshMin[1]}
	boundsMax := vec2{X: self.bedMeshConfigMeshMax[0], Y: self.bedMeshConfigMeshMax[1]}
	return bedmeshpkg.ApplyMinMaxLimit(minPt, maxPt, boundsMin, boundsMax)
}

func (self *AdaptiveBedMesh) getProbePoints(minPt, maxPt vec2) (int, int, []vec2, int) {
	return bedmeshpkg.GenerateProbeGrid(minPt, maxPt, bedmeshpkg.ProbeGridConfig{
		MaxHDist:  self.maxProbeHorizontalDistance,
		MaxVDist:  self.maxProbeVerticalDistance,
		MinCount:  self.minimumAxisProbeCounts,
		Algorithm: self.bedMeshConfigAlgorithm,
	})
}

func (self *AdaptiveBedMesh) applyProbePointLimits(xCount, yCount int) (int, int) {
	return bedmeshpkg.ApplyProbePointLimits(xCount, yCount, bedmeshpkg.ProbeGridConfig{
		MinCount:  self.minimumAxisProbeCounts,
		Algorithm: self.bedMeshConfigAlgorithm,
	})
}

func Load_config_adaptive_bed_mesh(config *ConfigWrapper) interface{} {
	return NewAdaptiveBedMesh(config)
}
