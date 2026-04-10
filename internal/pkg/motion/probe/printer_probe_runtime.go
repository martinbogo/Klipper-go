package probe

type ProbeRunContext interface {
	Core() *PrinterProbe
	ToolheadPosition() []float64
	Probe(speed float64) []float64
	Move(coord []interface{}, speed float64)
	BeginMultiProbe()
	EndMultiProbe()
}

func retractMove(probexy []float64, z float64) []interface{} {
	move := make([]interface{}, 0, len(probexy)+1)
	for _, item := range probexy {
		move = append(move, item)
	}
	move = append(move, z)
	return move
}

func RunProbeSequence(context ProbeRunContext, command ProbeCommand) []float64 {
	core := context.Core()
	zero := 0.0
	speed := command.Get_float("PROBE_SPEED", core.Speed, nil, nil, &zero, nil)
	liftSpeed := liftSpeedFromCommand(command, core.LiftSpeed)
	one := 1
	sampleCount := command.Get_int("SAMPLES", core.SampleCount, &one, nil)
	sampleRetractDist := command.Get_float("SAMPLE_RETRACT_DIST", core.SampleRetractDist, nil, nil, &zero, nil)
	samplesTolerance := command.Get_float("SAMPLES_TOLERANCE", core.SamplesTolerance, &zero, nil, &zero, nil)
	zeroInt := 0
	samplesRetries := command.Get_int("SAMPLES_TOLERANCE_RETRIES", core.SamplesRetries, &zeroInt, nil)
	samplesResult := command.Get("SAMPLES_RESULT", core.SamplesResult, "", &zero, &zero, &zero, &zero)
	mustNotifyMultiProbe := !core.MultiProbePending
	if mustNotifyMultiProbe {
		context.BeginMultiProbe()
	}
	probexy := append([]float64{}, context.ToolheadPosition()[:2]...)
	retries := 0
	positions := [][]float64{}
	for len(positions) < sampleCount {
		pos := context.Probe(speed)
		positions = append(positions, pos)
		if ExceedsTolerance(positions, samplesTolerance) {
			if retries >= samplesRetries {
				panic("Probe samples exceed samples_tolerance")
			}
			command.RespondInfo("Probe samples exceed tolerance. Retrying...", true)
			retries += 1
			positions = [][]float64{}
		}
		if len(positions) < sampleCount {
			context.Move(retractMove(probexy, pos[2]+sampleRetractDist), liftSpeed)
		}
	}
	if mustNotifyMultiProbe {
		context.EndMultiProbe()
	}
	if samplesResult == "median" {
		return MedianPosition(positions)
	}
	return MeanPosition(positions)
}
