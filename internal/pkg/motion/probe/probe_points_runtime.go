package probe

import "strings"

type ProbePointsAutomaticProbe interface {
	GetLiftSpeed(command ProbeCommand) float64
	GetOffsets() []float64
	BeginMultiProbe()
	EndMultiProbe()
	RunProbe(command ProbeCommand) []float64
}

type ProbePointsRuntime interface {
	EnsureNoManualProbe()
	LookupAutomaticProbe() ProbePointsAutomaticProbe
	Move(coord interface{}, speed float64)
	TouchLastMoveTime()
	StartManualProbe(finalize func([]float64))
}

func (self *ProbePointsHelper) MoveNext(context ProbePointsRuntime) bool {
	speed := self.liftSpeed
	if len(self.results) != 0 {
		speed = self.speed
	}
	context.Move([]interface{}{nil, nil, self.horizontalMoveZ}, speed)
	if len(self.results) >= len(self.probePoints) {
		context.TouchLastMoveTime()
	}
	done, _, target := self.NextProbePoint()
	if done {
		return true
	}
	context.Move(coordToInterfaces(target), self.speed)
	return false
}

func (self *ProbePointsHelper) StartProbe(context ProbePointsRuntime, command ProbeCommand) {
	context.EnsureNoManualProbe()
	probe := context.LookupAutomaticProbe()
	zero := 0.0
	method := strings.ToLower(command.Get("METHOD", "automatic", 0, &zero, &zero, &zero, &zero))
	if probe == nil || method != "automatic" {
		self.BeginManualSession()
		self.StartManualProbe(context)
		return
	}
	liftSpeed := probe.GetLiftSpeed(command)
	self.BeginAutomaticSession(liftSpeed, probe.GetOffsets())
	if self.horizontalMoveZ < self.probeOffsets[2] {
		panic("horizontal_move_z can t be less than probe's z_offset")
	}
	probe.BeginMultiProbe()
	for {
		done := self.MoveNext(context)
		if done {
			break
		}
		self.AppendResult(probe.RunProbe(command))
	}
	probe.EndMultiProbe()
}

func (self *ProbePointsHelper) StartManualProbe(context ProbePointsRuntime) {
	done := self.MoveNext(context)
	if !done {
		context.StartManualProbe(func(kinPos []float64) {
			self.HandleManualProbeResult(context, kinPos)
		})
	}
}

func (self *ProbePointsHelper) HandleManualProbeResult(context ProbePointsRuntime, kinPos []float64) {
	if kinPos == nil {
		return
	}
	self.AppendResult(kinPos)
	self.StartManualProbe(context)
}
