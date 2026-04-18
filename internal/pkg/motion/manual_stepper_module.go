package motion

import "fmt"

type ManualStepperHomeHandler func(movepos float64, speed float64, triggered bool, checkTrigger bool) error

type manualStepperLegacyMuxRegistrar interface {
	Register_mux_command(cmd string, key string, value string, handler func(interface{}) error, desc string)
}

type ManualStepperModule struct {
	steppers       []LegacyRailStepper
	runtime        *ManualStepperRuntime
	toolheadLookup func() ManualStepperToolhead
	motorLookup    func() ManualStepperMotorController
	homeHandler    ManualStepperHomeHandler
}

func NewManualStepperModule(steppers []LegacyRailStepper, velocity float64, accel float64,
	toolheadLookup func() ManualStepperToolhead,
	motorLookup func() ManualStepperMotorController,
	homeHandler ManualStepperHomeHandler,
) *ManualStepperModule {
	stepperCore := NewLegacyRailRuntime()
	for _, stepper := range steppers {
		stepperCore.AddStepper(stepper)
	}
	stepperCore.SetupItersolve("cartesian_stepper_alloc", uint8('x'))
	return &ManualStepperModule{
		steppers:       append([]LegacyRailStepper{}, steppers...),
		runtime:        NewManualStepperRuntime(velocity, accel, stepperCore),
		toolheadLookup: toolheadLookup,
		motorLookup:    motorLookup,
		homeHandler:    homeHandler,
	}
}

func (self *ManualStepperModule) RegisterLegacyMuxCommand(gcode manualStepperLegacyMuxRegistrar, stepperName string) {
	if self == nil || gcode == nil {
		return
	}
	gcode.Register_mux_command("MANUAL_STEPPER", "STEPPER", stepperName, self.HandleLegacyCommand, "Command a manually configured stepper")
}

func (self *ManualStepperModule) HandleLegacyCommand(argv interface{}) error {
	command, ok := argv.(ManualStepperCommand)
	if !ok {
		return fmt.Errorf("manual stepper command %T does not implement motion.ManualStepperCommand", argv)
	}
	return HandleManualStepperCommand(self, command)
}

func (self *ManualStepperModule) toolhead() ManualStepperToolhead {
	if self == nil || self.toolheadLookup == nil {
		return nil
	}
	return self.toolheadLookup()
}

func (self *ManualStepperModule) motorController() ManualStepperMotorController {
	if self == nil || self.motorLookup == nil {
		return nil
	}
	return self.motorLookup()
}

func (self *ManualStepperModule) stepperNames() []string {
	if self == nil {
		return nil
	}
	names := make([]string, 0, len(self.steppers))
	for _, stepper := range self.steppers {
		names = append(names, stepper.Get_name(false))
	}
	return names
}

func (self *ManualStepperModule) ManualStepperVelocity() float64 {
	if self == nil {
		return 0.
	}
	return self.runtime.Velocity()
}

func (self *ManualStepperModule) ManualStepperAccel() float64 {
	if self == nil {
		return 0.
	}
	return self.runtime.Accel()
}

func (self *ManualStepperModule) SetManualStepperEnabled(enable bool) {
	if self == nil {
		return
	}
	self.runtime.SetEnabled(self.toolhead(), self.motorController(), self.stepperNames(), enable)
}

func (self *ManualStepperModule) SetManualStepperPosition(setpos float64) {
	if self == nil {
		return
	}
	self.runtime.SetPosition(setpos)
}

func (self *ManualStepperModule) MoveManualStepper(movepos float64, speed float64, accel float64, sync bool) {
	if self == nil {
		return
	}
	self.runtime.Move(self.toolhead(), movepos, speed, accel, sync)
}

func (self *ManualStepperModule) HomeManualStepper(movepos float64, speed float64, accel float64, triggered bool, checkTrigger bool) error {
	if self == nil || self.homeHandler == nil {
		panic("No endstop for this manual stepper")
	}
	self.runtime.UpdateHomingAccel(accel)
	return self.homeHandler(movepos, speed, triggered, checkTrigger)
}

func (self *ManualStepperModule) SyncManualStepper() {
	if self == nil {
		return
	}
	self.runtime.SyncPrintTime(self.toolhead())
}

func (self *ManualStepperModule) Flush_step_generation() {
	self.SyncManualStepper()
}

func (self *ManualStepperModule) Get_position() []float64 {
	if self == nil {
		return []float64{0., 0., 0., 0.}
	}
	return self.runtime.Position()
}

func (self *ManualStepperModule) Set_position(newpos []float64, homingAxes []int) {
	_ = homingAxes
	if self == nil || len(newpos) == 0 {
		return
	}
	self.runtime.SetPosition(newpos[0])
}

func (self *ManualStepperModule) Get_status(eventtime float64) map[string]interface{} {
	_ = eventtime
	return map[string]interface{}{}
}

func (self *ManualStepperModule) Get_last_move_time() float64 {
	if self == nil {
		return 0.
	}
	return self.runtime.LastMoveTime(self.toolhead())
}

func (self *ManualStepperModule) Dwell(delay float64) {
	if self == nil {
		return
	}
	self.runtime.Dwell(delay)
}

func (self *ManualStepperModule) Drip_move(newpos []float64, speed float64) error {
	if self == nil {
		return nil
	}
	return self.runtime.DripMove(self.toolhead(), newpos, speed)
}

func (self *ManualStepperModule) Get_kinematics() interface{} {
	return self
}

func (self *ManualStepperModule) Get_steppers() []interface{} {
	if self == nil {
		return nil
	}
	steppers := make([]interface{}, 0, len(self.steppers))
	for _, stepper := range self.steppers {
		steppers = append(steppers, stepper)
	}
	return steppers
}

func (self *ManualStepperModule) Calc_position(stepperPositions map[string]float64) []float64 {
	if self == nil {
		return []float64{0., 0., 0.}
	}
	return self.runtime.CalcPosition(stepperPositions)
}
