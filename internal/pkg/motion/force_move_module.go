package motion

import (
	"fmt"

	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

const (
	BuzzDistance = 1.
	BuzzVelocity = BuzzDistance / .250
	StallTime    = 0.100
)

var BuzzRadiansDistance = 0.017453292519943295
var BuzzRadiansVelocity = BuzzRadiansDistance / .250

type ForceMoveStepperDriver interface {
	ForceMoveStepper
	Get_name(short bool) string
	Units_in_radians() bool
	Setup_default_pulse_duration(pulseduration interface{}, step_both_edge bool)
	Get_pulse_duration() (interface{}, bool)
	Mcu_to_commanded_position(mcuPos int) float64
	Get_dir_inverted() (uint32, uint32)
	Get_mcu_position() int
}

type forceMoveToolhead interface {
	ForceMoveToolhead
	Get_position() []float64
	Set_position(newpos []float64, homingAxes []int)
}

type ForceMoveModule struct {
	printer  printerpkg.ModulePrinter
	steppers map[string]ForceMoveStepperDriver
	core     *ForceMover
}

const (
	cmdStepperBuzzHelp          = "Oscillate a given stepper to help id it"
	cmdForceMoveHelp            = "Manually move a stepper; invalidates kinematics"
	cmdSetKinematicPositionHelp = "Force a low-level kinematic position"
)

func LoadConfigForceMove(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	self := &ForceMoveModule{
		printer:  printer,
		steppers: make(map[string]ForceMoveStepperDriver),
		core:     NewForceMover(),
	}
	gcode := printer.GCode()
	gcode.RegisterCommand("STEPPER_BUZZ", self.cmdStepperBuzz, false, cmdStepperBuzzHelp)
	if config.Bool("enable_force_move", true) {
		gcode.RegisterCommand("FORCE_MOVE", self.cmdForceMove, false, cmdForceMoveHelp)
		gcode.RegisterCommand("SET_KINEMATIC_POSITION", self.cmdSetKinematicPosition, false, cmdSetKinematicPositionHelp)
	}
	return self
}

func (self *ForceMoveModule) RegisterStepper(stepper ForceMoveStepperDriver) {
	self.steppers[stepper.Get_name(false)] = stepper
}

func (self *ForceMoveModule) Register_stepper(stepper ForceMoveStepperDriver) {
	self.RegisterStepper(stepper)
}

func (self *ForceMoveModule) LookupStepper(name string) ForceMoveStepperDriver {
	stepper, ok := self.steppers[name]
	if !ok {
		panic(fmt.Sprintf("Unknown stepper %s\n", name))
	}
	return stepper
}

func (self *ForceMoveModule) Lookup_stepper(name string) ForceMoveStepperDriver {
	return self.LookupStepper(name)
}

func (self *ForceMoveModule) lookupToolhead() forceMoveToolhead {
	return self.printer.LookupObject("toolhead", nil).(forceMoveToolhead)
}

func (self *ForceMoveModule) forceEnable(stepper ForceMoveStepperDriver) bool {
	toolhead := self.lookupToolhead()
	printTime := toolhead.Get_last_move_time()
	enable, err := self.printer.StepperEnable().LookupEnable(stepper.Get_name(false))
	if err != nil {
		panic(err)
	}
	wasEnable := enable.IsMotorEnabled()
	if !wasEnable {
		enable.MotorEnable(printTime)
		toolhead.Dwell(StallTime)
	}
	return wasEnable
}

func (self *ForceMoveModule) restoreEnable(stepper ForceMoveStepperDriver, wasEnable bool) {
	if wasEnable {
		return
	}
	toolhead := self.lookupToolhead()
	toolhead.Dwell(StallTime)
	printTime := toolhead.Get_last_move_time()
	enable, err := self.printer.StepperEnable().LookupEnable(stepper.Get_name(false))
	if err != nil {
		panic(err)
	}
	enable.MotorDisable(printTime)
	toolhead.Dwell(StallTime)
}

func (self *ForceMoveModule) ManualMove(stepper ForceMoveStepperDriver, dist, speed float64, accel *float64) {
	self.core.ManualMove(self.lookupToolhead(), stepper, dist, speed, accel)
}

func (self *ForceMoveModule) lookupStepperCommand(gcmd printerpkg.Command) ForceMoveStepperDriver {
	name := gcmd.String("STEPPER", "")
	if name == "manual_stepper" {
		name = "manual_stepper select_stepper"
	}
	return self.LookupStepper(name)
}

func (self *ForceMoveModule) cmdStepperBuzz(gcmd printerpkg.Command) error {
	stepper := self.lookupStepperCommand(gcmd)
	logger.Infof("Stepper buzz %s", stepper.Get_name(false))
	wasEnable := self.forceEnable(stepper)
	toolhead := self.lookupToolhead()
	dist, speed := BuzzDistance, BuzzVelocity
	if stepper.Units_in_radians() {
		dist, speed = BuzzRadiansDistance, BuzzRadiansVelocity
	}
	for i := 0; i < 10; i++ {
		self.ManualMove(stepper, dist, speed, nil)
		toolhead.Dwell(.050)
		self.ManualMove(stepper, -dist, speed, nil)
		toolhead.Dwell(.450)
	}
	self.restoreEnable(stepper, wasEnable)
	return nil
}

func (self *ForceMoveModule) cmdForceMove(gcmd printerpkg.Command) error {
	stepper := self.lookupStepperCommand(gcmd)
	distance := gcmd.Float("DISTANCE", 0.)
	speed := gcmd.Float("VELOCITY", 0.)
	accel := gcmd.Float("ACCEL", 0.)
	logger.Debugf("FORCE_MOVE %s distance=%.3f velocity=%.3f accel=%.3f", stepper.Get_name(false), distance, speed, accel)
	self.forceEnable(stepper)
	self.ManualMove(stepper, distance, speed, &accel)
	return nil
}

func (self *ForceMoveModule) cmdSetKinematicPosition(gcmd printerpkg.Command) error {
	toolhead := self.lookupToolhead()
	toolhead.Get_last_move_time()
	curpos := toolhead.Get_position()
	x := gcmd.Float("X", curpos[0])
	y := gcmd.Float("Y", curpos[1])
	z := gcmd.Float("Z", curpos[2])
	logger.Infof("SET_KINEMATIC_POSITION pos=%.3f,%.3f,%.3f", x, y, z)
	toolhead.Set_position([]float64{x, y, z, curpos[3]}, []int{0, 1, 2})
	return nil
}
