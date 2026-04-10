package vibration

import "goklipper/common/utils/maths"

// SampleDecodeParams holds the per-chip state needed to decode a batch of raw samples.
type SampleDecodeParams struct {
	AxesMap         [][]float64
	LastSequence    int
	ClockSync       *ClockSyncRegression
	BytesPerSample  int
	SamplesPerBlock int
	// ScaleDivisor is applied after axis mapping (e.g. 4.0 for LIS2DW12, 1.0 for ADXL345).
	ScaleDivisor float64
}

// SampleDecodeResult holds extracted time-stamped samples and any error count increment.
type SampleDecodeResult struct {
	Samples    [][]float64
	ErrorCount int
}

// UnpackXYZ is a chip-specific byte unpacker.
// It receives one sample's raw bytes (as int slice) and returns (rawX, rawY, rawZ, valid).
// Return valid=false to skip a sample and increment the error counter.
type UnpackXYZ func(d []int) (int, int, int, bool)

// ExtractAccelSamples decodes raw accelerometer message data into time-stamped [t, x, y, z]
// sample slices. Chip-specific byte unpacking is provided via the unpack callback.
func ExtractAccelSamples(rawSamples []map[string]interface{}, p SampleDecodeParams, unpack UnpackXYZ) SampleDecodeResult {
	x_pos := int(p.AxesMap[0][0])
	x_scale := p.AxesMap[0][1]
	y_pos := int(p.AxesMap[1][0])
	y_scale := p.AxesMap[1][1]
	z_pos := int(p.AxesMap[2][0])
	z_scale := p.AxesMap[2][1]
	divisor := p.ScaleDivisor
	if divisor == 0 {
		divisor = 1
	}
	lastSequence := p.LastSequence
	time_base, chip_base, inv_freq := p.ClockSync.Get_time_translation()

	count := 0
	seq := 0
	errorCount := 0
	samples := make([][]float64, len(rawSamples)*p.SamplesPerBlock)
	var i int
	for _, params := range rawSamples {
		seq_diff := (lastSequence - int(params["sequence"].(int64))) & 0xffff
		seq_diff -= (seq_diff & 0x8000) << 1
		seq = lastSequence - seq_diff
		d := params["data"].([]int)
		msg_cdiff := float64(seq)*float64(p.SamplesPerBlock) - chip_base
		for i = 0; i < len(d)/p.BytesPerSample; i++ {
			d_xyz := d[i*p.BytesPerSample : (i+1)*p.BytesPerSample]
			rx, ry, rz, valid := unpack(d_xyz)
			if !valid {
				errorCount++
				continue
			}
			raw_xyz := [3]int{rx, ry, rz}
			x := maths.Round(float64(raw_xyz[x_pos])*x_scale/divisor, 6)
			y := maths.Round(float64(raw_xyz[y_pos])*y_scale/divisor, 6)
			z := maths.Round(float64(raw_xyz[z_pos])*z_scale/divisor, 6)
			ptime := maths.Round(time_base+(msg_cdiff+float64(i))*inv_freq, 6)
			samples[count] = []float64{ptime, x, y, z}
			count++
		}
	}
	p.ClockSync.Set_last_chip_clock(float64(seq*p.SamplesPerBlock + i))
	return SampleDecodeResult{
		Samples:    samples[:count],
		ErrorCount: errorCount,
	}
}
