package heater

import (
	"fmt"
	"strings"

	printerpkg "goklipper/internal/pkg/printer"
)

const (
	ADCSampleTime      = 0.001
	ADCSampleCount     = 8
	ADCReportTime      = 0.300
	ADCRangeCheckCount = 4
)

type PrinterADCtoTemperature struct {
	AdcConvert          Linear
	AdcPin              printerpkg.ADCPin
	TemperatureCallback func(float64, float64)
}

var _ printerpkg.TemperatureSensor = (*PrinterADCtoTemperature)(nil)

func NewPrinterADCtoTemperature(config printerpkg.ModuleConfig, adcConvert Linear) *PrinterADCtoTemperature {
	printer := config.Printer()
	pinsObject := printer.LookupObject("pins", nil)
	pins, ok := pinsObject.(printerpkg.PinRegistry)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement printer.PinRegistry: %T", pinsObject))
	}
	adcPin := pins.SetupADC(config.String("sensor_pin", "", true))
	self := &PrinterADCtoTemperature{
		AdcConvert: adcConvert,
		AdcPin:     adcPin,
	}
	adcPin.SetupCallback(ADCReportTime, self.AdcCallback)
	queryObject := config.LoadObject("query_adc")
	queryRegistry, ok := queryObject.(printerpkg.ADCQueryRegistry)
	if !ok {
		panic(fmt.Sprintf("query_adc object does not implement printer.ADCQueryRegistry: %T", queryObject))
	}
	queryRegistry.RegisterADC(config.Name(), adcPin)
	return self
	}

func (self *PrinterADCtoTemperature) SetupCallback(callback func(float64, float64)) {
	self.TemperatureCallback = callback
}

func (self *PrinterADCtoTemperature) Setup_callback(callback func(float64, float64)) {
	self.SetupCallback(callback)
}

func (self *PrinterADCtoTemperature) GetReportTimeDelta() float64 {
	return ADCReportTime
}

func (self *PrinterADCtoTemperature) Get_report_time_delta() float64 {
	return self.GetReportTimeDelta()
}

func (self *PrinterADCtoTemperature) AdcCallback(readTime float64, readValue float64) {
	if self.TemperatureCallback == nil {
		return
	}
	temp := self.AdcConvert.Calc_temp(readValue)
	self.TemperatureCallback(readTime+ADCSampleCount*ADCSampleTime, temp)
}

func (self *PrinterADCtoTemperature) Adc_callback(readTime float64, readValue float64) {
	self.AdcCallback(readTime, readValue)
}

func (self *PrinterADCtoTemperature) SetupMinMax(minTemp float64, maxTemp float64) {
	val1 := self.AdcConvert.Calc_adc(minTemp)
	val2 := self.AdcConvert.Calc_adc(maxTemp)
	minVal := val1
	maxVal := val2
	if val2 < val1 {
		minVal = val2
		maxVal = val1
	}
	self.AdcPin.SetupMinMax(ADCSampleTime, ADCSampleCount, minVal, maxVal, ADCRangeCheckCount)
}

func (self *PrinterADCtoTemperature) Setup_minmax(minTemp float64, maxTemp float64) {
	self.SetupMinMax(minTemp, maxTemp)
}

func (self *PrinterADCtoTemperature) Stop() {}

func configSuffixName(sectionName string) string {
	parts := strings.Split(sectionName, " ")
	if len(parts) <= 1 {
		return sectionName
	}
	return strings.Join(parts[1:], " ")
}

func buildLinearVoltage(config printerpkg.ModuleConfig, params [][]float64) (Linear, error) {
	return NewLinearVoltage(
		config.Float("adc_voltage", 5.0),
		config.Float("voltage_offset", 5.0),
		params,
		config.Name(),
	)
}

func buildLinearResistance(config printerpkg.ModuleConfig, samples [][]float64) (Linear, error) {
	return NewLinearResistance(config.Float("pullup_resistor", 4700.0), samples, config.Name())
}

type customLinearSensor interface {
	Name() string
	Create(printerpkg.ModuleConfig) printerpkg.TemperatureSensor
}

type CustomLinearVoltage struct {
	name   string
	params [][]float64
}

func NewCustomLinearVoltage(config printerpkg.ModuleConfig) *CustomLinearVoltage {
	self := &CustomLinearVoltage{name: configSuffixName(config.Name())}
	for i := 1; i < 1000; i++ {
		temp := config.OptionalFloat(fmt.Sprintf("temperature%d", i))
		if temp == nil {
			break
		}
		voltage := config.OptionalFloat(fmt.Sprintf("voltage%d", i))
		if voltage == nil {
			panic(fmt.Sprintf("missing voltage%d in section %s", i, config.Name()))
		}
		self.params = append(self.params, []float64{*temp, *voltage})
	}
	return self
}

func (self *CustomLinearVoltage) Name() string {
	return self.name
}

func (self *CustomLinearVoltage) Create(config printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
	linearVoltage, err := buildLinearVoltage(config, self.params)
	if err != nil {
		panic(err)
	}
	return NewPrinterADCtoTemperature(config, linearVoltage)
}

type CustomLinearResistance struct {
	name    string
	samples [][]float64
}

func NewCustomLinearResistance(config printerpkg.ModuleConfig) *CustomLinearResistance {
	self := &CustomLinearResistance{name: configSuffixName(config.Name())}
	for i := 1; i < 1000; i++ {
		temp := config.OptionalFloat(fmt.Sprintf("temperature%d", i))
		if temp == nil {
			break
		}
		resistance := config.OptionalFloat(fmt.Sprintf("resistance%d", i))
		if resistance == nil {
			panic(fmt.Sprintf("missing resistance%d in section %s", i, config.Name()))
		}
		self.samples = append(self.samples, []float64{*temp, *resistance})
	}
	return self
}

func (self *CustomLinearResistance) Name() string {
	return self.name
}

func (self *CustomLinearResistance) Create(config printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
	linearResistance, err := buildLinearResistance(config, self.samples)
	if err != nil {
		panic(err)
	}
	return NewPrinterADCtoTemperature(config, linearResistance)
}

func LoadConfigADCTemperature(config printerpkg.ModuleConfig) interface{} {
	registry := config.Printer().TemperatureSensors()
	for _, item := range DefaultVoltageSensors {
		sensorType := item.Sensor_type
		params := item.Params
		registry.AddSensorFactory(sensorType, func(section printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
			linearVoltage, err := buildLinearVoltage(section, params)
			if err != nil {
				panic(err)
			}
			return NewPrinterADCtoTemperature(section, linearVoltage)
		})
	}
	for _, item := range DefaultResistanceSensors {
		sensorType := item.Sensor_type
		samples := item.Params
		registry.AddSensorFactory(sensorType, func(section printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
			linearResistance, err := buildLinearResistance(section, samples)
			if err != nil {
				panic(err)
			}
			return NewPrinterADCtoTemperature(section, linearResistance)
		})
	}
	return nil
}

func LoadConfigPrefixCustomLinear(config printerpkg.ModuleConfig) interface{} {
	var customSensor customLinearSensor
	if config.OptionalFloat("resistance1") == nil {
		customSensor = NewCustomLinearVoltage(config)
	} else {
		customSensor = NewCustomLinearResistance(config)
	}
	config.Printer().TemperatureSensors().AddSensorFactory(customSensor.Name(), customSensor.Create)
	return nil
}