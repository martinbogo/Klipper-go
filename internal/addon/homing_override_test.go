package addon

import (
	"math"
	"reflect"
	"testing"
)

func TestHomingOverrideShouldOverride(t *testing.T) {
	core := NewHomingOverride([]float64{math.NaN(), math.NaN(), math.NaN()}, "XZ")

	if !core.ShouldOverride(map[string]bool{}) {
		t.Fatalf("ShouldOverride() should override when no axes are specified")
	}
	if !core.ShouldOverride(map[string]bool{"X": true}) {
		t.Fatalf("ShouldOverride() should override when configured axis is requested")
	}
	if !core.ShouldOverride(map[string]bool{"Z": true}) {
		t.Fatalf("ShouldOverride() should override for another configured axis")
	}
	if core.ShouldOverride(map[string]bool{"Y": true}) {
		t.Fatalf("ShouldOverride() should not override when only non-configured axes are requested")
	}
}

func TestHomingOverrideApplyStartPosition(t *testing.T) {
	core := NewHomingOverride([]float64{10, math.NaN(), 5}, "XYZ")
	pos, axes := core.ApplyStartPosition([]float64{1, 2, 3, 4})

	if got, want := pos, []float64{10, 2, 5, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplyStartPosition() pos = %v, want %v", got, want)
	}
	if got, want := axes, []int{0, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplyStartPosition() axes = %v, want %v", got, want)
	}
}
