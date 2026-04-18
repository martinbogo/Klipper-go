package io

import (
	"fmt"
	"strconv"
	"strings"

	printerpkg "goklipper/internal/pkg/printer"
)

const buttonsQueryTime = .002
const buttonsRetransmitCount = 50

const adcButtonsReportTime = 0.015
const adcButtonsDebounceTime = 0.025
const adcButtonsSampleTime = 0.001
const adcButtonsSampleCount = 6

type buttonPinLookup interface {
	printerpkg.PinRegistry
	LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{}
}

type buttonCommand interface {
	Send(data interface{}, minclock int64, reqclock int64)
}

type buttonMCU interface {
	printerpkg.MCURuntime
	AllocCommandQueue() interface{}
	LookupCommand(msgformat string, cq interface{}) (interface{}, error)
}

type buttonReactor interface {
	printerpkg.ModuleReactor
	RegisterAsyncCallback(callback func(float64))
}

type configuredButtonPin struct {
	pin    string
	pullup int
}

type mcuButtons struct {
	reactor      buttonReactor
	mcu          buttonMCU
	pinList      []configuredButtonPin
	core         *DigitalButtonState
	ackCmd       buttonCommand
	commandQueue interface{}
	oid          int
}

func newMCUButtons(printer printerpkg.ModulePrinter, mcu buttonMCU) *mcuButtons {
	reactorObj := printer.Reactor()
	reactor, ok := reactorObj.(buttonReactor)
	if !ok {
		panic(fmt.Sprintf("reactor does not implement buttonReactor: %T", reactorObj))
	}
	self := &mcuButtons{
		reactor: reactor,
		mcu:     mcu,
		pinList: []configuredButtonPin{},
		core:    NewDigitalButtonState(),
	}
	self.mcu.RegisterConfigCallback(self.buildConfig)
	return self
}

func (self *mcuButtons) SetupButtons(pins []map[string]interface{}, callback func(float64, int)) {
	buttonPins := make([]ButtonPinConfig, 0, len(pins))
	for _, pinParams := range pins {
		buttonPins = append(buttonPins, ButtonPinConfig{Invert: buttonParamBool(pinParams["invert"])})
		self.pinList = append(self.pinList, configuredButtonPin{
			pin:    fmt.Sprintf("%v", pinParams["pin"]),
			pullup: buttonParamInt(pinParams["pullup"]),
		})
	}
	self.core.AddPins(buttonPins, callback)
	if self.commandQueue == nil {
		self.commandQueue = self.mcu.AllocCommandQueue()
	}
	self.ackCmd = nil
	self.oid = 0
}

func (self *mcuButtons) buildConfig() {
	if len(self.pinList) == 0 {
		return
	}
	self.oid = self.mcu.CreateOID()
	self.mcu.AddConfigCmd(fmt.Sprintf("config_buttons oid=%d button_count=%d", self.oid, self.core.PinCount()), false, false)
	for i, pinParams := range self.pinList {
		self.mcu.AddConfigCmd(fmt.Sprintf("buttons_add oid=%d pos=%d pin=%s pull_up=%d", self.oid, i, pinParams.pin, pinParams.pullup), true, false)
	}
	self.ackCmd = self.lookupCommand("buttons_ack oid=%c count=%c", self.commandQueue)
	clock := self.mcu.GetQuerySlot(self.oid)
	restTicks := self.mcu.SecondsToClock(buttonsQueryTime)
	self.mcu.AddConfigCmd(fmt.Sprintf("buttons_query oid=%d clock=%d rest_ticks=%d retransmit_count=%d invert=%d", self.oid, clock, restTicks, buttonsRetransmitCount, self.core.InvertMask()), true, false)
	self.mcu.RegisterResponse(self.handleButtonsState, "buttons_state", self.oid)
}

func (self *mcuButtons) lookupCommand(msgformat string, commandQueue interface{}) buttonCommand {
	command, err := self.mcu.LookupCommand(msgformat, commandQueue)
	if err != nil {
		panic(err)
	}
	typed, ok := command.(buttonCommand)
	if !ok {
		panic(fmt.Sprintf("command does not implement buttonCommand: %T", command))
	}
	return typed
}

func (self *mcuButtons) handleButtonsState(params map[string]interface{}) error {
	self.core.HandleButtonsState(params["ack_count"].(int64), params["state"].([]int),
		func(newCount int) {
			self.ackCmd.Send([]int64{int64(self.oid), int64(newCount)}, 0, 0)
		},
		func(callback func(float64)) {
			self.reactor.RegisterAsyncCallback(callback)
		},
	)
	return nil
}

type mcuADCButtons struct {
	reactor        buttonReactor
	pin            string
	mcuADC         printerpkg.ADCPin
	core           *ADCButtonState
	queryADCPrefix printerpkg.ADCQueryRegistry
}

func newMCUADCButtons(printer printerpkg.ModulePrinter, queryADC printerpkg.ADCQueryRegistry, pin string, pullup float64) *mcuADCButtons {
	reactorObj := printer.Reactor()
	reactor, ok := reactorObj.(buttonReactor)
	if !ok {
		panic(fmt.Sprintf("reactor does not implement buttonReactor: %T", reactorObj))
	}
	pinsObj := printer.LookupObject("pins", nil)
	pins, ok := pinsObj.(buttonPinLookup)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement buttonPinLookup: %T", pinsObj))
	}
	adcPin := pins.SetupADC(pin)
	adcPin.SetupMinMax(adcButtonsSampleTime, adcButtonsSampleCount, 0, 1, 0)
	self := &mcuADCButtons{
		reactor:        reactor,
		pin:            pin,
		mcuADC:         adcPin,
		core:           NewADCButtonState(pullup, adcButtonsDebounceTime),
		queryADCPrefix: queryADC,
	}
	adcPin.SetupCallback(adcButtonsReportTime, self.adcCallback)
	queryADC.RegisterADC("adc_button:"+strings.TrimSpace(pin), adcPin)
	return self
}

func (self *mcuADCButtons) SetupButton(minValue float64, maxValue float64, callback func(float64, bool)) {
	self.core.AddButton(minValue, maxValue, callback)
}

func (self *mcuADCButtons) adcCallback(readTime float64, readValue float64) {
	self.core.ADCCallback(readTime, readValue, func(callback func(float64)) {
		self.reactor.RegisterAsyncCallback(callback)
	})
}

type PrinterButtonsModule struct {
	printer    printerpkg.ModulePrinter
	queryADC   printerpkg.ADCQueryRegistry
	mcuButtons map[string]*mcuButtons
	adcButtons map[string]*mcuADCButtons
}

func LoadConfigButtons(config printerpkg.ModuleConfig) interface{} {
	return NewPrinterButtonsModule(config)
}

func NewPrinterButtonsModule(config printerpkg.ModuleConfig) *PrinterButtonsModule {
	queryADCObj := config.LoadObject("query_adc")
	queryADC, ok := queryADCObj.(printerpkg.ADCQueryRegistry)
	if !ok {
		panic(fmt.Sprintf("query_adc object does not implement ADCQueryRegistry: %T", queryADCObj))
	}
	return &PrinterButtonsModule{
		printer:    config.Printer(),
		queryADC:   queryADC,
		mcuButtons: map[string]*mcuButtons{},
		adcButtons: map[string]*mcuADCButtons{},
	}
}

func (self *PrinterButtonsModule) RegisterADCButton(pin string, minVal float64, maxVal float64, pullup float64, callback func(eventTime float64, state bool)) {
	adcButtons := self.adcButtons[pin]
	if adcButtons == nil {
		adcButtons = newMCUADCButtons(self.printer, self.queryADC, pin, pullup)
		self.adcButtons[pin] = adcButtons
	}
	adcButtons.SetupButton(minVal, maxVal, callback)
}

func (self *PrinterButtonsModule) Register_adc_button(pin string, minVal float64, maxVal float64, pullup float64, callback func(eventTime float64, state bool)) {
	self.RegisterADCButton(pin, minVal, maxVal, pullup, callback)
}

func (self *PrinterButtonsModule) RegisterADCButtonPush(pin string, minVal float64, maxVal float64, pullup float64, callback func(eventTime float64)) {
	helper := func(eventTime float64, state bool) {
		if state {
			callback(eventTime)
		}
	}
	self.RegisterADCButton(pin, minVal, maxVal, pullup, helper)
}

func (self *PrinterButtonsModule) Register_adc_button_push(pin string, minVal float64, maxVal float64, pullup float64, callback func(eventTime float64)) {
	self.RegisterADCButtonPush(pin, minVal, maxVal, pullup, callback)
}

func (self *PrinterButtonsModule) RegisterButtons(pins []string, callback func(float64, int)) {
	if len(pins) == 0 {
		return
	}
	pinRegistryObj := self.printer.LookupObject("pins", nil)
	pinRegistry, ok := pinRegistryObj.(buttonPinLookup)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement buttonPinLookup: %T", pinRegistryObj))
	}

	mcuName := ""
	pinParamsList := make([]map[string]interface{}, 0, len(pins))
	for _, pin := range pins {
		pinParams := pinRegistry.LookupPin(pin, true, true, nil)
		chipName := fmt.Sprintf("%v", pinParams["chip_name"])
		if mcuName != "" && chipName != mcuName {
			panic("button pins must be on same mcu")
		}
		mcuName = chipName
		pinParamsList = append(pinParamsList, pinParams)
	}

	mcuObj := self.printer.LookupMCU(mcuName)
	mcu, ok := mcuObj.(buttonMCU)
	if !ok {
		panic(fmt.Sprintf("mcu object does not implement buttonMCU: %T", mcuObj))
	}
	mcuButtons := self.mcuButtons[mcuName]
	if mcuButtons == nil || len(mcuButtons.pinList)+len(pinParamsList) > 8 {
		mcuButtons = newMCUButtons(self.printer, mcu)
		self.mcuButtons[mcuName] = mcuButtons
	}
	mcuButtons.SetupButtons(pinParamsList, callback)
}

func (self *PrinterButtonsModule) Register_buttons(pins []string, callback func(float64, int)) {
	self.RegisterButtons(pins, callback)
}

func (self *PrinterButtonsModule) RegisterRotaryEncoder(pin1 string, pin2 string, cwCallback func(), ccwCallback func(), stepsPerDetent int) {
	var encoder *RotaryEncoder
	if stepsPerDetent == 2 {
		encoder = NewHalfStepRotaryEncoder(func(eventtime float64) {
			cwCallback()
		}, func(eventtime float64) {
			ccwCallback()
		})
	} else if stepsPerDetent == 4 {
		encoder = NewFullStepRotaryEncoder(func(eventtime float64) {
			cwCallback()
		}, func(eventtime float64) {
			ccwCallback()
		})
	} else {
		panic(fmt.Sprintf("%d steps per detent not supported", stepsPerDetent))
	}
	self.RegisterButtons([]string{pin1, pin2}, encoder.Encoder_callback)
}

func (self *PrinterButtonsModule) Register_rotary_encoder(pin1 string, pin2 string, cwCallback func(), ccwCallback func(), stepsPerDetent int) {
	self.RegisterRotaryEncoder(pin1, pin2, cwCallback, ccwCallback, stepsPerDetent)
}

func (self *PrinterButtonsModule) RegisterButtonPush(pin string, callback func(float64)) {
	helper := func(eventtime float64, state int) {
		if state == 1 {
			callback(eventtime)
		}
	}
	self.RegisterButtons([]string{pin}, helper)
}

func (self *PrinterButtonsModule) Register_button_push(pin string, callback func(float64)) {
	self.RegisterButtonPush(pin, callback)
}

func buttonParamBool(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed == "1" || strings.EqualFold(trimmed, "true")
	default:
		trimmed := strings.TrimSpace(fmt.Sprintf("%v", value))
		return trimmed == "1" || strings.EqualFold(trimmed, "true")
	}
}

func buttonParamInt(value interface{}) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case bool:
		if typed {
			return 1
		}
		return 0
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			panic(fmt.Sprintf("unable to parse button int parameter %q: %v", typed, err))
		}
		return parsed
	default:
		parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprintf("%v", value)))
		if err != nil {
			panic(fmt.Sprintf("unable to parse button int parameter %v: %v", value, err))
		}
		return parsed
	}
}
