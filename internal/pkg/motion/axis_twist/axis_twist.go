package axis_twist

import "math"

// InterpolateZCompensation returns the interpolated Z compensation value
// for a given X coordinate, based on evenly-spaced compensation samples
// between startX and endX.
func InterpolateZCompensation(xCoord float64, compensations []float64, startX, endX float64) float64 {
	if len(compensations) == 0 {
		return 0
	}
	sampleCount := len(compensations)
	spacing := (endX - startX) / float64(sampleCount-1)
	interpolateT := (xCoord - startX) / spacing
	interpolateI := int(math.Floor(interpolateT))
	interpolateI = constrain(interpolateI, 0, sampleCount-2)
	interpolateT -= float64(interpolateI)
	return lerp(interpolateT, compensations[interpolateI], compensations[interpolateI+1])
}

// CalculateNozzlePoints generates evenly-spaced 2D points along the X axis
// starting at startPoint, with the given count and interval distance.
func CalculateNozzlePoints(startPoint []float64, sampleCount int, intervalDist float64) [][]float64 {
	points := make([][]float64, 0, sampleCount)
	for i := 0; i < sampleCount; i++ {
		x := startPoint[0] + float64(i)*intervalDist
		y := startPoint[1]
		points = append(points, []float64{x, y})
	}
	return points
}

// CalculateProbePoints offsets nozzle points by the given probe X/Y offsets.
func CalculateProbePoints(nozzlePoints [][]float64, probeXOffset, probeYOffset float64) [][]float64 {
	points := make([][]float64, 0, len(nozzlePoints))
	for _, pt := range nozzlePoints {
		x := pt[0] - probeXOffset
		y := pt[1] - probeYOffset
		points = append(points, []float64{x, y})
	}
	return points
}

// NormalizeCompensations subtracts the mean from each element so results
// are independent of z_offset.
func NormalizeCompensations(results []float64) (normalized []float64, avg float64) {
	sum := 0.0
	for _, v := range results {
		sum += v
	}
	avg = sum / float64(len(results))
	normalized = make([]float64, len(results))
	for i, x := range results {
		normalized[i] = avg - x
	}
	return normalized, avg
}

func constrain(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func lerp(t, a, b float64) float64 {
	return (1-t)*a + t*b
}
