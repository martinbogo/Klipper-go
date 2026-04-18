package motion

import "testing"

type fakeSyncTarget struct {
	*fakeCommandExtruder
	lastPosition float64
}

func (self *fakeSyncTarget) LegacyLastPosition() float64 {
	return self.lastPosition
}

type fakeExtruderTemperatureRuntime struct {
	active             Extruder
	lookup             map[string]Extruder
	setExtruder        Extruder
	setTemp            float64
	setWait            bool
	setTemperatureCall int
}

func (self *fakeExtruderTemperatureRuntime) ActiveExtruder() Extruder {
	return self.active
}

func (self *fakeExtruderTemperatureRuntime) LookupExtruder(section string) Extruder {
	if self.lookup == nil {
		return nil
	}
	return self.lookup[section]
}

func (self *fakeExtruderTemperatureRuntime) SetTemperature(extruder Extruder, temp float64, wait bool) error {
	self.setExtruder = extruder
	self.setTemp = temp
	self.setWait = wait
	self.setTemperatureCall++
	return nil
}

type fakeCommandExtruder struct {
	name   string
	heater interface{}
}

func (self *fakeCommandExtruder) Update_move_time(flush_time float64, clear_history_time float64) {}
func (self *fakeCommandExtruder) Check_move(move *Move) error                                     { return nil }
func (self *fakeCommandExtruder) Find_past_position(print_time float64) float64                   { return 0.0 }
func (self *fakeCommandExtruder) Calc_junction(prev_move, move *Move) float64                     { return 0.0 }
func (self *fakeCommandExtruder) Move(print_time float64, move *Move)                             {}
func (self *fakeCommandExtruder) Get_name() string                                                { return self.name }
func (self *fakeCommandExtruder) Get_heater() interface{}                                         { return self.heater }
func (self *fakeCommandExtruder) Get_trapq() interface{}                                          { return nil }

func TestHandleLegacyExtruderTemperatureCommandUsesActiveExtruderByDefault(t *testing.T) {
	active := &fakeCommandExtruder{name: "active", heater: "heater0"}
	runtime := &fakeExtruderTemperatureRuntime{active: active}

	if err := HandleLegacyExtruderTemperatureCommand(runtime, 210.0, 0, false); err != nil {
		t.Fatalf("unexpected temperature command error: %v", err)
	}
	if runtime.setTemperatureCall != 1 {
		t.Fatalf("expected one set-temperature call, got %d", runtime.setTemperatureCall)
	}
	if runtime.setExtruder != active {
		t.Fatalf("expected active extruder to be targeted")
	}
	if runtime.setTemp != 210.0 || runtime.setWait {
		t.Fatalf("unexpected set-temperature payload: temp=%v wait=%v", runtime.setTemp, runtime.setWait)
	}
}

func TestHandleLegacyExtruderTemperatureCommandUsesIndexedExtruderLookup(t *testing.T) {
	target := &fakeCommandExtruder{name: "extruder2", heater: "heater2"}
	runtime := &fakeExtruderTemperatureRuntime{
		active: &fakeCommandExtruder{name: "active", heater: "heater0"},
		lookup: map[string]Extruder{"extruder2": target},
	}

	if err := HandleLegacyExtruderTemperatureCommand(runtime, 225.0, 2, true); err != nil {
		t.Fatalf("unexpected temperature command error: %v", err)
	}
	if runtime.setExtruder != target {
		t.Fatalf("expected indexed extruder lookup to be targeted")
	}
	if runtime.setTemp != 225.0 || !runtime.setWait {
		t.Fatalf("unexpected set-temperature payload: temp=%v wait=%v", runtime.setTemp, runtime.setWait)
	}
}

func TestHandleLegacyExtruderTemperatureCommandIgnoresMissingIndexedExtruderWhenCoolingDown(t *testing.T) {
	runtime := &fakeExtruderTemperatureRuntime{active: &fakeCommandExtruder{name: "active", heater: "heater0"}}

	if err := HandleLegacyExtruderTemperatureCommand(runtime, 0.0, 3, false); err != nil {
		t.Fatalf("unexpected temperature command error: %v", err)
	}
	if runtime.setTemperatureCall != 0 {
		t.Fatalf("expected no heater update when missing indexed extruder is cooling down")
	}
}

func TestHandleLegacyExtruderTemperatureCommandPanicsWhenHeatingMissingIndexedExtruder(t *testing.T) {
	runtime := &fakeExtruderTemperatureRuntime{active: &fakeCommandExtruder{name: "active", heater: "heater0"}}

	defer func() {
		recovered := recover()
		if recovered != "Extruder not configured" {
			t.Fatalf("expected missing extruder panic, got %#v", recovered)
		}
	}()

	_ = HandleLegacyExtruderTemperatureCommand(runtime, 200.0, 1, false)
}

func TestBuildPressureAdvanceScanPlanDisablesScanWindowWhenAdvanceIsNil(t *testing.T) {
	plan := BuildPressureAdvanceScanPlan(0.05, 0.08, nil, 0.12)

	if plan.PreviousDelay != 0.04 {
		t.Fatalf("expected previous delay 0.04, got %v", plan.PreviousDelay)
	}
	if plan.NextDelay != 0.0 {
		t.Fatalf("expected next delay 0, got %v", plan.NextDelay)
	}
	if plan.AppliedSmoothTime != 0.0 {
		t.Fatalf("expected applied smooth time 0, got %v", plan.AppliedSmoothTime)
	}
}

func TestResolveExtruderRotationDistanceUpdateFlipsNegativeDistance(t *testing.T) {
	update := ResolveExtruderRotationDistanceUpdate(-7.5, 0)

	if update.RotationDistance != 7.5 {
		t.Fatalf("expected positive rotation distance, got %v", update.RotationDistance)
	}
	if update.NextInvertDir != 1 {
		t.Fatal("expected negative rotation distance to flip invert direction")
	}
	if displayed := DisplayExtruderRotationDistance(update.RotationDistance, update.NextInvertDir, 0); displayed != -7.5 {
		t.Fatalf("expected displayed distance -7.5, got %v", displayed)
	}
}

func TestResolveLegacyExtruderSyncTargetRequiresLegacyPosition(t *testing.T) {
	target, err := ResolveLegacyExtruderSyncTarget(&fakeSyncTarget{
		fakeCommandExtruder: &fakeCommandExtruder{name: "extruder", heater: "heater0"},
		lastPosition:        12.5,
	}, "extruder")
	if err != nil {
		t.Fatalf("unexpected sync target error: %v", err)
	}
	if target.LegacyLastPosition() != 12.5 {
		t.Fatalf("expected legacy position 12.5, got %v", target.LegacyLastPosition())
	}
}

func TestResolveLegacyExtruderSyncStateReturnsPositionAndTrapq(t *testing.T) {
	state, err := ResolveLegacyExtruderSyncState(&fakeSyncTarget{
		fakeCommandExtruder: &fakeCommandExtruder{name: "extruder", heater: "heater0"},
		lastPosition:        12.5,
	}, "extruder")
	if err != nil {
		t.Fatalf("unexpected sync state error: %v", err)
	}
	if state.Position != 12.5 {
		t.Fatalf("expected sync position 12.5, got %v", state.Position)
	}
	if state.Trapq != nil {
		t.Fatalf("expected nil trapq from fake target, got %#v", state.Trapq)
	}
}

func TestResolveLegacyExtruderSyncTargetRejectsUnexpectedType(t *testing.T) {
	_, err := ResolveLegacyExtruderSyncTarget("not-an-extruder", "extruder")
	if err == nil {
		t.Fatal("expected invalid extruder error")
	}
	if err.Error() != "extruder' is not a valid extruder" {
		t.Fatalf("unexpected error %q", err.Error())
	}
}

func TestLegacyExtruderTemperatureRuntimeFuncsDelegatesToClosures(t *testing.T) {
	active := &fakeCommandExtruder{name: "active", heater: "heater0"}
	lookup := &fakeCommandExtruder{name: "extruder2", heater: "heater2"}
	setCalls := 0
	runtime := LegacyExtruderTemperatureRuntimeFuncs{
		ActiveExtruderFunc: func() Extruder { return active },
		LookupExtruderFunc: func(section string) Extruder {
			if section != "extruder2" {
				t.Fatalf("unexpected lookup section %q", section)
			}
			return lookup
		},
		SetTemperatureFunc: func(extruder Extruder, temp float64, wait bool) error {
			setCalls++
			if extruder != lookup {
				t.Fatalf("unexpected extruder %#v", extruder)
			}
			if temp != 220.0 || !wait {
				t.Fatalf("unexpected temperature payload temp=%v wait=%v", temp, wait)
			}
			return nil
		},
	}
	if runtime.ActiveExtruder() != active {
		t.Fatal("expected active extruder closure to be used")
	}
	if runtime.LookupExtruder("extruder2") != lookup {
		t.Fatal("expected lookup closure to be used")
	}
	if err := runtime.SetTemperature(lookup, 220.0, true); err != nil {
		t.Fatalf("unexpected set temperature error: %v", err)
	}
	if setCalls != 1 {
		t.Fatalf("expected one set-temperature call, got %d", setCalls)
	}
}
