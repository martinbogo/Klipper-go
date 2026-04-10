package project

import (
	"errors"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	"goklipper/common/value"
	tmcpkg "goklipper/internal/pkg/tmc"
)

type MCU_TMC_SPI_chain struct {
	mutex   *ReactorMutex
	spi     *MCU_SPI
	runtime *tmcpkg.SPIChainRuntime
}

func NewMCU_TMC_SPI_chain(config *ConfigWrapper, chain_len int64) *MCU_TMC_SPI_chain {
	self := new(MCU_TMC_SPI_chain)
	printer := config.Get_printer()
	self.mutex = printer.Get_reactor().Mutex(false)
	share := value.None
	if chain_len > 1 {
		share = "tmc_spi_cs"
	}
	self.spi, _ = MCU_SPI_from_config(config, 3, "", 4000000, share, false)
	self.runtime = tmcpkg.NewSPIChainRuntime(
		chain_len,
		&tmcSPIBusTransportAdapter{spi: self.spi},
		func() bool {
			return value.IsNotNone(printer.Get_start_args()["debugoutput"])
		},
	)
	return self
}

type tmcSPIBusTransportAdapter struct {
	spi *MCU_SPI
}

func (self *tmcSPIBusTransportAdapter) Send(data []int, minclock, reqclock int64) {
	self.spi.Spi_send(data, minclock, reqclock)
}

func (self *tmcSPIBusTransportAdapter) Transfer(data []int, minclock, reqclock int64) string {
	params := self.spi.Spi_transfer(data, minclock, reqclock).(map[string]interface{})
	return cast.ToString(params["response"])
}

func (self *tmcSPIBusTransportAdapter) TransferWithPreface(preface_data, data []int, minclock, reqclock int64) string {
	params := self.spi.Spi_transfer_with_preface(preface_data, data, minclock, reqclock).(map[string]interface{})
	return cast.ToString(params["response"])
}

func (self *tmcSPIBusTransportAdapter) PrintTimeToClock(printTime float64) int64 {
	return self.spi.get_mcu().Print_time_to_clock(printTime)
}

func Lookup_tmc_spi_chain(config *ConfigWrapper) (*MCU_TMC_SPI_chain, int64) {
	_chain_len := config.GetintNone("chain_length", value.None, 2, 0, true)
	if value.IsNone(_chain_len) {
		return NewMCU_TMC_SPI_chain(config, 1), 1
	}
	chain_len := cast.ToInt64(_chain_len)
	ppins := MustLookupPins(config.Get_printer())
	cs_pin_params := ppins.Lookup_pin(cast.ToString(config.Get("cs_pin", object.Sentinel{}, true)), false, false, "tmc_spi_cs")
	tmc_spi := cs_pin_params["class"]
	if value.IsNone(tmc_spi) {
		cs_pin_params["class"] = NewMCU_TMC_SPI_chain(config, chain_len)
		tmc_spi = cs_pin_params["class"]
	}
	chain := tmc_spi.(*MCU_TMC_SPI_chain)
	if chain_len != chain.runtime.ChainLen() {
		panic(errors.New("TMC SPI chain must have same length"))
	}
	chain_pos := config.Getint("chain_position", 0, 1, cast.ForceInt(chain_len), true)
	if err := chain.runtime.RegisterPosition(int64(chain_pos)); err != nil {
		panic(err)
	}
	return chain, int64(chain_pos)
}

type MCU_TMC_SPI struct {
	fields  *tmcpkg.FieldHelper
	runtime *tmcpkg.SPIRegisterTransportRuntime
	mutex   *ReactorMutex
}

var _ tmcpkg.RegisterAccess = (*MCU_TMC_SPI)(nil)

func NewMCU_TMC_SPI(config *ConfigWrapper, name_to_reg map[string]int64, fields *tmcpkg.FieldHelper) *MCU_TMC_SPI {
	self := new(MCU_TMC_SPI)
	name := str.LastName(config.Get_name())
	chain, chain_pos := Lookup_tmc_spi_chain(config)
	self.fields = fields
	self.runtime = tmcpkg.NewSPIRegisterTransportRuntime(name, name_to_reg, chain_pos, chain.runtime)
	self.mutex = chain.mutex
	return self
}

func (self *MCU_TMC_SPI) Get_fields() *tmcpkg.FieldHelper { return self.fields }

func (self *MCU_TMC_SPI) Get_register(reg_name string) (int64, error) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	return self.runtime.GetRegister(reg_name)
}

func (self *MCU_TMC_SPI) Set_register(reg_name string, val int64, print_time *float64) error {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	return self.runtime.SetRegister(reg_name, val, print_time)
}

var _ tmcpkg.RegisterAccess = (*MCU_TMC2660_SPI)(nil)

type MCU_TMC2660_SPI struct {
	fields  *tmcpkg.FieldHelper
	runtime *tmcpkg.TMC2660SPITransportRuntime
	mutex   *ReactorMutex
}

func NewMCU_TMC2660_SPI(config *ConfigWrapper, name_to_reg map[string]int64, fields *tmcpkg.FieldHelper) *MCU_TMC2660_SPI {
	self := new(MCU_TMC2660_SPI)
	printer := config.Get_printer()
	self.mutex = printer.Get_reactor().Mutex(false)
	spi, _ := MCU_SPI_from_config(config, 0, "", 4000000, "", false)
	self.fields = fields
	self.runtime = tmcpkg.NewTMC2660SPITransportRuntime(
		name_to_reg,
		fields,
		&tmcSPIBusTransportAdapter{spi: spi},
		func() bool {
			return value.IsNotNone(printer.Get_start_args()["debugoutput"])
		},
	)
	return self
}

func (self *MCU_TMC2660_SPI) Get_fields() *tmcpkg.FieldHelper { return self.fields }

func (self *MCU_TMC2660_SPI) Get_register(reg_name string) (int64, error) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	return self.runtime.GetRegister(reg_name)
}

func (self *MCU_TMC2660_SPI) Set_register(reg_name string, val int64, print_time *float64) error {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	return self.runtime.SetRegister(reg_name, val, print_time)
}
