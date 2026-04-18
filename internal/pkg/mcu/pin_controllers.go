package mcu

import (
	"fmt"
	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

type DigitalOutControllerMCU interface {
	printerpkg.PrintTimeEstimator
	CreateOID() int
	RegisterConfigCallback(func())
	Request_move_queue_slot()
	AddConfigCmd(cmd string, isInit bool, onRestart bool)
	AllocCommandQueue() interface{}
	LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error)
	SecondsToClock(time float64) int64
	PrintTimeToClock(printTime float64) int64
}

type PWMPinControllerMCU interface {
	DigitalOutControllerMCU
	Get_constant_float(name string) float64
	Monotonic() float64
	EstimatedPrintTime(eventtime float64) float64
}

type ADCControllerMCU interface {
	CreateOID() int
	RegisterConfigCallback(func())
	AddConfigCmd(cmd string, isInit bool, onRestart bool)
	GetQuerySlot(oid int) int64
	SecondsToClock(time float64) int64
	Get_constant_float(name string) float64
	RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{})
	Clock32ToClock64(clock32 int64) int64
	ClockToPrintTime(clock int64) float64
}

type LegacyPinController interface {
	DigitalOutControllerMCU
	PWMPinControllerMCU
	ADCControllerMCU
}

type LegacyEndstopPinFactory func(pinParams map[string]interface{}) interface{}

func SetupLegacyControllerPin(controller LegacyPinController, pinType string, pinParams map[string]interface{}, endstopFactory LegacyEndstopPinFactory) interface{} {
	switch pinType {
	case "endstop":
		if endstopFactory == nil {
			return nil
		}
		return endstopFactory(pinParams)
	case "digital_out":
		return NewDigitalOutPin(controller, pinParams)
	case "pwm":
		return NewPWMPin(controller, pinParams)
	case "adc":
		return NewADCPin(controller, pinParams)
	default:
		return nil
	}
}

type DigitalOutPin struct {
	Pin            string
	Invert         int
	mcu            DigitalOutControllerMCU
	lastClock      int64
	Max_duration   float64
	setCmd         OutputCommandSender
	Oid            int
	Start_value    int
	Shutdown_value int
}

func NewDigitalOutPin(mcu DigitalOutControllerMCU, pinParams map[string]interface{}) *DigitalOutPin {
	self := &DigitalOutPin{
		mcu:            mcu,
		Oid:            -1,
		Pin:            pinParams["pin"].(string),
		Invert:         pinParams["invert"].(int),
		Max_duration:   2.0,
		lastClock:      0,
		Start_value:    pinParams["invert"].(int),
		Shutdown_value: pinParams["invert"].(int),
	}
	mcu.RegisterConfigCallback(self.Build_config)
	return self
}

func (self *DigitalOutPin) SetupMaxDuration(maxDuration float64) {
	self.Setup_max_duration(maxDuration)
}

func (self *DigitalOutPin) Setup_max_duration(max_duration float64) {
	self.Max_duration = max_duration
}

func (self *DigitalOutPin) Setup_start_value(start_value float64, shutdown_value float64) {
	state := DigitalOutRuntimeState{Invert: self.Invert}
	self.Start_value, self.Shutdown_value = state.SetupStartValue(start_value, shutdown_value)
}

func (self *DigitalOutPin) Build_config() {
	plan := BuildDigitalOutConfigPlan(self.Max_duration, self.Start_value, self.Shutdown_value, self.mcu.SecondsToClock)
	self.mcu.Request_move_queue_slot()
	self.Oid = self.mcu.CreateOID()
	setupPlan := BuildDigitalOutConfigSetupPlan(self.Oid, self.Pin, self.Start_value, self.Shutdown_value, plan)
	for _, cmd := range setupPlan.Commands {
		self.mcu.AddConfigCmd(cmd.Cmd, cmd.IsInit, cmd.OnRestart)
	}
	cmdQueue := self.mcu.AllocCommandQueue()
	command, err := self.mcu.LookupCommandRaw(setupPlan.LookupFormat, cmdQueue)
	if err != nil {
		panic(err)
	}
	sender, ok := command.(OutputCommandSender)
	if !ok {
		panic(fmt.Sprintf("digital out command sender has unexpected type %T", command))
	}
	self.setCmd = sender
}

func (self *DigitalOutPin) Set_digital(print_time float64, value int) {
	state := DigitalOutRuntimeState{Invert: self.Invert, LastClock: self.lastClock}
	state.SetDigital(print_time, value, self.mcu.PrintTimeToClock, self.setCmd, self.Oid)
	self.lastClock = state.LastClock
}

func (self *DigitalOutPin) SetDigital(printTime float64, value int) {
	self.Set_digital(printTime, value)
}

func (self *DigitalOutPin) MCU() printerpkg.PrintTimeEstimator {
	return self.mcu
}

type PWMPin struct {
	Pin            string
	Invert         int
	mcu            PWMPinControllerMCU
	pwmMax         float64
	Hardware_pwm   interface{}
	lastClock      int64
	Cycle_time     float64
	Start_value    float64
	setCmd         OutputCommandSender
	Oid            int
	Max_duration   float64
	Shutdown_value float64
}

func NewPWMPin(mcu PWMPinControllerMCU, pinParams map[string]interface{}) *PWMPin {
	self := &PWMPin{
		mcu:          mcu,
		Hardware_pwm: false,
		Cycle_time:   0.1,
		Max_duration: 2.0,
		Oid:          -1,
		Pin:          pinParams["pin"].(string),
		Invert:       pinParams["invert"].(int),
		lastClock:    0,
		pwmMax:       0.0,
	}
	self.Start_value = float64(self.Invert)
	self.Shutdown_value = self.Start_value
	mcu.RegisterConfigCallback(self.Build_config)
	return self
}

func (self *PWMPin) SetupMaxDuration(maxDuration float64) {
	self.Setup_max_duration(maxDuration)
}

func (self *PWMPin) SetupCycleTime(cycleTime float64, hardwarePWM bool) {
	self.Setup_cycle_time(cycleTime, hardwarePWM)
}

func (self *PWMPin) SetupStartValue(startValue float64, shutdownValue float64) {
	self.Setup_start_value(startValue, shutdownValue)
}

func (self *PWMPin) Setup_max_duration(max_duration float64) {
	self.Max_duration = max_duration
}

func (self *PWMPin) Setup_cycle_time(cycle_time float64, hardware_pwm bool) {
	self.Cycle_time = cycle_time
	self.Hardware_pwm = hardware_pwm
}

func (self *PWMPin) Setup_start_value(start_value float64, shutdown_value float64) {
	state := PWMRuntimeState{Invert: self.Invert}
	self.Start_value, self.Shutdown_value = state.SetupStartValue(start_value, shutdown_value)
}

func (self *PWMPin) Build_config() {
	cmdQueue := self.mcu.AllocCommandQueue()
	plan := BuildPWMConfigPlan(self.Max_duration, self.Cycle_time, self.Start_value, self.Shutdown_value, self.Hardware_pwm.(bool), self.mcu.Get_constant_float("_pwm_max"), self.mcu.Monotonic, self.mcu.EstimatedPrintTime, self.mcu.PrintTimeToClock, self.mcu.SecondsToClock)
	self.lastClock = plan.LastClock
	self.pwmMax = plan.PWMMax
	self.mcu.Request_move_queue_slot()
	self.Oid = self.mcu.CreateOID()
	setupPlan := BuildPWMConfigSetupPlan(self.Oid, self.Pin, self.Hardware_pwm.(bool), plan)
	for _, cmd := range setupPlan.Commands {
		self.mcu.AddConfigCmd(cmd.Cmd, cmd.IsInit, cmd.OnRestart)
	}
	command, err := self.mcu.LookupCommandRaw(setupPlan.LookupFormat, cmdQueue)
	if err != nil {
		panic(err)
	}
	sender, ok := command.(OutputCommandSender)
	if !ok {
		panic(fmt.Sprintf("pwm command sender has unexpected type %T", command))
	}
	self.setCmd = sender
}

func (self *PWMPin) Set_pwm(print_time float64, val float64) {
	state := PWMRuntimeState{Invert: self.Invert, PWMMax: self.pwmMax, LastClock: self.lastClock}
	state.SetPWM(print_time, val, self.mcu.PrintTimeToClock, self.setCmd, self.Oid)
	self.lastClock = state.LastClock
}

func (self *PWMPin) SetPWM(printTime float64, value float64) {
	self.Set_pwm(printTime, value)
}

func (self *PWMPin) MCU() interface{} {
	return self.mcu
}

type ADCPin struct {
	Pin               string
	mcu               ADCControllerMCU
	Report_clock      int64
	Min_sample        float64
	Max_sample        float64
	Last_state        []float64
	Inv_max_adc       float64
	Sample_count      int
	Range_check_count int
	Oid               int
	Callback          func(float64, float64)
	Sample_time       float64
	Report_time       float64
}

func NewADCPin(mcu ADCControllerMCU, pinParams map[string]interface{}) *ADCPin {
	self := &ADCPin{
		mcu:               mcu,
		Pin:               pinParams["pin"].(string),
		Min_sample:        0.0,
		Max_sample:        0.0,
		Sample_time:       0.0,
		Report_time:       0.0,
		Sample_count:      0,
		Range_check_count: 0,
		Report_clock:      0,
		Last_state:        []float64{0.0, 0.0},
		Oid:               -1,
		Callback:          nil,
		Inv_max_adc:       0.0,
	}
	mcu.RegisterConfigCallback(self.Build_config)
	return self
}

func (self *ADCPin) Setup_minmax(sample_time float64, sample_count int, minval float64, maxval float64, range_check_count int) {
	self.Sample_time = sample_time
	self.Sample_count = sample_count
	self.Min_sample = minval
	self.Max_sample = maxval
	self.Range_check_count = range_check_count
}

func (self *ADCPin) Setup_adc_callback(report_time float64, callback func(float64, float64)) {
	self.Report_time = report_time
	self.Callback = callback
}

func (self *ADCPin) Get_last_value() []float64 {
	return self.Last_state
}

func (self *ADCPin) SetupCallback(reportTime float64, callback func(float64, float64)) {
	self.Setup_adc_callback(reportTime, callback)
}

func (self *ADCPin) SetupMinMax(sampleTime float64, sampleCount int, minval float64, maxval float64, rangeCheckCount int) {
	self.Setup_minmax(sampleTime, sampleCount, minval, maxval, rangeCheckCount)
}

func (self *ADCPin) GetLastValue() [2]float64 {
	return [2]float64{self.Last_state[0], self.Last_state[1]}
}

func (self *ADCPin) runtimeState() ADCRuntimeState {
	state := ADCRuntimeState{
		InvMaxADC:   self.Inv_max_adc,
		ReportClock: self.Report_clock,
	}
	if len(self.Last_state) >= 2 {
		state.LastValue = [2]float64{self.Last_state[0], self.Last_state[1]}
	}
	return state
}

func (self *ADCPin) applyRuntimeState(state ADCRuntimeState) {
	self.Last_state = []float64{state.LastValue[0], state.LastValue[1]}
	self.Inv_max_adc = state.InvMaxADC
	self.Report_clock = state.ReportClock
}

func (self *ADCPin) Build_config() {
	if self.Sample_count == 0 {
		return
	}
	self.Oid = self.mcu.CreateOID()
	state := self.runtimeState()
	plan := state.BuildConfigPlan(self.Oid, self.Sample_time, self.Report_time, self.Min_sample, self.Max_sample, self.Sample_count, self.Range_check_count, self.mcu.GetQuerySlot, self.mcu.SecondsToClock, self.mcu.Get_constant_float("ADC_MAX"))
	self.applyRuntimeState(state)
	setupPlan := BuildADCConfigSetupPlan(self.Oid, self.Pin, plan)
	self.mcu.AddConfigCmd(setupPlan.ConfigCmd, false, false)
	self.mcu.AddConfigCmd(setupPlan.QueryCmd, true, false)
	logger.Infof("REGISTERING %s", setupPlan.ResponseLogLabel)
	self.mcu.RegisterResponse(self.Handle_analog_in_state, setupPlan.ResponseName, self.Oid)
}

func (self *ADCPin) Handle_analog_in_state(params map[string]interface{}) error {
	state := self.runtimeState()
	lastState := state.ProcessAnalogInState(params, self.mcu.Clock32ToClock64, self.mcu.ClockToPrintTime)
	self.applyRuntimeState(state)
	if self.Callback != nil {
		self.Callback(lastState[1], lastState[0])
	}
	return nil
}
