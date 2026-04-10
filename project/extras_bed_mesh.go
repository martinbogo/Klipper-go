package project

import (
	"encoding/json"
	"fmt"
	"goklipper/common/lock"
	"goklipper/common/logger"
	"goklipper/common/utils/maths"
	"goklipper/common/utils/object"
	"goklipper/common/utils/sys"
	addonpkg "goklipper/internal/addon"
	gcodepkg "goklipper/internal/pkg/gcode"
	bedmeshpkg "goklipper/internal/pkg/motion/bed_mesh"
	"math"
	"reflect"
	"strconv"
	"strings"
)

const PROFILE_VERSION = 1

var PROFILE_OPTIONS = map[string]reflect.Kind{
	"min_x": reflect.Float64, "max_x": reflect.Float64, "min_y": reflect.Float64, "max_y": reflect.Float64,
	"x_count": reflect.Int, "y_count": reflect.Int, "mesh_x_pps": reflect.Int, "mesh_y_pps": reflect.Int,
	"algo": reflect.String, "tension": reflect.Float64}

// retreive commma separated pair from config
func Parse_config_pair(config *ConfigWrapper, option string, default_ interface{}, minval float64, maxval float64) []int {
	pair := config.Getintlist(option, []interface{}{default_, default_}, ",", 0, true)
	if len(pair) != 2 {
		if len(pair) != 1 {
			panic(fmt.Sprintf("bed_mesh: malformed '%s' value: %s",
				option,
				config.Get(option, object.Sentinel{}, true)))
		}

		// pair = (pair[0], pair[0])
		pair[1] = pair[0]
	}
	if minval != 0 {
		if float64(pair[0]) < minval || float64(pair[1]) < minval {
			panic(fmt.Sprintf("Option '%s' in section bed_mesh must have a minimum of %s",
				option, strconv.FormatFloat(minval, 'f', -1, 64)))
		}
	}
	if maxval != 0 {
		if float64(pair[0]) > maxval || float64(pair[1]) > maxval {
			panic(fmt.Sprintf("Option '%s' in section bed_mesh must have a maximum of %s",
				option, strconv.FormatFloat(maxval, 'f', -1, 64)))
		}
	}
	return pair

}

// retreive commma separated pair from a g-code command
func Parse_gcmd_pair(gcmd *GCodeCommand, name string, minval *float64, maxval *float64) []int {
	var pair []int
	name_arr := strings.Split(gcmd.Get(name, nil, "", nil,
		nil, nil, nil), ",")
	for _, v := range name_arr {
		pair_val, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			panic(fmt.Sprintf("Unable to parse parameter '%s'", name))
		}
		pair = append(pair, pair_val)
	}
	if len(pair) != 2 {
		if len(pair) != 1 {
			panic(fmt.Sprintf("Unable to parse parameter '%s'", name))
		}
		// pair = (pair[0], pair[0])
		pair[1] = pair[0]
	}
	if minval != nil {
		if float64(pair[0]) < *minval || float64(pair[1]) < *minval {
			panic(fmt.Sprintf("Parameter '%s' must have a minimum of %d",
				name, minval))
		}
	}
	if maxval != nil {
		if float64(pair[0]) > *maxval || float64(pair[1]) > *maxval {
			panic(fmt.Sprintf("Parameter '%s' must have a maximum of %d", name, maxval))
		}
	}
	return pair
}

// retreive commma separated coordinate from a g-code command
func Parse_gcmd_coord(gcmd *GCodeCommand, name string) (float64, float64) {
	name_arr := strings.Split(gcmd.Get(name, nil, "", nil, nil, nil, nil), ",")
	v1, err := strconv.ParseFloat(strings.TrimSpace(name_arr[0]), 64)
	v2, err1 := strconv.ParseFloat(strings.TrimSpace(name_arr[1]), 64)
	if err != nil || err1 != nil {
		panic(fmt.Sprintf("Unable to parse parameter '%s'", name))
	}
	return v1, v2

}

const FADE_DISABLE = 0x7FFFFFFF

type BedMesh struct {
	Printer           *Printer
	Last_position     []float64
	Target_position   []float64
	Bmc               *BedMeshCalibrate
	Z_mesh            *bedmeshpkg.ZMesh
	Toolhead          *Toolhead
	Horizontal_move_z float64
	Fade_start        float64
	Fade_end          float64
	Fade_dist         float64
	Log_fade_complete bool
	Base_fade_target  float64
	Fade_target       float64
	Zero_ref_pos      []float64
	Gcode             *GCodeDispatch
	Splitter          *bedmeshpkg.MoveSplitter
	Pmgr              *ProfileManager
	Save_profile      func(string) error
	Status            map[string]interface{}
	Sl                lock.SpinLock
	move_transform    gcodepkg.LegacyMoveTransform
}

func NewBedMesh(config *ConfigWrapper) *BedMesh {
	self := &BedMesh{}
	self.Printer = config.Get_printer()
	self.Printer.Register_event_handler("project:connect", self.Handle_connect)
	self.Last_position = []float64{0., 0., 0., 0.}
	self.Bmc = NewBedMeshCalibrate(config, self)
	self.Toolhead = nil
	self.Z_mesh = nil
	self.Zero_ref_pos = nil
	self.Horizontal_move_z = config.Getfloat("horizontal_move_z", 5., 0, 0, 0, 0, true)
	self.Fade_start = config.Getfloat("fade_start", 1., 0, 0, 0, 0, true)
	self.Fade_end = config.Getfloat("fade_end", 0., 0, 0, 0, 0, true)
	self.Fade_dist = self.Fade_end - self.Fade_start
	if self.Fade_dist <= 0. {
		self.Fade_start, self.Fade_end = FADE_DISABLE, FADE_DISABLE
	}
	self.Log_fade_complete = false
	self.Base_fade_target = config.Getfloat("fade_target", 0., 0, 0, 0, 0, true)
	self.Fade_target = 0.
	gcode_obj := self.Printer.Lookup_object("gcode", object.Sentinel{})
	//if err != nil {
	//	logger.Error(err)
	//}
	self.Gcode = gcode_obj.(*GCodeDispatch)
	self.Splitter = bedmeshpkg.NewMoveSplitter(
		config.Getfloat("split_delta_z", .025, 0.01, 0, 0, 0, true),
		config.Getfloat("move_check_distance", 5., 1., 0, 0, 0, true),
	)
	// setup persistent storage
	self.Pmgr = NewProfileManager(config, self)
	self.Save_profile = self.Pmgr.Save_profile
	// register gcodes
	self.Gcode.Register_command(
		"BED_MESH_OUTPUT", self.Cmd_BED_MESH_OUTPUT,
		false, cmd_BED_MESH_OUTPUT_help)
	self.Gcode.Register_command("BED_MESH_MAP", self.Cmd_BED_MESH_MAP,
		false, cmd_BED_MESH_MAP_help)
	self.Gcode.Register_command(
		"BED_MESH_CLEAR", self.Cmd_BED_MESH_CLEAR,
		false, cmd_BED_MESH_CLEAR_help)
	self.Gcode.Register_command(
		"BED_MESH_OFFSET", self.Cmd_BED_MESH_OFFSET,
		false, cmd_BED_MESH_OFFSET_help)
	// Register transform
	gcode_move := self.Printer.Load_object(config, "gcode_move", object.Sentinel{})
	gcode_move.(*gcodepkg.GCodeMoveModule).Set_move_transform(self, false)
	// initialize status dict
	self.Update_status()
	return self
}

func (self *BedMesh) Handle_connect(event []interface{}) error {
	toolhead_obj := self.Printer.Lookup_object("toolhead", object.Sentinel{})
	self.Toolhead = toolhead_obj.(*Toolhead)
	// self.Bmc.Print_generated_points(self.Gcode.Respond_info)
	return nil
}
func (self *BedMesh) Set_mesh(mesh *bedmeshpkg.ZMesh) {
	if mesh != nil && self.Fade_end != FADE_DISABLE {
		self.Log_fade_complete = true
		if self.Base_fade_target == 0.0 {
			self.Fade_target = mesh.Avg_z
		} else {
			self.Fade_target = self.Base_fade_target
			minZ, maxZ := mesh.Get_z_range()
			if !(minZ <= self.Fade_target && self.Fade_target <= maxZ) && self.Fade_target != 0.0 {
				// fade target is non-zero, out of mesh range
				errTarget := self.Fade_target
				self.Z_mesh = nil
				self.Fade_target = 0.0
				panic(fmt.Sprintf("bed_mesh: ERROR, fade_target lies outside of mesh z range\nmin: %.4f, max: %.4f, fade_target: %.4f", minZ, maxZ, errTarget))
			}
		}
		minZ, maxZ := mesh.Get_z_range()
		if self.Fade_dist <= math.Max(math.Abs(minZ), math.Abs(maxZ)) {
			self.Z_mesh = nil
			self.Fade_target = 0.0
			panic(fmt.Sprintf("bed_mesh:  Mesh extends outside of the fade range, please see the fade_start and fade_end options in example-extras.cfg. fade distance: %.2f mesh min: %.4f mesh max: %.4f",
				self.Fade_dist, minZ, maxZ))
		}
	} else {
		self.Fade_target = 0.0
	}
	self.Z_mesh = mesh
	self.Splitter.Initialize(mesh, self.Fade_target)
	// cache the current position before a transform takes place
	gcode_move := self.Printer.Lookup_object("gcode_move", object.Sentinel{})
	gcode_move.(*gcodepkg.GCodeMoveModule).Reset_last_position(nil)
	self.Update_status()
}
func (self *BedMesh) Get_z_factor(z_pos float64) float64 {
	return bedmeshpkg.CalcZFadeFactor(z_pos, self.Fade_start, self.Fade_end)
}
func (self *BedMesh) Get_position() []float64 {
	// Return last, non-transformed position
	if self.Z_mesh == nil {
		// No mesh calibrated, so send toolhead position
		self.Last_position = append([]float64{}, self.Toolhead.Get_position()...)
		self.Last_position[2] -= self.Fade_target
	} else {
		// return current position minus the current z-adjustment
		x := self.Toolhead.Get_position()[0]
		y := self.Toolhead.Get_position()[1]
		z := self.Toolhead.Get_position()[2]
		e := self.Toolhead.Get_position()[3]
		max_adj := self.Z_mesh.Calc_z(x, y)
		factor := 1.
		z_adj := max_adj - self.Fade_target
		if math.Min(z, z-max_adj) >= self.Fade_end {
			// Fade out is complete, no factor
			factor = 0.
		} else if math.Max(z, z-max_adj) >= self.Fade_start {
			// Likely in the process of fading out adjustment.
			// Because we don't yet know the gcode z position, use
			// algebra to calculate the factor from the toolhead pos
			factor = (self.Fade_end + self.Fade_target - z) /
				(self.Fade_dist - z_adj)
			factor = bedmeshpkg.Constrain(factor, 0., 1.)
		}
		final_z_adj := factor*z_adj + self.Fade_target
		self.Last_position = []float64{x, y, z - final_z_adj, e}
	}
	last_position := make([]float64, len(self.Last_position))
	copy(last_position, self.Last_position)
	return last_position
}
func (self *BedMesh) Move(newpos []float64, speed float64) {
	// self.Sl.Lock()
	// defer self.Sl.UnLock()
	target_position := make([]float64, len(newpos))
	copy(target_position, newpos)
	self.Target_position = target_position
	factor := self.Get_z_factor(newpos[2])
	if self.Z_mesh == nil || factor == 0. {
		// No mesh calibrated, or mesh leveling phased out.
		x := newpos[0]
		y := newpos[1]
		z := newpos[2]
		e := newpos[3]
		if self.Log_fade_complete {
			self.Log_fade_complete = false
			logger.Debugf("bed_mesh fade complete: Current Z: %.4f fade_target: %.4f ", z, self.Fade_target)
		}
		self.Toolhead.Move([]float64{x, y, z + self.Fade_target, e}, speed)
	} else {
		self.Splitter.Build_move(self.Last_position, newpos, factor)
		for !self.Splitter.Traverse_complete {
			split_move := self.Splitter.Split()
			if len(split_move) > 0 {
				self.Toolhead.Move(split_move, speed)
			} else {
				panic("Mesh Leveling: Error splitting move ")
			}
		}

	}
	self.Last_position = append([]float64{}, newpos...)
}
func (self *BedMesh) Get_status(eventtime float64) map[string]interface{} {
	return sys.DeepCopyMap(self.Status)
}

func (self *BedMesh) Update_status() {
	self.Status = map[string]interface{}{
		"profile_name":  "",
		"mesh_min":      []float64{0., 0.},
		"mesh_max":      []float64{0., 0.},
		"probed_matrix": [][]float64{},
		"mesh_matrix":   [][]float64{},
		"profiles":      self.Pmgr.Get_profiles(),
	}
	if self.Z_mesh != nil {
		params := self.Z_mesh.Get_mesh_params()
		mesh_min := []float64{params["min_x"].(float64), params["min_y"].(float64)}
		mesh_max := []float64{params["max_x"].(float64), params["max_y"].(float64)}
		probed_matrix := self.Z_mesh.Get_probed_matrix()
		mesh_matrix := self.Z_mesh.Get_mesh_matrix()
		self.Status["profile_name"] = self.Pmgr.Get_current_profile()
		self.Status["mesh_min"] = mesh_min
		self.Status["mesh_max"] = mesh_max
		self.Status["probed_matrix"] = probed_matrix
		self.Status["mesh_matrix"] = mesh_matrix
	}
}
func (self *BedMesh) Get_mesh() *bedmeshpkg.ZMesh {
	return self.Z_mesh
}

const cmd_BED_MESH_OUTPUT_help = "Retrieve interpolated grid of probed z-points"

func (self *BedMesh) Cmd_BED_MESH_OUTPUT(gcmd *GCodeCommand) {
	if gcmd.Get_int("PGP", 0, nil, nil) > 0 {
		// Print Generated Points instead of mesh
		self.Bmc.Print_generated_points(gcmd.Respond_info)
	} else if self.Z_mesh == nil {
		gcmd.Respond_info("Bed has not been probed", true)
	} else {
		self.Z_mesh.Print_probed_matrix(gcmd.Respond_info)
		horizontal_move_z := int(self.Horizontal_move_z)
		self.Z_mesh.Print_mesh(logger.Debug, &horizontal_move_z)
	}
}

const cmd_BED_MESH_MAP_help = "Serialize mesh and output to terminal"

func (self *BedMesh) Cmd_BED_MESH_MAP(gcmd *GCodeCommand) error {
	if self.Z_mesh != nil {
		params := self.Z_mesh.Get_mesh_params()
		outdict := map[string]interface{}{
			"mesh_min":    []float64{params["min_x"].(float64), params["min_y"].(float64)},
			"mesh_max":    []float64{params["max_x"].(float64), params["max_y"].(float64)},
			"z_positions": self.Z_mesh.Get_probed_matrix()}
		jsonStr, _ := json.Marshal(outdict)
		gcmd.Respond_raw("mesh_map_output " + string(jsonStr))
	} else {
		gcmd.Respond_info("Bed has not been probed", true)
	}
	return nil
}

const cmd_BED_MESH_CLEAR_help = "Clear the Mesh so no z-adjustment is made"

func (self *BedMesh) Cmd_BED_MESH_CLEAR(gcmd interface{}) error {
	self.Set_mesh(nil)
	return nil
}

const cmd_BED_MESH_OFFSET_help = "Add X/Y offsets to the mesh lookup"

func (self *BedMesh) Cmd_BED_MESH_OFFSET(gcmd *GCodeCommand) error {
	if self.Z_mesh != nil {
		offsets := make([]float64, 2)
		for i, Axis := range []string{"x", "y"} {
			offsets[i] = gcmd.Get_float(Axis, nil, nil, nil, nil, nil)
		}
		self.Z_mesh.Set_mesh_offsets(offsets)
		gcode_move_obj := self.Printer.Lookup_object("gcode_move", object.Sentinel{})
		//if err != nil {
		//	logger.Error(err)
		//}
		gcode_move := gcode_move_obj.(*gcodepkg.GCodeMoveModule)
		gcode_move.Reset_last_position(nil)
	} else {
		gcmd.Respond_info("No mesh loaded to offset", true)
	}
	return nil
}

type BedMeshCalibrate struct {
	Printer                  *Printer
	Orig_config              map[string]interface{}
	Radius                   *float64
	Origin                   []float64
	Mesh_min                 []float64
	Mesh_max                 []float64
	Relative_reference_index *int
	Faulty_regions           [][][]float64
	Substituted_indices      []bedmeshpkg.PointSubstitution
	Bedmesh                  *BedMesh
	Mesh_config              map[string]interface{}
	Orig_points              [][]float64
	Points                   [][]float64
	Profile_name             string
	Probe_helper             *ProbePointsHelper
	Gcode                    *GCodeDispatch
	config                   *ConfigWrapper
	adaptive_margin          float64
	min_bedmesh_area_size    []float64
}

func NewBedMeshCalibrate(config *ConfigWrapper, bedmesh *BedMesh) *BedMeshCalibrate {
	self := &BedMeshCalibrate{}
	self.config = config
	self.Printer = config.Get_printer()
	self.Orig_config = map[string]interface{}{}
	self.Orig_config["radius"] = nil
	self.Orig_config["origin"] = nil
	self.Radius = nil
	self.Origin = nil
	self.Mesh_min = []float64{0., 0.}
	self.Mesh_max = []float64{0., 0.}
	self.Relative_reference_index = get_relative_reference_index(config, self)
	self.Faulty_regions = [][][]float64{}
	self.Substituted_indices = nil
	self.Orig_config["rri"] = self.Relative_reference_index
	self.adaptive_margin = config.Getfloat("adaptive_margin", 0.0, 0., 0., 0., 0., true)
	self.Bedmesh = bedmesh
	self.min_bedmesh_area_size = config.Getfloatlist("min_bedmesh_area_size", []float64{100.0, 100.0}, ",", 2, true)
	self.Mesh_config = map[string]interface{}{}
	self.Init_mesh_config(config)
	self.Generate_points()
	self.Profile_name = ""
	self.Orig_points = self.Points
	self.Probe_helper = NewProbePointsHelper(
		config, self.Probe_finalize, self.Get_adjusted_points())
	self.Probe_helper.Minimum_points(3)
	self.Probe_helper.Use_xy_offsets(true)
	gcode_obj := self.Printer.Lookup_object("gcode", object.Sentinel{})
	//if err != nil {
	//	logger.Error(err)
	//}
	self.Gcode = gcode_obj.(*GCodeDispatch)
	self.Gcode.Register_command(
		"BED_MESH_CALIBRATE", self.Cmd_BED_MESH_CALIBRATE,
		false, cmd_BED_MESH_CALIBRATE_help)
	/* self.Gcode.Register_command(
	"BED_MESH_CALIBRATE_BY_LEIVQ", self.Cmd_BED_MESH_CALIBRATE_BY_LEIVQ,
	false, cmd_BED_MESH_CALIBRATE_help) */
	return self
}

func get_relative_reference_index(config *ConfigWrapper, self *BedMeshCalibrate) *int {
	if !config.Fileconfig().Has_option(config.Section, "relative_reference_index") {
		return nil
	} else {
		v := config.Fileconfig().Getint(config.Section, "relative_reference_index")
		relative_reference_index := v.(int)
		self.Relative_reference_index = &relative_reference_index
		return &relative_reference_index
	}
}
func (self *BedMeshCalibrate) Generate_points() {
	regions := make([]bedmeshpkg.FaultyRegion, 0, len(self.Faulty_regions))
	for _, region := range self.Faulty_regions {
		if len(region) < 2 {
			continue
		}
		regions = append(regions, bedmeshpkg.FaultyRegion{
			Min: append([]float64(nil), region[0]...),
			Max: append([]float64(nil), region[1]...),
		})
	}
	probeResult, err := bedmeshpkg.GenerateProbePoints(
		self.Radius,
		append([]float64(nil), self.Origin...),
		append([]float64(nil), self.Mesh_min...),
		append([]float64(nil), self.Mesh_max...),
		self.Mesh_config["x_count"].(int),
		self.Mesh_config["y_count"].(int),
		regions,
	)
	if err != nil {
		panic(err)
	}
	self.Points = probeResult.Points
	self.Substituted_indices = probeResult.Substitutions
}
func (self *BedMeshCalibrate) Print_generated_points(print_func func(msg string, log bool)) {
	xOffset, yOffset := 0., 0.
	probe_obj := self.Printer.Lookup_object("probe", nil)
	if probe_obj != nil {
		xOffset, yOffset, _ = probe_obj.(*PrinterProbe).Get_offsets()
	}
	print_func("bed_mesh: generated points\nIndex| Tool Adjusted | Probe", true)
	for i, v := range self.Points {
		adjPt := fmt.Sprintf("(%.1f, %.1f)", v[0]-xOffset, v[1]-yOffset)
		meshPt := fmt.Sprintf("(%.1f, %.1f)", v[0], v[1])
		print_func(fmt.Sprintf("%-4d| %-16s | %s", i, adjPt, meshPt), true)
	}
	if self.Relative_reference_index != nil {
		rri := *self.Relative_reference_index
		pt := self.Points[rri]
		print_func(fmt.Sprintf("bed_mesh: relative_reference_index %d is (%.2f, %.2f)", rri, pt[0], pt[1]), true)
	}
	if len(self.Substituted_indices) != 0 {
		print_func("bed_mesh: faulty region points", true)
		for _, substitution := range self.Substituted_indices {
			pt := self.Points[substitution.Index]
			print_func(fmt.Sprintf("%d (%.2f, %.2f), substituted points: %v", substitution.Index, pt[0], pt[1], substitution.Points), true)
		}
	}
}
func (self *BedMeshCalibrate) Init_mesh_config(config *ConfigWrapper) {
	mesh_cfg := self.Mesh_config
	orig_cfg := self.Orig_config
	radius := config.Getfloat("mesh_radius", 0, 0, 0, 0., 0, true)
	self.Radius = &radius
	min_x, min_y, max_x, max_y := 0., 0., 0., 0.
	x_cnt, y_cnt := 0, 0
	if *self.Radius != 0 {
		origin := config.Getfloatlist("mesh_radius", 0, ",", 2, true)
		self.Origin = origin
		x_cnt = config.Getint("round_probe_count", 5, 3, 0, true)
		y_cnt = x_cnt
		// round beds must have an odd number of points along each axis
		if x_cnt&1 == 0 {
			panic("bed_mesh: probe_count must be odd for round beds")
		}
		// radius may have precision to .1mm
		radius := math.Floor(*self.Radius*10) / 10
		self.Radius = &radius
		orig_cfg["radius"] = radius
		orig_cfg["origin"] = self.Origin
		min_x = -*self.Radius
		min_y = -*self.Radius
		max_x = *self.Radius
		max_y = *self.Radius
	} else {
		// rectangular
		val1, val2 := 3., 0.
		pps := Parse_config_pair(config, "probe_count", 3, val1, val2)
		x_cnt = pps[0]
		y_cnt = pps[1]
		float_arr1 := config.Getfloatlist("mesh_min", nil, ",", 2, true)
		min_x = float_arr1[0]
		min_y = float_arr1[1]
		float_arr2 := config.Getfloatlist("mesh_max", nil, ",", 2, true)
		max_x = float_arr2[0]
		max_y = float_arr2[1]
		if max_x <= min_x || max_y <= min_y {
			panic("bed_mesh: invalid min/max points")
		}
	}
	orig_cfg["x_count"] = x_cnt
	mesh_cfg["x_count"] = x_cnt
	orig_cfg["y_count"] = y_cnt
	mesh_cfg["y_count"] = y_cnt
	orig_cfg["mesh_min"] = []float64{min_x, min_y}
	self.Mesh_min = []float64{min_x, min_y}
	orig_cfg["mesh_max"] = []float64{max_x, max_y}
	self.Mesh_max = []float64{max_x, max_y}

	pps := Parse_config_pair(config, "mesh_pps", 2, 0., 0.)
	orig_cfg["mesh_x_pps"] = pps[0]
	mesh_cfg["mesh_x_pps"] = pps[0]
	orig_cfg["mesh_y_pps"] = pps[1]
	mesh_cfg["mesh_y_pps"] = pps[1]
	orig_cfg["algo"] = strings.ToLower(strings.TrimSpace(config.Get("algorithm", "lagrange", true).(string)))
	mesh_cfg["algo"] = orig_cfg["algo"]
	orig_cfg["tension"] = config.Getfloat("bicubic_tension", .2, 0., 2., 0, 0, true)
	mesh_cfg["tension"] = orig_cfg["tension"]
	for i := 1; i < 100; i++ {
		start := config.Getfloatlist(fmt.Sprintf("faulty_region_%d_min", i), nil, ",", 2, true)
		if len(start) == 0 {
			break
		}
		end := config.Getfloatlist(fmt.Sprintf("faulty_region_%d_max", i), nil, ",", 2, true)
		c1, c3, c2, c4, err := bedmeshpkg.NormalizeFaultyRegionCorners(start, end)
		if err != nil {
			panic(err.Error())
		}
		if err := bedmeshpkg.ValidateFaultyRegionOverlap(c1, c3, c2, c4, self.Faulty_regions, i); err != nil {
			panic(err.Error())
		}
		self.Faulty_regions = append(self.Faulty_regions, [][]float64{c1, c3})
	}
	self.Verify_algorithm()
}

func (self *BedMeshCalibrate) Verify_algorithm() {
	params := self.Mesh_config
	normalizedAlgo, forced, err := bedmeshpkg.NormalizeMeshAlgorithm(
		params["algo"].(string),
		params["mesh_x_pps"].(int),
		params["mesh_y_pps"].(int),
		params["x_count"].(int),
		params["y_count"].(int),
	)
	if err != nil {
		panic(err)
	}
	if forced && normalizedAlgo == "lagrange" {
		logger.Debugf(
			"bed_mesh: bicubic interpolation with a probe_count of less than 4 points detected.  Forcing lagrange interpolation. Configured Probe Count: %d, %d",
			self.Mesh_config["x_count"], self.Mesh_config["y_count"],
		)
	}
	params["algo"] = normalizedAlgo
}
func (self *BedMeshCalibrate) set_adaptive_mesh(gcmd *GCodeCommand) bool {
	if gcmd.Get_int("ADAPTIVE", 0, nil, nil) == 0 {
		return false
	}
	exclude_objects := self.Printer.Lookup_object("exclude_object", nil).(*addonpkg.ExcludeObjectModule)
	if exclude_objects == nil {
		gcmd.Respond_info("Exclude objects not enabled. Using full mesh...", true)
		return false
	}
	objects := exclude_objects.Get_status(0)["objects"]
	if objects == nil {
		return false
	}

	margin := gcmd.Get_float("ADAPTIVE_MARGIN", self.adaptive_margin, nil, nil, nil, nil)
	//# List all exclude_object points by axis and iterate over
	//# all polygon points, and pick the min and max or each axis
	var list_of_xs, list_of_ys []float64
	gcmd.Respond_info(fmt.Sprintf("Found %d objects", len(objects.([]map[string]interface{}))), true)
	for _, obj := range objects.([]map[string]interface{}) {
		for _, point := range obj["polygon"].([][]float64) {
			list_of_xs = append(list_of_xs, point[0])
			list_of_ys = append(list_of_ys, point[1])
		}
	}
	x_min := bedmeshpkg.SliceMin(list_of_xs)
	y_min := bedmeshpkg.SliceMin(list_of_ys)
	x_max := bedmeshpkg.SliceMax(list_of_xs)
	y_max := bedmeshpkg.SliceMax(list_of_ys)

	mesh_min := [2]float64{math.Min(x_min, x_max), math.Min(y_min, y_max)}
	mesh_max := [2]float64{math.Max(x_min, x_max), math.Max(y_min, y_max)}

	adjusted_mesh_min := []float64{mesh_min[0] - margin, mesh_min[1] - margin}
	adjusted_mesh_max := []float64{mesh_max[0] + margin, mesh_max[1] + margin}
	logger.Debug(fmt.Sprintf("Adapted mesh bounds: (%f,%f)", adjusted_mesh_min, adjusted_mesh_max))

	adjusted_mesh_min[0] = math.Max(adjusted_mesh_min[0], self.Orig_config["mesh_min"].([]float64)[0])
	adjusted_mesh_min[1] = math.Max(adjusted_mesh_min[1], self.Orig_config["mesh_min"].([]float64)[1])
	adjusted_mesh_max[0] = math.Min(adjusted_mesh_max[0], self.Orig_config["mesh_max"].([]float64)[0])
	adjusted_mesh_max[1] = math.Min(adjusted_mesh_max[1], self.Orig_config["mesh_max"].([]float64)[1])
	var adjusted_mesh_size = []float64{adjusted_mesh_max[0] - adjusted_mesh_min[0],
		adjusted_mesh_max[1] - adjusted_mesh_min[1]}

	var ratio = []float64{adjusted_mesh_size[0] / (self.Orig_config["mesh_max"].([]float64)[0] - self.Orig_config["mesh_min"].([]float64)[0]),
		adjusted_mesh_size[1] / (self.Orig_config["mesh_max"].([]float64)[1] - self.Orig_config["mesh_min"].([]float64)[1])}
	logger.Debug(fmt.Sprintf("Original mesh bounds: (%v,%v)", self.Orig_config["mesh_min"], self.Orig_config["mesh_max"]))
	logger.Debug(fmt.Sprintf("Original probe count: (%v,%v)", self.Mesh_config["x_count"], self.Mesh_config["y_count"]))
	logger.Debug(fmt.Sprintf("Adapted mesh bounds: (%f,%f)", adjusted_mesh_min, adjusted_mesh_max))
	logger.Debug(fmt.Sprintf("Ratio: (%f, %f)", ratio[0], ratio[1]))

	new_x_probe_count := int(
		math.Ceil(float64(self.Mesh_config["x_count"].(int)) * ratio[0]))
	new_y_probe_count := int(
		math.Ceil(float64(self.Mesh_config["y_count"].(int)) * ratio[1]))
	//# There is one case, where we may have to adjust the probe counts:
	//# axis0 < 4 and axis1 > 6 (see _verify_algorithm).
	min_num_of_probes := 3
	if maths.Max(new_x_probe_count, new_y_probe_count) > 6 &&
		maths.Min(new_x_probe_count, new_y_probe_count) < 4 {
		min_num_of_probes = 4
	}
	new_x_probe_count = maths.Max(min_num_of_probes, new_x_probe_count)
	new_y_probe_count = maths.Max(min_num_of_probes, new_y_probe_count)

	gcmd.Respond_info(fmt.Sprintf("Adapted probe count: (%v,%v)",
		new_x_probe_count, new_y_probe_count), true)
	if *self.Radius != 0 {
		adaptedRadius := math.Sqrt(math.Pow(adjusted_mesh_max[0]-adjusted_mesh_min[0], 2)+math.Pow(adjusted_mesh_max[1]-adjusted_mesh_min[1], 2)) / 2
		adaptedOrigin := []float64{
			adjusted_mesh_min[0] + (adjusted_mesh_max[0]-adjusted_mesh_min[0])/2,
			adjusted_mesh_min[1] + (adjusted_mesh_max[1]-adjusted_mesh_min[1])/2,
		}
		if adaptedRadius+math.Sqrt(adaptedOrigin[0]*adaptedOrigin[0]+adaptedOrigin[1]*adaptedOrigin[1]) < *self.Radius {
			self.Radius = &adaptedRadius
			self.Origin = adaptedOrigin
			self.Mesh_min = []float64{-adaptedRadius, -adaptedRadius}
			self.Mesh_max = []float64{adaptedRadius, adaptedRadius}
			new_probe_count := maths.Max(new_x_probe_count, new_y_probe_count)
			new_probe_count += 1 - (new_probe_count % 2)
			self.Mesh_config["x_count"] = new_probe_count
			self.Mesh_config["y_count"] = new_probe_count
		}
	} else {
		if adjusted_mesh_size[0] < self.min_bedmesh_area_size[0] &&
			adjusted_mesh_size[1] < self.min_bedmesh_area_size[1] {

			adjusted_mesh_size = self.min_bedmesh_area_size
			x_center := (adjusted_mesh_min[0] + adjusted_mesh_max[0]) / 2
			y_center := (adjusted_mesh_min[1] + adjusted_mesh_max[1]) / 2

			adjusted_mesh_min = []float64{x_center - self.min_bedmesh_area_size[0]/2, y_center - self.min_bedmesh_area_size[1]/2}
			adjusted_mesh_max = []float64{x_center + self.min_bedmesh_area_size[0]/2, y_center + self.min_bedmesh_area_size[1]/2}

			adjusted_mesh_min[0] = math.Max(adjusted_mesh_min[0], self.Orig_config["mesh_min"].([]float64)[0])
			adjusted_mesh_min[1] = math.Max(adjusted_mesh_min[1], self.Orig_config["mesh_min"].([]float64)[1])
			adjusted_mesh_max[0] = math.Min(adjusted_mesh_max[0], self.Orig_config["mesh_max"].([]float64)[0])
			adjusted_mesh_max[1] = math.Min(adjusted_mesh_max[1], self.Orig_config["mesh_max"].([]float64)[1])
			logger.Debug(fmt.Sprintf("Adaptive mesh bed area too small, readjust area: (%f,%f)", adjusted_mesh_min, adjusted_mesh_max), true)

		}

		self.Mesh_min = adjusted_mesh_min
		self.Mesh_max = adjusted_mesh_max
		self.Mesh_config["x_count"] = new_x_probe_count
		self.Mesh_config["y_count"] = new_y_probe_count
	}

	if bedmeshpkg.Isclose(ratio[0], 1.0, 1e-4, 1e-9) &&
		bedmeshpkg.Isclose(ratio[1], 1.0, 1e-4, 1e-9) {

		self.Profile_name = "default"
	} else {
		self.Profile_name = "adaptive"
	}

	return true
}

func (self *BedMeshCalibrate) Update_config(gcmd *GCodeCommand) {
	// reset default configuration
	if self.Orig_config["radius"] != nil {
		radius := self.Orig_config["radius"].(float64)
		self.Radius = &radius
	}
	if self.Orig_config["origin"] != nil {
		origin := self.Orig_config["origin"].([]float64)
		self.Origin = origin
	}
	// relative_reference_index := self.Orig_config["rri"].(int)
	self.Relative_reference_index = self.Orig_config["rri"].(*int)
	self.Mesh_min = self.Orig_config["mesh_min"].([]float64)
	self.Mesh_max = self.Orig_config["mesh_max"].([]float64)
	for key, _ := range self.Mesh_config {
		self.Mesh_config[key] = self.Orig_config[key]
	}
	params := gcmd.Get_command_parameters()
	need_cfg_update := false

	for key, _ := range params {
		if key == "RELATIVE_REFERENCE_INDEX" {
			if gcmd.Params["RELATIVE_REFERENCE_INDEX"] == "" {
				self.Relative_reference_index = nil
			} else {
				var val, _ = strconv.ParseInt(gcmd.Params["RELATIVE_REFERENCE_INDEX"], 10, 32)
				relative_reference_index := int(val)
				self.Relative_reference_index = &relative_reference_index

			}
			if *self.Relative_reference_index < 0 {
				self.Relative_reference_index = nil
			}
			need_cfg_update = true
			break
		}
	}

	if self.Radius != nil {
		for key, _ := range params {
			if key == "MESH_RADIUS" {
				radius := gcmd.Get_float("MESH_RADIUS", nil, nil, nil, nil, nil)
				self.Radius = &radius
				radius = math.Floor(float64(*self.Radius*10)) / 10
				self.Radius = &radius
				self.Mesh_min = []float64{-*self.Radius, -*self.Radius}
				self.Mesh_max = []float64{*self.Radius, *self.Radius}
				need_cfg_update = true
				break
			}
		}

		for key, _ := range params {
			if key == "MESH_ORIGIN" {
				v1, v2 := Parse_gcmd_coord(gcmd, "MESH_ORIGIN")
				self.Origin = []float64{v1, v2}
				need_cfg_update = true
				break
			}
		}

		for key, _ := range params {
			if key == "ROUND_PROBE_COUNT" {
				minval := 3.
				minval_int := int(minval)
				cnt := gcmd.Get_int("ROUND_PROBE_COUNT", nil, &minval_int, nil)
				self.Mesh_config["x_count"] = cnt
				self.Mesh_config["y_count"] = cnt
				need_cfg_update = true
				break
			}
		}

	} else {
		for key, _ := range params {
			if key == "MESH_MIN" {
				v1, v2 := Parse_gcmd_coord(gcmd, "MESH_MIN")

				self.Mesh_min = []float64{v1, v2}
				need_cfg_update = true
				break
			}
		}

		for key, _ := range params {
			if key == "MESH_MAX" {
				v1, v2 := Parse_gcmd_coord(gcmd, "MESH_MAX")
				self.Mesh_max = []float64{v1, v2}
				need_cfg_update = true
				break
			}
		}

		for key, _ := range params {
			if key == "PROBE_COUNT" {
				minval := 3.
				arr := Parse_gcmd_pair(gcmd, "PROBE_COUNT", &minval, nil)
				self.Mesh_config["x_count"] = arr[0]
				self.Mesh_config["y_count"] = arr[1]
				need_cfg_update = true
				break
			}
		}
	}
	for key, _ := range params {
		if key == "ALGORITHM" {
			self.Mesh_config["algo"] = strings.ToLower(strings.TrimSpace(gcmd.Get("ALGORITHM", nil, "", nil, nil, nil, nil)))
			need_cfg_update = true
			break
		}
	}

	need_mesh_update := self.set_adaptive_mesh(gcmd)
	if need_cfg_update || need_mesh_update {
		self.Verify_algorithm()
		self.Generate_points()
		if self.Relative_reference_index == nil && len(self.Bedmesh.Zero_ref_pos) == 2 {
			idx := -1
			for i, pt := range self.Points {
				if bedmeshpkg.Isclose(pt[0], self.Bedmesh.Zero_ref_pos[0], 1e-04, 1e-06) &&
					bedmeshpkg.Isclose(pt[1], self.Bedmesh.Zero_ref_pos[1], 1e-04, 1e-06) {
					idx = i
					break
				}
			}
			if idx >= 0 {
				idxCopy := idx
				self.Relative_reference_index = &idxCopy
				self.Orig_config["rri"] = self.Relative_reference_index
			} else {
				logger.Debugf("adaptive_bed_mesh: zero reference position (%.3f, %.3f) not found in generated points", self.Bedmesh.Zero_ref_pos[0], self.Bedmesh.Zero_ref_pos[1])
			}
			self.Bedmesh.Zero_ref_pos = nil
		}
		// gcmd.Respond_info("Generating new points...", true)
		// self.Print_generated_points(gcmd.Respond_info)
		pts := self.Get_adjusted_points()
		self.Probe_helper.Update_probe_points(pts, 3)
		var mesh_config_str_arr []string
		for key, item := range self.Mesh_config {
			mesh_config_str_arr = append(mesh_config_str_arr,
				fmt.Sprintf("%s: %v", key, item))
		}
		msg := strings.Join(mesh_config_str_arr, "\n")
		logger.Debugf("Updated Mesh Configuration:" + msg)
	} else {
		self.Points = self.Orig_points
		pts := self.Get_adjusted_points()
		self.Probe_helper.Update_probe_points(pts, 3)
	}
}

func (self *BedMeshCalibrate) Get_adjusted_points() [][]float64 {
	return bedmeshpkg.GetAdjustedPoints(self.Points, self.Substituted_indices)
}

const cmd_BED_MESH_CALIBRATE_help = "Perform Mesh Bed Leveling"

func (self *BedMeshCalibrate) Cmd_BED_MESH_CALIBRATE(gcmd interface{}) error {
	self.Profile_name = gcmd.(*GCodeCommand).Get("PROFILE", "default", "", nil, nil, nil, nil)
	if strings.TrimSpace(self.Profile_name) == "" {
		panic("Value for parameter 'PROFILE' must be specified")
	}
	self.Bedmesh.Set_mesh(nil)
	self.Update_config(gcmd.(*GCodeCommand))
	self.Probe_helper.Start_probe_callback(gcmd.(*GCodeCommand))
	return nil
}

func (self *BedMeshCalibrate) Probe_finalize(offsets []float64, positions [][]float64) string {
	x_offset := offsets[0]
	y_offset := offsets[1]
	z_offset := offsets[2]
	positions_back := [][]float64{}
	for _, p := range positions {
		arr := []float64{
			math.Round(p[0]*100) / 100,
			math.Round(p[1]*100) / 100,
			p[2],
		}
		positions_back = append(positions_back, arr)
	}
	positions = positions_back

	var params = make(map[string]interface{})
	for key, item := range self.Mesh_config {
		params[key] = item
	}
	params["min_x"] = bedmeshpkg.MinPoint(0, positions)[0] + x_offset
	params["max_x"] = bedmeshpkg.MaxPoint(0, positions)[0] + x_offset
	params["min_y"] = bedmeshpkg.MinPoint(1, positions)[1] + y_offset
	params["max_y"] = bedmeshpkg.MaxPoint(1, positions)[1] + y_offset
	x_cnt := params["x_count"]
	y_cnt := params["y_count"]

	if len(self.Substituted_indices) > 0 {
		corrected, err := bedmeshpkg.ProcessFaultySubstitutions(self.Points, self.Substituted_indices, positions, offsets)
		if err != nil {
			self.Dump_points(positions, corrected, offsets)
			panic(err.Error())
		}
		positions = corrected
	}

	probed_matrix, err := bedmeshpkg.AssembleProbedMatrix(positions, z_offset, x_cnt.(int), y_cnt.(int), self.Relative_reference_index, self.Radius)
	if err != nil {
		panic(err.Error())
	}
	z_mesh := bedmeshpkg.NewZMesh(params)
	z_mesh.Build_mesh(probed_matrix)
	self.Bedmesh.Set_mesh(z_mesh)
	logger.Debug("Mesh Bed Leveling Complete")
	self.Bedmesh.Save_profile(self.Profile_name)
	return ""
}

func (self *BedMeshCalibrate) Dump_points(probed_pts [][]float64, corrected_pts [][]float64, offsets []float64) {
	for _, line := range bedmeshpkg.FormatPointDebugLines(self.Points, probed_pts, corrected_pts, offsets) {
		logger.Debugf(line)
	}
}

type ProfileManager struct {
	Name                  string
	Printer               *Printer
	Gcode                 *GCodeDispatch
	Bedmesh               *BedMesh
	Profiles              map[string]interface{}
	Current_profile       string
	Incompatible_profiles []string
}

func NewProfileManager(config *ConfigWrapper, bedmesh *BedMesh) *ProfileManager {
	self := &ProfileManager{}
	self.Name = config.Get_name()
	self.Printer = config.Get_printer()
	gcode_obj := self.Printer.Lookup_object("gcode", object.Sentinel{})
	//if err != nil {
	//	logger.Error(err)
	//}
	self.Gcode = gcode_obj.(*GCodeDispatch)
	self.Bedmesh = bedmesh
	self.Profiles = map[string]interface{}{}
	self.Current_profile = ""
	self.Incompatible_profiles = []string{}
	// Fetch stored profiles from Config
	stored_profs := config.Get_prefix_sections(self.Name)
	stored_profs_back := []*ConfigWrapper{}
	for _, s := range stored_profs {
		if s.Get_name() != self.Name {
			stored_profs_back = append(stored_profs_back, s)
		}
	}
	stored_profs = stored_profs_back
	for _, profile := range stored_profs {
		name := strings.Join(strings.Split(profile.Get_name(), " ")[1:], "")
		version := profile.Getint("version", 0, 0, 0, true)
		if version != PROFILE_VERSION {
			logger.Errorf("bed_mesh: Profile [%s] not compatible with this version\n"+
				"of bed_mesh.  Profile Version: %d Current Version: %d ",
				name, version, PROFILE_VERSION)
			self.Incompatible_profiles = append(self.Incompatible_profiles, name)
			continue
		}
		self.Profiles[name] = map[string]interface{}{}
		zvals := profile.Getlists("points", nil, []string{",", "\n"}, 0, reflect.Float64, true)
		self.Profiles[name].(map[string]interface{})["points"] = zvals
		params := map[string]interface{}{}
		self.Profiles[name].(map[string]interface{})["mesh_params"] = params
		for key, t := range PROFILE_OPTIONS {
			if t == reflect.Int {
				params[key] = profile.Getint(key, object.Sentinel{}, 0, 0, true)
			} else if t == reflect.Float64 {
				params[key] = profile.Getfloat(key, object.Sentinel{}, 0, 0, 0, 0, true)
			} else if t == reflect.String {
				params[key] = profile.Get(key, object.Sentinel{}, true)
			}
		}

	}
	// Register GCode
	self.Gcode.Register_command("BED_MESH_PROFILE", self.Cmd_BED_MESH_PROFILE,
		false,
		cmd_BED_MESH_PROFILE_help)
	return self
}
func (self *ProfileManager) Get_profiles() map[string]interface{} {
	return self.Profiles
}
func (self *ProfileManager) Get_current_profile() string {
	return self.Current_profile
}
func (self *ProfileManager) Check_incompatible_profiles() {
	if len(self.Incompatible_profiles) != 0 {
		configfile_obj := self.Printer.Lookup_object("configfile", object.Sentinel{})
		//if err != nil {
		//	logger.Error(err)
		//}
		configfile := configfile_obj.(*PrinterConfig)
		for _, profile := range self.Incompatible_profiles {
			configfile.Remove_section("bed_mesh " + profile)
		}
		self.Gcode.Respond_info(fmt.Sprintf(
			"The following incompatible profiles have been detected\n"+
				"and are scheduled for removal:\n%s\n"+
				"The SAVE_CONFIG command will update the printer config\n"+
				"file and restart the printer", strings.Join(self.Incompatible_profiles, "\n")), true)

	}
}

func (self *ProfileManager) Save_profile(prof_name string) error {
	z_mesh := self.Bedmesh.Get_mesh()
	if z_mesh == nil {
		self.Gcode.Respond_info(fmt.Sprintf("Unable to save to profile [%s], the bed has not been probed",
			prof_name), true)
		return nil
	}
	probed_matrix := z_mesh.Get_probed_matrix()
	mesh_params := z_mesh.Get_mesh_params()
	configfile_obj := self.Printer.Lookup_object("configfile", object.Sentinel{})
	configfile := configfile_obj.(*PrinterConfig)
	//if err != nil {
	//	logger.Error(err)
	//}
	cfg_name := self.Name + " " + prof_name
	// set params
	z_values := bedmeshpkg.FormatProbedMatrixForConfig(probed_matrix)
	configfile.Set(cfg_name, "version", strconv.Itoa(PROFILE_VERSION))
	configfile.Set(cfg_name, "points", z_values)

	for key, value := range mesh_params {
		if _, ok := value.(string); ok {
			configfile.Set(cfg_name, key, value.(string))
		} else if _, ok := value.(int); ok {
			configfile.Set(cfg_name, key, strconv.Itoa(value.(int)))
		} else if _, ok := value.(float64); ok {
			configfile.Set(cfg_name, key, strconv.FormatFloat(value.(float64), 'f', -1, 64))
		}
	}
	// save copy in local storage
	// ensure any self.profiles returned as status remains immutable
	profiles := make(map[string]interface{})
	for key, val := range self.Profiles {
		profiles[key] = val
	}
	profile := make(map[string]interface{})
	profiles[prof_name] = profile
	profile["points"] = probed_matrix
	mesh_params_copy := make(map[string]interface{})
	for key, val := range mesh_params {
		mesh_params_copy[key] = val
	}
	profile["mesh_params"] = mesh_params_copy
	self.Profiles = profiles
	self.Current_profile = prof_name
	self.Bedmesh.Update_status()
	logger.Debugf("Bed Mesh state has been saved to profile [%s]\n"+
		"for the current session.  The SAVE_CONFIG command will\n"+
		"update the printer config file and restart the printer.",
		prof_name)
	return nil
}
func (self *ProfileManager) Load_profile(prof_name string) error {
	profile := self.Profiles[prof_name]
	if profile == nil {
		logger.Errorf("bed_mesh: Unknown profile [%s]", prof_name)
		return nil
	}
	probed_matrix := bedmeshpkg.NormalizeProbedMatrixType(profile.(map[string]interface{})["points"])
	mesh_params := profile.(map[string]interface{})["mesh_params"].(map[string]interface{})
	z_mesh := bedmeshpkg.NewZMesh(mesh_params)
	if err := self.Build_mesh_catch(z_mesh, probed_matrix); err != nil {
		return fmt.Errorf("%w", err)
	}
	self.Current_profile = prof_name
	self.Bedmesh.Set_mesh(z_mesh)
	return nil
}

func (pm *ProfileManager) Build_mesh_catch(z_mesh *bedmeshpkg.ZMesh, probed_matrix [][]float64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error: %v", r)
		}
	}()
	z_mesh.Build_mesh(probed_matrix)
	return nil
}

func (self *ProfileManager) Remove_profile(prof_name string) error {
	isIn := false
	for key, _ := range self.Profiles {
		if prof_name == key {
			isIn = true
			break
		}
	}
	if isIn {
		configfile_obj := self.Printer.Lookup_object("configfile", object.Sentinel{})
		//if err != nil {
		//	logger.Error(err)
		//}
		configfile := configfile_obj.(*PrinterConfig)
		configfile.Remove_section("bed_mesh " + prof_name)
		profiles := make(map[string]interface{})
		for key, val := range self.Profiles {
			profiles[key] = val
		}
		delete(profiles, prof_name)
		self.Bedmesh.Update_status()
		self.Gcode.Respond_info(fmt.Sprintf(
			"Profile [%s] removed from storage for this session.\n"+
				"The SAVE_CONFIG command will update the printer\n"+
				"configuration and restart the printer", prof_name), true)
	} else {
		self.Gcode.Respond_info(fmt.Sprintf(
			"No profile named [%s] to remove", prof_name), true)

	}
	return nil
}

const cmd_BED_MESH_PROFILE_help = "Bed Mesh Persistent Storage management"

func (self *ProfileManager) Cmd_BED_MESH_PROFILE(arg interface{}) error {
	gcmd := arg.(*GCodeCommand)
	options := map[string]func(string) error{
		"LOAD":   self.Load_profile,
		"SAVE":   self.Save_profile,
		"REMOVE": self.Remove_profile,
	}
	for key, _ := range options {
		name := gcmd.Get(key, nil, "", nil, nil, nil, nil)
		if name != "" {
			if strings.TrimSpace(name) == "" {
				panic(fmt.Sprintf("Value for parameter '%s' must be specified", key))
			}
			if name == "default" && key == "SAVE" {
				gcmd.Respond_info(
					"Profile 'default' is reserved, please choose"+
						" another profile name.", true)
			} else {
				options[key](name)
				return nil
			}
		}
	}
	gcmd.Respond_info(fmt.Sprintf("Invalid syntax '%s'", gcmd.Get_commandline()), true)
	return nil
}
func Load_config_bed_mesh(config *ConfigWrapper) interface{} {
	return NewBedMesh(config)
}
