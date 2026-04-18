package vibration

import (
	"goklipper/common/utils/str"
	printerpkg "goklipper/internal/pkg/printer"
)

type LIS2DW12Module struct {
	accelerometerModule
	spi AccelerometerModuleSPITransport
	mcu AccelerometerModuleMCUTransport
	oid int
}

func NewLIS2DW12Module(config printerpkg.ModuleConfig, spi AccelerometerModuleSPITransport) *LIS2DW12Module {
	self := &LIS2DW12Module{}
	NewAccelCommandHelper(config, self)
	cfg := requireAccelerometerModuleConfig(config)
	axesMap := BuildAxesMap(
		accelAxesList(cfg),
		LIS2DW12Info["SCALE_XY"].(float64),
		LIS2DW12Info["SCALE_Z"].(float64),
		"lis2dw12",
	)
	dataRate := cfg.Getint("rate", 1600, 0, 0, true)
	ValidateQueryRate(dataRate, LIS2DW12QueryRates, "lis2dw12")
	self.spi, self.mcu, self.oid = self.configure(config, AccelerometerRuntimeConfig{
		Name:     str.LastName(cfg.Get_name()),
		AxesMap:  axesMap,
		DataRate: dataRate,
	}, accelerometerModuleOptions{
		configCommand: "config_lis2dw12 oid=%d spi_oid=%d",
		queryCommand:  "query_lis2dw12 oid=%d clock=0 rest_ticks=0",
		responseName:  "lis2dw12_data",
		apiUpdate: func(float64) map[string]interface{} {
			return LIS2DW12APIUpdate(self.runtime)
		},
		apiStartStop: func(isStart bool) {
			self.runtime.APIStartStop(isStart, func() error {
				return StartLIS2DW12Measurements(self.runtime)
			}, func() {
				FinishLIS2DW12Measurements(self.runtime)
			})
		},
	}, spi, self.Build_config)
	return self
}

func (self *LIS2DW12Module) Build_config() {
	self.accelerometerModule.build_config(
		self.mcu,
		self.spi.CommandQueue(),
		self.oid,
		"query_lis2dw12 oid=%c clock=%u rest_ticks=%u",
		"query_lis2dw12_status oid=%c",
		"lis2dw12_status oid=%c clock=%u query_ticks=%u next_sequence=%hu buffered=%c fifo=%c limit_count=%hu",
	)
}

func (self *LIS2DW12Module) Read_reg(reg int) byte {
	return self.runtime.ReadRegister(LIS2DW12Registers["REG_MOD_READ"], reg)
}

func (self *LIS2DW12Module) Set_reg(reg, val int, minclock int64) error {
	return self.runtime.SetRegister(LIS2DW12Registers["REG_MOD_READ"], reg, val, minclock, "LIS2DW12")
}
