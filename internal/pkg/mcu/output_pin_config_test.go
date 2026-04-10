package mcu

import "testing"

func TestBuildDigitalOutConfigPlan(t *testing.T) {
	plan := BuildDigitalOutConfigPlan(2.0, 1, 1, func(seconds float64) int64 {
		return int64(seconds * 1000)
	})
	if plan.MaxDurationTicks != 2000 {
		t.Fatalf("unexpected max duration ticks %d", plan.MaxDurationTicks)
	}
}

func TestBuildDigitalOutConfigPlanPanicsOnMismatchedValues(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on mismatched digital output values")
		}
	}()
	BuildDigitalOutConfigPlan(1.0, 1, 0, func(seconds float64) int64 { return int64(seconds * 1000) })
}

func TestBuildPWMConfigPlanHardware(t *testing.T) {
	plan := BuildPWMConfigPlan(0.0, 0.1, 0.25, 0.5, true, 255, func() float64 {
		return 10
	}, func(eventtime float64) float64 {
		return eventtime + 1
	}, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, func(seconds float64) int64 {
		return int64(seconds * 1000)
	})
	if plan.LastClock != 11200 || plan.CycleTicks != 100 || plan.MaxDurationTicks != 0 || plan.PWMMax != 255 || plan.StartConfigValue != 63 || plan.ShutdownConfigValue != 127 || plan.InitialQueueValue != 64 {
		t.Fatalf("unexpected hardware pwm config plan %#v", plan)
	}
}

func TestBuildPWMConfigPlanSoftware(t *testing.T) {
	plan := BuildPWMConfigPlan(0.0, 0.1, 0.75, 1.0, false, 0, func() float64 {
		return 5
	}, func(eventtime float64) float64 {
		return eventtime + 0.5
	}, func(printTime float64) int64 {
		return int64(printTime * 1000)
	}, func(seconds float64) int64 {
		return int64(seconds * 1000)
	})
	if plan.LastClock != 5700 || plan.CycleTicks != 100 || plan.MaxDurationTicks != 0 || plan.PWMMax != 100 || plan.StartConfigValue != 0 || plan.ShutdownConfigValue != 1 || plan.InitialQueueValue != 75 {
		t.Fatalf("unexpected software pwm config plan %#v", plan)
	}
}

func TestBuildPWMConfigPlanPanicsOnInvalidSoftShutdownValue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on invalid soft pwm shutdown value")
		}
	}()
	BuildPWMConfigPlan(2.0, 0.1, 0.75, 0.5, false, 0, func() float64 { return 0 }, func(eventtime float64) float64 { return eventtime }, func(printTime float64) int64 { return int64(printTime * 1000) }, func(seconds float64) int64 { return int64(seconds * 1000) })
}
