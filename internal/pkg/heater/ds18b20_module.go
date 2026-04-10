package heater

import (
	"fmt"

	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

const (
	DS18ReportTime           = 3.0
	DS18MinReportTime        = 1.0
	DS18MaxConsecutiveErrors = 4
)

type DS18B20Sensor struct {
	sensorID    string
	temp        float64
	minTemp     float64
	maxTemp     float64
	reportClock int64
	reportTime  float64
	mcu         printerpkg.MCURuntime
	oid         int
	callback    func(float64, float64)
}

var _ printerpkg.TemperatureSensor = (*DS18B20Sensor)(nil)

func NewDS18B20Sensor(config printerpkg.ModuleConfig) *DS18B20Sensor {
	reportTime := config.Float("ds18_report_time", DS18ReportTime)
	if reportTime < DS18MinReportTime {
		panic(fmt.Sprintf("Option 'ds18_report_time' in section '%s' must have minimum of %f", config.Name(), DS18MinReportTime))
	}
	mcuName := config.String("sensor_mcu", "mcu", true)
	self := &DS18B20Sensor{
		sensorID:   config.String("serial_no", "", true),
		temp:       0.0,
		minTemp:    0.0,
		maxTemp:    0.0,
		reportTime: reportTime,
		mcu:        config.Printer().LookupMCU(mcuName),
	}
	self.oid = self.mcu.CreateOID()
	self.mcu.RegisterResponse(self.handleResponse, "ds18b20_result", self.oid)
	self.mcu.RegisterConfigCallback(self.buildConfig)
	return self
}

func (self *DS18B20Sensor) buildConfig() {
	sid := fmt.Sprintf("%x", self.sensorID)
	self.mcu.AddConfigCmd(
		fmt.Sprintf("config_ds18b20 oid=%d serial=%s max_error_count=%d",
			self.oid, sid, DS18MaxConsecutiveErrors),
		false, false)

	clock := self.mcu.GetQuerySlot(self.oid)
	self.reportClock = self.mcu.SecondsToClock(self.reportTime)
	self.mcu.AddConfigCmd(
		fmt.Sprintf("query_ds18b20 oid=%d clock=%d rest_ticks=%d min_value=%d max_value=%d",
			self.oid, clock, self.reportClock,
			int(self.minTemp*1000), int(self.maxTemp*1000)),
		true, false)
}

func (self *DS18B20Sensor) handleResponse(params map[string]interface{}) error {
	temp := params["value"].(float64) / 1000.0
	if fault, ok := params["fault"]; ok && fault != 0 {
		logger.Infof("ds18b20 reports fault %v (temp=%.1f)", fault, temp)
		return nil
	}
	self.temp = temp
	nextClock := self.mcu.Clock32ToClock64(params["next_clock"].(int64))
	lastReadClock := nextClock - self.reportClock
	lastReadTime := self.mcu.ClockToPrintTime(lastReadClock)
	if self.callback != nil {
		self.callback(lastReadTime, temp)
	}
	return nil
}

func (self *DS18B20Sensor) SetupMinMax(minTemp float64, maxTemp float64) {
	self.minTemp = minTemp
	self.maxTemp = maxTemp
}

func (self *DS18B20Sensor) SetupCallback(callback func(float64, float64)) {
	self.callback = callback
}

func (self *DS18B20Sensor) GetReportTimeDelta() float64 {
	return self.reportTime
}

func LoadConfigDS18B20(config printerpkg.ModuleConfig) interface{} {
	config.Printer().TemperatureSensors().AddSensorFactory(
		"DS18B20",
		func(section printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
			return NewDS18B20Sensor(section)
		},
	)
	return nil
}
