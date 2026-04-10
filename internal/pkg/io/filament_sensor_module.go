package io

import (
	"fmt"
	"strings"

	"goklipper/common/constants"
	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
	printpkg "goklipper/internal/print"
)

const checkRunoutTimeout = .250

type runoutOptionConfig interface {
	HasOption(option string) bool
}

type runoutGCode interface {
	printerpkg.GCodeRuntime
	RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string)
}

type runoutReactor interface {
	printerpkg.ModuleReactor
	Pause(waketime float64) float64
	RegisterCallback(callback func(float64), waketime float64)
}

type runoutPauseResume interface {
	Send_pause_command()
}

type runoutIdleTimeout interface {
	Get_status(eventtime float64) map[string]interface{}
}

type runoutButtons interface {
	Register_buttons([]string, func(float64, int))
}

type runoutExtruder interface {
	Find_past_position(printTime float64) float64
}

type runoutMCUEstimator interface {
	Estimated_print_time(eventtime float64) float64
}

type runoutHelper struct {
	name            string
	printer         printerpkg.ModulePrinter
	reactor         runoutReactor
	gcode           runoutGCode
	runoutPause     bool
	runoutGCode     printerpkg.Template
	insertGCode     printerpkg.Template
	pauseDelay      float64
	eventDelay      float64
	minEventSystime float64
	filamentPresent bool
	sensorEnabled   bool
}

func newRunoutHelper(config printerpkg.ModuleConfig) *runoutHelper {
	reactorObj := config.Printer().Reactor()
	reactor, ok := reactorObj.(runoutReactor)
	if !ok {
		panic(fmt.Sprintf("reactor does not implement runoutReactor: %T", reactorObj))
	}
	gcodeObj := config.Printer().GCode()
	gcode, ok := gcodeObj.(runoutGCode)
	if !ok {
		panic(fmt.Sprintf("gcode runtime does not implement runoutGCode: %T", gcodeObj))
	}
	nameParts := strings.Fields(config.Name())
	name := config.Name()
	if len(nameParts) > 0 {
		name = nameParts[len(nameParts)-1]
	}
	self := &runoutHelper{
		name:            name,
		printer:         config.Printer(),
		reactor:         reactor,
		gcode:           gcode,
		runoutPause:     config.Bool("pause_on_runout", true),
		pauseDelay:      config.Float("pause_delay", 0.5),
		eventDelay:      config.Float("event_delay", 3.0),
		minEventSystime: constants.NEVER,
		filamentPresent: false,
		sensorEnabled:   true,
	}
	if self.runoutPause {
		config.LoadObject("pause_resume")
	}
	if self.runoutPause || hasConfigOption(config, "runout_gcode") {
		self.runoutGCode = config.LoadTemplate("gcode_macro_1", "runout_gcode", "")
	}
	if hasConfigOption(config, "insert_gcode") {
		self.insertGCode = config.LoadTemplate("gcode_macro_1", "insert_gcode", "")
	}
	self.printer.RegisterEventHandler("project:ready", self.handleReady)
	self.gcode.RegisterMuxCommand("QUERY_FILAMENT_SENSOR", "SENSOR", self.name, self.cmdQueryFilamentSensor, self.cmdQueryFilamentSensorHelp())
	self.gcode.RegisterMuxCommand("SET_FILAMENT_SENSOR", "SENSOR", self.name, self.cmdSetFilamentSensor, self.cmdSetFilamentSensorHelp())
	return self
}

func hasConfigOption(config printerpkg.ModuleConfig, option string) bool {
	typed, ok := config.(runoutOptionConfig)
	if !ok {
		return false
	}
	return typed.HasOption(option)
}

func requiredConfigString(config printerpkg.ModuleConfig, option string) string {
	if !hasConfigOption(config, option) {
		panic(fmt.Sprintf("Option '%s' in section '%s' must be specified", option, config.Name()))
	}
	return config.String(option, "", true)
}

func (self *runoutHelper) handleReady([]interface{}) error {
	self.minEventSystime = self.reactor.Monotonic() + 2.
	return nil
}

func (self *runoutHelper) runoutEvent(eventtime float64) {
	pausePrefix := ""
	if self.runoutPause {
		pauseResumeObj := self.printer.LookupObject("pause_resume", nil)
		pauseResume, ok := pauseResumeObj.(runoutPauseResume)
		if !ok {
			panic(fmt.Sprintf("pause_resume object does not implement runoutPauseResume: %T", pauseResumeObj))
		}
		pauseResume.Send_pause_command()
		pausePrefix = "PAUSE\n"
		self.reactor.Pause(eventtime + self.pauseDelay)
	}
	self.execGCode(pausePrefix, self.runoutGCode)
}

func (self *runoutHelper) insertEvent(float64) {
	self.execGCode("", self.insertGCode)
}

func (self *runoutHelper) execGCode(prefix string, template printerpkg.Template) {
	if template == nil {
		return
	}
	script, err := template.Render(nil)
	if err != nil {
		panic(fmt.Sprintf("script running error: %v", err))
	}
	self.gcode.RunScript(prefix + script + "\nM400")
	self.minEventSystime = self.reactor.Monotonic() + self.eventDelay
}

func (self *runoutHelper) NoteFilamentPresent(isFilamentPresent bool) {
	if isFilamentPresent == self.filamentPresent {
		return
	}
	self.filamentPresent = isFilamentPresent
	eventtime := self.reactor.Monotonic()
	if eventtime < self.minEventSystime || !self.sensorEnabled {
		return
	}
	idleTimeoutObj := self.printer.LookupObject("idle_timeout", nil)
	idleTimeout, ok := idleTimeoutObj.(runoutIdleTimeout)
	if !ok {
		panic(fmt.Sprintf("idle_timeout object does not implement runoutIdleTimeout: %T", idleTimeoutObj))
	}
	isPrinting := idleTimeout.Get_status(eventtime)["state"] == printpkg.StatePrinting
	if isFilamentPresent {
		if !isPrinting && self.insertGCode != nil {
			self.minEventSystime = constants.NEVER
			logger.Infof("Filament Sensor %s: insert event detected, Time %.2f", self.name, eventtime)
			self.reactor.RegisterCallback(self.insertEvent, constants.NOW)
		}
		return
	}
	if isPrinting && self.runoutGCode != nil {
		self.minEventSystime = constants.NEVER
		logger.Infof("Filament Sensor %s: runout event detected, Time %.2f", self.name, eventtime)
		self.reactor.RegisterCallback(self.runoutEvent, constants.NOW)
	}
}

func (self *runoutHelper) Note_filament_present(isFilamentPresent bool) {
	self.NoteFilamentPresent(isFilamentPresent)
}

func (self *runoutHelper) FilamentPresent() bool {
	return self.filamentPresent
}

func (self *runoutHelper) GetStatus(eventtime float64) map[string]interface{} {
	return map[string]interface{}{
		"name":              self.name,
		"filament_detected": self.filamentPresent,
		"enabled":           self.sensorEnabled,
	}
}

func (self *runoutHelper) Get_status(eventtime float64) map[string]interface{} {
	return self.GetStatus(eventtime)
}

func (self *runoutHelper) cmdQueryFilamentSensorHelp() string {
	return "Query the status of the Filament Sensor"
}

func (self *runoutHelper) cmdQueryFilamentSensor(gcmd printerpkg.Command) error {
	message := fmt.Sprintf("Filament Sensor %s: filament not detected", self.name)
	if self.filamentPresent {
		message = fmt.Sprintf("Filament Sensor %s: filament detected", self.name)
	}
	gcmd.RespondInfo(message, true)
	return nil
}

func (self *runoutHelper) cmdSetFilamentSensorHelp() string {
	return "Sets the filament sensor on/off"
}

func (self *runoutHelper) cmdSetFilamentSensor(gcmd printerpkg.Command) error {
	self.sensorEnabled = gcmd.Int("ENABLE", 1, nil, nil) > 0
	return nil
}

func (self *runoutHelper) Cmd_QUERY_FILAMENT_SENSOR_help() string {
	return self.cmdQueryFilamentSensorHelp()
}

func (self *runoutHelper) Cmd_SET_FILAMENT_SENSOR_help() string {
	return self.cmdSetFilamentSensorHelp()
}

type SwitchSensorModule struct {
	runoutHelper *runoutHelper
}

func LoadConfigSwitchSensor(config printerpkg.ModuleConfig) interface{} {
	return NewSwitchSensorModule(config)
}

func NewSwitchSensorModule(config printerpkg.ModuleConfig) *SwitchSensorModule {
	buttonsObj := config.LoadObject("buttons")
	buttons, ok := buttonsObj.(runoutButtons)
	if !ok {
		panic(fmt.Sprintf("buttons object does not implement runoutButtons: %T", buttonsObj))
	}
	self := &SwitchSensorModule{runoutHelper: newRunoutHelper(config)}
	buttons.Register_buttons([]string{requiredConfigString(config, "switch_pin")}, self.buttonHandler)
	return self
}

func (self *SwitchSensorModule) buttonHandler(eventtime float64, state int) {
	self.runoutHelper.NoteFilamentPresent(state > 0)
}

func (self *SwitchSensorModule) _button_handler(eventtime float64, state int) {
	self.buttonHandler(eventtime, state)
}

func (self *SwitchSensorModule) FilamentPresent() bool {
	return self.runoutHelper.FilamentPresent()
}

func (self *SwitchSensorModule) GetStatus(eventtime float64) map[string]interface{} {
	return self.runoutHelper.GetStatus(eventtime)
}

func (self *SwitchSensorModule) Get_status(eventtime float64) map[string]interface{} {
	return self.GetStatus(eventtime)
}

type EncoderSensorModule struct {
	printer                printerpkg.ModulePrinter
	extruderName           string
	detectionLength        float64
	reactor                printerpkg.ModuleReactor
	runoutHelper           *runoutHelper
	extruder               runoutExtruder
	estimatedPrintTime     func(float64) float64
	filamentRunoutPos      float64
	extruderPosUpdateTimer printerpkg.TimerHandle
}

func LoadConfigPrefixEncoderSensor(config printerpkg.ModuleConfig) interface{} {
	return NewEncoderSensorModule(config)
}

func NewEncoderSensorModule(config printerpkg.ModuleConfig) *EncoderSensorModule {
	buttonsObj := config.LoadObject("buttons")
	buttons, ok := buttonsObj.(runoutButtons)
	if !ok {
		panic(fmt.Sprintf("buttons object does not implement runoutButtons: %T", buttonsObj))
	}
	self := &EncoderSensorModule{
		printer:         config.Printer(),
		extruderName:    requiredConfigString(config, "extruder"),
		detectionLength: config.Float("detection_length", 7.),
		reactor:         config.Printer().Reactor(),
		runoutHelper:    newRunoutHelper(config),
		extruder:        nil,
		estimatedPrintTime: nil,
		filamentRunoutPos:  0,
	}
	buttons.Register_buttons([]string{requiredConfigString(config, "switch_pin")}, self.encoderEvent)
	self.printer.RegisterEventHandler("project:ready", self.handleReady)
	self.printer.RegisterEventHandler("idle_timeout:printing", self.handlePrinting)
	self.printer.RegisterEventHandler("idle_timeout:ready", self.handleNotPrinting)
	self.printer.RegisterEventHandler("idle_timeout:idle", self.handleNotPrinting)
	return self
}

func (self *EncoderSensorModule) updateFilamentRunoutPos(eventtime float64) {
	if eventtime == 0 {
		eventtime = self.reactor.Monotonic()
	}
	self.filamentRunoutPos = self.getExtruderPos(eventtime) + self.detectionLength
}

func (self *EncoderSensorModule) handleReady([]interface{}) error {
	extruderObj := self.printer.LookupObject(self.extruderName, nil)
	extruder, ok := extruderObj.(runoutExtruder)
	if !ok {
		panic(fmt.Sprintf("extruder object does not implement runoutExtruder: %T", extruderObj))
	}
	mcuObj := self.printer.LookupObject("mcu", nil)
	mcu, ok := mcuObj.(runoutMCUEstimator)
	if !ok {
		panic(fmt.Sprintf("mcu object does not implement runoutMCUEstimator: %T", mcuObj))
	}
	self.extruder = extruder
	self.estimatedPrintTime = mcu.Estimated_print_time
	self.updateFilamentRunoutPos(0)
	self.extruderPosUpdateTimer = self.reactor.RegisterTimer(self.extruderPosUpdateEvent, constants.NEVER)
	return nil
}

func (self *EncoderSensorModule) handlePrinting([]interface{}) error {
	if self.extruderPosUpdateTimer != nil {
		self.extruderPosUpdateTimer.Update(constants.NOW)
	}
	return nil
}

func (self *EncoderSensorModule) handleNotPrinting([]interface{}) error {
	if self.extruderPosUpdateTimer != nil {
		self.extruderPosUpdateTimer.Update(constants.NEVER)
	}
	return nil
}

func (self *EncoderSensorModule) getExtruderPos(eventtime float64) float64 {
	if eventtime == 0 {
		eventtime = self.reactor.Monotonic()
	}
	printTime := self.estimatedPrintTime(eventtime)
	return self.extruder.Find_past_position(printTime)
}

func (self *EncoderSensorModule) extruderPosUpdateEvent(eventtime float64) float64 {
	extruderPos := self.getExtruderPos(eventtime)
	self.runoutHelper.NoteFilamentPresent(extruderPos < self.filamentRunoutPos)
	return eventtime + checkRunoutTimeout
}

func (self *EncoderSensorModule) encoderEvent(eventtime float64, state int) {
	if self.extruder == nil {
		return
	}
	self.updateFilamentRunoutPos(eventtime)
	self.runoutHelper.NoteFilamentPresent(true)
	_ = state
}

func (self *EncoderSensorModule) FilamentPresent() bool {
	return self.runoutHelper.FilamentPresent()
}

func (self *EncoderSensorModule) GetStatus(eventtime float64) map[string]interface{} {
	return self.runoutHelper.GetStatus(eventtime)
}

func (self *EncoderSensorModule) Get_status(eventtime float64) map[string]interface{} {
	return self.GetStatus(eventtime)
}

func (self *EncoderSensorModule) _update_filament_runout_pos(eventtime float64) {
	self.updateFilamentRunoutPos(eventtime)
}

func (self *EncoderSensorModule) _handle_ready(argv []interface{}) error {
	return self.handleReady(argv)
}

func (self *EncoderSensorModule) _handle_printing(argv []interface{}) error {
	return self.handlePrinting(argv)
}

func (self *EncoderSensorModule) _handle_not_printing(argv []interface{}) error {
	return self.handleNotPrinting(argv)
}

func (self *EncoderSensorModule) _get_extruder_pos(eventtime float64) float64 {
	return self.getExtruderPos(eventtime)
}

func (self *EncoderSensorModule) _extruder_pos_update_event(eventtime float64) float64 {
	return self.extruderPosUpdateEvent(eventtime)
}

func (self *EncoderSensorModule) encoder_event(eventtime float64, state int) {
	self.encoderEvent(eventtime, state)
}