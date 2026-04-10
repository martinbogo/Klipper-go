package bedmesh

import (
	"fmt"
	"math"
)

// NormalizeFaultyRegionCorners normalizes start/end coordinates to c1 (min corner)
// and c3 (max corner), then derives c2 and c4.
//
//	c4 ---- c3
//	|        |
//	c1 ---- c2
func NormalizeFaultyRegionCorners(start, end []float64) (c1, c3, c2, c4 []float64, err error) {
	if len(start) < 2 || len(end) < 2 {
		return nil, nil, nil, nil, fmt.Errorf("faulty region coordinates must have at least 2 elements")
	}
	length := int(math.Min(float64(len(start)), float64(len(end))))
	c1 = make([]float64, length)
	c3 = make([]float64, length)
	for j := 0; j < length; j++ {
		c1[j] = math.Min(start[j], end[j])
		c3[j] = math.Max(start[j], end[j])
	}
	c2 = []float64{c1[0], c3[1]}
	c4 = []float64{c3[0], c1[1]}
	return c1, c3, c2, c4, nil
}

// ValidateFaultyRegionOverlap checks that newRegion (described by c1/c3/c2/c4) does not
// overlap any region already in existingRegions. regionIndex is the 1-based index used
// in error messages.
func ValidateFaultyRegionOverlap(c1, c3, c2, c4 []float64, existingRegions [][][]float64, regionIndex int) error {
	for j, region := range existingRegions {
		prevC1 := region[0]
		prevC3 := region[1]
		prevC2 := []float64{prevC1[0], prevC3[1]}
		prevC4 := []float64{prevC3[0], prevC1[1]}

		// Validate that no existing corner is within the new region.
		for _, coord := range [][]float64{prevC1, prevC2, prevC3, prevC4} {
			if Within(coord, c1, c3, 0.0) {
				return fmt.Errorf(
					"bed_mesh: Existing faulty_region_%d %v overlaps added faulty_region_%d %v",
					j+1, [][]float64{prevC1, prevC3},
					regionIndex, [][]float64{c1, c3})
			}
		}
		// Validate that no new corner is within an existing region.
		for _, coord := range [][]float64{c1, c2, c3, c4} {
			if Within(coord, prevC1, prevC3, 0.0) {
				return fmt.Errorf(
					"bed_mesh: Added faulty_region_%d %v overlaps existing faulty_region_%d %v",
					regionIndex, [][]float64{c1, c3},
					j+1, [][]float64{prevC1, prevC3})
			}
		}
	}
	return nil
}
