package project

import (
	"errors"
	"fmt"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	reportpkg "goklipper/internal/pkg/motion/report"
	vibrationpkg "goklipper/internal/pkg/motion/vibration"
	serialpkg "goklipper/internal/pkg/serialhdl"
	"log"
	"strings"
	"sync"
)

// LIS2DW12 register tables are defined in vibrationpkg.
var LIS2DW12_REGISTERS = vibrationpkg.LIS2DW12Registers
var LIS2DW12_QUERY_RATES = vibrationpkg.LIS2DW12QueryRates
var LIS2DW12_CLK = vibrationpkg.LIS2DW12Clk
var LIS2DW12_INFO = vibrationpkg.LIS2DW12Info

// Printer class that controls LIS2DW12 chip
type LIS2DW12 struct {
	printer                 *Printer
	query_rate              int
	axes_map                [][]float64
	data_rate               int
	lock                    sync.Mutex
	raw_samples             []map[string]interface{}
	spi                     *MCU_SPI
	mcu                     *MCU
	oid                     int
	query_sensor_cmd        *serialpkg.CommandWrapper
	query_sensor_end_cmd    *serialpkg.CommandQueryWrapper
	query_sensor_status_cmd *serialpkg.CommandQueryWrapper
	last_sequence           int
	max_query_duration      int64
	last_limit_count        int
	last_error_count        int
	clock_sync              *vibrationpkg.ClockSyncRegression
	api_dump                *reportpkg.APIDumpHelper
	name                    string
}

func NewLIS2DW12(config *ConfigWrapper) *LIS2DW12 {
	self := new(LIS2DW12)
	self.printer = config.Get_printer()
	vibrationpkg.NewAccelCommandHelper(config, self)
	self.query_rate = 0
	am := map[string][]float64{
		"x": {0, LIS2DW12_INFO["SCALE_XY"].(float64)}, "y": {1, LIS2DW12_INFO["SCALE_XY"].(float64)}, "z": {2, LIS2DW12_INFO["SCALE_Z"].(float64)},
		"-x": {0, -LIS2DW12_INFO["SCALE_XY"].(float64)}, "-y": {1, -LIS2DW12_INFO["SCALE_XY"].(float64)}, "-z": {2, -LIS2DW12_INFO["SCALE_Z"].(float64)},
	}
	axes_map := config.Getlist("axes_map", []interface{}{"x", "y", "z"}, ",", 3, true)

	self.axes_map = make([][]float64, 3)
	for i, v := range axes_map.([]interface{}) {
		if _, ok := am[v.(string)]; !ok {
			panic(errors.New("Invalid lis2dw12 axes_map parameter"))
		}
		self.axes_map[i] = am[strings.TrimSpace(v.(string))]
	}

	self.data_rate = config.Getint("rate", 1600, 0, 0, true)
	if _, ok := LIS2DW12_QUERY_RATES[self.data_rate]; !ok {
		panic(errors.New("Invalid lis2dw12 axes_map parameter"))
	}
	// Measurement storage (accessed from background thread)
	self.lock = sync.Mutex{}
	self.raw_samples = make([]map[string]interface{}, 0)
	// Setup mcu sensor_sensor bulk query code
	var err error
	self.spi, err = MCU_SPI_from_config(config, 3, "cs_pin", 5000000, nil, false)
	if err != nil {
		panic(fmt.Errorf("MCU_SPI_from_config error: %v", err))
	}
	self.mcu = self.spi.get_mcu()
	mcu := self.mcu
	self.oid = mcu.Create_oid()
	oid := self.oid
	self.query_sensor_cmd, self.query_sensor_end_cmd = nil, nil
	self.query_sensor_status_cmd = nil
	mcu.Add_config_cmd(fmt.Sprintf("config_lis2dw12 oid=%d spi_oid=%d",
		oid, self.spi.Get_oid()), false, false)
	mcu.Add_config_cmd(fmt.Sprintf("query_lis2dw12 oid=%d clock=0 rest_ticks=0",
		oid), false, true)
	mcu.Register_config_callback(self.Build_config)
	mcu.Register_response(self._handle_sensor_data, "lis2dw12_data", oid)
	// Clock tracking
	self.last_sequence, self.max_query_duration = 0, 0
	self.last_limit_count, self.last_error_count = 0, 0
	self.clock_sync = vibrationpkg.NewClockSyncRegression(self.mcu, 640, 1./20.)
	// API server endpoints
	self.api_dump = newAPIDumpHelper(
		self.printer, self._api_update, self._api_startstop, 0.100)
	self.name = str.LastName(config.Get_name())
	wh, ok := self.printer.Lookup_object("webhooks", object.Sentinel{}).(*WebHooks)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "webhooks", self.printer.Lookup_object("webhooks", object.Sentinel{})))
	}
	wh.Register_mux_endpoint("lis2dw12/dump_lis2dw12", "sensor", self.name, self._handle_dump_lis2dw12)
	return self
}

func (self *LIS2DW12) Build_config() {
	cmdqueue := self.spi.get_command_queue()
	self.query_sensor_cmd, _ = self.mcu.Lookup_command(
		"query_lis2dw12 oid=%c clock=%u rest_ticks=%u", cmdqueue)
	self.query_sensor_end_cmd = self.mcu.Lookup_query_command(
		"query_lis2dw12 oid=%c clock=%u rest_ticks=%u",
		"lis2dw12_status oid=%c clock=%u query_ticks=%u next_sequence=%hu"+
			" buffered=%c fifo=%c limit_count=%hu", self.oid, cmdqueue, false)
	self.query_sensor_status_cmd = self.mcu.Lookup_query_command(
		"query_lis2dw12_status oid=%c",
		"lis2dw12_status oid=%c clock=%u query_ticks=%u next_sequence=%hu"+
			" buffered=%c fifo=%c limit_count=%hu", self.oid, cmdqueue, false)

}

func (self *LIS2DW12) Read_reg(reg int) byte {
	params := self.spi.Spi_transfer([]int{reg | LIS2DW12_REGISTERS["REG_MOD_READ"], 0x00}, 0, 0)
	response := params.(map[string]interface{})["response"].([]int)
	return byte(response[1])
}

func (self *LIS2DW12) Set_reg(reg, val int, minclock int64) error {
	self.spi.Spi_send([]int{reg, val & 0xFF}, minclock, 0)
	stored_val := self.Read_reg(reg)
	if int(stored_val) != val {
		panic(fmt.Errorf("Failed to set LIS2DW12 register [0x%x] to 0x%x: got 0x%x. "+
			"This is generally indicative of connection problems "+
			"(e.g. faulty wiring) or a faulty lis2dw12 chip.",
			reg, val, stored_val))
	}
	return nil
}

// Measurement collection
func (self *LIS2DW12) Is_measuring() bool {
	return self.query_rate > 0
}

func (self *LIS2DW12) _handle_sensor_data(params map[string]interface{}) error {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.raw_samples = append(self.raw_samples, params)
	return nil
}

func (self *LIS2DW12) Extract_samples(raw_samples []map[string]interface{}) [][]float64 {
	return vibrationpkg.ExtractAccelSamples(raw_samples, vibrationpkg.SampleDecodeParams{
		AxesMap:         self.axes_map,
		LastSequence:    self.last_sequence,
		ClockSync:       self.clock_sync,
		BytesPerSample:  int(LIS2DW12_CLK["BYTES_PER_SAMPLE"]),
		SamplesPerBlock: int(LIS2DW12_CLK["SAMPLES_PER_BLOCK"]),
		ScaleDivisor:    4,
	}, func(d []int) (int, int, int, bool) {
		rx := int(int16(d[0]&0xfc | ((d[3] & 0xff) << 8)))
		ry := int(int16(d[1]&0xfc | ((d[4] & 0xff) << 8)))
		rz := int(int16(d[2]&0xfc | ((d[5] & 0xff) << 8)))
		return rx, ry, rz, true
	}).Samples
}

func (self *LIS2DW12) _update_clock(minclock int64) error {
	result, err := vibrationpkg.UpdateAccelClock(
		self.query_sensor_status_cmd,
		self.mcu,
		self.clock_sync,
		vibrationpkg.AccelClockSyncConfig{
			OID:             self.oid,
			Label:           "lis2dw12",
			BytesPerSample:  int(LIS2DW12_CLK["BYTES_PER_SAMPLE"]),
			SamplesPerBlock: int(LIS2DW12_CLK["SAMPLES_PER_BLOCK"]),
		},
		minclock,
		vibrationpkg.AccelClockSyncState{
			LastSequence:     self.last_sequence,
			LastLimitCount:   self.last_limit_count,
			MaxQueryDuration: self.max_query_duration,
		},
	)
	if err != nil {
		return err
	}
	self.last_sequence = result.LastSequence
	self.last_limit_count = result.LastLimitCount
	self.max_query_duration = result.MaxQueryDuration
	return nil
}

func (self *LIS2DW12) _start_measurements() error {
	if self.Is_measuring() {
		return nil
	}
	// In case of miswiring, testing LIS2DW12 device ID prevents treating
	// noise or wrong signal as a correctly initialized device
	dev_id := self.Read_reg(LIS2DW12_REGISTERS["REG_DEVID"])
	if dev_id != byte(LIS2DW12_INFO["DEV_ID"].(int)) {
		panic(fmt.Errorf("Invalid lis2dw12 id (got %x vs %x).\n"+
			"This is generally indicative of connection problems\n"+
			"(e.g. faulty wiring) or a faulty lis2dw12 chip.",
			dev_id, byte(LIS2DW12_INFO["DEV_ID"].(int))))
	}

	// Setup chip in requested query rate
	self.Set_reg(LIS2DW12_REGISTERS["REG_CTRL1"], LIS2DW12_QUERY_RATES[self.data_rate]<<4|LIS2DW12_INFO["SET_CTRL1_MODE"].(int), 0)
	self.Set_reg(LIS2DW12_REGISTERS["REG_FIFO_CTRL"], 0x00, 0)
	self.Set_reg(LIS2DW12_REGISTERS["REG_CTRL6"], LIS2DW12_INFO["SET_CTRL6_ODR_FS"].(int), 0)
	self.Set_reg(LIS2DW12_REGISTERS["REG_FIFO_CTRL"], LIS2DW12_INFO["SET_FIFO_CTL"].(int), 0)

	// Setup samples
	self.lock.Lock()
	self.raw_samples = make([]map[string]interface{}, 0)
	self.lock.Unlock()
	// Start bulk reading
	systime := self.printer.Get_reactor().Monotonic()
	print_time := self.mcu.Estimated_print_time(systime) + LIS2DW12_CLK["MIN_MSG_TIME"]
	reqclock := self.mcu.Print_time_to_clock(print_time)
	rest_ticks := self.mcu.Seconds_to_clock(4. / float64(self.data_rate))
	self.query_rate = self.data_rate
	self.query_sensor_cmd.Send([]int64{int64(self.oid), reqclock, rest_ticks}, 0, reqclock)
	log.Printf("LIS2DW12 starting '%s' measurements", self.name)
	// Initialize clock tracking
	self.last_sequence = 0
	self.last_limit_count, self.last_error_count = 0, 0
	self.clock_sync.Reset(float64(reqclock), 0)
	self.max_query_duration = 1 << 31
	self._update_clock(reqclock)
	self.max_query_duration = 1 << 31
	return nil
}

func (self *LIS2DW12) _finish_measurements() {
	if !self.Is_measuring() {
		return
	}
	// Halt bulk reading
	self.query_sensor_end_cmd.Send([]int64{int64(self.oid), 0, 0}, 0, 0)
	self.query_rate = 0
	self.lock.Lock()
	self.raw_samples = make([]map[string]interface{}, 0)
	self.lock.Unlock()
	log.Printf("LIS2DW12 finished '%s' measurements", self.name)
}

// API interface
func (self *LIS2DW12) _api_update(eventtime float64) map[string]interface{} {
	self._update_clock(0)
	self.lock.Lock()
	raw_samples := self.raw_samples
	self.raw_samples = make([]map[string]interface{}, 0)
	self.lock.Unlock()
	if len(raw_samples) == 0 {
		return map[string]interface{}{}
	}
	samples := self.Extract_samples(raw_samples)
	if len(samples) == 0 {
		fmt.Print("len(samples) = 0 ")
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"data": samples, "errors": self.last_error_count,
		"overflows": self.last_limit_count,
	}

}

func (self *LIS2DW12) _api_startstop(is_start bool) {
	if is_start {
		self._start_measurements()
	} else {
		self._finish_measurements()
	}
}

func (self *LIS2DW12) _handle_dump_lis2dw12(web_request *WebRequest) {
	addAPIDumpClient(self.api_dump, web_request)
	hdr := []string{"time", "x_acceleration", "y_acceleration", "z_acceleration"}
	web_request.Send(map[string][]string{"header": hdr})

}

func (self *LIS2DW12) Start_internal_client() vibrationpkg.IAclient {
	cconn := self.api_dump.AddInternalClient()
	return vibrationpkg.NewAccelQueryHelper(MustLookupToolhead(self.printer), cconn)
}

func (self *LIS2DW12) Get_name() string {
	return self.name
}
func Load_config_LIS2DW12(config *ConfigWrapper) interface{} {
	return NewLIS2DW12(config)
}
