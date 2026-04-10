package project

import (
	"errors"
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	reportpkg "goklipper/internal/pkg/motion/report"
	vibrationpkg "goklipper/internal/pkg/motion/vibration"
	serialpkg "goklipper/internal/pkg/serialhdl"
	"strings"
	"sync"
)

// ADXL345 register tables are defined in vibrationpkg.
var ADXL345_REGISTERS = vibrationpkg.ADXL345Registers
var ADXL345_QUERY_RATES = vibrationpkg.ADXL345QueryRates
var ADXL345_CLK = vibrationpkg.ADXL345Clk
var ADXL345_INFO = vibrationpkg.ADXL345Info

type apiDumpReactorAdapter struct {
	reactor IReactor
}

func (self *apiDumpReactorAdapter) Monotonic() float64 {
	return self.reactor.Monotonic()
}

func (self *apiDumpReactorAdapter) RegisterTimer(callback func(float64) float64, waketime float64) interface{} {
	return self.reactor.Register_timer(callback, waketime)
}

func (self *apiDumpReactorAdapter) UnregisterTimer(timer interface{}) {
	if timer == nil {
		return
	}
	self.reactor.Unregister_timer(timer.(*ReactorTimer))
}

type apiDumpClientAdapter struct {
	client *ClientConnection
}

func (self *apiDumpClientAdapter) IsClosed() bool {
	return self.client.Is_closed()
}

func (self *apiDumpClientAdapter) Send(msg map[string]interface{}) {
	self.client.Send(msg)
}

func newAPIDumpHelper(printer *Printer, dataCB func(float64) map[string]interface{}, startstopCB func(bool), updateInterval float64) *reportpkg.APIDumpHelper {
	return reportpkg.NewAPIDumpHelper(&apiDumpReactorAdapter{reactor: printer.Get_reactor()}, dataCB, startstopCB, updateInterval)
}

func addAPIDumpClient(helper *reportpkg.APIDumpHelper, webRequest *WebRequest) {
	helper.AddClient(&apiDumpClientAdapter{client: webRequest.Get_client_connection()}, webRequest.Get_dict("response_template", nil))
}

// Printer class that controls ADXL345 chip
type ADXL345 struct {
	printer                  *Printer
	query_rate               int
	axes_map                 [][]float64
	data_rate                int
	lock                     sync.Mutex
	raw_samples              []map[string]interface{}
	spi                      *MCU_SPI
	mcu                      *MCU
	oid                      int
	query_adxl345_cmd        *serialpkg.CommandWrapper
	query_adxl345_end_cmd    *serialpkg.CommandQueryWrapper
	query_adxl345_status_cmd *serialpkg.CommandQueryWrapper
	last_sequence            int
	max_query_duration       int64
	last_limit_count         int
	last_error_count         int
	clock_sync               *vibrationpkg.ClockSyncRegression
	api_dump                 *reportpkg.APIDumpHelper
	name                     string
}

func NewADXL345(config *ConfigWrapper) *ADXL345 {
	self := new(ADXL345)
	self.printer = config.Get_printer()
	vibrationpkg.NewAccelCommandHelper(config, self)
	self.query_rate = 0
	am := map[string][]float64{
		"x": {0, ADXL345_INFO["SCALE_XY"].(float64)}, "y": {1, ADXL345_INFO["SCALE_XY"].(float64)}, "z": {2, ADXL345_INFO["SCALE_Z"].(float64)},
		"-x": {0, -ADXL345_INFO["SCALE_XY"].(float64)}, "-y": {1, -ADXL345_INFO["SCALE_XY"].(float64)}, "-z": {2, -ADXL345_INFO["SCALE_Z"].(float64)},
	}
	axes_map := config.Getlist("axes_map", []string{"x", "y", "z"}, ",", 3, true).([]string)
	self.axes_map = make([][]float64, len(axes_map))
	for i, v := range axes_map {
		if _, ok := am[v]; !ok {
			panic(errors.New("Invalid adxl345 axes_map parameter"))
		}
		self.axes_map[i] = am[strings.TrimSpace(v)]
	}

	self.data_rate = config.Getint("rate", 3200, 0, 0, true)
	if _, ok := ADXL345_QUERY_RATES[self.data_rate]; !ok {
		panic(errors.New("Invalid adxl345 axes_map parameter"))
	}
	// Measurement storage (accessed from background thread)
	self.lock = sync.Mutex{}
	self.raw_samples = make([]map[string]interface{}, 0)
	// Setup mcu sensor_adxl345 bulk query code
	var err error
	self.spi, err = MCU_SPI_from_config(config, 3, "cs_pin", 5000000, nil, false)
	if err != nil {
		panic(fmt.Errorf("MCU_SPI_from_config error: %v", err))
	}
	self.mcu = self.spi.get_mcu()
	mcu := self.mcu
	self.oid = mcu.Create_oid()
	oid := self.oid
	self.query_adxl345_cmd, self.query_adxl345_end_cmd = nil, nil
	self.query_adxl345_status_cmd = nil
	mcu.Add_config_cmd(fmt.Sprintf("config_adxl345 oid=%d spi_oid=%d",
		oid, self.spi.Get_oid()), false, false)
	mcu.Add_config_cmd(fmt.Sprintf("query_adxl345 oid=%d clock=0 rest_ticks=0",
		oid), false, true)
	mcu.Register_config_callback(self.Build_config)
	mcu.Register_response(self._handle_adxl345_data, "adxl345_data", oid)
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
	wh.Register_mux_endpoint("adxl345/dump_adxl345", "sensor", self.name, self._handle_dump_adxl345)
	return self
}

func (self *ADXL345) Build_config() {
	cmdqueue := self.spi.get_command_queue()
	self.query_adxl345_cmd, _ = self.mcu.Lookup_command(
		"query_adxl345 oid=%c clock=%u rest_ticks=%u", cmdqueue)
	self.query_adxl345_end_cmd = self.mcu.Lookup_query_command(
		"query_adxl345 oid=%c clock=%u rest_ticks=%u",
		"adxl345_status oid=%c clock=%u query_ticks=%u next_sequence=%hu"+
			" buffered=%c fifo=%c limit_count=%hu", self.oid, cmdqueue, false)
	self.query_adxl345_status_cmd = self.mcu.Lookup_query_command(
		"query_adxl345_status oid=%c",
		"adxl345_status oid=%c clock=%u query_ticks=%u next_sequence=%hu"+
			" buffered=%c fifo=%c limit_count=%hu", self.oid, cmdqueue, false)

}

func (self *ADXL345) Read_reg(reg int) byte {
	params := self.spi.Spi_transfer([]int{reg | ADXL345_REGISTERS["REG_MOD_READ"], 0x00}, 0, 0)
	response := params.(map[string]interface{})["response"].([]int)
	return byte(response[1])
}

func (self *ADXL345) Set_reg(reg, val int, minclock int64) error {
	self.spi.Spi_send([]int{reg, val & 0xFF}, minclock, 0)
	stored_val := self.Read_reg(reg)
	if int(stored_val) != val {
		panic(fmt.Errorf("Failed to set ADXL345 register [0x%x] to 0x%x: got 0x%x. "+
			"This is generally indicative of connection problems "+
			"(e.g. faulty wiring) or a faulty adxl345 chip.",
			reg, val, stored_val))
	}
	return nil
}

// Measurement collection
func (self *ADXL345) Is_measuring() bool {
	return self.query_rate > 0
}

func (self *ADXL345) _handle_adxl345_data(params map[string]interface{}) error {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.raw_samples = append(self.raw_samples, params)
	return nil
}

func (self *ADXL345) Extract_samples(raw_samples []map[string]interface{}) [][]float64 {
	result := vibrationpkg.ExtractAccelSamples(raw_samples, vibrationpkg.SampleDecodeParams{
		AxesMap:         self.axes_map,
		LastSequence:    self.last_sequence,
		ClockSync:       self.clock_sync,
		BytesPerSample:  int(ADXL345_CLK["BYTES_PER_SAMPLE"]),
		SamplesPerBlock: int(ADXL345_CLK["SAMPLES_PER_BLOCK"]),
		ScaleDivisor:    1,
	}, func(d []int) (int, int, int, bool) {
		xlow, ylow, zlow, xzhigh, yzhigh := d[0], d[1], d[2], d[3], d[4]
		if yzhigh&0x80 != 0 {
			return 0, 0, 0, false
		}
		rx := (xlow | ((xzhigh & 0x1f) << 8)) - ((xzhigh & 0x10) << 9)
		ry := (ylow | ((yzhigh & 0x1f) << 8)) - ((yzhigh & 0x10) << 9)
		rz := (zlow | ((xzhigh & 0xe0) << 3) | ((yzhigh & 0xe0) << 6)) - ((yzhigh & 0x40) << 7)
		return rx, ry, rz, true
	})
	self.last_error_count += result.ErrorCount
	return result.Samples
}

func (self *ADXL345) _update_clock(minclock int64) error {
	result, err := vibrationpkg.UpdateAccelClock(
		self.query_adxl345_status_cmd,
		self.mcu,
		self.clock_sync,
		vibrationpkg.AccelClockSyncConfig{
			OID:             self.oid,
			Label:           "adxl345",
			BytesPerSample:  int(ADXL345_CLK["BYTES_PER_SAMPLE"]),
			SamplesPerBlock: int(ADXL345_CLK["SAMPLES_PER_BLOCK"]),
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

func (self *ADXL345) _start_measurements() error {
	if self.Is_measuring() {
		return nil
	}
	// In case of miswiring, testing ADXL345 device ID prevents treating
	// noise or wrong signal as a correctly initialized device
	dev_id := self.Read_reg(ADXL345_REGISTERS["REG_DEVID"])
	if dev_id != byte(ADXL345_INFO["DEV_ID"].(int)) {
		panic(fmt.Errorf("Invalid adxl345 id (got %x vs %x).\n"+
			"This is generally indicative of connection problems\n"+
			"(e.g. faulty wiring) or a faulty adxl345 chip.",
			dev_id, byte(ADXL345_INFO["DEV_ID"].(int))))
	}
	// Setup chip in requested query rate
	self.Set_reg(ADXL345_REGISTERS["REG_POWER_CTL"], 0x00, 0)
	self.Set_reg(ADXL345_REGISTERS["REG_DATA_FORMAT"], 0x0B, 0)
	self.Set_reg(ADXL345_REGISTERS["REG_FIFO_CTL"], 0x00, 0)
	self.Set_reg(ADXL345_REGISTERS["REG_BW_RATE"], ADXL345_QUERY_RATES[self.data_rate], 0)
	self.Set_reg(ADXL345_REGISTERS["REG_FIFO_CTL"], ADXL345_INFO["SET_FIFO_CTL"].(int), 0)
	// Setup samples
	self.lock.Lock()
	self.raw_samples = make([]map[string]interface{}, 0)
	self.lock.Unlock()
	// Start bulk reading
	systime := self.printer.Get_reactor().Monotonic()
	print_time := self.mcu.Estimated_print_time(systime) + ADXL345_CLK["MIN_MSG_TIME"]
	reqclock := self.mcu.Print_time_to_clock(print_time)
	rest_ticks := self.mcu.Seconds_to_clock(4. / float64(self.data_rate))
	self.query_rate = self.data_rate
	self.query_adxl345_cmd.Send([]int64{int64(self.oid), reqclock, rest_ticks}, 0, reqclock)
	logger.Debugf("ADXL345 starting '%s' measurements", self.name)
	// Initialize clock tracking
	self.last_sequence = 0
	self.last_limit_count, self.last_error_count = 0, 0
	self.clock_sync.Reset(float64(reqclock), 0)
	self.max_query_duration = 1 << 31
	self._update_clock(reqclock)
	self.max_query_duration = 1 << 31
	return nil
}

func (self *ADXL345) _finish_measurements() {
	if !self.Is_measuring() {
		return
	}
	// Halt bulk reading
	self.query_adxl345_end_cmd.Send([]int64{int64(self.oid), 0, 0}, 0, 0)
	self.query_rate = 0
	self.lock.Lock()
	self.raw_samples = make([]map[string]interface{}, 0)
	self.lock.Unlock()
	logger.Debugf("ADXL345 finished '%s' measurements", self.name)
}

// API interface
func (self *ADXL345) _api_update(eventtime float64) map[string]interface{} {
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

func (self *ADXL345) _api_startstop(is_start bool) {
	if is_start {
		self._start_measurements()
	} else {
		self._finish_measurements()
	}
}

func (self *ADXL345) _handle_dump_adxl345(web_request *WebRequest) {
	addAPIDumpClient(self.api_dump, web_request)
	hdr := []string{"time", "x_acceleration", "y_acceleration", "z_acceleration"}
	web_request.Send(map[string][]string{"header": hdr})

}

func (self *ADXL345) Start_internal_client() vibrationpkg.IAclient {
	cconn := self.api_dump.AddInternalClient()
	return vibrationpkg.NewAccelQueryHelper(MustLookupToolhead(self.printer), cconn)
}

func (self *ADXL345) Get_name() string {
	return self.name
}
func Load_config_ADXL345(config *ConfigWrapper) interface{} {
	return NewADXL345(config)
}
