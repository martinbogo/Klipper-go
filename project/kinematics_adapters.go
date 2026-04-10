package project

import (
	"fmt"

	"goklipper/common/logger"
	"goklipper/common/utils/object"
	kinematicspkg "goklipper/internal/pkg/motion/kinematics"
)

func Load_kinematics(kin_name string) interface{} {
	kinematics := map[string]interface{}{
		"cartesian": Load_kinematics_cartesian,
		"corexy":    Load_kinematics_corexy,
	}

	if _, ok := kinematics[kin_name]; !ok {
		logger.Error(fmt.Errorf("module about %s not support", kin_name))
		return Load_kinematics_none
	}
	return kinematics[kin_name]
}

type kinematicsAdapter struct {
	runtime kinematicspkg.Kinematics
}

func (self *kinematicsAdapter) Get_steppers() []interface{} {
	steppers := self.runtime.GetSteppers()
	result := make([]interface{}, len(steppers))
	for i, stepper := range steppers {
		mcuStepper, ok := stepper.(*MCU_stepper)
		if !ok {
			panic(fmt.Errorf("kinematics stepper has unexpected type %T", stepper))
		}
		result[i] = mcuStepper
	}
	return result
}

func (self *kinematicsAdapter) Calc_position(stepper_positions map[string]float64) []float64 {
	return self.runtime.CalcPosition(stepper_positions)
}

func (self *kinematicsAdapter) Set_position(newpos []float64, homing_axes []int) {
	self.runtime.SetPosition(newpos, homing_axes)
}

func (self *kinematicsAdapter) Note_z_not_homed() {
	self.runtime.NoteZNotHomed()
}

func (self *kinematicsAdapter) Home(homing_state *Homing) {
	self.runtime.Home(&kinematicsHomingAdapter{homing: homing_state})
}

func (self *kinematicsAdapter) Check_move(move *Move) {
	self.runtime.CheckMove(&kinematicsMoveAdapter{move: move})
}

func (self *kinematicsAdapter) Get_status(eventtime float64) map[string]interface{} {
	return self.runtime.Status(eventtime)
}

type CartKinematics struct {
	*kinematicsAdapter
	rails []*PrinterRail
	core  *kinematicspkg.CartesianKinematics
}

func NewCartKinematics(toolhead *Toolhead, config *ConfigWrapper) *CartKinematics {
	rails := []*PrinterRail{}
	for _, axis := range []string{"x", "y", "z"} {
		rails = append(rails, LookupMultiRail(config.Getsection("stepper_"+axis), true, nil, false))
	}
	maxVelocity, maxAccel := toolhead.Get_max_velocity()
	cartesianConfig := kinematicspkg.CartesianConfig{
		Printer:      config.Get_printer(),
		Toolhead:     toolhead,
		Rails:        adaptKinematicsRails(rails),
		MaxZVelocity: config.Getfloat("max_z_velocity", maxVelocity, 0, maxVelocity, 0., 0, true),
		MaxZAccel:    config.Getfloat("max_z_accel", maxAccel, 0, maxAccel, 0., 0, true),
	}
	if config.Has_section("dual_carriage") {
		dcConfig := config.Getsection("dual_carriage")
		axisName := dcConfig.Getchoice("axis", map[interface{}]interface{}{"x": "x", "y": "y"}, object.Sentinel{}, true).(string)
		axisIndex := map[string]int{"x": 0, "y": 1}[axisName]
		dcRail := LookupMultiRail(dcConfig, true, nil, false)
		cartesianConfig.DualCarriage = &kinematicspkg.DualCarriageConfig{
			Axis:     axisIndex,
			AxisName: axisName,
			Rails: []kinematicspkg.Rail{
				cartesianConfig.Rails[axisIndex],
				&kinematicsRailAdapter{rail: dcRail},
			},
		}
	}
	core := kinematicspkg.NewCartesian(cartesianConfig)
	self := &CartKinematics{
		kinematicsAdapter: &kinematicsAdapter{runtime: core},
		rails:             append([]*PrinterRail{}, rails...),
		core:              core,
	}
	if cartesianConfig.DualCarriage != nil {
		gcode := config.Get_printer().Lookup_object("gcode", object.Sentinel{}).(*GCodeDispatch)
		gcode.Register_command("SET_DUAL_CARRIAGE", self.Cmd_SET_DUAL_CARRIAGE, false, self.cmd_SET_DUAL_CARRIAGE_help())
	}
	return self
}

func (self *CartKinematics) Home_axis(homing_state *Homing, axis int, rail *PrinterRail) {
	self.core.HomeAxis(&kinematicsHomingAdapter{homing: homing_state}, axis, &kinematicsRailAdapter{rail: rail})
}

func (self *CartKinematics) Motor_off(argv []interface{}) error {
	return self.core.MotorOff(argv)
}

func (self *CartKinematics) Check_endstops(move *Move) error {
	return self.core.CheckEndstops(&kinematicsMoveAdapter{move: move})
}

func (self *CartKinematics) Activate_carriage(carriage int) {
	self.core.ActivateCarriage(carriage)
}

func (self *CartKinematics) cmd_SET_DUAL_CARRIAGE_help() string {
	return kinematicspkg.SetDualCarriageHelp
}

func (self *CartKinematics) Cmd_SET_DUAL_CARRIAGE(arg interface{}) error {
	gcmd := arg.(*GCodeCommand)
	return kinematicspkg.HandleSetDualCarriageCommand(self.core, gcmd)
}

func Load_kinematics_cartesian(toolhead *Toolhead, config *ConfigWrapper) interface{} {
	return NewCartKinematics(toolhead, config)
}

type CorexyKinematics struct {
	*kinematicsAdapter
	rails []*PrinterRail
	core  *kinematicspkg.CoreXYKinematics
}

func NewCorexyKinematics(toolhead *Toolhead, config *ConfigWrapper) *CorexyKinematics {
	rails := []*PrinterRail{}
	for _, axis := range []string{"x", "y", "z"} {
		rails = append(rails, LookupMultiRail(config.Getsection("stepper_"+axis), true, nil, false))
	}
	maxVelocity, maxAccel := toolhead.Get_max_velocity()
	core := kinematicspkg.NewCoreXY(kinematicspkg.CoreXYConfig{
		Printer:      config.Get_printer(),
		Toolhead:     toolhead,
		Rails:        adaptKinematicsRails(rails),
		MaxZVelocity: config.Getfloat("max_z_velocity", maxVelocity, 0, maxVelocity, 0., 0, true),
		MaxZAccel:    config.Getfloat("max_z_accel", maxAccel, 0, maxAccel, 0., 0, true),
	})
	return &CorexyKinematics{
		kinematicsAdapter: &kinematicsAdapter{runtime: core},
		rails:             append([]*PrinterRail{}, rails...),
		core:              core,
	}
}

func (self *CorexyKinematics) Motor_off(argv []interface{}) error {
	return self.core.MotorOff(argv)
}

func (self *CorexyKinematics) Check_endstops(move *Move) error {
	return self.core.CheckEndstops(&kinematicsMoveAdapter{move: move})
}

func (self *CorexyKinematics) Get_axis_range(axis int) (float64, float64) {
	return self.core.GetAxisRange(axis)
}

func Load_kinematics_corexy(toolhead *Toolhead, config *ConfigWrapper) interface{} {
	return NewCorexyKinematics(toolhead, config)
}

type NoneKinematics struct {
	*kinematicsAdapter
	core        *kinematicspkg.NoneKinematics
	Axes_minmax []string
}

func NewNoneKinematics(toolhead *Toolhead, config *ConfigWrapper) *NoneKinematics {
	_ = config
	axesMinmax := toolhead.Coord
	core := kinematicspkg.NewNone(kinematicspkg.NoneConfig{AxesMinMax: axesMinmax})
	copiedAxes := append([]string{}, axesMinmax...)
	if copiedAxes == nil {
		copiedAxes = []string{"0.", "0.", "0.", "0."}
	}
	return &NoneKinematics{
		kinematicsAdapter: &kinematicsAdapter{runtime: core},
		core:              core,
		Axes_minmax:       copiedAxes,
	}
}

func (self *NoneKinematics) Home_axis(homing_state *Homing, axis int, rail *PrinterRail) {
	_, _, _ = homing_state, axis, rail
}

func (self *NoneKinematics) Motor_off(argv []interface{}) error {
	return self.core.MotorOff(argv)
}

func (self *NoneKinematics) Check_endstops(move *Move) error {
	return self.core.CheckEndstops(&kinematicsMoveAdapter{move: move})
}

func (self *NoneKinematics) Activate_carriage(carriage int) {
	_ = carriage
}

func (self *NoneKinematics) cmd_SET_DUAL_CARRIAGE_help() string {
	return "Set which carriage is active"
}

func (self *NoneKinematics) Cmd_SET_DUAL_CARRIAGE(arg interface{}) error {
	_ = arg
	return nil
}

func Load_kinematics_none(toolhead *Toolhead, config *ConfigWrapper) interface{} {
	return NewNoneKinematics(toolhead, config)
}

type kinematicsRailAdapter struct {
	rail *PrinterRail
}

func adaptKinematicsRails(rails []*PrinterRail) []kinematicspkg.Rail {
	adapted := make([]kinematicspkg.Rail, len(rails))
	for i, rail := range rails {
		adapted[i] = &kinematicsRailAdapter{rail: rail}
	}
	return adapted
}

func (self *kinematicsRailAdapter) Setup_itersolve(alloc_func string, params ...interface{}) {
	self.rail.Setup_itersolve(alloc_func, params...)
}

func (self *kinematicsRailAdapter) Get_steppers() []kinematicspkg.Stepper {
	steppers := self.rail.Get_steppers()
	adapted := make([]kinematicspkg.Stepper, len(steppers))
	for i, stepper := range steppers {
		adapted[i] = stepper
	}
	return adapted
}

func (self *kinematicsRailAdapter) Primary_endstop() kinematicspkg.RailEndstop {
	endstops := self.rail.Get_endstops()
	if len(endstops) == 0 {
		return nil
	}
	front := endstops[0].Front()
	if front == nil {
		return nil
	}
	return &kinematicsRailEndstopAdapter{endstop: front.Value}
}

func (self *kinematicsRailAdapter) Get_range() (float64, float64) {
	return self.rail.Get_range()
}

func (self *kinematicsRailAdapter) Set_position(newpos []float64) {
	self.rail.Set_position(newpos)
}

func (self *kinematicsRailAdapter) Get_homing_info() *kinematicspkg.RailHomingInfo {
	info := self.rail.Get_homing_info()
	return &kinematicspkg.RailHomingInfo{
		Speed:             info.Speed,
		PositionEndstop:   info.Position_endstop,
		RetractSpeed:      info.Retract_speed,
		RetractDist:       info.Retract_dist,
		PositiveDir:       info.Positive_dir,
		SecondHomingSpeed: info.Second_homing_speed,
	}
}

func (self *kinematicsRailAdapter) Set_trapq(tq interface{}) {
	self.rail.Set_trapq(tq)
}

func (self *kinematicsRailAdapter) Get_commanded_position() float64 {
	return self.rail.Get_commanded_position()
}

func (self *kinematicsRailAdapter) Get_name(short bool) string {
	return self.rail.Get_name(short)
}

type kinematicsRailEndstopAdapter struct {
	endstop interface{}
}

func (self *kinematicsRailEndstopAdapter) Add_stepper(stepper kinematicspkg.Stepper) {
	mcuStepper, ok := stepper.(*MCU_stepper)
	if !ok {
		panic(fmt.Errorf("endstop stepper has unexpected type %T", stepper))
	}
	switch typed := self.endstop.(type) {
	case *MCU_endstop:
		typed.Add_stepper(mcuStepper)
	case *ProbeEndstopWrapper:
		typed.Add_stepper(mcuStepper)
	default:
		panic(fmt.Errorf("endstop has unexpected type %T", self.endstop))
	}
}

type kinematicsMoveAdapter struct {
	move *Move
}

func (self *kinematicsMoveAdapter) EndPos() []float64 {
	return self.move.End_pos
}

func (self *kinematicsMoveAdapter) AxesD() []float64 {
	return self.move.Axes_d
}

func (self *kinematicsMoveAdapter) MoveD() float64 {
	return self.move.Move_d
}

func (self *kinematicsMoveAdapter) LimitSpeed(speed float64, accel float64) {
	self.move.Limit_speed(speed, accel)
}

func (self *kinematicsMoveAdapter) MoveError(msg string) error {
	return self.move.Move_error(msg)
}

type kinematicsHomingAdapter struct {
	homing *Homing
}

func (self *kinematicsHomingAdapter) GetAxes() []int {
	return self.homing.Get_axes()
}

func (self *kinematicsHomingAdapter) HomeRails(rails []kinematicspkg.Rail, forcepos []interface{}, homepos []interface{}) {
	projectRails := make([]*PrinterRail, len(rails))
	for i, rail := range rails {
		typed, ok := rail.(*kinematicsRailAdapter)
		if !ok {
			panic(fmt.Errorf("homing rail has unexpected type %T", rail))
		}
		projectRails[i] = typed.rail
	}
	self.homing.Home_rails(projectRails, forcepos, homepos)
}
