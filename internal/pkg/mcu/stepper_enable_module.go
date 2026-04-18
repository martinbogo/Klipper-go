package mcu

import (
	"fmt"

	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

const disableStallTime = 0.100

type stepperEnablePinLookup interface {
	LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{}
}

type stepperEnablePinChip interface {
	Setup_pin(pinType string, pinParams map[string]interface{}) interface{}
}

type stepperEnableToolhead interface {
	Dwell(delay float64)
	Get_last_move_time() float64
}

type stepperEnableLegacyConfig interface {
	Get(option string, defaultValue interface{}, noteValid bool) interface{}
}

type stepperEnableStepper interface {
	Get_name(short bool) string
	Add_active_callback(callback func(float64))
}

type legacyActiveStepperAdapter struct {
	stepper stepperEnableStepper
}

func (self *legacyActiveStepperAdapter) AddActiveCallback(callback func(float64)) {
	self.stepper.Add_active_callback(callback)
}

type PrinterStepperEnableModule struct {
	printer     printerpkg.ModulePrinter
	enableLines map[string]*EnableTracking
}

func LoadConfigStepperEnable(config printerpkg.ModuleConfig) interface{} {
	return NewPrinterStepperEnableModule(config)
}

func NewPrinterStepperEnableModule(config printerpkg.ModuleConfig) *PrinterStepperEnableModule {
	self := &PrinterStepperEnableModule{
		printer:     config.Printer(),
		enableLines: map[string]*EnableTracking{},
	}
	self.printer.RegisterEventHandler("gcode:request_restart", self._handle_request_restart)
	gcode := self.printer.GCode()
	gcode.RegisterCommand("M18", self.cmdM18, false, "")
	gcode.RegisterCommand("M84", self.cmdM18, false, "")
	gcode.RegisterCommand("SET_STEPPER_ENABLE", self.cmdSetStepperEnable, false, self.cmd_SET_STEPPER_ENABLE_help())
	return self
}

func (self *PrinterStepperEnableModule) lookupPins() stepperEnablePinLookup {
	pinsObj := self.printer.LookupObject("pins", nil)
	pins, ok := pinsObj.(stepperEnablePinLookup)
	if !ok {
		panic(fmt.Sprintf("pins object does not implement stepperEnablePinLookup: %T", pinsObj))
	}
	return pins
}

func (self *PrinterStepperEnableModule) lookupToolhead() stepperEnableToolhead {
	toolheadObj := self.printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(stepperEnableToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement stepperEnableToolhead: %T", toolheadObj))
	}
	return toolhead
}

func (self *PrinterStepperEnableModule) setupEnablePin(pin string) *StepperEnablePin {
	if pin == "" {
		enable := NewStepperEnablePin(nil, 9999)
		enable.SetDedicated(false)
		return enable
	}

	pinParams := self.lookupPins().LookupPin(pin, true, false, "stepper_enable")
	if existing := pinParams["class"]; existing != nil {
		enable, ok := existing.(*StepperEnablePin)
		if !ok {
			panic(fmt.Sprintf("shared stepper enable class type invalid: %T", existing))
		}
		enable.SetDedicated(false)
		return enable
	}

	chip, ok := pinParams["chip"].(stepperEnablePinChip)
	if !ok {
		panic(fmt.Sprintf("stepper enable chip does not implement stepperEnablePinChip: %T", pinParams["chip"]))
	}
	digitalOutObj := chip.Setup_pin("digital_out", pinParams)
	digitalOut, ok := digitalOutObj.(printerpkg.DigitalOutPin)
	if !ok {
		panic(fmt.Sprintf("stepper enable pin does not implement printer.DigitalOutPin: %T", digitalOutObj))
	}
	digitalOut.SetupMaxDuration(0)
	enable := NewStepperEnablePin(digitalOut, 0)
	pinParams["class"] = enable
	return enable
}

func stepperEnablePinFromConfig(config interface{}) string {
	switch typed := config.(type) {
	case printerpkg.ModuleConfig:
		return typed.String("enable_pin", "", true)
	case stepperEnableLegacyConfig:
		value, ok := typed.Get("enable_pin", "", true).(string)
		if !ok {
			panic(fmt.Sprintf("legacy enable_pin value is not a string: %T", typed.Get("enable_pin", "", true)))
		}
		return value
	default:
		panic(fmt.Sprintf("unsupported stepper enable config type: %T", config))
	}
}

func (self *PrinterStepperEnableModule) RegisterStepper(config interface{}, stepper stepperEnableStepper) {
	name := stepper.Get_name(false)
	enable := self.setupEnablePin(stepperEnablePinFromConfig(config))
	self.enableLines[name] = NewEnableTracking(&legacyActiveStepperAdapter{stepper: stepper}, enable)
}

func (self *PrinterStepperEnableModule) Register_stepper(config interface{}, stepper stepperEnableStepper) {
	self.RegisterStepper(config, stepper)
}

func (self *PrinterStepperEnableModule) MotorOff() {
	toolhead := self.lookupToolhead()
	toolhead.Dwell(disableStallTime)
	printTime := toolhead.Get_last_move_time()
	for _, enableLine := range self.enableLines {
		enableLine.MotorDisable(printTime)
	}
	self.printer.SendEvent("stepper_enable:motor_off", []interface{}{printTime})
	toolhead.Dwell(disableStallTime)
}

func (self *PrinterStepperEnableModule) Motor_off() {
	self.MotorOff()
}

func (self *PrinterStepperEnableModule) motorDebugEnable(stepper string, enable bool) {
	toolhead := self.lookupToolhead()
	toolhead.Dwell(disableStallTime)
	printTime := toolhead.Get_last_move_time()
	enableLine := self.enableLines[stepper]
	if enable {
		enableLine.MotorEnable(printTime)
		logger.Debugf("%s has been manually enabled", stepper)
	} else {
		enableLine.MotorDisable(printTime)
		logger.Debugf("%s has been manually disabled", stepper)
	}
	toolhead.Dwell(disableStallTime)
}

func (self *PrinterStepperEnableModule) GetStatus(_ float64) map[string]interface{} {
	steppers := make([]map[string]bool, 0, len(self.enableLines))
	for name, enableLine := range self.enableLines {
		steppers = append(steppers, map[string]bool{name: enableLine.IsMotorEnabled()})
	}
	return map[string]interface{}{"steppers": steppers}
}

func (self *PrinterStepperEnableModule) Get_status(eventtime float64) map[string]interface{} {
	return self.GetStatus(eventtime)
}

func (self *PrinterStepperEnableModule) handleRequestRestart([]interface{}) error {
	self.MotorOff()
	return nil
}

func (self *PrinterStepperEnableModule) _handle_request_restart(args []interface{}) error {
	return self.handleRequestRestart(args)
}

func (self *PrinterStepperEnableModule) cmdM18(printerpkg.Command) error {
	self.MotorOff()
	return nil
}

func (self *PrinterStepperEnableModule) cmd_M18(argv interface{}) error {
	command, ok := argv.(printerpkg.Command)
	if !ok {
		panic(fmt.Sprintf("stepper enable command does not implement printer.Command: %T", argv))
	}
	return self.cmdM18(command)
}

func (self *PrinterStepperEnableModule) cmd_SET_STEPPER_ENABLE_help() string {
	return "Enable/disable individual stepper by name"
}

func (self *PrinterStepperEnableModule) cmdSetStepperEnable(gcmd printerpkg.Command) error {
	stepperName := gcmd.String("STEPPER", "")
	if _, ok := self.enableLines[stepperName]; !ok {
		gcmd.RespondInfo(fmt.Sprintf("set_stepper_enable: Invalid stepper %s", stepperName), true)
		return nil
	}
	stepperEnable := gcmd.Int("ENABLE", 1, nil, nil)
	logger.Debug("enable stepper motor:", stepperName)
	self.motorDebugEnable(stepperName, stepperEnable == 1)
	return nil
}

func (self *PrinterStepperEnableModule) cmd_SET_STEPPER_ENABLE(argv interface{}) error {
	command, ok := argv.(printerpkg.Command)
	if !ok {
		panic(fmt.Sprintf("stepper enable command does not implement printer.Command: %T", argv))
	}
	return self.cmdSetStepperEnable(command)
}

func (self *PrinterStepperEnableModule) LookupEnable(name string) (printerpkg.StepperEnableLine, error) {
	return self.Lookup_enable(name)
}

func (self *PrinterStepperEnableModule) SetStepperEnabled(stepperName string, printTime float64, enable bool) {
	stepperEnable, _ := self.Lookup_enable(stepperName)
	if enable {
		stepperEnable.MotorEnable(printTime)
		return
	}
	stepperEnable.MotorDisable(printTime)
}

func (self *PrinterStepperEnableModule) Lookup_enable(name string) (*EnableTracking, error) {
	enableLine, ok := self.enableLines[name]
	if !ok {
		return nil, fmt.Errorf("Unknown stepper '%s'", name)
	}
	return enableLine, nil
}

func (self *PrinterStepperEnableModule) StepperNames() []string {
	return self.Get_steppers()
}

func (self *PrinterStepperEnableModule) Get_steppers() []string {
	keys := make([]string, 0, len(self.enableLines))
	for name := range self.enableLines {
		keys = append(keys, name)
	}
	return keys
}
