package vibration

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"goklipper/common/utils/str"
	"goklipper/common/value"
)

type TestAxis struct {
	name   string
	vibDir []float64
}

func NewTestAxis(axis string, vibDir []float64) *TestAxis {
	self := &TestAxis{}
	if value.IsNone(axis) || axis == "" {
		self.name = fmt.Sprintf("axis=%.3f,%.3f", vibDir[0], vibDir[1])
	} else {
		self.name = axis
	}

	if value.IsNone(vibDir) || len(vibDir) == 0 {
		if axis == "x" {
			self.vibDir = []float64{1.0, 0.0}
		} else {
			self.vibDir = []float64{0.0, 1.0}
		}
		return self
	}

	sum := 0.0
	for _, val := range vibDir {
		sum += val * val
	}
	if sum == 0 {
		self.vibDir = []float64{0.0, 0.0}
		return self
	}

	s := math.Sqrt(sum)
	normalized := make([]float64, len(vibDir))
	for idx, val := range vibDir {
		normalized[idx] = val / s
	}
	self.vibDir = normalized
	return self
}

func ParseAxis(rawAxis string) (*TestAxis, error) {
	if rawAxis == "" {
		return nil, nil
	}
	rawAxis = strings.ToLower(rawAxis)
	if rawAxis == "x" || rawAxis == "y" {
		return NewTestAxis(rawAxis, nil), nil
	}
	dirs := strings.Split(rawAxis, ",")
	if len(dirs) != 2 {
		return nil, fmt.Errorf("Invalid format of axis '%s'", rawAxis)
	}
	dirX, err1 := strconv.ParseFloat(strings.TrimSpace(dirs[0]), 64)
	dirY, err2 := strconv.ParseFloat(strings.TrimSpace(dirs[1]), 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("Unable to parse axis direction '%s'", rawAxis)
	}
	return NewTestAxis("", []float64{dirX, dirY}), nil
}

func (self *TestAxis) Matches(chipAxis string) bool {
	if self.vibDir[0] != 0.0 && strings.Contains(chipAxis, "x") {
		return true
	}
	if self.vibDir[1] != 0.0 && strings.Contains(chipAxis, "y") {
		return true
	}
	return false
}

func (self *TestAxis) GetName() string {
	return self.name
}

func (self *TestAxis) GetPoint(length float64) (float64, float64) {
	return self.vibDir[0] * length, self.vibDir[1] * length
}

func IsValidNameSuffix(nameSuffix string) bool {
	nameSuffix = strings.ReplaceAll(nameSuffix, "-", "")
	nameSuffix = strings.ReplaceAll(nameSuffix, "_", "")
	return str.IsAlphanum(nameSuffix)
}

func BuildFilename(base, nameSuffix string, axis *TestAxis, point []float64, chipName string) string {
	name := base
	if axis != nil {
		name += "_" + axis.GetName()
	}
	if chipName != "" {
		name += "_" + strings.ReplaceAll(chipName, " ", "_")
	}
	if len(point) == 3 {
		name += fmt.Sprintf("_%.3f_%.3f_%.3f", point[0], point[1], point[2])
	}
	name += "_" + nameSuffix
	return filepath.Join("/tmp", name+".csv")
}
