// Z-Probe support
//
// Copyright (C) 2017-2021  Kevin O"Connor <kevin@koconnor.net>
//
// This file may be distributed under the terms of the GNU GPLv3 license.
package project

import (
	"container/list"
	"fmt"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/value"
	gcodepkg "goklipper/internal/pkg/gcode"
	printerpkg "goklipper/internal/pkg/printer"
	probepkg "goklipper/internal/pkg/motion/probe"
	"reflect"
)

const HINT_TIMEOUT = "If the probe did not move far enough to trigger, then consider reducing the Z axis minimum position so the probecan travel further (the Z minimum position can be negative)."

type PrinterProbe struct {
	Printer             *Printer
	Name                string
	Mcu_probe           interface{}
	Speed               float64
	Lift_speed          float64
	X_offset            float64
	Y_offset            float64
	Z_offset            float64
	Probe_calibrate_z   float64
	Multi_probe_pending bool
	Last_state          bool
	Last_z_result       float64
	Gcode_move          *gcodepkg.GCodeMoveModule
	Sample_count        int
	Sample_retract_dist float64
	Samples_result      interface{}
	Samples_tolerance   float64
	Samples_retries     int
	Z_position          float64
	Gcode               *GCodeDispatch
	final_speed         float64
	core                *probepkg.PrinterProbe
}

func NewPrinterProbe(config *ConfigWrapper, mcu_probe interface{}) *PrinterProbe {
	var self = &PrinterProbe{}
	self.Printer = config.Get_printer()
	self.Name = config.Get_name()
	self.Mcu_probe = mcu_probe
	self.Speed = config.Getfloat("speed", 5.0, 0, 0, 0., 0, true)
	self.Lift_speed = config.Getfloat("lift_speed", self.Speed, 0, 0, 0., 0, true)
	self.X_offset = config.Getfloat("x_offset", 0., 0, 0, 0., 0, true)
	self.Y_offset = config.Getfloat("y_offset", 0., 0, 0, 0., 0, true)
	self.Z_offset = config.Getfloat("z_offset", 0, 0, 0, 0., 0, true)
	self.final_speed = config.Getfloat("final_speed", 2., 0, 0, 0., 0, true)
	self.Probe_calibrate_z = 0.
	self.Multi_probe_pending = false
	self.Last_state = false
	self.Last_z_result = 0.
	self.Gcode_move = self.Printer.Load_object(config, "gcode_move", object.Sentinel{}).(*gcodepkg.GCodeMoveModule)
	// Infer Z position to move to during a probe
	if config.Has_section("stepper_z") {
		var zconfig = config.Getsection("stepper_z")
		self.Z_position = zconfig.Getfloat("position_min", 0.,
			0, 0, 0., 0, false)
	} else {
		var pconfig = config.Getsection("printer")
		self.Z_position = pconfig.Getfloat("minimum_z_position", 0.,
			0, 0, 0., 0, false)
	}
	// Multi-sample support (for improved accuracy)
	self.Sample_count = config.Getint("samples", 1, 1, 0, false)
	self.Sample_retract_dist = config.Getfloat("sample_retract_dist", 2.,
		0, 0, 0., 0, false)
	var atypes = map[interface{}]interface{}{"median": "median", "average": "average", "weighted": "weighted"}
	self.Samples_result = config.Getchoice("samples_result", atypes,
		"average", true)
	self.Samples_tolerance = config.Getfloat("samples_tolerance", 0.100,
		0., 0, 0, 0, false)
	self.Samples_retries = config.Getint("samples_tolerance_retries", 0,
		0., 0, false)
	self.core = probepkg.NewPrinterProbe(
		self.Speed,
		self.Lift_speed,
		self.X_offset,
		self.Y_offset,
		self.Z_offset,
		self.final_speed,
		self.Z_position,
		self.Sample_count,
		self.Sample_retract_dist,
		self.Samples_result,
		self.Samples_tolerance,
		self.Samples_retries,
	)
	self.syncCoreState()
	// Register z_virtual_endstop pin
	pins := self.Printer.Lookup_object("pins", object.Sentinel{})
	pins.(*printerpkg.PrinterPins).Register_chip("probe", self)
	// Register homing event handlers
	self.Printer.Register_event_handler("homing:homing_move_begin",
		self.Handle_homing_move_begin)
	self.Printer.Register_event_handler("homing:homing_move_end",
		self.Handle_homing_move_end)
	self.Printer.Register_event_handler("homing:home_rails_begin",
		self.Handle_home_rails_begin)
	self.Printer.Register_event_handler("homing:home_rails_end",
		self.Handle_home_rails_end)
	self.Printer.Register_event_handler("gcode:command_error",
		self.Handle_command_error)

	// Register PROBE/QUERY_PROBE commands
	self.Gcode = MustLookupGcode(self.Printer)
	self.Gcode.Register_command("PROBE",
		self.Cmd_PROBE, false,
		cmd_PROBE_help)
	self.Gcode.Register_command("QUERY_PROBE",
		self.Cmd_QUERY_PROBE, false,
		cmd_QUERY_PROBE_help)
	self.Gcode.Register_command("PROBE_CALIBRATE",
		self.Cmd_PROBE_CALIBRATE, false,
		cmd_PROBE_CALIBRATE_help)
	self.Gcode.Register_command("PROBE_ACCURACY",
		self.Cmd_PROBE_ACCURACY, false,
		cmd_PROBE_ACCURACY_help)

	self.Gcode.Register_command("Z_OFFSET_APPLY_PROBE",
		self.Cmd_Z_OFFSET_APPLY_PROBE, false,
		cmd_Z_OFFSET_APPLY_PROBE_help)

	return self
}

func (self *PrinterProbe) syncCoreState() {
	if self.core == nil {
		return
	}
	self.Speed = self.core.Speed
	self.Lift_speed = self.core.LiftSpeed
	self.X_offset = self.core.XOffset
	self.Y_offset = self.core.YOffset
	self.Z_offset = self.core.ZOffset
	self.final_speed = self.core.FinalSpeed
	self.Probe_calibrate_z = self.core.ProbeCalibrateZ
	self.Multi_probe_pending = self.core.MultiProbePending
	self.Last_state = self.core.LastState
	self.Last_z_result = self.core.LastZResult
	self.Sample_count = self.core.SampleCount
	self.Sample_retract_dist = self.core.SampleRetractDist
	self.Samples_result = self.core.SamplesResult
	self.Samples_tolerance = self.core.SamplesTolerance
	self.Samples_retries = self.core.SamplesRetries
	self.Z_position = self.core.ZPosition
}

type printerProbeAdapter struct {
	probe *PrinterProbe
}

func (self *printerProbeAdapter) Core() *probepkg.PrinterProbe {
	return self.probe.core
}

func (self *printerProbeAdapter) Name() string {
	return self.probe.Name
}

func (self *printerProbeAdapter) SyncCoreState() {
	self.probe.syncCoreState()
}

func (self *printerProbeAdapter) SendEvent(event string) {
	self.probe.Printer.Send_event(event, nil)
}

func (self *printerProbeAdapter) MatchesHomingMoveEndstop(endstop interface{}) bool {
	probeEndstop, ok := endstop.(*ProbeEndstopWrapper)
	return ok && self.probe.Mcu_probe == probeEndstop
}

func (self *printerProbeAdapter) MatchesHomeRailEndstop(endstop interface{}) bool {
	return self.probe.Mcu_probe == endstop
}

func (self *printerProbeAdapter) PrepareProbe(move interface{}) {
	if probeEndstop, ok := self.probe.Mcu_probe.(*ProbeEndstopWrapper); ok {
		probeEndstop.Probe_prepare(move)
	}
}

func (self *printerProbeAdapter) FinishProbe(move interface{}) {
	if probeEndstop, ok := self.probe.Mcu_probe.(*ProbeEndstopWrapper); ok {
		probeEndstop.Probe_finish(move)
	}
}

func (self *printerProbeAdapter) BeginMCUMultiProbe() {
	switch typed := self.probe.Mcu_probe.(type) {
	case *PrinterProbe:
		typed.Multi_probe_begin()
	case *ProbeEndstopWrapper:
		typed.Multi_probe_begin()
	}
}

func (self *printerProbeAdapter) EndMCUMultiProbe() {
	switch typed := self.probe.Mcu_probe.(type) {
	case *PrinterProbe:
		typed.Multi_probe_end()
	case *ProbeEndstopWrapper:
		typed.Multi_probe_end()
	}
}

func (self *printerProbeAdapter) HomedAxes() string {
	toolhead := MustLookupToolhead(self.probe.Printer)
	homedAxes, _ := toolhead.Get_status(self.probe.Printer.Get_reactor().Monotonic())["homed_axes"].(string)
	return homedAxes
}

func (self *printerProbeAdapter) ToolheadPosition() []float64 {
	return MustLookupToolhead(self.probe.Printer).Get_position()
}

func (self *printerProbeAdapter) ProbingMove(target []float64, speed float64) []float64 {
	phoming := self.probe.Printer.Lookup_object("homing", object.Sentinel{}).(*PrinterHoming)
	switch typed := self.probe.Mcu_probe.(type) {
	case *MCU_endstop:
		return phoming.Probing_move(typed, target, speed)
	case *ProbeEndstopWrapper:
		return phoming.Probing_move(typed, target, speed)
	default:
		panic(fmt.Sprintf("probe endstop has unexpected type %T", self.probe.Mcu_probe))
	}
}

func (self *printerProbeAdapter) RespondInfo(msg string, log bool) {
	self.probe.Gcode.Respond_info(msg, log)
}

func (self *printerProbeAdapter) LastMoveTime() float64 {
	return MustLookupToolhead(self.probe.Printer).Get_last_move_time()
}

func (self *printerProbeAdapter) QueryEndstop(printTime float64) int {
	return self.probe.Mcu_probe.(*ProbeEndstopWrapper).Query_endstop(printTime)
}

func (self *printerProbeAdapter) Probe(speed float64) []float64 {
	return probepkg.RunProbeMove(self, speed)
}

func (self *printerProbeAdapter) RunProbeCommand(command probepkg.ProbeCommand) []float64 {
	return probepkg.RunProbeSequence(self, command)
}

func (self *printerProbeAdapter) Move(coord []interface{}, speed float64) {
	self.probe.Move(coord, speed)
}

func (self *printerProbeAdapter) BeginMultiProbe() {
	probepkg.BeginMultiProbe(self)
}

func (self *printerProbeAdapter) EndMultiProbe() {
	probepkg.EndMultiProbe(self)
}

func (self *printerProbeAdapter) EnsureNoManualProbe() {
	Verify_no_manual_probe(self.probe.Printer)
}

func (self *printerProbeAdapter) StartManualProbe(command probepkg.ProbeCommand, finalize func([]float64)) {
	gcmd, ok := command.(*GCodeCommand)
	if !ok {
		panic(fmt.Sprintf("probe command has unexpected type %T", command))
	}
	NewManualProbeHelper(self.probe.Printer, gcmd, finalize)
}

func (self *printerProbeAdapter) SetConfig(section string, option string, value string) {
	configfile := self.probe.Printer.Lookup_object("configfile", object.Sentinel{}).(*PrinterConfig)
	configfile.Set(section, option, value)
}

func (self *printerProbeAdapter) HomingOriginZ() float64 {
	return self.probe.Gcode_move.Get_status(0)["homing_origin"].([]float64)[2]
}

func flattenHomingMoveProbeEndstops(hmove *HomingMove) []interface{} {
	endstops := make([]interface{}, 0)
	for _, e := range hmove.Get_mcu_endstops() {
		es := e.(list.List)
		if es.Front() != nil {
			endstops = append(endstops, es.Front().Value)
		}
	}
	return endstops
}

func flattenHomeRailProbeEndstops(rails []*PrinterRail) []interface{} {
	endstops := make([]interface{}, 0)
	for _, rail := range rails {
		for _, val := range rail.Get_endstops() {
			if val.Front() != nil {
				endstops = append(endstops, val.Front().Value)
			}
		}
	}
	return endstops
}

func (self *PrinterProbe) Handle_homing_move_begin(args []interface{}) error {
	hmove := args[0].(*HomingMove)
	probepkg.HandleHomingMoveBegin(&printerProbeAdapter{probe: self}, hmove, flattenHomingMoveProbeEndstops(hmove))
	return nil
}

func (self *PrinterProbe) Handle_homing_move_end(args []interface{}) error {
	hmove := args[0].(*HomingMove)
	probepkg.HandleHomingMoveEnd(&printerProbeAdapter{probe: self}, hmove, flattenHomingMoveProbeEndstops(hmove))
	return nil
}

func (self *PrinterProbe) Handle_home_rails_begin(args []interface{}) error {
	rails := args[1].([]*PrinterRail)
	probepkg.HandleHomeRailsBegin(&printerProbeAdapter{probe: self}, flattenHomeRailProbeEndstops(rails))
	return nil
}

func (self *PrinterProbe) Handle_home_rails_end(args []interface{}) error {
	rails := args[1].([]*PrinterRail)
	probepkg.HandleHomeRailsEnd(&printerProbeAdapter{probe: self}, flattenHomeRailProbeEndstops(rails))
	return nil
}

func (self *PrinterProbe) Handle_command_error(args []interface{}) error {
	probepkg.HandleCommandError(&printerProbeAdapter{probe: self})
	return nil
}

func (self *PrinterProbe) Multi_probe_begin() {
	probepkg.BeginMultiProbe(&printerProbeAdapter{probe: self})
}

func (self *PrinterProbe) Multi_probe_end() {
	probepkg.EndMultiProbe(&printerProbeAdapter{probe: self})
}

func (self *PrinterProbe) Setup_pin(pin_type string, pin_params map[string]interface{}) interface{} {
	if pin_type != "endstop" || pin_params["pin"] != "z_virtual_endstop" {
		panic("Probe virtual endstop only useful as endstop pin")
	}

	if cast.ToInt(pin_params["invert"]) != 0 || cast.ToInt(pin_params["pullup"]) != 0 {
		panic("Can not pullup/invert probe virtual endstop")
	}
	return self.Mcu_probe
}

func (self *PrinterProbe) Get_lift_speed(gcmd *GCodeCommand) float64 {
	if gcmd != nil {
		zero := 0.0
		return gcmd.Get_float("LIFT_SPEED", self.Lift_speed, nil, nil, &zero, nil)
	}
	return self.core.LiftSpeed
}

func (self *PrinterProbe) Get_offsets() (float64, float64, float64) {
	return self.core.GetOffsets()
}
func (self *PrinterProbe) Probe(speed float64) []float64 {
	return probepkg.RunProbeMove(&printerProbeAdapter{probe: self}, speed)
}

func (self *PrinterProbe) Move(coord interface{}, speed float64) {
	toolhead := MustLookupToolhead(self.Printer)
	if _, ok := coord.([]*float64); ok {
		coord_interface := make([]interface{}, len(coord.([]*float64)))
		for i, item := range coord.([]*float64) {
			if item != nil {
				coord_interface[i] = *item
			} else {
				coord_interface[i] = nil
			}
		}
		toolhead.Manual_move(coord_interface, speed)
	} else {
		toolhead.Manual_move(coord.([]interface{}), speed)
	}

}

func (self *PrinterProbe) Calc_mean(positions [][]float64) []float64 {
	return probepkg.MeanPosition(positions)
}

func (self *PrinterProbe) Calc_median(positions [][]float64) []float64 {
	return probepkg.MedianPosition(positions)
}

func (self *PrinterProbe) Run_probe(gcmd *GCodeCommand) []float64 {
	return probepkg.RunProbeSequence(&printerProbeAdapter{probe: self}, gcmd)
}

const cmd_PROBE_help = "Probe Z-height at current XY position"

func (self *PrinterProbe) Cmd_PROBE(arg interface{}) error {
	err := probepkg.HandleProbeCommand(&printerProbeAdapter{probe: self}, arg.(*GCodeCommand))
	self.syncCoreState()
	return err
}

const cmd_QUERY_PROBE_help = "Return the status of the z-probe"

func (self *PrinterProbe) Cmd_QUERY_PROBE(arg interface{}) error {
	err := probepkg.HandleQueryProbeCommand(&printerProbeAdapter{probe: self}, arg.(*GCodeCommand))
	self.syncCoreState()
	return err
}

func (self *PrinterProbe) get_status(eventtime float64) map[string]interface{} {
	return self.core.Status()
}

const cmd_PROBE_ACCURACY_help = "Probe Z-height accuracy at current XY position"

func (self *PrinterProbe) Cmd_PROBE_ACCURACY(arg interface{}) error {
	return probepkg.HandleProbeAccuracyCommand(&printerProbeAdapter{probe: self}, arg.(*GCodeCommand))
}

const cmd_PROBE_CALIBRATE_help = "Calibrate the probe's z_offset"

func (self *PrinterProbe) Cmd_PROBE_CALIBRATE(gcmd interface{}) error {
	err := probepkg.HandleProbeCalibrateCommand(&printerProbeAdapter{probe: self}, gcmd.(*GCodeCommand))
	self.syncCoreState()
	return err
}

func (self *PrinterProbe) Cmd_Z_OFFSET_APPLY_PROBE(argv interface{}) error {
	return probepkg.HandleZOffsetApplyProbeCommand(&printerProbeAdapter{probe: self})
}

const cmd_Z_OFFSET_APPLY_PROBE_help = "Adjust the probe's z_offset"

// Endstop wrapper that enables probe specific features
type ProbeEndstopWrapper struct {
	Printer             *Printer
	Position_endstop    float64
	Stow_on_each_sample bool
	Activate_gcode      interface{}
	Deactivate_gcode    interface{}
	Mcu_endstop         *MCU_endstop
	Get_mcu             interface{} //func
	Add_stepper         func(interface{})
	Get_steppers        func() []interface{}
	Home_start          interface{} //func
	Home_wait           interface{} //func
	Query_endstop       func(float64) int
	Multi               string
	core                *probepkg.EndstopWrapper
}

func NewProbeEndstopWrapper(config *ConfigWrapper) *ProbeEndstopWrapper {
	self := &ProbeEndstopWrapper{}
	self.Printer = config.Get_printer()
	self.Position_endstop = config.Getfloat("z_offset", 0, 0, 0, 0, 0, true)
	self.Stow_on_each_sample = config.Getboolean(
		"deactivate_on_each_sample", nil, true)
	self.core = probepkg.NewEndstopWrapper(self.Position_endstop, self.Stow_on_each_sample)
	gcode_macro := self.Printer.Load_object(config, "gcode_macro_1", object.Sentinel{}).(*PrinterGCodeMacro)
	self.Activate_gcode = gcode_macro.Load_template(
		config, "activate_gcode", "")
	self.Deactivate_gcode = gcode_macro.Load_template(
		config, "deactivate_gcode", "")
	//Create an "endstop" object to handle the probe pin
	var ppins = self.Printer.Lookup_object("pins", object.Sentinel{})
	var pin = config.Get("pin", object.Sentinel{}, true)
	var pin_params = ppins.(*printerpkg.PrinterPins).Lookup_pin(pin.(string), true, true, nil)
	var mcu = pin_params["chip"]
	self.Mcu_endstop = mcu.(*MCU).Setup_pin("endstop", pin_params).(*MCU_endstop)
	self.Printer.Register_event_handler("project:mcu_identify",
		self.Handle_mcu_identify)
	// Wrappers
	self.Get_mcu = self.Mcu_endstop.Get_mcu
	self.Add_stepper = self.Mcu_endstop.Add_stepper
	self.Get_steppers = self.Mcu_endstop.Get_steppers
	self.Home_start = self.Mcu_endstop.Home_start
	self.Home_wait = self.Mcu_endstop.Home_wait
	self.Query_endstop = self.Mcu_endstop.Query_endstop
	// multi probes state
	self.Multi = self.core.Multi
	return self
}

func (self *ProbeEndstopWrapper) syncCoreState() {
	if self.core == nil {
		return
	}
	self.Position_endstop = self.core.PositionEndstop
	self.Stow_on_each_sample = self.core.StowOnEachSample
	self.Multi = self.core.Multi
}

func (self *ProbeEndstopWrapper) Handle_mcu_identify([]interface{}) error {
	var toolhead = self.Printer.Lookup_object("toolhead", object.Sentinel{})
	var kin = toolhead.(*Toolhead).Get_kinematics().(IKinematics)
	for _, stepper := range kin.Get_steppers() {
		if stepper.(*MCU_stepper).Is_active_axis('z') != 0 {
			self.Add_stepper(stepper)
		}
	}
	return nil
}

func (self *ProbeEndstopWrapper) Raise_probe() {
	var toolhead = self.Printer.Lookup_object("toolhead", object.Sentinel{})
	var start_pos = toolhead.(*Toolhead).Get_position()
	self.Deactivate_gcode.(*TemplateWrapper).Run_gcode_from_command(nil)
	if probepkg.CoordinatesChanged(start_pos, toolhead.(*Toolhead).Get_position()) {
		panic("project.Toolhead moved during probe activate_gcode script")
	}
}

func (self *ProbeEndstopWrapper) Lower_probe() {
	var toolhead = self.Printer.Lookup_object("toolhead", object.Sentinel{})

	var start_pos = toolhead.(*Toolhead).Get_position()
	self.Activate_gcode.(*TemplateWrapper).Run_gcode_from_command(nil)
	if probepkg.CoordinatesChanged(start_pos, toolhead.(*Toolhead).Get_position()) {
		panic("project.Toolhead moved during probe deactivate_gcode script")
	}
}

func (self *ProbeEndstopWrapper) Multi_probe_begin() {
	self.core.BeginMultiProbe()
	self.syncCoreState()
}

func (self *ProbeEndstopWrapper) Multi_probe_end() {
	if self.core.EndMultiProbe() {
		self.syncCoreState()
		self.Raise_probe()
		return
	}
	self.syncCoreState()
}

func (self *ProbeEndstopWrapper) Probe_prepare(hmove interface{}) {
	if self.core.PrepareForProbe() {
		self.syncCoreState()
		self.Lower_probe()
	}
}

func (self *ProbeEndstopWrapper) Probe_finish(hmove interface{}) {
	if self.core.FinishProbe() {
		self.Raise_probe()
	}
}

func (self *ProbeEndstopWrapper) Get_position_endstop() float64 {
	return self.core.GetPositionEndstop()
}

// Helper code that can probe a series of points and report the
// position at each point.
type ProbePointsHelper struct {
	printer              *Printer
	finalize_callback    func([]float64, [][]float64) string
	Start_probe_callback func(*GCodeCommand)
	probe_points         [][]float64
	name                 string
	gcode                *GCodeDispatch
	horizontal_move_z    float64
	speed                float64
	use_offsets          bool
	lift_speed           float64
	probe_offsets        []float64
	results              [][]float64
	posArr               []float64
	core                 *probepkg.ProbePointsHelper
}

type probePointsAutomaticProbeAdapter struct {
	probe *PrinterProbe
}

func (self *probePointsAutomaticProbeAdapter) GetLiftSpeed(command probepkg.ProbeCommand) float64 {
	gcmd, ok := command.(*GCodeCommand)
	if !ok {
		panic(fmt.Sprintf("probe command has unexpected type %T", command))
	}
	return self.probe.Get_lift_speed(gcmd)
}

func (self *probePointsAutomaticProbeAdapter) GetOffsets() []float64 {
	x, y, z := self.probe.Get_offsets()
	return []float64{x, y, z}
}

func (self *probePointsAutomaticProbeAdapter) BeginMultiProbe() {
	self.probe.Multi_probe_begin()
}

func (self *probePointsAutomaticProbeAdapter) EndMultiProbe() {
	self.probe.Multi_probe_end()
}

func (self *probePointsAutomaticProbeAdapter) RunProbe(command probepkg.ProbeCommand) []float64 {
	gcmd, ok := command.(*GCodeCommand)
	if !ok {
		panic(fmt.Sprintf("probe command has unexpected type %T", command))
	}
	return self.probe.Run_probe(gcmd)
}

type probePointsRuntimeAdapter struct {
	helper *ProbePointsHelper
}

func (self *probePointsRuntimeAdapter) EnsureNoManualProbe() {
	Verify_no_manual_probe(self.helper.printer)
}

func (self *probePointsRuntimeAdapter) LookupAutomaticProbe() probepkg.ProbePointsAutomaticProbe {
	probeObj := self.helper.printer.Lookup_object("probe", object.Sentinel{})
	probe, ok := probeObj.(*PrinterProbe)
	if !ok {
		return nil
	}
	return &probePointsAutomaticProbeAdapter{probe: probe}
}

func (self *probePointsRuntimeAdapter) Move(coord []interface{}, speed float64) {
	MustLookupToolhead(self.helper.printer).Manual_move(coord, speed)
}

func (self *probePointsRuntimeAdapter) TouchLastMoveTime() {
	MustLookupToolhead(self.helper.printer).Get_last_move_time()
}

func (self *probePointsRuntimeAdapter) StartManualProbe(finalize func([]float64)) {
	gcmd := self.helper.gcode.Create_gcode_command("", "", nil)
	NewManualProbeHelper(self.helper.printer, gcmd, finalize)
}

func NewProbePointsHelper(config *ConfigWrapper, finalize_callback interface{}, default_points [][]float64) *ProbePointsHelper {
	self := &ProbePointsHelper{}
	self.printer = config.Get_printer()
	self.finalize_callback = finalize_callback.(func([]float64, [][]float64) string)
	self.Start_probe_callback = self.Start_probe
	self.probe_points = default_points
	self.name = config.Get_name()
	self.gcode = MustLookupGcode(self.printer)
	// Read config settings
	if len(default_points) == 0 || config.Get("points", value.None, true) != nil {
		self.probe_points = config.Getlists("points", nil, []string{","},
			2, reflect.Float64, true).([][]float64)
	}
	self.horizontal_move_z = config.Getfloat("horizontal_move_z", 5., 0, 0, 0, 0, true)
	self.speed = config.Getfloat("speed", 50., 0, 0, 0., 0, true)
	self.use_offsets = false
	self.core = probepkg.NewProbePointsHelper(self.name, self.finalize_callback, self.probe_points, self.horizontal_move_z, self.speed)
	// Internal probing state
	self.syncCoreState()
	return self
}

func (self *ProbePointsHelper) syncCoreState() {
	if self.core == nil {
		return
	}
	self.name = self.core.Name()
	self.horizontal_move_z = self.core.HorizontalMoveZ()
	self.speed = self.core.Speed()
	self.lift_speed = self.core.LiftSpeed()
	self.probe_offsets = self.core.ProbeOffsets()
	self.results = make([][]float64, self.core.ResultCount())
}

func (self *ProbePointsHelper) Minimum_points(n int) {
	if !self.core.MinimumPoints(n) {
		panic(fmt.Sprintf("Need at least %d probe points for %s", n, self.name))
	}
}

func (self *ProbePointsHelper) Update_probe_points(points [][]float64, min_points int) {
	self.core.UpdateProbePoints(points)
	self.probe_points = points
	self.syncCoreState()
	self.Minimum_points(min_points)
}

func (self *ProbePointsHelper) Use_xy_offsets(use_offsets bool) {
	self.core.UseXYOffsets(use_offsets)
	self.use_offsets = use_offsets
}

func (self *ProbePointsHelper) Get_lift_speed() float64 {
	return self.core.LiftSpeed()
}

func (self *ProbePointsHelper) Move_next() bool {
	done := self.core.MoveNext(&probePointsRuntimeAdapter{helper: self})
	self.syncCoreState()
	return done
}

func (self *ProbePointsHelper) Start_probe(gcmd *GCodeCommand) {
	self.core.StartProbe(&probePointsRuntimeAdapter{helper: self}, gcmd)
	self.syncCoreState()
}

func Load_config_probe(config *ConfigWrapper) interface{} {
	return NewPrinterProbe(config, NewProbeEndstopWrapper(config))
}
