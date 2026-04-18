package vibration

import (
	"reflect"
	"testing"
)

type fakeAccelerometerMCU struct {
	monotonic         float64
	estimatedOffset   float64
	clockMultiplier   float64
	secondsMultiplier float64
}

func (self *fakeAccelerometerMCU) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeAccelerometerMCU) EstimatedPrintTime(eventtime float64) float64 {
	return eventtime + self.estimatedOffset
}

func (self *fakeAccelerometerMCU) PrintTimeToClock(printTime float64) int64 {
	return int64(printTime * self.clockMultiplier)
}

func (self *fakeAccelerometerMCU) SecondsToClock(time float64) int64 {
	return int64(time * self.secondsMultiplier)
}

func (self *fakeAccelerometerMCU) Seconds_to_clock(time float64) int64 {
	return self.SecondsToClock(time)
}

func (self *fakeAccelerometerMCU) Clock32_to_clock64(clock int64) int64 {
	return clock
}

func (self *fakeAccelerometerMCU) Clock_to_print_time(clock int64) float64 {
	return float64(clock) / self.clockMultiplier
}

type fakeAccelerometerSPI struct {
	transferResponse map[string]interface{}
	sent             []int
}

func (self *fakeAccelerometerSPI) TransferResponse(data []int, minclock, reqclock int64) []int {
	_ = data
	_ = minclock
	_ = reqclock
	return self.transferResponse["response"].([]int)
}

func (self *fakeAccelerometerSPI) Send(data []int, minclock, reqclock int64) {
	_ = minclock
	_ = reqclock
	self.sent = append([]int{}, data...)
}

type fakeAccelCommand struct {
	data     interface{}
	minclock int64
	reqclock int64
}

func (self *fakeAccelCommand) Send(data interface{}, minclock, reqclock int64) {
	self.data = data
	self.minclock = minclock
	self.reqclock = reqclock
}

type fakeAccelQueryCommand struct {
	data     interface{}
	minclock int64
	reqclock int64
	response interface{}
}

func (self *fakeAccelQueryCommand) Send(data interface{}, minclock, reqclock int64) interface{} {
	self.data = data
	self.minclock = minclock
	self.reqclock = reqclock
	return self.response
}

func TestBuildAxesMapTrimsAxes(t *testing.T) {
	mapped := BuildAxesMap([]string{" x", "-y ", "z"}, 10, 20, "adxl345")
	want := [][]float64{{0, 10}, {1, -10}, {2, 20}}
	if !reflect.DeepEqual(mapped, want) {
		t.Fatalf("BuildAxesMap() = %#v, want %#v", mapped, want)
	}
}

func TestAccelerometerRuntimeStartAndFinishMeasurements(t *testing.T) {
	mcu := &fakeAccelerometerMCU{monotonic: 10, estimatedOffset: 1, clockMultiplier: 1000, secondsMultiplier: 1000}
	runtime := NewAccelerometerRuntime(AccelerometerRuntimeConfig{
		Name:     "chip",
		AxesMap:  [][]float64{{0, 1}, {1, 1}, {2, 1}},
		DataRate: 400,
		SPI:      &fakeAccelerometerSPI{},
		MCU:      mcu,
		OID:      7,
	})
	startCmd := &fakeAccelCommand{}
	endCmd := &fakeAccelQueryCommand{}
	runtime.SetCommands(startCmd, endCmd, &fakeAccelQueryCommand{})
	runtime.rawSamples = []map[string]interface{}{{"stale": true}}
	var syncedClock int64
	if err := runtime.StartMeasurements(0.1, nil, func(reqclock int64) error {
		syncedClock = reqclock
		return nil
	}); err != nil {
		t.Fatalf("StartMeasurements() error = %v", err)
	}
	if !runtime.IsMeasuring() {
		t.Fatal("runtime should be measuring after StartMeasurements")
	}
	if syncedClock != 11100 {
		t.Fatalf("synced clock = %d, want 11100", syncedClock)
	}
	wantStart := []int64{7, 11100, 10}
	if !reflect.DeepEqual(startCmd.data, wantStart) {
		t.Fatalf("start command = %#v, want %#v", startCmd.data, wantStart)
	}
	if len(runtime.rawSamples) != 0 {
		t.Fatalf("raw samples should be cleared, got %d entries", len(runtime.rawSamples))
	}
	runtime.rawSamples = []map[string]interface{}{{"live": true}}
	runtime.FinishMeasurements()
	if runtime.IsMeasuring() {
		t.Fatal("runtime should stop measuring after FinishMeasurements")
	}
	wantStop := []int64{7, 0, 0}
	if !reflect.DeepEqual(endCmd.data, wantStop) {
		t.Fatalf("stop command = %#v, want %#v", endCmd.data, wantStop)
	}
	if len(runtime.rawSamples) != 0 {
		t.Fatalf("raw samples should be cleared on stop, got %d entries", len(runtime.rawSamples))
	}
}

func TestAccelerometerRuntimeAPIUpdateReturnsSamplesAndClearsBuffer(t *testing.T) {
	runtime := NewAccelerometerRuntime(AccelerometerRuntimeConfig{
		Name:     "chip",
		AxesMap:  [][]float64{{0, 1}, {1, 1}, {2, 1}},
		DataRate: 400,
		SPI:      &fakeAccelerometerSPI{},
		MCU:      &fakeAccelerometerMCU{clockMultiplier: 1000, secondsMultiplier: 1000},
		OID:      7,
	})
	runtime.rawSamples = []map[string]interface{}{{"sample": 1}}
	runtime.lastErrorCount = 3
	runtime.lastLimitCount = 4
	updateCalls := 0
	msg := runtime.APIUpdate(func(int64) error {
		updateCalls++
		return nil
	}, func(rawSamples []map[string]interface{}) [][]float64 {
		if len(rawSamples) != 1 {
			t.Fatalf("extract received %d samples, want 1", len(rawSamples))
		}
		return [][]float64{{1, 2, 3, 4}}
	})
	if updateCalls != 1 {
		t.Fatalf("update clock calls = %d, want 1", updateCalls)
	}
	if len(runtime.rawSamples) != 0 {
		t.Fatalf("raw samples should be cleared after APIUpdate, got %d entries", len(runtime.rawSamples))
	}
	if got := msg["errors"]; got != 3 {
		t.Fatalf("errors = %#v, want 3", got)
	}
	if got := msg["overflows"]; got != 4 {
		t.Fatalf("overflows = %#v, want 4", got)
	}
	data, ok := msg["data"].([][]float64)
	if !ok {
		t.Fatalf("data has unexpected type %T", msg["data"])
	}
	if !reflect.DeepEqual(data, [][]float64{{1, 2, 3, 4}}) {
		t.Fatalf("data = %#v, want %#v", data, [][]float64{{1, 2, 3, 4}})
	}
}
