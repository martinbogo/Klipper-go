package motion

import "testing"

func TestLegacyExtruderRuntimeGetStatusMergesHeaterAndStepperState(t *testing.T) {
	runtime := &LegacyExtruderRuntime{
		Nozzle_diameter: 0.4,
		Can_extrude: func() bool {
			return true
		},
		Heater_status: func(eventtime float64) map[string]float64 {
			if eventtime != 12.5 {
				t.Fatalf("expected eventtime 12.5, got %v", eventtime)
			}
			return map[string]float64{
				"temperature": 215,
				"target":      220,
			}
		},
		Stepper_status: func(eventtime float64) map[string]float64 {
			if eventtime != 12.5 {
				t.Fatalf("expected eventtime 12.5, got %v", eventtime)
			}
			return map[string]float64{
				"pressure_advance": 0.075,
			}
		},
	}

	status := runtime.Get_status(12.5)
	if status["temperature"] != 215.0 {
		t.Fatalf("expected heater temperature in status, got %#v", status)
	}
	if status["can_extrude"] != true {
		t.Fatalf("expected can_extrude=true, got %#v", status["can_extrude"])
	}
	if status["Nozzle_diameter"] != 0.4 {
		t.Fatalf("expected nozzle diameter in status, got %#v", status["Nozzle_diameter"])
	}
	if status["pressure_advance"] != 0.075 {
		t.Fatalf("expected stepper status merge, got %#v", status)
	}
}

func TestLegacyExtruderRuntimeMoveAppendsTrapqAndTracksPosition(t *testing.T) {
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{1, 0, 0, 1}, 20)
	move.Set_junction(4, 9, 1)

	type trapqCall struct {
		trapq     interface{}
		printTime float64
		startPosX float64
		axesRY    float64
		startV    float64
		cruiseV   float64
		accel     float64
	}
	var got trapqCall

	runtime := &LegacyExtruderRuntime{
		Trapq: "trapq",
		Trapq_append: func(tq interface{}, print_time,
			accel_t, cruise_t, decel_t,
			start_pos_x, start_pos_y, start_pos_z,
			axes_r_x, axes_r_y, axes_r_z,
			start_v, cruise_v, accel float64) {
			got = trapqCall{
				trapq:     tq,
				printTime: print_time,
				startPosX: start_pos_x,
				axesRY:    axes_r_y,
				startV:    start_v,
				cruiseV:   cruise_v,
				accel:     accel,
			}
		},
	}

	runtime.Move(7.5, move)

	if got.trapq != "trapq" {
		t.Fatalf("expected trapq handle to be forwarded, got %#v", got.trapq)
	}
	if got.printTime != 7.5 {
		t.Fatalf("expected print time 7.5, got %v", got.printTime)
	}
	if got.startPosX != move.Start_pos[3] {
		t.Fatalf("expected start position %v, got %v", move.Start_pos[3], got.startPosX)
	}
	if got.axesRY != 1.0 {
		t.Fatalf("expected pressure advance flag 1.0, got %v", got.axesRY)
	}
	if got.startV != move.Start_v || got.cruiseV != move.Cruise_v || got.accel != move.Accel {
		t.Fatalf("unexpected trapq motion values: %#v", got)
	}
	if runtime.Last_position != move.End_pos[3] {
		t.Fatalf("expected last position %v, got %v", move.End_pos[3], runtime.Last_position)
	}
}

func TestLegacyExtruderRuntimeFindPastPositionUsesStepperCallback(t *testing.T) {
	runtime := &LegacyExtruderRuntime{
		Find_stepper_past_position: func(printTime float64) float64 {
			if printTime != 3.25 {
				t.Fatalf("expected printTime 3.25, got %v", printTime)
			}
			return 42.5
		},
	}

	if got := runtime.Find_past_position(3.25); got != 42.5 {
		t.Fatalf("expected delegated past position, got %v", got)
	}
}

func TestDummyExtruderCheckMoveAndJunction(t *testing.T) {
	dummy := NewDummyExtruder()
	move := NewMove(testMoveConfig(), []float64{0, 0, 0, 0}, []float64{1, 2, 3, 4}, 20)

	err := dummy.Check_move(move)
	if err == nil || err.Error() == "" {
		t.Fatalf("expected dummy extruder move error, got %v", err)
	}
	if got := dummy.Calc_junction(nil, move); got != move.Max_cruise_v2 {
		t.Fatalf("expected default junction %v, got %v", move.Max_cruise_v2, got)
	}
	dummy.Move(3.5, move)
	if got := dummy.Find_past_position(2.5); got != 0 {
		t.Fatalf("expected zero past position, got %v", got)
	}
}
