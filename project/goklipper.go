package project

import (
	"container/list"
	"context"
	"errors"
	"flag"
	"fmt"
	"goklipper/common/configparser"
	"goklipper/common/constants"
	"goklipper/common/lock"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/collections"
	"goklipper/common/utils/file"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	"goklipper/common/utils/sys"
	"goklipper/common/value"
	addonpkg "goklipper/internal/addon"
	"goklipper/internal/pkg/chelper"
	pkgconfig "goklipper/internal/pkg/config"
	filamentpkg "goklipper/internal/pkg/filament"
	gcodepkg "goklipper/internal/pkg/gcode"
	heaterpkg "goklipper/internal/pkg/heater"
	mcupkg "goklipper/internal/pkg/mcu"
	moduleinitpkg "goklipper/internal/pkg/moduleinit"
	motionpkg "goklipper/internal/pkg/motion"
	bedmeshpkg "goklipper/internal/pkg/motion/bed_mesh"
	homingpkg "goklipper/internal/pkg/motion/homing"
	kinematicspkg "goklipper/internal/pkg/motion/kinematics"
	probepkg "goklipper/internal/pkg/motion/probe"
	vibrationpkg "goklipper/internal/pkg/motion/vibration"
	"goklipper/internal/pkg/msgproto"
	printerpkg "goklipper/internal/pkg/printer"
	reactorpkg "goklipper/internal/pkg/reactor"
	serialpkg "goklipper/internal/pkg/serialhdl"
	tmcpkg "goklipper/internal/pkg/tmc"
	"goklipper/internal/pkg/util"
	webhookspkg "goklipper/internal/pkg/webhooks"
	printpkg "goklipper/internal/print"
	"runtime/debug"

	"io/ioutil"
	"log"
	"math"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	message_ready   = "Printer is ready"
	message_startup = "Printer is not ready\nThe project host software is attempting to connect.Please\nretry in a few moments."

	message_restart = "Once the underlying issue is corrected, use the \"RESTART\"\ncommand to reload the config and restart the host software.\nPrinter is halted"

	message_protocol_error1 = "This is frequently caused by running an older version of the\nfirmware on the MCU(s).Fix by recompiling and flashing the\nfirmware."

	message_protocol_error2 = "Once the underlying issue is corrected, use the \"RESTART\"\ncommand to reload the config and restart the host software."

	message_mcu_connect_error = "Once the underlying issue is corrected, use the\n\"FIRMWARE_RESTART\" command to reset the firmware, reload the\nconfig, and restart the host software.\nError configuring printer"

	message_shutdown = "Once the underlying issue is corrected, use the\n\"FIRMWARE_RESTART\" command to reset the firmware, reload the\nconfig, and restart the host software.Printer is shutdown"
)

type (
	IReactor           = reactorpkg.IReactor
	ReactorFileHandler = reactorpkg.ReactorFileHandler
	ReactorTimer       = reactorpkg.ReactorTimer
	ReactorCompletion  = reactorpkg.ReactorCompletion
	ReactorCallback    = reactorpkg.ReactorCallback
	ReactorMutex       = reactorpkg.ReactorMutex
	SelectReactor      = reactorpkg.SelectReactor
	Poll               = reactorpkg.Poll
	PollReactor        = reactorpkg.PollReactor
	EPollReactor       = reactorpkg.EPollReactor
	GCodeCommand       = gcodepkg.DispatchCommand
	GCodeDispatch      = gcodepkg.Dispatcher
)

var (
	NewReactorFileHandler = reactorpkg.NewReactorFileHandler
	NewReactorTimer       = reactorpkg.NewReactorTimer
	NewReactorCompletion  = reactorpkg.NewReactorCompletion
	NewReactorCallback    = reactorpkg.NewReactorCallback
	NewReactorMutex       = reactorpkg.NewReactorMutex
	NewSelectReactor      = reactorpkg.NewSelectReactor
	NewPollReactor        = reactorpkg.NewPollReactor
	NewEPollReactor       = reactorpkg.NewEPollReactor
)

type Printer struct {
	runtime        *printerpkg.Runtime
	config_error   *Config_error
	Command_error  *CommandError
	reactor        IReactor
	reactorAdapter *printerpkg.ReactorAdapter
	Module         *printerpkg.ModuleRegistry
}

type CommandError struct {
	E string
}

type (
	WebRequest = webhookspkg.ConnectedRequest
)

type WebHooks struct {
	printer *Printer
	*webhookspkg.EndpointRegistry
	printerRegistry printerpkg.WebhookRegistry
	*webhookspkg.ServerSocket
}

var _ webhookspkg.RuntimeRegistry = (*WebHooks)(nil)

func NewWebHooks(printer *Printer) *WebHooks {
	self := &WebHooks{
		printer:          printer,
		EndpointRegistry: webhookspkg.NewEndpointRegistry(),
	}
	self.printerRegistry = webhookspkg.NewPrinterRegistry(self)
	_ = webhookspkg.RegisterTypedEndpoints[*WebRequest](self, []webhookspkg.TypedEndpointBinding[*WebRequest]{
		{Path: "info", Handler: self._handle_info_request},
		{Path: "Query/K3cInfo", Handler: self._handle_k3c_info},
		{Path: "emergency_stop", Handler: self._handle_estop_request},
		{Path: "filament_hub/get_config", Handler: self._handle_filament_hub_get_config},
		{Path: "filament_hub/set_config", Handler: self._handle_filament_hub_set_config},
		{Path: "filament_hub/start_drying", Handler: self._handle_filament_hub_start_drying},
		{Path: "filament_hub/stop_drying", Handler: self._handle_filament_hub_stop_drying},
		{Path: "filament_hub/filament_info", Handler: self._handle_filament_hub_filament_info},
		{Path: "filament_hub/info", Handler: self._handle_filament_hub_info},
		{Path: "filament_hub/query_version", Handler: self._handle_filament_hub_query_version},
		{Path: "filament_hub/set_filament_info", Handler: self._handle_filament_hub_set_filament_info},
		{Path: "print/query_resume_print", Handler: self._handle_print_query_resume_print},
	})
	_ = self.RegisterEndpoint("register_remote_method", self.HandleRemoteMethodRegistration)
	serverAddress := ""
	start_args := printer.Get_start_args()
	server_address, is_server_address := start_args["apiserver"]
	_, is_fileinput := start_args["debuginput"]
	if is_server_address && !is_fileinput {
		serverAddress = server_address.(string)
	}
	self.ServerSocket = webhookspkg.NewServerSocket(printer.Get_reactor(), serverAddress, self.EndpointRegistry, webhookspkg.NewConnectedEnvelope, self.handleClientInfo)
	printer.Register_event_handler("project:disconnect", self.ServerSocket.HandleDisconnect)
	printer.Register_event_handler("project:shutdown", self.ServerSocket.HandleShutdown)
	return self
}

func (self *WebHooks) handleClientInfo(client *webhookspkg.ClientConnection, clientInfo string) {
	logID := fmt.Sprintf("webhooks %s", client.UID())
	if clientInfo == "" {
		self.printer.Set_rollover_info(logID, "", false)
		return
	}
	rolloverMsg := fmt.Sprintf("webhooks client %s: %s", client.UID(), clientInfo)
	self.printer.Set_rollover_info(logID, rolloverMsg, false)
}

func (self *Printer) Webhooks() printerpkg.WebhookRegistry {
	webhooks := MustLookupWebhooks(self)
	return webhooks.printerRegistry
}

func MustLookupWebhooks(printer *Printer) *WebHooks {
	webhooks, ok := printer.Lookup_object("webhooks", object.Sentinel{}).(*WebHooks)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "webhooks", printer.Lookup_object("webhooks", object.Sentinel{})))
	}
	return webhooks
}

func Add_early_printer_objects_webhooks(printer *Printer) {
	webhooks := NewWebHooks(printer)
	printer.Add_object("webhooks", webhooks)
	webhookspkg.RegisterGCodeEndpoints(webhooks, MustLookupGcode(printer))
	webhookspkg.NewQueryStatusHelper(printer, webhooks, nil)
}

func (self *WebHooks) _handle_k3c_info(web_request *WebRequest) (interface{}, error) {
	state_message, state := self.printer.get_state_message()
	response := map[string]interface{}{
		"state":         state,
		"state_message": state_message,
		"ready":         state == "ready",
	}
	web_request.Send(response)
	return nil, nil
}

func (self *WebHooks) _handle_info_request(web_request *WebRequest) (interface{}, error) {
	client_info := web_request.Get_dict("client_info", nil)
	if client_info != nil {
		web_request.ClientConnection().SetClientInfo(fmt.Sprintf("%s", client_info), "")
	}
	state_message, state := self.printer.get_state_message()
	response := webhookspkg.BuildInfoResponse(state, state_message, self.printer.Get_start_args())
	web_request.Send(response)
	return nil, nil
}

func (self *WebHooks) _handle_estop_request(web_request *WebRequest) (interface{}, error) {
	self.printer.Invoke_shutdown("Shutdown due to webhooks request")
	return nil, nil
}

func (self *WebHooks) Get_status(eventtime float64) map[string]interface{} {
	state_message, state := self.printer.get_state_message()
	return map[string]interface{}{"state": state, "state_message": state_message}
}

func (self *WebHooks) aceSlotStatus(index int) map[string]interface{} {
	aceObj := self.printer.Lookup_object("filament_hub", nil)
	if aceObj == nil {
		return nil
	}
	type filamentStatusGetter interface {
		Get_status(eventtime float64) map[string]interface{}
	}
	ace, ok := aceObj.(filamentStatusGetter)
	if !ok {
		return nil
	}
	status := ace.Get_status(0)
	hubs, ok := status["filament_hubs"].([]interface{})
	if !ok || len(hubs) == 0 {
		return nil
	}
	hub, ok := hubs[0].(map[string]interface{})
	if !ok {
		return nil
	}
	slots, ok := hub["slots"].([]interface{})
	if !ok || index < 0 || index >= len(slots) {
		return nil
	}
	slot, _ := slots[index].(map[string]interface{})
	return slot
}

func (self *WebHooks) _handle_filament_hub_get_config(web_request *WebRequest) (interface{}, error) {
	logger.Infof("filament_hub/get_config requested")

	response := map[string]interface{}{
		"auto_refill":               filamentpkg.ReadAutoRefillConfig(filamentpkg.AmsConfigPath),
		"flush_multiplier":          1.5,
		"flush_multiplier_editable": 1,
		"flush_volume_max":          800,
		"flush_volume_min":          107,
		"runout_detect":             1,
	}
	web_request.Send(response)
	return nil, nil
}

func (self *WebHooks) _handle_filament_hub_set_config(web_request *WebRequest) (interface{}, error) {
	logger.Infof("filament_hub/set_config requested with params: %v", web_request.Params)

	if gcode, _ := self.printer.Lookup_object("gcode", nil).(*GCodeDispatch); gcode != nil {
		for k, v := range web_request.Params {
			if k == "auto_refill" {
				filamentpkg.UpdateAutoRefillInConfig(filamentpkg.AmsConfigPath, v)
				if filamentpkg.IsAutoRefillEnabled(v) {
					gcode.Run_script("ACE_ENABLE_ENDLESS_SPOOL")
				} else {
					gcode.Run_script("ACE_DISABLE_ENDLESS_SPOOL")
				}
			}

			valueStr, ok := filamentpkg.FormatConfigValue(v)
			if !ok {
				continue
			}
			cmd := fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_%s VALUE=%s", k, valueStr)
			gcode.Run_script(cmd)
		}
	}

	web_request.Send(map[string]interface{}{})
	return nil, nil
}

func (self *WebHooks) _handle_print_query_resume_print(web_request *WebRequest) (interface{}, error) {
	logger.Infof("Mock print/query_resume_print requested")
	response := map[string]interface{}{
		"can_resume": false,
	}
	web_request.Send(response)
	return nil, nil
}

func (self *WebHooks) _handle_filament_hub_filament_info(web_request *WebRequest) (interface{}, error) {
	logger.Infof("filament_hub/filament_info requested with params: %v", web_request.Params)

	index := web_request.Int("index", -1)
	info := filamentpkg.ParseFilamentFromConfig(index, filamentpkg.AmsConfigPath)
	if slot := self.aceSlotStatus(index); slot != nil {
		filamentpkg.MergeSlotStatusIntoFilamentInfo(&info, slot)
	}
	web_request.Send(filamentpkg.BuildFilamentInfoResponse(index, info))
	return nil, nil
}

func (self *WebHooks) _handle_filament_hub_info(web_request *WebRequest) (interface{}, error) {
	logger.Infof("filament_hub/info requested")
	response := map[string]interface{}{
		"infos": []interface{}{
			map[string]interface{}{
				"id":                0,
				"slots":             4,
				"SN":                "",
				"date":              "",
				"model":             "Anycubic Color Engine Pro",
				"firmware":          "V1.3.863",
				"structure_version": "0",
			},
		},
	}
	web_request.Send(response)
	return nil, nil
}

func (self *WebHooks) _handle_filament_hub_query_version(web_request *WebRequest) (interface{}, error) {
	logger.Infof("filament_hub/query_version requested with params: %v", web_request.Params)

	response := map[string]interface{}{
		"boot_version": "V1.0.1",
		"id":           0,
		"version":      "V1.3.863",
	}
	web_request.Send(response)
	return nil, nil
}

func (self *WebHooks) _handle_filament_hub_set_filament_info(web_request *WebRequest) (interface{}, error) {
	logger.Infof("filament_hub/set_filament_info requested with params: %v", web_request.Params)

	aceObj := self.printer.Lookup_object("filament_hub", nil)
	if aceObj == nil {
		logger.Error("Could not find hardware filament_hub object")
		return nil, fmt.Errorf("Unregistered endpoint")
	}

	index := web_request.Int("index", -1)
	typ, _ := web_request.Params["type"].(string)

	if colorMap, ok := web_request.Params["color"].(map[string]interface{}); ok {
		var r, g, b float64
		if rFloat, rOk := colorMap["R"].(float64); rOk {
			r = rFloat
		}
		if gFloat, gOk := colorMap["G"].(float64); gOk {
			g = gFloat
		}
		if bFloat, bOk := colorMap["B"].(float64); bOk {
			b = bFloat
		}
		parsedColor := []interface{}{int(r), int(g), int(b)}
		logger.Infof("Set_fil info called! index: %v, typ: %v, color: %v", index, typ, parsedColor)

		type FilamentInfoSetter interface {
			Set_filament_info(index int, typ string, sku string, color []interface{})
		}

		if ace, ok := aceObj.(FilamentInfoSetter); ok {
			ace.Set_filament_info(index, typ, "", parsedColor)
		} else {
			logger.Error("Filament_hub object does not implement Set_filament_info")
		}

		response := map[string]interface{}{}
		web_request.Send(response)
	} else {
		response := map[string]interface{}{}
		web_request.Send(response)
	}
	return nil, nil
}

func (self *WebHooks) _handle_filament_hub_start_drying(web_request *WebRequest) (interface{}, error) {
	logger.Infof("filament_hub/start_drying requested with params: %v", web_request.Params)
	duration, temp := filamentpkg.ParseDryingParams(web_request.Params)
	if gcode, _ := self.printer.Lookup_object("gcode", nil).(*GCodeDispatch); gcode != nil {
		gcode.Run_script(fmt.Sprintf("ACE_START_DRYING TEMP=%d DURATION=%d", temp, duration))
	}

	web_request.Send(map[string]interface{}{})
	return nil, nil
}

func (self *WebHooks) _handle_filament_hub_stop_drying(web_request *WebRequest) (interface{}, error) {
	logger.Infof("filament_hub/stop_drying requested")
	if gcode, _ := self.printer.Lookup_object("gcode", nil).(*GCodeDispatch); gcode != nil {
		gcode.Run_script("ACE_STOP_DRYING")
	}
	web_request.Send(map[string]interface{}{})
	return nil, nil
}

const HINT_TIMEOUT = probepkg.HintTimeout

type PrinterProbe struct {
	*probepkg.PrinterProbeModule
	printer   *Printer
	gcodeMove *gcodepkg.GCodeMoveModule
	mcuProbe  interface{}
}

var (
	_ probepkg.ProbeEventRuntime         = (*PrinterProbe)(nil)
	_ probepkg.ProbeMotionRuntime        = (*PrinterProbe)(nil)
	_ probepkg.ProbeCommandContext       = (*PrinterProbe)(nil)
	_ probepkg.ProbePointsAutomaticProbe = (*PrinterProbe)(nil)
)

func NewPrinterProbe(config *ConfigWrapper, mcuProbe interface{}) *PrinterProbe {
	module := probepkg.NewPrinterProbeModule(config, mcuProbe)
	self := &PrinterProbe{
		PrinterProbeModule: module,
		printer:            config.Get_printer(),
		gcodeMove:          MustLookupGCodeMove(config.Get_printer()),
		mcuProbe:           mcuProbe,
	}
	module.SetRuntime(self)
	return self
}

type probeEndstopQuerier interface {
	Query_endstop(float64) int
}

func (self *PrinterProbe) SendEvent(event string) {
	_, _ = self.printer.Send_event(event, nil)
}

func (self *PrinterProbe) MatchesHomingMoveEndstop(endstop interface{}) bool {
	return self.mcuProbe == endstop
}

func (self *PrinterProbe) MatchesHomeRailEndstop(endstop interface{}) bool {
	return self.mcuProbe == endstop
}

func (self *PrinterProbe) PrepareProbe(move interface{}) {
	if probeEndstop, ok := self.mcuProbe.(interface{ Probe_prepare(interface{}) }); ok {
		probeEndstop.Probe_prepare(move)
	}
}

func (self *PrinterProbe) FinishProbe(move interface{}) {
	if probeEndstop, ok := self.mcuProbe.(interface{ Probe_finish(interface{}) }); ok {
		probeEndstop.Probe_finish(move)
	}
}

func (self *PrinterProbe) BeginMCUMultiProbe() {
	if probe, ok := self.mcuProbe.(interface{ Multi_probe_begin() }); ok {
		probe.Multi_probe_begin()
	}
}

func (self *PrinterProbe) EndMCUMultiProbe() {
	if probe, ok := self.mcuProbe.(interface{ Multi_probe_end() }); ok {
		probe.Multi_probe_end()
	}
}

func (self *PrinterProbe) HomedAxes() string {
	toolhead := MustLookupToolhead(self.printer)
	homedAxes, _ := toolhead.Get_status(self.printer.Get_reactor().Monotonic())["homed_axes"].(string)
	return homedAxes
}

func (self *PrinterProbe) ToolheadPosition() []float64 {
	return MustLookupToolhead(self.printer).Get_position()
}

func (self *PrinterProbe) ProbingMove(target []float64, speed float64) []float64 {
	phoming := self.printer.Lookup_object("homing", object.Sentinel{}).(*PrinterHoming)
	return phoming.Probing_move(self.mcuProbe, target, speed)
}

func (self *PrinterProbe) RespondInfo(msg string, log bool) {
	MustLookupGcode(self.printer).Respond_info(msg, log)
}

func (self *PrinterProbe) LastMoveTime() float64 {
	return MustLookupToolhead(self.printer).Get_last_move_time()
}

func (self *PrinterProbe) QueryEndstop(printTime float64) int {
	querier, ok := self.mcuProbe.(probeEndstopQuerier)
	if !ok {
		panic(fmt.Sprintf("probe endstop has unexpected type %T", self.mcuProbe))
	}
	return querier.Query_endstop(printTime)
}

func (self *PrinterProbe) EnsureNoManualProbe() {
	self.printer.Lookup_object("manual_probe", object.Sentinel{}).(*ManualProbe).EnsureNoManualProbe()
}

func (self *PrinterProbe) StartManualProbe(command probepkg.ProbeCommand, finalize func([]float64)) {
	self.printer.Lookup_object("manual_probe", object.Sentinel{}).(*ManualProbe).StartManualProbe(command, finalize)
}

func (self *PrinterProbe) SetConfig(section string, option string, value string) {
	configfile := self.printer.Lookup_object("configfile", object.Sentinel{}).(*PrinterConfig)
	configfile.Set(section, option, value)
}

func (self *PrinterProbe) HomingOriginZ() float64 {
	return self.gcodeMove.Get_status(0)["homing_origin"].([]float64)[2]
}

type ProbeEndstopWrapper struct {
	printer         *Printer
	activateGCode   printerpkg.Template
	deactivateGCode printerpkg.Template
	mcuEndstop      *mcupkg.LegacyEndstop
	core            *probepkg.EndstopWrapper
}

func (self *ProbeEndstopWrapper) ToolheadPosition() []float64 {
	return self.printer.Lookup_object("toolhead", object.Sentinel{}).(*Toolhead).Get_position()
}

func (self *ProbeEndstopWrapper) RunActivateGCode() {
	self.activateGCode.RunGcodeFromCommand(nil)
}

func (self *ProbeEndstopWrapper) RunDeactivateGCode() {
	self.deactivateGCode.RunGcodeFromCommand(nil)
}

func (self *ProbeEndstopWrapper) KinematicsSteppers() []interface{} {
	toolhead := self.printer.Lookup_object("toolhead", object.Sentinel{}).(*Toolhead)
	return toolhead.Get_kinematics().(IKinematics).Get_steppers()
}

func (self *ProbeEndstopWrapper) AddStepper(stepper interface{}) {
	self.mcuEndstop.Add_stepper(stepper)
}

func (self *ProbeEndstopWrapper) Get_mcu() interface{} {
	return self.mcuEndstop.MCUKey()
}

func (self *ProbeEndstopWrapper) Get_steppers() []interface{} {
	return self.mcuEndstop.Get_steppers()
}

func (self *ProbeEndstopWrapper) Home_start(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64) interface{} {
	return self.mcuEndstop.Home_start(printTime, sampleTime, sampleCount, restTime, triggered)
}

func (self *ProbeEndstopWrapper) Home_wait(moveEndPrintTime float64) float64 {
	return self.mcuEndstop.Home_wait(moveEndPrintTime)
}

func (self *ProbeEndstopWrapper) Query_endstop(printTime float64) int {
	return self.mcuEndstop.Query_endstop(printTime)
}

func (self *ProbeEndstopWrapper) StepperIsActiveAxis(stepper interface{}, axis rune) bool {
	type activeAxisStepper interface {
		Is_active_axis(int8) int32
	}
	mcuStepper, ok := stepper.(activeAxisStepper)
	if !ok {
		panic(fmt.Errorf("probe endstop identify stepper has unexpected type %T", stepper))
	}
	return mcuStepper.Is_active_axis(int8(axis)) != 0
}

func NewProbeEndstopWrapper(config *ConfigWrapper) *ProbeEndstopWrapper {
	self := &ProbeEndstopWrapper{}
	self.printer = config.Get_printer()
	positionEndstop := config.Getfloat("z_offset", 0, 0, 0, 0, 0, true)
	stowOnEachSample := config.Getboolean("deactivate_on_each_sample", nil, true)
	self.core = probepkg.NewEndstopWrapper(positionEndstop, stowOnEachSample)
	self.activateGCode = config.LoadTemplate("gcode_macro_1", "activate_gcode", "")
	self.deactivateGCode = config.LoadTemplate("gcode_macro_1", "deactivate_gcode", "")
	ppins := self.printer.Lookup_object("pins", object.Sentinel{})
	pin := config.Get("pin", object.Sentinel{}, true)
	pinParams := ppins.(*printerpkg.PrinterPins).Lookup_pin(pin.(string), true, true, nil)
	mcu := pinParams["chip"]
	self.mcuEndstop = mcu.(*MCU).Setup_pin("endstop", pinParams).(*mcupkg.LegacyEndstop)
	self.printer.Register_event_handler("project:mcu_identify", self.Handle_mcu_identify)
	return self
}

func (self *ProbeEndstopWrapper) Handle_mcu_identify([]interface{}) error {
	self.core.HandleMCUIdentify(self)
	return nil
}

func (self *ProbeEndstopWrapper) Raise_probe() {
	self.core.RaiseProbe(self)
}

func (self *ProbeEndstopWrapper) Lower_probe() {
	self.core.LowerProbe(self)
}

func (self *ProbeEndstopWrapper) Multi_probe_begin() {
	self.core.BeginMultiProbe()
}

func (self *ProbeEndstopWrapper) Multi_probe_end() {
	self.core.HandleMultiProbeEnd(self)
}

func (self *ProbeEndstopWrapper) Probe_prepare(hmove interface{}) {
	self.core.HandleProbePrepare(self)
}

func (self *ProbeEndstopWrapper) Probe_finish(hmove interface{}) {
	self.core.HandleProbeFinish(self)
}

func (self *ProbeEndstopWrapper) Get_position_endstop() float64 {
	return self.core.GetPositionEndstop()
}

type ManualProbe struct {
	printer            *Printer
	gcode              *GCodeDispatch
	gcode_move         *gcodepkg.GCodeMoveModule
	toolhead           *Toolhead
	z_position_endstop float64
	status             map[string]interface{}
	a_position_endstop float64
	b_position_endstop float64
	c_position_endstop float64
	lastToolheadPos    []float64
	lastKinematicsPos  []float64
}

func newManualProbeStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_active":        false,
		"z_position":       nil,
		"z_position_lower": nil,
		"z_position_upper": nil,
		"isActive":         false,
		"zPosition":        nil,
		"zPositionLower":   nil,
		"zPositionUpper":   nil,
	}
}

func (self *ManualProbe) currentGCodeMove() *gcodepkg.GCodeMoveModule {
	if self.gcode_move == nil {
		self.gcode_move = MustLookupGCodeMove(self.printer)
	}
	return self.gcode_move
}

func (self *ManualProbe) prepareSession() {
	self.currentGCodeMove()
	self.currentToolhead()
}

func (self *ManualProbe) StartManualProbe(command probepkg.ProbeCommand, finalize func([]float64)) {
	self.EnsureNoManualProbe()
	self.prepareSession()
	speed := command.Get_float("SPEED", 5., nil, nil, nil, nil)
	probepkg.NewManualProbeSession(self, self, speed, finalize)
}

func (self *ManualProbe) EnsureNoManualProbe() {
	err := self.gcode.Register_command("ACCEPT", "dummy", false, "")
	if err != nil {
		panic("Already in a manual Z probe. Use ABORT to abort it.")
	}
	self.gcode.Register_command("ACCEPT", nil, false, "")
}

func (self *ManualProbe) ZPositionEndstop() float64 {
	return self.z_position_endstop
}

func (self *ManualProbe) DeltaPositionEndstops() (float64, float64, float64) {
	return self.a_position_endstop, self.b_position_endstop, self.c_position_endstop
}

func (self *ManualProbe) HomingOriginZ() float64 {
	return self.currentGCodeMove().Get_status(0)["homing_origin"].([]float64)[2]
}

func (self *ManualProbe) SetConfig(section string, option string, value string) {
	configfile := self.printer.Lookup_object("configfile", object.Sentinel{}).(*PrinterConfig)
	configfile.Set(section, option, value)
}

func (self *ManualProbe) RespondInfo(msg string, log bool) {
	self.gcode.Respond_info(msg, log)
}

func (self *ManualProbe) RegisterCommand(cmd string, handler func(probepkg.ManualProbeCommand) error, desc string) {
	self.gcode.Register_command(cmd, func(arg interface{}) error {
		return handler(arg.(*GCodeCommand))
	}, false, desc)
}

func (self *ManualProbe) ClearCommand(cmd string) {
	self.gcode.Register_command(cmd, nil, false, "")
}

func (self *ManualProbe) ResetStatus() {
	self.status = newManualProbeStatus()
}

func (self *ManualProbe) SetStatus(status map[string]interface{}) {
	self.status = status
}

func (self *ManualProbe) currentToolhead() *Toolhead {
	if self.toolhead == nil {
		self.toolhead = MustLookupToolhead(self.printer)
	}
	return self.toolhead
}

func (self *ManualProbe) ToolheadPosition() []float64 {
	return self.currentToolhead().Get_position()
}

func (self *ManualProbe) KinematicsPosition() []float64 {
	toolhead := self.currentToolhead()
	toolheadPos := toolhead.Get_position()
	if reflect.DeepEqual(toolheadPos, self.lastToolheadPos) {
		return append([]float64{}, self.lastKinematicsPos...)
	}
	toolhead.Flush_step_generation()
	kin := toolhead.Get_kinematics().(IKinematics)
	type commandedStepper interface {
		Get_name(bool) string
		Get_commanded_position() float64
	}
	kinPos := map[string]float64{}
	for _, stepper := range kin.Get_steppers() {
		namedStepper, ok := stepper.(commandedStepper)
		if !ok {
			continue
		}
		kinPos[namedStepper.Get_name(false)] = namedStepper.Get_commanded_position()
	}
	position := kin.Calc_position(kinPos)
	self.lastToolheadPos = append([]float64{}, toolheadPos...)
	self.lastKinematicsPos = append([]float64{}, position...)
	return append([]float64{}, position...)
}

func (self *ManualProbe) ManualMove(coord []interface{}, speed float64) {
	self.currentToolhead().Manual_move(coord, speed)
}

func NewManualProbe(config *ConfigWrapper) *ManualProbe {
	self := ManualProbe{}
	self.printer = config.Get_printer()
	self.gcode = MustLookupGcode(self.printer)
	self.gcode.Register_command("MANUAL_PROBE", self.cmd_MANUAL_PROBE, false, self.cmd_MANUAL_PROBE_help())
	kinematics := config.Getsection("printer").Get("kinematics", object.Sentinel{}, true).(string)
	if kinematics == "delta" {
		aTowerConfig := config.Getsection("stepper_a")
		self.a_position_endstop = aTowerConfig.Getfloat("position_endstop", object.Sentinel{}, 0., 0., 0., 0., false)

		bTowerConfig := config.Getsection("stepper_b")
		self.b_position_endstop = bTowerConfig.Getfloat("position_endstop", object.Sentinel{}, 0., 0., 0., 0., false)

		cTowerConfig := config.Getsection("stepper_c")
		self.c_position_endstop = cTowerConfig.Getfloat("position_endstop", object.Sentinel{}, 0., 0., 0., 0., false)
	}
	if config.Has_section("stepper_z") {
		zconfig := config.Getsection("stepper_z")
		self.z_position_endstop = zconfig.Getfloat("position_endstop", 0., 0, 0, 0, 0, false)
	}
	if self.z_position_endstop != 0 {
		self.gcode.Register_command("Z_ENDSTOP_CALIBRATE", self.cmd_Z_ENDSTOP_CALIBRATE, false, self.cmd_Z_ENDSTOP_CALIBRATE_help())
		self.gcode.Register_command("Z_OFFSET_APPLY_ENDSTOP", self.cmd_Z_OFFSET_APPLY_ENDSTOP, false, self.cmd_Z_OFFSET_APPLY_ENDSTOP_help())
	}
	if kinematics == "delta" {
		self.gcode.Register_command("Z_OFFSET_APPLY_ENDSTOP", self.cmd_Z_OFFSET_APPLY_DELTA_ENDSTOPS, false, self.cmd_Z_OFFSET_APPLY_ENDSTOP_help())
	}
	self.ResetStatus()

	return &self
}

func (self *ManualProbe) Get_status(eventTime int64) map[string]interface{} {
	return sys.DeepCopyMap(self.status)
}

func (self *ManualProbe) cmd_MANUAL_PROBE_help() string {
	return "Start manual probe helper script"
}

func (self *ManualProbe) cmd_MANUAL_PROBE(argv interface{}) error {
	return probepkg.HandleManualProbeCommand(self, argv.(*GCodeCommand))
}

func (self *ManualProbe) cmd_Z_ENDSTOP_CALIBRATE_help() string {
	return "Calibrate a Z endstop"
}

func (self *ManualProbe) cmd_Z_ENDSTOP_CALIBRATE(argv interface{}) error {
	return probepkg.HandleZEndstopCalibrateCommand(self, argv.(*GCodeCommand))
}

func (self *ManualProbe) cmd_Z_OFFSET_APPLY_ENDSTOP(argv interface{}) error {
	return probepkg.HandleZOffsetApplyEndstopCommand(self)
}

func (self *ManualProbe) cmd_Z_OFFSET_APPLY_DELTA_ENDSTOPS(argv interface{}) error {
	return probepkg.HandleZOffsetApplyDeltaEndstopsCommand(self)
}

func (self *ManualProbe) cmd_Z_OFFSET_APPLY_ENDSTOP_help() string {
	return "Adjust the z endstop_position"
}

func Load_config_ManualProbe(config *ConfigWrapper) interface{} {
	return NewManualProbe(config)
}

func Load_config_probe(config *ConfigWrapper) interface{} {
	return NewPrinterProbe(config, NewProbeEndstopWrapper(config))
}

type ExtruderStepper struct {
	Printer                      *Printer
	Name                         string
	Pressure_advance             interface{}
	Pressure_advance_smooth_time float64
	Config_pa                    float64
	Config_smooth_time           float64
	Stepper                      *mcupkg.LegacyStepper
	Sk_extruder                  interface{}
}

func NewExtruderStepper(config *ConfigWrapper) *ExtruderStepper {
	self := &ExtruderStepper{}
	self.Printer = config.Get_printer()
	name_arr := strings.Split(config.Get_name(), " ")
	self.Name = name_arr[len(name_arr)-1]
	self.Pressure_advance, self.Pressure_advance_smooth_time = 0., 0.
	self.Config_pa = config.Getfloat("pressure_advance", 0., 0., 0, 0, 0, true)
	self.Config_smooth_time = config.Getfloat(
		"pressure_advance_smooth_time", 0.040, 0., .200, .0, .0, true)
	printer := config.Get_printer()
	self.Stepper = mcupkg.LoadLegacyPrinterStepper(config, false, printer, func(moduleName string) interface{} {
		return printer.Load_object(config, moduleName, object.Sentinel{})
	}, func(module interface{}, stepper *mcupkg.LegacyStepper) {
		module.(*mcupkg.PrinterStepperEnableModule).Register_stepper(config, stepper)
	}, func(module interface{}, stepper *mcupkg.LegacyStepper) {
		module.(*motionpkg.ForceMoveModule).RegisterStepper(stepper)
	})
	self.Sk_extruder = chelper.Extruder_stepper_alloc()
	self.Stepper.Set_stepper_kinematics(self.Sk_extruder)
	self.Printer.Register_event_handler("project:connect",
		self.Handle_connect)
	gcode := MustLookupGcode(self.Printer)
	if self.Name == "extruder" {
		gcode.Register_mux_command("SET_PRESSURE_ADVANCE", "EXTRUDER", "", self.Cmd_default_SET_PRESSURE_ADVANCE, cmd_SET_PRESSURE_ADVANCE_help)
	}

	gcode.Register_mux_command("SET_PRESSURE_ADVANCE", "EXTRUDER", self.Name, self.Cmd_SET_PRESSURE_ADVANCE, cmd_SET_PRESSURE_ADVANCE_help)
	gcode.Register_mux_command("SET_EXTRUDER_ROTATION_DISTANCE", "EXTRUDER", self.Name, self.Cmd_SET_E_ROTATION_DISTANCE, cmd_SET_E_ROTATION_DISTANCE_help)
	gcode.Register_mux_command("SYNC_EXTRUDER_MOTION", "EXTRUDER", self.Name, self.Cmd_SYNC_EXTRUDER_MOTION, cmd_SYNC_EXTRUDER_MOTION_help)
	gcode.Register_mux_command("SET_EXTRUDER_STEP_DISTANCE", "EXTRUDER", self.Name, self.Cmd_SET_E_STEP_DISTANCE, cmd_SET_E_STEP_DISTANCE_help)
	gcode.Register_mux_command("SYNC_STEPPER_TO_EXTRUDER", "STEPPER", self.Name, self.Cmd_SYNC_STEPPER_TO_EXTRUDER, cmd_SYNC_STEPPER_TO_EXTRUDER_help)
	return self
}

func (self *ExtruderStepper) _ExtruderStepper() {
	chelper.Free(self.Sk_extruder)
}

func (self *ExtruderStepper) Handle_connect([]interface{}) error {
	toolhead := MustLookupToolhead(self.Printer)
	toolhead.Register_step_generator(self.Stepper.Generate_steps)
	self.Set_pressure_advance(self.Config_pa, self.Config_smooth_time)
	return nil
}

func (self *ExtruderStepper) Get_status(eventtime float64) map[string]float64 {
	return map[string]float64{
		"pressure_advance": cast.ToFloat64(self.Pressure_advance),
		"smooth_time":      self.Pressure_advance_smooth_time,
	}
}

func (self *ExtruderStepper) Find_past_position(print_time float64) float64 {
	mcuPos := self.Stepper.Get_past_mcu_position(print_time)
	return self.Stepper.Mcu_to_commanded_position(mcuPos)
}

func (self *ExtruderStepper) Sync_to_extruder(extruder_name string) {
	toolhead := MustLookupToolhead(self.Printer)
	toolhead.Flush_step_generation()
	if extruder_name == "" {
		self.Stepper.Set_trapq(nil)
		return
	}
	rawExtruder := self.Printer.Lookup_object(extruder_name, nil)
	state, err := motionpkg.ResolveLegacyExtruderSyncState(rawExtruder, extruder_name)
	if err != nil {
		panic(err)
	}
	self.Stepper.Set_position([]float64{state.Position, 0., 0.})
	self.Stepper.Set_trapq(state.Trapq)
}

func (self *ExtruderStepper) Set_pressure_advance(pressureAdvance interface{}, smoothTime float64) {
	plan := motionpkg.BuildPressureAdvanceScanPlan(self.Pressure_advance, self.Pressure_advance_smooth_time, pressureAdvance, smoothTime)
	toolhead := MustLookupToolhead(self.Printer)
	toolhead.Note_step_generation_scan_time(plan.NextDelay, plan.PreviousDelay)
	espa := chelper.Extruder_set_pressure_advance
	espa(self.Sk_extruder, cast.ToFloat64(pressureAdvance), plan.AppliedSmoothTime)
	self.Pressure_advance = pressureAdvance
	self.Pressure_advance_smooth_time = smoothTime
}

const cmd_SET_PRESSURE_ADVANCE_help = "Set pressure advance parameters"

func (self *ExtruderStepper) Cmd_default_SET_PRESSURE_ADVANCE(argv interface{}) error {
	toolhead := MustLookupToolhead(self.Printer)
	extruder := toolhead.Get_extruder()
	activeExtruder, ok := extruder.(interface{ Get_extruder_stepper() *ExtruderStepper })
	if !ok || activeExtruder.Get_extruder_stepper() == nil {
		panic("Active extruder does not have a stepper")
	}
	strapq := activeExtruder.Get_extruder_stepper().Stepper.Get_trapq()
	if strapq != extruder.Get_trapq() {
		panic("Unable to infer active extruder stepper")
	}
	activeExtruder.Get_extruder_stepper().Cmd_SET_PRESSURE_ADVANCE(argv)
	return nil
}

func (self *ExtruderStepper) Cmd_SET_PRESSURE_ADVANCE(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	zero := 0.
	maxval := .200
	pressure_advance := 0.0
	pressure_advance = gcmd.Get_float("ADVANCE", self.Pressure_advance, &zero, nil, nil, nil)
	smooth_time := gcmd.Get_float("SMOOTH_TIME", self.Pressure_advance_smooth_time, &zero, &maxval, nil, nil)
	self.Set_pressure_advance(pressure_advance, smooth_time)
	msg := fmt.Sprintf("pressure_advance: %.6f\n pressure_advance_smooth_time: %.6f", pressure_advance, smooth_time)
	self.Printer.Set_rollover_info(self.Name, fmt.Sprintf("%s: %s", self.Name, msg), false)
	gcmd.Respond_info(msg, true)
	return nil
}

const cmd_SET_E_ROTATION_DISTANCE_help = "Set extruder rotation distance"

func (self *ExtruderStepper) Cmd_SET_E_ROTATION_DISTANCE(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	rotationDist := 0.0
	if gcmd.Has("DISTANCE") {
		rotationDist = gcmd.Get_float("DISTANCE", nil, nil, nil, nil, nil)
		if rotationDist == 0.0 {
			panic("Rotation distance can not be zero")
		}
		_, origInvertDir := self.Stepper.Get_dir_inverted()
		update := motionpkg.ResolveExtruderRotationDistanceUpdate(rotationDist, origInvertDir)
		toolhead := MustLookupToolhead(self.Printer)
		toolhead.Flush_step_generation()
		self.Stepper.Set_rotation_distance(update.RotationDistance)
		self.Stepper.Set_dir_inverted(update.NextInvertDir)
		rotationDist = update.RotationDistance
	} else {
		rotationDist, _ = self.Stepper.Get_rotation_distance()
	}
	invertDir, origInvertDir := self.Stepper.Get_dir_inverted()
	rotationDist = motionpkg.DisplayExtruderRotationDistance(rotationDist, invertDir, origInvertDir)
	gcmd.Respond_info(fmt.Sprintf("Extruder '%s' rotation distance set to %0.6f", self.Name, rotationDist), true)
	return nil
}

const cmd_SYNC_EXTRUDER_MOTION_help = "Set extruder stepper motion queue"

func (self *ExtruderStepper) Cmd_SYNC_EXTRUDER_MOTION(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	ename := gcmd.Get("MOTION_QUEUE", object.Sentinel{}, "", nil, nil, nil, nil)
	self.Sync_to_extruder(ename)
	gcmd.Respond_info(fmt.Sprintf("Extruder stepper now syncing with '%s'", ename), true)
	return nil
}

const cmd_SET_E_STEP_DISTANCE_help = "Set extruder step distance"

func (self *ExtruderStepper) Cmd_SET_E_STEP_DISTANCE(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	zero := 0.
	step_dist := 0.0
	if gcmd.Has("DISTANCE") {
		step_dist = gcmd.Get_float("DISTANCE", object.Sentinel{}, nil, nil, &zero, nil)
		toolhead := MustLookupToolhead(self.Printer)
		toolhead.Flush_step_generation()
		_, steps_per_rotation := self.Stepper.Get_rotation_distance()
		self.Stepper.Set_rotation_distance(step_dist * float64(steps_per_rotation))
	} else {
		step_dist = self.Stepper.Get_step_dist()
	}
	gcmd.Respond_info(fmt.Sprintf("Extruder '%s' step distance set to %0.6f",
		self.Name, step_dist), true)
	return nil
}

const cmd_SYNC_STEPPER_TO_EXTRUDER_help = "Set extruder stepper"

func (self *ExtruderStepper) Cmd_SYNC_STEPPER_TO_EXTRUDER(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	ename := gcmd.Get("EXTRUDER", object.Sentinel{}, "", nil, nil, nil, nil)
	self.Sync_to_extruder(ename)
	gcmd.Respond_info(fmt.Sprintf("Extruder stepper now syncing with '%s'", ename), true)
	return nil
}

type PrinterExtruder struct {
	*motionpkg.LegacyExtruderRuntime
	Printer          *Printer
	Extruder_stepper *ExtruderStepper
}

func NewPrinterExtruder(config *ConfigWrapper, extruder_num int) *PrinterExtruder {
	self := &PrinterExtruder{LegacyExtruderRuntime: &motionpkg.LegacyExtruderRuntime{}}
	self.Printer = config.Get_printer()
	self.Name = config.Get_name()
	self.Last_position = 0.
	shared_heater := config.Get("shared_heater", value.None, true)
	pheaters := self.Printer.Load_object(config, "heaters", object.Sentinel{})
	gcode_id := fmt.Sprintf("T%d", extruder_num)
	if shared_heater == nil {
		self.Heater = pheaters.(*heaterpkg.PrinterHeaters).Setup_heater(config, gcode_id)
	} else {
		config.Deprecate("shared_heater", "")
		self.Heater = pheaters.(*heaterpkg.PrinterHeaters).Lookup_heater(shared_heater.(string))
	}
	self.Nozzle_diameter = config.Getfloat("nozzle_diameter", 0, 0, 0, 0., 0, true)
	filament_diameter := config.Getfloat("filament_diameter", 0, self.Nozzle_diameter, 0, 0., 0, true)
	self.Filament_area = math.Pi * math.Pow(filament_diameter*.5, 2)
	def_max_cross_section := 4. * self.Nozzle_diameter * self.Nozzle_diameter
	def_max_extrude_ratio := def_max_cross_section / self.Filament_area
	max_cross_section := config.Getfloat("max_extrude_cross_section", def_max_cross_section, 0, 0, 0., 0, true)
	self.Max_extrude_ratio = max_cross_section / self.Filament_area
	toolhead := MustLookupToolhead(self.Printer)
	max_velocity, max_accel := toolhead.Get_max_velocity()
	self.Max_e_velocity = config.Getfloat("max_extrude_only_velocity", max_velocity*def_max_extrude_ratio, 0, 0, 0., 0, true)
	self.Max_e_accel = config.Getfloat("max_extrude_only_accel", max_accel*def_max_extrude_ratio, 0, 0, 0., 0, true)
	self.Max_e_dist = config.Getfloat("max_extrude_only_distance", 50., 0, 0, 0., 0, true)
	self.Instant_corner_v = config.Getfloat("instantaneous_corner_velocity", 1., 0, 0, 0., 0, true)
	self.Trapq = chelper.Trapq_alloc()
	self.Trapq_append = chelper.Trapq_append
	self.Trapq_finalize_moves = chelper.Trapq_finalize_moves
	self.Extruder_stepper = nil
	if config.Get("step_pin", value.None, true) != nil ||
		config.Get("dir_pin", value.None, true) != nil ||
		config.Get("rotation_distance", value.None, true) != nil {

		self.Extruder_stepper = NewExtruderStepper(config)
		self.Extruder_stepper.Stepper.Set_trapq(self.Trapq)
	}
	self.Can_extrude = func() bool {
		return self.Heater.(*heaterpkg.Heater).Can_extrude
	}
	self.Heater_status = func(eventtime float64) map[string]float64 {
		return self.Heater.(*heaterpkg.Heater).Get_status(eventtime)
	}
	self.Heater_stats = func(eventtime float64) (bool, string) {
		return self.Heater.(*heaterpkg.Heater).Stats(eventtime)
	}
	self.Stepper_status = func(eventtime float64) map[string]float64 {
		if self.Extruder_stepper == nil {
			return nil
		}
		return self.Extruder_stepper.Get_status(eventtime)
	}
	self.Find_stepper_past_position = func(printTime float64) float64 {
		if self.Extruder_stepper == nil {
			return 0.
		}
		return self.Extruder_stepper.Find_past_position(printTime)
	}
	gcode := MustLookupGcode(self.Printer)
	if self.Name == "extruder" {
		toolhead.Set_extruder(self, 0.)
		gcode.Register_command("M104", self.Cmd_M104, false, "")
		gcode.Register_command("M109", self.Cmd_M109, false, "")
	}
	gcode.Register_mux_command("ACTIVATE_EXTRUDER", "EXTRUDER", self.Name,
		self.Cmd_ACTIVATE_EXTRUDER,
		cmd_ACTIVATE_EXTRUDER_help)
	return self
}

func (self *PrinterExtruder) _PrinterExtruder() {
	chelper.Trapq_free(self.Trapq)
}

func (self *PrinterExtruder) temperatureRuntime() motionpkg.LegacyExtruderTemperatureRuntime {
	return motionpkg.LegacyExtruderTemperatureRuntimeFuncs{
		ActiveExtruderFunc: func() motionpkg.Extruder {
			return MustLookupToolhead(self.Printer).Get_extruder()
		},
		LookupExtruderFunc: func(section string) motionpkg.Extruder {
			extruder := self.Printer.Lookup_object(section, nil)
			if extruder == nil {
				return nil
			}
			typed, ok := extruder.(motionpkg.Extruder)
			if !ok {
				panic(fmt.Sprintf("%s is not a valid extruder", section))
			}
			return typed
		},
		SetTemperatureFunc: func(extruder motionpkg.Extruder, temp float64, wait bool) error {
			pheaters := self.Printer.Lookup_object("heaters", object.Sentinel{})
			return pheaters.(*heaterpkg.PrinterHeaters).Set_temperature(extruder.Get_heater().(*heaterpkg.Heater), temp, wait)
		},
	}
}

func (self *PrinterExtruder) Cmd_M104(gcmd interface{}) error {
	command := gcmd.(*GCodeCommand)
	temp := command.Get_float("S", 0., nil, nil, nil, nil)
	zero := 0
	index := command.Get_int("T", nil, &zero, nil)
	return motionpkg.HandleLegacyExtruderTemperatureCommand(self.temperatureRuntime(), temp, index, false)
}

func (self *PrinterExtruder) Cmd_M109(gcmd interface{}) error {
	command := gcmd.(*GCodeCommand)
	temp := command.Get_float("S", 0., nil, nil, nil, nil)
	zero := 0
	index := command.Get_int("T", nil, &zero, nil)
	return motionpkg.HandleLegacyExtruderTemperatureCommand(self.temperatureRuntime(), temp, index, true)
}

const cmd_ACTIVATE_EXTRUDER_help = "Change the active extruder"

func (self *PrinterExtruder) Cmd_ACTIVATE_EXTRUDER(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	toolhead := MustLookupToolhead(self.Printer)
	if toolhead.Get_extruder() == self {
		gcmd.Respond_info(fmt.Sprintf("Extruder %s already active", self.Name), true)
		return nil
	}
	gcmd.Respond_info(fmt.Sprintf("Activating extruder %s", self.Name), true)
	toolhead.Flush_step_generation()
	toolhead.Set_extruder(self, self.Last_position)
	self.Printer.Send_event("extruder:activate_extruder", nil)
	return nil
}

func (self *PrinterExtruder) Get_extruder_stepper() *ExtruderStepper {
	return self.Extruder_stepper
}

type DummyExtruder struct {
	*motionpkg.DummyExtruder
}

func NewDummyExtruder() *DummyExtruder {
	return &DummyExtruder{DummyExtruder: motionpkg.NewDummyExtruder()}
}

func (self *DummyExtruder) Get_extruder_stepper() *ExtruderStepper {
	panic("Extruder not configured")
}

func Add_printer_objects_extruder(config *ConfigWrapper) {
	printer := config.Get_printer()
	for i := 0; i < 99; i++ {
		section := "extruder"
		if i > 0 {
			section = fmt.Sprintf("extruder%d", i)
		}
		if !config.Has_section(section) {
			break
		}
		pe := NewPrinterExtruder(config.Getsection(section), i)
		printer.Add_object(section, pe)
	}
}

type erro struct {
	err string
}

type MCU struct {
	error                string
	_printer             *Printer
	_clocksync           serialpkg.ClockSyncAble
	_reactor             IReactor
	_name                string
	Serial               *serialpkg.SerialReader
	_baud                int
	_canbus_iface        interface{}
	_serialport          string
	_restart_method      string
	_reset_cmd           *serialpkg.CommandWrapper
	_is_mcu_bridge       bool
	_emergency_stop_cmd  *serialpkg.CommandWrapper
	_runtime             *mcupkg.LifecycleState
	Oid_count            int
	_config_session      *mcupkg.ConfigSession
	_mcu_freq            float64
	_ffi_lib             interface{}
	_max_stepper_error   float64
	_reserved_move_slots int64
	_stepqueues          []interface{}
	_steppersync         interface{}
	_status              *mcupkg.MCUStatusTracker
	_config_reset_cmd    *serialpkg.CommandWrapper
	_flush_callbacks     []func(float64, int64)
}

func NewMCU(config *ConfigWrapper, clocksync serialpkg.ClockSyncAble) *MCU {
	self := MCU{}
	self._printer = config.Get_printer()
	printer := config.Get_printer()
	self._clocksync = clocksync
	self._reactor = printer.Get_reactor()
	self._name = config.Get_name()
	self._name = strings.TrimPrefix(self._name, "mcu ")
	wp := fmt.Sprintf("mcu '%s': ", self._name)
	self.Serial = serialpkg.NewSerialReader(serialpkg.NewReactorAdapter(self._reactor), wp)
	self._baud = 0
	self._canbus_iface = nil
	canbus_uuid := config.Get("canbus_uuid", value.None, true)
	if canbus_uuid != nil && canbus_uuid.(string) != "" {
		self._serialport = canbus_uuid.(string)
		self._canbus_iface = config.Get("canbus_interface", "can0", true)
	} else {
		self._serialport = config.Get("serial", object.Sentinel{}, true).(string)
		if (strings.HasPrefix(self._serialport, "/dev/rpmsg_") ||
			strings.HasPrefix(self._serialport, "/tmp/klipper_host_")) == false {
			self._baud = config.Getint("baud", 250000, 2400, 0, true)
		}
	}
	restart_methods := []string{"", "arduino", "cheetah", "command", "rpi_usb"}
	self._restart_method = "command"
	if self._baud > 0 {
		rmethods := map[interface{}]interface{}{}
		for _, m := range restart_methods {
			rmethods[m] = m
		}
		self._restart_method = config.Getchoice("restart_method",
			rmethods, nil, true).(string)
	}
	self._reset_cmd, self._config_reset_cmd = nil, nil
	self._is_mcu_bridge = false
	self._emergency_stop_cmd = nil
	self._runtime = mcupkg.NewLifecycleState()
	pins := printer.Lookup_object("pins", object.Sentinel{})
	pins.(*printerpkg.PrinterPins).Register_chip(self._name, &self)
	self.Oid_count = 0
	self._config_session = mcupkg.NewConfigSession()
	self._mcu_freq = 0.
	self._ffi_lib = chelper.Get_ffi()
	self._max_stepper_error = config.Getfloat("max_stepper_error", 0.000025,
		0., 0, 0, 0, true)
	self._reserved_move_slots = 0
	self._stepqueues = []interface{}{}
	self._steppersync = nil
	self._status = mcupkg.NewMCUStatusTracker()
	printer.Register_event_handler("project:firmware_restart",
		self._firmware_restart)
	printer.Register_event_handler("project:mcu_identify",
		self._mcu_identify)
	printer.Register_event_handler("project:connect", self._connect)
	printer.Register_event_handler("project:shutdown", self._shutdown)
	printer.Register_event_handler("project:disconnect", self._disconnect)
	printer.Register_event_handler("project:ready", self._ready)
	return &self
}

func (self *MCU) _handle_mcu_stats(params map[string]interface{}) error {
	return self._status.HandleMCUStats(params, self._mcu_freq)
}

func (self *MCU) _handle_shutdown(params map[string]interface{}) error {
	plan := mcupkg.BuildShutdownPlan(self._name, params, self._clocksync.Dump_debug(), self.Serial.Dump_debug())
	if !self._runtime.HandleShutdownPlan(plan) {
		return nil
	}
	log.Println(plan.LogMessage)
	self._printer.invoke_async_shutdown(plan.AsyncMessage)
	gcode := self._printer.Lookup_object("gcode", object.Sentinel{}).(*GCodeDispatch)
	gcode.Respond_info(plan.RespondInfo, true)
	return nil
}

func (self *MCU) _handle_starting(params map[string]interface{}) {
	if message := self._runtime.StartingShutdownMessage(self._name); message != "" {
		self._printer.invoke_async_shutdown(message)
	}
}

func (self *MCU) _check_restart(reason string) {
	decision := mcupkg.BuildRestartCheckDecision(self._printer.Get_start_args()["start_reason"], self._name, reason)
	if decision.Skip {
		return
	}
	logger.Debugf(decision.LogMessage)
	self._printer.Request_exit(decision.ExitReason)
	// Do not panic here. This path runs inside a reactor callback, and the
	// reactor callback wrapper swallows panics via sys.CatchPanic(), which can
	// prevent Runtime.Run() from reaching its normal post-run
	// project:firmware_restart dispatch. Let the caller observe the pending exit
	// and return its own error path instead.
	return
}

func (self *MCU) _connect_file(pace bool) {
	if self._name == "mcu" {
	} else {
	}
	_ = pace
}

func (self *MCU) _send_config(prev_crc *uint32) string {
	self.Serial.Get_msgparser().Get_constant("MCU", nil, reflect.String)
	ppins := self._printer.Lookup_object("pins", object.Sentinel{})
	pin_resolver := ppins.(*printerpkg.PrinterPins).Get_pin_resolver(self._name)
	errorMessage := mcupkg.SendConfigSession(
		self._config_session,
		self.Oid_count,
		func() int { return self.Oid_count },
		pin_resolver.Update_command,
		func(handler func(map[string]interface{}) error) {
			self.Register_response(handler, "starting", nil)
		},
		func(command string) {
			self.Serial.Send(command, 0, 0)
		},
		self._name,
		prev_crc,
		func(params map[string]interface{}) error {
			self._handle_starting(params)
			return nil
		})
	if errorMessage != "" {
		return errorMessage
	}
	return ""
}

func (self *MCU) _send_get_config() *mcupkg.ConfigSnapshot {
	result := mcupkg.QueryConfigSnapshot(mcupkg.ConfigQueryHooks{
		IsFileoutput: self.Is_fileoutput(),
		QueryConfig: func() map[string]interface{} {
			get_config_cmd := self.Lookup_query_command(
				"get_config",
				"config is_config=%c crc=%u is_shutdown=%c move_count=%hu", -1, nil, false)
			configParams := get_config_cmd.Send([]int64{}, 0, 0)
			if configParams == nil {
				return nil
			}
			return configParams.(map[string]interface{})
		},
		IsShutdown:       self._runtime.ShutdownActive(),
		ShutdownMessage:  self._runtime.ShutdownMessage(),
		MCUName:          self._name,
		HasClearShutdown: self.Try_lookup_command("clear_shutdown") != nil,
		SendClearShutdown: func() {
			cmd := self.Try_lookup_command("clear_shutdown")
			if cmd != nil {
				cmd.(*serialpkg.CommandWrapper).Send([]interface{}{}, 0, 0)
			}
		},
		ClearLocalShutdown: self._runtime.ClearShutdown,
		Sleep:              time.Sleep,
	})
	if result.ErrorMessage != "" {
		panic(&erro{result.ErrorMessage})
	}
	return result.Snapshot
}

func (self *MCU) _log_info() string {
	msgparser := self.Serial.Get_msgparser()
	message_count := len(msgparser.Get_messages())
	version, build_versions := msgparser.Get_version_info()
	return mcupkg.BuildMCULogInfo(self._name, message_count, version, build_versions, self.Get_constants())
}

func (self *MCU) _connect([]interface{}) error {
	connectResult := mcupkg.RunConnectRuntime(mcupkg.ConnectRuntimeHooks{
		QuerySnapshot: func() *mcupkg.ConfigSnapshot {
			return self._send_get_config()
		},
		RestartMethod:  self._restart_method,
		StartReason:    self._printer.Get_start_args()["start_reason"].(string),
		MCUName:        self._name,
		SendConfig:     self._send_config,
		TriggerRestart: self._check_restart,
		RestartRequested: func() bool {
			return self._printer.runtime.ExitResult() != ""
		},
		IsFileoutput:      self.Is_fileoutput(),
		ReservedMoveSlots: self._reserved_move_slots,
	})
	if connectResult.RestartPending {
		return nil
	}
	if connectResult.ReturnError != "" {
		return fmt.Errorf("%s", connectResult.ReturnError)
	}
	if connectResult.PanicMessage != "" {
		if connectResult.WrapPanicInMCUError {
			panic(&erro{connectResult.PanicMessage})
		}
		panic(connectResult.PanicMessage)
	}
	move_count := connectResult.MoveCount

	self._steppersync =
		chelper.Steppersync_alloc(self.Serial.Serialqueue, self._stepqueues,
			len(self._stepqueues), int(move_count-self._reserved_move_slots))

	chelper.Steppersync_set_time(self._steppersync, 0., self._mcu_freq)
	configuredInfo := mcupkg.BuildConfiguredMCUInfo(self._name, move_count, self._log_info())
	logger.Debugf(configuredInfo.MoveMessage)
	self._printer.Set_rollover_info(self._name, configuredInfo.RolloverInfo, false)
	return nil
}

func (self *MCU) _mcu_identify(argv []interface{}) error {
	serialPathExists := true
	plan := mcupkg.BuildConnectionPlan(self.Is_fileoutput(), self._restart_method, self._serialport, self._baud, self._canbus_iface != nil, serialPathExists)
	if plan.Mode == mcupkg.ConnectionModeFileoutput {
		self._connect_file(false)
	} else {
		exist, err := file.PathExists(self._serialport)
		serialPathExists = exist
		if err != nil {
			logger.Error(err.Error())
		}
		plan = mcupkg.BuildConnectionPlan(self.Is_fileoutput(), self._restart_method, self._serialport, self._baud, self._canbus_iface != nil, serialPathExists)
		if plan.NeedsPowerEnableReset {
			self._check_restart("enable power")
			if self._printer.runtime.ExitResult() != "" {
				return nil
			}
		}
		mcupkg.ExecuteConnectionPlan(plan, self._serialport, self._baud, mcupkg.ConnectionExecutionHooks{
			ConnectFileoutput: func() {
				self._connect_file(false)
			},
			ConnectCanbus: func() {
			},
			ConnectRemote: self.Serial.Connect_remote,
			ConnectUART: func(serialPort string, baud int, rts bool) {
				self.Serial.Connect_uart(serialPort, baud, rts)
			},
			ConnectPipe: self.Serial.Connect_pipe,
			ConnectClockSync: func() {
				self._clocksync.Connect(self.Serial)
			},
		})
	}
	_ = argv
	logger.Info(self._log_info())
	ppins := self._printer.Lookup_object("pins", object.Sentinel{})
	pin_resolver := ppins.(*printerpkg.PrinterPins).Get_pin_resolver(self._name)
	constants := self.Get_constants()
	for _, reserved := range mcupkg.CollectReservedPins(constants) {
		pin_resolver.Reserve_pin(reserved.Pin, reserved.Owner)
	}
	self._mcu_freq = self.Get_constant_float("CLOCK_FREQ")
	self._status.SetStatsSumsqBase(self.Get_constant_float("STATS_SUMSQ_BASE"))
	self._emergency_stop_cmd, _ = self.Lookup_command("emergency_stop", nil)
	reset_cmd := self.Try_lookup_command("reset")
	if reset_cmd != nil {
		self._reset_cmd = reset_cmd.(*serialpkg.CommandWrapper)
	}
	config_reset_cmd := self.Try_lookup_command("config_reset")
	if config_reset_cmd != nil {
		self._config_reset_cmd = config_reset_cmd.(*serialpkg.CommandWrapper)
	}
	msgparser := self.Serial.Get_msgparser()
	version, build_versions := msgparser.Get_version_info()
	identifyPlan := mcupkg.BuildIdentifyFinalizePlan(self._restart_method, self._reset_cmd != nil, self._config_reset_cmd != nil, msgparser.Get_constant("SERIAL_BAUD", nil, reflect.Float64), msgparser.Get_constant("CANBUS_BRIDGE", 0, reflect.String), version, build_versions, constants)
	self._restart_method = identifyPlan.RestartMethod
	logger.Infof("MCU '%s' restart capabilities: method=%s reset=%t config_reset=%t clocksync_active=%t", self._name, self._restart_method, self._reset_cmd != nil, self._config_reset_cmd != nil, self._clocksync.Is_active())
	if identifyPlan.IsMCUBridge {
		self._is_mcu_bridge = true
		self._printer.Register_event_handler("project:firmware_restart",
			self._firmware_restart_bridge)
	}
	self._status.SetStatusInfo(identifyPlan.StatusInfo)
	self.Register_response(self._handle_shutdown, "shutdown", nil)
	self.Register_response(self._handle_shutdown, "is_shutdown", nil)
	self.Register_response(self._handle_mcu_stats, "stats", nil)
	return nil
}

func (self *MCU) _disconnect(argv []interface{}) error {
	self.Serial.Disconnect()
	chelper.Steppersync_free(self._steppersync)
	self._steppersync = nil
	_ = argv
	return nil
}

func (self *MCU) _ready(argv []interface{}) error {
	check := mcupkg.BuildReadyFrequencyCheck(self.Is_fileoutput(), self._mcu_freq, self._reactor.Monotonic(), self._clocksync.Get_clock)
	if check.Skip {
		return nil
	}
	if check.IsMismatch {
		logger.Errorf("MCU %s configured for %dMhz but running at %dMhz!",
			self._name, check.MCUFreqMHz, check.CalcFreqMHz)
	} else {
		logger.Debugf("MCU %s configured for %dMhz running at %dMhz",
			self._name, check.MCUFreqMHz, check.CalcFreqMHz)
	}
	_ = argv
	return nil
}

func (self *MCU) _shutdown(argv []interface{}) error {
	force := false
	if len(argv) != 0 {
		force = argv[0].(bool)
	}
	decision := mcupkg.BuildEmergencyStopDecision(self._emergency_stop_cmd != nil, self._runtime.ShutdownActive(), force)
	if decision.Skip {
		return nil
	}
	self._emergency_stop_cmd.Send([]int64{}, 0, 0)
	return nil
}

func (self *MCU) _restart_arduino() {
	logger.Debugf("Attempting MCU '%s' reset", self._name)
	self._disconnect([]interface{}{})
	serialpkg.ArduinoReset(self._serialport, serialpkg.NewReactorAdapter(self._reactor))
}

func (self *MCU) _restart_cheetah() {
	logger.Debugf("Attempting MCU '%s' Cheetah-style reset", self._name)
	self._disconnect([]interface{}{})
	serialpkg.CheetahReset(self._serialport, serialpkg.NewReactorAdapter(self._reactor))
}

func (self *MCU) _restart_via_command() {
	plan := mcupkg.BuildCommandResetPlan(self._reset_cmd != nil, self._config_reset_cmd != nil, self._clocksync.Is_active(), self._name)
	if errorMessage := mcupkg.ExecuteCommandReset(plan, mcupkg.CommandResetExecutionHooks{
		DebugLog: func(message string) {
			logger.Debugf("%s", message)
		},
		MarkShutdown: func() {
			self._runtime.MarkShutdown()
		},
		SendEmergencyStop: func(force bool) {
			self._shutdown([]interface{}{force})
		},
		PauseSeconds: func(seconds float64) {
			self._reactor.Pause(self._reactor.Monotonic() + seconds)
		},
		SendConfigReset: func() {
			self._config_reset_cmd.Send([]int64{}, 0, 0)
		},
		SendReset: func() {
			self._reset_cmd.Send([]int64{}, 0, 0)
		},
		Sleep:      time.Sleep,
		Disconnect: func() { self._disconnect([]interface{}{}) },
	}); errorMessage != "" {
		logger.Errorf(errorMessage)
		return
	}
}

func (self *MCU) _restart_rpi_usb() {
	logger.Debugf("Attempting MCU '%s' reset via rpi usb power", self._name)
	self._disconnect([]interface{}{})
	self._reactor.Pause(self._reactor.Monotonic() + 2.)
}

func (self *MCU) _firmware_restart(argv []interface{}) error {
	var force bool
	if argv != nil {
		force = argv[0].(bool)
	} else {
		force = false
	}
	logger.Infof("MCU '%s': project:firmware_restart force=%t method=%s bridge=%t", self._name, force, self._restart_method, self._is_mcu_bridge)
	plan := mcupkg.BuildFirmwareRestartPlan(force, self._is_mcu_bridge, self._restart_method)
	mcupkg.ExecuteFirmwareRestartPlan(plan, mcupkg.FirmwareRestartExecutionHooks{
		RestartRPIUSB:     self._restart_rpi_usb,
		RestartViaCommand: self._restart_via_command,
		RestartCheetah:    self._restart_cheetah,
		RestartArduino:    self._restart_arduino,
	})
	return nil
}

func (self *MCU) _firmware_restart_bridge([]interface{}) error {
	self._firmware_restart([]interface{}{true})
	return nil
}

func (self *MCU) CalibrateClock(printTime float64, eventtime float64) []float64 {
	return self._clocksync.Calibrate_clock(printTime, eventtime)
}

func (self *MCU) ClockSyncActive() bool {
	return self._clocksync.Is_active()
}

func (self *MCU) IsFileoutput() bool {
	return self.Is_fileoutput()
}

func (self *MCU) SetTime(offset float64, freq float64) {
	chelper.Steppersync_set_time(self._steppersync, offset, freq)
}

func (self *MCU) Flush(clock uint64, clearHistoryClock uint64) int {
	return chelper.Steppersync_flush(self._steppersync, clock, clearHistoryClock)
}

func (self *MCU) Flush_moves(print_time float64, clear_history_time float64) {
	var sync mcupkg.StepperSync
	if self._steppersync != nil {
		sync = self
	}
	err := mcupkg.FlushMoves(print_time, clear_history_time, self, sync, self._flush_callbacks)
	if err != nil {
		logger.Error("Internal error in MCU stepcompress:", self._name)
	}
}

func (self *MCU) FlushMoves(flushTime float64, clearHistoryTime float64) {
	self.Flush_moves(flushTime, clearHistoryTime)
}

func (self *MCU) Check_active(print_time float64, eventtime float64) {
	state := mcupkg.MoveQueueTimingState{IsTimeout: self._runtime.TimeoutActive()}
	var sync mcupkg.StepperSync
	if self._steppersync != nil {
		sync = self
	}
	timedOut := state.CheckActive(print_time, eventtime, self, sync)
	self._runtime.SetTimeout(state.IsTimeout)
	timeoutPlan := mcupkg.BuildMoveQueueTimeoutPlan(timedOut, self._name, eventtime)
	if !timeoutPlan.TimedOut {
		return
	}
	logger.Errorf(timeoutPlan.LogMessage)
	self._printer.Invoke_shutdown(timeoutPlan.ShutdownMessage)

	gcode := self._printer.Lookup_object("gcode", object.Sentinel{}).(*GCodeDispatch)
	gcode.Respond_info(timeoutPlan.ShutdownMessage, true)
}

func (self *MCU) Is_fileoutput() bool {
	return self._printer.Get_start_args()["debugoutput"] != nil
}

func (self *MCU) Is_shutdown() bool {
	return self._runtime.ShutdownActive()
}

func (self *MCU) Get_shutdown_clock() int64 {
	return self._runtime.ShutdownClock()
}

func (self *MCU) Get_status(eventtime float64) map[string]interface{} {
	return self._status.GetStatus()
}

func (self *MCU) Stats(eventtime float64) (bool, string) {
	return self._status.Stats(self._name, self.Serial.Stats(eventtime), self._clocksync.Stats(eventtime))
}

func (self *MCU) Get_status_info() map[string]interface{} {
	return self._status.StatusInfo()
}

func (self *MCU) newManagedTrsync(trdispatch interface{}) *mcupkg.ManagedTrsync {
	return mcupkg.NewManagedTrsyncRuntimeForHost(self, mcupkg.ManagedTrsyncRuntimeFactoryHooks{
		RegisterShutdownHandler: func(handler func([]interface{}) error) {
			self.Get_printer().Register_event_handler("project:shutdown", handler)
		},
		CreateDispatch: func(tags mcupkg.TrsyncDispatchTags) interface{} {
			return chelper.Trdispatch_mcu_alloc(trdispatch, self.Serial.Serialqueue,
				tags.CmdQueue, tags.OID, tags.SetTimeoutTag, tags.TriggerTag, tags.StateTag)
		},
		SetupDispatch: func(handle interface{}, plan mcupkg.TrsyncStartPlan) {
			chelper.Trdispatch_mcu_setup(handle, uint64(plan.Clock), uint64(plan.ExpireClock), uint64(plan.ExpireTicks), uint64(plan.MinExtendTicks))
		},
		AsyncComplete: func(completion mcupkg.Completion, result map[string]interface{}) {
			if reactorCompletion, ok := completion.(*ReactorCompletion); ok {
				self.Get_printer().Get_reactor().Async_complete(reactorCompletion, result)
				return
			}
			completion.Complete(result)
		},
	})
}

func (self *MCU) GetStatus(eventtime float64) map[string]interface{} {
	return self.Get_status(eventtime)
}

func (self *MCU) NewTrsyncCommandQueue() interface{} {
	return self.newManagedTrsync(chelper.Trdispatch_alloc()).Get_command_queue()
}

func (self *MCU) Setup_pin(pin_type string, pin_params map[string]interface{}) interface{} {
	return mcupkg.SetupLegacyControllerPin(self, pin_type, pin_params, func(pinParams map[string]interface{}) interface{} {
		trdispatch := chelper.Trdispatch_alloc()
		return mcupkg.NewManagedLegacyEndstop(
			self,
			pinParams,
			trdispatch,
			func(mcuKey interface{}, trdispatch interface{}) mcupkg.EndstopManagedTrsync {
				return mcuKey.(*MCU).newManagedTrsync(trdispatch)
			},
			func() mcupkg.WaitableCompletion {
				return self.Get_printer().Get_reactor().Completion()
			},
			func(trdispatch interface{}, hostReason int64) {
				chelper.Trdispatch_start(trdispatch, uint32(hostReason))
			},
			func(trdispatch interface{}) {
				chelper.Trdispatch_stop(trdispatch)
			},
		)
	})
}

func Add_printer_objects_mcu(config *ConfigWrapper) {
	printer := config.Get_printer()
	reactor := printer.Get_reactor()
	mainsync := serialpkg.NewClockSync(serialpkg.NewReactorAdapter(reactor))
	printer.Add_object("mcu", NewMCU(config.Getsection("mcu"), mainsync))
	for _, s := range config.Get_prefix_sections("mcu ") {
		printer.Add_object(s.Section, NewMCU(
			s, serialpkg.NewSecondarySync(serialpkg.NewReactorAdapter(reactor), mainsync)))
	}
}

func Get_printer_mcu(printer *Printer, name string) *MCU {
	if name == "mcu" {
		return printer.Lookup_object(name, object.Sentinel{}).(*MCU)
	}

	return printer.Lookup_object("mcu "+name, object.Sentinel{}).(*MCU)
}

func (self *MCU) Create_oid() int {
	self.Oid_count += 1
	return self.Oid_count - 1
}

func (self *MCU) Register_config_callback(cb interface{}) {
	self._config_session.RegisterCallback(cb.(func()))
}

func (self *MCU) Add_config_cmd(cmd string, is_init bool, on_restart bool) {
	self._config_session.AddCommand(cmd, is_init, on_restart)
}

func (self *MCU) Get_query_slot(oid int) int64 {
	return mcupkg.QuerySlot(oid, self)
}

func (self *MCU) SecondsToClock(time float64) int64 {
	return self.Seconds_to_clock(time)
}

func (self *MCU) EstimatedPrintTime(eventtime float64) float64 {
	return self.Estimated_print_time(eventtime)
}

func (self *MCU) Monotonic() float64 {
	return self._reactor.Monotonic()
}

func (self *MCU) PrintTimeToClock(printTime float64) int64 {
	return self.Print_time_to_clock(printTime)
}

func (self *MCU) Register_stepqueue(stepqueue interface{}) {
	self._stepqueues = append(self._stepqueues, stepqueue)
}

func (self *MCU) Request_move_queue_slot() {
	self._reserved_move_slots += 1
}

func (self *MCU) Seconds_to_clock(time float64) int64 {
	return int64(time * self._mcu_freq)
}

func (self *MCU) Get_max_stepper_error() float64 {
	return self._max_stepper_error
}

func (self *MCU) Get_printer() *Printer {
	return self._printer
}

func (self *MCU) Get_name() string {
	return self._name
}

func (self *MCU) Register_response(cb interface{}, msg string, oid interface{}) {
	self.Serial.Register_response(cb, msg, oid)
}

func (self *MCU) Alloc_command_queue() interface{} {
	return self.Serial.Alloc_command_queue()
}

func (self *MCU) Lookup_command(msgformat string, cq interface{}) (*serialpkg.CommandWrapper, error) {
	return serialpkg.NewCommandWrapper(self.Serial, msgformat, cq)
}

func (self *MCU) Lookup_query_command(msgformat string, respformat string, oid int,
	cq interface{}, is_async bool) *serialpkg.CommandQueryWrapper {
	return serialpkg.NewCommandQueryWrapper(self.Serial, msgformat, respformat, oid,
		cq, is_async, self._printer.Command_error)
}

func (self *MCU) Try_lookup_command(msgformat string) interface{} {
	ret, err := self.Lookup_command(msgformat, nil)
	if err != nil {
		return nil
	}
	return ret
}

func (self *MCU) Lookup_command_tag(msgformat string) interface{} {
	return msgproto.FindCommandTag(self.Serial.Get_msgparser().Get_messages(), msgformat)
}

func (self *MCU) Get_enumerations() map[string]interface{} {
	return self.Serial.Get_msgparser().Get_enumerations()
}

func (self *MCU) Get_constants() map[string]interface{} {
	return self.Serial.Get_msgparser().Get_constants()
}

func (self *MCU) Get_constant_float(name string) float64 {
	return self.Serial.Get_msgparser().Get_constant_float(name, 0.0)
}

func (self *MCU) Print_time_to_clock(print_time float64) int64 {
	return self._clocksync.Print_time_to_clock(print_time)
}

func (self *MCU) Clock_to_print_time(clock int64) float64 {
	return self._clocksync.Clock_to_print_time(clock)
}

func (self *MCU) Estimated_print_time(eventtime float64) float64 {
	return self._clocksync.Estimated_print_time(eventtime)
}

func (self *MCU) Clock32_to_clock64(clock32 int64) int64 {
	return self._clocksync.Clock32_to_clock64(clock32)
}

func (self *MCU) CreateOID() int {
	return self.Create_oid()
}

func (self *MCU) RegisterConfigCallback(cb func()) {
	self.Register_config_callback(cb)
}

func (self *MCU) AddConfigCmd(cmd string, isInit bool, onRestart bool) {
	self.Add_config_cmd(cmd, isInit, onRestart)
}

func (self *MCU) GetQuerySlot(oid int) int64 {
	return self.Get_query_slot(oid)
}

func (self *MCU) RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{}) {
	self.Register_response(cb, msg, oid)
}

func (self *MCU) ClockToPrintTime(clock int64) float64 {
	return self.Clock_to_print_time(clock)
}

func (self *MCU) Clock32ToClock64(clock32 int64) int64 {
	return self.Clock32_to_clock64(clock32)
}

func (self *MCU) RegisterFlushCallback(callback func(float64, int64)) {
	self._flush_callbacks = append(self._flush_callbacks, callback)
}

func (self *MCU) AllocCommandQueue() interface{} {
	return self.Alloc_command_queue()
}

func (self *MCU) LookupCommand(msgformat string, cq interface{}) (interface{}, error) {
	command, err := self.Lookup_command(msgformat, cq)
	if err != nil {
		return nil, err
	}
	return command, nil
}

func (self *MCU) LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error) {
	return self.Lookup_command(msgformat, cq)
}

func (self *MCU) LookupCommandTag(msgformat string) interface{} {
	return self.Lookup_command_tag(msgformat)
}

func (self *MCU) LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{} {
	return self.Lookup_query_command(msgformat, respformat, oid, cq, isAsync)
}

var _ mcupkg.QuerySlotSource = (*MCU)(nil)
var _ mcupkg.MoveQueueTimingSource = (*MCU)(nil)
var _ mcupkg.StepperSync = (*MCU)(nil)

type Config_error struct {
	E string
}

type ConfigWrapper struct {
	printer         *Printer
	fileconfig      *configparser.RawConfigParser
	access_tracking map[string]interface{}
	Section         string
}

var _ printerpkg.ModuleConfig = (*ConfigWrapper)(nil)

func NewConfigWrapper(printer *Printer, fileconfig *configparser.RawConfigParser, access_tracking map[string]interface{}, section string) *ConfigWrapper {
	return &ConfigWrapper{
		printer:         printer,
		fileconfig:      fileconfig,
		access_tracking: pkgconfig.EnsureAccessTracking(access_tracking),
		Section:         section,
	}
}

func NewRootConfigWrapper(printer *Printer, fileconfig *configparser.RawConfigParser, section string) *ConfigWrapper {
	return NewConfigWrapper(printer, fileconfig, nil, section)
}

func (self *ConfigWrapper) accessTracking() map[string]interface{} {
	self.access_tracking = pkgconfig.EnsureAccessTracking(self.access_tracking)
	return self.access_tracking
}

func (self *ConfigWrapper) sectionWrapper(fileconfig *configparser.RawConfigParser, section string) *ConfigWrapper {
	return NewConfigWrapper(self.printer, fileconfig, self.accessTracking(), section)
}

func (self *ConfigWrapper) Fileconfig() *configparser.RawConfigParser {
	return self.fileconfig
}

func (self *ConfigWrapper) Get_printer() *Printer {
	return self.printer
}

func (self *ConfigWrapper) Get_name() string {
	return self.Section
}

func (self *ConfigWrapper) Name() string {
	return self.Get_name()
}

func (self *ConfigWrapper) String(option string, defaultValue string, noteValid bool) string {
	return self.Get(option, defaultValue, noteValid).(string)
}

func (self *ConfigWrapper) Bool(option string, defaultValue bool) bool {
	return self.Getboolean(option, defaultValue, true)
}

func (self *ConfigWrapper) Float(option string, defaultValue float64) float64 {
	return self.Getfloat(option, defaultValue, 0., 0., 0., 0., true)
}

func (self *ConfigWrapper) OptionalFloat(option string) *float64 {
	value := self.GetfloatNone(option, nil, 0., 0., 0., 0., true)
	if value == nil {
		return nil
	}
	floatValue := cast.ToFloat64(value)
	return &floatValue
}

func (self *ConfigWrapper) HasOption(option string) bool {
	return self.fileconfig.Has_option(self.Section, option)
}

func (self *ConfigWrapper) HasSection(section string) bool {
	return self.Has_section(section)
}

func (self *ConfigWrapper) SectionConfig(section string) printerpkg.ModuleConfig {
	return self.Getsection(section)
}

func (self *ConfigWrapper) LoadObject(section string) interface{} {
	return self.Get_printer().Load_object(self, section, object.Sentinel{})
}

func (self *ConfigWrapper) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	if module == "gcode_macro" || module == "gcode_macro_1" {
		return gcodepkg.LoadMacroTemplate(self, option, defaultValue)
	}
	panic(fmt.Sprintf("unsupported template loader module '%s'", module))
}

func (self *ConfigWrapper) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return self.LoadTemplate(module, option, "")
}

func (self *ConfigWrapper) Printer() printerpkg.ModulePrinter {
	return self.Get_printer()
}

func (self *ConfigWrapper) LoadSupportConfig(filename string) error {
	if _, err := os.Stat(filename); err != nil {
		return err
	}
	pconfig := self.Get_printer().Lookup_object("configfile", object.Sentinel{}).(*PrinterConfig)
	dconfig := pconfig.Read_config(filename)
	for _, c := range dconfig.Get_prefix_sections("") {
		dconfig.LoadObject(c.Get_name())
	}
	return nil
}

func (self *ConfigWrapper) noteAccess(option string, value interface{}) {
	self.access_tracking = pkgconfig.NoteAccess(self.accessTracking(), self.Section, option, value)
}

func (self *ConfigWrapper) trackAccess(option string, value interface{}, noteValid bool) {
	self.access_tracking = pkgconfig.MaybeNoteAccess(self.accessTracking(), self.Section, option, value, noteValid)
}

func (self *ConfigWrapper) getDefault(option string, defaultValue interface{}, noteValid bool) (interface{}, bool) {
	if self.fileconfig.Has_option(self.Section, option) {
		return nil, false
	}
	if !object.IsNotSentinel(defaultValue) {
		panic(fmt.Sprintf("Option '%s' in section '%s' must be specified", option, self.Section))
	}
	self.trackAccess(option, defaultValue, noteValid)
	return defaultValue, true
}

func (self *ConfigWrapper) Get(option string, default1 interface{}, note_valid bool) interface{} {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		return defaultValue
	}
	v := self.fileconfig.Get(self.Section, option)
	self.trackAccess(option, v, note_valid)
	return v
}

func (self *ConfigWrapper) Getint(option string, default1 interface{}, minval, maxval int,
	note_valid bool) int {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		ret, ok := defaultValue.(int)
		if ok {
			return ret
		}
		return 0
	}
	v := self.fileconfig.Getint(self.Section, option)
	n := v.(int)
	self.trackAccess(option, n, note_valid)
	if err := pkgconfig.ValidateIntRange(option, self.Section, n, minval, maxval); err != nil {
		panic(err)
	}
	return n
}

func (self *ConfigWrapper) Getint64(option string, default1 interface{}, minval, maxval int64,
	note_valid bool) int64 {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		ret, ok := defaultValue.(int64)
		if ok {
			return ret
		}
		return 0
	}
	v := self.fileconfig.Getint(self.Section, option)
	n := v.(int64)
	self.trackAccess(option, n, note_valid)
	if err := pkgconfig.ValidateInt64Range(option, self.Section, n, minval, maxval); err != nil {
		panic(err)
	}
	return n
}

func (self *ConfigWrapper) GetintNone(option string, def interface{}, minval, maxval int,
	note_valid bool) interface{} {
	if defaultValue, ok := self.getDefault(option, def, note_valid); ok {
		return defaultValue
	}
	v := self.fileconfig.Getint(self.Section, option)
	n := cast.ToInt(v)
	self.trackAccess(option, n, note_valid)
	if err := pkgconfig.ValidateIntRange(option, self.Section, n, minval, maxval); err != nil {
		logger.Error(err)
	}
	return n
}

func (self *ConfigWrapper) Getfloat(option string, default1 interface{}, minval, maxval,
	above, below float64, note_valid bool) float64 {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		ret, ok := defaultValue.(float64)
		if ok {
			return ret
		}
		return 0
	}
	v := self.fileconfig.Getfloat(self.Section, option)
	n, ok := v.(float64)
	if !ok {
		panic(fmt.Sprintf("Unable to parse option '%s' in section '%s'", option, self.Section))
	}
	self.trackAccess(option, n, note_valid)
	if err := pkgconfig.ValidateFloatRange(option, self.Section, n, minval, maxval, above, below); err != nil {
		panic(err)
	}

	return n
}

func (self *ConfigWrapper) GetfloatNone(option string, default1 interface{}, minval, maxval,
	above, below float64, note_valid bool) interface{} {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		return defaultValue
	}
	v := self.fileconfig.Getfloat64None(self.Section, option)
	if v == nil {
		return nil
	}
	n := cast.ToFloat64(v)
	self.trackAccess(option, n, note_valid)

	if err := pkgconfig.ValidateFloatRange(option, self.Section, n, minval, maxval, above, below); err != nil {
		logger.Error(err)
	}
	return v
}

func (self *ConfigWrapper) Getboolean(option string, default1 interface{}, note_valid bool) bool {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		ret, ok := defaultValue.(bool)
		if ok {
			return ret
		}
		return false
	}
	v := self.fileconfig.Getboolean(self.Section, option)
	result := v.(bool)
	self.trackAccess(option, result, note_valid)
	return result
}

func (self *ConfigWrapper) Getchoice(option string, choices map[interface{}]interface{}, default1 interface{}, note_valid bool) interface{} {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		ret, ok := defaultValue.(string)
		if ok {
			return ret
		}
		return ""
	}
	var c interface{}
	for k := range choices {
		if reflect.TypeOf(k).Kind() == reflect.Int {
			c = self.Getint(option, default1, 0, 0, true)
		} else {
			c = self.Get(option, default1, true)
		}
	}
	ret, ok := choices[c]
	if !ok {
		logger.Errorf("Choice '%s' for option '%s' in section '%s' is not a valid choice", c, option, self.Section)
	}
	return ret

}

func (self *ConfigWrapper) Getlists(option string, default1 interface{}, seps []string, count int, kind reflect.Kind, note_valid bool) interface{} {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		return defaultValue
	}
	if len(seps) == 2 && seps[0] == "," && seps[1] == "\n" {
		value := self.fileconfig.Get(self.Section, option)
		if str, ok := value.(string); ok {
			parsed := pkgconfig.ParseMultilineList(str, kind)
			self.trackAccess(option, parsed, note_valid)
			return parsed
		}
		parsed := [][]interface{}{}
		self.trackAccess(option, parsed, note_valid)
		return parsed
	}
	value := self.fileconfig.Get(self.Section, option)
	if str, ok := value.(string); ok {
		parsed := pkgconfig.ParseSeparatedList(str, seps, kind)
		self.trackAccess(option, parsed, note_valid)
		return parsed
	}
	parsed := []interface{}{}
	self.trackAccess(option, parsed, note_valid)
	return parsed
}

func fcparser(section, option string) {
	_ = section
	_ = option
}

func (self *ConfigWrapper) Getlist(option string, default1 interface{}, sep string, count int, note_valid bool) interface{} {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		return defaultValue
	}
	ret := []interface{}{}

	value := self.fileconfig.Get(self.Section, option)
	str, ok := value.(string)
	if ok {
		strs := strings.Split(str, sep)
		for _, s := range strs {
			ret = append(ret, s)
		}
	}
	for i := 0; i < count-len(ret); i++ {
		ret = append(ret, 0)
	}
	self.trackAccess(option, ret, note_valid)
	return ret
}

func (self *ConfigWrapper) Getintlist(option string, default1 interface{}, sep string, count int,
	note_valid bool) []int {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		ret, ok := defaultValue.([]int)
		if ok {
			return ret
		}
		return nil
	}
	if value := self.fileconfig.Get(self.Section, option); value != nil {
		if str, ok := value.(string); ok {
			parsed := pkgconfig.ParseIntListFromString(str, sep, count)
			self.trackAccess(option, parsed, note_valid)
			return parsed
		}
	}
	self.trackAccess(option, nil, note_valid)
	return nil
}

func (self *ConfigWrapper) Getfloatlist(option string, default1 interface{}, sep string, count int,
	note_valid bool) []float64 {
	if defaultValue, ok := self.getDefault(option, default1, note_valid); ok {
		ret, ok := defaultValue.([]float64)
		if ok {
			return ret
		}
		return nil
	}
	if value := self.fileconfig.Get(self.Section, option); value != nil {
		if str, ok := value.(string); ok {
			parsed := pkgconfig.ParseFloatListFromString(str, sep, count)
			self.trackAccess(option, parsed, note_valid)
			return parsed
		}
	}
	self.trackAccess(option, nil, note_valid)
	return nil
}

func (self *ConfigWrapper) Getsection(section string) *ConfigWrapper {
	return self.sectionWrapper(self.fileconfig, section)
}

func (self *ConfigWrapper) Has_section(section string) bool {
	return self.fileconfig.Has_section(section)
}

func (self *ConfigWrapper) Get_prefix_sections(prefix string) []*ConfigWrapper {
	configs := []*ConfigWrapper{}
	for _, s := range self.fileconfig.Sections() {
		if strings.HasPrefix(s, prefix) {
			configs = append(configs, self.Getsection(s))
		}
	}
	return configs
}

func (self *ConfigWrapper) Get_prefix_options(prefix string) []string {
	options, _ := self.fileconfig.Options(self.Section)
	prefixOpts := []string{}

	for o := range options {
		if prefix == "variable_" {
			if strings.HasPrefix(o, prefix) {
				prefixOpts = append(prefixOpts, o)
			}
		} else {
		}
	}

	return prefixOpts
}

func (self *ConfigWrapper) Deprecate(option, value string) {
	if !self.fileconfig.Has_option(self.Section, option) {
		return
	}
	msg := ""
	if value == "" {
		msg = fmt.Sprintf("Option '%s' in section '%s' is deprecated.", option, self.Section)
	} else {
		msg = fmt.Sprintf("Value '%s' in option '%s' in section '%s' is deprecated.", value, option, self.Section)
	}
	pconfig := self.printer.Lookup_object("configfile", object.Sentinel{}).(*PrinterConfig)
	pconfig.Deprecate(self.Section, option, value, msg)
}

type PrinterConfig struct {
	printer  *Printer
	autosave *ConfigWrapper
	status   *pkgconfig.RuntimeStatus
}

func NewPrinterConfig(printer *Printer) *PrinterConfig {
	self := PrinterConfig{}
	self.printer = printer
	self.autosave = nil
	self.status = pkgconfig.NewRuntimeStatus()
	gcode := self.printer.Lookup_object("gcode", object.Sentinel{})
	gcode.(*GCodeDispatch).Register_command("SAVE_CONFIG", self.cmd_SAVE_CONFIG, false, cmd_SAVE_CONFIG_help)
	return &self
}

func (self *PrinterConfig) Get_printer() *Printer {
	return self.printer
}

func (self *PrinterConfig) Read_config(filename string) *ConfigWrapper {
	data, _ := pkgconfig.ReadConfigFile(filename)
	return NewRootConfigWrapper(self.printer, pkgconfig.ParseConfigText(data, filename), "printer")
}

func (self *PrinterConfig) Read_main_config() *ConfigWrapper {
	filename := self.printer.Get_start_args()["config_file"].(string)
	bundle, err := pkgconfig.LoadMainConfigBundle(filename)
	if err != nil {
		panic("read config _read_config_file: " + err.Error())
	}
	self.autosave = NewRootConfigWrapper(self.printer, bundle.Autosave, "printer")
	return NewRootConfigWrapper(self.printer, bundle.Combined, "printer")
}

func (self *PrinterConfig) Check_unused_options(config *ConfigWrapper) {
	self.status.Rebuild(config.Fileconfig(), config.accessTracking())
}

func (self *PrinterConfig) Log_config(config *ConfigWrapper) {
	lines := []string{"===== Config file =====",
		pkgconfig.RenderConfig(config.Fileconfig()),
		"======================="}
	self.printer.Set_rollover_info("config", strings.Join(lines, "\n"), true)
}

func (self *PrinterConfig) Deprecate(section, option, value, msg string) {
	self.status.Deprecate(section, option, value, msg)
}

func (self *PrinterConfig) Get_status(eventtime float64) map[string]interface{} {
	_ = eventtime
	return self.status.Snapshot()
}

func (self *PrinterConfig) Set(section, option, val string) {
	if !self.autosave.fileconfig.Has_section(section) {
		self.autosave.fileconfig.Add_section(section)
	}

	self.autosave.fileconfig.Set(section, option, val)
	self.status.NotePendingSet(section, option, val)
	logger.Infof("save_config: set [%s] %s = %s", section, option, val)
}

func (self *PrinterConfig) Remove_section(section string) {
	removedAutosaveSection := self.autosave.fileconfig.Has_section(section)
	if removedAutosaveSection {
		self.autosave.fileconfig.Remove_section(section)
	}
	self.status.NotePendingRemoval(section, removedAutosaveSection)
}

const cmd_SAVE_CONFIG_help = "Overwrite config file and restart"

func (self *PrinterConfig) cmd_SAVE_CONFIG(argv interface{}) error {
	_ = argv
	if self.autosave.fileconfig.Sections() == nil {
		return nil
	}
	gcode := MustLookupGcode(self.printer)
	cfgname := self.printer.Get_start_args()["config_file"].(string)

	data, err := pkgconfig.ReadConfigFile(cfgname)
	if err != nil {
		msg := "Unable to parse existing config on SAVE_CONFIG"
		logger.Error(msg)
		return errors.New(msg)
	}
	plan, err := pkgconfig.BuildSaveConfigPlan(cfgname, data, self.autosave.Fileconfig(), time.Now)
	if err != nil {
		panic(err)
	}
	logger.Infof("SAVE_CONFIG to '%s' (backup in '%s')",
		cfgname, plan.BackupName)
	err1 := ioutil.WriteFile(plan.TempName, []byte(plan.Data), 0666)
	if err1 != nil {
		msg := "Unable to write config file during SAVE_CONFIG"
		logger.Error(msg)
		return err1
	}
	os.Rename(cfgname, plan.BackupName)
	os.Rename(plan.TempName, cfgname)

	gcode.Request_restart("restart")

	return nil
}

func Add_early_printer_objects1(printer *Printer) {
	mutex := printer.Get_reactor().Mutex(false)
	printer.Add_object("gcode", gcodepkg.NewDispatcher(gcodepkg.DispatcherOptions{
		Host: &gcodepkg.DispatcherHostFuncs{
			StartArgsFunc: func() map[string]interface{} {
				return printer.Get_start_args()
			},
			RegisterEventHandlerFunc: printer.Register_event_handler,
			LookupObjectFunc:         printer.Lookup_object,
			InvokeShutdownFunc: func(msg string) {
				printer.Invoke_shutdown(msg)
			},
			SendEventFunc: func(event string, params []interface{}) {
				_, _ = printer.Send_event(event, params)
			},
			RequestExitFunc: printer.Request_exit,
			StateMessageFunc: func() (string, string) {
				return printer.get_state_message()
			},
		},
		Lock:   mutex.Lock,
		Unlock: mutex.Unlock,
		IsBusy: mutex.Test,
	}))
}

func (self *Printer) reactorBridge() *printerpkg.ReactorAdapter {
	if self.reactorAdapter == nil {
		self.reactorAdapter = printerpkg.NewReactorAdapterFrom[*ReactorTimer, *ReactorCompletion](self.reactor)
	}
	return self.reactorAdapter
}

func NewPrinter(main_reactor IReactor, start_args map[string]interface{}) *Printer {
	self := Printer{}
	self.config_error = &Config_error{}
	self.Command_error = &CommandError{}
	self.reactor = main_reactor
	self.runtime = printerpkg.NewRuntime(self.reactorBridge(), start_args)
	self.reactor.Register_callback(self._connect, constants.NOW)
	// Init printer components that must be setup prior to config
	for _, m := range []func(*Printer){Add_early_printer_objects1,
		Add_early_printer_objects_webhooks} {
		m(&self)
	}
	self.Module = LoadMainModule()
	return &self
}

func LoadMainModule() *printerpkg.ModuleRegistry {
	return moduleinitpkg.BuildRegistry(registerProjectModules)
}

func registerConfigWrapperModule(module *printerpkg.ModuleRegistry, name string, init func(*ConfigWrapper) interface{}) {
	module.Register(name, moduleinitpkg.WrapModuleInit[*ConfigWrapper](init))
}

func registerProjectModules(module *printerpkg.ModuleRegistry) {
	for _, item := range []struct {
		name string
		init func(*ConfigWrapper) interface{}
	}{
		{name: "homing", init: Load_config_homing},
		{name: "probe", init: Load_config_probe},
		{name: "bed_mesh", init: Load_config_bed_mesh},
		{name: "leviq3", init: Load_config_LeviQ3},
		{name: "tmc2209", init: func(config *ConfigWrapper) interface{} {
			return tmcpkg.LoadConfigTMC2209(config, projectTMCDriverAdapter)
		}},
		{name: "tmc2240", init: func(config *ConfigWrapper) interface{} {
			return tmcpkg.LoadConfigTMC2240(config, projectTMCDriverAdapter)
		}},
		{name: "adxl345", init: func(config *ConfigWrapper) interface{} {
			spi, err := MCU_SPI_from_config(config, 3, "cs_pin", 5000000, nil, false)
			if err != nil {
				panic(fmt.Errorf("MCU_SPI_from_config error: %v", err))
			}
			module := vibrationpkg.NewADXL345Module(config, spi)
			webhookspkg.RegisterAccelerometerDumpEndpoint(MustLookupWebhooks(config.Get_printer()), "adxl345/dump_adxl345", module)
			return module
		}},
		{name: "lis2dw12", init: func(config *ConfigWrapper) interface{} {
			spi, err := MCU_SPI_from_config(config, 3, "cs_pin", 5000000, nil, false)
			if err != nil {
				panic(fmt.Errorf("MCU_SPI_from_config error: %v", err))
			}
			module := vibrationpkg.NewLIS2DW12Module(config, spi)
			webhookspkg.RegisterAccelerometerDumpEndpoint(MustLookupWebhooks(config.Get_printer()), "lis2dw12/dump_lis2dw12", module)
			return module
		}},
		{name: "manual_stepper", init: Load_config_manual_stepper},
		{name: "manual_probe", init: Load_config_ManualProbe},
		{name: "ace", init: func(config *ConfigWrapper) interface{} {
			return filamentpkg.LoadConfigACE(config)
		}},
		{name: "filament_hub", init: func(config *ConfigWrapper) interface{} {
			return filamentpkg.LoadConfigFilamentHub(config)
		}},
	} {
		registerConfigWrapperModule(module, item.name, item.init)
	}
}

func (self *Printer) Get_start_args() map[string]interface{} {
	return self.runtime.GetStartArgs()
}
func (self *Printer) Get_reactor() IReactor {
	return self.reactor
}
func (self *Printer) get_state_message() (string, string) {
	return self.runtime.StateMessage()
}
func (self *Printer) Is_shutdown() bool {
	return self.runtime.IsShutdown()
}
func (self *Printer) _set_state(msg string) {
	self.runtime.SetState(msg)
}
func (self *Printer) Add_object(name string, obj interface{}) error {
	return self.runtime.AddObject(name, obj)
}
func (self *Printer) Lookup_object(name string, default1 interface{}) interface{} {
	return self.runtime.LookupObject(name, default1)
}

func MustLookupToolhead(printer *Printer) *Toolhead {
	toolhead, ok := printer.Lookup_object("toolhead", object.Sentinel{}).(*Toolhead)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "toolhead", printer.Lookup_object("toolhead", object.Sentinel{})))
	}
	return toolhead
}

func MustLookupPins(printer *Printer) *printerpkg.PrinterPins {
	pins, ok := printer.Lookup_object("pins", object.Sentinel{}).(*printerpkg.PrinterPins)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "pins", printer.Lookup_object("pins", object.Sentinel{})))
	}
	return pins
}

func MustLookupGcode(printer *Printer) *GCodeDispatch {
	gcode, ok := printer.Lookup_object("gcode", object.Sentinel{}).(*GCodeDispatch)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "gcode", printer.Lookup_object("gcode", object.Sentinel{})))
	}
	return gcode
}

func MustLookupGCodeMove(printer *Printer) *gcodepkg.GCodeMoveModule {
	gcodeMove, ok := printer.Lookup_object("gcode_move", object.Sentinel{}).(*gcodepkg.GCodeMoveModule)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "gcode_move", printer.Lookup_object("gcode_move", object.Sentinel{})))
	}
	return gcodeMove
}

func (self *Printer) LookupObject(name string, defaultValue interface{}) interface{} {
	obj := self.Lookup_object(name, defaultValue)
	switch name {
	case "pins":
		if typed, ok := obj.(*printerpkg.PrinterPins); ok {
			return printerpkg.NewPinRegistryRuntimeAdapter(printerpkg.PinRegistryRuntimeAdapterOptions{
				RegisterChip: typed.Register_chip,
				SetupPWM: func(pin string) interface{} {
					return typed.Setup_pin("pwm", pin).(*mcupkg.PWMPin)
				},
				SetupDigitalOut: func(pin string) printerpkg.DigitalOutPin {
					return typed.Setup_pin("digital_out", pin).(*mcupkg.DigitalOutPin)
				},
				SetupADC: func(pin string) printerpkg.ADCPin {
					return typed.Setup_pin("adc", pin).(*mcupkg.ADCPin)
				},
				LookupPin: typed.Lookup_pin,
			})
		}
	case "heaters":
		if typed, ok := obj.(*heaterpkg.PrinterHeaters); ok {
			return printerpkg.NewHeaterManagerAdapter(printerpkg.HeaterManagerAdapterOptions{
				LookupHeater: func(name string) interface{} {
					return typed.Lookup_heater(name)
				},
				SetupHeater: func(config printerpkg.ModuleConfig, gcodeID string) interface{} {
					return typed.Setup_heater(config, gcodeID)
				},
				SetTemperature: func(heater interface{}, temp float64, wait bool) error {
					switch adapted := heater.(type) {
					case *printerpkg.HeaterRuntimeAdapter:
						rawHeater, ok := adapted.Source().(*heaterpkg.Heater)
						if !ok {
							break
						}
						return typed.Set_temperature(rawHeater, temp, wait)
					case *heaterpkg.Heater:
						return typed.Set_temperature(adapted, temp, wait)
					}
					panic("unsupported heater adapter type")
				},
			})
		}
	case "toolhead":
		if typed, ok := obj.(*Toolhead); ok {
			return typed
		}
	case "configfile":
		if typed, ok := obj.(*PrinterConfig); ok {
			return typed
		}
	}
	return obj
}

func (self *Printer) RegisterEventHandler(event string, callback func([]interface{}) error) {
	self.Register_event_handler(event, callback)
}

func (self *Printer) SendEvent(event string, params []interface{}) {
	_, _ = self.Send_event(event, params)
}

func (self *Printer) CurrentExtruderName() string {
	return MustLookupToolhead(self).Get_extruder().Get_name()
}

func (self *Printer) AddObject(name string, obj interface{}) error {
	return self.Add_object(name, obj)
}

func (self *Printer) LookupObjects(module string) []interface{} {
	return self.Lookup_objects(module)
}

func (self *Printer) HasStartArg(name string) bool {
	_, ok := self.Get_start_args()[name]
	return ok
}

func (self *Printer) LookupHeater(name string) printerpkg.HeaterRuntime {
	pheaters := self.Lookup_object("heaters", object.Sentinel{}).(*heaterpkg.PrinterHeaters)
	heater := pheaters.Lookup_heater(name)
	return printerpkg.NewHeaterRuntimeAdapter(heater, heater.Get_temp)
}

func (self *Printer) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	pheaters := self.Lookup_object("heaters", object.Sentinel{}).(*heaterpkg.PrinterHeaters)
	return printerpkg.NewTemperatureSensorRegistryAdapter(func(sensorType string, factory printerpkg.TemperatureSensorFactory) {
		pheaters.Add_sensor_factory(sensorType, factory)
	})
}

func (self *Printer) LookupMCU(name string) printerpkg.MCURuntime {
	return Get_printer_mcu(self, name)
}

func (self *Printer) InvokeShutdown(msg string) {
	self.Invoke_shutdown(msg)
}

func (self *Printer) IsShutdown() bool {
	return self.Is_shutdown()
}

func (self *Printer) Reactor() printerpkg.ModuleReactor {
	return self.reactorBridge()
}

func (self *Printer) StepperEnable() printerpkg.StepperEnableRuntime {
	stepperEnable, ok := self.Lookup_object("stepper_enable", object.Sentinel{}).(*mcupkg.PrinterStepperEnableModule)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "stepper_enable", self.Lookup_object("stepper_enable", object.Sentinel{})))
	}
	return stepperEnable
}

func (self *Printer) GCode() printerpkg.GCodeRuntime {
	return MustLookupGcode(self)
}

func (self *Printer) GCodeMove() printerpkg.MoveTransformController {
	return MustLookupGCodeMove(self)
}

func (self *Printer) Lookup_objects(module string) []interface{} {
	return self.runtime.LookupObjects(module)
}

func (self *Printer) Load_object(config *ConfigWrapper, section string, default1 interface{}) interface{} {
	obj := self.Module.LoadObject(section,
		func(name string) interface{} {
			return self.Lookup_object(name, nil)
		},
		func(name string) interface{} {
			return config.Getsection(name)
		},
		self.runtime.StoreObject,
	)
	if obj == nil {
		if _, ok := default1.(*object.Sentinel); ok {
			self.config_error.E = fmt.Sprintf("Unable to load module '%s'", section)
			logger.Info("moudle as ", section, " is not support")
		} else {
			if _, ok := default1.(object.Sentinel); ok {
				self.config_error.E = fmt.Sprintf("Unable to load module '%s'", section)
				logger.Info("moudle as ", section, " is not support")
			} else {
				return default1
			}

		}
	}
	return obj
}

func (self *Printer) Load_main_config_object(name string, config *ConfigWrapper, init func(*ConfigWrapper) interface{}) interface{} {
	if obj := self.Lookup_object(name, nil); obj != nil {
		return obj
	}
	obj := init(config)
	_ = self.Add_object(name, obj)
	return obj
}

func (self *Printer) _read_config() {
	pconfig := NewPrinterConfig(self)
	self.runtime.StoreObject("configfile", pconfig)

	config := pconfig.Read_main_config()
	// Create printer components
	config.Get_printer().Add_object("pins", printerpkg.NewPrinterPins())
	for _, m := range []func(*ConfigWrapper){Add_printer_objects_mcu} {
		m(config)
	}

	for _, section_config := range config.Get_prefix_sections("") {
		self.Load_object(config, section_config.Get_name(), value.None)
	}
	self.Load_main_config_object("toolhead", config, Load_config_toolhead)
	Add_printer_objects_extruder(config)
	// Validate that there are no undefined parameters in the config file
	pconfig.Check_unused_options(config)
}
func (self *Printer) _build_protocol_error_message(e interface{}) string {
	host_version := fmt.Sprintf("%v", self.Get_start_args()["software_version"])
	msg_update := []string{}
	msg_updated := []string{}
	for _, m := range self.Lookup_objects("mcu") {
		mcuInfo, ok := m.(map[string]interface{})
		if !ok {
			msg_update = append(msg_update, fmt.Sprintf("<unknown>: invalid MCU metadata %T", m))
			continue
		}
		mcu_name, _ := mcuInfo["mcu_name"].(string)
		if strings.TrimSpace(mcu_name) == "" {
			mcu_name = "<unknown>"
		}
		mcu, ok := mcuInfo["mcu"].(*MCU)
		if !ok || mcu == nil {
			msg_update = append(msg_update, fmt.Sprintf("%s: MCU not initialized", strings.TrimSpace(mcu_name)))
			continue
		}
		mcu_version := fmt.Sprintf("%v", mcu.Get_status(0)["mcu_version"])

		if mcu_version != host_version {
			msg_update = append(msg_update, fmt.Sprintf("%s: Current version %s", strings.TrimSpace(mcu_name), mcu_version))
		} else {
			msg_updated = append(msg_updated, fmt.Sprintf("%s: Current version %s", strings.TrimSpace(mcu_name), mcu_version))
		}
	}
	if len(msg_update) == 0 {
		msg_update = append(msg_update, "<none>")
	}
	if len(msg_updated) == 0 {
		msg_updated = append(msg_updated, "<none>")
	}
	return strings.Join([]string{
		fmt.Sprintf("MCU Protocol error: %v", e),
		message_protocol_error1,
		"",
		fmt.Sprintf("Host version: %s", host_version),
		"MCU version mismatch details:",
		strings.Join(msg_update, "\n"),
		"",
		"MCUs already matching the host:",
		strings.Join(msg_updated, "\n"),
		"",
		message_protocol_error2,
	}, "\n")
}
func (self *Printer) _connect(eventtime interface{}) interface{} {
	logger.Infof("printer: _connect enter eventtime=%v exit_result=%q", eventtime, self.runtime.ExitResult())
	self.tryCatchConnect1()
	if self.runtime.ExitResult() != "" {
		logger.Infof("printer: _connect returning early with exit_result=%q", self.runtime.ExitResult())
		return nil
	}
	self.tryCatchConnect2()
	logger.Infof("printer: _connect completed ready path exit_result=%q", self.runtime.ExitResult())
	return nil
}
func (self *Printer) tryCatchConnect1() {
	defer func() {
		if err := recover(); err != nil {
			if self.runtime.ExitResult() != "" {
				logger.Infof("printer: connect recover suppressed due to pending exit_result=%q recovered=%v", self.runtime.ExitResult(), err)
				return
			}
			_, ok1 := err.(*printerpkg.PinError)
			_, ok2 := err.(*Config_error)
			if ok1 || ok2 {
				logger.Error("Config error", err, string(debug.Stack()))
				self._set_state(fmt.Sprintf("%s\n%s", err, message_restart))
				return
			}
			_, ok11 := err.(msgproto.MsgprotoError)
			if ok11 {
				logger.Errorf("Protocol error during connect: %v\n%s", err, string(debug.Stack()))
				self._set_state(self._build_protocol_error_message(err))
				util.Dump_mcu_build()
				return
			}
			e, ok22 := err.(*erro)
			if ok22 {
				logger.Error("MCU error during connect", string(debug.Stack()))
				self._set_state(fmt.Sprintf("%s%s", e.err, message_mcu_connect_error))
				util.Dump_mcu_build()
				return
			}
			logger.Errorf("Unhandled exception during connect: %v, debug stack:\n%s\n", err, string(debug.Stack()))
			self._set_state(fmt.Sprintf("Internal error during connect: %s\n%s", err, message_restart))
			panic(err)
		}
	}()

	self._read_config()
	self.Send_event("project:mcu_identify", nil)
	cbs := self.runtime.EventHandlers("project:connect")
	logger.Info("Klipper-go: start running project:connect event handlers")
	for _, cb := range cbs {
		stateMessage, _ := self.get_state_message()
		if stateMessage != message_startup {
			return
		}
		logger.Infof("printer: invoking project:connect handler exit_result=%q", self.runtime.ExitResult())
		err := cb(nil)
		if err != nil {
			logger.Error("Config error: ", err)
			self._set_state(fmt.Sprintf("%s\n%s", err.Error(), message_restart))
			return
		}
		if self.runtime.ExitResult() != "" {
			logger.Infof("printer: connect handler returned with pending exit_result=%q", self.runtime.ExitResult())
			return
		}
	}
	logger.Info("Klipper-go: finished running project:connect event handlers")
}
func (self *Printer) tryCatchConnect2() {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("Unhandled exception during ready callback", err)
			self.Invoke_shutdown(fmt.Sprintf("Internal error during ready callback: %s", err))
			return
		}
	}()
	self._set_state(message_ready)
	logger.Info("Klipper-go: entered message_ready state!")
	cbs := self.runtime.EventHandlers("project:ready")
	for _, cb := range cbs {
		stateMessage, _ := self.get_state_message()
		if stateMessage != message_ready {
			return
		}
		err := cb(nil)
		if err != nil {
			logger.Error("Unhandled exception during ready callback")
			self._set_state(fmt.Sprintf("Internal error during ready callback:%s", err.Error()))
			return
		}
	}
}
func (self *Printer) Run() string {
	result := self.runtime.Run()
	logger.Infof("printer: Run returned result=%q", result)
	return result
}
func (self *Printer) Set_rollover_info(name, info string, isLog bool) {
	if isLog {
		logger.Debug(info)
	}
}
func (self *Printer) Invoke_shutdown(msg interface{}) interface{} {
	return self.runtime.InvokeShutdown(msg)
}

type ManualStepper struct {
	printer *Printer
	rail    *PrinterRail
	*motionpkg.ManualStepperModule
}

func NewManualStepper(config *ConfigWrapper) *ManualStepper {
	self := new(ManualStepper)
	self.printer = config.Get_printer()
	var steppers []motionpkg.LegacyRailStepper
	var homeHandler motionpkg.ManualStepperHomeHandler

	if config.Get("endstop_pin", "", true) != "" {
		var default_position_endstop *float64
		self.rail = NewPrinterRail(config, false, default_position_endstop, false)
		steppers = make([]motionpkg.LegacyRailStepper, 0, len(self.rail.Get_steppers()))
		for _, stepper := range self.rail.Get_steppers() {
			steppers = append(steppers, stepper)
		}
		homeHandler = func(movepos float64, speed float64, triggered bool, checkTrigger bool) error {
			pos := []float64{movepos, 0.0, 0.0, 0.0}
			endstops := self.rail.Get_endstops()
			phoming := self.printer.Lookup_object("homing", object.Sentinel{}).(*PrinterHoming)
			phoming.Manual_home(self, endstops, pos, speed, triggered, checkTrigger)
			return nil
		}
	} else {
		printer := config.Get_printer()
		steppers = []motionpkg.LegacyRailStepper{mcupkg.LoadLegacyPrinterStepper(config, false, printer, func(moduleName string) interface{} {
			return printer.Load_object(config, moduleName, object.Sentinel{})
		}, func(module interface{}, stepper *mcupkg.LegacyStepper) {
			module.(*mcupkg.PrinterStepperEnableModule).Register_stepper(config, stepper)
		}, func(module interface{}, stepper *mcupkg.LegacyStepper) {
			module.(*motionpkg.ForceMoveModule).RegisterStepper(stepper)
		})}
	}

	velocity := config.Getfloat("velocity", 5.0, 0, 0, 0, 0, true)
	accel := config.Getfloat("accel", 0.0, 0, 0, 0, 0, true)
	self.ManualStepperModule = motionpkg.NewManualStepperModule(
		steppers,
		velocity,
		accel,
		func() motionpkg.ManualStepperToolhead {
			return MustLookupToolhead(self.printer)
		},
		func() motionpkg.ManualStepperMotorController {
			stepperEnable := self.printer.Lookup_object("stepper_enable", object.Sentinel{}).(*mcupkg.PrinterStepperEnableModule)
			return stepperEnable
		},
		homeHandler,
	)

	stepperName := strings.Split(config.Get_name(), " ")[1]
	gcode := MustLookupGcode(self.printer)
	self.RegisterLegacyMuxCommand(gcode, stepperName)

	return self
}

func (self *ManualStepper) Check_move(move *Move) {
	_ = move
}

func (self *ManualStepper) Home(homing_state *Homing) {
	_ = homing_state
}

func (self *ManualStepper) Note_z_not_homed() {
}

func (self *ManualStepper) Drip_move(newpos []float64, speed float64, drip_completion *ReactorCompletion) error {
	_ = drip_completion
	return self.ManualStepperModule.Drip_move(newpos, speed)
}

func (self *ManualStepper) Get_kinematics() interface{} {
	return self
}

func Load_config_manual_stepper(config *ConfigWrapper) interface{} {
	return NewManualStepper(config)
}

type MCU_SPI struct {
	*mcupkg.LegacySPIBus
	mcu *MCU
}

var _ tmcpkg.SPIBusTransport = (*MCU_SPI)(nil)
var _ vibrationpkg.AccelerometerModuleSPITransport = (*MCU_SPI)(nil)

func NewMCU_SPI(mcu *MCU, bus string, pin interface{}, mode, speed int, sw_pins []interface{},
	cs_active_high bool) *MCU_SPI {
	return &MCU_SPI{
		LegacySPIBus: mcupkg.NewLegacySPIBus(
			mcupkg.NewLegacySPIBusCommandAdapter(mcu),
			func(pin string, owner string) {
				if pin == "" {
					return
				}
				ppins := MustLookupPins(mcu.Get_printer())
				pinResolver := ppins.Get_pin_resolver(mcu.Get_name())
				pinResolver.Reserve_pin(pin, owner)
			},
			bus,
			pin,
			mode,
			speed,
			sw_pins,
			cs_active_high,
		),
		mcu: mcu,
	}
}

func (self *MCU_SPI) setup_shutdown_msg(shutdown_seq []int) {
	self.SetupShutdownMsg(shutdown_seq)
}

func (self *MCU_SPI) Get_oid() int {
	return self.GetOID()
}

func (self *MCU_SPI) get_mcu() *MCU {
	return self.mcu
}

func (self *MCU_SPI) get_command_queue() interface{} {
	return self.CommandQueue()
}

func (self *MCU_SPI) MCU() vibrationpkg.AccelerometerModuleMCUTransport {
	return self.mcu
}

func (self *MCU_SPI) build_config() {
	self.BuildConfig()
}

func MCU_SPI_from_config(config *ConfigWrapper, mode int, pin_option string,
	default_speed int, share_type interface{},
	cs_active_high bool) (*MCU_SPI, error) {
	ppins := MustLookupPins(config.Get_printer())
	cs_pin := cast.ToString(config.Get(pin_option, object.Sentinel{}, true))
	cs_pin_params := ppins.Lookup_pin(cs_pin, false, false, share_type)
	pin := cs_pin_params["pin"]
	if cast.ToString(pin) == "None" {
		ppins.Reset_pin_sharing(cs_pin_params)
		pin = nil
	}

	mcu := cs_pin_params["chip"]
	speed := config.Getint("spi_speed", default_speed, 100000, 0, true)
	var bus interface{}
	var sw_pins []interface{}
	if value.IsNotNone(config.Get("spi_software_sclk_pin", nil, true)) {
		var sw_pin_names []string
		var sw_pin_params []map[string]interface{}
		for _, name := range []string{"miso", "mosi", "sclk"} {
			sw_pin_names = append(sw_pin_names, fmt.Sprintf("spi_software_%s_pin", name))
		}
		for _, name := range sw_pin_names {
			tmp := ppins.Lookup_pin(cast.ToString(config.Get(name, object.Sentinel{}, true)), false, false, share_type)
			sw_pin_params = append(sw_pin_params, tmp)
		}

		for _, pin_params := range sw_pin_params {
			if pin_params["chip"] != mcu {
				return nil, fmt.Errorf("%s: spi pins must be on same mcu", config.Get_name())
			}
		}
		sw_pins = make([]interface{}, 0)
		for _, pin_params := range sw_pin_params {
			_pin, ok := pin_params["pin"].(string)
			if ok {
				sw_pins = append(sw_pins, _pin)
			} else {
				logger.Debug("pin_params[\"pin\"] type should be []string")
			}
		}
		bus = nil
	} else {
		bus = config.Get("spi_bus", value.None, true)
		sw_pins = nil
	}

	return NewMCU_SPI(mcu.(*MCU), cast.ToString(bus), pin, mode, speed, sw_pins, cs_active_high), nil
}

func newTMCErrorCheckRuntime(config *ConfigWrapper, mcuTMC tmcpkg.RegisterAccess) *tmcpkg.ErrorCheckRuntime {
	printer := config.Get_printer()
	return tmcpkg.NewDriverErrorCheckRuntime(config, mcuTMC, tmcpkg.DriverErrorCheckOptions{
		Reactor: tmcpkg.ErrorCheckReactorFuncs{
			MonotonicFunc: printer.Get_reactor().Monotonic,
			PauseFunc:     printer.Get_reactor().Pause,
			RegisterTimerFunc: func(callback func(float64) float64, waketime float64) interface{} {
				return printer.Get_reactor().Register_timer(callback, waketime)
			},
			UnregisterTimerFunc: func(timer interface{}) {
				printer.Get_reactor().Unregister_timer(timer.(*ReactorTimer))
			},
		},
		Shutdown: func(msg string) {
			printer.Invoke_shutdown(msg)
		},
		RegisterMonitor: func() {
			pheaters := printer.Load_object(config, "heaters", object.Sentinel{}).(*heaterpkg.PrinterHeaters)
			pheaters.Register_monitor(config)
		},
	})
}

func newTMCVirtualPinHelper(config *ConfigWrapper, mcuTMC tmcpkg.RegisterAccess) *tmcpkg.DriverVirtualPinHelper {
	printer := config.Get_printer()
	ppins := MustLookupPins(printer)
	reactor := printer.Get_reactor()
	return tmcpkg.NewDriverVirtualPinHelper(config, mcuTMC, tmcpkg.DriverVirtualPinHelperOptions{
		RegisterChip: ppins.Register_chip,
		RegisterEvent: func(event string, callback func([]interface{}) error) {
			printer.Register_event_handler(event, callback)
		},
		SetupPin: func(pinType string, pin string) interface{} {
			return ppins.Setup_pin(pinType, pin)
		},
		ExtractHomingMoveEndstops: func(move interface{}) []interface{} {
			hmove, ok := move.(*HomingMove)
			if !ok {
				panic(fmt.Errorf("unexpected homing move type %T", move))
			}
			return tmcHomingMoveEndstops(hmove)
		},
		ReactorPause:     reactor.Pause,
		ReactorMonotonic: reactor.Monotonic,
	})
}

func tmcHomingMoveEndstops(move *HomingMove) []interface{} {
	endstops := make([]interface{}, 0, len(move.Endstops))
	for _, namedEndstop := range move.Endstops {
		if namedEndstop.Front() == nil {
			continue
		}
		endstops = append(endstops, namedEndstop.Front().Value)
	}
	return endstops
}

type tmcDriverAdapter struct{}

type tmcSPIChainCache struct {
	mutex   *ReactorMutex
	runtime *tmcpkg.SPIChainRuntime
}

var tmcUartMutexMapLock sync.Mutex

func lookup_tmc_uart_mutex(mcu *MCU) *ReactorMutex {
	tmcUartMutexMapLock.Lock()
	defer tmcUartMutexMapLock.Unlock()

	printer := mcu.Get_printer()
	cacheObj := printer.Lookup_object("tmc_uart", nil)
	var cache *tmcpkg.UARTMutexCache
	if value.IsNone(cacheObj) {
		cache = tmcpkg.NewUARTMutexCache()
		printer.Add_object("tmc_uart", cache)
	} else {
		cache = cacheObj.(*tmcpkg.UARTMutexCache)
	}
	mutex := cache.Lookup(mcu, func() interface{} {
		m := printer.Get_reactor().Mutex(false)
		return m
	})
	return mutex.(*ReactorMutex)
}

type MCU_TMC_uart_bitbang struct {
	mcu     *MCU
	mutex   *ReactorMutex
	runtime *tmcpkg.UARTBitbangRuntime
}

func NewMCU_TMC_uart_bitbang(rx_pin_params, tx_pin_params map[string]interface{}, select_pins_desc []string) *MCU_TMC_uart_bitbang {
	self := new(MCU_TMC_uart_bitbang)
	self.mcu = rx_pin_params["chip"].(*MCU)
	self.mutex = lookup_tmc_uart_mutex(self.mcu)
	muxPins := make([]interface{}, 0, len(select_pins_desc))
	if len(select_pins_desc) != 0 {
		ppins := MustLookupPins(self.mcu.Get_printer())
		for _, spd := range select_pins_desc {
			muxPins = append(muxPins, ppins.Lookup_pin(spd, true, false, nil)["pin"])
		}
	}
	self.runtime = tmcpkg.NewUARTBitbangRuntime(
		tmcpkg.UARTBitbangOwnerFuncs{
			CreateOIDFunc:         self.mcu.Create_oid,
			AllocCommandQueueFunc: self.mcu.Alloc_command_queue,
			AddConfigCmdFunc:      self.mcu.Add_config_cmd,
			RegisterConfigCallbackFunc: func(callback func()) {
				self.mcu.Register_config_callback(callback)
			},
			LookupCommandFunc: func(msgformat string, cmdQueue interface{}) (tmcpkg.UARTMuxCommand, error) {
				return self.mcu.Lookup_command(msgformat, cmdQueue)
			},
			LookupQueryCommandFunc: func(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) tmcpkg.UARTBitbangQuery {
				return self.mcu.Lookup_query_command(msgformat, respformat, oid, cmdQueue, isAsync)
			},
			SecondsToClockFunc:   self.mcu.Seconds_to_clock,
			PrintTimeToClockFunc: self.mcu.PrintTimeToClock,
			MCUTypeFunc: func() string {
				return cast.ToString(self.mcu.Get_constants()["project.MCU"])
			},
		},
		self.mcu,
		rx_pin_params["pullup"],
		rx_pin_params["pin"],
		tx_pin_params["pin"],
		muxPins,
	)
	return self
}

func (self *MCU_TMC_uart_bitbang) register_instance(rx_pin_params, tx_pin_params map[string]interface{},
	select_pins_desc []string, addr int) ([]int64, error) {
	selectPins := make([]tmcpkg.UARTSelectPin, 0, len(select_pins_desc))
	if len(select_pins_desc) != 0 {
		ppins := MustLookupPins(self.mcu.Get_printer())
		for _, pinDesc := range select_pins_desc {
			pinParams := ppins.Parse_pin(pinDesc, true, false)
			selectPins = append(selectPins, tmcpkg.UARTSelectPin{
				Owner:  pinParams["chip"],
				Pin:    pinParams["pin"],
				Invert: cast.ToBool(pinParams["invert"]),
			})
		}
	}
	return self.runtime.RegisterInstance(rx_pin_params["pin"], tx_pin_params["pin"], selectPins, addr)
}

func (self *MCU_TMC_uart_bitbang) reg_read(instance_id []int64, addr, reg int64) interface{} {
	if val, ok := self.runtime.ReadRegister(instance_id, addr, reg); ok {
		return val
	}
	return nil
}

func (self *MCU_TMC_uart_bitbang) reg_write(instance_id []int64, addr, reg, val int64, _print_time *float64) {
	self.runtime.WriteRegister(instance_id, addr, reg, val, _print_time)
}

func Lookup_tmc_uart_bitbang(config *ConfigWrapper, max_addr int64) ([]int64, int64, *MCU_TMC_uart_bitbang, error) {
	ppins := MustLookupPins(config.Get_printer())
	rx_pin_params := ppins.Lookup_pin(cast.ToString(config.Get("uart_pin", object.Sentinel{}, true)), false, true, "tmc_uart_rx")
	tx_pin_desc := config.Get("tx_pin", value.None, true)
	var tx_pin_params map[string]interface{}
	if value.IsNone(tx_pin_desc) {
		tx_pin_params = rx_pin_params
	} else {
		tx_pin_params = ppins.Lookup_pin(cast.ToString(tx_pin_desc), false, false, "tmc_uart_tx")
	}
	if rx_pin_params["chip"] != tx_pin_params["chip"] {
		return nil, 0, nil, errors.New("TMC uart rx and tx pins must be on the same mcu")
	}
	select_pins_desc := []string{}
	if selectPins := config.Getlist("select_pins", value.None, ",", 0, true); selectPins != nil && value.IsNotNone(selectPins) {
		select_pins_desc = cast.ToStringSlice(selectPins)
	}
	addr := config.Getint("uart_address", 0, 0, cast.ForceInt(max_addr), true)

	var mcu_uart interface{}
	var ok bool
	mcu_uart, ok = rx_pin_params["class"]

	if !ok || mcu_uart == nil {
		mcu_uart = NewMCU_TMC_uart_bitbang(rx_pin_params, tx_pin_params, select_pins_desc)
		rx_pin_params["class"] = mcu_uart
	} else {
		rx_pin_params["class"] = mcu_uart
	}

	bitbang, ok := mcu_uart.(*MCU_TMC_uart_bitbang)
	if !ok {
		return nil, 0, nil, fmt.Errorf("unexpected TMC uart class type %T", mcu_uart)
	}
	instance_id, err := bitbang.register_instance(rx_pin_params, tx_pin_params, select_pins_desc, addr)
	if err != nil {
		return nil, 0, nil, err
	}
	return instance_id, int64(addr), bitbang, nil
}

func newTMC_SPI_chain_cache(config *ConfigWrapper, chain_len int64) *tmcSPIChainCache {
	self := new(tmcSPIChainCache)
	printer := config.Get_printer()
	self.mutex = printer.Get_reactor().Mutex(false)
	share := value.None
	if chain_len > 1 {
		share = "tmc_spi_cs"
	}
	spi, err := MCU_SPI_from_config(config, 3, "", 4000000, share, false)
	if err != nil {
		panic(err)
	}
	self.runtime = tmcpkg.NewSPIChainRuntime(
		chain_len,
		spi,
		func() bool {
			return value.IsNotNone(printer.Get_start_args()["debugoutput"])
		},
	)
	return self
}

func lookupTMC_SPI_chain(config *ConfigWrapper) (*tmcpkg.SPIChainRuntime, *ReactorMutex, int64) {
	_chain_len := config.GetintNone("chain_length", value.None, 2, 0, true)
	if value.IsNone(_chain_len) {
		chain := newTMC_SPI_chain_cache(config, 1)
		return chain.runtime, chain.mutex, 1
	}
	chain_len := cast.ToInt64(_chain_len)
	ppins := MustLookupPins(config.Get_printer())
	cs_pin_params := ppins.Lookup_pin(cast.ToString(config.Get("cs_pin", object.Sentinel{}, true)), false, false, "tmc_spi_cs")
	tmc_spi := cs_pin_params["class"]
	if value.IsNone(tmc_spi) {
		cs_pin_params["class"] = newTMC_SPI_chain_cache(config, chain_len)
		tmc_spi = cs_pin_params["class"]
	}
	chain, ok := tmc_spi.(*tmcSPIChainCache)
	if !ok {
		panic(fmt.Errorf("unexpected TMC SPI chain cache type %T", tmc_spi))
	}
	if chain_len != chain.runtime.ChainLen() {
		panic(errors.New("TMC SPI chain must have same length"))
	}
	chain_pos := config.Getint("chain_position", 0, 1, cast.ForceInt(chain_len), true)
	if err := chain.runtime.RegisterPosition(int64(chain_pos)); err != nil {
		panic(err)
	}
	return chain.runtime, chain.mutex, int64(chain_pos)
}

func unwrapTMCDriverConfig(config tmcpkg.DriverConfig) *ConfigWrapper {
	typed, ok := config.(*ConfigWrapper)
	if !ok {
		panic(fmt.Sprintf("unexpected TMC driver config type %T", config))
	}
	return typed
}

func (tmcDriverAdapter) NewUART(config tmcpkg.DriverConfig, nameToReg map[string]int64, fields *tmcpkg.FieldHelper, maxAddr int64, tmcFrequency float64) tmcpkg.RegisterAccess {
	cfg := unwrapTMCDriverConfig(config)
	printer := cfg.Get_printer()
	name := str.LastName(cfg.Get_name())
	instanceID, addr, mcuUART, err := Lookup_tmc_uart_bitbang(cfg, maxAddr)
	if err != nil {
		panic(err)
	}
	transport := tmcpkg.NewUARTRegisterTransportFuncs(
		func(addr, reg int64) (int64, bool) {
			value := mcuUART.reg_read(instanceID, addr, reg)
			if value == nil {
				return 0, false
			}
			return cast.ToInt64(value), true
		},
		func(addr, reg, val int64, printTime *float64) {
			mcuUART.reg_write(instanceID, addr, reg, val, printTime)
		},
	)
	return tmcpkg.NewLockedUARTRegisterAccess(
		name,
		nameToReg,
		addr,
		transport,
		fields,
		func() bool {
			return value.IsNotNone(printer.Get_start_args()["debugoutput"])
		},
		tmcpkg.NewRegisterLockerFuncs(mcuUART.mutex.Lock, mcuUART.mutex.Unlock),
	)
}

func (tmcDriverAdapter) NewSPI(config tmcpkg.DriverConfig, nameToReg map[string]int64, fields *tmcpkg.FieldHelper) tmcpkg.RegisterAccess {
	cfg := unwrapTMCDriverConfig(config)
	name := str.LastName(cfg.Get_name())
	chainRuntime, chainMutex, chainPos := lookupTMC_SPI_chain(cfg)
	return tmcpkg.NewLockedSPIRegisterAccess(name, nameToReg, chainPos, chainRuntime, fields, tmcpkg.NewRegisterLockerFuncs(chainMutex.Lock, chainMutex.Unlock))
}

func (tmcDriverAdapter) NewTMC2660SPI(config tmcpkg.DriverConfig, nameToReg map[string]int64, fields *tmcpkg.FieldHelper) tmcpkg.RegisterAccess {
	cfg := unwrapTMCDriverConfig(config)
	printer := cfg.Get_printer()
	mutex := printer.Get_reactor().Mutex(false)
	spi, err := MCU_SPI_from_config(cfg, 0, "", 4000000, "", false)
	if err != nil {
		panic(err)
	}
	return tmcpkg.NewLockedTMC2660SPIRegisterAccess(
		nameToReg,
		fields,
		spi,
		func() bool {
			return value.IsNotNone(printer.Get_start_args()["debugoutput"])
		},
		tmcpkg.NewRegisterLockerFuncs(mutex.Lock, mutex.Unlock),
	)
}

func (tmcDriverAdapter) AttachVirtualPin(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess) {
	newTMCVirtualPinHelper(unwrapTMCDriverConfig(config), mcuTMC)
}

func (tmcDriverAdapter) NewCommandHelper(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess, currentHelper tmcpkg.CurrentControl) tmcpkg.DriverCommandHelper {
	cfg := unwrapTMCDriverConfig(config)
	printer := cfg.Get_printer()
	stepperEnable := printer.Load_object(cfg, "stepper_enable", object.Sentinel{}).(*mcupkg.PrinterStepperEnableModule)
	helper := tmcpkg.NewDriverCommandHelper(config, mcuTMC, currentHelper, tmcpkg.DriverCommandHelperOptions{
		StatusChecker: newTMCErrorCheckRuntime(cfg, mcuTMC),
		StepperEnable: tmcpkg.CommandStepperEnableLookupFunc(func(name string) (tmcpkg.CommandEnableLine, error) {
			return stepperEnable.Lookup_enable(name)
		}),
		RegisterEvent: func(event string, callback func([]interface{}) error) {
			printer.Register_event_handler(event, callback)
		},
		RegisterMuxCommand: MustLookupGcode(printer).Register_mux_command,
		LookupToolhead: func() tmcpkg.CommandToolhead {
			return MustLookupToolhead(printer)
		},
		LookupStepper: func(name string) tmcpkg.CommandStepper {
			forceMove, ok := printer.Lookup_object("force_move", object.Sentinel{}).(*motionpkg.ForceMoveModule)
			if !ok {
				panic(fmt.Errorf("lookup object %s type invalid: %#v", "force_move", printer.Lookup_object("force_move", object.Sentinel{})))
			}
			return forceMove.Lookup_stepper(name)
		},
		LookupMutex: func() tmcpkg.CommandMutex {
			mutex := MustLookupGcode(printer).Mutex()
			return tmcpkg.CommandMutexFuncs{LockFunc: mutex.Lock, UnlockFunc: mutex.Unlock}
		},
		ScheduleCallback: func(callback func(interface{}) interface{}, eventtime float64) {
			printer.Get_reactor().Register_callback(callback, eventtime)
		},
		Shutdown: printer.Invoke_shutdown,
	})
	_ = tmcpkg.ApplyDriverMicrostepConfig(config, func(section string) tmcpkg.DriverConfig {
		return tmcpkg.LookupDriverSectionFromConfig[*ConfigWrapper](cfg, section)
	}, mcuTMC)
	return helper
}

func (tmcDriverAdapter) ApplyStealthchop(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess, tmcFrequency float64) {
	_ = tmcpkg.ApplyDriverStealthchopConfig(config, func(section string) tmcpkg.DriverConfig {
		return tmcpkg.LookupDriverSectionFromConfig[*ConfigWrapper](unwrapTMCDriverConfig(config), section)
	}, mcuTMC, tmcFrequency)
}

func (tmcDriverAdapter) ApplyCoolstepThreshold(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess, tmcFrequency float64) {
	_ = tmcpkg.ApplyDriverCoolstepThresholdConfig(config, func(section string) tmcpkg.DriverConfig {
		return tmcpkg.LookupDriverSectionFromConfig[*ConfigWrapper](unwrapTMCDriverConfig(config), section)
	}, mcuTMC, tmcFrequency)
}

func (tmcDriverAdapter) ApplyHighVelocityThreshold(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess, tmcFrequency float64) {
	_ = tmcpkg.ApplyDriverHighVelocityThresholdConfig(config, func(section string) tmcpkg.DriverConfig {
		return tmcpkg.LookupDriverSectionFromConfig[*ConfigWrapper](unwrapTMCDriverConfig(config), section)
	}, mcuTMC, tmcFrequency)
}

func (tmcDriverAdapter) NewTMC2660CurrentHelper(config tmcpkg.DriverConfig, mcuTMC tmcpkg.RegisterAccess) tmcpkg.CurrentControl {
	cfg := unwrapTMCDriverConfig(config)
	printer := cfg.Get_printer()
	return tmcpkg.NewTMC2660CurrentHelper(
		cfg,
		mcuTMC,
		func(event string, callback func([]interface{}) error) {
			printer.Register_event_handler(event, callback)
		},
		func(callback func(interface{}) interface{}, eventtime float64) {
			printer.Get_reactor().Register_callback(callback, eventtime)
		},
	)
}

var projectTMCDriverAdapter tmcpkg.DriverAdapter = tmcDriverAdapter{}

func (self *Printer) invoke_async_shutdown(msg string) {
	_func := func(argv interface{}) interface{} {
		self.Invoke_shutdown(msg)
		return nil
	}
	self.reactor.Register_async_callback(_func, constants.NOW)
}
func (self *Printer) Register_event_handler(event string, callback func([]interface{}) error) {
	self.runtime.RegisterEventHandler(event, callback)
}
func (self *Printer) Send_event(event string, params []interface{}) ([]interface{}, error) {
	return self.runtime.SendEvent(event, params)
}
func (self *Printer) Request_exit(result string) {
	self.runtime.RequestExit(result)
}
func ModuleKlipper() {

}

type OptionParser struct {
	Debuginput  string
	Inputtty    string
	Apiserver   string
	Logfile     string
	Verbose     bool
	Debugoutput string
	Dictionary  string
	Import_test bool
	log_level   int
}

type Klipper struct {
}

func NewKlipper() *Klipper {
	Klipper := Klipper{}

	return &Klipper
}

//######################################################################
//# Startup
//######################################################################

func (self Klipper) Main() {
	usage := "[options] <your config file>"
	flag.Usage = func() {
		fmt.Println(os.Args[0], usage)
	}
	options := OptionParser{}
	flag.StringVar(&options.Debuginput, "i", "", "read commands from file instead of from tty port")
	flag.StringVar(&options.Inputtty, "I", "/tmp/printer", "input tty name (default is /tmp/printer)")
	flag.StringVar(&options.Apiserver, "a", "/tmp/unix_uds", "api server unix domain socket filename")
	flag.StringVar(&options.Logfile, "l", "/tmp/gklib.log", "write log to file instead of stderr")
	flag.BoolVar(&options.Verbose, "v", true, "enable debug messages")
	flag.StringVar(&options.Debugoutput, "o", "", "write output to file instead of to serial port")
	flag.StringVar(&options.Dictionary, "d", "", "file to read for mcu protocol dictionary")
	flag.BoolVar(&options.Import_test, "import-test", false, "perform an import module test")
	flag.IntVar(&options.log_level, "ll", int(logger.DebugLevel), "set logger level")

	flag.Parse()
	args := flag.Args()
	if options.Import_test {
		import_test()
	}
	start_args := map[string]interface{}{}
	if len(args) != 1 {
		flag.Usage()
		start_args["config_file"] = "./printer.cfg"
	} else {
		start_args["config_file"] = args[0]
	}

	start_args["apiserver"] = options.Apiserver
	start_args["start_reason"] = "startup"

	if options.Debuginput != "" {
		start_args["debuginput"] = options.Debuginput
		debuginput, err := os.OpenFile(options.Debuginput, os.O_RDONLY, 0644)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(3)
		}
		//debuginput =io. open(options.debuginput, 'rb')
		start_args["gcode_fd"] = debuginput.Fd()
	} else {
		//start_args["gcode_fd"] = util.create_pty(options.Inputtty)
		//debuginput, err := os.OpenFile(options.Inputtty, os.O_SYNC, 0644)
		//if err != nil {
		//      logger.Error(err.Error())
		//      os.Exit(3)
		//}
		//start_args["gcode_fd"] = debuginput.Fd()
	}
	if options.Debugoutput != "" {
		start_args["debugoutput"] = options.Debugoutput
	}

	// init logger
	debuglevel := logger.DebugLevel
	if options.Logfile != "" {
		start_args["log_file"] = options.Logfile
		if options.log_level > 0 {
			debuglevel = logger.LogLevel(options.log_level)
		}
		logger.InitLogger(debuglevel,
			options.Logfile,
			logger.SUPPORT_COLOR,
			2,
			2,
			7,
		)
	}

	logger.Info("Starting Klipper...")

	start_args["software_version"] = sys.GetSoftwareVersion()
	start_args["cpu_info"] = sys.GetCpuInfo()

	logger.Infof("Args: %s, Git version: %s, CPU: %s",
		strings.Join(flag.Args(), ""),
		start_args["software_version"].(string),
		start_args["cpu_info"].(string))

	runtime.GC()

	// Start Printer() class
	var res string
	for {
		main_reactor := NewEPollReactor(true)
		printer := NewPrinter(main_reactor, start_args)
		res = printer.Run()
		if res == "exit" || res == "error_exit" {
			break
		}
		time.Sleep(time.Second)
		logger.Info("Restarting printer")
		start_args["start_reason"] = res
	}
	if res == "error_exit" {
		os.Exit(-1)
	}
	os.Exit(0)
}
func import_test() {

	os.Exit(0)
}

const (
	defaultLeviQ3ProfileName   = "leviq3"
	defaultLeviQ3TravelSpeed   = 50.0
	defaultLeviQ3VerticalSpeed = 10.0
	defaultLeviQ3SafeTravelZ   = 5.0
	cmdLeviQ3Help              = "Run LeviQ3 preheat, wipe, probe, and build-plate Z-offset recovery"
	cmdLeviQ3PreheatingHelp    = "Preheat the bed for LeviQ3 and re-check if temperature drops below target-5"
	cmdLeviQ3WipingHelp        = "Run the LeviQ3 wipe sequence"
	cmdLeviQ3ProbeHelp         = "Home and probe the LeviQ3 bed mesh"
	cmdLeviQ3AutoZOffsetHelp   = "Run LeviQ3 auto Z-offset recovery"
	cmdLeviQ3TempOffsetHelp    = "Reset LeviQ3 temperature compensation state"
	cmdLeviQ3AutoZOnOffHelp    = "Enable or disable LeviQ3 auto Z-offset"
	cmdLeviQ3SetZOffsetHelp    = "Set the LeviQ3 build-plate Z offset"
	cmdLeviQ3HelpHelp          = "Show LeviQ3 command help"
	cmdLeviQ3ScratchDebugHelp  = "Trigger the recovered LeviQ3 scratch/debug notice path"
)

type LeviQ3Module struct {
	printer      *Printer
	profileName  string
	helper       *printpkg.LeviQ3Helper
	runtime      *leviq3Runtime
	gcode        *GCodeDispatch
	gcodeMove    *gcodepkg.GCodeMoveModule
	toolhead     *Toolhead
	probe        *PrinterProbe
	bedMesh      *BedMesh
	heaters      *heaterpkg.PrinterHeaters
	saveVars     *addonpkg.SaveVariablesModule
	stateLoaded  bool
	commandState *printpkg.LeviQ3CommandState
}

type leviq3Runtime struct {
	module *LeviQ3Module
}

func NewLeviQ3Module(config *ConfigWrapper) *LeviQ3Module {
	self := &LeviQ3Module{
		printer:      config.Get_printer(),
		profileName:  defaultLeviQ3ProfileName,
		commandState: printpkg.NewLeviQ3CommandState(),
	}
	if config.Fileconfig().Has_option(config.Get_name(), "profile_name") {
		if profileName, ok := config.Get("profile_name", defaultLeviQ3ProfileName, true).(string); ok {
			trimmed := strings.TrimSpace(profileName)
			if trimmed != "" {
				self.profileName = trimmed
			}
		}
	}
	self.runtime = &leviq3Runtime{module: self}
	hasOption := func(key string) bool {
		return config != nil && config.Fileconfig().Has_option(config.Get_name(), key)
	}
	configSource := printpkg.FuncConfigSource{
		Float64Func: func(key string, fallback float64) float64 {
			if !hasOption(key) {
				return fallback
			}
			return config.Getfloat(key, fallback, 0, 0, 0, 0, true)
		},
		IntFunc: func(key string, fallback int) int {
			if !hasOption(key) {
				return fallback
			}
			return config.Getint(key, fallback, 0, 0, true)
		},
		BoolFunc: func(key string, fallback bool) bool {
			if !hasOption(key) {
				return fallback
			}
			return config.Getboolean(key, fallback, true)
		},
		Float64SliceFunc: func(key string, fallback []float64) []float64 {
			if !hasOption(key) {
				return append([]float64(nil), fallback...)
			}
			return append([]float64(nil), config.Getfloatlist(key, fallback, ",", 0, true)...)
		},
		StringFunc: func(key string, fallback string) string {
			if !hasOption(key) {
				return fallback
			}
			value, _ := config.Get(key, fallback, true).(string)
			return value
		},
	}
	status := printpkg.FuncStatusSink{
		InfofFunc: func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			logger.Infof(msg)
			if self.commandState != nil && self.commandState.IsOpen() && self.gcode != nil {
				self.gcode.Respond_info(msg, true)
			}
		},
		ErrorfFunc: func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			logger.Errorf(msg)
			if self.commandState != nil && self.commandState.IsOpen() && self.gcode != nil {
				self.gcode.Respond_info(msg, true)
			}
		},
	}
	helper, err := printpkg.NewLeviQ3Helper(configSource, self.runtime, status)
	if err != nil {
		panic(err)
	}
	helper.SetHomingRetryCount(0)
	self.helper = helper
	self.gcode = MustLookupGcode(self.printer)
	self.registerCommands()
	self.printer.Register_event_handler("project:connect", self.handleConnect)
	self.printer.Register_event_handler("project:ready", self.handleReady)
	self.printer.Register_event_handler("project:shutdown", self.handleShutdown)
	self.printer.Register_event_handler("project:disconnect", self.handleDisconnect)
	self.printer.Register_event_handler("project:pre_cancel", self.handlePreCancel)
	_ = self.printer.Webhooks().RegisterEndpoint("leviq3/cancel", func() (interface{}, error) {
		return self.handleCancelRequest(nil)
	})
	return self
}

func Load_config_LeviQ3(config *ConfigWrapper) interface{} {
	return NewLeviQ3Module(config)
}

const FADE_DISABLE = 0x7FFFFFFF

type BedMesh struct {
	Printer           *Printer
	Last_position     []float64
	Target_position   []float64
	Bmc               *BedMeshCalibrate
	Z_mesh            *bedmeshpkg.ZMesh
	Toolhead          *Toolhead
	Horizontal_move_z float64
	Fade_start        float64
	Fade_end          float64
	Fade_dist         float64
	Log_fade_complete bool
	Base_fade_target  float64
	Fade_target       float64
	Zero_ref_pos      []float64
	Gcode             *GCodeDispatch
	Splitter          *bedmeshpkg.MoveSplitter
	Pmgr              *ProfileManager
	Save_profile      func(string) error
	Status            map[string]interface{}
	Sl                lock.SpinLock
	move_transform    gcodepkg.LegacyMoveTransform
}

func NewBedMesh(config *ConfigWrapper) *BedMesh {
	self := &BedMesh{}
	self.Printer = config.Get_printer()
	self.Printer.Register_event_handler("project:connect", self.Handle_connect)
	self.Last_position = []float64{0., 0., 0., 0.}
	self.Bmc = NewBedMeshCalibrate(config, self)
	self.Toolhead = nil
	self.Z_mesh = nil
	self.Zero_ref_pos = nil
	self.Horizontal_move_z = config.Getfloat("horizontal_move_z", 5., 0, 0, 0, 0, true)
	self.Fade_start = config.Getfloat("fade_start", 1., 0, 0, 0, 0, true)
	self.Fade_end = config.Getfloat("fade_end", 0., 0, 0, 0, 0, true)
	self.Fade_dist = self.Fade_end - self.Fade_start
	if self.Fade_dist <= 0. {
		self.Fade_start, self.Fade_end = FADE_DISABLE, FADE_DISABLE
	}
	self.Log_fade_complete = false
	self.Base_fade_target = config.Getfloat("fade_target", 0., 0, 0, 0, 0, true)
	self.Fade_target = 0.
	gcode_obj := self.Printer.Lookup_object("gcode", object.Sentinel{})
	self.Gcode = gcode_obj.(*GCodeDispatch)
	self.Splitter = bedmeshpkg.NewMoveSplitter(
		config.Getfloat("split_delta_z", .025, 0.01, 0, 0, 0, true),
		config.Getfloat("move_check_distance", 5., 1., 0, 0, 0, true),
	)
	self.Pmgr = NewProfileManager(config, self)
	self.Save_profile = self.Pmgr.Save_profile
	self.Gcode.Register_command(
		"BED_MESH_OUTPUT", self.Cmd_BED_MESH_OUTPUT,
		false, cmd_BED_MESH_OUTPUT_help)
	self.Gcode.Register_command("BED_MESH_MAP", self.Cmd_BED_MESH_MAP,
		false, cmd_BED_MESH_MAP_help)
	self.Gcode.Register_command(
		"BED_MESH_CLEAR", self.Cmd_BED_MESH_CLEAR,
		false, cmd_BED_MESH_CLEAR_help)
	self.Gcode.Register_command(
		"BED_MESH_OFFSET", self.Cmd_BED_MESH_OFFSET,
		false, cmd_BED_MESH_OFFSET_help)
	gcode_move := self.Printer.Load_object(config, "gcode_move", object.Sentinel{})
	gcode_move.(*gcodepkg.GCodeMoveModule).Set_move_transform(self, false)
	self.Update_status()
	return self
}

func (self *BedMesh) Handle_connect(event []interface{}) error {
	toolhead_obj := self.Printer.Lookup_object("toolhead", object.Sentinel{})
	self.Toolhead = toolhead_obj.(*Toolhead)
	_ = event
	return nil
}

func (self *BedMesh) Set_mesh(mesh *bedmeshpkg.ZMesh) {
	fadeTarget, logFadeComplete, err := bedmeshpkg.ResolveFadeTarget(
		mesh,
		self.Fade_end != FADE_DISABLE,
		self.Fade_dist,
		self.Base_fade_target,
	)
	if err != nil {
		self.Z_mesh = nil
		self.Fade_target = 0.0
		panic(err.Error())
	}
	self.Log_fade_complete = logFadeComplete
	self.Fade_target = fadeTarget
	self.Z_mesh = mesh
	self.Splitter.Initialize(mesh, self.Fade_target)
	gcode_move := self.Printer.Lookup_object("gcode_move", object.Sentinel{})
	gcode_move.(*gcodepkg.GCodeMoveModule).Reset_last_position(nil)
	self.Update_status()
}

func (self *BedMesh) ApplyRecoveredMesh(recovered *printpkg.BedMesh, meshParams map[string]interface{}, profileName string) error {
	if recovered == nil {
		return nil
	}
	zMesh, err := bedmeshpkg.RestoreZMeshFromProfile(
		bedmeshpkg.NewProfileRecord(recovered.CloneMatrix(), meshParams),
	)
	if err != nil {
		return err
	}
	self.Set_mesh(zMesh)
	if strings.TrimSpace(profileName) != "" {
		return self.Save_profile(profileName)
	}
	return nil
}

func (self *BedMesh) Get_z_factor(z_pos float64) float64 {
	return bedmeshpkg.CalcZFadeFactor(z_pos, self.Fade_start, self.Fade_end)
}

func (self *BedMesh) Get_position() []float64 {
	self.Last_position = bedmeshpkg.CalculateUntransformedPosition(
		self.Toolhead.Get_position(),
		self.Z_mesh,
		self.Fade_start,
		self.Fade_end,
		self.Fade_dist,
		self.Fade_target,
	)
	last_position := make([]float64, len(self.Last_position))
	copy(last_position, self.Last_position)
	return last_position
}

func (self *BedMesh) Move(newpos []float64, speed float64) {
	target_position := make([]float64, len(newpos))
	copy(target_position, newpos)
	self.Target_position = target_position
	factor := self.Get_z_factor(newpos[2])
	if self.Z_mesh == nil || factor == 0. {
		x := newpos[0]
		y := newpos[1]
		z := newpos[2]
		e := newpos[3]
		if self.Log_fade_complete {
			self.Log_fade_complete = false
			logger.Debugf("bed_mesh fade complete: Current Z: %.4f fade_target: %.4f ", z, self.Fade_target)
		}
		self.Toolhead.Move([]float64{x, y, z + self.Fade_target, e}, speed)
	} else {
		self.Splitter.Build_move(self.Last_position, newpos, factor)
		for !self.Splitter.Traverse_complete {
			split_move := self.Splitter.Split()
			if len(split_move) > 0 {
				self.Toolhead.Move(split_move, speed)
			} else {
				panic("Mesh Leveling: Error splitting move ")
			}
		}

	}
	self.Last_position = append([]float64{}, newpos...)
}

func (self *BedMesh) Get_status(eventtime float64) map[string]interface{} {
	_ = eventtime
	return sys.DeepCopyMap(self.Status)
}

func (self *BedMesh) Update_status() {
	self.Status = self.Pmgr.State.BuildStatus(self.Z_mesh).AsMap()
}

func (self *BedMesh) Get_mesh() *bedmeshpkg.ZMesh {
	return self.Z_mesh
}

const cmd_BED_MESH_OUTPUT_help = "Retrieve interpolated grid of probed z-points"

func (self *BedMesh) Cmd_BED_MESH_OUTPUT(gcmd *GCodeCommand) {
	if gcmd.Get_int("PGP", 0, nil, nil) > 0 {
		self.Bmc.Print_generated_points(gcmd.Respond_info)
	} else if self.Z_mesh == nil {
		gcmd.Respond_info("Bed has not been probed", true)
	} else {
		self.Z_mesh.Print_probed_matrix(gcmd.Respond_info)
		horizontal_move_z := int(self.Horizontal_move_z)
		self.Z_mesh.Print_mesh(logger.Debug, &horizontal_move_z)
	}
}

const cmd_BED_MESH_MAP_help = "Serialize mesh and output to terminal"

func (self *BedMesh) Cmd_BED_MESH_MAP(gcmd *GCodeCommand) error {
	if self.Z_mesh != nil {
		jsonStr, _ := bedmeshpkg.MarshalMeshMap(self.Z_mesh)
		gcmd.Respond_raw("mesh_map_output " + string(jsonStr))
	} else {
		gcmd.Respond_info("Bed has not been probed", true)
	}
	return nil
}

const cmd_BED_MESH_CLEAR_help = "Clear the Mesh so no z-adjustment is made"

func (self *BedMesh) Cmd_BED_MESH_CLEAR(gcmd interface{}) error {
	_ = gcmd
	self.Set_mesh(nil)
	return nil
}

const cmd_BED_MESH_OFFSET_help = "Add X/Y offsets to the mesh lookup"

func (self *BedMesh) Cmd_BED_MESH_OFFSET(gcmd *GCodeCommand) error {
	if self.Z_mesh != nil {
		offsets := make([]float64, 2)
		for i, Axis := range []string{"x", "y"} {
			offsets[i] = gcmd.Get_float(Axis, nil, nil, nil, nil, nil)
		}
		self.Z_mesh.Set_mesh_offsets(offsets)
		gcode_move_obj := self.Printer.Lookup_object("gcode_move", object.Sentinel{})
		gcode_move := gcode_move_obj.(*gcodepkg.GCodeMoveModule)
		gcode_move.Reset_last_position(nil)
	} else {
		gcmd.Respond_info("No mesh loaded to offset", true)
	}
	return nil
}

type BedMeshCalibrate struct {
	Printer               *Printer
	Bedmesh               *BedMesh
	State                 *bedmeshpkg.CalibrationState
	Profile_name          string
	Probe_helper          *probepkg.ProbePointsHelper
	Gcode                 *GCodeDispatch
	config                *ConfigWrapper
	adaptive_margin       float64
	min_bedmesh_area_size []float64
}

type bedMeshManualProbeCommand struct{}

func (self *bedMeshManualProbeCommand) Get(name string, defaultValue interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
	_ = name
	_ = parser
	_ = minval
	_ = maxval
	_ = above
	_ = below
	return cast.ToString(defaultValue)
}

func (self *bedMeshManualProbeCommand) Get_int(name string, defaultValue interface{}, minval *int, maxval *int) int {
	_ = name
	_ = minval
	_ = maxval
	return cast.ToInt(defaultValue)
}

func (self *bedMeshManualProbeCommand) Get_float(name string, defaultValue interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	_ = name
	_ = minval
	_ = maxval
	_ = above
	_ = below
	return cast.ToFloat64(defaultValue)
}

func (self *bedMeshManualProbeCommand) RespondInfo(msg string, log bool) {
	_ = msg
	_ = log
}

func loadBedMeshProbePoints(config *ConfigWrapper, defaultPoints [][]float64) [][]float64 {
	probePoints := defaultPoints
	if len(defaultPoints) == 0 || config.Get("points", value.None, true) != nil {
		probePoints = config.Getlists("points", nil, []string{","}, 2, reflect.Float64, true).([][]float64)
	}
	return probePoints
}

func NewBedMeshCalibrate(config *ConfigWrapper, bedmesh *BedMesh) *BedMeshCalibrate {
	self := &BedMeshCalibrate{}
	self.config = config
	self.Printer = config.Get_printer()
	self.adaptive_margin = config.Getfloat("adaptive_margin", 0.0, 0., 0., 0., 0., true)
	self.Bedmesh = bedmesh
	self.min_bedmesh_area_size = config.Getfloatlist("min_bedmesh_area_size", []float64{100.0, 100.0}, ",", 2, true)
	state, err := bedmeshpkg.NewCalibrationState(config, get_relative_reference_index(config))
	if err != nil {
		panic(err)
	}
	self.State = state
	self.Profile_name = ""
	self.Probe_helper = probepkg.NewProbePointsHelper(
		config.Get_name(),
		self.Probe_finalize,
		loadBedMeshProbePoints(config, self.State.AdjustedPoints()),
		config.Getfloat("horizontal_move_z", 5., 0, 0, 0, 0, true),
		config.Getfloat("speed", 50., 0, 0, 0., 0, true),
	)
	self.ensureMinimumProbePoints(3)
	self.Probe_helper.UseXYOffsets(true)
	gcode_obj := self.Printer.Lookup_object("gcode", object.Sentinel{})
	self.Gcode = gcode_obj.(*GCodeDispatch)
	self.Gcode.Register_command(
		"BED_MESH_CALIBRATE", self.Cmd_BED_MESH_CALIBRATE,
		false, cmd_BED_MESH_CALIBRATE_help)
	return self
}

func (self *BedMeshCalibrate) ensureMinimumProbePoints(n int) {
	if !self.Probe_helper.MinimumPoints(n) {
		panic(fmt.Sprintf("Need at least %d probe points for %s", n, self.Probe_helper.Name()))
	}
}

func (self *BedMeshCalibrate) updateProbePoints(points [][]float64, minPoints int) {
	self.Probe_helper.UpdateProbePoints(points)
	self.ensureMinimumProbePoints(minPoints)
}

func (self *BedMeshCalibrate) EnsureNoManualProbe() {
	self.Printer.Lookup_object("manual_probe", object.Sentinel{}).(*ManualProbe).EnsureNoManualProbe()
}

func (self *BedMeshCalibrate) LookupAutomaticProbe() probepkg.ProbePointsAutomaticProbe {
	probeObj := self.Printer.Lookup_object("probe", object.Sentinel{})
	probe, ok := probeObj.(*PrinterProbe)
	if !ok {
		return nil
	}
	return probe
}

func (self *BedMeshCalibrate) Move(coord interface{}, speed float64) {
	MustLookupToolhead(self.Printer).Manual_move(coord.([]interface{}), speed)
}

func (self *BedMeshCalibrate) TouchLastMoveTime() {
	MustLookupToolhead(self.Printer).Get_last_move_time()
}

func (self *BedMeshCalibrate) StartManualProbe(finalize func([]float64)) {
	self.Printer.Lookup_object("manual_probe", object.Sentinel{}).(*ManualProbe).StartManualProbe(&bedMeshManualProbeCommand{}, finalize)
}

func get_relative_reference_index(config *ConfigWrapper) *int {
	if !config.Fileconfig().Has_option(config.Section, "relative_reference_index") {
		return nil
	} else {
		v := config.Fileconfig().Getint(config.Section, "relative_reference_index")
		relative_reference_index := v.(int)
		return &relative_reference_index
	}
}

func (self *BedMeshCalibrate) Print_generated_points(print_func func(msg string, log bool)) {
	xOffset, yOffset := 0., 0.
	probe_obj := self.Printer.Lookup_object("probe", nil)
	if probe_obj != nil {
		xOffset, yOffset, _ = probe_obj.(*PrinterProbe).Get_offsets()
	}
	print_func("bed_mesh: generated points\nIndex| Tool Adjusted | Probe", true)
	for i, v := range self.State.Points {
		adjPt := fmt.Sprintf("(%.1f, %.1f)", v[0]-xOffset, v[1]-yOffset)
		meshPt := fmt.Sprintf("(%.1f, %.1f)", v[0], v[1])
		print_func(fmt.Sprintf("%-4d| %-16s | %s", i, adjPt, meshPt), true)
	}
	if self.State.RelativeReferenceIndex != nil {
		rri := *self.State.RelativeReferenceIndex
		pt := self.State.Points[rri]
		print_func(fmt.Sprintf("bed_mesh: relative_reference_index %d is (%.2f, %.2f)", rri, pt[0], pt[1]), true)
	}
	if len(self.State.Substitutions) != 0 {
		print_func("bed_mesh: faulty region points", true)
		for _, substitution := range self.State.Substitutions {
			pt := self.State.Points[substitution.Index]
			print_func(fmt.Sprintf("%d (%.2f, %.2f), substituted points: %v", substitution.Index, pt[0], pt[1], substitution.Points), true)
		}
	}
}

func (self *BedMeshCalibrate) set_adaptive_mesh(gcmd *GCodeCommand) bool {
	if gcmd.Get_int("ADAPTIVE", 0, nil, nil) == 0 {
		return false
	}
	exclude_objects := self.Printer.Lookup_object("exclude_object", nil).(*addonpkg.ExcludeObjectModule)
	if exclude_objects == nil {
		gcmd.Respond_info("Exclude objects not enabled. Using full mesh...", true)
		return false
	}
	objects := exclude_objects.Get_status(0)["objects"]
	if objects == nil {
		return false
	}
	objectPolygons, ok := objects.([]map[string]interface{})
	if !ok || len(objectPolygons) == 0 {
		logger.Debugf("adaptive_bed_mesh: unexpected exclude object payload %T", objects)
		return false
	}
	baseXCount := self.State.MeshConfig["x_count"].(int)
	baseYCount := self.State.MeshConfig["y_count"].(int)

	margin := gcmd.Get_float("ADAPTIVE_MARGIN", self.adaptive_margin, nil, nil, nil, nil)
	gcmd.Respond_info(fmt.Sprintf("Found %d objects", len(objectPolygons)), true)
	meshMin, meshMax, err := bedmeshpkg.ExtractExcludeObjectBounds(objectPolygons)
	if err != nil {
		logger.Debugf("adaptive_bed_mesh: unable to derive exclude-object bounds: %v", err)
		return false
	}
	minMeshSize := bedmeshpkg.Vec2{X: self.min_bedmesh_area_size[0], Y: self.min_bedmesh_area_size[1]}
	origMeshMin := self.State.OriginalMeshMin()
	origMeshMax := self.State.OriginalMeshMax()
	plan, err := bedmeshpkg.BuildAdaptiveMeshLayout(meshMin, meshMax, bedmeshpkg.AdaptiveMeshLayoutConfig{
		Margin:            margin,
		DefaultMin:        bedmeshpkg.Vec2{X: origMeshMin[0], Y: origMeshMin[1]},
		DefaultMax:        bedmeshpkg.Vec2{X: origMeshMax[0], Y: origMeshMax[1]},
		BaseXCount:        baseXCount,
		BaseYCount:        baseYCount,
		MinimumProbeCount: 3,
		MinimumMeshSize:   &minMeshSize,
		Algorithm:         self.State.MeshConfig["algo"].(string),
		BedRadius:         self.State.Radius,
	})
	if err != nil {
		logger.Debugf("adaptive_bed_mesh: unable to build adaptive mesh layout: %v", err)
		return false
	}
	logger.Debugf("Original mesh bounds: (%v,%v)", origMeshMin, origMeshMax)
	logger.Debugf("Original probe count: (%d,%d)", baseXCount, baseYCount)
	logger.Debugf("Adapted mesh bounds: ([%.3f %.3f],[%.3f %.3f])", plan.MeshMin.X, plan.MeshMin.Y, plan.MeshMax.X, plan.MeshMax.Y)
	logger.Debugf("Ratio: (%.4f, %.4f)", plan.RatioX, plan.RatioY)

	self.State.ApplyAdaptiveLayout(plan)
	gcmd.Respond_info(fmt.Sprintf("Adapted probe count: (%v,%v)", plan.XCount, plan.YCount), true)
	self.Profile_name = plan.ProfileName

	return true
}

func (self *BedMeshCalibrate) Update_config(gcmd *GCodeCommand) {
	self.State.ResetToOriginal()
	need_cfg_update, err := self.State.ApplyCommandOverrides(gcmd)
	if err != nil {
		panic(err.Error())
	}
	need_mesh_update := self.set_adaptive_mesh(gcmd)
	if need_cfg_update || need_mesh_update {
		refresh, err := self.State.Refresh(self.Bedmesh.Zero_ref_pos)
		if err != nil {
			panic(err.Error())
		}
		if refresh.ForcedLagrange {
			logger.Debugf(
				"bed_mesh: bicubic interpolation with a probe_count of less than 4 points detected.  Forcing lagrange interpolation. Configured Probe Count: %d, %d",
				self.State.MeshConfig["x_count"], self.State.MeshConfig["y_count"],
			)
		}
		if refresh.ZeroReferenceAttempted {
			if !refresh.ZeroReferenceMatched {
				logger.Debugf("adaptive_bed_mesh: zero reference position (%.3f, %.3f) not found in generated points", self.Bedmesh.Zero_ref_pos[0], self.Bedmesh.Zero_ref_pos[1])
			}
			self.Bedmesh.Zero_ref_pos = nil
		}
		self.updateProbePoints(refresh.AdjustedPoints, 3)
		var mesh_config_str_arr []string
		for key, item := range self.State.MeshConfig {
			mesh_config_str_arr = append(mesh_config_str_arr,
				fmt.Sprintf("%s: %v", key, item))
		}
		msg := strings.Join(mesh_config_str_arr, "\n")
		logger.Debugf("Updated Mesh Configuration:" + msg)
	} else {
		self.updateProbePoints(self.State.AdjustedPoints(), 3)
	}
}

const cmd_BED_MESH_CALIBRATE_help = "Perform Mesh Bed Leveling"

func (self *BedMeshCalibrate) Cmd_BED_MESH_CALIBRATE(gcmd interface{}) error {
	self.Profile_name = gcmd.(*GCodeCommand).Get("PROFILE", "default", "", nil, nil, nil, nil)
	if strings.TrimSpace(self.Profile_name) == "" {
		panic("Value for parameter 'PROFILE' must be specified")
	}
	self.Bedmesh.Set_mesh(nil)
	self.Update_config(gcmd.(*GCodeCommand))
	self.Probe_helper.StartProbe(self, gcmd.(*GCodeCommand))
	return nil
}

func (self *BedMeshCalibrate) Probe_finalize(offsets []float64, positions [][]float64) string {
	result, err := bedmeshpkg.FinalizeCalibration(offsets, positions, bedmeshpkg.CalibrationFinalizeConfig{
		MeshConfig:             self.State.MeshConfig,
		RelativeReferenceIndex: self.State.RelativeReferenceIndex,
		Radius:                 self.State.Radius,
		GeneratedPoints:        self.State.Points,
		Substitutions:          self.State.Substitutions,
	})
	if err != nil {
		if len(self.State.Substitutions) > 0 && len(result.CorrectedPositions) > 0 {
			self.Dump_points(positions, result.CorrectedPositions, offsets)
		}
		panic(err.Error())
	}

	z_mesh := bedmeshpkg.NewZMesh(result.MeshParams)
	z_mesh.Build_mesh(result.ProbedMatrix)
	self.Bedmesh.Set_mesh(z_mesh)
	logger.Debug("Mesh Bed Leveling Complete")
	self.Bedmesh.Save_profile(self.Profile_name)
	return ""
}

func (self *BedMeshCalibrate) Dump_points(probed_pts [][]float64, corrected_pts [][]float64, offsets []float64) {
	for _, line := range bedmeshpkg.FormatPointDebugLines(self.State.Points, probed_pts, corrected_pts, offsets) {
		logger.Debugf(line)
	}
}

type ProfileManager struct {
	Name    string
	Printer *Printer
	Gcode   *GCodeDispatch
	Bedmesh *BedMesh
	State   *bedmeshpkg.ProfileManager
}

func NewProfileManager(config *ConfigWrapper, bedmesh *BedMesh) *ProfileManager {
	self := &ProfileManager{}
	self.Name = config.Get_name()
	self.Printer = config.Get_printer()
	gcode_obj := self.Printer.Lookup_object("gcode", object.Sentinel{})
	self.Gcode = gcode_obj.(*GCodeDispatch)
	self.Bedmesh = bedmesh
	stored_profs := config.Get_prefix_sections(self.Name)
	stored_profs_back := []*ConfigWrapper{}
	for _, s := range stored_profs {
		if s.Get_name() != self.Name {
			stored_profs_back = append(stored_profs_back, s)
		}
	}
	stored_profs = stored_profs_back
	storedInputs := make([]bedmeshpkg.StoredProfileInput, 0, len(stored_profs))
	for _, profile := range stored_profs {
		name := strings.Join(strings.Split(profile.Get_name(), " ")[1:], "")
		version := profile.Getint("version", 0, 0, 0, true)
		zvals := profile.Getlists("points", nil, []string{",", "\n"}, 0, reflect.Float64, true)
		params := map[string]interface{}{}
		for key, t := range bedmeshpkg.ProfileOptionKinds {
			if t == reflect.Int {
				params[key] = profile.Getint(key, object.Sentinel{}, 0, 0, true)
			} else if t == reflect.Float64 {
				params[key] = profile.Getfloat(key, object.Sentinel{}, 0, 0, 0, 0, true)
			} else if t == reflect.String {
				params[key] = profile.Get(key, object.Sentinel{}, true)
			}
		}
		storedInputs = append(storedInputs, bedmeshpkg.StoredProfileInput{
			Name:       name,
			Version:    version,
			PointsData: zvals,
			MeshParams: params,
		})
	}
	self.State = bedmeshpkg.NewProfileManager(storedInputs)
	for _, profile := range self.State.IncompatibleProfiles() {
		logger.Errorf("bed_mesh: Profile [%s] not compatible with this version\n"+
			"of bed_mesh.  Profile Version: %d Current Version: %d ",
			profile.Name, profile.Version, bedmeshpkg.ProfileVersion)
	}
	self.Gcode.Register_command("BED_MESH_PROFILE", self.Cmd_BED_MESH_PROFILE,
		false,
		cmd_BED_MESH_PROFILE_help)
	return self
}

func (self *ProfileManager) Save_profile(prof_name string) error {
	record, err := self.State.SaveProfile(prof_name, self.Bedmesh.Get_mesh())
	if errors.Is(err, bedmeshpkg.ErrProfileSaveWithoutMesh) {
		self.Gcode.Respond_info(fmt.Sprintf("Unable to save to profile [%s], the bed has not been probed",
			prof_name), true)
		return nil
	}
	if err != nil {
		return err
	}
	configfile_obj := self.Printer.Lookup_object("configfile", object.Sentinel{})
	configfile := configfile_obj.(*PrinterConfig)
	cfg_name := self.Name + " " + prof_name
	for key, value := range record.ConfigValues() {
		configfile.Set(cfg_name, key, value)
	}
	self.Bedmesh.Update_status()
	logger.Debugf("Bed Mesh state has been saved to profile [%s]\n"+
		"for the current session.  The SAVE_CONFIG command will\n"+
		"update the printer config file and restart the printer.",
		prof_name)
	return nil
}

func (self *ProfileManager) Load_profile(prof_name string) error {
	z_mesh, err := self.State.LoadProfile(prof_name)
	if errors.Is(err, bedmeshpkg.ErrUnknownProfile) {
		logger.Errorf("bed_mesh: Unknown profile [%s]", prof_name)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	self.Bedmesh.Set_mesh(z_mesh)
	return nil
}

func (self *ProfileManager) Remove_profile(prof_name string) error {
	if self.State.RemoveProfile(prof_name) {
		configfile_obj := self.Printer.Lookup_object("configfile", object.Sentinel{})
		configfile := configfile_obj.(*PrinterConfig)
		configfile.Remove_section("bed_mesh " + prof_name)
		self.Bedmesh.Update_status()
		self.Gcode.Respond_info(fmt.Sprintf(
			"Profile [%s] removed from storage for this session.\n"+
				"The SAVE_CONFIG command will update the printer\n"+
				"configuration and restart the printer", prof_name), true)
	} else {
		self.Gcode.Respond_info(fmt.Sprintf(
			"No profile named [%s] to remove", prof_name), true)

	}
	return nil
}

const cmd_BED_MESH_PROFILE_help = "Bed Mesh Persistent Storage management"

func (self *ProfileManager) Cmd_BED_MESH_PROFILE(arg interface{}) error {
	gcmd := arg.(*GCodeCommand)
	options := map[string]func(string) error{
		"LOAD":   self.Load_profile,
		"SAVE":   self.Save_profile,
		"REMOVE": self.Remove_profile,
	}
	for key := range options {
		name := gcmd.Get(key, nil, "", nil, nil, nil, nil)
		if name != "" {
			if strings.TrimSpace(name) == "" {
				panic(fmt.Sprintf("Value for parameter '%s' must be specified", key))
			}
			if name == "default" && key == "SAVE" {
				gcmd.Respond_info(
					"Profile 'default' is reserved, please choose"+
						" another profile name.", true)
			} else {
				options[key](name)
				return nil
			}
		}
	}
	gcmd.Respond_info(fmt.Sprintf("Invalid syntax '%s'", gcmd.Commandline), true)
	return nil
}

func Load_config_bed_mesh(config *ConfigWrapper) interface{} {
	return NewBedMesh(config)
}

func (self *LeviQ3Module) registerCommand(name string, ready bool, help string, handler func(*GCodeCommand) error) {
	self.gcode.Register_command(name, func(arg interface{}) error {
		gcmd, _ := arg.(*GCodeCommand)
		return handler(gcmd)
	}, ready, help)
}

func (self *LeviQ3Module) registerCommands() {
	self.registerCommand("LEVIQ3", false, cmdLeviQ3Help, func(*GCodeCommand) error {
		return self.runLeviQ3Command(func(ctx context.Context) error {
			_, err := self.helper.CMD_LEVIQ3(ctx)
			return err
		})
	})
	self.registerCommand("LEVIQ3_PREHEATING", false, cmdLeviQ3PreheatingHelp, func(gcmd *GCodeCommand) error {
		mode := "enable"
		if enabled, specified := parseEnableState(gcmd, true); specified && !enabled {
			mode = "disable"
		}
		return self.runLeviQ3Command(func(ctx context.Context) error {
			return self.helper.CMD_LEVIQ3_PREHEATING(ctx, mode)
		})
	})
	self.registerCommand("LEVIQ3_WIPING", false, cmdLeviQ3WipingHelp, func(*GCodeCommand) error {
		return self.runLeviQ3Command(func(ctx context.Context) error {
			return self.helper.CMD_LEVIQ3_WIPING(ctx)
		})
	})
	self.registerCommand("LEVIQ3_PROBE", false, cmdLeviQ3ProbeHelp, func(*GCodeCommand) error {
		return self.runLeviQ3Command(func(ctx context.Context) error {
			_, err := self.helper.CMD_LEVIQ3_PROBE(ctx)
			return err
		})
	})
	self.registerCommand("LEVIQ3_AUTO_ZOFFSET", false, cmdLeviQ3AutoZOffsetHelp, func(*GCodeCommand) error {
		return self.runLeviQ3Command(func(ctx context.Context) error {
			return self.helper.CMD_LEVIQ3_auto_zoffset(ctx)
		})
	})
	self.registerCommand("LEVIQ3_AUTO_ZOFFSET_ON_OFF", false, cmdLeviQ3AutoZOnOffHelp, func(gcmd *GCodeCommand) error {
		enabled, _ := parseEnableState(gcmd, true)
		return self.runLeviQ3Command(func(context.Context) error {
			self.helper.CMD_LEVIQ3_auto_zoffset_ON_OFF(enabled)
			return nil
		})
	})
	self.registerCommand("LEVIQ3_TEMP_OFFSET", false, cmdLeviQ3TempOffsetHelp, func(*GCodeCommand) error {
		return self.runLeviQ3Command(func(ctx context.Context) error {
			return self.helper.CMD_LEVIQ3_TEMP_OFFSET(ctx)
		})
	})
	self.registerCommand("LEVIQ3_SET_ZOFFSET", false, cmdLeviQ3SetZOffsetHelp, func(gcmd *GCodeCommand) error {
		z := self.helper.CurrentZOffset()
		if gcmd != nil {
			z = gcmd.Get_float("Z", z, nil, nil, nil, nil)
		}
		return self.runLeviQ3Command(func(ctx context.Context) error {
			return self.helper.LEVIQ3_set_zoffset(ctx, z)
		})
	})
	self.registerCommand("LEVIQ3_HELP", true, cmdLeviQ3HelpHelp, func(gcmd *GCodeCommand) error {
		helpText := "LEVIQ3 helper unavailable"
		if self.helper != nil {
			helpText = self.helper.CMD_LEVIQ3_HELP()
		}
		if gcmd != nil {
			gcmd.Respond_info(helpText, true)
		}
		return nil
	})
	self.registerCommand("G9113", false, cmdLeviQ3ScratchDebugHelp, func(*GCodeCommand) error {
		return self.runLeviQ3Command(func(ctx context.Context) error {
			return self.helper.CMD_G9113(ctx)
		})
	})
}

func (self *LeviQ3Module) handleConnect([]interface{}) error {
	if err := self.refreshRuntimeObjects(); err != nil {
		return err
	}
	if self.stateLoaded {
		return nil
	}
	if err := self.helper.RestorePersistentStateVariable(self.saveVarsVariables(), printpkg.LeviQ3PersistentStateVariable); err != nil {
		return err
	}
	self.stateLoaded = true
	return nil
}

func (self *LeviQ3Module) handleReady([]interface{}) error {
	if err := self.refreshRuntimeObjects(); err != nil {
		return err
	}
	return self.applyRuntimeZOffset(self.helper.CurrentZOffset())
}

func (self *LeviQ3Module) handleShutdown([]interface{}) error {
	self.requestCancel("leviq3 cancelled by printer shutdown")
	return nil
}

func (self *LeviQ3Module) handleDisconnect([]interface{}) error {
	self.requestCancel("leviq3 cancelled by printer disconnect")
	return nil
}

func (self *LeviQ3Module) handlePreCancel([]interface{}) error {
	self.requestCancel("leviq3 cancelled by CANCEL_PRINT")
	return nil
}

func (self *LeviQ3Module) handleCancelRequest(_ *WebRequest) (interface{}, error) {
	self.requestCancel("leviq3 cancelled by webhook request")
	return map[string]interface{}{"status": "cancelled"}, nil
}

func (self *LeviQ3Module) refreshRuntimeObjects() error {
	self.gcode = MustLookupGcode(self.printer)
	self.toolhead = MustLookupToolhead(self.printer)
	self.gcodeMove = MustLookupGCodeMove(self.printer)
	heaters, ok := self.printer.Lookup_object("heaters", object.Sentinel{}).(*heaterpkg.PrinterHeaters)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "heaters", self.printer.Lookup_object("heaters", object.Sentinel{})))
	}
	self.heaters = heaters
	probeObj := self.printer.Lookup_object("probe", nil)
	if probeObj == nil {
		return fmt.Errorf("LEVIQ3 requires [probe] to be configured")
	}
	probe, ok := probeObj.(*PrinterProbe)
	if !ok || probe == nil {
		return fmt.Errorf("LEVIQ3 requires a native PrinterProbe, got %T", probeObj)
	}
	self.probe = probe
	bedMeshObj := self.printer.Lookup_object("bed_mesh", nil)
	if bedMeshObj == nil {
		return fmt.Errorf("LEVIQ3 requires [bed_mesh] to be configured")
	}
	bedMesh, ok := bedMeshObj.(*BedMesh)
	if !ok || bedMesh == nil {
		return fmt.Errorf("LEVIQ3 requires a native BedMesh, got %T", bedMeshObj)
	}
	self.bedMesh = bedMesh
	if self.printer.Lookup_object("heater_bed", nil) == nil {
		return fmt.Errorf("LEVIQ3 requires [heater_bed] to be configured")
	}
	if saveObj := self.printer.Lookup_object("save_variables", nil); saveObj != nil {
		if saveVars, ok := saveObj.(*addonpkg.SaveVariablesModule); ok {
			self.saveVars = saveVars
		}
	}
	return nil
}

func (self *LeviQ3Module) beginCommand() error {
	if err := self.refreshRuntimeObjects(); err != nil {
		return err
	}
	self.helper.ResetCancelState()
	self.commandState.Begin()
	return nil
}

func (self *LeviQ3Module) finishCommand() {
	if self.commandState != nil {
		self.commandState.Finish()
	}
}

func (self *LeviQ3Module) requestCancel(reason string) {
	if self.commandState != nil {
		reason = self.commandState.NoteCancellation(reason)
	}
	if self.helper != nil {
		self.helper.CancelEvent(reason)
	}
}

func (self *LeviQ3Module) ensureNotCancelled(stage string) error {
	if self.commandState == nil {
		if self.printer.Is_shutdown() {
			return fmt.Errorf("%s: printer shutdown", stage)
		}
		return nil
	}
	return self.commandState.EnsureActive(stage, self.printer.Is_shutdown())
}

func (self *LeviQ3Module) runLeviQ3Command(fn func(context.Context) error) error {
	if err := self.beginCommand(); err != nil {
		return err
	}
	defer self.finishCommand()
	if err := fn(context.Background()); err != nil {
		return err
	}
	return self.persistState()
}

func (self *LeviQ3Module) persistState() error {
	if self.saveVars == nil || self.gcode == nil || self.helper == nil {
		return nil
	}
	command, err := self.helper.PersistentStateSaveVariableCommand(printpkg.LeviQ3PersistentStateVariable)
	if err != nil {
		return err
	}
	self.gcode.Run_script_from_command(command)
	return nil
}

func (self *LeviQ3Module) saveVarsVariables() map[string]interface{} {
	if self.saveVars == nil {
		return nil
	}
	return self.saveVars.Variables()
}

func (self *LeviQ3Module) applyRuntimeZOffset(z float64) error {
	if self.gcode == nil || self.gcodeMove == nil {
		return nil
	}
	value := strconv.FormatFloat(z, 'f', -1, 64)
	command := self.gcode.Create_gcode_command(
		"SET_GCODE_OFFSET",
		fmt.Sprintf("SET_GCODE_OFFSET Z=%s", value),
		map[string]string{"Z": value},
	)
	self.gcodeMove.Cmd_SET_GCODE_OFFSET(command)
	self.gcodeMove.ResetLastPosition()
	return nil
}

func (self *LeviQ3Module) currentKinematicsRails() []*PrinterRail {
	if self.toolhead == nil {
		return nil
	}
	if kin, ok := self.toolhead.Get_kinematics().(railsProvider); ok {
		return kin.KinematicsRails()
	}
	return nil
}

func leviqAxisIndex(axis printpkg.Axis) int {
	return kinematicspkg.AxisIndex(string(axis))
}

func leviqAxisIndexes(axes []printpkg.Axis) []int {
	strs := make([]string, len(axes))
	for i, a := range axes {
		strs[i] = string(a)
	}
	return kinematicspkg.UniqueAxisIndexes(strs)
}

type leviq3RailHomingSnapshot struct {
	rail               *PrinterRail
	homingSpeed        float64
	secondHomingSpeed  float64
	homingRetractDist  float64
	homingRetractSpeed float64
	homingPositiveDir  bool
}

func (self *LeviQ3Module) overrideRailHomingParams(axisIndexes []int, params printpkg.HomingParams) func() {
	rails := self.currentKinematicsRails()
	snapshots := make([]leviq3RailHomingSnapshot, 0, len(axisIndexes))
	for _, axisIndex := range axisIndexes {
		if axisIndex < 0 || axisIndex >= len(rails) || rails[axisIndex] == nil {
			continue
		}
		rail := rails[axisIndex]
		info := rail.Get_homing_info()
		snapshots = append(snapshots, leviq3RailHomingSnapshot{
			rail:               rail,
			homingSpeed:        info.Speed,
			secondHomingSpeed:  info.SecondHomingSpeed,
			homingRetractDist:  info.RetractDist,
			homingRetractSpeed: info.RetractSpeed,
			homingPositiveDir:  info.PositiveDir,
		})
		updated := *info
		if params.SecondHomingSpeed() > 0 {
			updated.SecondHomingSpeed = params.SecondHomingSpeed()
		}
		if params.HomingRetractDist() > 0 {
			updated.RetractDist = params.HomingRetractDist()
		}
		if params.HomingRetractSpeed() > 0 {
			updated.RetractSpeed = params.HomingRetractSpeed()
		}
		if params.HomingSpeed() > 0 {
			updated.Speed = params.HomingSpeed()
		}
		updated.PositiveDir = params.HomingPositiveDir()
		rail.Set_homing_info(&updated)
	}
	return func() {
		for _, snapshot := range snapshots {
			snapshot.rail.Set_homing_info(&HomingInfo{
				Speed:             snapshot.homingSpeed,
				PositionEndstop:   snapshot.rail.Get_homing_info().PositionEndstop,
				RetractSpeed:      snapshot.homingRetractSpeed,
				RetractDist:       snapshot.homingRetractDist,
				PositiveDir:       snapshot.homingPositiveDir,
				SecondHomingSpeed: snapshot.secondHomingSpeed,
			})
		}
	}
}

func parseEnableState(gcmd *GCodeCommand, defaultValue bool) (bool, bool) {
	if gcmd == nil {
		return defaultValue, false
	}
	if gcmd.Has("ENABLE") {
		return gcmd.Get_int("ENABLE", 1, nil, nil) != 0, true
	}
	state := strings.TrimSpace(strings.ToLower(gcmd.Get("STATE", "", nil, nil, nil, nil, nil)))
	if state == "" {
		state = strings.TrimSpace(strings.ToLower(gcmd.Get("MODE", "", nil, nil, nil, nil, nil)))
	}
	if val, ok := gcodepkg.ParseBooleanString(state); ok {
		return val, true
	}
	return defaultValue, false
}

func (self *leviq3Runtime) moduleReady() error {
	if self == nil || self.module == nil {
		return fmt.Errorf("leviq3 runtime unavailable")
	}
	return self.module.refreshRuntimeObjects()
}

func (self *leviq3Runtime) Is_printer_ready(ctx context.Context) bool {
	if err := self.moduleReady(); err != nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	_, state := self.module.printer.get_state_message()
	return state == "ready" && !self.module.printer.Is_shutdown()
}

func (self *leviq3Runtime) Clear_homing_state(ctx context.Context) error {
	if err := self.moduleReady(); err != nil {
		return err
	}
	if err := self.module.ensureNotCancelled("clear homing state"); err != nil {
		return err
	}
	if kin, ok := self.module.toolhead.Get_kinematics().(interface{ Note_z_not_homed() }); ok {
		kin.Note_z_not_homed()
	}
	return nil
}

func (self *leviq3Runtime) Home_axis(ctx context.Context, axis printpkg.Axis, params printpkg.HomingParams) error {
	return self.Home_rails(ctx, []printpkg.Axis{axis}, params)
}

func (self *leviq3Runtime) Home_rails(ctx context.Context, axes []printpkg.Axis, params printpkg.HomingParams) error {
	if err := self.moduleReady(); err != nil {
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("home rails"); err != nil {
		return err
	}
	homing := NewHoming(self.module.printer)
	axisIndexes := leviqAxisIndexes(axes)
	homing.Set_axes(axisIndexes)
	restore := self.module.overrideRailHomingParams(axisIndexes, params)
	defer restore()
	kin, ok := self.module.toolhead.Get_kinematics().(IKinematics)
	if !ok {
		return fmt.Errorf("unsupported kinematics runtime %T", self.module.toolhead.Get_kinematics())
	}
	kin.Home(homing)
	return nil
}

func (self *leviq3Runtime) Homing_move(ctx context.Context, axis printpkg.Axis, target float64, speed float64) error {
	if err := self.moduleReady(); err != nil {
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("homing move"); err != nil {
		return err
	}
	coord := []interface{}{nil, nil, nil, nil}
	index := leviqAxisIndex(axis)
	if index < 0 {
		return fmt.Errorf("unsupported axis %q", axis)
	}
	coord[index] = target
	if speed <= 0 {
		speed = defaultLeviQ3TravelSpeed
	}
	self.module.toolhead.Manual_move(coord, speed)
	self.module.toolhead.Wait_moves()
	return nil
}

func (self *leviq3Runtime) bedHeater() (*heaterpkg.Heater, error) {
	if err := self.moduleReady(); err != nil {
		return nil, err
	}
	return self.module.heaters.Lookup_heater("heater_bed"), nil
}

func (self *leviq3Runtime) Set_bed_temperature(ctx context.Context, target float64) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("set bed temperature"); err != nil {
		return err
	}
	bed, err := self.bedHeater()
	if err != nil {
		return err
	}
	return self.module.heaters.Set_temperature(bed, target, false)
}

func (self *leviq3Runtime) Wait_for_temperature(ctx context.Context, target float64, timeout time.Duration) error {
	bed, err := self.bedHeater()
	if err != nil {
		return err
	}
	reactor := self.module.printer.Get_reactor()
	deadline := reactor.Monotonic() + timeout.Seconds()
	for {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := self.module.ensureNotCancelled("wait for temperature"); err != nil {
			return err
		}
		now := reactor.Monotonic()
		current, _ := bed.Get_temp(now)
		if current >= target {
			return nil
		}
		if timeout > 0 && now >= deadline {
			return fmt.Errorf("bed temperature wait timed out at %.2f/%.2f", current, target)
		}
		wake := now + 0.25
		if timeout > 0 && wake > deadline {
			wake = deadline
		}
		reactor.Pause(wake)
	}
}

func (self *leviq3Runtime) GetHotbedTemp(ctx context.Context) (float64, error) {
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if err := self.module.ensureNotCancelled("get hotbed temperature"); err != nil {
		return 0, err
	}
	bed, err := self.bedHeater()
	if err != nil {
		return 0, err
	}
	current, _ := bed.Get_temp(self.module.printer.Get_reactor().Monotonic())
	return current, nil
}

func (self *leviq3Runtime) Lower_probe(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("lower probe")
}

func (self *leviq3Runtime) Raise_probe(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("raise probe")
}

func (self *leviq3Runtime) safeTravelZ() float64 {
	safeZ := defaultLeviQ3SafeTravelZ
	if self.module.bedMesh != nil && self.module.bedMesh.Horizontal_move_z > safeZ {
		safeZ = self.module.bedMesh.Horizontal_move_z
	}
	if self.module.toolhead != nil {
		current := self.module.toolhead.Get_position()
		if len(current) > 2 && current[2] > safeZ {
			safeZ = current[2]
		}
	}
	return safeZ
}

func (self *leviq3Runtime) moveNozzle(coord []interface{}, speed float64) error {
	if err := self.module.ensureNotCancelled("move nozzle"); err != nil {
		return err
	}
	self.module.toolhead.Manual_move(coord, speed)
	self.module.toolhead.Wait_moves()
	return nil
}

func (self *leviq3Runtime) Run_probe(ctx context.Context, position printpkg.XY) (float64, error) {
	if err := self.moduleReady(); err != nil {
		return 0, err
	}
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if err := self.module.ensureNotCancelled("run probe"); err != nil {
		return 0, err
	}
	xOffset, yOffset, _ := self.module.probe.Get_offsets()
	if err := self.moveNozzle([]interface{}{nil, nil, self.safeTravelZ(), nil}, defaultLeviQ3VerticalSpeed); err != nil {
		return 0, err
	}
	if err := self.moveNozzle([]interface{}{position.X - xOffset, position.Y - yOffset, nil, nil}, defaultLeviQ3TravelSpeed); err != nil {
		return 0, err
	}
	result := self.module.probe.Run_probe(self.module.gcode.Create_gcode_command("PROBE", "PROBE", map[string]string{}))
	if len(result) < 3 {
		return 0, fmt.Errorf("probe returned invalid result %#v", result)
	}
	return result[2], nil
}

func (self *leviq3Runtime) Wipe_nozzle(ctx context.Context, position printpkg.XYZ) error {
	if err := self.moduleReady(); err != nil {
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("wipe nozzle"); err != nil {
		return err
	}
	current := self.module.toolhead.Get_position()
	safeZ := math.Max(self.safeTravelZ(), position.Z)
	if len(current) > 2 && current[2] < safeZ {
		if err := self.moveNozzle([]interface{}{nil, nil, safeZ, nil}, defaultLeviQ3VerticalSpeed); err != nil {
			return err
		}
	}
	if err := self.moveNozzle([]interface{}{position.X, position.Y, nil, nil}, defaultLeviQ3TravelSpeed); err != nil {
		return err
	}
	if err := self.moveNozzle([]interface{}{nil, nil, position.Z, nil}, defaultLeviQ3VerticalSpeed); err != nil {
		return err
	}
	return nil
}

func (self *leviq3Runtime) Set_gcode_offset(ctx context.Context, z float64) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("set gcode offset"); err != nil {
		return err
	}
	return self.module.applyRuntimeZOffset(z)
}

func (self *leviq3Runtime) Current_z_offset(ctx context.Context) float64 {
	if ctx != nil && ctx.Err() != nil {
		return self.module.helper.CurrentZOffset()
	}
	if self.module.gcodeMove == nil {
		return self.module.helper.CurrentZOffset()
	}
	status := self.module.gcodeMove.Get_status(0)
	homingOrigin, ok := status["homing_origin"].([]float64)
	if !ok || len(homingOrigin) < 3 {
		return self.module.helper.CurrentZOffset()
	}
	return homingOrigin[2]
}

func (self *leviq3Runtime) Save_mesh(ctx context.Context, mesh *printpkg.BedMesh) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("save mesh"); err != nil {
		return err
	}
	if err := self.moduleReady(); err != nil {
		return err
	}
	if self.module.bedMesh == nil || self.module.helper == nil {
		return nil
	}
	return self.module.bedMesh.ApplyRecoveredMesh(mesh, self.module.helper.MeshBuildParams(), self.module.profileName)
}

func (self *leviq3Runtime) Sleep(ctx context.Context, d time.Duration) error {
	reactor := self.module.printer.Get_reactor()
	deadline := reactor.Monotonic() + d.Seconds()
	for {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := self.module.ensureNotCancelled("sleep"); err != nil {
			return err
		}
		now := reactor.Monotonic()
		if now >= deadline {
			return nil
		}
		wake := now + 0.1
		if wake > deadline {
			wake = deadline
		}
		reactor.Pause(wake)
	}
}

func (self *leviq3Runtime) GetAutoZOffsetTemperature(ctx context.Context) (float64, error) {
	return self.GetHotbedTemp(ctx)
}

func (self *leviq3Runtime) Z_offset_apply_probe(ctx context.Context, z float64) error {
	_ = z
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("sync probe offset")
}

func (self *leviq3Runtime) Z_offset_apply_probe_absolute(ctx context.Context, z float64) error {
	_ = z
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("sync absolute probe offset")
}

func (self *leviq3Runtime) Set_mesh_offsets(ctx context.Context, offsets printpkg.XYZ) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("sync mesh offsets"); err != nil {
		return err
	}
	if self.module.bedMesh != nil && self.module.bedMesh.Get_mesh() != nil {
		self.module.bedMesh.Get_mesh().Set_mesh_offsets([]float64{offsets.X, offsets.Y})
	}
	return nil
}

func (self *leviq3Runtime) Set_rails_z_offset(ctx context.Context, z float64) error {
	_ = z
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("sync rail z offset")
}

func (self *leviq3Runtime) CancelLeviQ3(ctx context.Context, reason string) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if self.module.commandState != nil {
		self.module.commandState.NoteCancellation(reason)
	}
	return nil
}

const (
	BUFFER_TIME_LOW         = motionpkg.DefaultBufferTimeLow
	BUFFER_TIME_HIGH        = motionpkg.DefaultBufferTimeHigh
	BUFFER_TIME_START       = motionpkg.DefaultBufferTimeStart
	BGFLUSH_LOW_TIME        = motionpkg.DefaultBgFlushLowTime
	BGFLUSH_BATCH_TIME      = motionpkg.DefaultBgFlushBatchTime
	BGFLUSH_EXTRA_TIME      = motionpkg.DefaultBgFlushExtraTime
	MIN_KIN_TIME            = motionpkg.DefaultMinKinTime
	MOVE_BATCH_TIME         = motionpkg.DefaultMoveBatchTime
	STEPCOMPRESS_FLUSH_TIME = motionpkg.DefaultStepcompressFlushTime
	SDS_CHECK_TIME          = motionpkg.DefaultSdsCheckTime
	MOVE_HISTORY_EXPIRE     = motionpkg.DefaultMoveHistoryExpire
	DRIP_SEGMENT_TIME       = motionpkg.DefaultDripSegmentTime
	DRIP_TIME               = motionpkg.DefaultDripTime
)

type Move = motionpkg.Move

type LookAheadQueue = motionpkg.LookAheadQueue

type IToolhead interface {
	Get_position() []float64
	Get_kinematics() interface{}
	Flush_step_generation()
	Get_last_move_time() float64
	Dwell(delay float64)
	Drip_move(newpos []float64, speed float64, drip_completion *ReactorCompletion) error
	Set_position(newpos []float64, homingAxes []int)
}

type IKinematics interface {
	Set_position(newpos []float64, homing_axes []int)
	Check_move(move *Move)
	Get_status(eventtime float64) map[string]interface{}
	Get_steppers() []interface{}
	Note_z_not_homed()
	Calc_position(stepper_positions map[string]float64) []float64
	Home(homing_state *Homing)
}

type PrinterRail struct {
	stepper_units_in_radians bool
	steppers                 []*mcupkg.LegacyStepper
	endstops                 []list.List
	endstop_map              map[string]interface{}
	*motionpkg.LegacyRailRuntime
	*motionpkg.LegacyRailState
}

func NewPrinterRail(config *ConfigWrapper, need_position_minmax bool,
	default_position_endstop interface{}, units_in_radians bool) *PrinterRail {
	self := PrinterRail{}
	self.stepper_units_in_radians = units_in_radians
	self.steppers = []*mcupkg.LegacyStepper{}
	self.endstops = []list.List{}
	self.endstop_map = map[string]interface{}{}
	self.LegacyRailRuntime = motionpkg.NewLegacyRailRuntime()
	self.Add_extra_stepper(config)
	mcu_endstop := self.endstops[0].Front().Value
	settings := kinematicspkg.BuildLegacyRailSettings(config, mcu_endstop, need_position_minmax, default_position_endstop)
	self.LegacyRailState = motionpkg.NewLegacyRailState(settings.PositionMin, settings.PositionMax, motionpkg.LegacyRailHomingInfo{
		Speed:             settings.HomingSpeed,
		PositionEndstop:   settings.PositionEndstop,
		RetractSpeed:      settings.HomingRetractSpeed,
		RetractDist:       settings.HomingRetractDist,
		PositiveDir:       settings.HomingPositiveDir,
		SecondHomingSpeed: settings.SecondHomingSpeed,
	})
	return &self
}

type HomingInfo = motionpkg.LegacyRailHomingInfo

func (self *PrinterRail) Get_steppers() []*mcupkg.LegacyStepper {
	steppersBack := make([]*mcupkg.LegacyStepper, len(self.steppers))
	copy(steppersBack, self.steppers)
	return steppersBack
}

func (self *PrinterRail) Get_endstops() []list.List {
	endstopsBack := make([]list.List, len(self.endstops))
	copy(endstopsBack, self.endstops)
	return endstopsBack
}

func (self *PrinterRail) Add_extra_stepper(config *ConfigWrapper) {
	printer := config.Get_printer()
	stepper := mcupkg.LoadLegacyPrinterStepper(config, self.stepper_units_in_radians, printer, func(moduleName string) interface{} {
		return printer.Load_object(config, moduleName, object.Sentinel{})
	}, func(module interface{}, stepper *mcupkg.LegacyStepper) {
		module.(*mcupkg.PrinterStepperEnableModule).Register_stepper(config, stepper)
	}, func(module interface{}, stepper *mcupkg.LegacyStepper) {
		module.(*motionpkg.ForceMoveModule).RegisterStepper(stepper)
	})
	self.steppers = append(self.steppers, stepper)
	if self.LegacyRailRuntime != nil {
		self.LegacyRailRuntime.AddStepper(stepper)
	}
	if len(self.endstops) > 0 && config.Get("endstop_pin", value.None, false) == nil {
		self.endstops[0].Front().Value.(*mcupkg.LegacyEndstop).Add_stepper(stepper)
		return
	}
	endstopPin := config.Get("endstop_pin", object.Sentinel{}, false)
	pins := printer.Lookup_object("pins", object.Sentinel{})
	pinParams := pins.(*printerpkg.PrinterPins).Parse_pin(endstopPin.(string), true, true)
	name := stepper.Get_name(true)
	queryEndstops := printer.Load_object(config, "query_endstops", object.Sentinel{}).(*probepkg.QueryEndstopsModule)
	result, err := mcupkg.ResolveLegacyRailEndstop(
		mcupkg.LegacyRailEndstopEntriesFromRawMap(self.endstop_map),
		pinParams["chip_name"].(string),
		pinParams["pin"],
		pinParams["invert"],
		pinParams["pullup"],
		name,
		func() interface{} {
			return pins.(*printerpkg.PrinterPins).Setup_pin("endstop", endstopPin.(string))
		},
		queryEndstops.Register_endstop,
	)
	if err != nil {
		panic(fmt.Errorf("pinter rail %s %w", self.Get_name(false), err))
	}
	if result.Created {
		self.endstop_map[result.PinName] = mcupkg.RawLegacyRailEndstopEntry(result.Entry)
		var list list.List
		list.PushBack(result.Endstop)
		list.PushBack(name)
		self.endstops = append(self.endstops, list)
	}
	if !mcupkg.AttachStepperToLegacyRailEndstop(result.Endstop, stepper) {
		panic(fmt.Errorf("pinter rail %s unexpected endstop %T", self.Get_name(false), result.Endstop))
	}
}

func LookupMultiRail(config *ConfigWrapper, need_position_minmax bool, default_position_endstop interface{}, units_in_radians bool) *PrinterRail {
	dpe, ok := default_position_endstop.(*float64)
	if !ok {
		dpe = nil
	}
	rail := NewPrinterRail(config, need_position_minmax, dpe, units_in_radians)
	for i := 1; i < 99; i++ {
		if !config.Has_section(config.Get_name() + strconv.Itoa(i)) {
			break
		}
		rail.Add_extra_stepper(config.Getsection(config.Get_name() + strconv.Itoa(i)))
	}
	return rail
}

const (
	HOMING_START_DELAY   = homingpkg.HomingStartDelay
	ENDSTOP_SAMPLE_TIME  = homingpkg.EndstopSampleTime
	ENDSTOP_SAMPLE_COUNT = homingpkg.EndstopSampleCount
)

func newHomingReactorAdapter(reactor IReactor) homingpkg.Reactor {
	return &homingpkg.ReactorFuncs{RegisterCallbackFunc: func(callback func(interface{}) interface{}, waketime float64) homingpkg.Completion {
		return reactor.Register_callback(callback, waketime)
	}}
}

type projectHomingStepper interface {
	Get_name(bool) string
	Get_mcu_position() int
	Get_past_mcu_position(float64) int
	Calc_position_from_coord([]float64) float64
	Get_step_dist() float64
	Get_commanded_position() float64
}

type homingStepperAdapter struct {
	stepper projectHomingStepper
	*homingpkg.StepperFuncs
}

func newHomingStepperAdapter(stepper projectHomingStepper) *homingStepperAdapter {
	return &homingStepperAdapter{
		stepper: stepper,
		StepperFuncs: &homingpkg.StepperFuncs{
			GetNameFunc:               stepper.Get_name,
			GetMCUPositionFunc:        stepper.Get_mcu_position,
			GetPastMCUPositionFunc:    stepper.Get_past_mcu_position,
			CalcPositionFromCoordFunc: stepper.Calc_position_from_coord,
			GetStepDistFunc:           stepper.Get_step_dist,
			GetCommandedPositionFunc:  stepper.Get_commanded_position,
		},
	}
}

func newHomingKinematicsAdapter(kinematics IKinematics) homingpkg.Kinematics {
	return &homingpkg.KinematicsFuncs{
		GetSteppersFunc: func() []homingpkg.Stepper {
			steppers := kinematics.Get_steppers()
			adapted := make([]homingpkg.Stepper, len(steppers))
			for i, stepper := range steppers {
				mcuStepper, ok := stepper.(projectHomingStepper)
				if !ok {
					panic(fmt.Errorf("homing kinematics stepper has unexpected type %T", stepper))
				}
				adapted[i] = newHomingStepperAdapter(mcuStepper)
			}
			return adapted
		},
		CalcPositionFunc: kinematics.Calc_position,
	}
}

func newHomingToolheadAdapter(toolhead IToolhead) homingpkg.Toolhead {
	return &homingpkg.ToolheadFuncs{
		GetPositionFunc: toolhead.Get_position,
		GetKinematicsFunc: func() homingpkg.Kinematics {
			kinematics, ok := toolhead.Get_kinematics().(IKinematics)
			if !ok {
				panic(fmt.Errorf("homing toolhead kinematics has unexpected type %T", toolhead.Get_kinematics()))
			}
			return newHomingKinematicsAdapter(kinematics)
		},
		FlushStepGenerationFunc: toolhead.Flush_step_generation,
		GetLastMoveTimeFunc:     toolhead.Get_last_move_time,
		DwellFunc:               toolhead.Dwell,
		DripMoveFunc: func(newpos []float64, speed float64, dripCompletion homingpkg.Completion) error {
			if dripCompletion == nil {
				return toolhead.Drip_move(newpos, speed, nil)
			}
			runtimeCompletion, ok := dripCompletion.(*ReactorCompletion)
			if !ok {
				return fmt.Errorf("homing completion has unexpected type %T", dripCompletion)
			}
			return toolhead.Drip_move(newpos, speed, runtimeCompletion)
		},
		MoveFunc: func(newpos []float64, speed float64) {
			mover, ok := toolhead.(interface{ Move([]float64, float64) })
			if !ok {
				panic(fmt.Errorf("homing toolhead has unexpected move type %T", toolhead))
			}
			mover.Move(newpos, speed)
		},
		SetPositionFunc: toolhead.Set_position,
	}
}

type homingEndstopAdapter struct {
	endstop interface{}
	*homingpkg.EndstopFuncs
}

func newHomingEndstopAdapter(endstop interface{}) *homingEndstopAdapter {
	adapter := &homingEndstopAdapter{endstop: endstop}
	adapter.EndstopFuncs = &homingpkg.EndstopFuncs{
		GetSteppersFunc: func() []homingpkg.Stepper {
			typed, ok := adapter.endstop.(interface{ Get_steppers() []interface{} })
			if !ok {
				panic(fmt.Errorf("homing endstop has unexpected type %T", adapter.endstop))
			}
			steppers := typed.Get_steppers()
			adapted := make([]homingpkg.Stepper, len(steppers))
			for i, stepper := range steppers {
				mcuStepper, ok := stepper.(projectHomingStepper)
				if !ok {
					panic(fmt.Errorf("homing endstop stepper has unexpected type %T", stepper))
				}
				adapted[i] = newHomingStepperAdapter(mcuStepper)
			}
			return adapted
		},
		HomeStartFunc: func(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64) homingpkg.Completion {
			typed, ok := adapter.endstop.(interface {
				Home_start(float64, float64, int64, float64, int64) interface{}
			})
			if !ok {
				panic(fmt.Errorf("homing endstop has unexpected type %T", adapter.endstop))
			}
			runtimeCompletion := typed.Home_start(printTime, sampleTime, sampleCount, restTime, triggered)
			completion, ok := runtimeCompletion.(*ReactorCompletion)
			if !ok {
				panic(fmt.Errorf("homing endstop completion has unexpected type %T", runtimeCompletion))
			}
			return completion
		},
		HomeWaitFunc: func(moveEndPrintTime float64) float64 {
			typed, ok := adapter.endstop.(interface{ Home_wait(float64) float64 })
			if !ok {
				panic(fmt.Errorf("homing endstop has unexpected type %T", adapter.endstop))
			}
			return typed.Home_wait(moveEndPrintTime)
		},
	}
	return adapter
}

type homingRailAdapter struct {
	rail *PrinterRail
	*homingpkg.RailFuncs
}

func newHomingRailAdapter(rail *PrinterRail) *homingRailAdapter {
	adapter := &homingRailAdapter{rail: rail}
	adapter.RailFuncs = &homingpkg.RailFuncs{
		GetEndstopsFunc: func() []homingpkg.NamedEndstop {
			return adaptHomingEndstops(adapter.rail.Get_endstops())
		},
		GetHomingInfoFunc: func() *homingpkg.RailHomingInfo {
			info := adapter.rail.Get_homing_info()
			return &homingpkg.RailHomingInfo{
				Speed:             info.Speed,
				PositionEndstop:   info.PositionEndstop,
				RetractSpeed:      info.RetractSpeed,
				RetractDist:       info.RetractDist,
				PositiveDir:       info.PositiveDir,
				SecondHomingSpeed: info.SecondHomingSpeed,
			}
		},
	}
	return adapter
}

func adaptHomingEndstops(endstops []list.List) []homingpkg.NamedEndstop {
	adapted := make([]homingpkg.NamedEndstop, 0, len(endstops))
	for _, endstop := range endstops {
		if endstop.Front() == nil || endstop.Back() == nil {
			continue
		}
		name, ok := endstop.Back().Value.(string)
		if !ok {
			panic(fmt.Errorf("homing endstop name has unexpected type %T", endstop.Back().Value))
		}
		adapted = append(adapted, homingpkg.NamedEndstop{
			Endstop: newHomingEndstopAdapter(endstop.Front().Value),
			Name:    name,
		})
	}
	return adapted
}

func adaptHomingRails(rails []*PrinterRail) []homingpkg.Rail {
	adapted := make([]homingpkg.Rail, len(rails))
	for i, rail := range rails {
		adapted[i] = newHomingRailAdapter(rail)
	}
	return adapted
}

func namedEndstopsToProjectLists(endstops []homingpkg.NamedEndstop) []list.List {
	adapted := make([]list.List, 0, len(endstops))
	for _, endstop := range endstops {
		adapter, ok := endstop.Endstop.(*homingEndstopAdapter)
		if !ok {
			panic(fmt.Errorf("homing named endstop has unexpected type %T", endstop.Endstop))
		}
		entry := list.List{}
		entry.PushBack(adapter.endstop)
		entry.PushBack(endstop.Name)
		adapted = append(adapted, entry)
	}
	return adapted
}

type StepperPosition struct {
	Stepper      projectHomingStepper
	Endstop_name string
	Stepper_name string
	Start_pos    int
	Halt_pos     int
	Trig_pos     int
	core         *homingpkg.StepperPosition
}

func NewStepperPosition(stepper projectHomingStepper, endstop_name string) *StepperPosition {
	return newProjectStepperPosition(homingpkg.NewStepperPosition(newHomingStepperAdapter(stepper), endstop_name))
}

func (self *StepperPosition) Note_home_end(trigger_time float64) {
	if self.core == nil {
		self.Halt_pos = self.Stepper.Get_mcu_position()
		self.Trig_pos = self.Stepper.Get_past_mcu_position(trigger_time)
		return
	}
	self.core.NoteHomeEnd(trigger_time)
	self.syncFromCore()
}

func newProjectStepperPosition(position *homingpkg.StepperPosition) *StepperPosition {
	stepperAdapter, ok := position.Stepper.(*homingStepperAdapter)
	if !ok {
		panic(fmt.Errorf("homing stepper position has unexpected type %T", position.Stepper))
	}
	return &StepperPosition{
		Stepper:      stepperAdapter.stepper,
		Endstop_name: position.EndstopName,
		Stepper_name: position.StepperName,
		Start_pos:    position.StartPos,
		Halt_pos:     position.HaltPos,
		Trig_pos:     position.TrigPos,
		core:         position,
	}
}

func (self *StepperPosition) syncFromCore() {
	if self.core == nil {
		return
	}
	stepperAdapter, ok := self.core.Stepper.(*homingStepperAdapter)
	if !ok {
		panic(fmt.Errorf("homing stepper position has unexpected type %T", self.core.Stepper))
	}
	self.Stepper = stepperAdapter.stepper
	self.Endstop_name = self.core.EndstopName
	self.Stepper_name = self.core.StepperName
	self.Start_pos = self.core.StartPos
	self.Halt_pos = self.core.HaltPos
	self.Trig_pos = self.core.TrigPos
}

type HomingMove struct {
	Printer           *Printer
	Endstops          []list.List
	Toolhead          IToolhead
	Stepper_positions []*StepperPosition
	core              *homingpkg.Move
}

func NewHomingMove(printer *Printer, endstops []list.List, toolhead interface{}) *HomingMove {
	if toolhead == nil {
		toolhead = printer.Lookup_object("toolhead", object.Sentinel{})
	}
	runtimeToolhead, ok := toolhead.(IToolhead)
	if !ok {
		panic(fmt.Errorf("homing toolhead has unexpected type %T", toolhead))
	}
	self := &HomingMove{
		Printer:           printer,
		Endstops:          append([]list.List{}, endstops...),
		Toolhead:          runtimeToolhead,
		Stepper_positions: []*StepperPosition{},
	}
	self.core = homingpkg.NewMove(
		newHomingReactorAdapter(printer.Get_reactor()),
		newHomingToolheadAdapter(runtimeToolhead),
		adaptHomingEndstops(endstops),
		printer.Get_start_args()["debuginput"] != "",
	)
	return self
}

func (self *HomingMove) Get_mcu_endstops() []interface{} {
	result := make([]interface{}, 0, len(self.Endstops))
	for _, endstop := range self.Endstops {
		result = append(result, endstop)
	}
	return result
}

func (self *HomingMove) Calc_endstop_rate(mcu_endstop interface{}, movepos []float64, speed float64) float64 {
	return self.core.CalcEndstopRate(newHomingEndstopAdapter(mcu_endstop), movepos, speed)
}

func (self *HomingMove) Calc_toolhead_pos(kin_spos1 map[string]float64, offsets map[string]float64) []float64 {
	return self.core.CalcToolheadPos(kin_spos1, offsets)
}

func (self *HomingMove) syncStepperPositions() {
	runtimePositions := self.core.StepperPositions()
	self.Stepper_positions = make([]*StepperPosition, len(runtimePositions))
	for i, position := range runtimePositions {
		self.Stepper_positions[i] = newProjectStepperPosition(position)
	}
}

func (self *HomingMove) executeRuntime(movepos []float64, speed float64, probe_pos bool, triggered bool, check_triggered bool) ([]float64, float64, error) {
	if _, eventErr := self.Printer.Send_event("homing:homing_move_begin", []interface{}{self}); eventErr != nil {
		return nil, 0, eventErr
	}
	triggerPos, triggerTime, err := self.core.Execute(movepos, speed, probe_pos, triggered, check_triggered)
	self.syncStepperPositions()
	_, eventErr := self.Printer.Send_event("homing:homing_move_end", []interface{}{self})
	if err == nil && eventErr != nil {
		err = eventErr
	}
	return triggerPos, triggerTime, err
}

func (self *HomingMove) Homing_move(movepos []float64, speed float64, probe_pos bool, triggered bool, check_triggered bool) (interface{}, float64) {
	triggerPos, triggerTime, err := self.executeRuntime(movepos, speed, probe_pos, triggered, check_triggered)
	if err != nil {
		panic(err.Error())
	}
	return triggerPos, triggerTime
}

func (self *HomingMove) Check_no_movement() string {
	self.syncStepperPositions()
	return self.core.CheckNoMovement()
}

func newHomingMoveExecutor(move *HomingMove) homingpkg.MoveExecutor {
	return &homingpkg.MoveExecutorFuncs{
		ExecuteFunc:         move.executeRuntime,
		CheckNoMovementFunc: move.Check_no_movement,
		StepperPositionsFunc: func() []*homingpkg.StepperPosition {
			return move.core.StepperPositions()
		},
	}
}

type Homing struct {
	Printer         *Printer
	Toolhead        *Toolhead
	Changed_axes    []int
	Trigger_mcu_pos map[string]float64
	Adjust_pos      map[string]float64
	core            *homingpkg.State
}

func NewHoming(printer *Printer) *Homing {
	self := &Homing{Printer: printer}
	toolhead := printer.Lookup_object("toolhead", object.Sentinel{})
	self.Toolhead = toolhead.(*Toolhead)
	self.Changed_axes = []int{}
	self.Trigger_mcu_pos = map[string]float64{}
	self.Adjust_pos = map[string]float64{}
	self.core = homingpkg.NewState(newHomingToolheadAdapter(self.Toolhead))
	self.syncFromCore()
	return self
}

func (self *Homing) Set_axes(axes []int) {
	self.core.SetAxes(axes)
	self.syncFromCore()
}

func (self *Homing) Get_axes() []int {
	return self.core.GetAxes()
}

func (self *Homing) Get_trigger_position(stepper_name string) float64 {
	return self.core.GetTriggerPosition(stepper_name)
}

func (self *Homing) Set_stepper_adjustment(stepper_name string, adjustment float64) {
	self.core.SetStepperAdjustment(stepper_name, adjustment)
	self.syncFromCore()
}

func (self *Homing) Fill_coord(coord []interface{}) []float64 {
	return self.core.FillCoord(coord)
}

func (self *Homing) Set_homed_position(pos []float64) {
	self.core.SetHomedPosition(self.Fill_coord(collections.FloatInterface(pos)))
}

func (self *Homing) Home_rails(rails []*PrinterRail, forcepos []interface{}, movepos []interface{}) {
	if _, err := self.Printer.Send_event("homing:home_rails_begin", []interface{}{self, rails}); err != nil {
		panic(err.Error())
	}
	err := self.core.HomeRailsWithPositions(
		adaptHomingRails(rails),
		forcepos,
		movepos,
		func(endstops []homingpkg.NamedEndstop) homingpkg.MoveExecutor {
			return newHomingMoveExecutor(NewHomingMove(self.Printer, namedEndstopsToProjectLists(endstops), self.Toolhead))
		},
		func() error {
			self.Adjust_pos = map[string]float64{}
			_, eventErr := self.Printer.Send_event("homing:home_rails_end", []interface{}{self, rails})
			return eventErr
		},
	)
	self.syncFromCore()
	if err != nil {
		panic(err.Error())
	}
}

func (self *Homing) syncFromCore() {
	self.Changed_axes = self.core.GetAxes()
	self.Trigger_mcu_pos = self.core.TriggerPositions()
	self.Adjust_pos = self.core.Adjustments()
}

type PrinterHoming struct {
	Printer *Printer
}

func NewPrinterHoming(config *ConfigWrapper) *PrinterHoming {
	var self = PrinterHoming{}
	self.Printer = config.Get_printer()
	gcode := self.Printer.Lookup_object("gcode", object.Sentinel{})
	gcode.(*GCodeDispatch).Register_command("G28", self.Cmd_G28, false, "")
	return &self
}

func (self *PrinterHoming) Manual_home(toolhead interface{}, endstops []list.List, pos []float64, speed float64, triggered bool, check_triggered bool) {
	hmove := NewHomingMove(self.Printer, endstops, toolhead)
	if err := homingpkg.ManualHome(hmove.executeRuntime, pos, speed, triggered, check_triggered); err != nil {
		panic(err.Error())
	}
}

func (self *PrinterHoming) Probing_move(mcu_probe interface{}, pos []float64, speed float64) []float64 {
	endstops := []list.List{}
	endstop := list.List{}
	endstop.PushBack(mcu_probe)
	endstop.PushBack("probe")
	endstops = append(endstops, endstop)
	hmove := NewHomingMove(self.Printer, endstops, nil)
	probePos, err := homingpkg.ProbingMove(hmove.executeRuntime, hmove.Check_no_movement, pos, speed)
	if err != nil {
		panic(err.Error())
	}
	return probePos
}

func (self *PrinterHoming) Cmd_G28(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	homing_state := NewHoming(self.Printer)
	toolhead := self.Printer.Lookup_object("toolhead", object.Sentinel{})
	kin := toolhead.(*Toolhead).Get_kinematics().(IKinematics)
	homingpkg.CommandG28(
		gcmd.Has,
		homing_state.Set_axes,
		func() {
			kin.Home(homing_state)
		},
		homingpkg.HomeRecoveryOptions{
			IsShutdown: self.Printer.Is_shutdown,
			MotorOff: func() {
				motor := self.Printer.Lookup_object("stepper_enable", object.Sentinel{})
				motor.(*mcupkg.PrinterStepperEnableModule).Motor_off()
			},
		},
	)
	return nil
}

func Load_config_homing(config *ConfigWrapper) interface{} {
	return NewPrinterHoming(config)
}

func loadKinematicsRails(config *ConfigWrapper) []*PrinterRail {
	rails := make([]*PrinterRail, 0, 3)
	for _, axis := range []string{"x", "y", "z"} {
		rails = append(rails, LookupMultiRail(config.Getsection("stepper_"+axis), true, nil, false))
	}
	return rails
}

type kinematicsFactory func(*Toolhead, *ConfigWrapper) IKinematics

type railsProvider interface {
	KinematicsRails() []*PrinterRail
}

func Load_kinematics(kin_name string) kinematicsFactory {
	switch kin_name {
	case "cartesian":
		return Load_kinematics_cartesian
	case "corexy":
		return Load_kinematics_corexy
	default:
		logger.Error(fmt.Errorf("module about %s not support", kin_name))
		return Load_kinematics_none
	}
}

type kinematicsAdapter struct {
	runtime kinematicspkg.Kinematics
}

func (self *kinematicsAdapter) Get_steppers() []interface{} {
	steppers := self.runtime.GetSteppers()
	result := make([]interface{}, len(steppers))
	for i, stepper := range steppers {
		mcuStepper, ok := stepper.(*mcupkg.LegacyStepper)
		if !ok {
			panic(fmt.Errorf("kinematics stepper has unexpected type %T", stepper))
		}
		result[i] = mcuStepper
	}
	return result
}

func (self *kinematicsAdapter) Calc_position(stepper_positions map[string]float64) []float64 {
	return self.runtime.CalcPosition(stepper_positions)
}

func (self *kinematicsAdapter) Set_position(newpos []float64, homing_axes []int) {
	self.runtime.SetPosition(newpos, homing_axes)
}

func (self *kinematicsAdapter) Note_z_not_homed() {
	self.runtime.NoteZNotHomed()
}

func (self *kinematicsAdapter) Home(homing_state *Homing) {
	self.runtime.Home(newKinematicsHomingAdapter(homing_state))
}

func (self *kinematicsAdapter) Check_move(move *Move) {
	self.runtime.CheckMove(move)
}

func (self *kinematicsAdapter) Get_status(eventtime float64) map[string]interface{} {
	return self.runtime.Status(eventtime)
}

type CartKinematics struct {
	*kinematicsAdapter
	core *kinematicspkg.CartesianKinematics
}

func NewCartKinematics(toolhead *Toolhead, config *ConfigWrapper) *CartKinematics {
	rails := loadKinematicsRails(config)
	maxVelocity, maxAccel := toolhead.Get_max_velocity()
	cartesianConfig := kinematicspkg.CartesianConfig{
		Printer:      config.Get_printer(),
		Toolhead:     toolhead,
		Rails:        adaptKinematicsRails(rails),
		MaxZVelocity: config.Getfloat("max_z_velocity", maxVelocity, 0, maxVelocity, 0., 0, true),
		MaxZAccel:    config.Getfloat("max_z_accel", maxAccel, 0, maxAccel, 0., 0, true),
	}
	if config.Has_section("dual_carriage") {
		dcConfig := config.Getsection("dual_carriage")
		axisName := dcConfig.Getchoice("axis", map[interface{}]interface{}{"x": "x", "y": "y"}, object.Sentinel{}, true).(string)
		axisIndex := map[string]int{"x": 0, "y": 1}[axisName]
		dcRail := LookupMultiRail(dcConfig, true, nil, false)
		cartesianConfig.DualCarriage = &kinematicspkg.DualCarriageConfig{
			Axis:     axisIndex,
			AxisName: axisName,
			Rails: []kinematicspkg.Rail{
				cartesianConfig.Rails[axisIndex],
				newKinematicsRailAdapter(dcRail),
			},
		}
	}
	core := kinematicspkg.NewCartesian(cartesianConfig)
	self := &CartKinematics{
		kinematicsAdapter: &kinematicsAdapter{runtime: core},
		core:              core,
	}
	if cartesianConfig.DualCarriage != nil {
		gcode := config.Get_printer().Lookup_object("gcode", object.Sentinel{}).(*GCodeDispatch)
		gcode.Register_command("SET_DUAL_CARRIAGE", self.Cmd_SET_DUAL_CARRIAGE, false, kinematicspkg.SetDualCarriageHelp)
	}
	return self
}

func (self *CartKinematics) Cmd_SET_DUAL_CARRIAGE(arg interface{}) error {
	gcmd := arg.(*GCodeCommand)
	return kinematicspkg.HandleSetDualCarriageCommand(self.core, gcmd)
}

func (self *CartKinematics) KinematicsRails() []*PrinterRail {
	return projectRailsFromKinematicRails(self.core.Rails())
}

func Load_kinematics_cartesian(toolhead *Toolhead, config *ConfigWrapper) IKinematics {
	return NewCartKinematics(toolhead, config)
}

type CorexyKinematics struct {
	*kinematicsAdapter
	core *kinematicspkg.CoreXYKinematics
}

func NewCorexyKinematics(toolhead *Toolhead, config *ConfigWrapper) *CorexyKinematics {
	rails := loadKinematicsRails(config)
	maxVelocity, maxAccel := toolhead.Get_max_velocity()
	core := kinematicspkg.NewCoreXY(kinematicspkg.CoreXYConfig{
		Printer:      config.Get_printer(),
		Toolhead:     toolhead,
		Rails:        adaptKinematicsRails(rails),
		MaxZVelocity: config.Getfloat("max_z_velocity", maxVelocity, 0, maxVelocity, 0., 0, true),
		MaxZAccel:    config.Getfloat("max_z_accel", maxAccel, 0, maxAccel, 0., 0, true),
	})
	return &CorexyKinematics{
		kinematicsAdapter: &kinematicsAdapter{runtime: core},
		core:              core,
	}
}

func (self *CorexyKinematics) KinematicsRails() []*PrinterRail {
	return projectRailsFromKinematicRails(self.core.Rails())
}

func Load_kinematics_corexy(toolhead *Toolhead, config *ConfigWrapper) IKinematics {
	return NewCorexyKinematics(toolhead, config)
}

type NoneKinematics struct {
	*kinematicsAdapter
}

func NewNoneKinematics(toolhead *Toolhead, config *ConfigWrapper) *NoneKinematics {
	_ = config
	core := kinematicspkg.NewNone(kinematicspkg.NoneConfig{AxesMinMax: toolhead.Coord})
	return &NoneKinematics{
		kinematicsAdapter: &kinematicsAdapter{runtime: core},
	}
}

func Load_kinematics_none(toolhead *Toolhead, config *ConfigWrapper) IKinematics {
	return NewNoneKinematics(toolhead, config)
}

type kinematicsRailAdapter struct {
	rail *PrinterRail
	*kinematicspkg.RailFuncs
}

func adaptKinematicsRails(rails []*PrinterRail) []kinematicspkg.Rail {
	adapted := make([]kinematicspkg.Rail, len(rails))
	for i, rail := range rails {
		adapted[i] = newKinematicsRailAdapter(rail)
	}
	return adapted
}

func newKinematicsRailAdapter(rail *PrinterRail) *kinematicsRailAdapter {
	adapter := &kinematicsRailAdapter{rail: rail}
	adapter.RailFuncs = &kinematicspkg.RailFuncs{
		SetupItersolveFunc: func(allocFunc string, params ...interface{}) {
			adapter.rail.Setup_itersolve(allocFunc, params...)
		},
		GetSteppersFunc: func() []kinematicspkg.Stepper {
			steppers := adapter.rail.Get_steppers()
			adapted := make([]kinematicspkg.Stepper, len(steppers))
			for i, stepper := range steppers {
				adapted[i] = stepper
			}
			return adapted
		},
		PrimaryEndstopFunc: func() kinematicspkg.RailEndstop {
			endstops := adapter.rail.Get_endstops()
			if len(endstops) == 0 {
				return nil
			}
			front := endstops[0].Front()
			if front == nil {
				return nil
			}
			return newKinematicsRailEndstopAdapter(front.Value)
		},
		GetRangeFunc: func() (float64, float64) {
			return adapter.rail.Get_range()
		},
		SetPositionFunc: func(newpos []float64) {
			adapter.rail.Set_position(newpos)
		},
		GetHomingInfoFunc: func() *kinematicspkg.RailHomingInfo {
			info := adapter.rail.Get_homing_info()
			return &kinematicspkg.RailHomingInfo{
				Speed:             info.Speed,
				PositionEndstop:   info.PositionEndstop,
				RetractSpeed:      info.RetractSpeed,
				RetractDist:       info.RetractDist,
				PositiveDir:       info.PositiveDir,
				SecondHomingSpeed: info.SecondHomingSpeed,
			}
		},
		SetTrapqFunc: func(tq interface{}) {
			adapter.rail.Set_trapq(tq)
		},
		GetCommandedPositionFunc: func() float64 {
			return adapter.rail.Get_commanded_position()
		},
		GetNameFunc: func(short bool) string {
			return adapter.rail.Get_name(short)
		},
	}
	return adapter
}

func projectRailsFromKinematicRails(rails []kinematicspkg.Rail) []*PrinterRail {
	projectRails := make([]*PrinterRail, len(rails))
	for i, rail := range rails {
		adapter, ok := rail.(*kinematicsRailAdapter)
		if !ok {
			panic(fmt.Errorf("kinematics rail has unexpected type %T", rail))
		}
		projectRails[i] = adapter.rail
	}
	return projectRails
}

func newKinematicsRailEndstopAdapter(endstop interface{}) kinematicspkg.RailEndstop {
	return &kinematicspkg.RailEndstopFuncs{AddStepperFunc: func(stepper kinematicspkg.Stepper) {
		mcuStepper, ok := stepper.(*mcupkg.LegacyStepper)
		if !ok {
			panic(fmt.Errorf("endstop stepper has unexpected type %T", stepper))
		}
		switch typed := endstop.(type) {
		case *mcupkg.LegacyEndstop:
			typed.Add_stepper(mcuStepper)
		case *ProbeEndstopWrapper:
			typed.AddStepper(mcuStepper)
		default:
			panic(fmt.Errorf("endstop has unexpected type %T", endstop))
		}
	}}
}

func newKinematicsHomingAdapter(homing *Homing) kinematicspkg.HomingState {
	return &kinematicspkg.HomingStateFuncs{
		GetAxesFunc: func() []int {
			return homing.Get_axes()
		},
		HomeRailsFunc: func(rails []kinematicspkg.Rail, forcepos []interface{}, homepos []interface{}) {
			projectRails := make([]*PrinterRail, len(rails))
			for i, rail := range rails {
				typed, ok := rail.(*kinematicsRailAdapter)
				if !ok {
					panic(fmt.Errorf("homing rail has unexpected type %T", rail))
				}
				projectRails[i] = typed.rail
			}
			homing.Home_rails(projectRails, forcepos, homepos)
		},
	}
}

type IExtruder = motionpkg.Extruder

const cmd_SET_VELOCITY_LIMIT_help = "Set printer velocity limits"

type Toolhead struct {
	Printer         *Printer
	Reactor         IReactor
	All_mcus        []*MCU
	Mcu             *MCU
	core            motionpkg.ToolheadCoreState
	lookahead       *LookAheadQueue
	Flush_timer     *ReactorTimer
	Priming_timer   *ReactorTimer
	Drip_completion *ReactorCompletion
	Trapq           interface{}
	Trapq_append    func(tq interface{}, print_time,
		accel_t, cruise_t, decel_t,
		start_pos_x, start_pos_y, start_pos_z,
		axes_r_x, axes_r_y, axes_r_z,
		start_v, cruise_v, accel float64)
	Trapq_finalize_moves     func(interface{}, float64, float64)
	Step_generators          []func(float64 float64)
	Coord                    []string
	Extruder                 IExtruder
	Kin                      IKinematics
	VelocityRangeLimit       [][2]float64
	VelocityRangeLimitHitLog bool
	move_transform           gcodepkg.LegacyMoveTransform
}

var (
	_ motionpkg.ToolheadMoveRuntime   = (*Toolhead)(nil)
	_ motionpkg.ToolheadDwellRuntime  = (*Toolhead)(nil)
	_ motionpkg.MoveBatchSink         = (*Toolhead)(nil)
	_ motionpkg.PauseTimeSource       = (*Toolhead)(nil)
	_ motionpkg.PrintTimeSource       = (*Toolhead)(nil)
	_ motionpkg.SyncPrintTimeNotifier = (*Toolhead)(nil)
)

func (self *Toolhead) Monotonic() float64 { return self.Reactor.Monotonic() }
func (self *Toolhead) EstimatedPrintTime(eventtime float64) float64 {
	return self.Mcu.Estimated_print_time(eventtime)
}
func (self *Toolhead) Pause(waketime float64) float64 { return self.Reactor.Pause(waketime) }
func (self *Toolhead) SyncPrintTime(curTime float64, estPrintTime float64, printTime float64) {
	_, _ = self.Printer.Send_event("toolhead:sync_print_time", []interface{}{curTime, estPrintTime, printTime})
}
func (self *Toolhead) CommandedPosition() []float64     { return self.core.CommandedPosition() }
func (self *Toolhead) MoveConfig() motionpkg.MoveConfig { return self.core.MoveConfig() }
func (self *Toolhead) SetCommandedPosition(position []float64) {
	self.core.SetCommandedPosition(position)
}
func (self *Toolhead) CheckKinematicMove(move *Move) { self.Kin.Check_move(move) }
func (self *Toolhead) CheckExtruderMove(move *Move)  { self.Extruder.Check_move(move) }
func (self *Toolhead) QueueMove(move *Move) {
	if self.lookahead.Add_move(move, self.Extruder) {
		self.lookahead.Flush(true)
	}
}
func (self *Toolhead) PrintTime() float64                         { return self.core.PrintTime }
func (self *Toolhead) NeedCheckPause() float64                    { return self.core.NeedCheckPause }
func (self *Toolhead) CheckPause()                                { self._Check_pause() }
func (self *Toolhead) GetLastMoveTime() float64                   { return self.Get_last_move_time() }
func (self *Toolhead) AdvanceMoveTime(nextPrintTime float64)      { self._advance_move_time(nextPrintTime) }
func (self *Toolhead) SubmitMove(newpos []float64, speed float64) { self.Move(newpos, speed) }
func (self *Toolhead) SetKinematicPosition(newpos []float64, homingAxes []int) {
	self.Kin.Set_position(newpos, homingAxes)
}
func (self *Toolhead) EmitSetPositionEvent() { self.Printer.Send_event("toolhead:set_position", nil) }
func (self *Toolhead) EmitManualMoveEvent()  { self.Printer.Send_event("toolhead:manual_move", nil) }
func (self *Toolhead) FlushStepGeneration()  { self.Flush_step_generation() }
func (self *Toolhead) CanPause() bool        { return self.core.CanPause }
func (self *Toolhead) NoteMovequeueActivity(mqTime float64, setStepGenTime bool) {
	self.Note_mcu_movequeue_activity(mqTime, setStepGenTime)
}
func (self *Toolhead) QueueKinematicMove(printTime float64, move *Move) {
	self.Trapq_append(self.Trapq, printTime, move.Accel_t, move.Cruise_t, move.Decel_t,
		move.Start_pos[0], move.Start_pos[1], move.Start_pos[2],
		move.Axes_r[0], move.Axes_r[1], move.Axes_r[2],
		move.Start_v, move.Cruise_v, move.Accel)
}
func (self *Toolhead) QueueExtruderMove(printTime float64, move *Move) {
	self.Extruder.Move(printTime, move)
}
func (self *Toolhead) WaitMoves() { self.Wait_moves() }
func (self *Toolhead) VelocitySettings() motionpkg.ToolheadVelocitySettings {
	return self.core.VelocitySettings()
}
func (self *Toolhead) ApplyVelocityLimitResult(result motionpkg.ToolheadVelocityLimitResult) {
	self.core.ApplyVelocityLimitResult(result)
}
func (self *Toolhead) SetRolloverInfo(msg string) {
	self.Printer.Set_rollover_info("toolhead", msg, true)
}
func (self *Toolhead) FlushLookahead() { self._flush_lookahead() }
func (self *Toolhead) WaitMovesState() motionpkg.ToolheadWaitMovesState {
	return self.core.WaitMovesState()
}
func (self *Toolhead) PauseState() motionpkg.ToolheadPauseState { return self.core.PauseState() }
func (self *Toolhead) ApplyPauseState(state motionpkg.ToolheadPauseState) {
	self.core.ApplyPauseState(state)
}
func (self *Toolhead) EnsurePrimingTimer(waketime float64) {
	if self.Priming_timer == nil {
		self.Priming_timer = self.Reactor.Register_timer(self.Priming_handler, constants.NEVER)
	}
	self.Reactor.Update_timer(self.Priming_timer, waketime)
}
func (self *Toolhead) SpecialQueuingState() string { return self.core.SpecialQueuingState }
func (self *Toolhead) ClearPrimingTimer() {
	self.Reactor.Unregister_timer(self.Priming_timer)
	self.Priming_timer = nil
}
func (self *Toolhead) SetCheckStallTime(value float64) { self.core.CheckStallTime = value }
func (self *Toolhead) FlushHandlerState() motionpkg.ToolheadFlushHandlerState {
	return self.core.FlushHandlerState()
}
func (self *Toolhead) AdvanceFlushTime(flushTime float64)  { self._advance_flush_time(flushTime) }
func (self *Toolhead) SetDoKickFlushTimer(value bool)      { self.core.DoKickFlushTimer = value }
func (self *Toolhead) KinFlushDelay() float64              { return self.core.KinFlushDelay }
func (self *Toolhead) FlushLookaheadQueue(lazy bool)       { self.lookahead.Flush(lazy) }
func (self *Toolhead) SetSpecialQueuingState(state string) { self.core.SpecialQueuingState = state }
func (self *Toolhead) SetNeedCheckPause(value float64)     { self.core.NeedCheckPause = value }
func (self *Toolhead) UpdateFlushTimer(waketime float64) {
	self.Reactor.Update_timer(self.Flush_timer, waketime)
}
func (self *Toolhead) SetLookaheadFlushTime(value float64) { self.lookahead.Set_flush_time(value) }
func (self *Toolhead) SetDripCompletion(completion motionpkg.DripCompletion) {
	runtimeCompletion, _ := completion.(*ReactorCompletion)
	self.Drip_completion = runtimeCompletion
}
func (self *Toolhead) ResetLookaheadQueue() { self.lookahead.Reset() }
func (self *Toolhead) FinalizeDripMoves()   { self.Trapq_finalize_moves(self.Trapq, constants.NEVER, 0) }
func (self *Toolhead) IsCommandError(recovered interface{}) bool {
	_, ok := recovered.(*CommandError)
	return ok
}
func (self *Toolhead) LastFlushTime() float64            { return self.core.LastFlushTime }
func (self *Toolhead) PrintStall() float64               { return self.core.PrintStall }
func (self *Toolhead) SetClearHistoryTime(value float64) { self.core.ClearHistoryTime = value }
func (self *Toolhead) CheckActiveDrivers(maxQueueTime float64, eventtime float64) {
	for _, m := range self.All_mcus {
		m.Check_active(maxQueueTime, eventtime)
	}
}
func (self *Toolhead) LookaheadEmpty() bool { return self.lookahead.Queue_len() == 0 }
func (self *Toolhead) KinematicsStatus(eventtime float64) map[string]interface{} {
	return self.Kin.Get_status(eventtime)
}
func (self *Toolhead) ExtruderName() string { return self.Extruder.Get_name() }

func Load_config_toolhead(config *ConfigWrapper) interface{} { return NewToolhead(config) }

func NewToolhead(config *ConfigWrapper) *Toolhead {
	self := &Toolhead{}
	self.Printer = config.Get_printer()
	self.Reactor = self.Printer.Get_reactor()
	object_arr := self.Printer.Lookup_objects("mcu")
	self.All_mcus = []*MCU{}
	for _, m := range object_arr {
		for k1, m1 := range m.(map[string]interface{}) {
			if strings.HasPrefix(k1, "mcu") {
				self.All_mcus = append(self.All_mcus, m1.(*MCU))
			}
		}
	}
	self.Mcu = self.All_mcus[0]
	self.core = motionpkg.NewToolheadCoreState(!self.Mcu.Is_fileoutput())
	self.lookahead = motionpkg.NewMoveQueue(self)
	self.lookahead.Set_flush_time(BUFFER_TIME_HIGH)
	self.core.ApplyVelocityLimitResult(motionpkg.BuildToolheadInitialVelocityResult(motionpkg.ReadToolheadVelocitySettings(config)))
	self.Drip_completion = nil
	self.Flush_timer = self.Reactor.Register_timer(self._flush_handler, constants.NEVER)
	self.Trapq = chelper.Trapq_alloc()
	self.Trapq_append = chelper.Trapq_append
	self.Trapq_finalize_moves = chelper.Trapq_finalize_moves
	self.Step_generators = []func(float64 float64){}
	gcode_obj := self.Printer.Lookup_object("gcode", object.Sentinel{})
	gcode := gcode_obj.(*GCodeDispatch)
	self.Coord = append([]string{}, gcode.Coord...)
	self.Extruder = NewDummyExtruder()
	kin_name := config.Get("kinematics", object.Sentinel{}, true)
	kinematics := Load_kinematics(kin_name.(string))
	self.Kin = kinematics(self, config)
	gcode.Register_command("G4", self.Cmd_G4, false, "")
	gcode.Register_command("M400", self.Cmd_M400, false, "")
	gcode.Register_command("SET_VELOCITY_LIMIT", self.Cmd_SET_VELOCITY_LIMIT, false, cmd_SET_VELOCITY_LIMIT_help)
	gcode.Register_command("M204", self.Cmd_M204, false, "")
	self.Printer.Register_event_handler("project:shutdown", self.Handle_shutdown)
	for _, moduleName := range motionpkg.DefaultToolheadSupportModules() {
		config.LoadObject(moduleName)
	}
	return self
}

func (self *Toolhead) _Toolhead()                                  { chelper.Trapq_free(self.Trapq) }
func (self *Toolhead) Get_transform() gcodepkg.LegacyMoveTransform { return self.move_transform }
func (self *Toolhead) timingConfig() motionpkg.ToolheadTimingConfig {
	return motionpkg.DefaultTimingConfig()
}
func (self *Toolhead) timingFlushActions() motionpkg.FlushActions {
	flushDrivers := make([]motionpkg.MoveQueueFlusher, 0, len(self.All_mcus))
	for _, m := range self.All_mcus {
		flushDrivers = append(flushDrivers, m)
	}
	return motionpkg.FlushActions{StepGenerators: self.Step_generators, FinalizeMoves: func(freeTime float64, clearHistoryTime float64) {
		self.Trapq_finalize_moves(self.Trapq, freeTime, clearHistoryTime)
	}, UpdateExtruderMoveTime: self.Extruder.Update_move_time, FlushDrivers: flushDrivers}
}
func (self *Toolhead) pauseConfig() motionpkg.ToolheadPauseConfig {
	return motionpkg.DefaultPauseConfig()
}
func (self *Toolhead) flushConfig() motionpkg.ToolheadFlushConfig {
	return motionpkg.DefaultFlushConfig()
}
func (self *Toolhead) dripConfig() motionpkg.ToolheadDripConfig { return motionpkg.DefaultDripConfig() }
func (self *Toolhead) dripMoveConfig() motionpkg.ToolheadDripMoveConfig {
	return motionpkg.DefaultDripMoveConfig()
}
func (self *Toolhead) Cmd_G4(arg interface{}) error {
	return motionpkg.HandleToolheadG4Command(self, arg.(*GCodeCommand))
}
func (self *Toolhead) Cmd_M400(gcmd interface{}) error {
	_ = gcmd
	return motionpkg.HandleToolheadM400Command(self)
}
func (self *Toolhead) Cmd_SET_VELOCITY_LIMIT(arg interface{}) error {
	result, queryOnly := motionpkg.HandleToolheadSetVelocityLimitCommand(self, arg.(*GCodeCommand))
	if queryOnly {
		logger.Debugf(result.Summary)
	}
	return nil
}
func (self *Toolhead) Cmd_M204(cmd interface{}) error {
	return motionpkg.HandleToolheadM204Command(self, cmd.(*GCodeCommand))
}
func (self *Toolhead) M204(accel float64) { motionpkg.ApplyToolheadAcceleration(self, accel) }
func (self *Toolhead) _advance_flush_time(flush_time float64) {
	self.core.AdvanceFlushTime(flush_time, self.timingConfig(), self.timingFlushActions())
}
func (self *Toolhead) _advance_move_time(next_print_time float64) {
	self.core.AdvanceMoveTime(next_print_time, self.timingConfig(), self.timingFlushActions())
}
func (self *Toolhead) _calc_print_time() { self.core.CalcPrintTime(self.timingConfig(), self, self) }
func (self *Toolhead) Process_moves(moves []*Move) {
	if err := motionpkg.ProcessToolheadMoveBatch(&self.core, moves, self, self._calc_print_time, self, self, self.Drip_completion, self.dripConfig()); err != nil {
		panic(err)
	}
}
func (self *Toolhead) _flush_lookahead() {
	reset := self.core.ResetAfterLookaheadFlush(BUFFER_TIME_HIGH)
	self.lookahead.Flush(false)
	self.lookahead.Set_flush_time(reset.LookaheadFlushTime)
}
func (self *Toolhead) Flush_step_generation() {
	self._flush_lookahead()
	self._advance_flush_time(self.core.StepGenTime)
	self.core.MinRestartTime = math.Max(self.core.MinRestartTime, self.core.PrintTime)
}
func (self *Toolhead) Get_last_move_time() float64 {
	if self.core.SpecialQueuingState != "" {
		self._flush_lookahead()
		self._calc_print_time()
	} else {
		self.lookahead.Flush(false)
	}
	return self.core.PrintTime
}
func (self *Toolhead) _Check_pause() { motionpkg.CheckToolheadPause(self, self, self.pauseConfig()) }
func (self *Toolhead) Priming_handler(eventtime float64) float64 {
	_ = eventtime
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Exception in priming_handler")
			self.Printer.Invoke_shutdown("Exception in priming_handler")
		}
	}()
	return motionpkg.HandleToolheadPrimingCallback(self)
}
func (self *Toolhead) _flush_handler(eventtime float64) float64 {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("Exception in flush_handler")
			self.Printer.Invoke_shutdown("Exception in flush_handler")
		}
	}()
	est_print_time := self.Mcu.Estimated_print_time(eventtime)
	return motionpkg.HandleToolheadFlushCallback(eventtime, est_print_time, self, self.flushConfig())
}
func (self *Toolhead) Get_position() []float64 { return self.core.CommandedPosition() }
func (self *Toolhead) GetPosition() []float64  { return self.Get_position() }
func (self *Toolhead) Set_position(newpos []float64, homingAxes []int) {
	motionpkg.ApplyToolheadSetPosition(self, newpos, homingAxes, func(printTime float64, position []float64) {
		chelper.Trapq_set_position(self.Trapq, printTime, position[0], position[1], position[2])
	})
}
func (self *Toolhead) SetPosition(newpos []float64, homingAxes []int) {
	self.Set_position(newpos, homingAxes)
}
func (self *Toolhead) Move(newpos []float64, speed float64) {
	motionpkg.RunToolheadMove(self, newpos, speed)
}
func (self *Toolhead) Manual_move(coord []interface{}, speed float64) {
	motionpkg.RunToolheadManualMove(self, coord, speed)
}
func (self *Toolhead) ManualMove(coord []interface{}, speed float64) { self.Manual_move(coord, speed) }
func (self *Toolhead) Dwell(delay float64)                           { motionpkg.RunToolheadDwell(self, delay) }
func (self *Toolhead) Wait_moves()                                   { motionpkg.HandleToolheadWaitMoves(self, self, self.pauseConfig()) }
func (self *Toolhead) Set_extruder(extruder IExtruder, extrude_pos float64) {
	self.Extruder = extruder
	self.core.CommandedPos[3] = extrude_pos
}
func (self *Toolhead) Get_extruder() IExtruder { return self.Extruder }
func (self *Toolhead) Drip_move(newpos []float64, speed float64, drip_completion *ReactorCompletion) error {
	motionpkg.RunToolheadDripMove(self, newpos, speed, drip_completion, self.dripMoveConfig())
	return nil
}
func (self *Toolhead) Stats(eventtime float64) (bool, string) {
	return motionpkg.BuildToolheadStatsReport(self, eventtime, MOVE_HISTORY_EXPIRE)
}
func (self *Toolhead) Check_busy(eventtime float64) (float64, float64, bool) {
	busyState := motionpkg.BuildToolheadBusyReport(self, eventtime)
	return busyState.PrintTime, busyState.EstimatedPrintTime, busyState.LookaheadEmpty
}
func (self *Toolhead) Get_status(eventtime float64) map[string]interface{} {
	return motionpkg.BuildToolheadStatusReport(self, eventtime)
}
func (self *Toolhead) Handle_shutdown([]interface{}) error {
	self.core.CanPause = false
	self.lookahead.Reset()
	return nil
}
func (self *Toolhead) Get_kinematics() interface{} { return self.Kin }
func (self *Toolhead) Get_trapq() interface{}      { return self.Trapq }
func (self *Toolhead) Register_step_generator(handler func(float64)) {
	self.Step_generators = append(self.Step_generators, handler)
}
func (self *Toolhead) Note_step_generation_scan_time(delay, old_delay float64) {
	self.Flush_step_generation()
	self.core.UpdateStepGenerationScanDelay(delay, old_delay, self.timingConfig())
}
func (self *Toolhead) Register_lookahead_callback(callback func(float64)) {
	last_move := self.lookahead.Get_last()
	if last_move == nil {
		callback(self.Get_last_move_time())
		return
	}
	last_move.Timing_callbacks = append(last_move.Timing_callbacks, callback)
}
func (self *Toolhead) RegisterLookaheadCallback(callback func(float64)) {
	self.Register_lookahead_callback(callback)
}
func (self *Toolhead) Note_mcu_movequeue_activity(mq_time float64, set_step_gen_time bool) {
	kickFlushTimer := self.core.NoteMovequeueActivity(mq_time, set_step_gen_time)
	if kickFlushTimer {
		self.Reactor.Update_timer(self.Flush_timer, constants.NOW)
	}
}
func (self *Toolhead) NoteMCUMovequeueActivity(mqTime float64, setStepGenTime bool) {
	self.Note_mcu_movequeue_activity(mqTime, setStepGenTime)
}
func (self *Toolhead) HomedAxes(eventtime float64) string {
	return motionpkg.ToolheadHomedAxes(self, eventtime)
}
func (self *Toolhead) NoteZNotHomed() { motionpkg.NoteToolheadZNotHomed(self.Get_kinematics()) }
func (self *Toolhead) Get_max_velocity() (float64, float64) {
	return self.core.MaxVelocity, self.core.MaxAccel
}
