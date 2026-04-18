package motion

import "testing"

type fakeManualStepperMuxRegistration struct {
	cmd     string
	key     string
	value   string
	desc    string
	handler func(interface{}) error
	count   int
}

func (self *fakeManualStepperMuxRegistration) Register_mux_command(cmd string, key string, value string, handler func(interface{}) error, desc string) {
	self.cmd = cmd
	self.key = key
	self.value = value
	self.desc = desc
	self.handler = handler
	self.count++
}

func TestManualStepperModuleRegistersLegacyCommandAndCoordinatesRuntime(t *testing.T) {
	stepper := &fakeLegacyRailStepper{name: "manual_stepper", commandedPosition: 2.0}
	toolhead := &fakeManualStepperToolhead{lastMoveTime: 4.0}
	motors := &fakeManualStepperMotorController{}
	homeCalls := 0
	module := NewManualStepperModule(
		[]LegacyRailStepper{stepper},
		5.0,
		2.5,
		func() ManualStepperToolhead { return toolhead },
		func() ManualStepperMotorController { return motors },
		func(movepos float64, speed float64, triggered bool, checkTrigger bool) error {
			homeCalls++
			if movepos != 9.0 || speed != 7.0 || !triggered || checkTrigger {
				t.Fatalf("home callback args = %v %v %v %v", movepos, speed, triggered, checkTrigger)
			}
			return nil
		},
	)

	if stepper.setupCalls != 1 {
		t.Fatalf("SetupItersolve calls = %d, want 1", stepper.setupCalls)
	}
	if len(stepper.trapqs) != 1 || stepper.trapqs[0] == nil {
		t.Fatal("expected constructor to bind trapq to stepper")
	}

	registration := &fakeManualStepperMuxRegistration{}
	module.RegisterLegacyMuxCommand(registration, "select_stepper")
	if registration.count != 1 {
		t.Fatalf("Register_mux_command calls = %d, want 1", registration.count)
	}
	if registration.cmd != "MANUAL_STEPPER" || registration.key != "STEPPER" || registration.value != "select_stepper" {
		t.Fatalf("registration = %+v", registration)
	}

	command := &fakeManualStepperCommand{
		ints: map[string]int{"ENABLE": 1, "SYNC": 0},
		floats: map[string]float64{
			"SET_POSITION": 3.5,
			"MOVE":         8.0,
			"SPEED":        6.0,
			"ACCEL":        2.0,
		},
	}
	if err := registration.handler(command); err != nil {
		t.Fatalf("legacy handler error = %v", err)
	}
	if len(motors.calls) != 1 || !motors.calls[0].enable {
		t.Fatalf("motor calls = %#v, want enabled call", motors.calls)
	}
	if len(stepper.positions) != 1 || stepper.positions[0][0] != 3.5 {
		t.Fatalf("positions = %#v, want [3.5 ...]", stepper.positions)
	}
	if stepper.generateCalls != 1 {
		t.Fatalf("Generate_steps calls = %d, want 1", stepper.generateCalls)
	}

	if err := module.HomeManualStepper(9.0, 7.0, 4.0, true, false); err != nil {
		t.Fatalf("HomeManualStepper() error = %v", err)
	}
	if homeCalls != 1 {
		t.Fatalf("home callback calls = %d, want 1", homeCalls)
	}
	if got := module.Get_last_move_time(); got < 4.0 {
		t.Fatalf("Get_last_move_time() = %v, want >= 4.0", got)
	}
	if got := module.Calc_position(map[string]float64{"manual_stepper": 11.0}); got[0] != 11.0 {
		t.Fatalf("Calc_position() = %#v, want first axis 11.0", got)
	}
	if len(module.Get_steppers()) != 1 {
		t.Fatalf("Get_steppers() len = %d, want 1", len(module.Get_steppers()))
	}
	if err := module.Drip_move([]float64{12.0}, 5.0); err != nil {
		t.Fatalf("Drip_move() error = %v", err)
	}
	if stepper.generateCalls != 2 {
		t.Fatalf("Generate_steps calls after drip = %d, want 2", stepper.generateCalls)
	}
	module.Dwell(0.5)
	module.Flush_step_generation()
	if len(toolhead.dwells) == 0 {
		t.Fatal("expected dwell activity after flush")
	}
	if got := module.Get_position(); got[0] != 2.0 {
		t.Fatalf("Get_position() = %#v, want first axis 2.0", got)
	}
	module.Set_position([]float64{14.0, 0, 0, 0}, nil)
	if len(stepper.positions) != 2 || stepper.positions[1][0] != 14.0 {
		t.Fatalf("Set_position() positions = %#v, want second axis 14.0", stepper.positions)
	}
}

func TestManualStepperModuleRejectsUnexpectedLegacyCommandAndRequiresHomeHandler(t *testing.T) {
	module := NewManualStepperModule(nil, 5.0, 2.0, nil, nil, nil)
	if err := module.HandleLegacyCommand(struct{}{}); err == nil {
		t.Fatal("expected type error for invalid legacy command")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when homing without handler")
		}
	}()
	_ = module.HomeManualStepper(1.0, 2.0, 3.0, true, true)
}
