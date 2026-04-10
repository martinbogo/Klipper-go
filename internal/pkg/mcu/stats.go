package mcu

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"goklipper/common/utils/cast"
)

type StatsState struct {
	TickAvg    float64
	TickStddev float64
	TickAwake  float64
}

func (self *StatsState) HandleMCUStats(params map[string]interface{}, mcuFreq float64, statsSumsqBase float64) {
	count := cast.ToFloat64(params["count"])
	tickSum := cast.ToFloat64(params["sum"])
	tickSumsq := cast.ToFloat64(params["sumsq"])
	c := 1.0 / (count * mcuFreq)
	self.TickAvg = tickSum * c
	tickSumsq = tickSumsq * statsSumsqBase
	diff := count*tickSumsq - tickSum*tickSum
	self.TickStddev = c * math.Sqrt(math.Max(0.0, diff))
	self.TickAwake = tickSum / mcuFreq
}

func (self *StatsState) BuildStatsSummary(name string, serialStats string, clockSyncStats string) (bool, string, map[string]interface{}) {
	load := fmt.Sprintf("mcu_awake=%.03f mcu_task_avg=%.06f mcu_task_stddev=%.06f", self.TickAwake, self.TickAvg, self.TickStddev)
	stats := fmt.Sprintf("%s %s %s", load, serialStats, clockSyncStats)
	parts := map[string]string{}
	for _, item := range strings.Split(stats, " ") {
		spl := strings.Split(item, "=")
		if len(spl) != 2 {
			continue
		}
		parts[spl[0]] = spl[1]
	}
	lastStats := map[string]interface{}{}
	for key, value := range parts {
		if strings.Contains(value, ".") {
			parsed, err := strconv.ParseFloat(value, 64)
			if err == nil {
				lastStats[key] = parsed
			}
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil {
			lastStats[key] = parsed
		}
	}
	return true, fmt.Sprintf("%s: %s", name, stats), lastStats
}

type ReadyFrequencyCheck struct {
	Skip        bool
	IsMismatch  bool
	MCUFreqMHz  int64
	CalcFreqMHz int64
}

func BuildReadyFrequencyCheck(isFileoutput bool, mcuFreq float64, systime float64, getClock func(float64) int64) ReadyFrequencyCheck {
	if isFileoutput {
		return ReadyFrequencyCheck{Skip: true}
	}
	calcFreq := getClock(systime+1) - getClock(systime)
	freqDiff := math.Abs(mcuFreq - float64(calcFreq))
	mcuFreqMHz := int64(mcuFreq/1000000.0 + 0.5)
	calcFreqMHz := int64(float64(calcFreq)/1000000.0 + 0.5)
	return ReadyFrequencyCheck{
		MCUFreqMHz:  mcuFreqMHz,
		CalcFreqMHz: calcFreqMHz,
		IsMismatch:  mcuFreqMHz != calcFreqMHz && freqDiff > mcuFreq*0.01,
	}
}
