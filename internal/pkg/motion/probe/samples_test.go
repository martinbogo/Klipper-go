package probe

import (
	"math"
	"testing"
)

func TestMeanAndMedianPosition(t *testing.T) {
	positions := [][]float64{{1, 2, 0.30}, {3, 4, 0.10}, {5, 6, 0.20}}

	if got := MeanPosition(positions); math.Abs(got[0]-3) > 1e-9 || math.Abs(got[1]-4) > 1e-9 || math.Abs(got[2]-0.2) > 1e-9 {
		t.Fatalf("MeanPosition() = %v", got)
	}
	if got := MedianPosition(positions); math.Abs(got[0]-5) > 1e-9 || math.Abs(got[1]-6) > 1e-9 || math.Abs(got[2]-0.2) > 1e-9 {
		t.Fatalf("MedianPosition() = %v", got)
	}
}

func TestMedianPositionEvenSampleCountAveragesMiddlePair(t *testing.T) {
	positions := [][]float64{{0, 0, 0.40}, {4, 4, 0.10}, {2, 2, 0.20}, {6, 6, 0.30}}

	if got := MedianPosition(positions); math.Abs(got[0]-4) > 1e-9 || math.Abs(got[1]-4) > 1e-9 || math.Abs(got[2]-0.25) > 1e-9 {
		t.Fatalf("MedianPosition() = %v", got)
	}
}

func TestExceedsToleranceAndAccuracy(t *testing.T) {
	positions := [][]float64{{0, 0, 0.10}, {0, 0, 0.20}, {0, 0, 0.40}}

	if !ExceedsTolerance(positions, 0.25) {
		t.Fatalf("ExceedsTolerance() should report true")
	}
	if ExceedsTolerance(positions, 0.35) {
		t.Fatalf("ExceedsTolerance() should report false")
	}

	stats := Accuracy(positions)
	if math.Abs(stats.Maximum-0.40) > 1e-9 || math.Abs(stats.Minimum-0.10) > 1e-9 || math.Abs(stats.Range-0.30) > 1e-9 {
		t.Fatalf("Accuracy() extrema = %+v", stats)
	}
	if math.Abs(stats.Average-0.2333333333) > 1e-9 {
		t.Fatalf("Accuracy() average = %f", stats.Average)
	}
	if math.Abs(stats.Median-0.20) > 1e-9 {
		t.Fatalf("Accuracy() median = %f", stats.Median)
	}
	if math.Abs(stats.Sigma-0.1247219129) > 1e-9 {
		t.Fatalf("Accuracy() sigma = %f", stats.Sigma)
	}
}
