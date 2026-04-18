package bedmesh

import (
	"fmt"
	"math"
)

// CalcZFadeFactor returns the Z fade scaling factor for a given Z position.
// Returns 1.0 when below fade_start, linearly fades to 0.0 at fade_end,
// and returns 0.0 at or above fade_end.
func CalcZFadeFactor(z_pos, fade_start, fade_end float64) float64 {
	if z_pos >= fade_end {
		return 0.
	} else if z_pos >= fade_start {
		fade_dist := fade_end - fade_start
		return (fade_end - z_pos) / fade_dist
	}
	return 1.
}

func ResolveFadeTarget(mesh *ZMesh, fadeEnabled bool, fadeDist, baseFadeTarget float64) (float64, bool, error) {
	if mesh == nil || !fadeEnabled {
		return 0., false, nil
	}

	fadeTarget := baseFadeTarget
	if fadeTarget == 0.0 {
		fadeTarget = mesh.Avg_z
	} else {
		minZ, maxZ := mesh.Get_z_range()
		if !(minZ <= fadeTarget && fadeTarget <= maxZ) {
			return 0., false, fmt.Errorf(
				"bed_mesh: ERROR, fade_target lies outside of mesh z range\nmin: %.4f, max: %.4f, fade_target: %.4f",
				minZ, maxZ, fadeTarget,
			)
		}
	}

	minZ, maxZ := mesh.Get_z_range()
	if fadeDist <= math.Max(math.Abs(minZ), math.Abs(maxZ)) {
		return 0., false, fmt.Errorf(
			"bed_mesh:  Mesh extends outside of the fade range, please see the fade_start and fade_end options in example-extras.cfg. fade distance: %.2f mesh min: %.4f mesh max: %.4f",
			fadeDist, minZ, maxZ,
		)
	}
	return fadeTarget, true, nil
}

func CalculateUntransformedPosition(toolheadPos []float64, mesh *ZMesh, fadeStart, fadeEnd, fadeDist, fadeTarget float64) []float64 {
	pos := append([]float64(nil), toolheadPos...)
	if len(pos) < 4 {
		return pos
	}
	if mesh == nil {
		pos[2] -= fadeTarget
		return pos
	}

	x, y, z, e := pos[0], pos[1], pos[2], pos[3]
	maxAdj := mesh.Calc_z(x, y)
	factor := 1.
	zAdj := maxAdj - fadeTarget
	if math.Min(z, z-maxAdj) >= fadeEnd {
		factor = 0.
	} else if math.Max(z, z-maxAdj) >= fadeStart {
		factor = (fadeEnd + fadeTarget - z) / (fadeDist - zAdj)
		factor = Constrain(factor, 0., 1.)
	}
	finalZAdj := factor*zAdj + fadeTarget
	return []float64{x, y, z - finalZAdj, e}
}
