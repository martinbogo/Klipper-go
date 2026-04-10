package bedmesh

import (
	"fmt"
	"math"
	"strings"
)

type BedMeshError struct {
	msg string
}

func (self *BedMeshError) Error() string {
	return self.msg
}

func Isclose(a float64, b float64, relTol float64, absTol float64) bool {
	return math.Abs(a-b) <= math.Max(relTol*math.Max(math.Abs(a), math.Abs(b)), absTol)
}

func Constrain(val float64, minVal, maxVal float64) float64 {
	return math.Min(maxVal, math.Max(minVal, val))
}

func Lerp(t, v0, v1 float64) float64 {
	return (1-t)*v0 + t*v1
}

func Within(coord []float64, minC, maxC []float64, tol float64) bool {
	return (maxC[0]+tol) >= coord[0] && coord[0] >= (minC[0]-tol) &&
		(maxC[1]+tol) >= coord[1] && coord[1] >= (minC[1]-tol)
}

func SliceMin(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minValue := values[0]
	for _, v := range values[1:] {
		if v < minValue {
			minValue = v
		}
	}
	return minValue
}

func SliceMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxValue := values[0]
	for _, v := range values[1:] {
		if v > maxValue {
			maxValue = v
		}
	}
	return maxValue
}

func MaxPoint(index int, arr [][]float64) []float64 {
	maxArr := arr[0]
	for _, val := range arr {
		if maxArr[index] < val[index] {
			maxArr = val
		}
	}
	return maxArr
}

func MinPoint(index int, arr [][]float64) []float64 {
	minArr := arr[0]
	for _, val := range arr {
		if val[index] < minArr[index] {
			minArr = val
		}
	}
	return minArr
}

// FormatPointDebugLines formats a calibration point dump as printable lines.
// The first line is the header; subsequent lines are one entry per index.
// generatedPts are the expected probe coordinates; offsets are applied before display.
func FormatPointDebugLines(generatedPts, probedPts, correctedPts [][]float64, offsets []float64) []string {
	maxLen := int(math.Max(float64(len(generatedPts)), math.Max(float64(len(probedPts)), float64(len(correctedPts)))))
	lines := make([]string, 0, maxLen+1)
	lines = append(lines, fmt.Sprintf(
		"bed_mesh: calibration point dump\nIndex | %-17s| %-25s| Corrected Point",
		"Generated Point", "Probed Point"))
	for i := 0; i < maxLen; i++ {
		genPt, probedPt, corrPt := "", "", ""
		if i < len(generatedPts) {
			length := int(math.Min(float64(len(generatedPts[i])), 2))
			var offPt []float64
			for j := 0; j < length; j++ {
				offPt = append(offPt, generatedPts[i][j]-offsets[j])
			}
			genPt = fmt.Sprintf("(%.2f, %.2f)", offPt[0], offPt[1])
		}
		if i < len(probedPts) {
			probedPt = fmt.Sprintf("(%.2f, %.2f, %.4f)", probedPts[i][0], probedPts[i][1], probedPts[i][2])
		}
		if i < len(correctedPts) {
			corrPt = fmt.Sprintf("(%.2f, %.2f, %.4f)", correctedPts[i][0], correctedPts[i][1], correctedPts[i][2])
		}
		lines = append(lines, fmt.Sprintf("  %-4d| %-17s| %-25s| %s", i, genPt, probedPt, corrPt))
	}
	return lines
}

// FormatProbedMatrixForConfig formats a 2D probed matrix as a multi-line
// string suitable for config file storage.
func FormatProbedMatrixForConfig(probedMatrix [][]float64) string {
	var b strings.Builder
	for idx, line := range probedMatrix {
		if idx != 0 {
			b.WriteString("\n")
		}
		zvalTmp := make([]string, 0, len(line))
		for _, p := range line {
			zvalTmp = append(zvalTmp, fmt.Sprintf("%.6f", p))
		}
		b.WriteString(strings.Join(zvalTmp, ", "))
	}
	return b.String()
}

// NormalizeProbedMatrixType converts an interface{} slice of interface or float
// rows into a [][]float64. Handles both [][]interface{} and [][]float64 inputs.
func NormalizeProbedMatrixType(pointsData interface{}) [][]float64 {
	switch arr := pointsData.(type) {
	case [][]interface{}:
		result := make([][]float64, len(arr))
		for i, row := range arr {
			result[i] = make([]float64, len(row))
			for j, val := range row {
				result[i][j] = val.(float64)
			}
		}
		return result
	case [][]float64:
		result := make([][]float64, len(arr))
		for i, row := range arr {
			result[i] = make([]float64, len(row))
			copy(result[i], row)
		}
		return result
	default:
		return nil
	}
}

// ExtractExcludeObjectBounds converts exclude-object polygon data
// ([]map[string]interface{} with "polygon" keys) into a bounding box.
func ExtractExcludeObjectBounds(objects []map[string]interface{}) (Vec2, Vec2, error) {
	if len(objects) == 0 {
		return Vec2{}, Vec2{}, fmt.Errorf("no exclude object data available")
	}
	points := make([]Vec2, 0, len(objects)*2)
	for _, obj := range objects {
		poly, ok := obj["polygon"].([][]float64)
		if !ok {
			return Vec2{}, Vec2{}, fmt.Errorf("invalid polygon data in exclude object")
		}
		for _, pt := range poly {
			if len(pt) < 2 {
				continue
			}
			points = append(points, Vec2{X: pt[0], Y: pt[1]})
		}
	}
	if len(points) == 0 {
		return Vec2{}, Vec2{}, fmt.Errorf("no polygon vertices found in exclude object data")
	}
	return GetPolygonMinMax(points)
}
