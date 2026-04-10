package probe

import (
	"math"
	"sort"
)

type AccuracyStats struct {
	Maximum float64
	Minimum float64
	Range   float64
	Average float64
	Median  float64
	Sigma   float64
}

func MeanPosition(positions [][]float64) []float64 {
	if len(positions) == 0 {
		return nil
	}
	count := float64(len(positions))
	res := make([]float64, 3)
	for i := 0; i < 3; i++ {
		sum := 0.0
		for _, pos := range positions {
			sum += pos[i]
		}
		res[i] = sum / count
	}
	return res
}

func MedianPosition(positions [][]float64) []float64 {
	if len(positions) == 0 {
		return nil
	}
	zSorted := make([][]float64, len(positions))
	copy(zSorted, positions)
	sort.Slice(zSorted, func(i, j int) bool {
		return zSorted[i][2] < zSorted[j][2]
	})
	middle := len(zSorted) / 2
	if len(zSorted)&1 == 1 {
		picked := make([]float64, len(zSorted[middle]))
		copy(picked, zSorted[middle])
		return picked
	}
	return MeanPosition(zSorted[middle-1 : middle+1])
}

func ZRange(positions [][]float64) (float64, float64) {
	if len(positions) == 0 {
		return 0, 0
	}
	maxValue, minValue := positions[0][2], positions[0][2]
	for _, item := range positions[1:] {
		if item[2] > maxValue {
			maxValue = item[2]
		}
		if item[2] < minValue {
			minValue = item[2]
		}
	}
	return maxValue, minValue
}

func ExceedsTolerance(positions [][]float64, tolerance float64) bool {
	if len(positions) == 0 {
		return false
	}
	maxValue, minValue := ZRange(positions)
	return (maxValue - minValue) > tolerance
}

func Accuracy(positions [][]float64) AccuracyStats {
	if len(positions) == 0 {
		return AccuracyStats{}
	}
	maxValue, minValue := ZRange(positions)
	avgValue := MeanPosition(positions)[2]
	median := MedianPosition(positions)[2]
	deviationSum := 0.0
	for _, pos := range positions {
		deviationSum += math.Pow(pos[2]-avgValue, 2.)
	}
	return AccuracyStats{
		Maximum: maxValue,
		Minimum: minValue,
		Range:   maxValue - minValue,
		Average: avgValue,
		Median:  median,
		Sigma:   math.Pow(deviationSum/float64(len(positions)), 0.5),
	}
}
