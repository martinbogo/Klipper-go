package project

import (
	"goklipper/common/utils/object"
	"goklipper/common/utils/sys"
	gcodepkg "goklipper/internal/pkg/gcode"
	probepkg "goklipper/internal/pkg/motion/probe"
	"reflect"
)

type ManualProbe struct {
	printer            *Printer
	gcode              *GCodeDispatch
	gcode_move         *gcodepkg.GCodeMoveModule
	z_position_endstop float64
	status             map[string]interface{}
	a_position_endstop float64
	b_position_endstop float64
	c_position_endstop float64
}

type manualProbeModuleContext struct {
	probe *ManualProbe
}

func unwrapManualProbeCommand(command probepkg.ProbeCommand) *GCodeCommand {
	if typed, ok := command.(*GCodeCommand); ok {
		return typed
	}
	panic("unsupported manual probe command adapter")
}

func (self *manualProbeModuleContext) StartManualProbe(command probepkg.ProbeCommand, finalize func([]float64)) {
	NewManualProbeHelper(self.probe.printer, unwrapManualProbeCommand(command), finalize)
}

func (self *manualProbeModuleContext) ZPositionEndstop() float64 {
	return self.probe.z_position_endstop
}

func (self *manualProbeModuleContext) DeltaPositionEndstops() (float64, float64, float64) {
	return self.probe.a_position_endstop, self.probe.b_position_endstop, self.probe.c_position_endstop
}

func (self *manualProbeModuleContext) HomingOriginZ() float64 {
	return self.probe.gcode_move.Get_status(0)["homing_origin"].([]float64)[2]
}

func (self *manualProbeModuleContext) SetConfig(section string, option string, value string) {
	configfile := self.probe.printer.Lookup_object("configfile", object.Sentinel{}).(*PrinterConfig)
	configfile.Set(section, option, value)
}

func (self *manualProbeModuleContext) RespondInfo(msg string, log bool) {
	self.probe.gcode.Respond_info(msg, log)
}

type manualProbeCommandAdapter struct {
	command *GCodeCommand
}

func (self *manualProbeCommandAdapter) Parameters() map[string]string {
	return self.command.Get_command_parameters()
}

func (self *manualProbeCommandAdapter) RespondInfo(msg string, log bool) {
	self.command.Respond_info(msg, log)
}

type manualProbeGCodeAdapter struct {
	gcode *GCodeDispatch
}

func (self *manualProbeGCodeAdapter) RegisterCommand(cmd string, handler func(probepkg.ManualProbeCommand) error, desc string) {
	self.gcode.Register_command(cmd, func(arg interface{}) error {
		return handler(&manualProbeCommandAdapter{command: arg.(*GCodeCommand)})
	}, false, desc)
}

func (self *manualProbeGCodeAdapter) ClearCommand(cmd string) {
	self.gcode.Register_command(cmd, nil, false, "")
}

func (self *manualProbeGCodeAdapter) RespondInfo(msg string, log bool) {
	self.gcode.Respond_info(msg, log)
}

type manualProbeRuntimeAdapter struct {
	toolhead          *Toolhead
	manualProbe       *ManualProbe
	lastToolheadPos   []float64
	lastKinematicsPos []float64
}

func (self *manualProbeRuntimeAdapter) ResetStatus() {
	self.manualProbe.Reset_status()
}

func (self *manualProbeRuntimeAdapter) SetStatus(status map[string]interface{}) {
	self.manualProbe.status = status
}

func (self *manualProbeRuntimeAdapter) ToolheadPosition() []float64 {
	return self.toolhead.Get_position()
}

func (self *manualProbeRuntimeAdapter) KinematicsPosition() []float64 {
	toolheadPos := self.toolhead.Get_position()
	if reflect.DeepEqual(toolheadPos, self.lastToolheadPos) {
		return append([]float64{}, self.lastKinematicsPos...)
	}
	self.toolhead.Flush_step_generation()
	kin := self.toolhead.Get_kinematics().(IKinematics)
	kinPos := map[string]float64{}
	for _, stepper := range kin.Get_steppers() {
		kinPos[stepper.(*MCU_stepper).Get_name(false)] = stepper.(*MCU_stepper).Get_commanded_position()
	}
	position := kin.Calc_position(kinPos)
	self.lastToolheadPos = append([]float64{}, toolheadPos...)
	self.lastKinematicsPos = append([]float64{}, position...)
	return append([]float64{}, position...)
}

func (self *manualProbeRuntimeAdapter) ManualMove(coord []interface{}, speed float64) {
	self.toolhead.Manual_move(coord, speed)
}

func NewManualProbeHelper(printer *Printer, gcmd *GCodeCommand, finalize_callback func([]float64)) {
	Verify_no_manual_probe(printer)
	gcode := MustLookupGcode(printer)
	toolhead := MustLookupToolhead(printer)
	manualProbe := printer.Lookup_object("manual_probe", object.Sentinel{}).(*ManualProbe)
	speed := gcmd.Get_float("SPEED", 5., nil, nil, nil, nil)
	probepkg.NewManualProbeSession(
		&manualProbeGCodeAdapter{gcode: gcode},
		&manualProbeRuntimeAdapter{toolhead: toolhead, manualProbe: manualProbe},
		speed,
		finalize_callback,
	)
}

func NewManualProbe(config *ConfigWrapper) *ManualProbe {
	self := ManualProbe{}
	self.printer = config.Get_printer()
	// register commands
	self.gcode = MustLookupGcode(self.printer)
	self.gcode_move = MustLookupGCodeMove(self.printer)
	self.gcode.Register_command("MANUAL_PROBE", self.cmd_MANUAL_PROBE,
		false, self.cmd_MANUAL_PROBE_help())
	//# Endstop values for linear delta printers with vertical A,B,C towers
	a_tower_config := config.Getsection("stepper_a")
	self.a_position_endstop = a_tower_config.Getfloat("position_endstop", object.Sentinel{}, 0., 0., 0., 0., false)

	b_tower_config := config.Getsection("stepper_b")
	self.b_position_endstop = b_tower_config.Getfloat("position_endstop", object.Sentinel{}, 0., 0., 0., 0., false)

	c_tower_config := config.Getsection("stepper_c")
	self.c_position_endstop = c_tower_config.Getfloat("position_endstop", object.Sentinel{}, 0., 0., 0., 0., false)

	//# Conditionally register appropriate commands depending on printer
	//# Cartestian printers with separate Z Axis
	zconfig := config.Getsection("stepper_z")
	self.z_position_endstop = zconfig.Getfloat("Position_endstop", object.Sentinel{}, 0, 0, 0, 0, false)
	if self.z_position_endstop != 0 {
		self.gcode.Register_command("Z_ENDSTOP_CALIBRATE",
			self.cmd_Z_ENDSTOP_CALIBRATE,
			false, self.cmd_Z_ENDSTOP_CALIBRATE_help())
		self.gcode.Register_command("Z_OFFSET_APPLY_ENDSTOP",
			self.cmd_Z_OFFSET_APPLY_ENDSTOP,
			false, self.cmd_Z_OFFSET_APPLY_ENDSTOP_help())
	}
	//# Linear delta printers with A,B,C towers
	if "delta" == config.Getsection("printer").Get("kinematics", object.Sentinel{}, true).(string) {
		self.gcode.Register_command("Z_OFFSET_APPLY_ENDSTOP",
			self.cmd_Z_OFFSET_APPLY_DELTA_ENDSTOPS,
			false, self.cmd_Z_OFFSET_APPLY_ENDSTOP_help())
	}
	self.Reset_status()

	return &self
}

func (self *ManualProbe) Reset_status() {
	self.status = map[string]interface{}{
		"is_active":        false,
		"z_position":       nil,
		"z_position_lower": nil,
		"z_position_upper": nil,
	}
}

func (self *ManualProbe) Get_status(eventTime int64) map[string]interface{} {
	return sys.DeepCopyMap(self.status)
}

func (self *ManualProbe) cmd_MANUAL_PROBE_help() string {
	return "Start manual probe helper script"
}

func (self *ManualProbe) cmd_MANUAL_PROBE(argv interface{}) error {
	err := probepkg.HandleManualProbeCommand(&manualProbeModuleContext{probe: self}, argv.(*GCodeCommand))
	return err
}

func (self *ManualProbe) cmd_Z_ENDSTOP_CALIBRATE_help() string {
	return "Calibrate a Z endstop"
}

func (self *ManualProbe) cmd_Z_ENDSTOP_CALIBRATE(argv interface{}) error {
	err := probepkg.HandleZEndstopCalibrateCommand(&manualProbeModuleContext{probe: self}, argv.(*GCodeCommand))
	return err
}

func (self *ManualProbe) cmd_Z_OFFSET_APPLY_ENDSTOP(argv interface{}) error {
	err := probepkg.HandleZOffsetApplyEndstopCommand(&manualProbeModuleContext{probe: self})
	return err
}

func (self *ManualProbe) cmd_Z_OFFSET_APPLY_DELTA_ENDSTOPS(argv interface{}) error {
	err := probepkg.HandleZOffsetApplyDeltaEndstopsCommand(&manualProbeModuleContext{probe: self})
	return err
}

func (self *ManualProbe) cmd_Z_OFFSET_APPLY_ENDSTOP_help() string {
	return "Adjust the z endstop_position"
}

// Verify that a manual probe isn't already in progress
func Verify_no_manual_probe(printer *Printer) {
	gcode := printer.Lookup_object("gcode", object.Sentinel{})
	err := gcode.(*GCodeDispatch).Register_command("ACCEPT", "dummy", false, "")
	if err != nil {
		panic("Already in a manual Z probe. Use ABORT to abort it.")
	}
	gcode.(*GCodeDispatch).Register_command("ACCEPT", nil, false, "")
}

func Load_config_ManualProbe(config *ConfigWrapper) interface{} {
	return NewManualProbe(config)
}
