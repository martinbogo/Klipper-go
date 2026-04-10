package project

import "C"
import (
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	"goklipper/internal/pkg/chelper"
	mcupkg "goklipper/internal/pkg/mcu"
	motionpkg "goklipper/internal/pkg/motion"
	"strings"
)

type MCU_stepper struct {
	_name                     string
	_rotation_dist            float64
	_steps_per_rotation       int
	_step_pulse_duration      interface{}
	_units_in_radians         bool
	_step_dist                float64
	_mcu                      *MCU
	_oid                      int
	_step_pin                 interface{}
	_invert_step              int
	_dir_pin                  interface{}
	_invert_dir               uint32
	_orig_invert_dir          uint32
	_step_both_edge           bool
	_req_step_both_edge       bool
	_mcu_position_offset      float64
	_reset_cmd_tag            interface{}
	_get_position_cmd         interface{}
	_active_callbacks         []func(float64)
	_stepqueue                interface{}
	_stepper_kinematics       interface{}
	_itersolve_generate_steps func(interface{}, float64) int32
	_itersolve_check_active   func(interface{}, float64) float64
	_trapq                    interface{}
}

func NewMCU_stepper(name string, step_pin_params map[string]interface{}, dir_pin_params map[string]interface{}, rotation_dist float64, steps_per_rotation int,
	step_pulse_duration interface{}, _units_in_radians bool) *MCU_stepper {
	self := MCU_stepper{}
	self._name = name
	self._rotation_dist = rotation_dist
	self._steps_per_rotation = steps_per_rotation
	self._step_pulse_duration = step_pulse_duration
	self._units_in_radians = _units_in_radians
	self._step_dist = rotation_dist / float64(steps_per_rotation)
	self._mcu, _ = step_pin_params["chip"].(*MCU)
	self._oid = self._mcu.Create_oid()
	self._mcu.Register_config_callback(self._build_config)
	self._step_pin, _ = step_pin_params["pin"]
	invert_step, ok := step_pin_params["invert"]
	if ok {
		self._invert_step, _ = invert_step.(int)
	}
	if dir_pin_params["chip"].(*MCU) != self._mcu {
		panic("Stepper dir pin must be on same mcu as step pin")
	}
	self._dir_pin, _ = dir_pin_params["pin"]
	invert_dir, ok1 := dir_pin_params["invert"]
	if ok1 {
		self._invert_dir = uint32(invert_dir.(int))
		self._orig_invert_dir = self._invert_dir
	}
	self._step_both_edge = false
	self._req_step_both_edge = false
	self._mcu_position_offset = 0.
	self._reset_cmd_tag = nil
	self._get_position_cmd = nil
	self._active_callbacks = []func(float64){}
	self._stepqueue = chelper.Stepcompress_alloc(uint32(self._oid))
	//runtime.SetFinalizer(self,self._MCU_stepper)
	chelper.Stepcompress_set_invert_sdir(self._stepqueue, self._invert_dir)
	self._mcu.Register_stepqueue(self._stepqueue)
	self._stepper_kinematics = nil
	self._itersolve_generate_steps = chelper.Itersolve_generate_steps
	self._itersolve_check_active = chelper.Itersolve_check_active
	self._trapq = nil
	self._mcu.Get_printer().Register_event_handler("project:connect", self._query_mcu_position)
	return &self
}
func (self *MCU_stepper) _MCU_stepper() {
	chelper.Stepcompress_free(self._stepqueue)
	chelper.Free(self._stepper_kinematics)
}
func (self *MCU_stepper) Get_mcu() *MCU {
	return self._mcu
}

func (self *MCU_stepper) Get_name(short bool) string {
	if short && strings.HasPrefix(self._name, "stepper_") {
		return self._name[8:]
	}
	return self._name
}

func (self *MCU_stepper) MCUKey() interface{} {
	return self.Get_mcu()
}

func (self *MCU_stepper) Name(short bool) string {
	return self.Get_name(short)
}

func (self *MCU_stepper) Raw() interface{} {
	return self
}

func (self *MCU_stepper) Units_in_radians() bool {
	// Returns true if distances are in radians instead of millimeters
	return self._units_in_radians
}

func (self *MCU_stepper) Get_pulse_duration() (interface{}, bool) {
	return self._step_pulse_duration, self._req_step_both_edge
}

func (self *MCU_stepper) Setup_default_pulse_duration(pulseduration interface{}, step_both_edge bool) {
	if self._step_pulse_duration == nil {
		self._step_pulse_duration = pulseduration
	}
	self._req_step_both_edge = step_both_edge
}

func (self *MCU_stepper) Setup_itersolve(alloc_func string, params interface{}) {
	if alloc_func == "cartesian_stepper_alloc" {
		axis := params.([]interface{})[0].(uint8)
		sk := chelper.Cartesian_stepper_alloc(int8(axis))
		self.Set_stepper_kinematics(sk)
	} else if alloc_func == "corexy_stepper_alloc" {
		axis := params.([]interface{})[0].(uint8)
		sk := chelper.Corexy_stepper_alloc(int8(axis))
		self.Set_stepper_kinematics(sk)
	} else {
		logger.Debug("ERROR: Setup_itersolve not implement:", alloc_func)
	}
}

func (self *MCU_stepper) _build_config() {
	stepperBothEdgeConstant := self._mcu.Get_constants()["STEPPER_BOTH_EDGE"]
	plan := mcupkg.BuildStepperPulseConfigPlan(self._step_pulse_duration, self._req_step_both_edge, self._invert_step, stepperBothEdgeConstant, self._mcu.Seconds_to_clock)
	setupPlan := mcupkg.BuildStepperConfigSetupPlan(self._oid, self._step_pin, self._dir_pin, plan.InvertStep, plan.StepPulseTicks, self._mcu.Get_max_stepper_error(), self._mcu.Seconds_to_clock)
	self._step_pulse_duration = plan.StepPulseDuration
	self._step_both_edge = plan.StepBothEdge
	self._mcu.Add_config_cmd(setupPlan.ConfigCmds[0], false, false)
	self._mcu.Add_config_cmd(setupPlan.ConfigCmds[1], false, true)
	stepCmdTag := self._mcu.Lookup_command_tag(setupPlan.StepQueueLookup)
	dirCmdTag := self._mcu.Lookup_command_tag(setupPlan.DirLookup)
	self._reset_cmd_tag = self._mcu.Lookup_command_tag(setupPlan.ResetLookup)
	self._get_position_cmd = self._mcu.Lookup_query_command(
		setupPlan.PositionQueryFormat,
		setupPlan.PositionReplyFormat, self._oid, nil, false)
	//ffiMain, ffiLib := chelper.get_ffi()
	chelper.Stepcompress_fill(self._stepqueue, setupPlan.MaxErrorTicks,
		int32(stepCmdTag.(int)), int32(dirCmdTag.(int)))
}

func (self *MCU_stepper) Get_oid() int {
	return self._oid
}
func (self *MCU_stepper) Get_step_dist() float64 {
	return self._step_dist
}

func (self *MCU_stepper) Get_rotation_distance() (float64, int) {
	return self._rotation_dist, self._steps_per_rotation
}

func (self *MCU_stepper) Set_rotation_distance(rotation_dist float64) {
	mcu_pos := self.Get_mcu_position()
	self._rotation_dist = rotation_dist
	self._step_dist = rotation_dist / float64(self._steps_per_rotation)
	self.Set_stepper_kinematics(self._stepper_kinematics)
	self._set_mcu_position(mcu_pos)
}

func (self *MCU_stepper) Get_dir_inverted() (uint32, uint32) {
	return self._invert_dir, self._orig_invert_dir
}

func (self *MCU_stepper) Set_dir_inverted(invert_dir uint32) {
	if invert_dir == self._invert_dir {
		return
	}
	self._invert_dir = invert_dir
	//ffi_main, ffi_lib := chelper.Get_ffi()

	chelper.Stepcompress_set_invert_sdir(self._stepqueue, invert_dir)
	self._mcu.Get_printer().Send_event("stepper:set_dir_inverted", []interface{}{self})
}

func (self *MCU_stepper) Calc_position_from_coord(coord []float64) float64 {
	//ffi_main, ffi_lib := chelper.Get_ffi()
	return chelper.Itersolve_calc_position_from_coord(
		self._stepper_kinematics, coord[0], coord[1], coord[2])
}

func (self *MCU_stepper) Set_position(coord []float64) {
	mcu_pos := self.Get_mcu_position()
	sk := self._stepper_kinematics
	//ffi_main, ffi_lib := chelper.Get_ffi()
	chelper.Itersolve_set_position(sk, coord[0], coord[1], coord[2])
	self._set_mcu_position(mcu_pos)
}

func (self *MCU_stepper) Get_commanded_position() float64 {
	//ffi_main, ffi_lib = chelper.get_ffi()
	return chelper.Itersolve_get_commanded_pos(self._stepper_kinematics)
}

func (self *MCU_stepper) positionState() mcupkg.StepperPositionState {
	return mcupkg.StepperPositionState{
		StepDist:          self._step_dist,
		MCUPositionOffset: self._mcu_position_offset,
		InvertDir:         self._invert_dir,
	}
}

func (self *MCU_stepper) applyPositionState(state mcupkg.StepperPositionState) {
	self._mcu_position_offset = state.MCUPositionOffset
}

func (self *MCU_stepper) FindPastPosition(clock uint64) int64 {
	return chelper.Stepcompress_find_past_position(self._stepqueue, clock)
}

func (self *MCU_stepper) Reset(lastStepClock uint64) int {
	return chelper.Stepcompress_reset(self._stepqueue, lastStepClock)
}

func (self *MCU_stepper) QueueMessage(data []uint32) int {
	return chelper.Stepcompress_queue_msg(self._stepqueue, data, len(data))
}

func (self *MCU_stepper) SetLastPosition(clock uint64, lastPosition int64) int {
	return chelper.Stepcompress_set_last_position(self._stepqueue, clock, lastPosition)
}

func (self *MCU_stepper) Get_mcu_position() int {
	state := self.positionState()
	return state.GetMCUPosition(self.Get_commanded_position())
}

func (self *MCU_stepper) _set_mcu_position(mcu_pos int) {
	state := self.positionState()
	state.SetMCUPosition(mcu_pos, self.Get_commanded_position())
	self.applyPositionState(state)
}

func (self *MCU_stepper) Get_past_mcu_position(print_time float64) int {
	state := mcupkg.StepperPositionState{}
	return state.PastMCUPosition(print_time, self._mcu.Print_time_to_clock, self)
}

func (self *MCU_stepper) Mcu_to_commanded_position(mcu_pos int) float64 {
	state := self.positionState()
	return state.MCUToCommandedPosition(mcu_pos)
}

func (self *MCU_stepper) Dump_steps(count int, start_clock uint64, end_clock uint64) ([]interface{}, int) {
	//ffi_main, ffi_lib = chelper.get_ffi()
	data := chelper.New_pull_history_steps()
	_data := []interface{}{}
	for _, d := range data {
		_data = append(_data, d)
	}
	count = chelper.Stepcompress_extract_old(self._stepqueue, data, count,
		start_clock, end_clock)
	//return data, count
	return _data, count
}

func (self *MCU_stepper) Set_stepper_kinematics(sk interface{}) interface{} {
	old_sk := self._stepper_kinematics
	mcu_pos := 0
	if old_sk != nil {
		mcu_pos = self.Get_mcu_position()
	}
	self._stepper_kinematics = sk
	chelper.Itersolve_set_stepcompress(sk, self._stepqueue, self._step_dist)
	self.Set_trapq(self._trapq)
	self._set_mcu_position(mcu_pos)
	return old_sk
}

func (self *MCU_stepper) Note_homing_end() {
	err := mcupkg.ApplyStepperHomingReset(self, self._reset_cmd_tag.(int), self._oid)
	if err != nil {
		panic(err.Error())
	}

	self._query_mcu_position(nil)
}

func (self *MCU_stepper) _query_mcu_position([]interface{}) error {
	if self._mcu.Is_fileoutput() {
		return nil
	}
	state := self.positionState()
	_, err := mcupkg.ExecuteStepperPositionQuery(self._get_position_cmd.(mcupkg.StepperPositionQuerySender), self._oid, self.Get_commanded_position(), &state, self, chelper.CdoubleTofloat64, self._mcu.Estimated_print_time, self._mcu.Print_time_to_clock)
	if err != nil {
		panic(err.Error())
	}
	self.applyPositionState(state)
	self._mcu.Get_printer().Send_event("stepper:sync_mcu_position", []interface{}{self})
	return nil
}

func (self *MCU_stepper) Get_trapq() interface{} {
	return self._trapq
}

func (self *MCU_stepper) Set_trapq(tq interface{}) interface{} {
	//ffi_main, ffi_lib := chelper.GetFFI()
	if tq == nil {
		tq = nil
	}
	chelper.Itersolve_set_trapq(self._stepper_kinematics, tq)
	old_tq := self._trapq
	self._trapq = tq
	return old_tq
}

func (self *MCU_stepper) Add_active_callback(cb func(float64)) {
	self._active_callbacks = append(self._active_callbacks, cb)
}

func (self *MCU_stepper) CheckActive(flushTime float64) float64 {
	return self._itersolve_check_active(self._stepper_kinematics, flushTime)
}

func (self *MCU_stepper) Generate(flushTime float64) int32 {
	return self._itersolve_generate_steps(self._stepper_kinematics, flushTime)
}

func (self *MCU_stepper) Generate_steps(flush_time float64) {
	state := mcupkg.StepGenerationState{ActiveCallbacks: self._active_callbacks}
	err := state.GenerateSteps(flush_time, self)
	self._active_callbacks = state.ActiveCallbacks
	if err != nil {
		panic(err.Error())
	}
}

func (self *MCU_stepper) Is_active_axis(axis int8) int32 {
	return chelper.Itersolve_is_active_axis(self._stepper_kinematics, uint8(axis))
}

type legacyProjectStepperModuleRegistrar struct {
	config  *ConfigWrapper
	stepper *MCU_stepper
}

func (self *legacyProjectStepperModuleRegistrar) RegisterStepperEnable(module interface{}) {
	module.(*mcupkg.PrinterStepperEnableModule).Register_stepper(self.config, self.stepper)
}

func (self *legacyProjectStepperModuleRegistrar) RegisterForceMove(module interface{}) {
	module.(*motionpkg.ForceMoveModule).RegisterStepper(self.stepper)
}

// PrinterStepper Helper code to build a stepper object from a config section
func PrinterStepper(config *ConfigWrapper, units_in_radians bool) *MCU_stepper {
	printer := config.Get_printer()
	plan := mcupkg.BuildLegacyStepperFactoryPlan(config, units_in_radians)
	mcu_stepper := NewMCU_stepper(plan.Name, plan.StepPinParams, plan.DirPinParams, plan.RotationDist, plan.StepsPerRotation, plan.StepPulseDuration, units_in_radians)
	mcupkg.RegisterLegacyStepperModules(func(moduleName string) interface{} {
		return printer.Load_object(config, moduleName, object.Sentinel{})
	}, &legacyProjectStepperModuleRegistrar{config: config, stepper: mcu_stepper})
	return mcu_stepper
}

var _ mcupkg.EndstopRegistryStepper = (*MCU_stepper)(nil)
var _ mcupkg.StepGenerationOps = (*MCU_stepper)(nil)
var _ mcupkg.StepcompressQueue = (*MCU_stepper)(nil)
