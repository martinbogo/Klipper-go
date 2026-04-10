package printer

import (
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/utils/sys"
	"os"
	"reflect"
	"strconv"
	"strings"
)

type SystemStats struct {
	lastProcessTime  float64
	totalProcessTime float64
	lastLoadAvg      float64
	lastMemAvail     int
	memFile          *os.File
}

func NewSystemStats(meminfoPath string) *SystemStats {
	self := &SystemStats{}
	self.memFile, _ = os.Open(meminfoPath)
	return self
}

func (self *SystemStats) Close() error {
	if self.memFile != nil {
		err := self.memFile.Close()
		self.memFile = nil
		return err
	}
	return nil
}

func (self *SystemStats) Stats(eventtime float64) (bool, string) {
	pidStat, _ := sys.PidStat()
	ptime := pidStat.Stime + pidStat.Utime
	pdiff := ptime - self.lastProcessTime
	self.lastProcessTime = ptime
	if pdiff > 0.0 {
		self.totalProcessTime += pdiff
	}

	loadavg, _ := sys.Loadavg()
	self.lastLoadAvg = loadavg.Load1
	msg := fmt.Sprintf("sysload=%.2f cputime=%.3f", self.lastLoadAvg,
		self.totalProcessTime)

	if self.memFile != nil {
		data, err := os.ReadFile(self.memFile.Name())
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemAvailable:") {
					self.lastMemAvail, _ = strconv.Atoi(strings.Fields(line)[1])
					msg = fmt.Sprintf("%s memavail=%d", msg, self.lastMemAvail)
					break
				}
			}
		}
	}
	return true, msg
}

func (self *SystemStats) GetStatus(eventtime float64) map[string]float64 {
	return map[string]float64{
		"sysload":  self.lastLoadAvg,
		"cputime":  self.totalProcessTime,
		"memavail": float64(self.lastMemAvail),
	}
}

type StatsCollector struct {
	statsCallbacks []reflect.Value
}

func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		statsCallbacks: make([]reflect.Value, 0),
	}
}

func (self *StatsCollector) RegisterObjects(objects []interface{}) {
	for _, obj := range objects {
		self.RegisterObject(obj)
	}
}

func (self *StatsCollector) RegisterObject(obj interface{}) {
	if obj == nil {
		return
	}
	reflectValue := reflect.ValueOf(obj)
	method := reflectValue.MethodByName("Stats")
	if method.Kind() == reflect.Func {
		self.statsCallbacks = append(self.statsCallbacks, method)
		return
	}
	if reflectValue.Kind() == reflect.Ptr {
		field := reflectValue.Elem().FieldByName("Stats")
		if field.Kind() == reflect.Func {
			self.statsCallbacks = append(self.statsCallbacks, field)
		}
	}
}

func (self *StatsCollector) GenerateStats(eventtime float64, isShutdown bool) (float64, string, bool) {
	defer sys.CatchPanic()
	stats := make([]string, 0)
	for _, cb := range self.statsCallbacks {
		values := cb.Call([]reflect.Value{reflect.ValueOf(eventtime)})
		if values[0].Interface().(bool) {
			stats = append(stats, values[1].Interface().(string))
		}
	}

	if len(stats) != 0 && isShutdown {
		return constants.NEVER, fmt.Sprintf("Stats %.1f: %s", eventtime, strings.Join(stats, " ")), true
	}
	return eventtime + 1, "", false
}
