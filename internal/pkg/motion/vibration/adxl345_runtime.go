package vibration

import (
	"fmt"
	"goklipper/common/logger"
)

type adxl345Runtime interface {
	adxl345SampleRuntime
	IsMeasuring() bool
	Name() string
	ReadRegister(readModifier int, reg int) byte
	SetRegister(readModifier int, reg, val int, minclock int64, label string) error
	StartMeasurements(minMsgTime float64, initialize func() error, syncClock func(int64) error) error
	FinishMeasurements()
	UpdateClock(label string, bytesPerSample, samplesPerBlock int, minclock int64) error
	APIUpdate(updateClock func(int64) error, extractSamples func([]map[string]interface{}) [][]float64) map[string]interface{}
}

func UpdateADXL345Clock(runtime adxl345Runtime, minclock int64) error {
	return runtime.UpdateClock(
		"adxl345",
		int(ADXL345Clk["BYTES_PER_SAMPLE"]),
		int(ADXL345Clk["SAMPLES_PER_BLOCK"]),
		minclock,
	)
}

func ADXL345APIUpdate(runtime adxl345Runtime) map[string]interface{} {
	return runtime.APIUpdate(func(minclock int64) error {
		return UpdateADXL345Clock(runtime, minclock)
	}, func(rawSamples []map[string]interface{}) [][]float64 {
		return ExtractADXL345Samples(runtime, rawSamples)
	})
}

func StartADXL345Measurements(runtime adxl345Runtime) error {
	if runtime.IsMeasuring() {
		return nil
	}
	if err := runtime.StartMeasurements(ADXL345Clk["MIN_MSG_TIME"], func() error {
		return initializeADXL345(runtime)
	}, func(minclock int64) error {
		return UpdateADXL345Clock(runtime, minclock)
	}); err != nil {
		return err
	}
	logger.Debugf("ADXL345 starting '%s' measurements", runtime.Name())
	return nil
}

func FinishADXL345Measurements(runtime adxl345Runtime) {
	if !runtime.IsMeasuring() {
		return
	}
	runtime.FinishMeasurements()
	logger.Debugf("ADXL345 finished '%s' measurements", runtime.Name())
}

func initializeADXL345(runtime adxl345Runtime) error {
	devID := runtime.ReadRegister(ADXL345Registers["REG_MOD_READ"], ADXL345Registers["REG_DEVID"])
	if devID != byte(ADXL345Info["DEV_ID"].(int)) {
		panic(fmt.Errorf("Invalid adxl345 id (got %x vs %x).\n"+
			"This is generally indicative of connection problems\n"+
			"(e.g. faulty wiring) or a faulty adxl345 chip.",
			devID, byte(ADXL345Info["DEV_ID"].(int))))
	}
	if err := runtime.SetRegister(ADXL345Registers["REG_MOD_READ"], ADXL345Registers["REG_POWER_CTL"], 0x00, 0, "ADXL345"); err != nil {
		return err
	}
	if err := runtime.SetRegister(ADXL345Registers["REG_MOD_READ"], ADXL345Registers["REG_DATA_FORMAT"], 0x0B, 0, "ADXL345"); err != nil {
		return err
	}
	if err := runtime.SetRegister(ADXL345Registers["REG_MOD_READ"], ADXL345Registers["REG_FIFO_CTL"], 0x00, 0, "ADXL345"); err != nil {
		return err
	}
	if err := runtime.SetRegister(ADXL345Registers["REG_MOD_READ"], ADXL345Registers["REG_BW_RATE"], ADXL345QueryRates[runtimeDataRate(runtime)], 0, "ADXL345"); err != nil {
		return err
	}
	if err := runtime.SetRegister(ADXL345Registers["REG_MOD_READ"], ADXL345Registers["REG_FIFO_CTL"], ADXL345Info["SET_FIFO_CTL"].(int), 0, "ADXL345"); err != nil {
		return err
	}
	return nil
}

func runtimeDataRate(runtime adxl345Runtime) int {
	if typed, ok := runtime.(interface{ DataRate() int }); ok {
		return typed.DataRate()
	}
	panic("adxl345 runtime missing DataRate")
}
