// Helper code for implementing homing operations
//
// Copyright (C) 2016-2021  Kevin O"Connor <kevin@koconnor.net>
//
// This file may be distributed under the terms of the GNU GPLv3 license.
package project

import (
	"container/list"
	"fmt"
	"goklipper/common/utils/collections"
	"goklipper/common/utils/object"
	mcupkg "goklipper/internal/pkg/mcu"
	homingpkg "goklipper/internal/pkg/motion/homing"
)

const (
	HOMING_START_DELAY   = homingpkg.HomingStartDelay
	ENDSTOP_SAMPLE_TIME  = homingpkg.EndstopSampleTime
	ENDSTOP_SAMPLE_COUNT = homingpkg.EndstopSampleCount
)

type homingCompletionAdapter struct {
	runtime *ReactorCompletion
}

func (self *homingCompletionAdapter) Wait(waketime float64, waketimeResult interface{}) interface{} {
	return self.runtime.Wait(waketime, waketimeResult)
}

func (self *homingCompletionAdapter) Complete(result interface{}) {
	self.runtime.Complete(result)
}

func unwrapHomingCompletion(completion homingpkg.Completion) (*ReactorCompletion, error) {
	adapter, ok := completion.(*homingCompletionAdapter)
	if !ok {
		return nil, fmt.Errorf("homing completion has unexpected type %T", completion)
	}
	return adapter.runtime, nil
}

type homingReactorAdapter struct {
	reactor IReactor
}

func (self *homingReactorAdapter) RegisterCallback(callback func(interface{}) interface{}, waketime float64) homingpkg.Completion {
	return &homingCompletionAdapter{runtime: self.reactor.Register_callback(callback, waketime)}
}

type homingStepperAdapter struct {
	stepper *MCU_stepper
}

func (self *homingStepperAdapter) GetName(short bool) string {
	return self.stepper.Get_name(short)
}

func (self *homingStepperAdapter) GetMCUPosition() int {
	return self.stepper.Get_mcu_position()
}

func (self *homingStepperAdapter) GetPastMCUPosition(printTime float64) int {
	return self.stepper.Get_past_mcu_position(printTime)
}

func (self *homingStepperAdapter) CalcPositionFromCoord(coord []float64) float64 {
	return self.stepper.Calc_position_from_coord(coord)
}

func (self *homingStepperAdapter) GetStepDist() float64 {
	return self.stepper.Get_step_dist()
}

func (self *homingStepperAdapter) GetCommandedPosition() float64 {
	return self.stepper.Get_commanded_position()
}

type homingKinematicsAdapter struct {
	kinematics IKinematics
}

func (self *homingKinematicsAdapter) GetSteppers() []homingpkg.Stepper {
	steppers := self.kinematics.Get_steppers()
	adapted := make([]homingpkg.Stepper, len(steppers))
	for i, stepper := range steppers {
		mcuStepper, ok := stepper.(*MCU_stepper)
		if !ok {
			panic(fmt.Errorf("homing kinematics stepper has unexpected type %T", stepper))
		}
		adapted[i] = &homingStepperAdapter{stepper: mcuStepper}
	}
	return adapted
}

func (self *homingKinematicsAdapter) CalcPosition(stepperPositions map[string]float64) []float64 {
	return self.kinematics.Calc_position(stepperPositions)
}

type homingDripToolheadAdapter struct {
	toolhead IToolhead
}

func (self *homingDripToolheadAdapter) GetPosition() []float64 {
	return self.toolhead.Get_position()
}

func (self *homingDripToolheadAdapter) GetKinematics() homingpkg.Kinematics {
	kinematics, ok := self.toolhead.Get_kinematics().(IKinematics)
	if !ok {
		panic(fmt.Errorf("homing toolhead kinematics has unexpected type %T", self.toolhead.Get_kinematics()))
	}
	return &homingKinematicsAdapter{kinematics: kinematics}
}

func (self *homingDripToolheadAdapter) FlushStepGeneration() {
	self.toolhead.Flush_step_generation()
}

func (self *homingDripToolheadAdapter) GetLastMoveTime() float64 {
	return self.toolhead.Get_last_move_time()
}

func (self *homingDripToolheadAdapter) Dwell(delay float64) {
	self.toolhead.Dwell(delay)
}

func (self *homingDripToolheadAdapter) DripMove(newpos []float64, speed float64, dripCompletion homingpkg.Completion) error {
	runtimeCompletion, err := unwrapHomingCompletion(dripCompletion)
	if err != nil {
		return err
	}
	return self.toolhead.Drip_move(newpos, speed, runtimeCompletion)
}

func (self *homingDripToolheadAdapter) SetPosition(newpos []float64, homingAxes []int) {
	self.toolhead.Set_position(newpos, homingAxes)
}

type homingToolheadAdapter struct {
	toolhead *Toolhead
}

func (self *homingToolheadAdapter) GetPosition() []float64 {
	return self.toolhead.Get_position()
}

func (self *homingToolheadAdapter) GetKinematics() homingpkg.Kinematics {
	return &homingKinematicsAdapter{kinematics: self.toolhead.Get_kinematics().(IKinematics)}
}

func (self *homingToolheadAdapter) FlushStepGeneration() {
	self.toolhead.Flush_step_generation()
}

func (self *homingToolheadAdapter) GetLastMoveTime() float64 {
	return self.toolhead.Get_last_move_time()
}

func (self *homingToolheadAdapter) Dwell(delay float64) {
	self.toolhead.Dwell(delay)
}

func (self *homingToolheadAdapter) DripMove(newpos []float64, speed float64, dripCompletion homingpkg.Completion) error {
	runtimeCompletion, err := unwrapHomingCompletion(dripCompletion)
	if err != nil {
		return err
	}
	return self.toolhead.Drip_move(newpos, speed, runtimeCompletion)
}

func (self *homingToolheadAdapter) Move(newpos []float64, speed float64) {
	self.toolhead.Move(newpos, speed)
}

func (self *homingToolheadAdapter) SetPosition(newpos []float64, homingAxes []int) {
	self.toolhead.Set_position(newpos, homingAxes)
}

type homingEndstopAdapter struct {
	endstop interface{}
}

func (self *homingEndstopAdapter) GetSteppers() []homingpkg.Stepper {
	var steppers []interface{}
	switch typed := self.endstop.(type) {
	case *MCU_endstop:
		steppers = typed.Get_steppers()
	case *ProbeEndstopWrapper:
		steppers = typed.Get_steppers()
	default:
		panic(fmt.Errorf("homing endstop has unexpected type %T", self.endstop))
	}
	adapted := make([]homingpkg.Stepper, len(steppers))
	for i, stepper := range steppers {
		mcuStepper, ok := stepper.(*MCU_stepper)
		if !ok {
			panic(fmt.Errorf("homing endstop stepper has unexpected type %T", stepper))
		}
		adapted[i] = &homingStepperAdapter{stepper: mcuStepper}
	}
	return adapted
}

func (self *homingEndstopAdapter) HomeStart(printTime float64, sampleTime float64, sampleCount int64, restTime float64, triggered int64) homingpkg.Completion {
	var runtimeCompletion interface{}
	switch typed := self.endstop.(type) {
	case *MCU_endstop:
		runtimeCompletion = typed.Home_start(printTime, sampleTime, sampleCount, restTime, triggered)
	case *ProbeEndstopWrapper:
		runtimeCompletion = typed.Home_start.(func(float64, float64, int64, float64, int64) interface{})(printTime, sampleTime, sampleCount, restTime, triggered)
	default:
		panic(fmt.Errorf("homing endstop has unexpected type %T", self.endstop))
	}
	completion, ok := runtimeCompletion.(*ReactorCompletion)
	if !ok {
		panic(fmt.Errorf("homing endstop completion has unexpected type %T", runtimeCompletion))
	}
	return &homingCompletionAdapter{runtime: completion}
}

func (self *homingEndstopAdapter) HomeWait(moveEndPrintTime float64) float64 {
	switch typed := self.endstop.(type) {
	case *MCU_endstop:
		return typed.Home_wait(moveEndPrintTime)
	case *ProbeEndstopWrapper:
		return typed.Home_wait.(func(float64) float64)(moveEndPrintTime)
	default:
		panic(fmt.Errorf("homing endstop has unexpected type %T", self.endstop))
	}
}

type homingRailAdapter struct {
	rail *PrinterRail
}

func (self *homingRailAdapter) GetEndstops() []homingpkg.NamedEndstop {
	return adaptHomingEndstops(self.rail.Get_endstops())
}

func (self *homingRailAdapter) GetHomingInfo() *homingpkg.RailHomingInfo {
	info := self.rail.Get_homing_info()
	return &homingpkg.RailHomingInfo{
		Speed:             info.Speed,
		PositionEndstop:   info.Position_endstop,
		RetractSpeed:      info.Retract_speed,
		RetractDist:       info.Retract_dist,
		PositiveDir:       info.Positive_dir,
		SecondHomingSpeed: info.Second_homing_speed,
	}
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
			Endstop: &homingEndstopAdapter{endstop: endstop.Front().Value},
			Name:    name,
		})
	}
	return adapted
}

func adaptHomingRails(rails []*PrinterRail) []homingpkg.Rail {
	adapted := make([]homingpkg.Rail, len(rails))
	for i, rail := range rails {
		adapted[i] = &homingRailAdapter{rail: rail}
	}
	return adapted
}

func collectRailEndstops(rails []*PrinterRail) []list.List {
	endstops := []list.List{}
	for _, rail := range rails {
		endstops = append(endstops, rail.Get_endstops()...)
	}
	return endstops
}

// Return a completion that completes when all completions in a list complete
func Multi_complete(Printer *Printer, completions []*ReactorCompletion) *ReactorCompletion {
	adapted := make([]homingpkg.Completion, len(completions))
	for i, completion := range completions {
		adapted[i] = &homingCompletionAdapter{runtime: completion}
	}
	combined, err := unwrapHomingCompletion(homingpkg.MultiComplete(&homingReactorAdapter{reactor: Printer.Get_reactor()}, adapted))
	if err != nil {
		panic(err)
	}
	return combined
}

// Tracking of stepper positions during a homing/probing move
type StepperPosition struct {
	Stepper      *MCU_stepper
	Endstop_name string
	Stepper_name string
	Start_pos    int
	Halt_pos     int
	Trig_pos     int
	core         *homingpkg.StepperPosition
}

func NewStepperPosition(stepper *MCU_stepper, endstop_name string) *StepperPosition {
	return newProjectStepperPosition(homingpkg.NewStepperPosition(&homingStepperAdapter{stepper: stepper}, endstop_name))
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

// Implementation of homing/probing moves
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
		&homingReactorAdapter{reactor: printer.Get_reactor()},
		&homingDripToolheadAdapter{toolhead: runtimeToolhead},
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
	return self.core.CalcEndstopRate(&homingEndstopAdapter{endstop: mcu_endstop}, movepos, speed)
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
	_, _ = self.Printer.Send_event("homing:homing_move_begin", []interface{}{self})
	triggerPos, triggerTime, err := self.core.Execute(movepos, speed, probe_pos, triggered, check_triggered)
	self.syncStepperPositions()
	_, eventErr := self.Printer.Send_event("homing:homing_move_end", []interface{}{self})
	if eventErr != nil {
		err = eventErr
	}
	return triggerPos, triggerTime, err
}

func (self *HomingMove) Homing_move(movepos []float64, speed float64, probe_pos bool,
	triggered bool, check_triggered bool) (interface{}, float64) {
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

type homingMoveExecutorAdapter struct {
	move *HomingMove
}

func (self *homingMoveExecutorAdapter) Execute(movepos []float64, speed float64, probePos bool, triggered bool, checkTriggered bool) ([]float64, float64, error) {
	return self.move.executeRuntime(movepos, speed, probePos, triggered, checkTriggered)
}

func (self *homingMoveExecutorAdapter) CheckNoMovement() string {
	return self.move.Check_no_movement()
}

func (self *homingMoveExecutorAdapter) StepperPositions() []*homingpkg.StepperPosition {
	return self.move.core.StepperPositions()
}

// State tracking of homing requests
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
	self.core = homingpkg.NewState(&homingToolheadAdapter{toolhead: self.Toolhead})
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
	_, _ = self.Printer.Send_event("homing:home_rails_begin", []interface{}{self, rails})
	endstops := collectRailEndstops(rails)
	err := self.core.HomeRailsWithPositions(
		adaptHomingRails(rails),
		forcepos,
		movepos,
		func([]homingpkg.NamedEndstop) homingpkg.MoveExecutor {
			return &homingMoveExecutorAdapter{move: NewHomingMove(self.Printer, endstops, self.Toolhead)}
		},
		func() {
			self.Adjust_pos = map[string]float64{}
			_, _ = self.Printer.Send_event("homing:home_rails_end", []interface{}{self, rails})
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
	// Register g-code commands
	gcode := self.Printer.Lookup_object("gcode", object.Sentinel{})
	gcode.(*GCodeDispatch).Register_command("G28", self.Cmd_G28, false, "")
	return &self
}

func (self *PrinterHoming) Manual_home(toolhead interface{}, endstops []list.List, pos []float64, speed float64,
	triggered bool, check_triggered bool) {
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
