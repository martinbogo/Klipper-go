package mcu

import "testing"

type fakeTrsyncManagedStepper struct {
	name            string
	oid             int
	noteHomingCount int
}

func (self *fakeTrsyncManagedStepper) MCUKey() interface{} { return self.name }
func (self *fakeTrsyncManagedStepper) Name(short bool) string {
	if short {
		return self.name
	}
	return self.name
}
func (self *fakeTrsyncManagedStepper) Raw() interface{} { return self }
func (self *fakeTrsyncManagedStepper) Get_oid() int     { return self.oid }
func (self *fakeTrsyncManagedStepper) Note_homing_end() { self.noteHomingCount++ }

type fakeTrsyncCompletion struct {
	completed []interface{}
}

func (self *fakeTrsyncCompletion) Complete(result interface{}) {
	self.completed = append(self.completed, result)
}

func TestTrsyncControllerAddStepperDeduplicates(t *testing.T) {
	controller := NewTrsyncController()
	stepper := &fakeTrsyncManagedStepper{name: "stepper_x", oid: 7}
	controller.AddStepper(stepper)
	controller.AddStepper(stepper)

	if got := len(controller.Steppers()); got != 1 {
		t.Fatalf("expected 1 stepper, got %d", got)
	}
}

func TestTrsyncControllerStartAndStopCoordinateStepperNotifications(t *testing.T) {
	controller := NewTrsyncController()
	stepperA := &fakeTrsyncManagedStepper{name: "stepper_x", oid: 7}
	stepperB := &fakeTrsyncManagedStepper{name: "stepper_y", oid: 8}
	controller.AddStepper(stepperA)
	controller.AddStepper(stepperB)

	var stopped []int
	completion := &fakeTrsyncCompletion{}
	controller.Start(9, 5.0, 0.25, completion, 0.1, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, func(seconds float64) int64 {
		return int64(seconds * 1000)
	}, nil, nil, nil, func(stepperOID int) {
		stopped = append(stopped, stepperOID)
	}, nil)

	if len(stopped) != 2 || stopped[0] != 7 || stopped[1] != 8 {
		t.Fatalf("unexpected stopped stepper oids: %#v", stopped)
	}

	reason := controller.Stop(9, false, nil, func(oid int, hostReason int64) int64 {
		if oid != 9 {
			t.Fatalf("unexpected oid: %d", oid)
		}
		if hostReason != ReasonHostRequest {
			t.Fatalf("unexpected host reason: %d", hostReason)
		}
		return ReasonEndstopHit
	})

	if reason != ReasonEndstopHit {
		t.Fatalf("expected endstop hit reason, got %d", reason)
	}
	if stepperA.noteHomingCount != 1 || stepperB.noteHomingCount != 1 {
		t.Fatalf("expected homing notifications, got %d and %d", stepperA.noteHomingCount, stepperB.noteHomingCount)
	}
}
