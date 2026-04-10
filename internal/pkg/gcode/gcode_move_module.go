package gcode

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

type LegacyMoveTransform interface {
	Move([]float64, float64)
	Get_position() []float64
}

type legacyGCodeCommand interface {
	printerpkg.Command
	Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	Get_int(name string, _default interface{}, minval *int, maxval *int) int
	Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64
	Get_command_parameters() map[string]string
	Get_commandline() string
}

type legacyToolheadRuntime interface {
	Move([]float64, float64)
	Get_position() []float64
	Get_transform() LegacyMoveTransform
	Get_kinematics() interface{}
}

type homingAxesSource interface {
	Get_axes() []int
}

type stepperCollection interface {
	Get_steppers() []interface{}
}

type stepperState interface {
	Get_name(bool) string
	Get_mcu_position() int
	Get_commanded_position() float64
}

type positionCalculator interface {
	Calc_position(map[string]float64) []float64
}

type savedState struct {
	absoluteCoord   bool
	absoluteExtrude bool
	basePosition    []float64
	lastPosition    []float64
	homingPosition  []float64
	speed           float64
	speedFactor     float64
	extrudeFactor   float64
}

type GCodeMoveModule struct {
	printer                  printerpkg.ModulePrinter
	gcode                    printerpkg.GCodeRuntime
	isPrinterReady           bool
	absoluteCoord            bool
	absoluteExtrude          bool
	basePosition             []float64
	lastPosition             []float64
	homingPosition           []float64
	speed                    float64
	speedFactor              float64
	extrudeFactor            float64
	savedStates              map[string]*savedState
	moveTransform            LegacyMoveTransform
	moveWithTransform        func([]float64, float64)
	positionWithTransform    func() []float64
	cmdSetGCodeOffsetHelp    string
	cmdSaveGCodeStateHelp    string
	cmdRestoreGCodeStateHelp string
	cmdGetPositionHelp       string
}

func LoadConfigGCodeMove(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	self := &GCodeMoveModule{
		printer:                  printer,
		gcode:                    printer.GCode(),
		isPrinterReady:           false,
		absoluteCoord:            true,
		absoluteExtrude:          true,
		basePosition:             []float64{0.0, 0.0, 0.0, 0.0},
		lastPosition:             []float64{0.0, 0.0, 0.0, 0.0},
		homingPosition:           []float64{0.0, 0.0, 0.0, 0.0},
		speed:                    25.0,
		speedFactor:              1.0 / 60.0,
		extrudeFactor:            1.0,
		savedStates:              map[string]*savedState{},
		moveTransform:            nil,
		moveWithTransform:        nil,
		positionWithTransform:    func() []float64 { return []float64{0.0, 0.0, 0.0, 0.0} },
		cmdSetGCodeOffsetHelp:    "Set a virtual offset to g-code positions",
		cmdSaveGCodeStateHelp:    "Save G-Code coordinate state",
		cmdRestoreGCodeStateHelp: "Restore a previously saved G-Code state",
		cmdGetPositionHelp:       "Return information on the current location of the toolhead",
	}
	printer.RegisterEventHandler("project:ready", self._handle_ready)
	printer.RegisterEventHandler("project:shutdown", self._handle_shutdown)
	printer.RegisterEventHandler("toolhead:set_position", self.Reset_last_position)
	printer.RegisterEventHandler("toolhead:manual_move", self.Reset_last_position)
	printer.RegisterEventHandler("gcode:command_error", self.Reset_last_position)
	printer.RegisterEventHandler("extruder:activate_extruder", self._handle_activate_extruder)
	printer.RegisterEventHandler("homing:home_rails_end", self._handle_home_rails_end)

	self.gcode.RegisterCommand("G1", self.wrapLegacy(self.Cmd_G1), false, "")
	self.gcode.RegisterCommand("G20", self.wrapLegacy(self.Cmd_G20), false, "")
	self.gcode.RegisterCommand("G21", self.wrapLegacy(self.Cmd_G21), false, "")
	self.gcode.RegisterCommand("M82", self.wrapLegacy(self.Cmd_M82), false, "")
	self.gcode.RegisterCommand("M83", self.wrapLegacy(self.Cmd_M83), false, "")
	self.gcode.RegisterCommand("G90", self.wrapLegacy(self.Cmd_G90), false, "")
	self.gcode.RegisterCommand("G91", self.wrapLegacy(self.Cmd_G91), false, "")
	self.gcode.RegisterCommand("G92", self.wrapLegacy(self.Cmd_G92), false, "")
	self.gcode.RegisterCommand("M220", self.wrapLegacy(self.Cmd_M220), false, "")
	self.gcode.RegisterCommand("M221", self.wrapLegacy(self.cmd_M221), false, "")
	self.gcode.RegisterCommand("SET_GCODE_OFFSET", self.wrapLegacyVoid(self.Cmd_SET_GCODE_OFFSET), false, self.cmdSetGCodeOffsetHelp)
	self.gcode.RegisterCommand("SAVE_GCODE_STATE", self.wrapLegacyVoid(self.Cmd_SAVE_GCODE_STATE), false, self.cmdSaveGCodeStateHelp)
	self.gcode.RegisterCommand("RESTORE_GCODE_STATE", self.wrapLegacyVoid(self.Cmd_RESTORE_GCODE_STATE), false, self.cmdRestoreGCodeStateHelp)
	self.gcode.RegisterCommand("G0", self.wrapLegacy(self.Cmd_G1), false, "")
	self.gcode.RegisterCommand("M114", self.wrapLegacy(self.Cmd_M114), true, "")
	self.gcode.RegisterCommand("GET_POSITION", self.wrapLegacyVoid(self.Cmd_GET_POSITION), true, self.cmdGetPositionHelp)
	return self
}

func (self *GCodeMoveModule) wrapLegacy(handler func(interface{}) error) func(printerpkg.Command) error {
	return func(command printerpkg.Command) error {
		return handler(self.requireLegacyCommand(command))
	}
}

func (self *GCodeMoveModule) wrapLegacyVoid(handler func(interface{})) func(printerpkg.Command) error {
	return func(command printerpkg.Command) error {
		handler(self.requireLegacyCommand(command))
		return nil
	}
}

func (self *GCodeMoveModule) requireLegacyCommand(command printerpkg.Command) legacyGCodeCommand {
	legacy, ok := command.(legacyGCodeCommand)
	if !ok {
		panic("gcode_move requires legacy command helpers")
	}
	return legacy
}

func (self *GCodeMoveModule) lookupToolhead() legacyToolheadRuntime {
	toolheadObj := self.printer.LookupObject("toolhead", nil)
	if toolheadObj == nil {
		return nil
	}
	toolhead, ok := toolheadObj.(legacyToolheadRuntime)
	if !ok {
		panic("gcode_move requires legacy toolhead helpers")
	}
	return toolhead
}

func (self *GCodeMoveModule) _handle_ready(args []interface{}) error {
	self.isPrinterReady = true
	toolhead := self.lookupToolhead()
	if toolhead == nil {
		panic("gcode_move requires toolhead runtime")
	}
	if self.moveTransform == nil {
		self.moveWithTransform = toolhead.Move
		self.positionWithTransform = toolhead.Get_position
	}
	self.Reset_last_position(nil)
	return nil
}

func (self *GCodeMoveModule) _handle_shutdown(args []interface{}) error {
	if !self.isPrinterReady {
		return nil
	}

	self.isPrinterReady = false
	logger.Infof("gcode state: absolute_coord=%v absolute_extrude=%v "+
		"base_position=%v last_position=%v "+
		"homing_position=%v speed_factor=%v "+
		"extrude_factor=%v speed=%v",
		self.absoluteCoord, self.absoluteExtrude,
		self.basePosition, self.lastPosition,
		self.homingPosition, self.speedFactor,
		self.extrudeFactor, self.speed)
	return nil
}

func (self *GCodeMoveModule) _handle_activate_extruder(args []interface{}) error {
	_ = self.Reset_last_position(nil)
	self.extrudeFactor = 1.
	self.basePosition[3] = self.lastPosition[3]
	return nil
}

func (self *GCodeMoveModule) _handle_home_rails_end(args []interface{}) error {
	self.Reset_last_position(nil)
	if len(args) == 0 {
		return nil
	}
	homingState, ok := args[0].(homingAxesSource)
	if !ok {
		panic("gcode_move requires homing axes helper")
	}
	for _, axis := range homingState.Get_axes() {
		self.basePosition[axis] = self.homingPosition[axis]
	}
	return nil
}

func (self *GCodeMoveModule) Set_move_transform(transform LegacyMoveTransform, force bool) LegacyMoveTransform {
	if self.moveTransform != nil && !force {
		panic("G-Code move transform already specified")
	}

	oldTransform := self.moveTransform
	if oldTransform == nil {
		toolhead := self.lookupToolhead()
		if toolhead != nil {
			oldTransform = toolhead.Get_transform()
		}
	}
	self.moveTransform = transform
	self.moveWithTransform = transform.Move
	self.positionWithTransform = transform.Get_position
	return oldTransform
}

func (self *GCodeMoveModule) SetMoveTransform(transform printerpkg.MoveTransform, force bool) printerpkg.MoveTransform {
	oldTransform := self.Set_move_transform(&moduleMoveTransformAdapter{inner: transform}, force)
	if oldTransform == nil {
		return nil
	}
	return &legacyMoveTransformAdapter{inner: oldTransform}
}

func (self *GCodeMoveModule) _get_gcode_position() []float64 {
	p := make([]float64, 0, len(self.lastPosition))
	length := int(math.Min(float64(len(self.lastPosition)), float64(len(self.basePosition))))
	for i := 0; i < length; i++ {
		p = append(p, self.lastPosition[i]-self.basePosition[i])
	}
	p[3] /= self.extrudeFactor
	return p
}

func (self *GCodeMoveModule) _get_gcode_speed() float64 {
	return self.speed / self.speedFactor
}

func (self *GCodeMoveModule) _get_gcode_speed_override() float64 {
	return self.speedFactor * 60.
}

func (self *GCodeMoveModule) Get_status(eventtime float64) map[string]interface{} {
	movePosition := self._get_gcode_position()
	return map[string]interface{}{
		"speed_factor":         self._get_gcode_speed_override(),
		"speed":                self._get_gcode_speed(),
		"extrude_factor":       self.extrudeFactor,
		"absolute_coordinates": self.absoluteCoord,
		"absolute_extrude":     self.absoluteExtrude,
		"homing_origin":        []float64{self.homingPosition[0], self.homingPosition[1], self.homingPosition[2], self.homingPosition[3]},
		"position":             []float64{self.lastPosition[0], self.lastPosition[1], self.lastPosition[2], self.lastPosition[3]},
		"gcode_position":       []float64{movePosition[0], movePosition[1], movePosition[2], movePosition[3]},
	}
}

func (self *GCodeMoveModule) State() printerpkg.GCodeMoveState {
	status := self.Get_status(0.)
	position := status["gcode_position"].([]float64)
	positionCopy := make([]float64, len(position))
	copy(positionCopy, position)
	return printerpkg.GCodeMoveState{
		GCodePosition:       positionCopy,
		AbsoluteCoordinates: status["absolute_coordinates"].(bool),
		AbsoluteExtrude:     status["absolute_extrude"].(bool),
	}
}

func (self *GCodeMoveModule) GCodePositionZ() float64 {
	return self.Get_status(0.)["gcode_position"].([]float64)[2]
}

func (self *GCodeMoveModule) Reset_last_position(args []interface{}) error {
	if self.isPrinterReady && self.positionWithTransform != nil {
		self.lastPosition = self.positionWithTransform()
	}
	return nil
}

func (self *GCodeMoveModule) ResetLastPosition() {
	self.Reset_last_position(nil)
}

func (self *GCodeMoveModule) applyMoveParameters(params map[string]string, commandline string) error {
	for pos, axis := range strings.Split("X Y Z", " ") {
		if raw, ok := params[axis]; ok {
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				logger.Error(err)
				continue
			}
			if !self.absoluteCoord {
				self.lastPosition[pos] += value
			} else {
				self.lastPosition[pos] = value + self.basePosition[pos]
			}
		}
	}

	if raw, ok := params["E"]; ok {
		eVal, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			logger.Error(err)
		} else {
			value := eVal * self.extrudeFactor
			if !self.absoluteCoord || !self.absoluteExtrude {
				self.lastPosition[3] += value
			} else {
				self.lastPosition[3] = value + self.basePosition[3]
			}
		}
	}

	if raw, ok := params["F"]; ok {
		gcodeSpeed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			panic(fmt.Sprintf("Unable to parse move '%s'", commandline))
		}
		if gcodeSpeed <= 0 {
			panic(fmt.Sprintf("Invalid speed in '%s'", commandline))
		}
		self.speed = gcodeSpeed * self.speedFactor
	}
	if self.moveWithTransform == nil {
		panic("gcode_move not ready")
	}
	self.moveWithTransform(self.lastPosition, self.speed)
	return nil
}

func (self *GCodeMoveModule) LinearMove(params map[string]string) error {
	return self.applyMoveParameters(params, "G1")
}

func (self *GCodeMoveModule) Cmd_G1(argv interface{}) error {
	gcmd := argv.(legacyGCodeCommand)
	return self.applyMoveParameters(gcmd.Get_command_parameters(), gcmd.Get_commandline())
}

func (self *GCodeMoveModule) Cmd_G20(argv interface{}) error {
	panic("Machine does not support G20 (inches) command")
}

func (self *GCodeMoveModule) Cmd_G21(argv interface{}) error {
	return nil
}

func (self *GCodeMoveModule) Cmd_M82(argv interface{}) error {
	self.absoluteExtrude = true
	return nil
}

func (self *GCodeMoveModule) Cmd_M83(argv interface{}) error {
	self.absoluteExtrude = false
	return nil
}

func (self *GCodeMoveModule) Cmd_G90(argv interface{}) error {
	self.absoluteCoord = true
	return nil
}

func (self *GCodeMoveModule) Cmd_G91(argv interface{}) error {
	self.absoluteCoord = false
	return nil
}

func (self *GCodeMoveModule) Cmd_G92(argv interface{}) error {
	gcmd := argv.(legacyGCodeCommand)
	offsets := make([]interface{}, 0, 4)
	noOffsets := true
	for _, axis := range strings.Split("X Y Z E", " ") {
		value := gcmd.Get_float(axis, math.NaN(), nil, nil, nil, nil)
		if math.IsNaN(value) {
			offsets = append(offsets, nil)
			continue
		}
		noOffsets = false
		offsets = append(offsets, value)
	}

	for i, offset := range offsets {
		if offset == nil {
			continue
		}
		value := offset.(float64)
		if i == 3 {
			value *= self.extrudeFactor
		}
		self.basePosition[i] = self.lastPosition[i] - value
	}
	if noOffsets {
		copy(self.basePosition, self.lastPosition)
	}
	return nil
}

func (self *GCodeMoveModule) Cmd_M114(argv interface{}) error {
	gcmd := argv.(legacyGCodeCommand)
	p := self._get_gcode_position()
	gcmd.RespondRaw(fmt.Sprintf("X:%.3f Y:%.3f Z:%.3f E:%.3f", p[0], p[1], p[2], p[3]))
	return nil
}

func (self *GCodeMoveModule) Cmd_M220(argv interface{}) error {
	gcmd := argv.(legacyGCodeCommand)
	zero := 0.
	value := gcmd.Get_float("S", 100., nil, nil, &zero, nil) / (60. * 100.)
	self.speed = self._get_gcode_speed() * value
	self.speedFactor = value
	return nil
}

func (self *GCodeMoveModule) cmd_M221(argv interface{}) error {
	gcmd := argv.(legacyGCodeCommand)
	above := 0.
	newExtrudeFactor := gcmd.Get_float("S", 100., nil, nil, &above, nil) / 100.
	lastEPos := self.lastPosition[3]
	eValue := (lastEPos - self.basePosition[3]) / self.extrudeFactor
	self.basePosition[3] = lastEPos - eValue*newExtrudeFactor
	self.extrudeFactor = newExtrudeFactor
	return nil
}

func (self *GCodeMoveModule) Cmd_SET_GCODE_OFFSET(argv interface{}) {
	gcmd := argv.(legacyGCodeCommand)
	moveDelta := []float64{0., 0., 0., 0.}
	for pos, axis := range strings.Split("X Y Z E", " ") {
		offset := gcmd.Get_float(axis, math.NaN(), nil, nil, nil, nil)
		if math.IsNaN(offset) {
			offset = gcmd.Get_float(axis+"_ADJUST", math.NaN(), nil, nil, nil, nil)
			if math.IsNaN(offset) {
				continue
			}
			offset += self.homingPosition[pos]
		}
		delta := offset - self.homingPosition[pos]
		moveDelta[pos] = delta
		self.basePosition[pos] += delta
		self.homingPosition[pos] = offset
	}
	if gcmd.Get_int("MOVE", 0, nil, nil) != 0 {
		speed := gcmd.Get_float("MOVE_SPEED", self.speed, nil, nil, nil, nil)
		for pos, delta := range moveDelta {
			self.lastPosition[pos] += delta
		}
		self.moveWithTransform(self.lastPosition, speed)
	}
}

func (self *GCodeMoveModule) Cmd_SAVE_GCODE_STATE(argv interface{}) {
	gcmd := argv.(legacyGCodeCommand)
	stateName := gcmd.Get("NAME", "default", nil, nil, nil, nil, nil)
	basePosition := append([]float64{}, self.basePosition...)
	lastPosition := append([]float64{}, self.lastPosition...)
	homingPosition := append([]float64{}, self.homingPosition...)
	self.savedStates[stateName] = &savedState{
		absoluteCoord:   self.absoluteCoord,
		absoluteExtrude: self.absoluteExtrude,
		basePosition:    basePosition,
		lastPosition:    lastPosition,
		homingPosition:  homingPosition,
		speed:           self.speed,
		speedFactor:     self.speedFactor,
		extrudeFactor:   self.extrudeFactor,
	}
}

func (self *GCodeMoveModule) Cmd_RESTORE_GCODE_STATE(argv interface{}) {
	gcmd := argv.(legacyGCodeCommand)
	stateName := gcmd.Get("NAME", "default", nil, nil, nil, nil, nil)
	state := self.savedStates[stateName]
	if state == nil {
		panic(fmt.Sprintf("Unknown g-code state: %s", stateName))
	}
	self.absoluteCoord = state.absoluteCoord
	self.absoluteExtrude = state.absoluteExtrude
	self.basePosition = append([]float64{}, state.basePosition...)
	self.homingPosition = append([]float64{}, state.homingPosition...)
	self.speed = state.speed
	self.speedFactor = state.speedFactor
	self.extrudeFactor = state.extrudeFactor
	eDiff := self.lastPosition[3] - state.lastPosition[3]
	self.basePosition[3] += eDiff
	if gcmd.Get_int("MOVE", 0, nil, nil) != 0 {
		zero := 0.
		speed := gcmd.Get_float("MOVE_SPEED", self.speed, nil, nil, &zero, nil)
		self.lastPosition[0] = state.lastPosition[0]
		self.lastPosition[1] = state.lastPosition[1]
		self.lastPosition[2] = state.lastPosition[2]
		self.moveWithTransform(self.lastPosition, speed)
	}
}

func (self *GCodeMoveModule) Cmd_GET_POSITION(argv interface{}) {
	gcmd := argv.(legacyGCodeCommand)
	toolhead := self.lookupToolhead()
	if toolhead == nil {
		panic("Printer not ready")
	}
	kinematics := toolhead.Get_kinematics()
	steppersProvider, ok := kinematics.(stepperCollection)
	if !ok {
		panic("gcode_move requires stepper collection helper")
	}
	calcPosition, ok := kinematics.(positionCalculator)
	if !ok {
		panic("gcode_move requires kinematics position calculator")
	}
	steppers := steppersProvider.Get_steppers()

	var mcuPos strings.Builder
	for _, stepperObj := range steppers {
		stepper, ok := stepperObj.(stepperState)
		if !ok {
			panic("gcode_move requires stepper status helper")
		}
		mcuPos.WriteString(fmt.Sprintf("%s:%d ", stepper.Get_name(false), stepper.Get_mcu_position()))
	}

	cinfo := make(map[string]float64, len(steppers))
	var stepperPos strings.Builder
	for _, stepperObj := range steppers {
		stepper := stepperObj.(stepperState)
		cinfo[stepper.Get_name(false)] = stepper.Get_commanded_position()
		stepperPos.WriteString(fmt.Sprintf("%s:%.6f ", stepper.Get_name(false), stepper.Get_commanded_position()))
	}

	kinfo := make(map[string][]float64, 3)
	for _, axis := range strings.Split("X Y Z", " ") {
		kinfo[axis] = calcPosition.Calc_position(cinfo)
	}

	var kinPos strings.Builder
	for axis, values := range kinfo {
		kinPos.WriteString(fmt.Sprintf("%s:%.6f ", axis, values))
	}

	var toolheadPos strings.Builder
	axisNames := []string{"X", "Y", "Z", "E"}
	position := toolhead.Get_position()
	for i := 0; i < len(axisNames); i++ {
		toolheadPos.WriteString(fmt.Sprintf("%s:%.6f", axisNames[i], position[i]))
	}

	var gcodePos strings.Builder
	for i := 0; i < len(axisNames); i++ {
		gcodePos.WriteString(fmt.Sprintf("%s:%.6f", axisNames[i], self.lastPosition[i]))
	}

	var basePos strings.Builder
	for i := 0; i < len(axisNames); i++ {
		basePos.WriteString(fmt.Sprintf("%s:%.6f", axisNames[i], self.basePosition[i]))
	}

	var homingPos strings.Builder
	for i, axis := range []string{"X", "Y", "Z"} {
		homingPos.WriteString(fmt.Sprintf("%s:%.6f", axis, self.homingPosition[i]))
	}

	msg := fmt.Sprintf("mcu: %s\n"+
		"stepper: %s\n"+
		"kinematic: %s\n"+
		"toolhead: %s\n"+
		"gcode: %s\n"+
		"gcode base: %s\n"+
		"gcode homing: %s",
		mcuPos.String(), stepperPos.String(), kinPos.String(), toolheadPos.String(),
		gcodePos.String(), basePos.String(), homingPos.String())
	gcmd.RespondInfo(msg, true)
}

type moduleMoveTransformAdapter struct {
	inner printerpkg.MoveTransform
}

func (self *moduleMoveTransformAdapter) Move(newpos []float64, speed float64) {
	self.inner.Move(newpos, speed)
}

func (self *moduleMoveTransformAdapter) Get_position() []float64 {
	return self.inner.GetPosition()
}

type legacyMoveTransformAdapter struct {
	inner LegacyMoveTransform
}

func (self *legacyMoveTransformAdapter) GetPosition() []float64 {
	return self.inner.Get_position()
}

func (self *legacyMoveTransformAdapter) Move(newpos []float64, speed float64) {
	self.inner.Move(newpos, speed)
}
