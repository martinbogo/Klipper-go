package probe

import "fmt"

type ManualProbeModuleContext interface {
	StartManualProbe(command ProbeCommand, finalize func([]float64))
	ZPositionEndstop() float64
	DeltaPositionEndstops() (float64, float64, float64)
	HomingOriginZ() float64
	SetConfig(section string, option string, value string)
	RespondInfo(msg string, log bool)
}

func manualProbeFinalize(context ManualProbeModuleContext) func([]float64) {
	return func(kinPos []float64) {
		if len(kinPos) > 0 {
			context.RespondInfo(fmt.Sprintf("Z position is %.3f", kinPos[2]), true)
		}
	}
}

func zEndstopFinalize(context ManualProbeModuleContext) func([]float64) {
	return func(kinPos []float64) {
		if len(kinPos) == 0 {
			return
		}

		zPos := context.ZPositionEndstop() - kinPos[2]
		context.RespondInfo(fmt.Sprintf("stepper_z: Position_endstop: %.3f\n"+
			"The SAVE_CONFIG command will update the printer config file\n "+
			"with the above and restart the printer.", zPos), true)
		context.SetConfig("stepper_z", "Position_endstop", fmt.Sprintf("%.3f", zPos))
	}
}

func HandleManualProbeCommand(context ManualProbeModuleContext, command ProbeCommand) error {
	context.StartManualProbe(command, manualProbeFinalize(context))
	return nil
}

func HandleZEndstopCalibrateCommand(context ManualProbeModuleContext, command ProbeCommand) error {
	context.StartManualProbe(command, zEndstopFinalize(context))
	return nil
}

func HandleZOffsetApplyEndstopCommand(context ManualProbeModuleContext) error {
	offset := context.HomingOriginZ()
	if offset == 0 {
		context.RespondInfo("Nothing to do: Z Offset is 0", true)
		return nil
	}
	newCalibrate := context.ZPositionEndstop() - offset
	context.RespondInfo(fmt.Sprintf("stepper_z: Position_endstop: %.3f\n"+
		"The SAVE_CONFIG command will update the printer config file\n "+
		"with the above and restart the printer.", newCalibrate), true)
	context.SetConfig("stepper_z", "Position_endstop", fmt.Sprintf("%.3f", newCalibrate))
	return nil
}

func HandleZOffsetApplyDeltaEndstopsCommand(context ManualProbeModuleContext) error {
	offset := context.HomingOriginZ()
	if offset == 0 {
		context.RespondInfo("Nothing to do: Z Offset is 0", true)
		return nil
	}
	aPositionEndstop, bPositionEndstop, cPositionEndstop := context.DeltaPositionEndstops()
	newACalibrate := aPositionEndstop - offset
	newBCalibrate := bPositionEndstop - offset
	newCCalibrate := cPositionEndstop - offset
	context.RespondInfo(fmt.Sprintf(
		"stepper_a: position_endstop: %.3f\n"+
			"stepper_b: position_endstop: %.3f\n"+
			"stepper_c: position_endstop: %.3f\n"+
			"The SAVE_CONFIG command will update the printer config file\n "+
			"with the above and restart the printer.", newACalibrate, newBCalibrate, newCCalibrate), true)
	context.SetConfig("stepper_a", "Position_endstop", fmt.Sprintf("%.3f", newACalibrate))
	context.SetConfig("stepper_b", "Position_endstop", fmt.Sprintf("%.3f", newBCalibrate))
	context.SetConfig("stepper_c", "Position_endstop", fmt.Sprintf("%.3f", newCCalibrate))
	return nil
}
