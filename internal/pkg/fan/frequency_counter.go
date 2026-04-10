package fan

import (
	"fmt"

	printerpkg "goklipper/internal/pkg/printer"
)

type counterPinLookup interface {
	LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{}
}

type MCUCounter struct {
	mcu        printerpkg.MCURuntime
	oid        int
	pin        string
	pullup     int
	pollTime   float64
	pollTicks  int64
	sampleTime float64
	callback   func(float64, int64, float64)
	lastCount  int64
}

func NewMCUCounter(printer printerpkg.ModulePrinter, pinLookup counterPinLookup,
	pin string, sampleTime float64, pollTime float64) *MCUCounter {
	pinParams := pinLookup.LookupPin(pin, true, false, nil)
	mcuName, ok := pinParams["chip_name"].(string)
	if !ok {
		mcuName = fmt.Sprintf("%v", pinParams["chip_name"])
	}
	self := &MCUCounter{
		mcu:        printer.LookupMCU(mcuName),
		oid:        0,
		pin:        fmt.Sprintf("%v", pinParams["pin"]),
		pullup:     lookupPinPullup(pinParams["pullup"]),
		pollTime:   pollTime,
		pollTicks:  0,
		sampleTime: sampleTime,
		callback:   nil,
		lastCount:  0,
	}
	self.oid = self.mcu.CreateOID()
	self.mcu.RegisterConfigCallback(self.BuildConfig)
	return self
}

func lookupPinPullup(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case bool:
		if typed {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func (self *MCUCounter) BuildConfig() {
	self.mcu.AddConfigCmd(fmt.Sprintf("config_counter oid=%d pin=%v pull_up=%v",
		self.oid, self.pin, self.pullup), false, false)
	clock := self.mcu.GetQuerySlot(self.oid)
	self.pollTicks = self.mcu.SecondsToClock(self.pollTime)
	sampleTicks := self.mcu.SecondsToClock(self.sampleTime)
	self.mcu.AddConfigCmd(fmt.Sprintf("query_counter oid=%d clock=%d poll_ticks=%d sample_ticks=%d",
		self.oid, clock, self.pollTicks, sampleTicks), true, false)
	self.mcu.RegisterResponse(self.HandleCounterState, "counter_state", self.oid)
}

func (self *MCUCounter) SetupCallback(cb func(float64, int64, float64)) {
	self.callback = cb
}

func (self *MCUCounter) HandleCounterState(params map[string]interface{}) error {
	nextClock := self.mcu.Clock32ToClock64(params["next_clock"].(int64))
	time := self.mcu.ClockToPrintTime(nextClock - self.pollTicks)
	countClock := self.mcu.Clock32ToClock64(params["count_clock"].(int64))
	countTime := self.mcu.ClockToPrintTime(countClock)
	lastCount := self.lastCount
	deltaCount := (params["count"].(int64) - lastCount) & 0xffffffff
	count := lastCount + deltaCount
	self.lastCount = count
	if self.callback != nil {
		self.callback(time, count, countTime)
	}
	return nil
}

type FrequencyCounter struct {
	callback  func(float64, float64)
	lastTime  float64
	lastCount int64
	freq      float64
	counter   *MCUCounter
}

func NewFrequencyCounter(printer printerpkg.ModulePrinter, pinLookup counterPinLookup,
	pin string, sampleTime float64, pollTime float64) *FrequencyCounter {
	self := &FrequencyCounter{
		callback:  nil,
		lastTime:  0,
		lastCount: 0,
		freq:      0.0,
	}
	self.counter = NewMCUCounter(printer, pinLookup, pin, sampleTime, pollTime)
	self.counter.SetupCallback(self.counterCallback)
	return self
}

func (self *FrequencyCounter) counterCallback(time float64, count int64, countTime float64) {
	if self.lastTime == 0 {
		self.lastTime = time
	} else {
		deltaTime := countTime - self.lastTime
		if deltaTime > 0 {
			self.lastTime = countTime
			deltaCount := count - self.lastCount
			self.freq = float64(deltaCount) / deltaTime
		} else {
			self.lastTime = time
			self.freq = 0.0
		}
		if self.callback != nil {
			self.callback(time, self.freq)
		}
	}
	self.lastCount = count
}

func (self *FrequencyCounter) Get_frequency() float64 {
	return self.freq
}
