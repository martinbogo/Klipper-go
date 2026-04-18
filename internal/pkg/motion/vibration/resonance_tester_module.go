package vibration

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"goklipper/common/logger"
	"goklipper/common/utils/object"
	printerpkg "goklipper/internal/pkg/printer"
)

type accelChip interface {
	Start_internal_client() accelClient
	Get_name() string
}

type resonanceLegacyConfig interface {
	printerpkg.ModuleConfig
	Get(option string, default1 interface{}, noteValid bool) interface{}
	Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int
	Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64
	Getlists(option string, default1 interface{}, seps []string, count int, kind reflect.Kind, noteValid bool) interface{}
	HasOption(option string) bool
}

type resonanceCommand interface {
	printerpkg.Command
	Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	Get_int(name string, _default interface{}, minval *int, maxval *int) int
	Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64
}

type resonanceToolhead interface {
	Get_position() []float64
	Get_status(eventtime float64) map[string]interface{}
	Move(newpos []float64, speed float64)
	Manual_move(coord []interface{}, speed float64)
	Wait_moves()
	Dwell(delay float64)
	M204(accel float64)
}

func requireResonanceConfig(config printerpkg.ModuleConfig) resonanceLegacyConfig {
	legacy, ok := config.(resonanceLegacyConfig)
	if !ok {
		panic(fmt.Sprintf("config does not implement resonanceLegacyConfig: %T", config))
	}
	return legacy
}

func requireResonanceCommand(gcmd printerpkg.Command) resonanceCommand {
	cmd, ok := gcmd.(resonanceCommand)
	if !ok {
		panic(fmt.Sprintf("gcode command does not implement resonanceCommand: %T", gcmd))
	}
	return cmd
}

func requireResonanceToolhead(printer printerpkg.ModulePrinter) resonanceToolhead {
	toolheadObj := printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(resonanceToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement resonanceToolhead: %T", toolheadObj))
	}
	return toolhead
}

func requireConfigStore(printer printerpkg.ModulePrinter) configStore {
	configObj := printer.LookupObject("configfile", nil)
	config, ok := configObj.(configStore)
	if !ok {
		panic(fmt.Sprintf("configfile object does not implement configStore: %T", configObj))
	}
	return config
}

func requireAccelChip(obj interface{}, lookupName string) accelChip {
	chip, ok := obj.(accelChip)
	if !ok {
		panic(fmt.Sprintf("accelerometer object %q does not implement accelChip: %T", lookupName, obj))
	}
	return chip
}

func float64Ptr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func toolheadMinimumCruiseRatio(status map[string]interface{}) float64 {
	if ratio, ok := status["minimum_cruise_ratio"].(float64); ok {
		if ratio < 0.0 {
			return 0.0
		}
		if ratio > 1.0 {
			return 1.0
		}
		return ratio
	}
	maxAccel, maxAccelOK := status["max_accel"].(float64)
	maxAccelToDecel, accelToDecelOK := status["max_accel_to_decel"].(float64)
	if !maxAccelOK || !accelToDecelOK || maxAccel <= 0.0 {
		return 0.0
	}
	ratio := 1.0 - maxAccelToDecel/maxAccel
	if ratio < 0.0 {
		return 0.0
	}
	if ratio > 1.0 {
		return 1.0
	}
	return ratio
}

type VibrationPulseTest struct {
	printer      printerpkg.ModulePrinter
	gcode        printerpkg.GCodeRuntime
	min_freq     float64
	max_freq     float64
	accel_per_hz float64
	hz_per_sec   float64
	probe_points [][]float64
	freq_start   float64
	freq_end     float64
}

func NewVibrationPulseTest(config printerpkg.ModuleConfig) *VibrationPulseTest {
	cfg := requireResonanceConfig(config)
	self := &VibrationPulseTest{}
	self.printer = cfg.Printer()
	self.gcode = self.printer.GCode()
	self.min_freq = cfg.Getfloat("min_freq", 5., 1., 0, 0, 0, true)
	self.max_freq = cfg.Getfloat("max_freq", 10000./75., self.min_freq, 200., 0, 0, true)
	self.accel_per_hz = cfg.Getfloat("accel_per_hz", 75., 0, 0, 0, 0., true)
	self.hz_per_sec = cfg.Getfloat("hz_per_sec", 1., 0.1, 1000., 0, 0, true)
	probePoints := cfg.Getlists("probe_points", nil, []string{",", "\n"}, 3, reflect.Float64, true)
	for _, point := range probePoints.([][]interface{}) {
		converted := make([]float64, 0, len(point))
		for _, value := range point {
			converted = append(converted, value.(float64))
		}
		self.probe_points = append(self.probe_points, converted)
	}
	return self
}

func (self *VibrationPulseTest) Get_start_test_points() [][]float64 {
	return self.probe_points
}

func (self *VibrationPulseTest) Prepare_test(gcmd printerpkg.Command) {
	cmd := requireResonanceCommand(gcmd)
	self.freq_start = cmd.Get_float("FREQ_START", self.min_freq, float64Ptr(1.), nil, nil, nil)
	self.freq_end = cmd.Get_float("FREQ_END", self.max_freq, float64Ptr(self.freq_start), float64Ptr(200.), nil, nil)
	self.hz_per_sec = cmd.Get_float("HZ_PER_SEC", self.hz_per_sec, nil, float64Ptr(1000.), float64Ptr(0.), nil)
}

func (self *VibrationPulseTest) Run_test(axis *TestAxis, gcmd printerpkg.Command) {
	cmd := requireResonanceCommand(gcmd)
	toolhead := requireResonanceToolhead(self.printer)
	pos := toolhead.Get_position()
	X, Y, Z, E := pos[0], pos[1], pos[2], pos[3]
	sign := 1.0
	freq := self.freq_start
	systime := self.printer.Reactor().Monotonic()
	toolheadInfo := toolhead.Get_status(systime)
	oldMaxAccel, _ := toolheadInfo["max_accel"].(float64)
	oldMinimumCruiseRatio := toolheadMinimumCruiseRatio(toolheadInfo)
	maxAccel := self.freq_end * self.accel_per_hz
	self.gcode.RunScriptFromCommand(fmt.Sprintf("SET_VELOCITY_LIMIT ACCEL=%.3f MINIMUM_CRUISE_RATIO=0.000", maxAccel))

	inputShaperObj := self.printer.LookupObject("input_shaper", nil)
	var inputShaper *InputShaper
	if typed, ok := inputShaperObj.(*InputShaper); ok && cmd.Get_int("INPUT_SHAPING", 0, nil, nil) == 0 {
		inputShaper = typed
		inputShaper.Disable_shaping()
		logger.Debug("Disabled [input_shaper] for resonance testing")
	}

	cmd.RespondInfo(fmt.Sprintf("Testing frequency %.0f Hz", freq), true)
	for freq <= self.freq_end+0.000001 {
		tSeg := .25 / freq
		accel := self.accel_per_hz * freq
		maxV := accel * tSeg
		toolhead.M204(accel)
		length := .5 * accel * math.Pow(tSeg, 2)
		dX, dY := axis.GetPoint(length)
		nX := X + sign*dX
		nY := Y + sign*dY
		toolhead.Move([]float64{nX, nY, Z, E}, maxV)
		toolhead.Move([]float64{X, Y, Z, E}, maxV)
		sign = -sign
		oldFreq := freq
		freq += 2. * tSeg * self.hz_per_sec
		if math.Floor(freq) > math.Floor(oldFreq) {
			cmd.RespondInfo(fmt.Sprintf("Testing frequency %.0f Hz", freq), true)
		}
	}

	self.gcode.RunScriptFromCommand(fmt.Sprintf("SET_VELOCITY_LIMIT ACCEL=%.3f MINIMUM_CRUISE_RATIO=%.3f", oldMaxAccel, oldMinimumCruiseRatio))
	if inputShaper != nil {
		inputShaper.Enable_shaping()
		cmd.RespondInfo("Re-enabled [input_shaper]", true)
	}
}

type rawValue struct {
	Chip_axis string
	Aclient   accelClient
	Name      string
}

type ResonanceTester struct {
	printer          printerpkg.ModulePrinter
	gcode            printerpkg.GCodeRuntime
	move_speed       float64
	test             *VibrationPulseTest
	accel_chip_names [][2]string
	max_smoothing    float64
	accel_chips      []struct {
		Chip_axis string
		Chip      accelChip
	}
}

func NewResonanceTester(config printerpkg.ModuleConfig) *ResonanceTester {
	cfg := requireResonanceConfig(config)
	self := &ResonanceTester{}
	self.printer = cfg.Printer()
	self.move_speed = cfg.Getfloat("move_speed", 50., 0, 0, 0., 0, true)
	self.test = NewVibrationPulseTest(config)
	if !cfg.HasOption("accel_chip_x") {
		self.accel_chip_names = [][2]string{{"xy", strings.TrimSpace(cfg.Get("accel_chip", object.Sentinel{}, true).(string))}}
	} else {
		self.accel_chip_names = [][2]string{
			{"x", strings.TrimSpace(cfg.Get("accel_chip_x", object.Sentinel{}, true).(string))},
			{"y", strings.TrimSpace(cfg.Get("accel_chip_y", object.Sentinel{}, true).(string))},
		}
		if self.accel_chip_names[0][1] == self.accel_chip_names[1][1] {
			self.accel_chip_names = [][2]string{{"xy", self.accel_chip_names[0][1]}}
		}
	}
	self.max_smoothing = cfg.Getfloat("max_smoothing", 0., 0.05, 0, 0, 0, true)
	self.gcode = self.printer.GCode()
	self.gcode.RegisterCommand("MEASURE_AXES_NOISE", self.Cmd_MEASURE_AXES_NOISE, false, cmd_MEASURE_AXES_NOISE_help)
	self.gcode.RegisterCommand("TEST_RESONANCES", self.Cmd_TEST_RESONANCES, false, cmd_TEST_RESONANCES_help)
	self.gcode.RegisterCommand("SHAPER_CALIBRATE", self.Cmd_SHAPER_CALIBRATE, false, cmd_SHAPER_CALIBRATE_help)
	self.printer.RegisterEventHandler("project:connect", self.connect)
	return self
}

func (self *ResonanceTester) connect(_ []interface{}) error {
	for _, names := range self.accel_chip_names {
		self.accel_chips = append(self.accel_chips, struct {
			Chip_axis string
			Chip      accelChip
		}{
			Chip_axis: names[0],
			Chip:      requireAccelChip(self.printer.LookupObject(names[1], nil), names[1]),
		})
	}
	return nil
}

func (self *ResonanceTester) _run_test(gcmd printerpkg.Command, axes []*TestAxis, helper *ShaperCalibrate, raw_name_suffix string, accel_chips []accelChip, test_point []float64) map[*TestAxis]*CalibrationData {
	toolhead := requireResonanceToolhead(self.printer)
	calibration_data := make(map[*TestAxis]*CalibrationData, len(axes))
	for _, axis := range axes {
		calibration_data[axis] = nil
	}
	self.test.Prepare_test(gcmd)
	test_points := self.test.Get_start_test_points()
	hasExplicitPoint := len(test_point) > 0
	if hasExplicitPoint {
		test_points = [][]float64{test_point}
	}
	for _, point := range test_points {
		movePoints := make([]interface{}, 0, len(point))
		for _, p := range point {
			movePoints = append(movePoints, p)
		}
		toolhead.Manual_move(movePoints, self.move_speed)
		if len(test_points) > 1 || hasExplicitPoint {
			gcmd.RespondInfo(fmt.Sprintf("Probing point (%.3f, %.3f, %.3f)", point[0], point[1], point[2]), true)
		}
		for _, axis := range axes {
			toolhead.Wait_moves()
			toolhead.Dwell(0.500)
			if len(axes) > 1 {
				gcmd.RespondInfo(fmt.Sprintf("Testing axis %s", axis.GetName()), true)
			}
			rawValues := []rawValue{}
			if accel_chips == nil {
				for _, chip := range self.accel_chips {
					if axis.Matches(chip.Chip_axis) {
						rawValues = append(rawValues, rawValue{Chip_axis: chip.Chip_axis, Aclient: chip.Chip.Start_internal_client(), Name: chip.Chip.Get_name()})
					}
				}
			} else {
				for _, chip := range accel_chips {
					rawValues = append(rawValues, rawValue{Chip_axis: axis.GetName(), Aclient: chip.Start_internal_client(), Name: chip.Get_name()})
				}
			}
			self.test.Run_test(axis, gcmd)
			for _, rv := range rawValues {
				rv.Aclient.Finish_measurements()
				if raw_name_suffix != "" {
					var filenamePoint []float64
					if len(test_points) > 1 || hasExplicitPoint {
						filenamePoint = point
					}
					filenameChipName := ""
					if accel_chips == nil {
						filenameChipName = rv.Name
					}
					rawName := BuildFilename("raw_data", raw_name_suffix, axis, filenamePoint, filenameChipName)
					rv.Aclient.Write_to_file(rawName)
					gcmd.RespondInfo(fmt.Sprintf("Writing raw accelerometer data to %s file", rawName), true)
				}
			}
			if helper == nil {
				continue
			}
			for _, rv := range rawValues {
				if !rv.Aclient.Has_valid_samples() {
					panic(fmt.Errorf("accelerometer '%s' measured no data", rv.Name))
				}
				newData := helper.Process_accelerometer_data(rv.Aclient)
				if calibration_data[axis] == nil {
					calibration_data[axis] = newData
				} else {
					calibration_data[axis].Add_data(newData)
				}
			}
		}
	}
	return calibration_data
}

const cmd_TEST_RESONANCES_help = "Runs the resonance test for a specifed axis"

func (self *ResonanceTester) Cmd_TEST_RESONANCES(gcmd printerpkg.Command) error {
	cmd := requireResonanceCommand(gcmd)
	axis, err := ParseAxis(strings.ToLower(cmd.Get("AXIS", "", nil, nil, nil, nil, nil)))
	if err != nil {
		panic(err)
	}
	if axis == nil {
		panic(errors.New("AXIS parameter is required"))
	}
	accel_chips_arg := cmd.Get("CHIPS", "", nil, nil, nil, nil, nil)
	test_point_arg := cmd.Get("POINT", "", nil, nil, nil, nil, nil)
	var test_point []float64
	if test_point_arg != "" {
		testCoords := strings.Split(test_point_arg, ",")
		if len(testCoords) != 3 {
			panic(errors.New("Invalid POINT parameter, must be 'x,y,z'"))
		}
		test_point = make([]float64, 3)
		for i, raw := range testCoords {
			parsed, parseErr := strconvParseFloat(strings.TrimSpace(raw))
			if parseErr != nil {
				panic(errors.New("Invalid POINT parameter, must be 'x,y,z' where x, y and z are valid floating point numbers"))
			}
			test_point[i] = parsed
		}
	}
	var parsed_chips []accelChip
	if accel_chips_arg != "" {
		for _, chip_name := range strings.Split(accel_chips_arg, ",") {
			lookupName := strings.TrimSpace(chip_name)
			if !strings.Contains(lookupName, "adxl345") {
				lookupName = "adxl345 " + lookupName
			}
			parsed_chips = append(parsed_chips, requireAccelChip(self.printer.LookupObject(lookupName, nil), lookupName))
		}
	}
	outputs := strings.Split(strings.ToLower(cmd.Get("OUTPUT", "resonances", nil, nil, nil, nil, nil)), ",")
	for _, output := range outputs {
		if !containsString([]string{"resonances", "raw_data"}, output) {
			panic(fmt.Errorf("Unsupported output '%+v', only 'resonances' and 'raw_data' are supported", output))
		}
	}
	if len(outputs) == 0 {
		panic(errors.New("No output specified, at least one of 'resonances' or 'raw_data' must be set in OUTPUT parameter"))
	}
	name_suffix := cmd.Get("NAME", time.Now().Format("20060102_150102"), nil, nil, nil, nil, nil)
	if !IsValidNameSuffix(name_suffix) {
		panic(errors.New("Invalid NAME parameter"))
	}
	csv_output := containsString(outputs, "resonances")
	raw_output := containsString(outputs, "raw_data")
	var helper *ShaperCalibrate
	if csv_output {
		helper = NewShaperCalibrate(self.printer)
	}
	raw_name_suffix := ""
	if raw_output {
		raw_name_suffix = name_suffix
	}
	var test_accel_chips []accelChip
	if accel_chips_arg != "" {
		test_accel_chips = parsed_chips
	}
	data := self._run_test(gcmd, []*TestAxis{axis}, helper, raw_name_suffix, test_accel_chips, test_point)[axis]
	if csv_output {
		csv_name := self.save_calibration_data("resonances", name_suffix, helper, axis, data, nil, test_point)
		gcmd.RespondInfo(fmt.Sprintf("Resonances data written to %s file", csv_name), true)
	}
	return nil
}

const cmd_SHAPER_CALIBRATE_help = "Simular to TEST_RESONANCES but suggest input shaper config"

func (self *ResonanceTester) Cmd_SHAPER_CALIBRATE(gcmd printerpkg.Command) error {
	cmd := requireResonanceCommand(gcmd)
	axis := strings.ToLower(cmd.Get("AXIS", "", nil, nil, nil, nil, nil))
	var calibrate_axes []*TestAxis
	if axis == "" {
		calibrate_axes = []*TestAxis{NewTestAxis("x", nil), NewTestAxis("y", nil)}
	} else if axis != "x" && axis != "y" {
		panic(fmt.Errorf("Unsupported axis '%s'", axis))
	} else {
		calibrate_axes = []*TestAxis{NewTestAxis(axis, nil)}
	}
	max_smoothing := cmd.Get_float("MAX_SMOOTHING", self.max_smoothing, float64Ptr(0.05), nil, nil, nil)
	name_suffix := cmd.Get("NAME", time.Now().Format("20060102_150102"), nil, nil, nil, nil, nil)
	if !IsValidNameSuffix(name_suffix) {
		panic(errors.New("Invalid NAME parameter"))
	}
	helper := NewShaperCalibrate(self.printer)
	calibration_data := self._run_test(gcmd, calibrate_axes, helper, "", nil, nil)
	configfile := requireConfigStore(self.printer)
	for _, axis := range calibrate_axes {
		axis_name := axis.GetName()
		gcmd.RespondInfo(fmt.Sprintf("Calculating the best input shaper parameters for %s axis", axis_name), true)
		calibration_data[axis].Normalize_to_frequencies()
		best_shaper, all_shapers := helper.find_best_shaper(calibration_data[axis], max_smoothing)
		gcmd.RespondInfo(fmt.Sprintf("Recommended shaper_type_%s = %s, shaper_freq_%s = %.1f Hz", axis_name, best_shaper.name, axis_name, best_shaper.freq), true)
		if cmd.Get_int("IS_SAVE", 1, nil, nil) == 1 {
			helper.Save_params(configfile, axis_name, best_shaper.name, best_shaper.freq)
		}
		csv_name := self.save_calibration_data("calibration_data", name_suffix, helper, axis, calibration_data[axis], all_shapers, nil)
		gcmd.RespondInfo(fmt.Sprintf("Shaper calibration data written to %s file", csv_name), true)
	}
	gcmd.RespondInfo("The SAVE_CONFIG command will update the printer config file\nwith these parameters and restart the printer.", true)
	return nil
}

const cmd_MEASURE_AXES_NOISE_help = "Measures noise of all enabled accelerometer chips"

func (self *ResonanceTester) Cmd_MEASURE_AXES_NOISE(gcmd printerpkg.Command) error {
	cmd := requireResonanceCommand(gcmd)
	meas_time := cmd.Get_float("MEAS_TIME", 2., nil, nil, nil, nil)
	raw_values := make([]rawValue, 0, len(self.accel_chips))
	for _, chip := range self.accel_chips {
		raw_values = append(raw_values, rawValue{Chip_axis: chip.Chip_axis, Aclient: chip.Chip.Start_internal_client()})
	}
	toolhead := requireResonanceToolhead(self.printer)
	toolhead.Dwell(meas_time)
	for _, rv := range raw_values {
		rv.Aclient.Finish_measurements()
	}
	helper := NewShaperCalibrate(self.printer)
	for _, rv := range raw_values {
		if !rv.Aclient.Has_valid_samples() {
			panic(fmt.Errorf("%+v-axis accelerometer measured no data", rv.Chip_axis))
		}
		data := helper.Process_accelerometer_data(rv.Aclient)
		vx := mean(data.psd_x)
		vy := mean(data.psd_y)
		vz := mean(data.psd_z)
		gcmd.RespondInfo(fmt.Sprintf("Axes noise for %s-axis accelerometer: %.6f (x), %.6f (y), %.6f (z)", rv.Chip_axis, vx, vy, vz), true)
	}
	return nil
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func strconvParseFloat(raw string) (float64, error) {
	return strconv.ParseFloat(raw, 64)
}

func (self *ResonanceTester) save_calibration_data(base_name, name_suffix string, shaper_calibrate *ShaperCalibrate, axis *TestAxis, calibration_data *CalibrationData, all_shapers []CalibrationResult, point []float64) string {
	output := BuildFilename(base_name, name_suffix, axis, point, "")
	if err := shaper_calibrate.Save_calibration_data(output, calibration_data, all_shapers); err != nil {
		panic(err)
	}
	return output
}

func LoadConfigResonanceTester(config printerpkg.ModuleConfig) interface{} {
	return NewResonanceTester(config)
}
