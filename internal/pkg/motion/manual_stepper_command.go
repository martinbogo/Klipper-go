package motion

type ManualStepperCommand interface {
	Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64
	Get_int(name string, _default interface{}, minval *int, maxval *int) int
}

type ManualStepperCommandRuntime interface {
	ManualStepperVelocity() float64
	ManualStepperAccel() float64
	SetManualStepperEnabled(enable bool)
	SetManualStepperPosition(setpos float64)
	HomeManualStepper(movepos float64, speed float64, accel float64, triggered bool, checkTrigger bool) error
	MoveManualStepper(movepos float64, speed float64, accel float64, sync bool)
	SyncManualStepper()
}

func HandleManualStepperCommand(runtime ManualStepperCommandRuntime, command ManualStepperCommand) error {
	if enable := command.Get_int("ENABLE", -1, nil, nil); enable != -1 {
		runtime.SetManualStepperEnabled(enable == 1)
	}

	if setpos := command.Get_float("SET_POSITION", -1.0, nil, nil, nil, nil); setpos != -1.0 {
		runtime.SetManualStepperPosition(setpos)
	}

	speed := command.Get_float("SPEED", runtime.ManualStepperVelocity(), nil, nil, nil, nil)
	accel := command.Get_float("ACCEL", runtime.ManualStepperAccel(), nil, nil, nil, nil)
	homingMove := command.Get_int("STOP_ON_ENDSTOP", 0, nil, nil)
	if homingMove != 0 {
		movepos := command.Get_float("MOVE", 0.0, nil, nil, nil, nil)
		return runtime.HomeManualStepper(movepos, speed, accel, homingMove > 0, homingMove == 1)
	}
	if movepos := command.Get_float("MOVE", -1.0, nil, nil, nil, nil); movepos != -1.0 {
		sync := command.Get_int("SYNC", 1, nil, nil)
		runtime.MoveManualStepper(movepos, speed, accel, sync == 1)
	} else if command.Get_int("SYNC", 0, nil, nil) == 1 {
		runtime.SyncManualStepper()
	}
	return nil
}
