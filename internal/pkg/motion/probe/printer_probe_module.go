package probe

import (
	"container/list"
	"fmt"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	printerpkg "goklipper/internal/pkg/printer"
	"reflect"
)

const HintTimeout = "If the probe did not move far enough to trigger, then consider reducing the Z axis minimum position so the probecan travel further (the Z minimum position can be negative)."

const cmdProbeHelp = "Probe Z-height at current XY position"
const cmdQueryProbeHelp = "Return the status of the z-probe"
const cmdProbeAccuracyHelp = "Probe Z-height accuracy at current XY position"
const cmdProbeCalibrateHelp = "Calibrate the probe's z_offset"
const cmdZOffsetApplyProbeHelp = "Adjust the probe's z_offset"

type probeModuleConfig interface {
	printerpkg.ModuleConfig
	Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64
	Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int
	Getchoice(option string, choices map[interface{}]interface{}, default1 interface{}, noteValid bool) interface{}
	SectionConfig(section string) printerpkg.ModuleConfig
	HasSection(section string) bool
}

type probeSectionConfig interface {
	Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64
}

func mustProbeSection(config printerpkg.ModuleConfig, section string) probeSectionConfig {
	sectionConfig, ok := config.(probeSectionConfig)
	if !ok {
		panic(fmt.Sprintf("probe section %q has unexpected config type %T", section, config))
	}
	return sectionConfig
}

type probeToolhead interface {
	Get_position() []float64
	Get_last_move_time() float64
	Manual_move([]interface{}, float64)
	Get_status(float64) map[string]interface{}
}

type probeHoming interface {
	Probing_move(interface{}, []float64, float64) []float64
}

type probeConfigWriter interface {
	Set(section, option, val string)
}

type probeGCodeMoveStatus interface {
	Get_status(float64) map[string]interface{}
}

type probePinsRegistry interface {
	RegisterChip(name string, chip interface{})
}

type probeMoveEndstops interface {
	Get_mcu_endstops() []interface{}
}

type probeRailEndstops interface {
	Get_endstops() []list.List
}

type probeModuleRuntime interface {
	ProbeCommandContext
	ProbeEventRuntime
	ProbeMotionRuntime
	ProbeRunContext
}

type PrinterProbeModule struct {
	printer   printerpkg.ModulePrinter
	name      string
	mcuProbe  interface{}
	gcodeMove probeGCodeMoveStatus
	gcode     printerpkg.GCodeRuntime
	core      *PrinterProbe
	runtime   probeModuleRuntime
}

func NewPrinterProbeModule(config printerpkg.ModuleConfig, mcuProbe interface{}) *PrinterProbeModule {
	self := &PrinterProbeModule{
		printer:  config.Printer(),
		name:     config.Name(),
		mcuProbe: mcuProbe,
		gcode:    config.Printer().GCode(),
	}
	richer, ok := config.(probeModuleConfig)
	if !ok {
		panic(fmt.Sprintf("probe module config %T is missing project probe helpers", config))
	}
	speed := richer.Getfloat("speed", 5.0, 0, 0, 0., 0, true)
	liftSpeed := richer.Getfloat("lift_speed", speed, 0, 0, 0., 0, true)
	xOffset := richer.Getfloat("x_offset", 0., 0, 0, 0., 0, true)
	yOffset := richer.Getfloat("y_offset", 0., 0, 0, 0., 0, true)
	zOffset := richer.Getfloat("z_offset", 0, 0, 0, 0., 0, true)
	finalSpeed := richer.Getfloat("final_speed", 2., 0, 0, 0., 0, true)
	zPosition := 0.0
	if richer.HasSection("stepper_z") {
		zPosition = mustProbeSection(richer.SectionConfig("stepper_z"), "stepper_z").Getfloat("position_min", 0., 0, 0, 0., 0, false)
	} else {
		zPosition = mustProbeSection(richer.SectionConfig("printer"), "printer").Getfloat("minimum_z_position", 0., 0, 0, 0., 0, false)
	}
	sampleCount := richer.Getint("samples", 1, 1, 0, false)
	sampleRetractDist := richer.Getfloat("sample_retract_dist", 2., 0, 0, 0., 0, false)
	choices := map[interface{}]interface{}{"median": "median", "average": "average", "weighted": "weighted"}
	samplesResult := richer.Getchoice("samples_result", choices, "average", true)
	samplesTolerance := richer.Getfloat("samples_tolerance", 0.100, 0., 0, 0, 0, false)
	samplesRetries := richer.Getint("samples_tolerance_retries", 0, 0, 0, false)
	self.core = NewPrinterProbe(
		speed,
		liftSpeed,
		xOffset,
		yOffset,
		zOffset,
		finalSpeed,
		zPosition,
		sampleCount,
		sampleRetractDist,
		samplesResult,
		samplesTolerance,
		samplesRetries,
	)
	if gcodeMove, ok := config.LoadObject("gcode_move").(probeGCodeMoveStatus); ok {
		self.gcodeMove = gcodeMove
	}
	return self
}

func (self *PrinterProbeModule) Core() *PrinterProbe {
	return self.core
}

func (self *PrinterProbeModule) LiftSpeed() float64 {
	return self.core.LiftSpeed
}

func (self *PrinterProbeModule) Name() string {
	return self.name
}

func (self *PrinterProbeModule) MCUProbe() interface{} {
	return self.mcuProbe
}

func (self *PrinterProbeModule) Printer() printerpkg.ModulePrinter {
	return self.printer
}

func (self *PrinterProbeModule) SetRuntime(runtime probeModuleRuntime) {
	self.runtime = runtime
	pins := self.printer.LookupObject("pins", object.Sentinel{})
	registry, ok := pins.(probePinsRegistry)
	if !ok {
		panic(fmt.Sprintf("probe pins registry has unexpected type %T", pins))
	}
	registry.RegisterChip("probe", self)
	self.printer.RegisterEventHandler("homing:homing_move_begin", self.Handle_homing_move_begin)
	self.printer.RegisterEventHandler("homing:homing_move_end", self.Handle_homing_move_end)
	self.printer.RegisterEventHandler("homing:home_rails_begin", self.Handle_home_rails_begin)
	self.printer.RegisterEventHandler("homing:home_rails_end", self.Handle_home_rails_end)
	self.printer.RegisterEventHandler("gcode:command_error", self.Handle_command_error)
	self.gcode.RegisterCommand("PROBE", self.handleProbeCommand, false, cmdProbeHelp)
	self.gcode.RegisterCommand("QUERY_PROBE", self.handleQueryProbeCommand, false, cmdQueryProbeHelp)
	self.gcode.RegisterCommand("PROBE_CALIBRATE", self.handleProbeCalibrateCommand, false, cmdProbeCalibrateHelp)
	self.gcode.RegisterCommand("PROBE_ACCURACY", self.handleProbeAccuracyCommand, false, cmdProbeAccuracyHelp)
	self.gcode.RegisterCommand("Z_OFFSET_APPLY_PROBE", self.handleZOffsetApplyProbeCommand, false, cmdZOffsetApplyProbeHelp)
}

func (self *PrinterProbeModule) runtimeContext() probeModuleRuntime {
	if self.runtime == nil {
		panic("probe runtime not configured")
	}
	return self.runtime
}

func flattenHomingMoveProbeEndstops(move interface{}) []interface{} {
	provider, ok := move.(probeMoveEndstops)
	if !ok {
		panic(fmt.Sprintf("probe homing move has unexpected type %T", move))
	}
	endstops := make([]interface{}, 0)
	for _, entry := range provider.Get_mcu_endstops() {
		switch typed := entry.(type) {
		case list.List:
			if typed.Front() != nil {
				endstops = append(endstops, typed.Front().Value)
			}
		case *list.List:
			if typed != nil && typed.Front() != nil {
				endstops = append(endstops, typed.Front().Value)
			}
		default:
			panic(fmt.Sprintf("probe homing move endstop entry has unexpected type %T", entry))
		}
	}
	return endstops
}

func flattenHomeRailProbeEndstops(rails interface{}) []interface{} {
	values := reflect.ValueOf(rails)
	if values.Kind() != reflect.Slice && values.Kind() != reflect.Array {
		panic(fmt.Sprintf("probe rails collection has unexpected type %T", rails))
	}
	endstops := make([]interface{}, 0)
	for i := 0; i < values.Len(); i++ {
		rail := values.Index(i).Interface()
		provider, ok := rail.(probeRailEndstops)
		if !ok {
			panic(fmt.Sprintf("probe rail has unexpected type %T", rail))
		}
		for _, entry := range provider.Get_endstops() {
			if entry.Front() != nil {
				endstops = append(endstops, entry.Front().Value)
			}
		}
	}
	return endstops
}

func (self *PrinterProbeModule) Handle_homing_move_begin(args []interface{}) error {
	HandleHomingMoveBegin(self.runtimeContext(), args[0], flattenHomingMoveProbeEndstops(args[0]))
	return nil
}

func (self *PrinterProbeModule) Handle_homing_move_end(args []interface{}) error {
	HandleHomingMoveEnd(self.runtimeContext(), args[0], flattenHomingMoveProbeEndstops(args[0]))
	return nil
}

func (self *PrinterProbeModule) Handle_home_rails_begin(args []interface{}) error {
	HandleHomeRailsBegin(self.runtimeContext(), flattenHomeRailProbeEndstops(args[1]))
	return nil
}

func (self *PrinterProbeModule) Handle_home_rails_end(args []interface{}) error {
	HandleHomeRailsEnd(self.runtimeContext(), flattenHomeRailProbeEndstops(args[1]))
	return nil
}

func (self *PrinterProbeModule) Handle_command_error(args []interface{}) error {
	_ = args
	HandleCommandError(self.runtimeContext())
	return nil
}

func (self *PrinterProbeModule) Setup_pin(pinType string, pinParams map[string]interface{}) interface{} {
	ValidateVirtualEndstopPin(pinType, cast.ToString(pinParams["pin"]), cast.ToInt(pinParams["invert"]) != 0, cast.ToInt(pinParams["pullup"]) != 0)
	return self.mcuProbe
}

func (self *PrinterProbeModule) Get_offsets() (float64, float64, float64) {
	return self.core.GetOffsets()
}

func (self *PrinterProbeModule) Move(coord interface{}, speed float64) {
	toolheadObj := self.printer.LookupObject("toolhead", object.Sentinel{})
	toolhead, ok := toolheadObj.(probeToolhead)
	if !ok {
		panic(fmt.Sprintf("probe toolhead has unexpected type %T", toolheadObj))
	}
	if typed, ok := coord.([]*float64); ok {
		coordInterface := make([]interface{}, len(typed))
		for i, item := range typed {
			if item != nil {
				coordInterface[i] = *item
				continue
			}
			coordInterface[i] = nil
		}
		toolhead.Manual_move(coordInterface, speed)
		return
	}
	toolhead.Manual_move(coord.([]interface{}), speed)
}

func (self *PrinterProbeModule) Probe(speed float64) []float64 {
	return RunProbeMove(self.runtimeContext(), speed)
}

func (self *PrinterProbeModule) Run_probe(command interface{}) []float64 {
	return RunProbeSequence(self.runtimeContext(), self.asProbeCommand(command))
}

func (self *PrinterProbeModule) RunProbeCommand(command ProbeCommand) []float64 {
	return self.Run_probe(command)
}

func (self *PrinterProbeModule) BeginMultiProbe() {
	BeginMultiProbe(self.runtimeContext())
}

func (self *PrinterProbeModule) EndMultiProbe() {
	EndMultiProbe(self.runtimeContext())
}

func (self *PrinterProbeModule) GetLiftSpeed(command ProbeCommand) float64 {
	return LiftSpeedFromCommand(command, self.LiftSpeed())
}

func (self *PrinterProbeModule) GetOffsets() []float64 {
	x, y, z := self.Get_offsets()
	return []float64{x, y, z}
}

func (self *PrinterProbeModule) RunProbe(command ProbeCommand) []float64 {
	return self.RunProbeCommand(command)
}

func (self *PrinterProbeModule) Cmd_PROBE(arg interface{}) error {
	return HandleProbeCommand(self.runtimeContext(), self.asProbeCommand(arg))
}

func (self *PrinterProbeModule) Cmd_QUERY_PROBE(arg interface{}) error {
	return HandleQueryProbeCommand(self.runtimeContext(), self.asProbeCommand(arg))
}

func (self *PrinterProbeModule) Cmd_PROBE_ACCURACY(arg interface{}) error {
	return HandleProbeAccuracyCommand(self.runtimeContext(), self.asProbeCommand(arg))
}

func (self *PrinterProbeModule) Cmd_PROBE_CALIBRATE(arg interface{}) error {
	return HandleProbeCalibrateCommand(self.runtimeContext(), self.asProbeCommand(arg))
}

func (self *PrinterProbeModule) Cmd_Z_OFFSET_APPLY_PROBE(arg interface{}) error {
	_ = arg
	return HandleZOffsetApplyProbeCommand(self.runtimeContext())
}

func (self *PrinterProbeModule) handleProbeCommand(command printerpkg.Command) error {
	return self.Cmd_PROBE(command)
}

func (self *PrinterProbeModule) handleQueryProbeCommand(command printerpkg.Command) error {
	return self.Cmd_QUERY_PROBE(command)
}

func (self *PrinterProbeModule) handleProbeAccuracyCommand(command printerpkg.Command) error {
	return self.Cmd_PROBE_ACCURACY(command)
}

func (self *PrinterProbeModule) handleProbeCalibrateCommand(command printerpkg.Command) error {
	return self.Cmd_PROBE_CALIBRATE(command)
}

func (self *PrinterProbeModule) handleZOffsetApplyProbeCommand(command printerpkg.Command) error {
	return self.Cmd_Z_OFFSET_APPLY_PROBE(command)
}

func (self *PrinterProbeModule) asProbeCommand(command interface{}) ProbeCommand {
	if probeCommand, ok := command.(ProbeCommand); ok {
		return probeCommand
	}
	if genericCommand, ok := command.(printerpkg.Command); ok {
		return &probeCommandAdapter{command: genericCommand}
	}
	panic(fmt.Sprintf("probe command has unexpected type %T", command))
}

type probeCommandAdapter struct {
	command printerpkg.Command
}

func (self *probeCommandAdapter) Get(name string, defaultValue interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string {
	_ = parser
	_ = minval
	_ = maxval
	_ = above
	_ = below
	if params := self.command.Parameters(); params != nil {
		if value, ok := params[name]; ok {
			return value
		}
	}
	return cast.ToString(defaultValue)
}

func (self *probeCommandAdapter) Get_int(name string, defaultValue interface{}, minval *int, maxval *int) int {
	defaultInt := cast.ToInt(defaultValue)
	return self.command.Int(name, defaultInt, minval, maxval)
}

func (self *probeCommandAdapter) Get_float(name string, defaultValue interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64 {
	_ = minval
	_ = maxval
	_ = above
	_ = below
	defaultFloat := cast.ToFloat64(defaultValue)
	return self.command.Float(name, defaultFloat)
}

func (self *probeCommandAdapter) RespondInfo(msg string, log bool) {
	self.command.RespondInfo(msg, log)
}
