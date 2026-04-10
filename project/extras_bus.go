package project

import (
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/value"
	mcupkg "goklipper/internal/pkg/mcu"
	serialpkg "goklipper/internal/pkg/serialhdl"
)

type mcuBusNameResolverAdapter struct {
	mcu *MCU
}

func (self *mcuBusNameResolverAdapter) Enumerations() map[string]interface{} {
	return self.mcu.Get_enumerations()
}

func (self *mcuBusNameResolverAdapter) Constants() map[string]interface{} {
	return self.mcu.Get_constants()
}

func (self *mcuBusNameResolverAdapter) MCUName() string {
	return self.mcu.Get_name()
}

func (self *mcuBusNameResolverAdapter) ReservePin(pin string, owner string) {
	if pin == "" {
		return
	}
	ppins := MustLookupPins(self.mcu.Get_printer())
	pinResolver := ppins.Get_pin_resolver(self.mcu.Get_name())
	pinResolver.Reserve_pin(pin, owner)
}

type mcuBusCommandSenderAdapter struct {
	cmd *serialpkg.CommandWrapper
}

func (self *mcuBusCommandSenderAdapter) Send(data interface{}, minclock, reqclock int64) {
	self.cmd.Send(data, minclock, reqclock)
}

type mcuBusQuerySenderAdapter struct {
	cmd *serialpkg.CommandQueryWrapper
}

func (self *mcuBusQuerySenderAdapter) Send(data interface{}, minclock, reqclock int64) interface{} {
	return self.cmd.Send(data, minclock, reqclock)
}

func (self *mcuBusQuerySenderAdapter) SendWithPreface(preface mcupkg.BusCommandSender, prefaceData interface{}, data interface{}, minclock, reqclock int64) interface{} {
	adapter, ok := preface.(*mcuBusCommandSenderAdapter)
	if !ok {
		panic(fmt.Errorf("unexpected preface sender type %T", preface))
	}
	return self.cmd.Send_with_preface(adapter.cmd, prefaceData, data, minclock, reqclock)
}

type mcuBusCommandOwnerAdapter struct {
	mcu *MCU
}

func (self *mcuBusCommandOwnerAdapter) AddConfigCmd(cmd string, isInit, onRestart bool) {
	self.mcu.Add_config_cmd(cmd, isInit, onRestart)
}

func (self *mcuBusCommandOwnerAdapter) LookupCommand(msgformat string, cmdQueue interface{}) (mcupkg.BusCommandSender, error) {
	cmd, err := self.mcu.Lookup_command(msgformat, cmdQueue)
	if err != nil {
		return nil, err
	}
	return &mcuBusCommandSenderAdapter{cmd: cmd}, nil
}

func (self *mcuBusCommandOwnerAdapter) LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) mcupkg.BusQuerySender {
	return &mcuBusQuerySenderAdapter{cmd: self.mcu.Lookup_query_command(msgformat, respformat, oid, cmdQueue, isAsync)}
}

/**
######################################################################
# SPI
######################################################################
*/

// Helper code for working with devices connected to an project.MCU via an SPI bus
type MCU_SPI struct {
	mcu       *MCU
	oid       int
	cmd_queue interface{}
	runtime   *mcupkg.SPIBusRuntime
}

func NewMCU_SPI(mcu *MCU, bus string, pin interface{}, mode, speed int, sw_pins []interface{},
	cs_active_high bool) *MCU_SPI {
	self := new(MCU_SPI)

	self.mcu = mcu
	self.oid = mcu.Create_oid()
	plan := mcupkg.BuildSPIConfigSetupPlan(self.oid, pin, mode, speed, sw_pins, cs_active_high)
	for _, cmd := range plan.InitialConfigCmds {
		mcu.Add_config_cmd(cmd, false, false)
	}

	self.cmd_queue = mcu.Alloc_command_queue()
	self.runtime = mcupkg.NewSPIBusRuntime(
		&mcuBusCommandOwnerAdapter{mcu: mcu},
		&mcuBusNameResolverAdapter{mcu: mcu},
		self.oid,
		bus,
		plan.ConfigFormat,
		self.cmd_queue,
	)
	mcu.Register_config_callback(self.build_config)
	return self
}

func (self *MCU_SPI) setup_shutdown_msg(shutdown_seq []int) {
	self.mcu.Add_config_cmd(mcupkg.BuildSPIShutdownConfigCommand(self.mcu.Create_oid(), self.oid, shutdown_seq), false, false)
}

func (self *MCU_SPI) Get_oid() int {
	return self.oid
}

func (self *MCU_SPI) get_mcu() *MCU {
	return self.mcu
}

func (self *MCU_SPI) get_command_queue() interface{} {
	return self.cmd_queue
}

func (self *MCU_SPI) build_config() {
	self.runtime.BuildConfig()
}

func (self *MCU_SPI) Spi_send(data []int, minclock, reqclock int64) {
	self.runtime.Send(data, minclock, reqclock)
}

func (self *MCU_SPI) Spi_transfer(data []int, minclock, reqclock int64) interface{} {
	return self.runtime.Transfer(data, minclock, reqclock)
}

func (self *MCU_SPI) Spi_transfer_with_preface(preface_data, data []int,
	minclock, reqclock int64) interface{} {
	return self.runtime.TransferWithPreface(preface_data, data, minclock, reqclock)
}

// Helper to setup an spi bus from settings in a config section

func MCU_SPI_from_config(config *ConfigWrapper, mode int, pin_option string,
	default_speed int, share_type interface{},
	cs_active_high bool) (*MCU_SPI, error) {
	ppins := MustLookupPins(config.Get_printer())
	cs_pin := cast.ToString(config.Get(pin_option, object.Sentinel{}, true))
	cs_pin_params := ppins.Lookup_pin(cs_pin, false, false, share_type)
	pin := cs_pin_params["pin"]
	if cast.ToString(pin) == "None" {
		ppins.Reset_pin_sharing(cs_pin_params)
		pin = nil
	}

	// Load bus parameters
	mcu := cs_pin_params["chip"]
	speed := config.Getint("spi_speed", default_speed, 100000, 0, true)
	var bus interface{}
	var sw_pins []interface{}
	if value.IsNotNone(config.Get("spi_software_sclk_pin", nil, true)) {
		var sw_pin_names []string
		var sw_pin_params []map[string]interface{}
		for _, name := range []string{"miso", "mosi", "sclk"} {
			sw_pin_names = append(sw_pin_names, fmt.Sprintf("spi_software_%s_pin", name))
		}
		for _, name := range sw_pin_names {
			tmp := ppins.Lookup_pin(cast.ToString(config.Get(name, object.Sentinel{}, true)), false, false, share_type)
			sw_pin_params = append(sw_pin_params, tmp)
		}

		for _, pin_params := range sw_pin_params {
			if pin_params["chip"] != mcu {
				return nil, fmt.Errorf("%s: spi pins must be on same mcu",
					config.Get_name())
			}
		}
		// sw_pins = tuple([pin_params['pin'] for pin_params in sw_pin_params])
		sw_pins = make([]interface{}, 0)
		for _, pin_params := range sw_pin_params {
			_pin, ok := pin_params["pin"].(string)
			if ok {
				sw_pins = append(sw_pins, _pin)
			} else {
				logger.Debug("pin_params[\"pin\"] type should be []string")
			}
		}
		bus = nil
	} else {
		bus = config.Get("spi_bus", value.None, true)
		sw_pins = nil
	}

	// Create MCU_SPI object

	return NewMCU_SPI(mcu.(*MCU), cast.ToString(bus), pin, mode, speed, sw_pins, cs_active_high), nil
}
