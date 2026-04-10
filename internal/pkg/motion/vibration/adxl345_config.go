package vibration

// ADXL345 register addresses.
var ADXL345Registers = map[string]int{
	"REG_DEVID":       0x00,
	"REG_BW_RATE":     0x2C,
	"REG_POWER_CTL":   0x2D,
	"REG_DATA_FORMAT": 0x31,
	"REG_FIFO_CTL":    0x38,
	"REG_MOD_READ":    0x80,
	"REG_MOD_MULTI":   0x40,
}

// ADXL345 supported query rates.
var ADXL345QueryRates = map[int]int{
	25: 0x8, 50: 0x9, 100: 0xa, 200: 0xb, 400: 0xc,
	800: 0xd, 1600: 0xe, 3200: 0xf,
}

// ADXL345 clock/sample parameters.
var ADXL345Clk = map[string]float64{
	"MIN_MSG_TIME":      0.100,
	"BYTES_PER_SAMPLE":  5,
	"SAMPLES_PER_BLOCK": 10,
}

// ADXL345 device identification and calibration constants.
var ADXL345Info = map[string]interface{}{
	"DEV_ID":         0xe5,
	"SET_FIFO_CTL":   0x90,
	"FREEFALL_ACCEL": 9.80665 * 1000.,
	"SCALE_XY":       0.003774 * 9.80665 * 1000., // 1 / 265 (at 3.3V) mg/LSB
	"SCALE_Z":        0.003906 * 9.80665 * 1000., // 1 / 256 (at 3.3V) mg/LSB
}
