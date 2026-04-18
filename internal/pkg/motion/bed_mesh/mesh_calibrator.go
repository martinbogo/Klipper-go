package bedmesh

import (
	"fmt"
	"goklipper/common/logger"
	"math"
	"strings"
)

type FaultyRegion struct {
	Min []float64
	Max []float64
}

type PointSubstitution struct {
	Index  int
	Points [][]float64
}

type PointGenerationResult struct {
	Points        [][]float64
	Substitutions []PointSubstitution
}

type CalibrationFinalizeConfig struct {
	MeshConfig             map[string]interface{}
	RelativeReferenceIndex *int
	Radius                 *float64
	GeneratedPoints        [][]float64
	Substitutions          []PointSubstitution
}

type CalibrationFinalizeResult struct {
	CorrectedPositions [][]float64
	ProbedMatrix       [][]float64
	MeshParams         map[string]interface{}
}

func clonePoint(point []float64) []float64 {
	return append([]float64(nil), point...)
}

func cloneMeshParams(meshConfig map[string]interface{}) map[string]interface{} {
	params := make(map[string]interface{}, len(meshConfig)+4)
	for key, item := range meshConfig {
		params[key] = item
	}
	return params
}

func roundProbePositions(positions [][]float64) [][]float64 {
	rounded := make([][]float64, 0, len(positions))
	for _, pos := range positions {
		if len(pos) < 3 {
			rounded = append(rounded, append([]float64(nil), pos...))
			continue
		}
		rounded = append(rounded, []float64{
			math.Round(pos[0]*100) / 100,
			math.Round(pos[1]*100) / 100,
			pos[2],
		})
	}
	return rounded
}

func FinalizeCalibration(offsets []float64, positions [][]float64, cfg CalibrationFinalizeConfig) (CalibrationFinalizeResult, error) {
	result := CalibrationFinalizeResult{}
	if len(positions) == 0 {
		return result, fmt.Errorf("bed_mesh: no probe positions provided")
	}

	result.CorrectedPositions = roundProbePositions(positions)
	result.MeshParams = cloneMeshParams(cfg.MeshConfig)
	result.MeshParams["min_x"] = MinPoint(0, result.CorrectedPositions)[0] + offsets[0]
	result.MeshParams["max_x"] = MaxPoint(0, result.CorrectedPositions)[0] + offsets[0]
	result.MeshParams["min_y"] = MinPoint(1, result.CorrectedPositions)[1] + offsets[1]
	result.MeshParams["max_y"] = MaxPoint(1, result.CorrectedPositions)[1] + offsets[1]

	xCount, ok := result.MeshParams["x_count"].(int)
	if !ok {
		return result, fmt.Errorf("bed_mesh: invalid x_count mesh config type %T", result.MeshParams["x_count"])
	}
	yCount, ok := result.MeshParams["y_count"].(int)
	if !ok {
		return result, fmt.Errorf("bed_mesh: invalid y_count mesh config type %T", result.MeshParams["y_count"])
	}

	if len(cfg.Substitutions) > 0 {
		corrected, err := ProcessFaultySubstitutions(cfg.GeneratedPoints, cfg.Substitutions, result.CorrectedPositions, offsets)
		result.CorrectedPositions = corrected
		if err != nil {
			return result, err
		}
	}

	probedMatrix, err := AssembleProbedMatrix(
		result.CorrectedPositions,
		offsets[2],
		xCount,
		yCount,
		cfg.RelativeReferenceIndex,
		cfg.Radius,
	)
	if err != nil {
		return result, err
	}

	result.ProbedMatrix = probedMatrix
	return result, nil
}

func GenerateProbePoints(radius *float64, origin []float64, meshMin []float64, meshMax []float64, xCount int, yCount int, faultyRegions []FaultyRegion) (PointGenerationResult, error) {
	result := PointGenerationResult{}
	if xCount < 1 || yCount < 1 {
		return result, fmt.Errorf("bed_mesh: invalid probe count %d x %d", xCount, yCount)
	}
	minX := meshMin[0]
	minY := meshMin[1]
	maxX := meshMax[0]
	maxY := meshMax[1]
	xDist := (maxX - minX) / (float64(xCount) - 1)
	yDist := (maxY - minY) / (float64(yCount) - 1)
	// floor distances down to next hundredth
	xDist = math.Floor(xDist*100) / 100
	yDist = math.Floor(yDist*100) / 100
	if xDist < 1. || yDist < 1. {
		return result, fmt.Errorf("bed_mesh: min/max points too close together")
	}
	if radius != nil && *radius != 0 {
		yDist = xDist
		newR := float64(xCount) / 2.0 * xDist
		minY = -newR
		minX = minY
		maxY = newR
		maxX = maxY
	}

	posY := minY
	points := make([][]float64, 0, xCount*yCount)
	for i := 0; i < yCount; i++ {
		for j := 0; j < xCount; j++ {
			var posX float64
			if i%2 == 0 {
				posX = minX + float64(j)*xDist
			} else {
				posX = maxX - float64(j)*xDist
			}
			if radius == nil || *radius == 0 {
				points = append(points, []float64{posX, posY})
				continue
			}
			distFromOrigin := math.Sqrt(posX*posX + posY*posY)
			if distFromOrigin <= *radius {
				points = append(points, []float64{origin[0] + posX, origin[1] + posY})
			}
		}
		posY += yDist
	}
	result.Points = points
	if len(points) == 0 || len(faultyRegions) == 0 {
		return result, nil
	}

	lastY := points[0][1]
	isReversed := false
	for i, coord := range points {
		if !Isclose(coord[1], lastY, 1e-09, 0.0) {
			isReversed = !isReversed
		}
		lastY = coord[1]

		var adjCoords [][]float64
		for _, region := range faultyRegions {
			if Within(coord, region.Min, region.Max, .00001) {
				adjCoords = [][]float64{
					{region.Min[0], coord[1]},
					{coord[0], region.Min[1]},
					{coord[0], region.Max[1]},
					{region.Max[0], coord[1]},
				}
				if isReversed {
					adjCoords[0], adjCoords[len(adjCoords)-1] = adjCoords[len(adjCoords)-1], adjCoords[0]
				}
				break
			}
		}
		if len(adjCoords) == 0 {
			continue
		}

		validCoords := make([][]float64, 0, len(adjCoords))
		for _, ac := range adjCoords {
			if radius == nil || *radius == 0 {
				if Within(ac, []float64{minX, minY}, []float64{maxX, maxY}, .000001) {
					validCoords = append(validCoords, clonePoint(ac))
				}
				continue
			}
			dx := ac[0]
			dy := ac[1]
			if len(origin) >= 2 {
				dx -= origin[0]
				dy -= origin[1]
			}
			distFromOrigin := math.Sqrt(dx*dx + dy*dy)
			if distFromOrigin <= *radius {
				validCoords = append(validCoords, clonePoint(ac))
			}
		}
		if len(validCoords) == 0 {
			return PointGenerationResult{}, fmt.Errorf("bed_mesh: unable to generate coordinates for faulty region at index: %d", i)
		}
		result.Substitutions = append(result.Substitutions, PointSubstitution{Index: i, Points: validCoords})
	}

	return result, nil
}

func NormalizeMeshAlgorithm(algo string, xPps int, yPps int, xCount int, yCount int) (string, bool, error) {
	algo = strings.ToLower(strings.TrimSpace(algo))
	switch algo {
	case "lagrange", "bicubic", "direct":
	default:
		return "", false, fmt.Errorf("bed_mesh: Unknown algorithm <%s>", algo)
	}

	maxProbeCount := xCount
	if yCount > maxProbeCount {
		maxProbeCount = yCount
	}
	minProbeCount := xCount
	if yCount < minProbeCount {
		minProbeCount = yCount
	}
	maxPps := xPps
	if yPps > maxPps {
		maxPps = yPps
	}

	if maxPps == 0 {
		return "direct", algo != "direct", nil
	}
	if algo == "lagrange" && maxProbeCount > 6 {
		return "", false, fmt.Errorf("bed_mesh: cannot exceed a probe_count of 6 when using lagrange interpolation. Configured Probe Count: %d, %d", xCount, yCount)
	}
	if algo == "bicubic" && minProbeCount < 4 {
		if maxProbeCount > 6 {
			return "", false, fmt.Errorf("bed_mesh: invalid probe_count option when using bicubic interpolation.  Combination of 3 points on one axis with more than 6 on another is not permitted. Configured Probe Count: %d, %d", xCount, yCount)
		}
		return "lagrange", true, nil
	}
	return algo, false, nil
}

// GetAdjustedPoints rebuilds the probe-point array by substituting
// faulty-region replacement points for the original points.
func GetAdjustedPoints(points [][]float64, substitutions []PointSubstitution) [][]float64 {
	if len(substitutions) == 0 {
		return points
	}
	adjPts := [][]float64{}
	lastIndex := 0
	for _, substitution := range substitutions {
		i := substitution.Index
		pts := substitution.Points
		adjPts = append(adjPts, points[lastIndex:i]...)
		adjPts = append(adjPts, pts...)
		lastIndex = i + 1
	}
	adjPts = append(adjPts, points[lastIndex:]...)
	return adjPts
}

func FindProbePointIndex(points [][]float64, target Vec2) int {
	for i, point := range points {
		if len(point) < 2 {
			continue
		}
		if Isclose(point[0], target.X, 1e-04, 1e-06) && Isclose(point[1], target.Y, 1e-04, 1e-06) {
			return i
		}
	}
	return -1
}

// AssembleProbedMatrix converts a sorted list of probe positions
// into a 2-D row-major matrix.
func AssembleProbedMatrix(
	positions [][]float64,
	zOffset float64,
	xCount, yCount int,
	relativeRefIndex *int,
	radius *float64,
) ([][]float64, error) {
	if relativeRefIndex != nil {
		zOffset = positions[*relativeRefIndex][2]
	}

	var probedMatrix [][]float64
	var row []float64
	prevPos := positions[0]
	for _, pos := range positions {
		if !Isclose(pos[1], prevPos[1], 1e-09, .1) {
			probedMatrix = append(probedMatrix, row)
			row = []float64{}
		}
		if pos[0] > prevPos[0] {
			row = append(row, pos[2]-zOffset)
		} else {
			row = append([]float64{pos[2] - zOffset}, row...)
		}
		prevPos = pos
	}
	probedMatrix = append(probedMatrix, row)

	if len(probedMatrix) != yCount {
		return nil, fmt.Errorf("bed_mesh: Invalid y-axis table length\nProbed table length: %d Probed Table:\n%v",
			len(probedMatrix), probedMatrix)
	}

	if radius != nil && *radius > 0 {
		for _, row := range probedMatrix {
			rowSize := len(row)
			if rowSize&1 == 0 {
				return nil, fmt.Errorf("bed_mesh: incorrect number of points sampled on X\nProbed Table:\n%v", probedMatrix)
			}
			bufCnt := xCount - rowSize/2
			if bufCnt == 0 {
				continue
			}
			leftBuffer := make([]float64, bufCnt)
			rightBuffer := make([]float64, bufCnt)
			for i := 0; i < bufCnt; i++ {
				leftBuffer[i] = row[0]
				rightBuffer[i] = row[rowSize-1]
			}
			row = append(leftBuffer, row...)
			row = append(row, rightBuffer...)
		}
	}

	for _, row := range probedMatrix {
		if len(row) != xCount {
			return nil, fmt.Errorf("bed_mesh: invalid x-axis table length\nProbed table length: %d Probed Table:\n%v",
				len(probedMatrix), probedMatrix)
		}
	}

	return probedMatrix, nil
}

// ProcessFaultySubstitutions merges faulty-region substitute probe points back
// into a single corrected position list. It averages the Z values of all
// substitute samples, validates the corrected list length and coordinates,
// and returns the corrected positions. On failure a non-nil error is returned
// together with the partially-built corrected slice for debug display.
func ProcessFaultySubstitutions(
	generatedPts [][]float64,
	substitutions []PointSubstitution,
	positions [][]float64,
	offsets []float64,
) ([][]float64, error) {
	if len(substitutions) == 0 {
		return positions, nil
	}
	zOffset := offsets[2]
	var correctedPts [][]float64
	idxOffset := 0
	startIdx := 0
	var fpt []float64
	for _, sub := range substitutions {
		i := sub.Index
		pts := sub.Points
		length := int(math.Min(float64(len(generatedPts[i])), 2))
		fpt = nil
		for j := 0; j < length; j++ {
			fpt = append(fpt, generatedPts[i][j]-offsets[j])
		}
		idx := i + idxOffset
		correctedPts = append(correctedPts, positions[startIdx:idx]...)
		var avgZ float64
		for _, p := range positions[idx : idx+len(pts)] {
			avgZ += p[2]
		}
		avgZ /= float64(len(pts))
		idxOffset += len(pts) - 1
		startIdx = idx + len(pts)
		fpt = append(fpt, avgZ)
		logger.Debug(
			"bed_mesh: Replacing value at faulty index %d"+
				" (%.4f, %.4f): avg value = %.6f, avg w/ z_offset = %.6f",
			i, fpt[0], fpt[1], avgZ, avgZ-zOffset)
		correctedPts = append(correctedPts, fpt)
	}
	correctedPts = append(correctedPts, positions[startIdx:]...)

	if len(generatedPts) != len(correctedPts) {
		return correctedPts, &BedMeshError{msg: fmt.Sprintf(
			"bed_mesh: invalid position list size, generated count: %d, probed count: %d",
			len(generatedPts), len(correctedPts))}
	}
	length := int(math.Min(float64(len(generatedPts)), float64(len(correctedPts))))
	for k := 0; k < length; k++ {
		gen_pt := generatedPts[k]
		probed := correctedPts[k]
		innerLength := int(math.Min(float64(len(gen_pt)), 2.))
		var offPt []float64
		for l := 0; l < innerLength; l++ {
			offPt = append(offPt, gen_pt[l]-offsets[l])
		}
		if !Isclose(offPt[0], probed[0], 1e-09, .1) || !Isclose(offPt[1], probed[1], 1e-09, .1) {
			return correctedPts, &BedMeshError{msg: fmt.Sprintf(
				"bed_mesh: point mismatch, orig = (%.2f, %.2f), probed = (%.2f, %.2f)",
				offPt[0], offPt[1], probed[0], probed[1])}
		}
	}
	return correctedPts, nil
}
