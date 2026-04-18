package vibration

import (
	"fmt"
	"goklipper/common/logger"
)

type lis2dw12Runtime interface {
	lis2dw12SampleRuntime
	IsMeasuring() bool
	Name() string
	ReadRegister(readModifier int, reg int) byte
	SetRegister(readModifier int, reg, val int, minclock int64, label string) error
	StartMeasurements(minMsgTime float64, initialize func() error, syncClock func(int64) error) error
	FinishMeasurements()
	UpdateClock(label string, bytesPerSample, samplesPerBlock int, minclock int64) error
	APIUpdate(updateClock func(int64) error, extractSamples func([]map[string]interface{}) [][]float64) map[string]interface{}
}

func UpdateLIS2DW12Clock(runtime lis2dw12Runtime, minclock int64) error {
	return runtime.UpdateClock(
		"lis2dw12",
		int(LIS2DW12Clk["BYTES_PER_SAMPLE"]),
		int(LIS2DW12Clk["SAMPLES_PER_BLOCK"]),
		minclock,
	)
}

func LIS2DW12APIUpdate(runtime lis2dw12Runtime) map[string]interface{} {
	return runtime.APIUpdate(func(minclock int64) error {
		return UpdateLIS2DW12Clock(runtime, minclock)
	}, func(rawSamples []map[string]interface{}) [][]float64 {
		return ExtractLIS2DW12Samples(runtime, rawSamples)
	})
}

func StartLIS2DW12Measurements(runtime lis2dw12Runtime) error {
	if runtime.IsMeasuring() {
		return nil
	}
	if err := runtime.StartMeasurements(LIS2DW12Clk["MIN_MSG_TIME"], func() error {
		return initializeLIS2DW12(runtime)
	}, func(minclock int64) error {
		return UpdateLIS2DW12Clock(runtime, minclock)
	}); err != nil {
		return err
	}
	logger.Debugf("LIS2DW12 starting '%s' measurements", runtime.Name())
	return nil
}

func FinishLIS2DW12Measurements(runtime lis2dw12Runtime) {
	if !runtime.IsMeasuring() {
		return
	}
	runtime.FinishMeasurements()
	logger.Debugf("LIS2DW12 finished '%s' measurements", runtime.Name())
}

func initializeLIS2DW12(runtime lis2dw12Runtime) error {
	devID := runtime.ReadRegister(LIS2DW12Registers["REG_MOD_READ"], LIS2DW12Registers["REG_DEVID"])
	if devID != byte(LIS2DW12Info["DEV_ID"].(int)) {
		panic(fmt.Errorf("Invalid lis2dw12 id (got %x vs %x).\n"+
			"This is generally indicative of connection problems\n"+
			"(e.g. faulty wiring) or a faulty lis2dw12 chip.",
			devID, byte(LIS2DW12Info["DEV_ID"].(int))))
	}
	if err := runtime.SetRegister(
		LIS2DW12Registers["REG_MOD_READ"],
		LIS2DW12Registers["REG_CTRL1"],
		LIS2DW12QueryRates[runtimeDataRate(runtime)]<<4|LIS2DW12Info["SET_CTRL1_MODE"].(int),
		0,
		"LIS2DW12",
	); err != nil {
		return err
	}
	if err := runtime.SetRegister(LIS2DW12Registers["REG_MOD_READ"], LIS2DW12Registers["REG_FIFO_CTRL"], 0x00, 0, "LIS2DW12"); err != nil {
		return err
	}
	if err := runtime.SetRegister(LIS2DW12Registers["REG_MOD_READ"], LIS2DW12Registers["REG_CTRL6"], LIS2DW12Info["SET_CTRL6_ODR_FS"].(int), 0, "LIS2DW12"); err != nil {
		return err
	}
	if err := runtime.SetRegister(LIS2DW12Registers["REG_MOD_READ"], LIS2DW12Registers["REG_FIFO_CTRL"], LIS2DW12Info["SET_FIFO_CTL"].(int), 0, "LIS2DW12"); err != nil {
		return err
	}
	return nil
}
