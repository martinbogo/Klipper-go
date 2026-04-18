package bedmesh

import (
	"fmt"
	"goklipper/common/utils/object"
	"math"
	"strconv"
	"strings"
)

type ConfigPairReader interface {
	Getintlist(option string, default1 interface{}, sep string, count int, noteValid bool) []int
	Get(option string, default1 interface{}, noteValid bool) interface{}
}

type GCodeValueReader interface {
	Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
}

type BedMeshError struct {
	msg string
}

type IntPairParseErrorKind string

const (
	IntPairParseErrorMalformed IntPairParseErrorKind = "malformed"
	IntPairParseErrorMinimum   IntPairParseErrorKind = "minimum"
	IntPairParseErrorMaximum   IntPairParseErrorKind = "maximum"
)

type IntPairParseError struct {
	Name  string
	Kind  IntPairParseErrorKind
	Limit float64
}

func (self *IntPairParseError) Error() string {
	switch self.Kind {
	case IntPairParseErrorMinimum:
		return fmt.Sprintf("%s must have a minimum of %.6g", self.Name, self.Limit)
	case IntPairParseErrorMaximum:
		return fmt.Sprintf("%s must have a maximum of %.6g", self.Name, self.Limit)
	default:
		return fmt.Sprintf("unable to parse %s", self.Name)
	}
}

func (self *BedMeshError) Error() string {
	return self.msg
}

func NormalizeIntPair(values []int, name string, minVal *float64, maxVal *float64) ([]int, error) {
	var pair []int
	switch len(values) {
	case 1:
		pair = []int{values[0], values[0]}
	case 2:
		pair = append([]int(nil), values...)
	default:
		return nil, &IntPairParseError{Name: name, Kind: IntPairParseErrorMalformed}
	}
	if minVal != nil {
		if float64(pair[0]) < *minVal || float64(pair[1]) < *minVal {
			return nil, &IntPairParseError{Name: name, Kind: IntPairParseErrorMinimum, Limit: *minVal}
		}
	}
	if maxVal != nil {
		if float64(pair[0]) > *maxVal || float64(pair[1]) > *maxVal {
			return nil, &IntPairParseError{Name: name, Kind: IntPairParseErrorMaximum, Limit: *maxVal}
		}
	}
	return pair, nil
}

func ParseIntPair(input string, name string, minVal *float64, maxVal *float64) ([]int, error) {
	trimmedInput := strings.TrimSpace(input)
	if trimmedInput == "" {
		return nil, &IntPairParseError{Name: name, Kind: IntPairParseErrorMalformed}
	}
	parts := strings.Split(trimmedInput, ",")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, &IntPairParseError{Name: name, Kind: IntPairParseErrorMalformed}
		}
		value, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, &IntPairParseError{Name: name, Kind: IntPairParseErrorMalformed}
		}
		values = append(values, value)
	}
	return NormalizeIntPair(values, name, minVal, maxVal)
}

func ParseCoordPair(input string) (float64, float64, error) {
	parts := strings.Split(strings.TrimSpace(input), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid coordinate pair")
	}
	v1, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	v2, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil || err2 != nil {
		return 0, 0, fmt.Errorf("invalid coordinate pair")
	}
	return v1, v2, nil
}

func floatLimitPtr(limit float64) *float64 {
	if limit == 0 {
		return nil
	}
	value := limit
	return &value
}

func ParseConfigIntPair(config ConfigPairReader, option string, defaultValue interface{}, minVal float64, maxVal float64) ([]int, error) {
	pair, err := NormalizeIntPair(
		config.Getintlist(option, []interface{}{defaultValue, defaultValue}, ",", 0, true),
		option,
		floatLimitPtr(minVal),
		floatLimitPtr(maxVal),
	)
	if err == nil {
		return pair, nil
	}
	if pairErr, ok := err.(*IntPairParseError); ok {
		switch pairErr.Kind {
		case IntPairParseErrorMinimum:
			return nil, fmt.Errorf("Option '%s' in section bed_mesh must have a minimum of %s",
				option, strconv.FormatFloat(minVal, 'f', -1, 64))
		case IntPairParseErrorMaximum:
			return nil, fmt.Errorf("Option '%s' in section bed_mesh must have a maximum of %s",
				option, strconv.FormatFloat(maxVal, 'f', -1, 64))
		}
	}
	return nil, fmt.Errorf("bed_mesh: malformed '%s' value: %v",
		option,
		config.Get(option, object.Sentinel{}, true))
}

func ParseGCodeIntPair(gcmd GCodeValueReader, name string, minVal *float64, maxVal *float64) ([]int, error) {
	pair, err := ParseIntPair(gcmd.Get(name, nil, "", nil, nil, nil, nil), name, minVal, maxVal)
	if err == nil {
		return pair, nil
	}
	if pairErr, ok := err.(*IntPairParseError); ok {
		switch pairErr.Kind {
		case IntPairParseErrorMinimum:
			return nil, fmt.Errorf("Parameter '%s' must have a minimum of %d", name, int(*minVal))
		case IntPairParseErrorMaximum:
			return nil, fmt.Errorf("Parameter '%s' must have a maximum of %d", name, int(*maxVal))
		}
	}
	return nil, fmt.Errorf("Unable to parse parameter '%s'", name)
}

func ParseGCodeCoord(gcmd GCodeValueReader, name string) (float64, float64, error) {
	v1, v2, err := ParseCoordPair(gcmd.Get(name, nil, "", nil, nil, nil, nil))
	if err != nil {
		return 0, 0, fmt.Errorf("Unable to parse parameter '%s'", name)
	}
	return v1, v2, nil
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

// BuildMeshSafely wraps ZMesh.Build_mesh and converts legacy panics into errors
// so project shells can keep persistence and recovery flows lightweight.
func BuildMeshSafely(zMesh *ZMesh, probedMatrix [][]float64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error: %v", r)
		}
	}()
	zMesh.Build_mesh(probedMatrix)
	return nil
}
