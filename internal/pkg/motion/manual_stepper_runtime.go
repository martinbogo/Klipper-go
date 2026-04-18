package motion

import "math"

type ManualStepperToolhead interface {
	GetLastMoveTime() float64
	Dwell(delay float64)
	NoteMovequeueActivity(mqTime float64, setStepGenTime bool)
}

type ManualStepperMotorController interface {
	SetStepperEnabled(stepperName string, printTime float64, enable bool)
}

type ManualStepperRuntime struct {
	velocity    float64
	accel       float64
	homingAccel float64
	motionCore  *ManualStepperCore
	stepperCore *LegacyRailRuntime
}

func NewManualStepperRuntime(velocity float64, accel float64, stepperCore *LegacyRailRuntime) *ManualStepperRuntime {
	runtime := &ManualStepperRuntime{
		velocity:    velocity,
		accel:       accel,
		homingAccel: accel,
		motionCore:  NewManualStepperCore(),
		stepperCore: stepperCore,
	}
	if stepperCore != nil {
		stepperCore.SetTrapq(runtime.motionCore.Trapq())
	}
	return runtime
}

func (self *ManualStepperRuntime) Velocity() float64 {
	if self == nil {
		return 0.
	}
	return self.velocity
}

func (self *ManualStepperRuntime) Accel() float64 {
	if self == nil {
		return 0.
	}
	return self.accel
}

func (self *ManualStepperRuntime) UpdateHomingAccel(accel float64) {
	if self == nil {
		return
	}
	self.homingAccel = accel
}

func (self *ManualStepperRuntime) SyncPrintTime(toolhead ManualStepperToolhead) {
	if self == nil || toolhead == nil {
		return
	}
	if delay := self.motionCore.SyncPrintTime(toolhead.GetLastMoveTime()); delay > 0 {
		toolhead.Dwell(delay)
	}
}

func (self *ManualStepperRuntime) SetEnabled(toolhead ManualStepperToolhead, motors ManualStepperMotorController, stepperNames []string, enable bool) {
	if self == nil || motors == nil {
		return
	}
	self.SyncPrintTime(toolhead)
	printTime := self.motionCore.NextCmdTime()
	for _, stepperName := range stepperNames {
		motors.SetStepperEnabled(stepperName, printTime, enable)
	}
	self.SyncPrintTime(toolhead)
}

func (self *ManualStepperRuntime) SetPosition(setpos float64) {
	if self == nil || self.stepperCore == nil {
		return
	}
	self.stepperCore.SetPosition([]float64{setpos, 0., 0.})
}

func (self *ManualStepperRuntime) Move(toolhead ManualStepperToolhead, movepos float64, speed float64, accel float64, sync bool) float64 {
	if self == nil || self.stepperCore == nil {
		return 0.
	}
	self.SyncPrintTime(toolhead)
	currentPosition := self.stepperCore.GetCommandedPosition()
	moveEndTime := self.motionCore.QueueMove(currentPosition, movepos, speed, accel)
	self.stepperCore.GenerateSteps(moveEndTime)
	self.motionCore.FinalizeMoves()
	if toolhead != nil {
		toolhead.NoteMovequeueActivity(moveEndTime, false)
	}
	if sync {
		self.SyncPrintTime(toolhead)
	}
	return moveEndTime
}

func (self *ManualStepperRuntime) Position() []float64 {
	if self == nil || self.stepperCore == nil {
		return []float64{0., 0., 0., 0.}
	}
	return []float64{self.stepperCore.GetCommandedPosition(), 0., 0., 0.}
}

func (self *ManualStepperRuntime) LastMoveTime(toolhead ManualStepperToolhead) float64 {
	if self == nil {
		return 0.
	}
	self.SyncPrintTime(toolhead)
	return self.motionCore.NextCmdTime()
}

func (self *ManualStepperRuntime) Dwell(delay float64) {
	if self == nil {
		return
	}
	self.motionCore.Dwell(math.Max(0., delay))
}

func (self *ManualStepperRuntime) DripMove(toolhead ManualStepperToolhead, newpos []float64, speed float64) error {
	if len(newpos) == 0 {
		return nil
	}
	self.Move(toolhead, newpos[0], speed, self.homingAccel, true)
	return nil
}

func (self *ManualStepperRuntime) CalcPosition(stepperPositions map[string]float64) []float64 {
	if self == nil || self.stepperCore == nil {
		return []float64{0., 0., 0.}
	}
	return []float64{stepperPositions[self.stepperCore.GetName(false)], 0., 0.}
}
