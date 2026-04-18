package vibration

import (
	"fmt"
	"testing"
)

type fakeLIS2DW12Runtime struct {
	measuring          bool
	name               string
	dataRate           int
	registers          map[int]byte
	setCalls           []string
	startCalled        bool
	startMinMsgTime    float64
	startSyncClockCall int64
	updateClockArgs    struct {
		label           string
		bytesPerSample  int
		samplesPerBlock int
		minclock        int64
	}
	apiUpdateCalled bool
	apiRawSamples   []map[string]interface{}
	finishCalled    bool
}

func (self *fakeLIS2DW12Runtime) ExtractSamples(rawSamples []map[string]interface{}, bytesPerSample, samplesPerBlock, scaleDivisor int, decode func([]int) (int, int, int, bool)) [][]float64 {
	self.apiRawSamples = rawSamples
	return [][]float64{{5, 6, 7, 8}}
}

func (self *fakeLIS2DW12Runtime) IsMeasuring() bool { return self.measuring }
func (self *fakeLIS2DW12Runtime) Name() string      { return self.name }
func (self *fakeLIS2DW12Runtime) DataRate() int     { return self.dataRate }

func (self *fakeLIS2DW12Runtime) ReadRegister(readModifier int, reg int) byte {
	_ = readModifier
	return self.registers[reg]
}

func (self *fakeLIS2DW12Runtime) SetRegister(readModifier int, reg, val int, minclock int64, label string) error {
	_ = readModifier
	_ = minclock
	self.setCalls = append(self.setCalls, fmt.Sprintf("%s:%d=%d", label, reg, val))
	self.registers[reg] = byte(val)
	return nil
}

func (self *fakeLIS2DW12Runtime) StartMeasurements(minMsgTime float64, initialize func() error, syncClock func(int64) error) error {
	self.startCalled = true
	self.startMinMsgTime = minMsgTime
	if initialize != nil {
		if err := initialize(); err != nil {
			return err
		}
	}
	self.measuring = true
	if syncClock != nil {
		self.startSyncClockCall = 2468
		if err := syncClock(self.startSyncClockCall); err != nil {
			return err
		}
	}
	return nil
}

func (self *fakeLIS2DW12Runtime) FinishMeasurements() {
	self.finishCalled = true
	self.measuring = false
}

func (self *fakeLIS2DW12Runtime) UpdateClock(label string, bytesPerSample, samplesPerBlock int, minclock int64) error {
	self.updateClockArgs.label = label
	self.updateClockArgs.bytesPerSample = bytesPerSample
	self.updateClockArgs.samplesPerBlock = samplesPerBlock
	self.updateClockArgs.minclock = minclock
	return nil
}

func (self *fakeLIS2DW12Runtime) APIUpdate(updateClock func(int64) error, extractSamples func([]map[string]interface{}) [][]float64) map[string]interface{} {
	self.apiUpdateCalled = true
	if err := updateClock(8642); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	data := extractSamples([]map[string]interface{}{{"data": "raw"}})
	return map[string]interface{}{"data": data}
}

func TestStartLIS2DW12MeasurementsInitializesRegistersAndClock(t *testing.T) {
	runtime := &fakeLIS2DW12Runtime{
		name:      "toolhead",
		dataRate:  1600,
		registers: map[int]byte{LIS2DW12Registers["REG_DEVID"]: byte(LIS2DW12Info["DEV_ID"].(int))},
	}

	if err := StartLIS2DW12Measurements(runtime); err != nil {
		t.Fatalf("StartLIS2DW12Measurements() error = %v", err)
	}

	if !runtime.startCalled {
		t.Fatal("expected StartMeasurements to be invoked")
	}
	if runtime.startMinMsgTime != LIS2DW12Clk["MIN_MSG_TIME"] {
		t.Fatalf("unexpected min msg time: %v", runtime.startMinMsgTime)
	}
	if len(runtime.setCalls) != 4 {
		t.Fatalf("expected four register writes, got %d (%v)", len(runtime.setCalls), runtime.setCalls)
	}
	if runtime.updateClockArgs.label != "lis2dw12" || runtime.updateClockArgs.minclock != 2468 {
		t.Fatalf("unexpected clock update args: %#v", runtime.updateClockArgs)
	}
	if runtime.registers[LIS2DW12Registers["REG_CTRL1"]] != byte(LIS2DW12QueryRates[1600]<<4|LIS2DW12Info["SET_CTRL1_MODE"].(int)) {
		t.Fatalf("expected CTRL1 register to be configured, got %#v", runtime.registers[LIS2DW12Registers["REG_CTRL1"]])
	}
}

func TestStartLIS2DW12MeasurementsNoopsWhenAlreadyMeasuring(t *testing.T) {
	runtime := &fakeLIS2DW12Runtime{measuring: true, name: "toolhead", dataRate: 1600, registers: map[int]byte{}}

	if err := StartLIS2DW12Measurements(runtime); err != nil {
		t.Fatalf("StartLIS2DW12Measurements() error = %v", err)
	}
	if runtime.startCalled {
		t.Fatal("expected StartMeasurements not to run when already measuring")
	}
}

func TestLIS2DW12APIUpdateDelegatesClockUpdateAndExtraction(t *testing.T) {
	runtime := &fakeLIS2DW12Runtime{name: "toolhead", dataRate: 1600, registers: map[int]byte{}}

	result := LIS2DW12APIUpdate(runtime)

	if !runtime.apiUpdateCalled {
		t.Fatal("expected APIUpdate to be invoked")
	}
	if runtime.updateClockArgs.label != "lis2dw12" || runtime.updateClockArgs.minclock != 8642 {
		t.Fatalf("unexpected API clock update args: %#v", runtime.updateClockArgs)
	}
	data, ok := result["data"].([][]float64)
	if !ok || len(data) != 1 {
		t.Fatalf("expected extracted sample payload, got %#v", result)
	}
}

func TestFinishLIS2DW12MeasurementsStopsActiveRuntime(t *testing.T) {
	runtime := &fakeLIS2DW12Runtime{measuring: true, name: "toolhead", dataRate: 1600, registers: map[int]byte{}}

	FinishLIS2DW12Measurements(runtime)

	if !runtime.finishCalled || runtime.measuring {
		t.Fatalf("expected runtime to stop measuring, got finishCalled=%v measuring=%v", runtime.finishCalled, runtime.measuring)
	}
}
