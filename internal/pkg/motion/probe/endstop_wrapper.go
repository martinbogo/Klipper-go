package probe

const (
	MultiOff   = "OFF"
	MultiFirst = "FIRST"
	MultiOn    = "ON"
)

type EndstopWrapper struct {
	PositionEndstop  float64
	StowOnEachSample bool
	Multi            string
}

type EndstopRuntime interface {
	ToolheadPosition() []float64
	RunActivateGCode()
	RunDeactivateGCode()
}

type EndstopIdentifyRuntime interface {
	KinematicsSteppers() []interface{}
	StepperIsActiveAxis(stepper interface{}, axis rune) bool
	AddStepper(stepper interface{})
}

func NewEndstopWrapper(positionEndstop float64, stowOnEachSample bool) *EndstopWrapper {
	return &EndstopWrapper{
		PositionEndstop:  positionEndstop,
		StowOnEachSample: stowOnEachSample,
		Multi:            MultiOff,
	}
}

func (self *EndstopWrapper) BeginMultiProbe() {
	if self.StowOnEachSample {
		return
	}
	self.Multi = MultiFirst
}

func (self *EndstopWrapper) EndMultiProbe() bool {
	if self.StowOnEachSample {
		return false
	}
	self.Multi = MultiOff
	return true
}

func (self *EndstopWrapper) PrepareForProbe() bool {
	if self.Multi == MultiOff || self.Multi == MultiFirst {
		if self.Multi == MultiFirst {
			self.Multi = MultiOn
		}
		return true
	}
	return false
}

func (self *EndstopWrapper) FinishProbe() bool {
	return self.Multi == MultiOff
}

func (self *EndstopWrapper) GetPositionEndstop() float64 {
	return self.PositionEndstop
}

func ValidateVirtualEndstopPin(pinType string, pin string, invert bool, pullup bool) {
	if pinType != "endstop" || pin != "z_virtual_endstop" {
		panic("Probe virtual endstop only useful as endstop pin")
	}
	if invert || pullup {
		panic("Can not pullup/invert probe virtual endstop")
	}
}

func (self *EndstopWrapper) HandleMCUIdentify(runtime EndstopIdentifyRuntime) {
	for _, stepper := range runtime.KinematicsSteppers() {
		if runtime.StepperIsActiveAxis(stepper, 'z') {
			runtime.AddStepper(stepper)
		}
	}
}

func (self *EndstopWrapper) RaiseProbe(runtime EndstopRuntime) {
	startPos := runtime.ToolheadPosition()
	runtime.RunDeactivateGCode()
	if CoordinatesChanged(startPos, runtime.ToolheadPosition()) {
		panic("project.Toolhead moved during probe activate_gcode script")
	}
}

func (self *EndstopWrapper) LowerProbe(runtime EndstopRuntime) {
	startPos := runtime.ToolheadPosition()
	runtime.RunActivateGCode()
	if CoordinatesChanged(startPos, runtime.ToolheadPosition()) {
		panic("project.Toolhead moved during probe deactivate_gcode script")
	}
}

func (self *EndstopWrapper) HandleMultiProbeEnd(runtime EndstopRuntime) {
	if self.EndMultiProbe() {
		self.RaiseProbe(runtime)
	}
}

func (self *EndstopWrapper) HandleProbePrepare(runtime EndstopRuntime) {
	if self.PrepareForProbe() {
		self.LowerProbe(runtime)
	}
}

func (self *EndstopWrapper) HandleProbeFinish(runtime EndstopRuntime) {
	if self.FinishProbe() {
		self.RaiseProbe(runtime)
	}
}

func CoordinatesChanged(startPos, currentPos []float64) bool {
	limit := 3
	if len(startPos) < limit {
		limit = len(startPos)
	}
	if len(currentPos) < limit {
		limit = len(currentPos)
	}
	for i := 0; i < limit; i++ {
		if currentPos[i] != startPos[i] {
			return true
		}
	}
	return false
}
