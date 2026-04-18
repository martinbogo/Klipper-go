package io

import (
	"fmt"
	"strconv"
	"strings"

	"goklipper/common/logger"
	"goklipper/common/utils/maths"
	printerpkg "goklipper/internal/pkg/printer"
)

type cs1237PinLookup interface {
	LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{}
}

type cs1237Toolhead interface {
	Dwell(delay float64)
}

type cs1237Command interface {
	Send(data interface{}, minclock int64, reqclock int64)
}

type cs1237MCU interface {
	printerpkg.MCURuntime
	LookupCommand(msgformat string, cq interface{}) (interface{}, error)
	NewTrsyncCommandQueue() interface{}
}

type cs1237Completion interface {
	Wait(waketime float64, waketimeResult interface{}) interface{}
}

type cs1237Reactor interface {
	printerpkg.ModuleReactor
	Completion() interface{}
	AsyncComplete(completion interface{}, result map[string]interface{})
}

type CS1237Module struct {
	doutPin            string
	sclkPin            string
	levelPin           string
	register           int
	sensitivity        int
	mcu                cs1237MCU
	oid                int
	commandQueue       interface{}
	resetCSCmd         cs1237Command
	enableCSCmd        cs1237Command
	csReportCmd        cs1237Command
	queryCSDiff        cs1237Command
	csCalibrationPhase cs1237Command
	csCalibrationVal   cs1237Command
	printer            printerpkg.ModulePrinter
	adcValue           int64
	rawValue           int64
	sensorState        int64
	report             bool
	queryCompletion    interface{}
	enableCount        int
}

func LoadConfigCS1237(config printerpkg.ModuleConfig) interface{} {
	return NewCS1237Module(config)
}

func NewCS1237Module(config printerpkg.ModuleConfig) *CS1237Module {
	printer := config.Printer()
	pinsObj := printer.LookupObject("pins", nil)
	pins, ok := pinsObj.(cs1237PinLookup)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement cs1237PinLookup: %T", pinsObj))
	}
	doutPinParams := pins.LookupPin(config.String("dout_pin", "", true), true, true, nil)
	sclkPinParams := pins.LookupPin(config.String("sclk_pin", "", true), true, true, nil)
	levelPinParams := pins.LookupPin(config.String("level_pin", "", true), true, true, nil)
	mcuName, _ := sclkPinParams["chip_name"].(string)
	mcuObj := printer.LookupMCU(mcuName)
	mcu, ok := mcuObj.(cs1237MCU)
	if !ok {
		panic(fmt.Sprintf("mcu object does not implement cs1237MCU: %T", mcuObj))
	}
	self := &CS1237Module{
		doutPin:     fmt.Sprintf("%v", doutPinParams["pin"]),
		sclkPin:     fmt.Sprintf("%v", sclkPinParams["pin"]),
		levelPin:    fmt.Sprintf("%v", levelPinParams["pin"]),
		register:    configInt(config, "register"),
		sensitivity: configInt(config, "sensitivity"),
		mcu:         mcu,
		printer:     printer,
	}
	self.commandQueue = self.mcu.NewTrsyncCommandQueue()
	self.mcu.RegisterConfigCallback(self.buildConfig)
	self.printer.RegisterEventHandler("homing:multi_probe_begin", self.enableCS1237)
	self.printer.RegisterEventHandler("homing:multi_probe_end", self.disableCS1237)
	return self
}

func configInt(config printerpkg.ModuleConfig, option string) int {
	value := strings.TrimSpace(config.String(option, "", true))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Sprintf("invalid %s value %q: %v", option, value, err))
	}
	return parsed
}

func (self *CS1237Module) buildConfig() {
	self.oid = self.mcu.CreateOID()
	self.mcu.AddConfigCmd(fmt.Sprintf("config_cs1237 oid=%d level_pin=%s dout_pin=%s sclk_pin=%s register=%d sensitivity=%d",
		self.oid, self.levelPin, self.doutPin, self.sclkPin, self.register, self.sensitivity), false, false)
	self.resetCSCmd = self.lookupCommand("reset_cs1237 oid=%c count=%c")
	self.csReportCmd = self.lookupCommand("start_cs1237_report oid=%c enable=%c ticks=%i print_state=%c sensitivity=%i")
	self.enableCSCmd = self.lookupCommand("enable_cs1237 oid=%c state=%c")
	self.mcu.RegisterResponse(self.handleQueryState, "cs1237_state", self.oid)
	self.queryCSDiff = self.lookupCommand("query_cs1237_diff oid=%c")
	self.mcu.RegisterResponse(self.handleDiff, "cs1237_diff", self.oid)
	self.csCalibrationPhase = self.lookupCommand("cs1237_calibration_phase oid=%c cali_state=%c speed_state=%c")
	self.csCalibrationVal = self.lookupCommand("cs1237_calibration_DataProcess oid=%c")
	self.mcu.RegisterResponse(self.handleCalibrationValue, "cs1237_calibration_Val", self.oid)
}

func (self *CS1237Module) lookupCommand(msgformat string) cs1237Command {
	command, err := self.mcu.LookupCommand(msgformat, self.commandQueue)
	if err != nil {
		panic(err)
	}
	typed, ok := command.(cs1237Command)
	if !ok {
		panic(fmt.Sprintf("command does not implement cs1237Command: %T", command))
	}
	return typed
}

func (self *CS1237Module) reactor() cs1237Reactor {
	reactorObj := self.printer.Reactor()
	reactor, ok := reactorObj.(cs1237Reactor)
	if !ok {
		panic(fmt.Sprintf("reactor does not implement cs1237Reactor: %T", reactorObj))
	}
	return reactor
}

func (self *CS1237Module) completion() cs1237Completion {
	if self.queryCompletion == nil {
		self.queryCompletion = self.reactor().Completion()
	}
	completion, ok := self.queryCompletion.(cs1237Completion)
	if !ok {
		panic(fmt.Sprintf("completion does not implement cs1237Completion: %T", self.queryCompletion))
	}
	return completion
}

func (self *CS1237Module) CheckStart(checkPeriod float64, checkType int64, sensitivity int64, delaySendTime float64) {
	tick := self.mcu.SecondsToClock(checkPeriod)
	delayT := self.mcu.SecondsToClock(delaySendTime)
	self.csReportCmd.Send([]int64{int64(self.oid), 1, tick, checkType, sensitivity}, delayT, 0)
}

func (self *CS1237Module) CheckStop(checkType int64) {
	self.csReportCmd.Send([]int64{int64(self.oid), 0, 0, checkType, 0}, 0, 0)
}

func (self *CS1237Module) handleQueryState(params map[string]interface{}) error {
	self.adcValue = maths.Int64_conversion(params["adc"].(int64))
	self.rawValue = maths.Int64_conversion(params["raw"].(int64))
	self.sensorState = params["state"].(int64)
	logger.Debug("params ", params, self.rawValue, self.adcValue)
	return nil
}

func (self *CS1237Module) Diff() (int64, int64, error) {
	defer func() {
		self.queryCompletion = nil
	}()
	self.queryCSDiff.Send([]int64{int64(self.oid)}, 0, 0)
	timeout := self.reactor().Monotonic() + 2
	params := self.completion().Wait(timeout, nil)
	if params != nil {
		result := params.(map[string]interface{})
		return result["diff"].(int64), result["raw"].(int64), nil
	}
	logger.Debug("cs1237_query response timeout")
	return -1, -1, fmt.Errorf("cs1237_query response timeout")
}

func (self *CS1237Module) Calibration(calibrationState int64, speedState int64) {
	self.csCalibrationPhase.Send([]int64{int64(self.oid), calibrationState, speedState}, 0, 0)
}

func (self *CS1237Module) handleCalibrationValue(params map[string]interface{}) error {
	if params["BlockPreVal"] == nil {
		params["BlockPreVal"] = int64(0)
	}
	if params["TargetVal"] == nil {
		params["TargetVal"] = int64(0)
	}
	if params["RealVal"] == nil {
		params["RealVal"] = int64(0)
	}
	params["BlockPreVal"] = maths.Int64_conversion(params["BlockPreVal"].(int64))
	params["TargetVal"] = maths.Int64_conversion(params["TargetVal"].(int64))
	params["RealVal"] = maths.Int64_conversion(params["RealVal"].(int64))
	if self.queryCompletion != nil {
		self.reactor().AsyncComplete(self.queryCompletion, params)
	}
	return nil
}

func (self *CS1237Module) CalibrationVal() (int64, int64, int64) {
	defer func() {
		self.queryCompletion = nil
	}()
	self.csCalibrationVal.Send([]int64{int64(self.oid)}, 0, 0)
	timeout := self.reactor().Monotonic() + 2
	params := self.completion().Wait(timeout, nil)
	if params != nil {
		result := params.(map[string]interface{})
		return result["BlockPreVal"].(int64), result["TargetVal"].(int64), result["RealVal"].(int64)
	}
	logger.Error("cs1237_calibration_Val response timeout")
	return -1, -1, -1
}

func (self *CS1237Module) handleDiff(params map[string]interface{}) error {
	params["raw"] = maths.Int64_conversion(params["raw"].(int64))
	if self.queryCompletion != nil {
		self.reactor().AsyncComplete(self.queryCompletion, params)
	}
	return nil
}

func (self *CS1237Module) Get_status(eventtime float64) map[string]interface{} {
	resp := map[string]interface{}{}
	if !self.report {
		return resp
	}
	resp["adc"] = self.adcValue
	resp["raw"] = self.rawValue
	resp["state"] = self.sensorState
	return resp
}

func (self *CS1237Module) Reset_cs(num int) {
	count := 3
	if num > 0 {
		count = num
	}
	self.resetCSCmd.Send([]int64{int64(self.oid), int64(count)}, 0, 0)
	toolheadObj := self.printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(cs1237Toolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement cs1237Toolhead: %T", toolheadObj))
	}
	toolhead.Dwell(0.1)
}

func (self *CS1237Module) enableCS1237([]interface{}) error {
	if self.enableCSCmd != nil {
		if self.enableCount == 0 {
			self.enableCSCmd.Send([]int64{int64(self.oid), 1}, 0, 0)
		}
		self.enableCount++
	}
	self.printer.GCode().RunScriptFromCommand("G4 P500")
	return nil
}

func (self *CS1237Module) disableCS1237([]interface{}) error {
	if self.enableCSCmd != nil {
		self.enableCount--
		if self.enableCount == 0 {
			self.enableCSCmd.Send([]int64{int64(self.oid), 0}, 0, 0)
		}
	}
	return nil
}

func (self *CS1237Module) Stats(eventtime float64) (bool, string) {
	return true, fmt.Sprintf("cs1237:adc_value=%d raw_value=%d sensor_state=%d", self.adcValue, self.rawValue, self.sensorState)
}
