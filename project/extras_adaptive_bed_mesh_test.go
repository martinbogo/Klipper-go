package project

import (
	"math"
	"testing"
)

func nearlyEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestGetProbePointsSerpentine(t *testing.T) {
	abm := &AdaptiveBedMesh{
		maxProbeHorizontalDistance: 35,
		maxProbeVerticalDistance:   35,
		minimumAxisProbeCounts:     3,
		bedMeshConfigAlgorithm:     "lagrange",
	}
	min := vec2{X: 0, Y: 0}
	max := vec2{X: 100, Y: 100}

	xCount, yCount, pts, relIdx := abm.getProbePoints(min, max)
	if xCount < 3 || yCount < 3 {
		t.Fatalf("expected minimum probe counts of 3, got %d x %d", xCount, yCount)
	}
	if xCount > 6 || yCount > 6 {
		t.Fatalf("lagrange algorithm should cap probe counts at 6, got %d x %d", xCount, yCount)
	}

	expectedLen := xCount * yCount
	if len(pts) != expectedLen {
		t.Fatalf("unexpected number of probe points: %d", len(pts))
	}
	if relIdx < 0 || relIdx >= expectedLen {
		t.Fatalf("relative index out of range: %d (len=%d)", relIdx, expectedLen)
	}

	if xCount >= 2 && yCount >= 2 {
		firstRow := pts[:xCount]
		secondRow := pts[xCount : 2*xCount]
		if !nearlyEqual(firstRow[0].X, min.X, 1e-6) || !nearlyEqual(firstRow[xCount-1].X, max.X, 1e-6) {
			t.Fatalf("unexpected first row ordering: %v", firstRow)
		}
		if !nearlyEqual(secondRow[0].X, firstRow[xCount-1].X, 1e-6) || !nearlyEqual(secondRow[xCount-1].X, firstRow[0].X, 1e-6) {
			t.Fatalf("expected serpentine ordering, got %v", secondRow)
		}
	}
}

func TestApplyProbePointLimits(t *testing.T) {
	abm := &AdaptiveBedMesh{
		minimumAxisProbeCounts: 3,
		bedMeshConfigAlgorithm: "lagrange",
	}
	x, y := abm.applyProbePointLimits(10, 8)
	if x != 6 || y != 6 {
		t.Fatalf("lagrange limits should clamp counts to 6, got %d x %d", x, y)
	}

	abm.bedMeshConfigAlgorithm = "bicubic"
	x, y = abm.applyProbePointLimits(2, 8)
	if x != 4 || y != 8 {
		t.Fatalf("bicubic should raise shorter side when opposite exceeds 6, got %d x %d", x, y)
	}
}
