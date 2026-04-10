package probe

import "fmt"

type ProbeCommand interface {
	Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	Get_int(name string, _default interface{}, minval *int, maxval *int) int
	Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64
	RespondInfo(msg string, log bool)
}

type ProbeCommandContext interface {
	Name() string
	Core() *PrinterProbe
	ToolheadPosition() []float64
	LastMoveTime() float64
	QueryEndstop(printTime float64) int
	Probe(speed float64) []float64
	RunProbeCommand(command ProbeCommand) []float64
	Move(coord []interface{}, speed float64)
	BeginMultiProbe()
	EndMultiProbe()
	EnsureNoManualProbe()
	StartManualProbe(command ProbeCommand, finalize func([]float64))
	SetConfig(section string, option string, value string)
	HomingOriginZ() float64
	RespondInfo(msg string, log bool)
}

func liftSpeedFromCommand(command ProbeCommand, defaultValue float64) float64 {
	zero := 0.0
	return command.Get_float("LIFT_SPEED", defaultValue, nil, nil, &zero, nil)
}

func coordToInterfaces(coord []float64) []interface{} {
	converted := make([]interface{}, len(coord))
	for i, value := range coord {
		converted[i] = value
	}
	return converted
}

func probeCalibrateFinalize(context ProbeCommandContext) func([]float64) {
	return func(kinPos []float64) {
		if len(kinPos) == 0 {
			return
		}

		zOffset := context.Core().CalibratedOffset(kinPos)
		context.RespondInfo(fmt.Sprintf(
			"%s: z_offset: %.3f\n"+
				"The SAVE_CONFIG command will update the printer config file\n"+
				"with the above and restart the printer.",
			context.Name(), zOffset), true)
		context.SetConfig(context.Name(), "z_offset", fmt.Sprintf("%.3f", zOffset))
	}
}

func HandleProbeCommand(context ProbeCommandContext, command ProbeCommand) error {
	pos := context.RunProbeCommand(command)
	command.RespondInfo(fmt.Sprintf("Result is z=%.6f", pos[2]), true)
	context.Core().RecordLastZResult(pos[2])
	return nil
}

func HandleQueryProbeCommand(context ProbeCommandContext, command ProbeCommand) error {
	count := command.Get_int("COUNT", 5, nil, nil)
	for i := 0; i < count; i++ {
		res := context.QueryEndstop(context.LastMoveTime())
		context.Core().RecordLastState(res == 1)
		if res == 1 {
			command.RespondInfo(fmt.Sprintf("probe: %s", "TRIGGERED"), true)
			break
		}
		command.RespondInfo(fmt.Sprintf("probe: %s", "open"), true)
	}
	return nil
}

func HandleProbeAccuracyCommand(context ProbeCommandContext, command ProbeCommand) error {
	core := context.Core()
	zero := 0.0
	one := 1
	speed := command.Get_float("PROBE_SPEED", core.Speed, nil, nil, &zero, nil)
	liftSpeed := liftSpeedFromCommand(command, core.LiftSpeed)
	sampleCount := command.Get_int("SAMPLES", 10, &one, nil)
	sampleRetractDist := command.Get_float("SAMPLE_RETRACT_DIST", core.SampleRetractDist, nil, nil, &zero, nil)
	pos := context.ToolheadPosition()
	command.RespondInfo(fmt.Sprintf("PROBE_ACCURACY at X:%.3f Y:%.3f Z:%.3f"+
		" (samples=%d retract=%.3f"+
		" speed=%.1f lift_speed=%.1f)\n", pos[0], pos[1], pos[2],
		sampleCount, sampleRetractDist,
		speed, liftSpeed), true)
	context.BeginMultiProbe()
	positions := [][]float64{}
	for {
		pos := context.Probe(speed)
		positions = append(positions, pos)
		context.Move([]interface{}{nil, nil, pos[2] + sampleRetractDist}, liftSpeed)
		if len(positions) >= sampleCount {
			break
		}
	}
	context.EndMultiProbe()
	stats := Accuracy(positions)
	command.RespondInfo(
		fmt.Sprintf("probe accuracy results: maximum %.6f, minimum %.6f, range %.6f,"+
			"average %.6f, median %.6f, standard deviation %.6f",
			stats.Maximum, stats.Minimum, stats.Range, stats.Average, stats.Median, stats.Sigma), true)
	return nil
}

func HandleProbeCalibrateCommand(context ProbeCommandContext, command ProbeCommand) error {
	core := context.Core()
	liftSpeed := liftSpeedFromCommand(command, core.LiftSpeed)
	context.EnsureNoManualProbe()
	curpos := context.RunProbeCommand(command)
	core.SetProbeCalibrateZ(curpos[2])
	curpos[2] += 5.
	context.Move(coordToInterfaces(curpos), liftSpeed)
	curpos[0] += core.XOffset
	curpos[1] += core.YOffset
	context.Move(coordToInterfaces(curpos), core.Speed)
	context.StartManualProbe(command, probeCalibrateFinalize(context))
	return nil
}

func HandleZOffsetApplyProbeCommand(context ProbeCommandContext) error {
	offset := context.HomingOriginZ()
	if offset == 0 {
		context.RespondInfo("Nothing to do: Z Offset is 0", true)
		return nil
	}
	newCalibrate := context.Core().ZOffset - offset
	context.RespondInfo(fmt.Sprintf(
		"%s: z_offset: %.3f\n"+
			"The SAVE_CONFIG command will update the printer config file\n"+
			"with the above and restart the printer.",
		context.Name(), newCalibrate), true)
	context.SetConfig(context.Name(), "z_offset", fmt.Sprintf("%.4f", newCalibrate))
	return nil
}
