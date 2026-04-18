package vibration

type lis2dw12SampleRuntime interface {
	ExtractSamples(rawSamples []map[string]interface{}, bytesPerSample, samplesPerBlock, scaleDivisor int, decode func([]int) (int, int, int, bool)) [][]float64
}

// LIS2DW12 register addresses.
var LIS2DW12Registers = map[string]int{
	"REG_DEVID":     0x0F,
	"REG_CTRL1":     0x20,
	"REG_CTRL6":     0x25,
	"REG_FIFO_CTRL": 0x2E,
	"REG_MOD_READ":  0x80,
}

// LIS2DW12 supported query rates.
var LIS2DW12QueryRates = map[int]int{
	25: 0x3, 50: 0x4, 100: 0x5, 200: 0x6, 400: 0x7,
	800: 0x8, 1600: 0x9,
}

// LIS2DW12 clock/sample parameters.
var LIS2DW12Clk = map[string]float64{
	"MIN_MSG_TIME":      0.100,
	"BYTES_PER_SAMPLE":  6,
	"SAMPLES_PER_BLOCK": 8,
}

// LIS2DW12 device identification and calibration constants.
var LIS2DW12Info = map[string]interface{}{
	"DEV_ID":           0x44,
	"POWER_OFF":        0x00,
	"SET_CTRL1_MODE":   0x04,
	"SET_FIFO_CTL":     0xC0,
	"SET_CTRL6_ODR_FS": 0x04,
	"FREEFALL_ACCEL":   9.80665 * 1000.,
	"SCALE_XY":         0.000244140625 * 9.80665 * 1000, // 1 / 4096 (at 3.3V) mg/LSB
	"SCALE_Z":          0.000244140625 * 9.80665 * 1000, // 1 / 4096 (at 3.3V) mg/LSB
}

func ExtractLIS2DW12Samples(runtime lis2dw12SampleRuntime, rawSamples []map[string]interface{}) [][]float64 {
	return runtime.ExtractSamples(rawSamples, int(LIS2DW12Clk["BYTES_PER_SAMPLE"]), int(LIS2DW12Clk["SAMPLES_PER_BLOCK"]), 4, func(d []int) (int, int, int, bool) {
		rx := int(int16(d[0]&0xfc | ((d[3] & 0xff) << 8)))
		ry := int(int16(d[1]&0xfc | ((d[4] & 0xff) << 8)))
		rz := int(int16(d[2]&0xfc | ((d[5] & 0xff) << 8)))
		return rx, ry, rz, true
	})
}
