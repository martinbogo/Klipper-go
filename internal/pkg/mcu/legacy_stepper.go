package mcu

import (
	"goklipper/common/logger"
	"goklipper/internal/pkg/chelper"
)

type LegacyStepperController interface {
	CreateOID() int
	RegisterConfigCallback(func())
	Register_stepqueue(stepqueue interface{})
	Get_constants() map[string]interface{}
	Seconds_to_clock(time float64) int64
	Get_max_stepper_error() float64
	Add_config_cmd(cmd string, is_init bool, on_restart bool)
	Lookup_command_tag(msgformat string) interface{}
	LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{}
	Is_fileoutput() bool
	Estimated_print_time(eventtime float64) float64
	Print_time_to_clock(print_time float64) int64
}

type LegacyStepper struct {
	rotationDist           float64
	stepsPerRotation       int
	stepDist               float64
	controller             LegacyStepperController
	oid                    int
	stepPin                interface{}
	invertStep             int
	dirPin                 interface{}
	invertDir              uint32
	origInvertDir          uint32
	mcuPositionOffset      float64
	resetCmdTag            interface{}
	getPositionCmd         interface{}
	activeCallbacks        []func(float64)
	stepqueue              interface{}
	stepperKinematics      interface{}
	itersolveGenerateSteps func(interface{}, float64) int32
	itersolveCheckActive   func(interface{}, float64) float64
	trapq                  interface{}
	runtime                *LegacyStepperRuntimeState
	sendPrinterEvent       func(string, []interface{})
	eventSelf              interface{}
}

func NewLegacyStepper(name string, stepPinParams map[string]interface{}, dirPinParams map[string]interface{}, rotationDist float64, stepsPerRotation int,
	stepPulseDuration interface{}, unitsInRadians bool, registerConnectHandler func(func([]interface{}) error), sendPrinterEvent func(string, []interface{})) *LegacyStepper {
	controller, ok := stepPinParams["chip"].(LegacyStepperController)
	if !ok {
		panic("stepper step pin chip does not implement legacy stepper controller")
	}
	if dirPinParams["chip"] != stepPinParams["chip"] {
		panic("Stepper dir pin must be on same mcu as step pin")
	}
	self := &LegacyStepper{
		rotationDist:           rotationDist,
		stepsPerRotation:       stepsPerRotation,
		stepDist:               rotationDist / float64(stepsPerRotation),
		controller:             controller,
		oid:                    controller.CreateOID(),
		stepPin:                stepPinParams["pin"],
		mcuPositionOffset:      0.,
		resetCmdTag:            nil,
		getPositionCmd:         nil,
		activeCallbacks:        []func(float64){},
		stepqueue:              nil,
		stepperKinematics:      nil,
		itersolveGenerateSteps: chelper.Itersolve_generate_steps,
		itersolveCheckActive:   chelper.Itersolve_check_active,
		trapq:                  nil,
		runtime:                NewLegacyStepperRuntimeState(name, stepPulseDuration, unitsInRadians),
		sendPrinterEvent:       sendPrinterEvent,
	}
	if invertStep, ok := stepPinParams["invert"]; ok {
		self.invertStep = invertStep.(int)
	}
	self.dirPin = dirPinParams["pin"]
	if invertDir, ok := dirPinParams["invert"]; ok {
		self.invertDir = uint32(invertDir.(int))
		self.origInvertDir = self.invertDir
	}
	self.stepqueue = chelper.Stepcompress_alloc(uint32(self.oid))
	chelper.Stepcompress_set_invert_sdir(self.stepqueue, self.invertDir)
	controller.RegisterConfigCallback(self.Build_config)
	controller.Register_stepqueue(self.stepqueue)
	if registerConnectHandler != nil {
		registerConnectHandler(self.QueryMCUPosition)
	}
	return self
}

func (self *LegacyStepper) BindEventSelf(eventSelf interface{}) {
	self.eventSelf = eventSelf
}

func (self *LegacyStepper) eventValue() interface{} {
	if self.eventSelf != nil {
		return self.eventSelf
	}
	return self
}

func (self *LegacyStepper) Get_name(short bool) string {
	return self.runtime.Name(short)
}

func (self *LegacyStepper) MCUKey() interface{} {
	return self.controller
}

func (self *LegacyStepper) Name(short bool) string {
	return self.Get_name(short)
}

func (self *LegacyStepper) Raw() interface{} {
	return self.eventValue()
}

func (self *LegacyStepper) Units_in_radians() bool {
	return self.runtime.UnitsInRadians()
}

func (self *LegacyStepper) Get_pulse_duration() (interface{}, bool) {
	return self.runtime.PulseDuration()
}

func (self *LegacyStepper) Setup_default_pulse_duration(pulseduration interface{}, step_both_edge bool) {
	self.runtime.SetupDefaultPulseDuration(pulseduration, step_both_edge)
}

func (self *LegacyStepper) Setup_itersolve(alloc_func string, params interface{}) {
	sk, err := AllocateLegacyStepperKinematics(alloc_func, params)
	if err != nil {
		logger.Debug("ERROR:", err.Error())
		return
	}
	self.Set_stepper_kinematics(sk)
}

func (self *LegacyStepper) Build_config() {
	stepperBothEdgeConstant := self.controller.Get_constants()["STEPPER_BOTH_EDGE"]
	plan := self.runtime.BuildPulseConfigPlan(self.invertStep, stepperBothEdgeConstant, self.controller.Seconds_to_clock)
	setupPlan := BuildStepperConfigSetupPlan(self.oid, self.stepPin, self.dirPin, plan.InvertStep, plan.StepPulseTicks, self.controller.Get_max_stepper_error(), self.controller.Seconds_to_clock)
	self.controller.Add_config_cmd(setupPlan.ConfigCmds[0], false, false)
	self.controller.Add_config_cmd(setupPlan.ConfigCmds[1], false, true)
	stepCmdTag := self.controller.Lookup_command_tag(setupPlan.StepQueueLookup)
	dirCmdTag := self.controller.Lookup_command_tag(setupPlan.DirLookup)
	self.resetCmdTag = self.controller.Lookup_command_tag(setupPlan.ResetLookup)
	self.getPositionCmd = self.controller.LookupQueryCommand(setupPlan.PositionQueryFormat, setupPlan.PositionReplyFormat, self.oid, nil, false)
	chelper.Stepcompress_fill(self.stepqueue, uint32(self.oid), setupPlan.MaxErrorTicks, int32(stepCmdTag.(int)), int32(dirCmdTag.(int)))
}

func (self *LegacyStepper) Get_oid() int {
	return self.oid
}

func (self *LegacyStepper) Get_step_dist() float64 {
	return self.stepDist
}

func (self *LegacyStepper) Get_rotation_distance() (float64, int) {
	return self.rotationDist, self.stepsPerRotation
}

func (self *LegacyStepper) Set_rotation_distance(rotation_dist float64) {
	mcuPos := self.Get_mcu_position()
	self.rotationDist = rotation_dist
	self.stepDist = rotation_dist / float64(self.stepsPerRotation)
	self.Set_stepper_kinematics(self.stepperKinematics)
	self._set_mcu_position(mcuPos)
}

func (self *LegacyStepper) Get_dir_inverted() (uint32, uint32) {
	return self.invertDir, self.origInvertDir
}

func (self *LegacyStepper) Set_dir_inverted(invert_dir uint32) {
	if invert_dir == self.invertDir {
		return
	}
	self.invertDir = invert_dir
	chelper.Stepcompress_set_invert_sdir(self.stepqueue, invert_dir)
	if self.sendPrinterEvent != nil {
		self.sendPrinterEvent("stepper:set_dir_inverted", []interface{}{self.eventValue()})
	}
}

func (self *LegacyStepper) Calc_position_from_coord(coord []float64) float64 {
	return CalcLegacyStepperPositionFromCoord(self.stepperKinematics, coord)
}

func (self *LegacyStepper) Set_position(coord []float64) {
	mcuPos := self.Get_mcu_position()
	SetLegacyStepperPosition(self.stepperKinematics, coord)
	self._set_mcu_position(mcuPos)
}

func (self *LegacyStepper) Get_commanded_position() float64 {
	return GetLegacyStepperCommandedPosition(self.stepperKinematics)
}

func (self *LegacyStepper) positionState() StepperPositionState {
	return StepperPositionState{StepDist: self.stepDist, MCUPositionOffset: self.mcuPositionOffset, InvertDir: self.invertDir}
}

func (self *LegacyStepper) applyPositionState(state StepperPositionState) {
	self.mcuPositionOffset = state.MCUPositionOffset
}

func (self *LegacyStepper) FindPastPosition(clock uint64) int64 {
	return chelper.Stepcompress_find_past_position(self.stepqueue, clock)
}

func (self *LegacyStepper) Reset(lastStepClock uint64) int {
	return chelper.Stepcompress_reset(self.stepqueue, lastStepClock)
}

func (self *LegacyStepper) QueueMessage(data []uint32) int {
	return chelper.Stepcompress_queue_msg(self.stepqueue, data, len(data))
}

func (self *LegacyStepper) SetLastPosition(clock uint64, lastPosition int64) int {
	return chelper.Stepcompress_set_last_position(self.stepqueue, clock, lastPosition)
}

func (self *LegacyStepper) Get_mcu_position() int {
	state := self.positionState()
	return state.GetMCUPosition(self.Get_commanded_position())
}

func (self *LegacyStepper) _set_mcu_position(mcu_pos int) {
	state := self.positionState()
	state.SetMCUPosition(mcu_pos, self.Get_commanded_position())
	self.applyPositionState(state)
}

func (self *LegacyStepper) Get_past_mcu_position(print_time float64) int {
	state := StepperPositionState{}
	return state.PastMCUPosition(print_time, self.controller.Print_time_to_clock, self)
}

func (self *LegacyStepper) Mcu_to_commanded_position(mcu_pos int) float64 {
	state := self.positionState()
	return state.MCUToCommandedPosition(mcu_pos)
}

func (self *LegacyStepper) Dump_steps(count int, start_clock uint64, end_clock uint64) ([]interface{}, int) {
	data := chelper.New_pull_history_steps()
	resolved := make([]interface{}, 0, len(data))
	for _, entry := range data {
		resolved = append(resolved, entry)
	}
	count = chelper.Stepcompress_extract_old(self.stepqueue, data, count, start_clock, end_clock)
	return resolved, count
}

func (self *LegacyStepper) Set_stepper_kinematics(sk interface{}) interface{} {
	oldSK := self.stepperKinematics
	mcuPos := 0
	if oldSK != nil {
		mcuPos = self.Get_mcu_position()
	}
	self.stepperKinematics = sk
	ConfigureLegacyStepperKinematics(sk, self.stepqueue, self.stepDist, self.trapq)
	self._set_mcu_position(mcuPos)
	return oldSK
}

func (self *LegacyStepper) Note_homing_end() {
	err := ApplyStepperHomingReset(self, self.resetCmdTag.(int), self.oid)
	if err != nil {
		panic(err.Error())
	}
	if err := self.QueryMCUPosition(nil); err != nil {
		panic(err.Error())
	}
}

func (self *LegacyStepper) QueryMCUPosition([]interface{}) error {
	if self.controller.Is_fileoutput() {
		return nil
	}
	state := self.positionState()
	query, ok := self.getPositionCmd.(StepperPositionQuerySender)
	if !ok {
		panic("stepper position query sender has unexpected type")
	}
	_, err := ExecuteStepperPositionQuery(query, self.oid, self.Get_commanded_position(), &state, self, chelper.CdoubleTofloat64, self.controller.Estimated_print_time, self.controller.Print_time_to_clock)
	if err != nil {
		return err
	}
	self.applyPositionState(state)
	if self.sendPrinterEvent != nil {
		self.sendPrinterEvent("stepper:sync_mcu_position", []interface{}{self.eventValue()})
	}
	return nil
}

func (self *LegacyStepper) Get_trapq() interface{} {
	return self.trapq
}

func (self *LegacyStepper) Set_trapq(tq interface{}) interface{} {
	if tq == nil {
		tq = nil
	}
	SetLegacyStepperTrapq(self.stepperKinematics, tq)
	oldTQ := self.trapq
	self.trapq = tq
	return oldTQ
}

func (self *LegacyStepper) Add_active_callback(cb func(float64)) {
	self.activeCallbacks = append(self.activeCallbacks, cb)
}

func (self *LegacyStepper) CheckActive(flushTime float64) float64 {
	return self.itersolveCheckActive(self.stepperKinematics, flushTime)
}

func (self *LegacyStepper) Generate(flushTime float64) int32 {
	return self.itersolveGenerateSteps(self.stepperKinematics, flushTime)
}

func (self *LegacyStepper) Generate_steps(flush_time float64) {
	state := StepGenerationState{ActiveCallbacks: self.activeCallbacks}
	err := state.GenerateSteps(flush_time, self)
	self.activeCallbacks = state.ActiveCallbacks
	if err != nil {
		panic(err.Error())
	}
}

func (self *LegacyStepper) Is_active_axis(axis int8) int32 {
	return LegacyStepperActiveAxis(self.stepperKinematics, axis)
}
