package vibration

import (
	"fmt"
	"math"
)

type accelClockStatusQuery interface {
	Send(data interface{}, minclock, reqclock int64) interface{}
}

type accelClockUpdateMCU interface {
	Clock32_to_clock64(clock int64) int64
	Seconds_to_clock(seconds float64) int64
}

type AccelClockSyncConfig struct {
	OID             int
	Label           string
	BytesPerSample  int
	SamplesPerBlock int
}

type AccelClockSyncState struct {
	LastSequence     int
	LastLimitCount   int
	MaxQueryDuration int64
}

type AccelClockSyncResult struct {
	LastSequence     int
	LastLimitCount   int
	MaxQueryDuration int64
}

func UpdateAccelClock(query accelClockStatusQuery, mcu accelClockUpdateMCU, clockSync *ClockSyncRegression, config AccelClockSyncConfig, minclock int64, state AccelClockSyncState) (AccelClockSyncResult, error) {
	var params map[string]interface{}
	isDone := false
	for retry := 0; retry < 5; retry++ {
		response := query.Send([]int64{int64(config.OID)}, minclock, 0)
		if response == nil {
			continue
		}
		params = response.(map[string]interface{})
		fifo := int(params["fifo"].(int64)) & 0x7f
		if fifo <= 32 {
			isDone = true
			break
		}
	}
	if !isDone {
		return AccelClockSyncResult{}, fmt.Errorf("Unable to query %s fifo", config.Label)
	}

	mcuClock := mcu.Clock32_to_clock64(params["clock"].(int64))
	sequence := state.LastSequence&(^0xffff) | int(params["next_sequence"].(int64))
	if sequence < state.LastSequence {
		sequence += 0x10000
	}
	limitCount := state.LastLimitCount&(^0xffff) | int(params["limit_count"].(int64))
	if limitCount < state.LastLimitCount {
		limitCount += 0x10000
	}
	duration := params["query_ticks"].(int64)
	if duration > state.MaxQueryDuration {
		return AccelClockSyncResult{
			LastSequence:     sequence,
			LastLimitCount:   limitCount,
			MaxQueryDuration: int64(math.Max(float64(2*state.MaxQueryDuration), float64(mcu.Seconds_to_clock(.000005)))),
		}, nil
	}

	fifo := int(params["fifo"].(int64)) & 0x7f
	buffered := int(params["buffered"].(int64))
	msgCount := sequence*config.SamplesPerBlock + buffered/config.BytesPerSample + fifo
	chipClock := msgCount + 1
	clockSync.Update(float64(int64(mcuClock)+duration/2), float64(chipClock))
	return AccelClockSyncResult{
		LastSequence:     sequence,
		LastLimitCount:   limitCount,
		MaxQueryDuration: 2 * duration,
	}, nil
}
