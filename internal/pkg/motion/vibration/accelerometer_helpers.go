package vibration

import (
	"errors"
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/str"
	"goklipper/common/value"
	printerpkg "goklipper/internal/pkg/printer"
	"math"
	"os"
	"strings"
	"time"
)

type IAclient = accelClient

type IAccelChip interface {
	Start_internal_client() IAclient
	Set_reg(reg, val int, minclock int64) error
	Read_reg(reg int) byte
	Get_name() string
}

type accelQueryToolhead interface {
	Get_last_move_time() float64
	Wait_moves()
}

type accelDumpConnection interface {
	Finalize()
	Get_messages() []map[string]map[string]interface{}
}

type accelHelperConfig interface {
	printerpkg.ModuleConfig
	Get_name() string
	Has_section(section string) bool
}

type accelCommandToolhead interface {
	Dwell(delay float64)
}

type accelGCode interface {
	printerpkg.GCodeRuntime
	RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string)
}

type accelLegacyCommand interface {
	printerpkg.Command
	Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	Get_int(name string, _default interface{}, minval *int, maxval *int) int
	Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64
}

type clockSyncMCU interface {
	Clock_to_print_time(clock int64) float64
}

func requireAccelConfig(config printerpkg.ModuleConfig) accelHelperConfig {
	legacy, ok := config.(accelHelperConfig)
	if !ok {
		panic(fmt.Sprintf("config does not implement accelHelperConfig: %T", config))
	}
	return legacy
}

func requireAccelToolhead(printer printerpkg.ModulePrinter) accelCommandToolhead {
	toolheadObj := printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(accelCommandToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement accelCommandToolhead: %T", toolheadObj))
	}
	return toolhead
}

func requireAccelGCode(printer printerpkg.ModulePrinter) accelGCode {
	gcodeObj := printer.GCode()
	gcode, ok := gcodeObj.(accelGCode)
	if !ok {
		panic(fmt.Sprintf("gcode runtime does not implement accelGCode: %T", gcodeObj))
	}
	return gcode
}

func requireAccelCommand(gcmd printerpkg.Command) accelLegacyCommand {
	cmd, ok := gcmd.(accelLegacyCommand)
	if !ok {
		panic(fmt.Sprintf("accelerometer command does not implement accelLegacyCommand: %T", gcmd))
	}
	return cmd
}

// Helper class to obtain measurements
type AccelQueryHelper struct {
	toolhead           accelQueryToolhead
	ccon               accelDumpConnection
	request_start_time float64
	request_end_time   float64
	samples            [][]float64
	raw_samples        []map[string]map[string]interface{}
}

func NewAccelQueryHelper(toolhead accelQueryToolhead, ccon accelDumpConnection) *AccelQueryHelper {
	self := &AccelQueryHelper{}
	self.toolhead = toolhead
	self.ccon = ccon
	printTime := self.toolhead.Get_last_move_time()
	self.request_start_time = printTime
	self.request_end_time = printTime
	self.samples = [][]float64{}
	self.raw_samples = nil
	return self
}

func (self *AccelQueryHelper) Finish_measurements() {
	self.request_end_time = self.toolhead.Get_last_move_time()
	self.toolhead.Wait_moves()
	self.ccon.Finalize()
}

func (self *AccelQueryHelper) _get_raw_samples() []map[string]map[string]interface{} {
	rawSamples := self.ccon.Get_messages()
	if len(rawSamples) > 0 {
		self.raw_samples = rawSamples
	}
	return self.raw_samples
}

func (self *AccelQueryHelper) Has_valid_samples() bool {
	rawSamples := self._get_raw_samples()
	for _, msg := range rawSamples {
		data, ok := msg["params"]["data"].([][]float64)
		if !ok || len(data) == 0 {
			continue
		}
		firstSampleTime := data[0][0]
		lastSampleTime := data[len(data)-1][0]
		if firstSampleTime > self.request_end_time || lastSampleTime < self.request_start_time {
			continue
		}
		return true
	}
	return false
}

func (self *AccelQueryHelper) Get_samples() [][]float64 {
	rawSamples := self._get_raw_samples()
	if len(rawSamples) == 0 {
		return self.samples
	}
	total := 0
	for _, msg := range rawSamples {
		data, ok := msg["params"]["data"].([][]float64)
		if !ok {
			continue
		}
		total += len(data)
	}
	count := 0
	self.samples = make([][]float64, total)
	samples := self.samples
	for _, msg := range rawSamples {
		data, ok := msg["params"]["data"].([][]float64)
		if !ok {
			continue
		}
		for _, sample := range data {
			sampTime := sample[0]
			if sampTime < self.request_start_time {
				continue
			}
			if sampTime > self.request_end_time {
				break
			}
			samples[count] = []float64{sampTime, sample[1], sample[2], sample[3]}
			count += 1
		}
	}
	self.samples = samples[:count]
	return self.samples
}

func (self *AccelQueryHelper) Write_to_file(filename string) {
	writeImpl := func() {
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModeExclusive|0666)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		f.Write([]byte("#time,accel_x,accel_y,accel_z\r\n"))
		samples := self.samples
		if len(samples) == 0 {
			samples = self.Get_samples()
		}
		for _, val := range samples {
			f.Write([]byte(fmt.Sprintf("%.6f,%.6f,%.6f,%.6f\n", val[0], val[1], val[2], val[3])))
		}
	}
	go writeImpl()
}

// Helper class for G-Code commands
type AccelCommandHelper struct {
	printer   printerpkg.ModulePrinter
	chip      IAccelChip
	bg_client IAclient
	base_name string
	name      string
}

func NewAccelCommandHelper(config printerpkg.ModuleConfig, chip IAccelChip) *AccelCommandHelper {
	cfg := requireAccelConfig(config)
	self := &AccelCommandHelper{}
	self.printer = cfg.Printer()
	self.chip = chip
	self.bg_client = nil
	nameParts := strings.Split(cfg.Get_name(), " ")
	self.base_name = nameParts[0]
	self.name = nameParts[len(nameParts)-1]
	self.Register_commands(cfg.Get_name())
	if len(nameParts) == 1 {
		if self.name == "adxl345" || !cfg.Has_section("adxl345") {
			self.Register_commands("")
		}
	}
	return self
}

func (self *AccelCommandHelper) Register_commands(name string) {
	gcode := requireAccelGCode(self.printer)
	gcode.RegisterMuxCommand("ACCELEROMETER_MEASURE", "CHIP", name, self.Cmd_ACCELEROMETER_MEASURE, cmd_ACCELEROMETER_MEASURE_help)
	gcode.RegisterMuxCommand("ACCELEROMETER_QUERY", "CHIP", name, self.Cmd_ACCELEROMETER_QUERY, cmd_ACCELEROMETER_QUERY_help)
	gcode.RegisterMuxCommand("ACCELEROMETER_DEBUG_READ", "CHIP", name, self.Cmd_ACCELEROMETER_DEBUG_READ, cmd_ACCELEROMETER_DEBUG_READ_help)
	gcode.RegisterMuxCommand("ACCELEROMETER_DEBUG_WRITE", "CHIP", name, self.Cmd_ACCELEROMETER_DEBUG_WRITE, cmd_ACCELEROMETER_DEBUG_WRITE_help)
}

const cmd_ACCELEROMETER_MEASURE_help = "Start/stop accelerometer"

func (self *AccelCommandHelper) Cmd_ACCELEROMETER_MEASURE(gcmd printerpkg.Command) error {
	cmd := requireAccelCommand(gcmd)
	if value.IsNone(self.bg_client) {
		self.bg_client = self.chip.Start_internal_client()
		logger.Debug("accelerometer measurements started")
		return nil
	}
	name := cmd.Get("NAME", time.Now().Format("20230215_161242"), nil, nil, nil, nil, nil)
	normalized := strings.ReplaceAll(strings.ReplaceAll(name, "-", ""), "_", "")
	if !str.IsAlphanum(normalized) {
		panic(errors.New("Invalid NAME parameter"))
	}
	bgClient := self.bg_client
	self.bg_client = nil
	bgClient.Finish_measurements()
	filename := fmt.Sprintf("/tmp/%s-%s.csv", self.base_name, name)
	if self.base_name != self.name {
		filename = fmt.Sprintf("/tmp/%s-%s-%s.csv", self.base_name, self.name, name)
	}
	bgClient.Write_to_file(filename)
	gcmd.RespondInfo(fmt.Sprintf("Writing raw accelerometer data to %s file", filename), true)
	return nil
}

const cmd_ACCELEROMETER_QUERY_help = "Query accelerometer for the current values"

func (self *AccelCommandHelper) Cmd_ACCELEROMETER_QUERY(gcmd printerpkg.Command) error {
	aclient := self.chip.Start_internal_client()
	requireAccelToolhead(self.printer).Dwell(1.)
	aclient.Finish_measurements()
	values := aclient.Get_samples()
	if len(values) == 0 {
		return errors.New("No accelerometer measurements found")
	}
	accelX := values[len(values)-1][1]
	accelY := values[len(values)-1][2]
	accelZ := values[len(values)-1][3]
	gcmd.RespondInfo(fmt.Sprintf("accelerometer values (x, y, z): %.6f, %.6f, %.6f", accelX, accelY, accelZ), true)
	return nil
}

const cmd_ACCELEROMETER_DEBUG_READ_help = "Query register (for debugging)"

func (self *AccelCommandHelper) Cmd_ACCELEROMETER_DEBUG_READ(gcmd printerpkg.Command) error {
	cmd := requireAccelCommand(gcmd)
	zero, maxReg := 0, 126
	reg := cmd.Get_int("REG", 0, &zero, &maxReg)
	val := self.chip.Read_reg(reg)
	gcmd.RespondInfo(fmt.Sprintf("Accelerometer REG[0x%x] = 0x%x", reg, val), true)
	return nil
}

const cmd_ACCELEROMETER_DEBUG_WRITE_help = "Set register (for debugging)"

func (self *AccelCommandHelper) Cmd_ACCELEROMETER_DEBUG_WRITE(gcmd printerpkg.Command) error {
	cmd := requireAccelCommand(gcmd)
	zero, maxReg, maxVal := 0, 126, 255
	reg := cmd.Get_int("REG", 0, &zero, &maxReg)
	val := cmd.Get_int("VAL", 0, &zero, &maxVal)
	return self.chip.Set_reg(reg, val, 0)
}

// Helper class for chip clock synchronization via linear regression
type ClockSyncRegression struct {
	mcu                   clockSyncMCU
	chip_clock_smooth     float64
	decay                 float64
	last_chip_clock       float64
	last_exp_mcu_clock    float64
	mcu_clock_avg         float64
	mcu_clock_variance    float64
	chip_clock_avg        float64
	chip_clock_covariance float64
	last_mcu_clock        float64
}

func NewClockSyncRegression(mcu clockSyncMCU, chip_clock_smooth float64, decay float64) *ClockSyncRegression {
	self := new(ClockSyncRegression)
	self.mcu = mcu
	self.chip_clock_smooth = chip_clock_smooth
	self.decay = decay
	return self
}

func (self *ClockSyncRegression) Reset(mcu_clock, chip_clock float64) {
	self.mcu_clock_avg = mcu_clock
	self.last_mcu_clock = mcu_clock
	self.chip_clock_avg = chip_clock
	self.mcu_clock_variance = 0.
	self.chip_clock_covariance = 0.
	self.last_chip_clock = 0.
	self.last_exp_mcu_clock = 0.
}

func (self *ClockSyncRegression) Update(mcu_clock, chip_clock float64) {
	decay := self.decay
	diffMCUClock := mcu_clock - self.mcu_clock_avg
	self.mcu_clock_avg += decay * diffMCUClock
	self.mcu_clock_variance = (1. - decay) * (self.mcu_clock_variance + math.Pow(diffMCUClock, 2)*decay)
	diffChipClock := chip_clock - self.chip_clock_avg
	self.chip_clock_avg += decay * diffChipClock
	self.chip_clock_covariance = (1. - decay) * (self.chip_clock_covariance + diffMCUClock*diffChipClock*decay)
}

func (self *ClockSyncRegression) Set_last_chip_clock(chip_clock float64) {
	baseMCU, baseChip, invCFreq := self.Get_clock_translation()
	self.last_chip_clock = chip_clock
	self.last_exp_mcu_clock = baseMCU + (chip_clock-baseChip)*invCFreq
}

func (self *ClockSyncRegression) Get_clock_translation() (float64, float64, float64) {
	invChipFreq := self.mcu_clock_variance / self.chip_clock_covariance
	if value.Not(self.last_chip_clock) {
		return self.mcu_clock_avg, self.chip_clock_avg, invChipFreq
	}
	sChipClock := self.last_chip_clock + self.chip_clock_smooth
	scDiff := sChipClock - self.chip_clock_avg
	sMCUClock := self.mcu_clock_avg + scDiff*invChipFreq
	mDiff := sMCUClock - self.last_exp_mcu_clock
	sInvChipFreq := mDiff / self.chip_clock_smooth
	return self.last_exp_mcu_clock, self.last_chip_clock, sInvChipFreq
}

func (self *ClockSyncRegression) Get_time_translation() (float64, float64, float64) {
	baseMCU, baseChip, invCFreq := self.Get_clock_translation()
	baseTime := self.mcu.Clock_to_print_time(int64(baseMCU))
	invFreq := self.mcu.Clock_to_print_time(int64(baseMCU+invCFreq)) - baseTime
	return baseTime, baseChip, invFreq
}