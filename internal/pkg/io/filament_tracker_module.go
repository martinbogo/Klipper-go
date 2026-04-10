package io

import (
	"fmt"

	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

const filamentTrackerADCReportTime = 0.005
const filamentTrackerADCSampleTime = 0.001
const filamentTrackerADCSampleCount = 31
const filamentTrackerADCReferVoltage = 0.70

type filamentTrackerButtons interface {
	Register_buttons([]string, func(float64, int))
}

type FilamentTrackerStatus struct {
	Detection_length     float64 `json:"detection_length"`
	Filament_present     int     `json:"filament_present"`
	Encoder_pulse        int     `json:"encoder_pulse"`
	Encoder_signal_state int     `json:"encoder_signal_state"`
}

type filamentTrackerSignalStatus struct {
	rawADCValue float64
	eventTime   float64
}

type FilamentTrackerModule struct {
	printer         printerpkg.ModulePrinter
	reactor         printerpkg.ModuleReactor
	detectADC       printerpkg.ADCPin
	encoderADC      printerpkg.ADCPin
	blockPinState   filamentTrackerSignalStatus
	breakPinState   filamentTrackerSignalStatus
	filamentPresent int
	safeUnwindLen   float64
	lengthPerPulse  float64
	signalType      string
	trackerStatus   *FilamentTrackerStatus
	lastPosMap      map[string]float64
	callback        func(float64, int)
}

func LoadConfigFilamentTracker(config printerpkg.ModuleConfig) interface{} {
	return NewFilamentTrackerModule(config)
}

func NewFilamentTrackerModule(config printerpkg.ModuleConfig) *FilamentTrackerModule {
	self := &FilamentTrackerModule{
		printer:        config.Printer(),
		reactor:        config.Printer().Reactor(),
		safeUnwindLen:  config.Float("safe_unwind_len", 100.0),
		lengthPerPulse: config.Float("length_per_pulse", 0.0),
		signalType:     config.String("signal_type", "gpio", true),
		trackerStatus: &FilamentTrackerStatus{
			Filament_present:     0,
			Encoder_pulse:        0,
			Encoder_signal_state: 0,
			Detection_length:     -1.0,
		},
		lastPosMap: map[string]float64{},
	}
	breakPin := config.String("tracker_break_pin", "", true)
	blockPin := config.String("tracker_block_pin", "", true)
	if breakPin == "" || blockPin == "" {
		return self
	}

	if self.signalType == "adc" {
		pinsObj := self.printer.LookupObject("pins", nil)
		pins, ok := pinsObj.(printerpkg.PinRegistry)
		if !ok {
			panic(fmt.Sprintf("pins object does not implement PinRegistry: %T", pinsObj))
		}
		self.detectADC = pins.SetupADC(breakPin)
		self.detectADC.SetupCallback(filamentTrackerADCReportTime, self.breakButtonADCCallback)
		self.detectADC.SetupMinMax(filamentTrackerADCSampleTime, filamentTrackerADCSampleCount, 0, 1, 0)
		self.encoderADC = pins.SetupADC(blockPin)
		self.encoderADC.SetupCallback(filamentTrackerADCReportTime, self.blockButtonADCCallback)
		self.encoderADC.SetupMinMax(filamentTrackerADCSampleTime, filamentTrackerADCSampleCount, 0, 1, 0)
	} else if self.signalType == "gpio" {
		buttonsObj := config.LoadObject("buttons")
		buttons, ok := buttonsObj.(filamentTrackerButtons)
		if !ok {
			panic(fmt.Sprintf("buttons object does not implement filamentTrackerButtons: %T", buttonsObj))
		}
		buttons.Register_buttons([]string{breakPin, blockPin}, self.breakButtonIOHandler)
	}

	return self
}

func (self *FilamentTrackerModule) noteFilamentPresent(isFilamentPresent int) {
	if isFilamentPresent == self.trackerStatus.Filament_present {
		return
	}
	self.trackerStatus.Filament_present = isFilamentPresent
	if self.callback != nil {
		self.callback(self.reactor.Monotonic(), isFilamentPresent)
	}
}

func (self *FilamentTrackerModule) _note_filament_present(isFilamentPresent int) {
	self.noteFilamentPresent(isFilamentPresent)
}

func (self *FilamentTrackerModule) IsFilamentPresent() bool {
	return self.trackerStatus.Filament_present == 1
}

func (self *FilamentTrackerModule) Is_filament_present() bool {
	return self.IsFilamentPresent()
}

func (self *FilamentTrackerModule) breakButtonADCCallback(readTime float64, readValue float64) {
	self.breakPinState.rawADCValue = readValue
	self.breakPinState.eventTime = readTime
	self.updateTrackerState(readTime)
}

func (self *FilamentTrackerModule) _break_button_adc_handler(readTime float64, readValue float64) {
	self.breakButtonADCCallback(readTime, readValue)
}

func (self *FilamentTrackerModule) blockButtonADCCallback(readTime float64, readValue float64) {
	self.blockPinState.rawADCValue = readValue
	self.blockPinState.eventTime = readTime
	self.updateTrackerState(readTime)
}

func (self *FilamentTrackerModule) _block_button_adc_handler(readTime float64, readValue float64) {
	self.blockButtonADCCallback(readTime, readValue)
}

func (self *FilamentTrackerModule) RegisterCallback(callback func(float64, int)) {
	self.callback = callback
}

func (self *FilamentTrackerModule) Register_callback(callback func(float64, int)) {
	self.RegisterCallback(callback)
}

func (self *FilamentTrackerModule) updateTrackerState(eventtime float64) {
	if self.blockPinState.rawADCValue > filamentTrackerADCReferVoltage && self.breakPinState.rawADCValue > filamentTrackerADCReferVoltage {
		self.noteFilamentPresent(0)
	} else {
		self.noteFilamentPresent(1)
	}

	encoderState := 0
	if self.blockPinState.rawADCValue > filamentTrackerADCReferVoltage {
		encoderState = 1
	}
	if self.trackerStatus.Encoder_signal_state != encoderState {
		self.trackerStatus.Encoder_signal_state = encoderState
		self.trackerStatus.Encoder_pulse++
		if self.trackerStatus.Encoder_pulse%20 == 0 {
			logger.Debug("filament tracker encoder pulse:", self.trackerStatus.Encoder_pulse)
		}
	}
	_ = eventtime
	}

func (self *FilamentTrackerModule) breakButtonIOHandler(eventtime float64, state int) {
	currentState := 0
	if state == 0 {
		currentState = 0
	} else {
		currentState = 1
	}
	if self.filamentPresent != currentState {
		logger.Debug("filament state gpio:", state)
		self.filamentPresent = currentState
		if self.callback != nil {
			self.callback(eventtime, currentState)
		}
	}
	}

func (self *FilamentTrackerModule) _break_button_io_handler(eventtime float64, state int) {
	self.breakButtonIOHandler(eventtime, state)
}

func (self *FilamentTrackerModule) GetSafeUnwindLen() int {
	return int(self.safeUnwindLen)
}

func (self *FilamentTrackerModule) Get_safe_unwind_len() int {
	return self.GetSafeUnwindLen()
}

func (self *FilamentTrackerModule) GetDetectionLength() float64 {
	return self.trackerStatus.Detection_length
}

func (self *FilamentTrackerModule) Get_detection_length() float64 {
	return self.GetDetectionLength()
}

func (self *FilamentTrackerModule) StartPosRecord(label string) {
	self.lastPosMap[label] = self.trackerStatus.Detection_length
}

func (self *FilamentTrackerModule) GetPosRecord(label string) float64 {
	return self.trackerStatus.Detection_length - self.lastPosMap[label]
}

func (self *FilamentTrackerModule) Get_status(eventtime float64) map[string]interface{} {
	return map[string]interface{}{
		"detection_length":     self.trackerStatus.Detection_length,
		"filament_present":     self.trackerStatus.Filament_present,
		"encoder_pulse":        self.trackerStatus.Encoder_pulse,
		"encoder_signal_state": self.trackerStatus.Encoder_signal_state,
	}
	}