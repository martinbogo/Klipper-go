package vibration

import (
	"fmt"
	"testing"
)

type fakeADXL345Runtime struct {
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

func (self *fakeADXL345Runtime) ExtractSamples(rawSamples []map[string]interface{}, bytesPerSample, samplesPerBlock, scaleDivisor int, decode func([]int) (int, int, int, bool)) [][]float64 {
	self.apiRawSamples = rawSamples
	return [][]float64{{1, 2, 3, 4}}
}

func (self *fakeADXL345Runtime) IsMeasuring() bool { return self.measuring }
func (self *fakeADXL345Runtime) Name() string      { return self.name }
func (self *fakeADXL345Runtime) DataRate() int     { return self.dataRate }

func (self *fakeADXL345Runtime) ReadRegister(readModifier int, reg int) byte {
	_ = readModifier
	return self.registers[reg]
}

func (self *fakeADXL345Runtime) SetRegister(readModifier int, reg, val int, minclock int64, label string) error {
	_ = readModifier
	_ = minclock
	self.setCalls = append(self.setCalls, fmt.Sprintf("%s:%d=%d", label, reg, val))
	self.registers[reg] = byte(val)
	return nil
}

func (self *fakeADXL345Runtime) StartMeasurements(minMsgTime float64, initialize func() error, syncClock func(int64) error) error {
	self.startCalled = true
	self.startMinMsgTime = minMsgTime
	if initialize != nil {
		if err := initialize(); err != nil {
			return err
		}
	}
	self.measuring = true
	if syncClock != nil {
		self.startSyncClockCall = 1234
		if err := syncClock(self.startSyncClockCall); err != nil {
			return err
		}
	}
	return nil
}

func (self *fakeADXL345Runtime) FinishMeasurements() {
	self.finishCalled = true
	self.measuring = false
}

func (self *fakeADXL345Runtime) UpdateClock(label string, bytesPerSample, samplesPerBlock int, minclock int64) error {
	self.updateClockArgs.label = label
	self.updateClockArgs.bytesPerSample = bytesPerSample
	self.updateClockArgs.samplesPerBlock = samplesPerBlock
	self.updateClockArgs.minclock = minclock
	return nil
}

func (self *fakeADXL345Runtime) APIUpdate(updateClock func(int64) error, extractSamples func([]map[string]interface{}) [][]float64) map[string]interface{} {
	self.apiUpdateCalled = true
	if err := updateClock(4321); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	data := extractSamples([]map[string]interface{}{{"data": "raw"}})
	return map[string]interface{}{"data": data}
}

func TestStartADXL345MeasurementsInitializesRegistersAndClock(t *testing.T) {
	runtime := &fakeADXL345Runtime{
		name:      "toolhead",
		dataRate:  3200,
		registers: map[int]byte{ADXL345Registers["REG_DEVID"]: byte(ADXL345Info["DEV_ID"].(int))},
	}

	if err := StartADXL345Measurements(runtime); err != nil {
		t.Fatalf("StartADXL345Measurements() error = %v", err)
	}

	if !runtime.startCalled {
		t.Fatal("expected StartMeasurements to be invoked")
	}
	if runtime.startMinMsgTime != ADXL345Clk["MIN_MSG_TIME"] {
		t.Fatalf("unexpected min msg time: %v", runtime.startMinMsgTime)
	}
	if len(runtime.setCalls) != 5 {
		t.Fatalf("expected five register writes, got %d (%v)", len(runtime.setCalls), runtime.setCalls)
	}
	if runtime.updateClockArgs.label != "adxl345" || runtime.updateClockArgs.minclock != 1234 {
		t.Fatalf("unexpected clock update args: %#v", runtime.updateClockArgs)
	}
	if runtime.registers[ADXL345Registers["REG_BW_RATE"]] != byte(ADXL345QueryRates[3200]) {
		t.Fatalf("expected BW rate register to be configured, got %#v", runtime.registers[ADXL345Registers["REG_BW_RATE"]])
	}
}

func TestStartADXL345MeasurementsNoopsWhenAlreadyMeasuring(t *testing.T) {
	runtime := &fakeADXL345Runtime{measuring: true, name: "toolhead", dataRate: 3200, registers: map[int]byte{}}

	if err := StartADXL345Measurements(runtime); err != nil {
		t.Fatalf("StartADXL345Measurements() error = %v", err)
	}
	if runtime.startCalled {
		t.Fatal("expected StartMeasurements not to run when already measuring")
	}
}

func TestADXL345APIUpdateDelegatesClockUpdateAndExtraction(t *testing.T) {
	runtime := &fakeADXL345Runtime{name: "toolhead", dataRate: 3200, registers: map[int]byte{}}

	result := ADXL345APIUpdate(runtime)

	if !runtime.apiUpdateCalled {
		t.Fatal("expected APIUpdate to be invoked")
	}
	if runtime.updateClockArgs.label != "adxl345" || runtime.updateClockArgs.minclock != 4321 {
		t.Fatalf("unexpected API clock update args: %#v", runtime.updateClockArgs)
	}
	data, ok := result["data"].([][]float64)
	if !ok || len(data) != 1 {
		t.Fatalf("expected extracted sample payload, got %#v", result)
	}
}

func TestFinishADXL345MeasurementsStopsActiveRuntime(t *testing.T) {
	runtime := &fakeADXL345Runtime{measuring: true, name: "toolhead", dataRate: 3200, registers: map[int]byte{}}

	FinishADXL345Measurements(runtime)

	if !runtime.finishCalled || runtime.measuring {
		t.Fatalf("expected runtime to stop measuring, got finishCalled=%v measuring=%v", runtime.finishCalled, runtime.measuring)
	}
}
