package probe

import (
	"fmt"
	"strings"
)

type ProbeMotionRuntime interface {
	Core() *PrinterProbe
	HomedAxes() string
	ToolheadPosition() []float64
	ProbingMove(target []float64, speed float64) []float64
	RespondInfo(msg string, log bool)
}

func RunProbeMove(runtime ProbeMotionRuntime, speed float64) []float64 {
	if !strings.Contains(runtime.HomedAxes(), "z") {
		panic("Must home before probe")
	}
	pos := runtime.ToolheadPosition()
	pos[2] = runtime.Core().ZPosition
	epos := runtime.ProbingMove(pos, speed)
	runtime.RespondInfo(fmt.Sprintf("probe at %.3f,%.3f is z=%.6f", epos[0], epos[1], epos[2]), true)
	return epos[:3]
}
