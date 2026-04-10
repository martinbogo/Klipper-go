package vibration

import (
	"reflect"
	"testing"
)

type fakeAccelClockQueryCall struct {
	data     interface{}
	minclock int64
	reqclock int64
}

type fakeAccelClockQuery struct {
	responses []interface{}
	calls     []fakeAccelClockQueryCall
}

func (self *fakeAccelClockQuery) Send(data interface{}, minclock, reqclock int64) interface{} {
	self.calls = append(self.calls, fakeAccelClockQueryCall{data: data, minclock: minclock, reqclock: reqclock})
	if len(self.responses) == 0 {
		return nil
	}
	response := self.responses[0]
	self.responses = self.responses[1:]
	return response
}

type fakeAccelClockUpdateMCU struct {
	clock64Offset   int64
	clocksPerSecond int64
}

func (self *fakeAccelClockUpdateMCU) Clock32_to_clock64(clock int64) int64 {
	return clock + self.clock64Offset
}

func (self *fakeAccelClockUpdateMCU) Seconds_to_clock(seconds float64) int64 {
	return int64(seconds * float64(self.clocksPerSecond))
}

func (self *fakeAccelClockUpdateMCU) Clock_to_print_time(clock int64) float64 {
	return float64(clock) / float64(self.clocksPerSecond)
}

func TestUpdateAccelClockUpdatesClockSyncState(t *testing.T) {
	query := &fakeAccelClockQuery{responses: []interface{}{
		map[string]interface{}{
			"fifo":          int64(4),
			"clock":         int64(200),
			"next_sequence": int64(7),
			"buffered":      int64(12),
			"limit_count":   int64(3),
			"query_ticks":   int64(20),
		},
	}}
	mcu := &fakeAccelClockUpdateMCU{clock64Offset: 1000, clocksPerSecond: 1000000}
	clockSync := NewClockSyncRegression(mcu, 640, 0.5)
	clockSync.Reset(1000, 0)

	result, err := UpdateAccelClock(query, mcu, clockSync, AccelClockSyncConfig{
		OID:             11,
		Label:           "lis2dw12",
		BytesPerSample:  6,
		SamplesPerBlock: 8,
	}, 55, AccelClockSyncState{MaxQueryDuration: 100})
	if err != nil {
		t.Fatalf("UpdateAccelClock returned error: %v", err)
	}
	if got, want := len(query.calls), 1; got != want {
		t.Fatalf("expected %d query call, got %d", want, got)
	}
	if got, want := query.calls[0].data, []int64{11}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected query payload %#v, want %#v", got, want)
	}
	if query.calls[0].minclock != 55 {
		t.Fatalf("expected minclock 55, got %d", query.calls[0].minclock)
	}
	if result.LastSequence != 7 {
		t.Fatalf("expected last sequence 7, got %d", result.LastSequence)
	}
	if result.LastLimitCount != 3 {
		t.Fatalf("expected last limit count 3, got %d", result.LastLimitCount)
	}
	if result.MaxQueryDuration != 40 {
		t.Fatalf("expected max query duration 40, got %d", result.MaxQueryDuration)
	}
	if clockSync.mcu_clock_variance == 0 {
		t.Fatal("expected clock sync regression to update")
	}
}

func TestUpdateAccelClockRetriesUntilFIFOIsReady(t *testing.T) {
	query := &fakeAccelClockQuery{responses: []interface{}{
		map[string]interface{}{
			"fifo":          int64(40),
			"clock":         int64(100),
			"next_sequence": int64(1),
			"buffered":      int64(0),
			"limit_count":   int64(0),
			"query_ticks":   int64(5),
		},
		map[string]interface{}{
			"fifo":          int64(8),
			"clock":         int64(120),
			"next_sequence": int64(2),
			"buffered":      int64(10),
			"limit_count":   int64(1),
			"query_ticks":   int64(5),
		},
	}}
	mcu := &fakeAccelClockUpdateMCU{clocksPerSecond: 1000000}
	clockSync := NewClockSyncRegression(mcu, 640, 0.5)
	clockSync.Reset(100, 0)

	result, err := UpdateAccelClock(query, mcu, clockSync, AccelClockSyncConfig{
		OID:             9,
		Label:           "adxl345",
		BytesPerSample:  5,
		SamplesPerBlock: 10,
	}, 0, AccelClockSyncState{MaxQueryDuration: 100})
	if err != nil {
		t.Fatalf("UpdateAccelClock returned error: %v", err)
	}
	if got, want := len(query.calls), 2; got != want {
		t.Fatalf("expected %d query calls, got %d", want, got)
	}
	if result.LastSequence != 2 {
		t.Fatalf("expected last sequence 2, got %d", result.LastSequence)
	}
	if result.LastLimitCount != 1 {
		t.Fatalf("expected last limit count 1, got %d", result.LastLimitCount)
	}
}

func TestUpdateAccelClockSkipsLongQueriesAndWrapsCounters(t *testing.T) {
	query := &fakeAccelClockQuery{responses: []interface{}{
		map[string]interface{}{
			"fifo":          int64(1),
			"clock":         int64(10),
			"next_sequence": int64(1),
			"buffered":      int64(0),
			"limit_count":   int64(2),
			"query_ticks":   int64(25),
		},
	}}
	mcu := &fakeAccelClockUpdateMCU{clocksPerSecond: 1000000}
	clockSync := NewClockSyncRegression(mcu, 640, 0.5)
	clockSync.Reset(10, 0)

	result, err := UpdateAccelClock(query, mcu, clockSync, AccelClockSyncConfig{
		OID:             7,
		Label:           "lis2dw12",
		BytesPerSample:  6,
		SamplesPerBlock: 8,
	}, 0, AccelClockSyncState{LastSequence: 0xffff, LastLimitCount: 0xffff, MaxQueryDuration: 10})
	if err != nil {
		t.Fatalf("UpdateAccelClock returned error: %v", err)
	}
	if result.LastSequence != 0x10001 {
		t.Fatalf("expected wrapped sequence 0x10001, got %#x", result.LastSequence)
	}
	if result.LastLimitCount != 0x10002 {
		t.Fatalf("expected wrapped limit count 0x10002, got %#x", result.LastLimitCount)
	}
	if result.MaxQueryDuration != 20 {
		t.Fatalf("expected max query duration 20, got %d", result.MaxQueryDuration)
	}
	if clockSync.mcu_clock_variance != 0 {
		t.Fatal("expected long query to skip clock sync update")
	}
}

func TestUpdateAccelClockErrorsWhenFIFOStaysBusy(t *testing.T) {
	query := &fakeAccelClockQuery{responses: []interface{}{
		map[string]interface{}{"fifo": int64(40)},
		map[string]interface{}{"fifo": int64(41)},
		map[string]interface{}{"fifo": int64(42)},
		map[string]interface{}{"fifo": int64(43)},
		map[string]interface{}{"fifo": int64(44)},
	}}
	mcu := &fakeAccelClockUpdateMCU{clocksPerSecond: 1000000}
	clockSync := NewClockSyncRegression(mcu, 640, 0.5)
	clockSync.Reset(10, 0)

	_, err := UpdateAccelClock(query, mcu, clockSync, AccelClockSyncConfig{OID: 5, Label: "adxl345", BytesPerSample: 5, SamplesPerBlock: 10}, 0, AccelClockSyncState{MaxQueryDuration: 10})
	if err == nil {
		t.Fatal("expected fifo busy error")
	}
	if err.Error() != "Unable to query adxl345 fifo" {
		t.Fatalf("unexpected error %q", err.Error())
	}
	if got, want := len(query.calls), 5; got != want {
		t.Fatalf("expected %d query calls, got %d", want, got)
	}
}
