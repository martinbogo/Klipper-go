package kinematics

import "strings"

// AxisIndex converts an axis name ("x", "y", "z") into its numeric index
// (0, 1, 2). Returns -1 for any unrecognised axis name.
func AxisIndex(axis string) int {
	switch strings.ToLower(axis) {
	case "x":
		return 0
	case "y":
		return 1
	case "z":
		return 2
	default:
		return -1
	}
}

// UniqueAxisIndexes converts a slice of axis names into a deduplicated,
// ordered slice of numeric indices. Invalid names are skipped.
func UniqueAxisIndexes(axes []string) []int {
	out := make([]int, 0, len(axes))
	seen := map[int]struct{}{}
	for _, axis := range axes {
		idx := AxisIndex(axis)
		if idx < 0 {
			continue
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		out = append(out, idx)
	}
	return out
}
