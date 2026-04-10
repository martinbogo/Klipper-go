package addon

import "strings"

type HomingOverride struct {
	StartPos []float64
	Axes     string
	InScript bool
}

func NewHomingOverride(startPos []float64, axes string) *HomingOverride {
	copied := make([]float64, len(startPos))
	copy(copied, startPos)
	return &HomingOverride{
		StartPos: copied,
		Axes:     strings.ToUpper(axes),
	}
}

func (self *HomingOverride) ShouldOverride(requestedAxes map[string]bool) bool {
	if len(requestedAxes) == 0 {
		return true
	}
	for _, axis := range self.Axes {
		if requestedAxes[string(axis)] {
			return true
		}
	}
	return false
}

func (self *HomingOverride) ApplyStartPosition(pos []float64) ([]float64, []int) {
	updated := make([]float64, len(pos))
	copy(updated, pos)
	homingAxes := make([]int, 0, len(self.StartPos))
	for axis, loc := range self.StartPos {
		if !isNoneFloat(loc) {
			updated[axis] = loc
			homingAxes = append(homingAxes, axis)
		}
	}
	return updated, homingAxes
}

func (self *HomingOverride) SetInScript(inScript bool) {
	self.InScript = inScript
}

func isNoneFloat(v float64) bool {
	return v != v
}
