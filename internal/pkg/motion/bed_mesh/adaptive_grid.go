package bedmesh

import "math"

// IsEven reports whether val is even.
func IsEven(val int) bool {
	return val%2 == 0
}

// Linspace returns count evenly spaced float64 values from start to end (inclusive).
// If count <= 1, returns a slice containing just start.
func Linspace(start, end float64, count int) []float64 {
	if count <= 1 {
		return []float64{start}
	}
	step := (end - start) / float64(count-1)
	vals := make([]float64, count)
	for i := 0; i < count; i++ {
		vals[i] = start + float64(i)*step
	}
	return vals
}

// ApplyMinMaxMargin expands a bounding box outward by margin in all directions.
func ApplyMinMaxMargin(minPt, maxPt Vec2, margin float64) (Vec2, Vec2) {
	return Vec2{X: minPt.X - margin, Y: minPt.Y - margin},
		Vec2{X: maxPt.X + margin, Y: maxPt.Y + margin}
}

// ApplyMinMaxLimit clamps a bounding box to within boundsMin/boundsMax.
func ApplyMinMaxLimit(minPt, maxPt, boundsMin, boundsMax Vec2) (Vec2, Vec2) {
	limitedMin := Vec2{X: math.Max(minPt.X, boundsMin.X), Y: math.Max(minPt.Y, boundsMin.Y)}
	limitedMax := Vec2{X: math.Min(maxPt.X, boundsMax.X), Y: math.Min(maxPt.Y, boundsMax.Y)}
	return limitedMin, limitedMax
}

// ProbeGridConfig holds the parameters for adaptive probe grid generation.
type ProbeGridConfig struct {
	// MaxHDist is the maximum horizontal distance between probe points.
	MaxHDist float64
	// MaxVDist is the maximum vertical distance between probe points.
	MaxVDist float64
	// MinCount is the minimum number of probe points required along each axis.
	MinCount int
	// Algorithm is the bed mesh interpolation algorithm ("lagrange" or "bicubic").
	Algorithm string
}

// ApplyProbePointLimits enforces minimum probe counts and per-algorithm limits on xCount/yCount.
func ApplyProbePointLimits(xCount, yCount int, cfg ProbeGridConfig) (int, int) {
	if xCount < cfg.MinCount {
		xCount = cfg.MinCount
	}
	if yCount < cfg.MinCount {
		yCount = cfg.MinCount
	}

	switch cfg.Algorithm {
	case "lagrange":
		if xCount > 6 {
			xCount = 6
		}
		if yCount > 6 {
			yCount = 6
		}
	case "bicubic":
		minCnt := min(xCount, yCount)
		maxCnt := max(xCount, yCount)
		if minCnt < 4 && maxCnt > 6 {
			if xCount < 4 {
				xCount = 4
			}
			if yCount < 4 {
				yCount = 4
			}
		}
	default:
		// leave counts as-is for unknown algorithms
	}

	return xCount, yCount
}

// GenerateProbeGrid generates a serpentine probe grid for the given bounding box and config.
// Returns the x/y point counts, the ordered probe points, and the relative reference index.
func GenerateProbeGrid(minPt, maxPt Vec2, cfg ProbeGridConfig) (xCount, yCount int, probePoints []Vec2, relIdx int) {
	hDistance := maxPt.X - minPt.X
	vDistance := maxPt.Y - minPt.Y

	hSpan := cfg.MaxHDist
	if hSpan <= 0 {
		hSpan = hDistance
	}
	vSpan := cfg.MaxVDist
	if vSpan <= 0 {
		vSpan = vDistance
	}

	if hSpan <= 0 {
		hSpan = 1
	}
	if vSpan <= 0 {
		vSpan = 1
	}

	xCount = int(math.Ceil(hDistance / hSpan))
	yCount = int(math.Ceil(vDistance / vSpan))
	if xCount < 1 {
		xCount = 1
	}
	if yCount < 1 {
		yCount = 1
	}

	xCount, yCount = ApplyProbePointLimits(xCount, yCount, cfg)

	xPoints := Linspace(minPt.X, maxPt.X, xCount)
	yPoints := Linspace(minPt.Y, maxPt.Y, yCount)

	probePoints = make([]Vec2, 0, len(xPoints)*len(yPoints))
	for yIdx, y := range yPoints {
		if IsEven(yIdx) {
			for _, x := range xPoints {
				probePoints = append(probePoints, Vec2{X: x, Y: y})
			}
		} else {
			for i := len(xPoints) - 1; i >= 0; i-- {
				probePoints = append(probePoints, Vec2{X: xPoints[i], Y: y})
			}
		}
	}

	relIdx = int(math.Round(float64(xCount*yCount) / 2.0))
	if relIdx >= len(probePoints) {
		relIdx = len(probePoints) - 1
	}

	return xCount, yCount, probePoints, relIdx
}
