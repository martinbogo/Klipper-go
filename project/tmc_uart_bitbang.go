package project

import (
	"errors"
	"fmt"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	"goklipper/common/value"
	serialpkg "goklipper/internal/pkg/serialhdl"
	tmcpkg "goklipper/internal/pkg/tmc"
	"strings"
)

type MCU_analog_mux struct {
	mcu            *MCU
	cmd_queue      interface{}
	oids           []int
	pins           []interface{}
	pin_values     []int64
	update_pin_cmd *serialpkg.CommandWrapper
}

func NewMCU_analog_mux(mcu *MCU, cmd_queue interface{}, select_pins_desc []string) *MCU_analog_mux {
	self := new(MCU_analog_mux)
	self.mcu = mcu
	self.cmd_queue = cmd_queue
	ppins := MustLookupPins(mcu.Get_printer())
	var select_pin_params = make([]map[string]interface{}, 0)
	for _, spd := range select_pins_desc {
		select_pin_params = append(select_pin_params, ppins.Lookup_pin(spd, true, false, nil))
	}
	self.oids = make([]int, 0, len(select_pin_params))
	self.pins = make([]interface{}, 0, len(select_pin_params))
	self.pin_values = make([]int64, 0, len(select_pin_params))
	for _, pp := range select_pin_params {
		self.oids = append(self.oids, self.mcu.Create_oid())
		self.pins = append(self.pins, pp["pin"])
		self.pin_values = append(self.pin_values, -1)
	}
	for i := 0; i < len(self.oids); i++ {
		self.mcu.Add_config_cmd(fmt.Sprintf("config_digital_out oid=%d pin=%s value=0 default_value=0 max_duration=0",
			self.oids[i], self.pins[i]), false, false)
	}
	self.update_pin_cmd = nil
	self.mcu.Register_config_callback(self.build_config)
	return self
}

func (self *MCU_analog_mux) build_config() {
	self.update_pin_cmd, _ = self.mcu.Lookup_command("update_digital_out oid=%c value=%c", self.cmd_queue)
}

func (self *MCU_analog_mux) Activate(instance_id []int64) {
	for i := 0; i < len(self.oids); i++ {
		oid, old, new := self.oids[i], self.pin_values[i], instance_id[i]
		if old != new {
			self.update_pin_cmd.Send([]int64{int64(oid), int64(new)}, 0, 0)
		}
	}
	self.pin_values = instance_id
}

func lookup_tmc_uart_mutex(mcu *MCU) *ReactorMutex {
	printer := mcu.Get_printer()
	cacheObj := printer.Lookup_object("tmc_uart", nil)
	var cache *tmcpkg.UARTMutexCache
	if value.IsNone(cacheObj) {
		cache = tmcpkg.NewUARTMutexCache()
		printer.Add_object("tmc_uart", cache)
	} else {
		cache = cacheObj.(*tmcpkg.UARTMutexCache)
	}
	mutex := cache.Lookup(mcu, func() interface{} {
		return printer.Get_reactor().Mutex(false)
	})
	return mutex.(*ReactorMutex)
}

const (
	TMC_BAUD_RATE     float64 = 40000
	TMC_BAUD_RATE_AVR float64 = 9000
)

type MCU_TMC_uart_bitbang struct {
	mcu              *MCU
	mutex            *ReactorMutex
	pullup           interface{}
	rx_pin           interface{}
	tx_pin           interface{}
	oid              int
	cmd_queue        interface{}
	analog_mux       *MCU_analog_mux
	resources        *tmcpkg.UARTSharedResourceRuntime
	tmcuart_send_cmd *serialpkg.CommandQueryWrapper
}

func NewMCU_TMC_uart_bitbang(rx_pin_params, tx_pin_params map[string]interface{}, select_pins_desc []string) *MCU_TMC_uart_bitbang {
	self := new(MCU_TMC_uart_bitbang)
	self.mcu = rx_pin_params["chip"].(*MCU)
	self.mutex = lookup_tmc_uart_mutex(self.mcu)
	self.pullup = rx_pin_params["pullup"]
	self.rx_pin = rx_pin_params["pin"]
	self.tx_pin = tx_pin_params["pin"]
	self.oid = self.mcu.Create_oid()
	self.cmd_queue = self.mcu.Alloc_command_queue()
	self.analog_mux = nil
	if len(select_pins_desc) != 0 {
		self.analog_mux = NewMCU_analog_mux(self.mcu, self.cmd_queue, select_pins_desc)
	}
	muxPins := []interface{}(nil)
	if self.analog_mux != nil {
		muxPins = append(muxPins, self.analog_mux.pins...)
	}
	self.resources = tmcpkg.NewUARTSharedResourceRuntime(self.rx_pin, self.tx_pin, self.mcu, muxPins, self.analog_mux)
	self.tmcuart_send_cmd = nil
	self.mcu.Register_config_callback(self.build_config)
	return self
}

func (self *MCU_TMC_uart_bitbang) build_config() {
	baud := TMC_BAUD_RATE
	mcu_type := cast.ToString(self.mcu.Get_constants()["project.MCU"])
	if strings.HasPrefix(mcu_type, "atmega") || strings.HasPrefix(mcu_type, "at90usb") {
		baud = TMC_BAUD_RATE_AVR
	}
	bit_ticks := self.mcu.Seconds_to_clock(1. / baud)
	self.mcu.Add_config_cmd(fmt.Sprintf("config_tmcuart oid=%d rx_pin=%s pull_up=%d tx_pin=%s bit_time=%d",
		self.oid, self.rx_pin, self.pullup, self.tx_pin, bit_ticks), false, false)
	self.tmcuart_send_cmd = self.mcu.Lookup_query_command(
		"tmcuart_send oid=%c write=%*s read=%c",
		"tmcuart_response oid=%c read=%*s", self.oid,
		self.cmd_queue, true)
}

func (self *MCU_TMC_uart_bitbang) register_instance(rx_pin_params, tx_pin_params map[string]interface{},
	select_pins_desc []string, addr int) ([]int64, error) {
	selectPins := make([]tmcpkg.UARTSelectPin, 0, len(select_pins_desc))
	if len(select_pins_desc) != 0 {
		ppins := MustLookupPins(self.mcu.Get_printer())
		for _, pinDesc := range select_pins_desc {
			pinParams := ppins.Parse_pin(pinDesc, true, false)
			selectPins = append(selectPins, tmcpkg.UARTSelectPin{
				Owner:  pinParams["chip"],
				Pin:    pinParams["pin"],
				Invert: cast.ToBool(pinParams["invert"]),
			})
		}
	}
	return self.resources.RegisterInstance(rx_pin_params["pin"], tx_pin_params["pin"], selectPins, addr)
}

func (self *MCU_TMC_uart_bitbang) _calc_crc8(data []int64) int64 { return tmcpkg.CalcCRC8ATM(data) }
func (self *MCU_TMC_uart_bitbang) _add_serial_bits(data []int64) []int64 {
	return tmcpkg.AddSerialBits(data)
}
func (self *MCU_TMC_uart_bitbang) _encode_read(sync, addr, reg int64) []int64 {
	return tmcpkg.EncodeUARTRead(sync, addr, reg)
}
func (self *MCU_TMC_uart_bitbang) _encode_write(sync, addr, reg, val int64) []int64 {
	return tmcpkg.EncodeUARTWrite(sync, addr, reg, val)
}
func (self *MCU_TMC_uart_bitbang) _decode_read(reg int64, data []int64) interface{} {
	if val, ok := tmcpkg.DecodeUARTRead(reg, data); ok {
		return val
	}
	return nil
}

func (self *MCU_TMC_uart_bitbang) reg_read(instance_id []int64, addr, reg int64) interface{} {
	self.resources.PrepareTransfer(instance_id)
	msg := self._encode_read(0xf5, addr, reg)
	data := []interface{}{int64(self.oid), msg, int64(10)}
	params := self.tmcuart_send_cmd.Send(data, 0, 0)
	read_params := []int64{}
	for _, x := range params.(map[string]interface{})["read"].([]int) {
		read_params = append(read_params, int64(x))
	}
	return self._decode_read(reg, read_params)
}

func (self *MCU_TMC_uart_bitbang) reg_write(instance_id []int64, addr, reg, val int64, _print_time *float64) {
	minclock := int64(0)
	if value.IsNotNone(_print_time) {
		print_time := cast.Float64(_print_time)
		minclock = self.mcu.Print_time_to_clock(print_time)
	}
	self.resources.PrepareTransfer(instance_id)
	msg := self._encode_write(0xf5, addr, reg|0x80, val)
	data := []interface{}{int64(self.oid), msg, int64(0)}
	self.tmcuart_send_cmd.Send(data, minclock, 0)
}

func Lookup_tmc_uart_bitbang(config *ConfigWrapper, max_addr int64) ([]int64, int64, *MCU_TMC_uart_bitbang, error) {
	ppins := MustLookupPins(config.Get_printer())
	rx_pin_params := ppins.Lookup_pin(cast.ToString(config.Get("uart_pin", object.Sentinel{}, true)), false, true, "tmc_uart_rx")
	tx_pin_desc := config.Get("tx_pin", value.None, true)
	var tx_pin_params map[string]interface{}
	if value.IsNone(tx_pin_desc) {
		tx_pin_params = rx_pin_params
	} else {
		tx_pin_params = ppins.Lookup_pin(cast.ToString(tx_pin_desc), false, false, "tmc_uart_tx")
	}
	if rx_pin_params["chip"] != tx_pin_params["chip"] {
		return nil, 0, nil, errors.New("TMC uart rx and tx pins must be on the same mcu")
	}
	select_pins_desc := cast.ToStringSlice(config.Getlist("select_pins", value.None, ",", 0, true))
	addr := config.Getint("uart_address", 0, 0, cast.ForceInt(max_addr), true)
	mcu_uart, ok := rx_pin_params["class"]
	if !ok || mcu_uart == nil {
		mcu_uart = NewMCU_TMC_uart_bitbang(rx_pin_params, tx_pin_params, select_pins_desc)
		rx_pin_params["class"] = mcu_uart
	}
	instance_id, _ := mcu_uart.(*MCU_TMC_uart_bitbang).register_instance(rx_pin_params, tx_pin_params, select_pins_desc, addr)
	return instance_id, int64(addr), mcu_uart.(*MCU_TMC_uart_bitbang), nil
}

type tmcUARTTransportAdapter struct {
	instanceID []int64
	bitbang    *MCU_TMC_uart_bitbang
}

func (self *tmcUARTTransportAdapter) ReadRegister(addr, reg int64) (int64, bool) {
	value := self.bitbang.reg_read(self.instanceID, addr, reg)
	if value == nil {
		return 0, false
	}
	return cast.ToInt64(value), true
}

func (self *tmcUARTTransportAdapter) WriteRegister(addr, reg, val int64, printTime *float64) {
	self.bitbang.reg_write(self.instanceID, addr, reg, val, printTime)
}

type MCU_TMC_uart struct {
	fields        *tmcpkg.FieldHelper
	runtime       *tmcpkg.UARTTransportRuntime
	mutex         *ReactorMutex
	tmc_frequency float64
	switch_addr   int64
}

var _ tmcpkg.RegisterAccess = (*MCU_TMC_uart)(nil)

func NewMCU_TMC_uart(config *ConfigWrapper, name_to_reg map[string]int64, fields *tmcpkg.FieldHelper, max_addr int64, tmc_frequency float64) *MCU_TMC_uart {
	self := new(MCU_TMC_uart)
	printer := config.Get_printer()
	name := str.LastName(config.Get_name())
	instanceID, addr, mcuUART, _ := Lookup_tmc_uart_bitbang(config, max_addr)
	self.fields = fields
	self.runtime = tmcpkg.NewUARTTransportRuntime(
		name,
		name_to_reg,
		addr,
		&tmcUARTTransportAdapter{instanceID: instanceID, bitbang: mcuUART},
		func() bool {
			return value.IsNotNone(printer.Get_start_args()["debugoutput"])
		},
	)
	self.mutex = mcuUART.mutex
	self.tmc_frequency = tmc_frequency
	self.switch_addr = int64(config.Getint("switch_addr", 0, 0, cast.ForceInt(max_addr), true))
	return self
}

func (self *MCU_TMC_uart) Get_fields() *tmcpkg.FieldHelper { return self.fields }

func (self *MCU_TMC_uart) Get_register(reg_name string) (int64, error) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	return self.runtime.GetRegister(reg_name)
}

func (self *MCU_TMC_uart) Set_register(reg_name string, val int64, print_time *float64) error {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	err := self.runtime.SetRegister(reg_name, val, print_time)
	if err != nil {
		panic(err)
	}
	return nil
}

func (self *MCU_TMC_uart) get_tmc_frequency() float64 { return self.tmc_frequency }
