package vibration

import (
	"errors"
	"fmt"
	reportpkg "goklipper/internal/pkg/motion/report"
	serialpkg "goklipper/internal/pkg/serialhdl"
	"strings"
	"sync"
)

type accelerometerRuntimeSPI interface {
	Send(data []int, minclock, reqclock int64)
	TransferResponse(data []int, minclock, reqclock int64) []int
}

type accelerometerRuntimeMCU interface {
	clockSyncMCU
	Clock32_to_clock64(clock int64) int64
	Seconds_to_clock(seconds float64) int64
	Monotonic() float64
	EstimatedPrintTime(eventtime float64) float64
	PrintTimeToClock(printTime float64) int64
	SecondsToClock(time float64) int64
}

type accelerometerCommand interface {
	Send(data interface{}, minclock, reqclock int64)
}

type accelerometerQueryCommand interface {
	Send(data interface{}, minclock, reqclock int64) interface{}
}

type accelerometerCommandLookup interface {
	Lookup_command(msgformat string, cq interface{}) (*serialpkg.CommandWrapper, error)
	Lookup_query_command(msgformat string, respformat string, oid int, cq interface{}, is_async bool) *serialpkg.CommandQueryWrapper
}

type AccelerometerRuntimeConfig struct {
	Name      string
	AxesMap   [][]float64
	DataRate  int
	SPI       accelerometerRuntimeSPI
	MCU       accelerometerRuntimeMCU
	OID       int
	ClockSync *ClockSyncRegression
}

type AccelerometerRuntime struct {
	axesMap          [][]float64
	dataRate         int
	queryRate        int
	lock             sync.Mutex
	rawSamples       []map[string]interface{}
	spi              accelerometerRuntimeSPI
	mcu              accelerometerRuntimeMCU
	oid              int
	queryCmd         accelerometerCommand
	queryEndCmd      accelerometerQueryCommand
	queryStatusCmd   accelerometerQueryCommand
	lastSequence     int
	maxQueryDuration int64
	lastLimitCount   int
	lastErrorCount   int
	clockSync        *ClockSyncRegression
	apiDump          *reportpkg.APIDumpHelper
	name             string
}

func BuildAxesMap(axes []string, scaleXY, scaleZ float64, sensor string) [][]float64 {
	axisMap := map[string][]float64{
		"x": {0, scaleXY}, "y": {1, scaleXY}, "z": {2, scaleZ},
		"-x": {0, -scaleXY}, "-y": {1, -scaleXY}, "-z": {2, -scaleZ},
	}
	mappedAxes := make([][]float64, len(axes))
	for i, axis := range axes {
		normalized := strings.TrimSpace(axis)
		values, ok := axisMap[normalized]
		if !ok {
			panic(errors.New(fmt.Sprintf("Invalid %s axes_map parameter", sensor)))
		}
		mappedAxes[i] = values
	}
	return mappedAxes
}

func ValidateQueryRate(dataRate int, queryRates map[int]int, sensor string) {
	if _, ok := queryRates[dataRate]; !ok {
		panic(errors.New(fmt.Sprintf("Invalid %s axes_map parameter", sensor)))
	}
}

func NewAccelerometerRuntime(config AccelerometerRuntimeConfig) *AccelerometerRuntime {
	runtime := &AccelerometerRuntime{
		axesMap:    config.AxesMap,
		dataRate:   config.DataRate,
		queryRate:  0,
		lock:       sync.Mutex{},
		rawSamples: make([]map[string]interface{}, 0),
		spi:        config.SPI,
		mcu:        config.MCU,
		oid:        config.OID,
		name:       config.Name,
	}
	if config.ClockSync != nil {
		runtime.clockSync = config.ClockSync
	} else {
		runtime.clockSync = NewClockSyncRegression(config.MCU, 640, 1./20.)
	}
	return runtime
}

func (self *AccelerometerRuntime) SetAPIDump(apiDump *reportpkg.APIDumpHelper) {
	self.apiDump = apiDump
}

func (self *AccelerometerRuntime) AddClient(client reportpkg.APIDumpClient, template map[string]interface{}) {
	if self.apiDump == nil {
		panic(errors.New("accelerometer api dump helper not configured"))
	}
	self.apiDump.AddClient(client, template)
}

func (self *AccelerometerRuntime) StartInternalClient(toolhead accelQueryToolhead) IAclient {
	if self.apiDump == nil {
		panic(errors.New("accelerometer api dump helper not configured"))
	}
	cconn := self.apiDump.AddInternalClient()
	return NewAccelQueryHelper(toolhead, cconn)
}

func (self *AccelerometerRuntime) Name() string {
	return self.name
}

func (self *AccelerometerRuntime) DataRate() int {
	return self.dataRate
}

func (self *AccelerometerRuntime) SetCommands(queryCmd accelerometerCommand, queryEndCmd accelerometerQueryCommand, queryStatusCmd accelerometerQueryCommand) {
	self.queryCmd = queryCmd
	self.queryEndCmd = queryEndCmd
	self.queryStatusCmd = queryStatusCmd
}

func LookupAccelerometerCommands(mcu accelerometerCommandLookup, cmdQueue interface{}, oid int, queryFormat string, statusQueryFormat string, statusFormat string) (*serialpkg.CommandWrapper, *serialpkg.CommandQueryWrapper, *serialpkg.CommandQueryWrapper) {
	queryCmd, _ := mcu.Lookup_command(queryFormat, cmdQueue)
	queryEndCmd := mcu.Lookup_query_command(queryFormat, statusFormat, oid, cmdQueue, false)
	queryStatusCmd := mcu.Lookup_query_command(statusQueryFormat, statusFormat, oid, cmdQueue, false)
	return queryCmd, queryEndCmd, queryStatusCmd
}

func (self *AccelerometerRuntime) ReadRegister(readModifier int, reg int) byte {
	response := self.spi.TransferResponse([]int{reg | readModifier, 0x00}, 0, 0)
	return byte(response[1])
}

func (self *AccelerometerRuntime) SetRegister(readModifier int, reg, val int, minclock int64, label string) error {
	self.spi.Send([]int{reg, val & 0xFF}, minclock, 0)
	storedVal := self.ReadRegister(readModifier, reg)
	if int(storedVal) != val {
		panic(fmt.Errorf("Failed to set %s register [0x%x] to 0x%x: got 0x%x. "+
			"This is generally indicative of connection problems "+
			"(e.g. faulty wiring) or a faulty %s chip.",
			label, reg, val, storedVal, strings.ToLower(label)))
	}
	return nil
}

func (self *AccelerometerRuntime) IsMeasuring() bool {
	return self.queryRate > 0
}

func (self *AccelerometerRuntime) HandleSensorData(params map[string]interface{}) error {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.rawSamples = append(self.rawSamples, params)
	return nil
}

func (self *AccelerometerRuntime) ExtractSamples(rawSamples []map[string]interface{}, bytesPerSample, samplesPerBlock, scaleDivisor int, decode func([]int) (int, int, int, bool)) [][]float64 {
	result := ExtractAccelSamples(rawSamples, SampleDecodeParams{
		AxesMap:         self.axesMap,
		LastSequence:    self.lastSequence,
		ClockSync:       self.clockSync,
		BytesPerSample:  bytesPerSample,
		SamplesPerBlock: samplesPerBlock,
		ScaleDivisor:    float64(scaleDivisor),
	}, decode)
	self.lastErrorCount += result.ErrorCount
	return result.Samples
}

func (self *AccelerometerRuntime) UpdateClock(label string, bytesPerSample, samplesPerBlock int, minclock int64) error {
	if self.queryStatusCmd == nil {
		return fmt.Errorf("accelerometer status command not configured for %s", label)
	}
	result, err := UpdateAccelClock(
		self.queryStatusCmd,
		self.mcu,
		self.clockSync,
		AccelClockSyncConfig{
			OID:             self.oid,
			Label:           label,
			BytesPerSample:  bytesPerSample,
			SamplesPerBlock: samplesPerBlock,
		},
		minclock,
		AccelClockSyncState{
			LastSequence:     self.lastSequence,
			LastLimitCount:   self.lastLimitCount,
			MaxQueryDuration: self.maxQueryDuration,
		},
	)
	if err != nil {
		return err
	}
	self.lastSequence = result.LastSequence
	self.lastLimitCount = result.LastLimitCount
	self.maxQueryDuration = result.MaxQueryDuration
	return nil
}

func (self *AccelerometerRuntime) StartMeasurements(minMsgTime float64, initialize func() error, syncClock func(int64) error) error {
	if self.queryCmd == nil {
		return errors.New("accelerometer query command not configured")
	}
	if initialize != nil {
		if err := initialize(); err != nil {
			return err
		}
	}
	self.lock.Lock()
	self.rawSamples = make([]map[string]interface{}, 0)
	self.lock.Unlock()
	systime := self.mcu.Monotonic()
	printTime := self.mcu.EstimatedPrintTime(systime) + minMsgTime
	reqclock := self.mcu.PrintTimeToClock(printTime)
	restTicks := self.mcu.SecondsToClock(4. / float64(self.dataRate))
	self.queryRate = self.dataRate
	self.queryCmd.Send([]int64{int64(self.oid), reqclock, restTicks}, 0, reqclock)
	self.lastSequence = 0
	self.lastLimitCount, self.lastErrorCount = 0, 0
	self.clockSync.Reset(float64(reqclock), 0)
	self.maxQueryDuration = 1 << 31
	if syncClock != nil {
		_ = syncClock(reqclock)
	}
	self.maxQueryDuration = 1 << 31
	return nil
}

func (self *AccelerometerRuntime) FinishMeasurements() {
	if self.queryEndCmd != nil {
		self.queryEndCmd.Send([]int64{int64(self.oid), 0, 0}, 0, 0)
	}
	self.queryRate = 0
	self.lock.Lock()
	self.rawSamples = make([]map[string]interface{}, 0)
	self.lock.Unlock()
}

func (self *AccelerometerRuntime) APIUpdate(updateClock func(int64) error, extractSamples func([]map[string]interface{}) [][]float64) map[string]interface{} {
	if updateClock != nil {
		_ = updateClock(0)
	}
	self.lock.Lock()
	rawSamples := self.rawSamples
	self.rawSamples = make([]map[string]interface{}, 0)
	self.lock.Unlock()
	if len(rawSamples) == 0 {
		return map[string]interface{}{}
	}
	samples := extractSamples(rawSamples)
	if len(samples) == 0 {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"data":      samples,
		"errors":    self.lastErrorCount,
		"overflows": self.lastLimitCount,
	}
}

func (self *AccelerometerRuntime) APIStartStop(isStart bool, start func() error, stop func()) {
	if isStart {
		if start != nil {
			_ = start()
		}
		return
	}
	if stop != nil {
		stop()
	}
}
