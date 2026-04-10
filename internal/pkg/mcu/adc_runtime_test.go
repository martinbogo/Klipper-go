package mcu

import "testing"

func TestCalculateScaledADCValue(t *testing.T) {
	if value := CalculateScaledADCValue(512, 1.0/1024.0); value != 0.5 {
		t.Fatalf("unexpected scaled value %f", value)
	}
}

func TestADCRuntimeStateProcessAnalogInState(t *testing.T) {
	state := &ADCRuntimeState{InvMaxADC: 0.25, ReportClock: 50}
	lastValue := state.ProcessAnalogInState(map[string]interface{}{"value": int64(3), "next_clock": int64(250)}, func(clock int64) int64 {
		return clock + 1000
	}, func(clock int64) float64 {
		return float64(clock) / 100.0
	})
	if lastValue != [2]float64{0.75, 12.0} {
		t.Fatalf("unexpected processed value %#v", lastValue)
	}
	if state.LastValue != lastValue {
		t.Fatalf("expected state to store last value %#v, got %#v", lastValue, state.LastValue)
	}
}
