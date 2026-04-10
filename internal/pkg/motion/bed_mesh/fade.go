package bedmesh

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
