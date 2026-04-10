package vibration

import (
	"math"
	"testing"
)

func TestNewTestAxisNormalizesDirection(t *testing.T) {
	axis := NewTestAxis("", []float64{3, 4})

	dx, dy := axis.GetPoint(5)
	if math.Abs(dx-3) > 1e-9 || math.Abs(dy-4) > 1e-9 {
		t.Fatalf("GetPoint() = (%f, %f), want (3, 4)", dx, dy)
	}
	if axis.GetName() != "axis=3.000,4.000" {
		t.Fatalf("GetName() = %q", axis.GetName())
	}
}

func TestParseAxisSupportsNamedAndVectorAxes(t *testing.T) {
	axis, err := ParseAxis("x")
	if err != nil {
		t.Fatalf("ParseAxis(x) error: %v", err)
	}
	if axis.GetName() != "x" {
		t.Fatalf("ParseAxis(x) name = %q", axis.GetName())
	}
	if !axis.Matches("xy") {
		t.Fatalf("ParseAxis(x) should match x chip axis")
	}

	axis, err = ParseAxis("3,4")
	if err != nil {
		t.Fatalf("ParseAxis(3,4) error: %v", err)
	}
	dx, dy := axis.GetPoint(10)
	if math.Abs(dx-6) > 1e-9 || math.Abs(dy-8) > 1e-9 {
		t.Fatalf("vector axis GetPoint() = (%f, %f), want (6, 8)", dx, dy)
	}
}

func TestIsValidNameSuffixAndBuildFilename(t *testing.T) {
	if !IsValidNameSuffix("run_2024-01") {
		t.Fatalf("IsValidNameSuffix() rejected valid suffix")
	}
	if IsValidNameSuffix("bad/name") {
		t.Fatalf("IsValidNameSuffix() accepted invalid suffix")
	}

	axis := NewTestAxis("x", nil)
	got := BuildFilename("raw_data", "test_01", axis, []float64{1, 2, 3}, "adxl345 toolhead")
	want := "/tmp/raw_data_x_adxl345_toolhead_1.000_2.000_3.000_test_01.csv"
	if got != want {
		t.Fatalf("BuildFilename() = %q, want %q", got, want)
	}
}
