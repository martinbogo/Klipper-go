package vibration

import (
	"goklipper/common/utils/str"
	printerpkg "goklipper/internal/pkg/printer"
)

type ADXL345Module struct {
	accelerometerModule
	spi AccelerometerModuleSPITransport
	mcu AccelerometerModuleMCUTransport
	oid int
}

func NewADXL345Module(config printerpkg.ModuleConfig, spi AccelerometerModuleSPITransport) *ADXL345Module {
	self := &ADXL345Module{}
	NewAccelCommandHelper(config, self)
	cfg := requireAccelerometerModuleConfig(config)
	axesMap := BuildAxesMap(
		accelAxesList(cfg),
		ADXL345Info["SCALE_XY"].(float64),
		ADXL345Info["SCALE_Z"].(float64),
		"adxl345",
	)
	dataRate := cfg.Getint("rate", 3200, 0, 0, true)
	ValidateQueryRate(dataRate, ADXL345QueryRates, "adxl345")
	self.spi, self.mcu, self.oid = self.configure(config, AccelerometerRuntimeConfig{
		Name:     str.LastName(cfg.Get_name()),
		AxesMap:  axesMap,
		DataRate: dataRate,
	}, accelerometerModuleOptions{
		configCommand: "config_adxl345 oid=%d spi_oid=%d",
		queryCommand:  "query_adxl345 oid=%d clock=0 rest_ticks=0",
		responseName:  "adxl345_data",
		apiUpdate: func(float64) map[string]interface{} {
			return ADXL345APIUpdate(self.runtime)
		},
		apiStartStop: func(isStart bool) {
			self.runtime.APIStartStop(isStart, func() error {
				return StartADXL345Measurements(self.runtime)
			}, func() {
				FinishADXL345Measurements(self.runtime)
			})
		},
	}, spi, self.Build_config)
	return self
}

func (self *ADXL345Module) Build_config() {
	self.accelerometerModule.build_config(
		self.mcu,
		self.spi.CommandQueue(),
		self.oid,
		"query_adxl345 oid=%c clock=%u rest_ticks=%u",
		"query_adxl345_status oid=%c",
		"adxl345_status oid=%c clock=%u query_ticks=%u next_sequence=%hu buffered=%c fifo=%c limit_count=%hu",
	)
}

func (self *ADXL345Module) Read_reg(reg int) byte {
	return self.runtime.ReadRegister(ADXL345Registers["REG_MOD_READ"], reg)
}

func (self *ADXL345Module) Set_reg(reg, val int, minclock int64) error {
	return self.runtime.SetRegister(ADXL345Registers["REG_MOD_READ"], reg, val, minclock, "ADXL345")
}
