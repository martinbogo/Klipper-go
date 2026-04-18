package heater

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/collections"
	"goklipper/common/utils/str"
	printerpkg "goklipper/internal/pkg/printer"
)

const (
	KELVIN_TO_CELSIUS = -273.15
	MAX_HEAT_TIME     = 5.0
	AMBIENT_TEMP      = 25.
	PID_PARAM_BASE    = 255.
)

const (
	PID_SETTLE_DELTA = 1.
	PID_SETTLE_SLOPE = .1
)

const cmd_SET_HEATER_TEMPERATURE_help = "Sets a heater temperature"
const cmd_TURN_OFF_HEATERS_help = "Turn off all heaters"
const cmd_TEMPERATURE_WAIT_help = "Wait for a temperature on a sensor"

type heaterPWMPin interface {
	MCU() interface{}
	SetupMaxDuration(maxDuration float64)
	SetupCycleTime(cycleTime float64, hardwarePWM bool)
	SetPWM(printTime float64, value float64)
}

type heaterMCUEstimator interface {
	EstimatedPrintTime(eventtime float64) float64
}

type heaterPinRegistry interface {
	SetupPWM(pin string) interface{}
}

type heaterToolhead interface {
	Get_last_move_time() float64
	RegisterLookaheadCallback(callback func(float64))
}

type heaterReactor interface {
	printerpkg.ModuleReactor
	Pause(waketime float64) float64
}

type heaterGCodeRuntime interface {
	printerpkg.GCodeRuntime
	RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string)
	RespondRaw(msg string)
}

type heaterCommandAck interface {
	Ack(msg string) bool
}

type heaterManager interface {
	Set_temperature(heater interface{}, temp float64, wait bool) error
}

type directHeaterManager interface {
	Set_temperature(heater *Heater, temp float64, wait bool) error
}

type temperatureSensorFactory = printerpkg.TemperatureSensorFactory

type Heater struct {
	Printer          printerpkg.ModulePrinter
	Name             string
	Sensor           printerpkg.TemperatureSensor
	Min_temp         float64
	Max_temp         float64
	Pwm_delay        float64
	Min_extrude_temp float64
	Can_extrude      bool
	Max_power        float64
	Smooth_time      float64
	Inv_smooth_time  float64
	Lock             sync.Mutex
	Last_temp        float64
	Smoothed_temp    float64
	Target_temp      float64
	Last_temp_time   float64
	Next_pwm_time    float64
	Last_pwm_value   float64
	Control          interface{}
	Mcu_pwm          interface{}
}

type iTemperature_update interface {
	Temperature_update(read_time float64, temp float64, target_temp float64)
}

type CheckBusyer interface {
	Check_busy(float64, float64, float64) bool
}

func normalizeTemperatureSensorFactory(sensorFactory interface{}) temperatureSensorFactory {
	switch factory := sensorFactory.(type) {
	case temperatureSensorFactory:
		return factory
	case func(printerpkg.ModuleConfig) printerpkg.TemperatureSensor:
		return factory
	default:
		panic(fmt.Sprintf("unsupported temperature sensor factory type %T", sensorFactory))
	}
}

func requireConfigOption(config printerpkg.ModuleConfig, option string) {
	extended, ok := config.(interface{ HasOption(string) bool })
	if ok && !extended.HasOption(option) {
		panic(fmt.Sprintf("Option '%s' in section '%s' must be specified", option, config.Name()))
	}
}

func validateFloatOption(section string, option string, value float64, minval float64, maxval float64, above float64, below float64) {
	if minval != 0 && value < minval {
		panic(fmt.Sprintf("Option '%s' in section '%s' must have minimum of %f", option, section, minval))
	}
	if maxval != 0 && value > maxval {
		panic(fmt.Sprintf("Option '%s' in section '%s' must have maximum of %f", option, section, maxval))
	}
	if above != 0 && value <= above {
		panic(fmt.Sprintf("Option '%s' in section '%s' must be above %f", option, section, above))
	}
	if below != 0 && value >= below {
		panic(fmt.Sprintf("Option '%s' in section '%s' must be below %f", option, section, below))
	}
}

func configString(config printerpkg.ModuleConfig, option string, defaultValue string, required bool) string {
	if required {
		requireConfigOption(config, option)
	}
	return config.String(option, defaultValue, true)
}

func configFloat(config printerpkg.ModuleConfig, option string, defaultValue float64,
	minval float64, maxval float64, above float64, below float64, required bool) float64 {
	if required {
		requireConfigOption(config, option)
	}
	value := config.Float(option, defaultValue)
	validateFloatOption(config.Name(), option, value, minval, maxval, above, below)
	return value
}

func requireHeaterPins(printer printerpkg.ModulePrinter) heaterPinRegistry {
	pinsObj := printer.LookupObject("pins", nil)
	pins, ok := pinsObj.(heaterPinRegistry)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement heaterPinRegistry: %T", pinsObj))
	}
	return pins
}

func requireHeaterToolhead(printer printerpkg.ModulePrinter) heaterToolhead {
	toolheadObj := printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(heaterToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement heaterToolhead: %T", toolheadObj))
	}
	return toolhead
}

func requireHeaterReactor(printer printerpkg.ModulePrinter) heaterReactor {
	reactorObj := printer.Reactor()
	reactor, ok := reactorObj.(heaterReactor)
	if !ok {
		panic(fmt.Sprintf("reactor does not implement heaterReactor: %T", reactorObj))
	}
	return reactor
}

func requireHeaterGCode(printer printerpkg.ModulePrinter) heaterGCodeRuntime {
	gcodeObj := printer.GCode()
	gcode, ok := gcodeObj.(heaterGCodeRuntime)
	if !ok {
		panic(fmt.Sprintf("gcode runtime does not implement heaterGCodeRuntime: %T", gcodeObj))
	}
	return gcode
}

func setHeaterTemperature(printer printerpkg.ModulePrinter, heater *Heater, temp float64, wait bool) error {
	heatersObj := printer.LookupObject("heaters", nil)
	if heaters, ok := heatersObj.(heaterManager); ok {
		return heaters.Set_temperature(heater, temp, wait)
	}
	if heaters, ok := heatersObj.(directHeaterManager); ok {
		return heaters.Set_temperature(heater, temp, wait)
	}
	panic(fmt.Sprintf("heaters object does not implement heater temperature control: %T", heatersObj))
}

func pwmPin(self *Heater) heaterPWMPin {
	pin, ok := self.Mcu_pwm.(heaterPWMPin)
	if !ok {
		panic(fmt.Sprintf("heater pwm pin does not implement heaterPWMPin: %T", self.Mcu_pwm))
	}
	return pin
}

func pwmEstimator(self *Heater) heaterMCUEstimator {
	estimatorObj := pwmPin(self).MCU()
	estimator, ok := estimatorObj.(heaterMCUEstimator)
	if !ok {
		panic(fmt.Sprintf("heater pwm MCU does not implement heaterMCUEstimator: %T", estimatorObj))
	}
	return estimator
}

func defaultMinExtrudeTemp(heaterName string, maxTemp float64) float64 {
	if strings.HasPrefix(strings.ToLower(heaterName), "extruder") {
		return math.Min(170., maxTemp)
	}
	return 0.
}

func NewHeater(config printerpkg.ModuleConfig, sensor printerpkg.TemperatureSensor) *Heater {
	self := &Heater{}
	self.Printer = config.Printer()
	nameParts := strings.Split(config.Name(), " ")
	self.Name = nameParts[len(nameParts)-1]
	self.Sensor = sensor
	self.Min_temp = configFloat(config, "min_temp", 0., KELVIN_TO_CELSIUS, 0., 0., 0., true)
	self.Max_temp = configFloat(config, "max_temp", 0., 0., 0., self.Min_temp, 0., true)
	self.Sensor.SetupMinMax(self.Min_temp, self.Max_temp)
	self.Sensor.SetupCallback(self.Temperature_callback)
	self.Pwm_delay = self.Sensor.GetReportTimeDelta()
	defaultMinExtrudeTemp := defaultMinExtrudeTemp(self.Name, self.Max_temp)
	minExtrudeLowerBound := self.Min_temp
	if defaultMinExtrudeTemp <= 0. {
		minExtrudeLowerBound = 0.
	}
	self.Min_extrude_temp = configFloat(config, "min_extrude_temp", defaultMinExtrudeTemp, minExtrudeLowerBound, self.Max_temp, 0., 0., false)
	self.Can_extrude = self.Min_extrude_temp <= 0. || self.Printer.HasStartArg("debugoutput")
	self.Max_power = configFloat(config, "max_power", 1., 0., 1., 0., 0., false)
	self.Smooth_time = configFloat(config, "smooth_time", 1., 0., 0., 0., 0., false)
	self.Inv_smooth_time = 1. / self.Smooth_time
	self.Lock = sync.Mutex{}
	self.Last_temp, self.Smoothed_temp, self.Target_temp = 0., 0., 0.
	self.Last_temp_time = 0.
	self.Next_pwm_time = 0.
	self.Last_pwm_value = 0.

	controlType := configString(config, "control", "watermark", false)
	switch controlType {
	case "watermark":
		self.Control = NewControlBangBang(self, config)
	case "pid":
		self.Control = NewControlPID(self, config)
	default:
		panic(fmt.Sprintf("Choice '%s' for option 'control' in section '%s' is not a valid choice", controlType, config.Name()))
	}

	heaterPin := configString(config, "heater_pin", "", true)
	pins := requireHeaterPins(self.Printer)
	self.Mcu_pwm = pins.SetupPWM(heaterPin)
	pwmCycleTime := configFloat(config, "pwm_cycle_time", 0.100, 0., self.Pwm_delay, 0., 0., false)
	pwmPin(self).SetupCycleTime(pwmCycleTime, false)
	pwmPin(self).SetupMaxDuration(MAX_HEAT_TIME)

	config.LoadObject(fmt.Sprintf("verify_heater %s", self.Name))
	config.LoadObject("pid_calibrate")
	requireHeaterGCode(self.Printer).RegisterMuxCommand(
		"SET_HEATER_TEMPERATURE", "HEATER", self.Name,
		self.Cmd_SET_HEATER_TEMPERATURE,
		cmd_SET_HEATER_TEMPERATURE_help,
	)
	return self
}

func (self *Heater) Set_pwm(read_time float64, value float64) {
	if self.Target_temp <= 0. {
		value = 0.
	}
	if (read_time < self.Next_pwm_time || self.Last_pwm_value == 0.) &&
		math.Abs(value-self.Last_pwm_value) < 0.05 {
		return
	}
	pwmTime := read_time + self.Pwm_delay
	self.Next_pwm_time = pwmTime + 0.75*MAX_HEAT_TIME
	self.Last_pwm_value = value
	pwmPin(self).SetPWM(pwmTime, value)
}

func (self *Heater) Temperature_callback(read_time float64, temp float64) {
	self.Lock.Lock()
	defer self.Lock.Unlock()
	timeDiff := read_time - self.Last_temp_time
	self.Last_temp = temp
	self.Last_temp_time = read_time
	self.Control.(iTemperature_update).Temperature_update(read_time, temp, self.Target_temp)
	tempDiff := temp - self.Smoothed_temp
	adjTime := math.Min(timeDiff*self.Inv_smooth_time, 1.)
	self.Smoothed_temp += tempDiff * adjTime
	self.Can_extrude = self.Smoothed_temp >= self.Min_extrude_temp
}

func (self *Heater) Get_pwm_delay() float64 {
	return self.Pwm_delay
}

func (self *Heater) Get_max_power() float64 {
	return self.Max_power
}

func (self *Heater) Get_smooth_time() float64 {
	return self.Smooth_time
}

func (self *Heater) Set_temp(degrees float64) {
	if degrees != 0 && (degrees < self.Min_temp || degrees > self.Max_temp) {
		panic(fmt.Sprintf("Requested temperature (%.1f) out of range (%.1f:%.1f)", degrees, self.Min_temp, self.Max_temp))
	}
	self.Lock.Lock()
	defer self.Lock.Unlock()
	self.Target_temp = degrees
}

func (self *Heater) Get_temp(eventtime float64) (float64, float64) {
	printTime := pwmEstimator(self).EstimatedPrintTime(eventtime) - 5.
	self.Lock.Lock()
	defer self.Lock.Unlock()
	if self.Last_temp_time < printTime {
		return 0., self.Target_temp
	}
	return self.Smoothed_temp, self.Target_temp
}

func (self *Heater) Check_busy(eventtime float64) interface{} {
	self.Lock.Lock()
	defer self.Lock.Unlock()
	return self.Control.(CheckBusyer).Check_busy(eventtime, self.Smoothed_temp, self.Target_temp)
}

func (self *Heater) Set_control(control interface{}) interface{} {
	self.Lock.Lock()
	defer self.Lock.Unlock()
	oldControl := self.Control
	self.Control = control
	self.Target_temp = 0.
	return oldControl
}

func (self *Heater) Alter_target(target_temp float64) {
	if target_temp != 0 {
		target_temp = math.Max(self.Min_temp, math.Min(self.Max_temp, target_temp))
	}
	self.Target_temp = target_temp
}

func (self *Heater) Stats(eventtime float64) (bool, string) {
	self.Lock.Lock()
	defer self.Lock.Unlock()
	targetTemp := self.Target_temp
	lastTemp := self.Last_temp
	lastPwmValue := self.Last_pwm_value
	isActive := targetTemp != 0 || lastTemp > 50.
	return isActive, fmt.Sprintf("%s: target=%.0f temp=%.1f pwm=%.3f",
		self.Name, targetTemp, lastTemp, lastPwmValue)
}

func (self *Heater) Get_status(eventtime float64) map[string]float64 {
	self.Lock.Lock()
	defer self.Lock.Unlock()
	targetTemp := self.Target_temp
	smoothedTemp := self.Smoothed_temp
	lastPwmValue := self.Last_pwm_value
	return map[string]float64{
		"temperature": math.Round(smoothedTemp),
		"target":      targetTemp,
		"power":       lastPwmValue,
	}
}

func (self *Heater) Cmd_SET_HEATER_TEMPERATURE(gcmd printerpkg.Command) error {
	temp := gcmd.Float("TARGET", 0.)
	return setHeaterTemperature(self.Printer, self, temp, false)
}

type ControlBangBang struct {
	Heater           *Heater
	Heater_max_power float64
	Max_delta        float64
	Heating          bool
}

func NewControlBangBang(heater *Heater, config printerpkg.ModuleConfig) interface{} {
	self := &ControlBangBang{}
	self.Heater = heater
	self.Heater_max_power = heater.Get_max_power()
	self.Max_delta = configFloat(config, "max_delta", 2.0, 0., 0., 0., 0., false)
	self.Heating = false
	return self
}

func (self *ControlBangBang) Temperature_update(read_time float64, temp float64, target_temp float64) {
	if self.Heating && temp >= target_temp+self.Max_delta {
		self.Heating = false
	} else if !self.Heating && temp <= target_temp-self.Max_delta {
		self.Heating = true
	}
	if self.Heating {
		self.Heater.Set_pwm(read_time, self.Heater_max_power)
	} else {
		self.Heater.Set_pwm(read_time, 0.)
	}
}

func (self *ControlBangBang) Check_busy(eventtime float64, smoothed_temp float64, target_temp float64) bool {
	return smoothed_temp < target_temp-self.Max_delta
}

type ControlPID struct {
	Heater           *Heater
	Heater_max_power float64
	Kp               float64
	Ki               float64
	Kd               float64
	Min_deriv_time   float64
	Temp_integ_max   float64
	Prev_temp        float64
	Prev_temp_time   float64
	Prev_temp_deriv  float64
	Prev_temp_integ  float64
}

func NewControlPID(heater *Heater, config printerpkg.ModuleConfig) interface{} {
	self := &ControlPID{}
	self.Heater = heater
	self.Heater_max_power = heater.Get_max_power()
	self.Kp = configFloat(config, "pid_Kp", 0., 0., 0., 0., 0., true) / PID_PARAM_BASE
	self.Ki = configFloat(config, "pid_Ki", 0., 0., 0., 0., 0., true) / PID_PARAM_BASE
	self.Kd = configFloat(config, "pid_Kd", 0., 0., 0., 0., 0., true) / PID_PARAM_BASE
	self.Min_deriv_time = heater.Get_smooth_time()
	self.Temp_integ_max = 0.
	if self.Ki != 0 {
		self.Temp_integ_max = self.Heater_max_power / self.Ki
	}
	self.Prev_temp = AMBIENT_TEMP
	self.Prev_temp_time = 0.
	self.Prev_temp_deriv = 0.
	self.Prev_temp_integ = 0.
	return self
}

func (self *ControlPID) Temperature_update(read_time float64, temp float64, target_temp float64) {
	timeDiff := read_time - self.Prev_temp_time
	tempDiff := temp - self.Prev_temp
	var tempDeriv float64
	if timeDiff >= self.Min_deriv_time {
		tempDeriv = tempDiff / timeDiff
	} else {
		tempDeriv = (self.Prev_temp_deriv*(self.Min_deriv_time-timeDiff) + tempDiff) / self.Min_deriv_time
	}
	tempErr := target_temp - temp
	tempInteg := self.Prev_temp_integ + tempErr*timeDiff
	tempInteg = math.Max(0., math.Min(self.Temp_integ_max, tempInteg))
	co := self.Kp*tempErr + self.Ki*tempInteg - self.Kd*tempDeriv
	boundedCo := math.Max(0., math.Min(self.Heater_max_power, co))
	self.Heater.Set_pwm(read_time, boundedCo)
	self.Prev_temp = temp
	self.Prev_temp_time = read_time
	self.Prev_temp_deriv = tempDeriv
	if co == boundedCo {
		self.Prev_temp_integ = tempInteg
	}
}

func (self *ControlPID) Check_busy(eventtime float64, smoothed_temp float64, target_temp float64) bool {
	tempDiff := target_temp - smoothed_temp
	return math.Abs(tempDiff) > PID_SETTLE_DELTA || math.Abs(self.Prev_temp_deriv) > PID_SETTLE_SLOPE
}

type heaterTemperatureReporter interface {
	Get_temp(eventtime float64) (float64, float64)
}

type PrinterHeaters struct {
	Printer            printerpkg.ModulePrinter
	Sensor_factories   map[string]temperatureSensorFactory
	Heaters            map[string]*Heater
	Gcode_id_to_sensor map[string]interface{}
	Available_heaters  []string
	Available_sensors  []string
	Available_monitors []string
	Has_started        bool
	Have_load_sensors  bool
	loadDefaultSensors func() error
}

func buildDefaultSensorLoader(config printerpkg.ModuleConfig) func() error {
	extended, ok := config.(interface{ LoadSupportConfig(string) error })
	if !ok {
		return func() error {
			return fmt.Errorf("config does not implement support config loading: %T", config)
		}
	}
	return func() error {
		var lastErr error
		for _, candidate := range []string{"temperature_sensors.cfg", filepath.Join("config", "temperature_sensors.cfg")} {
			if err := extended.LoadSupportConfig(candidate); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("unable to load temperature_sensors.cfg")
		}
		return lastErr
	}
}

func NewPrinterHeaters(config printerpkg.ModuleConfig) *PrinterHeaters {
	self := &PrinterHeaters{}
	self.Printer = config.Printer()
	self.Sensor_factories = map[string]temperatureSensorFactory{}
	self.Heaters = map[string]*Heater{"": nil}
	self.Gcode_id_to_sensor = map[string]interface{}{}
	self.Available_heaters = []string{}
	self.Available_sensors = []string{}
	self.Available_monitors = []string{}
	self.Has_started = false
	self.Have_load_sensors = false
	self.loadDefaultSensors = buildDefaultSensorLoader(config)
	self.Printer.RegisterEventHandler("project:ready", self.Handle_ready)
	self.Printer.RegisterEventHandler("gcode:request_restart", self.Turn_off_all_heaters)
	gcode := requireHeaterGCode(self.Printer)
	gcode.RegisterCommand("TURN_OFF_HEATERS", self.Cmd_TURN_OFF_HEATERS, true, cmd_TURN_OFF_HEATERS_help)
	gcode.RegisterCommand("M105", self.Cmd_M105, true, "")
	gcode.RegisterCommand("TEMPERATURE_WAIT", self.Cmd_TEMPERATURE_WAIT, true, cmd_TEMPERATURE_WAIT_help)
	return self
}

func (self *PrinterHeaters) Load_config(config printerpkg.ModuleConfig) {
	if self.Have_load_sensors {
		return
	}
	self.Have_load_sensors = true
	if self.loadDefaultSensors == nil {
		return
	}
	if err := self.loadDefaultSensors(); err != nil {
		logger.Errorf(fmt.Sprintf("Cannot load config temperature_sensors.cfg: %v", err))
	}
}

func (self *PrinterHeaters) Add_sensor_factory(sensor_type string, sensor_factory interface{}) {
	self.Sensor_factories[sensor_type] = normalizeTemperatureSensorFactory(sensor_factory)
}

func (self *PrinterHeaters) Setup_heater(config printerpkg.ModuleConfig, gcode_id string) *Heater {
	nameParts := strings.Split(config.Name(), " ")
	heaterName := nameParts[len(nameParts)-1]
	if self.Heaters[heaterName] != nil {
		panic(fmt.Sprintf("Heater %s already registered", heaterName))
	}
	sensor := self.Setup_sensor(config)
	heater := NewHeater(config, sensor)
	self.Heaters[heaterName] = heater
	self.Register_sensor(config, heater, gcode_id)
	self.Available_heaters = append(self.Available_heaters, config.Name())
	return heater
}

func (self *PrinterHeaters) Get_all_heaters() []string {
	return self.Available_heaters
}

func (self *PrinterHeaters) Lookup_heater(heater_name string) *Heater {
	if self.Heaters[heater_name] == nil {
		panic(fmt.Sprintf("Unknown heater  %s", heater_name))
	}
	return self.Heaters[heater_name]
}

func (self *PrinterHeaters) Setup_sensor(config printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
	if !self.Have_load_sensors {
		self.Load_config(config)
	}
	sensorType := configString(config, "sensor_type", "", true)
	factory := self.Sensor_factories[sensorType]
	if factory == nil {
		panic(fmt.Sprintf("Unknown temperature sensor  %s", sensorType))
	}
	if sensorType == "NTC 100K beta 3950" {
		if configWithDeprecation, ok := config.(interface{ Deprecate(string, string) }); ok {
			configWithDeprecation.Deprecate("sensor_type", "NTC 100K beta 3950")
		}
	}
	return factory(config)
}

func (self *PrinterHeaters) Register_sensor(config printerpkg.ModuleConfig, psensor interface{}, gcode_id string) {
	self.Available_sensors = append(self.Available_sensors, config.Name())
	if gcode_id == "" {
		gcode_id = configString(config, "gcode_id", "", false)
		if gcode_id == "" {
			return
		}
	}
	if self.Gcode_id_to_sensor[gcode_id] != nil {
		panic(fmt.Sprintf("G-Code sensor id %s already registered", gcode_id))
	}
	self.Gcode_id_to_sensor[gcode_id] = psensor
}

func (self *PrinterHeaters) Register_monitor(config printerpkg.ModuleConfig) {
	self.Available_monitors = append(self.Available_monitors, config.Name())
}

func (self *PrinterHeaters) Get_status(eventtime float64) map[string]interface{} {
	return map[string]interface{}{
		"available_heaters":  self.Available_heaters,
		"available_sensors":  self.Available_sensors,
		"available_monitors": self.Available_monitors,
	}
}

func (self *PrinterHeaters) Turn_off_all_heaters(argv []interface{}) error {
	for _, heater := range self.Heaters {
		if heater != nil {
			heater.Set_temp(0.)
		}
	}
	return nil
}

func (self *PrinterHeaters) Cmd_TURN_OFF_HEATERS(gcmd printerpkg.Command) error {
	return self.Turn_off_all_heaters(nil)
}

func (self *PrinterHeaters) Handle_ready(args []interface{}) error {
	self.Has_started = true
	return nil
}

func (self *PrinterHeaters) Get_temp(eventtime float64, heater_type string) string {
	out := make([]string, 0, len(self.Gcode_id_to_sensor))
	gcodeIDs := str.MapStringKeys(self.Gcode_id_to_sensor)
	sort.Strings(gcodeIDs)
	if self.Has_started {
		for _, gcodeID := range gcodeIDs {
			sensorObj := self.Gcode_id_to_sensor[gcodeID]
			if sensorObj == nil {
				logger.Errorf(fmt.Sprintf("G-Code sensor id %s must not be nil", gcodeID))
				continue
			}
			sensor, ok := sensorObj.(heaterTemperatureReporter)
			if !ok {
				logger.Errorf(fmt.Sprintf("G-Code sensor id %s, %+v must implement heaterTemperatureReporter", gcodeID, sensorObj))
				continue
			}
			cur, target := sensor.Get_temp(eventtime)
			out = append(out, fmt.Sprintf("%s:%.1f /%.1f", gcodeID, cur, target))
		}
	}
	if len(out) == 0 {
		return "T:0"
	}
	return strings.Join(out, " ")
}

func (self *PrinterHeaters) Cmd_M105(gcmd printerpkg.Command) error {
	msg := self.Get_temp(self.Printer.Reactor().Monotonic(), "extruder")
	if acker, ok := gcmd.(heaterCommandAck); ok {
		if acker.Ack(msg) {
			gcmd.RespondRaw(msg)
		}
	}
	return nil
}

func (self *PrinterHeaters) Wait_for_temperature(heater *Heater) error {
	if self.Printer.HasStartArg("debugoutput") {
		return nil
	}
	toolhead := requireHeaterToolhead(self.Printer)
	gcode := requireHeaterGCode(self.Printer)
	reactor := requireHeaterReactor(self.Printer)
	eventtime := reactor.Monotonic()
	for !self.Printer.IsShutdown() && cast.ToBool(heater.Check_busy(eventtime)) {
		_ = toolhead.Get_last_move_time()
		gcode.RespondRaw(self.Get_temp(eventtime, heater.Name))
		eventtime = reactor.Pause(eventtime + 1.)
	}
	return nil
}

func (self *PrinterHeaters) Set_temperature(heater *Heater, temp float64, wait bool) error {
	toolhead := requireHeaterToolhead(self.Printer)
	toolhead.RegisterLookaheadCallback(func(pt float64) {})
	heater.Set_temp(temp)
	if wait && temp > 0 {
		return self.Wait_for_temperature(heater)
	}
	return nil
}

func parseParameterFloat(params map[string]string, name string, defaultValue float64) (float64, bool) {
	value, ok := params[name]
	if !ok || value == "" {
		return defaultValue, false
	}
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed, true
}

func (self *PrinterHeaters) Cmd_TEMPERATURE_WAIT(gcmd printerpkg.Command) error {
	sensorName := gcmd.String("SENSOR", "")
	if sensorName == "" {
		panic("Error on TEMPERATURE_WAIT: missing SENSOR.")
	}
	if !collections.Contains(self.Available_sensors, sensorName) {
		panic(fmt.Sprintf("Unknown sensor %s", sensorName))
	}
	params := gcmd.Parameters()
	minTemp, hasMinTemp := parseParameterFloat(params, "MINIMUM", math.Inf(-1))
	maxTemp, hasMaxTemp := parseParameterFloat(params, "MAXIMUM", math.Inf(1))
	if !hasMinTemp && !hasMaxTemp {
		panic("Error on TEMPERATURE_WAIT: missing MINIMUM or MAXIMUM.")
	}
	if self.Printer.HasStartArg("debugoutput") {
		return nil
	}
	var sensor *Heater
	if self.Heaters[sensorName] != nil {
		sensor = self.Heaters[sensorName]
	} else {
		sensor = self.Printer.LookupObject(sensorName, nil).(*Heater)
	}
	reactor := requireHeaterReactor(self.Printer)
	eventtime := reactor.Monotonic()
	for {
		temp, _ := sensor.Get_temp(eventtime)
		if temp >= minTemp && temp <= maxTemp {
			return nil
		}
		gcmd.RespondRaw(self.Get_temp(eventtime, sensorName))
		eventtime = reactor.Pause(eventtime + 1.)
		if self.Printer.IsShutdown() {
			break
		}
	}
	return nil
}

func LoadConfigHeaters(config printerpkg.ModuleConfig) interface{} {
	return NewPrinterHeaters(config)
}
