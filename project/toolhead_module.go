package project

import (
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	"goklipper/internal/pkg/chelper"
	gcodepkg "goklipper/internal/pkg/gcode"
	motionpkg "goklipper/internal/pkg/motion"
	"math"
	"strings"
)

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

type toolheadMoveQueueFlusher struct {
	mcu *MCU
}

func (self *toolheadMoveQueueFlusher) FlushMoves(flushTime float64, clearHistoryTime float64) {
	self.mcu.Flush_moves(flushTime, clearHistoryTime)
}

type toolheadMoveBatchSink struct {
	toolhead *Toolhead
}

type toolheadPrintTimeSource struct {
	reactor IReactor
	mcu     *MCU
}

type toolheadPrintTimeNotifier struct {
	printer *Printer
}

type toolheadPauseSource struct {
	reactor IReactor
	mcu     *MCU
}

type toolheadMoveControlAdapter struct {
	toolhead *Toolhead
}

type toolheadDwellRuntimeAdapter struct {
	toolhead *Toolhead
}

type toolheadWaitRuntimeAdapter struct {
	toolhead *Toolhead
}

type toolheadPauseRuntimeAdapter struct {
	toolhead *Toolhead
}

type toolheadPrimingRuntimeAdapter struct {
	toolhead *Toolhead
}

type toolheadFlushRuntimeAdapter struct {
	toolhead *Toolhead
}

type toolheadDripMoveRuntimeAdapter struct {
	toolhead *Toolhead
}

type toolheadDripRuntimeAdapter struct {
	toolhead *Toolhead
}

func (self *toolheadPrintTimeSource) Monotonic() float64 {
	return self.reactor.Monotonic()
}

func (self *toolheadPrintTimeSource) EstimatedPrintTime(eventtime float64) float64 {
	return self.mcu.Estimated_print_time(eventtime)
}

func (self *toolheadPrintTimeNotifier) SyncPrintTime(curTime float64, estPrintTime float64, printTime float64) {
	_, _ = self.printer.Send_event("toolhead:sync_print_time", []interface{}{curTime, estPrintTime, printTime})
}

func (self *toolheadPauseSource) Monotonic() float64 {
	return self.reactor.Monotonic()
}

func (self *toolheadPauseSource) EstimatedPrintTime(eventtime float64) float64 {
	return self.mcu.Estimated_print_time(eventtime)
}

func (self *toolheadPauseSource) Pause(waketime float64) float64 {
	return self.reactor.Pause(waketime)
}

func (self *toolheadMoveControlAdapter) CommandedPosition() []float64 {
	return append([]float64{}, self.toolhead.Commanded_pos...)
}

func (self *toolheadMoveControlAdapter) MoveConfig() motionpkg.MoveConfig {
	return motionpkg.MoveConfig{
		Max_accel:          self.toolhead.Max_accel,
		Junction_deviation: self.toolhead.Junction_deviation,
		Max_velocity:       self.toolhead.Max_velocity,
		Max_accel_to_decel: self.toolhead.Max_accel_to_decel,
	}
}

func (self *toolheadMoveControlAdapter) SetCommandedPosition(position []float64) {
	self.toolhead.Commanded_pos = append([]float64{}, position...)
}

func (self *toolheadMoveControlAdapter) CheckKinematicMove(move *Move) {
	self.toolhead.Kin.Check_move(move)
}

func (self *toolheadMoveControlAdapter) CheckExtruderMove(move *Move) {
	self.toolhead.Extruder.Check_move(move)
}

func (self *toolheadMoveControlAdapter) QueueMove(move *Move) {
	self.toolhead.lookahead.Add_move(move, self.toolhead.Extruder)
}

func (self *toolheadMoveControlAdapter) PrintTime() float64 {
	return self.toolhead.Print_time
}

func (self *toolheadMoveControlAdapter) NeedCheckPause() float64 {
	return self.toolhead.Need_check_pause
}

func (self *toolheadMoveControlAdapter) CheckPause() {
	self.toolhead._Check_pause()
}

func (self *toolheadDwellRuntimeAdapter) GetLastMoveTime() float64 {
	return self.toolhead.Get_last_move_time()
}

func (self *toolheadDwellRuntimeAdapter) AdvanceMoveTime(nextPrintTime float64) {
	self.toolhead._advance_move_time(nextPrintTime)
}

func (self *toolheadDwellRuntimeAdapter) CheckPause() {
	self.toolhead._Check_pause()
}

func (self *toolheadWaitRuntimeAdapter) FlushLookahead() {
	self.toolhead._flush_lookahead()
}

func (self *toolheadWaitRuntimeAdapter) WaitMovesState() motionpkg.ToolheadWaitMovesState {
	return motionpkg.ToolheadWaitMovesState{
		SpecialQueuingState: self.toolhead.Special_queuing_state,
		PrintTime:           self.toolhead.Print_time,
		CanPause:            self.toolhead.Can_pause,
	}
}

func (self *toolheadPauseRuntimeAdapter) PauseState() motionpkg.ToolheadPauseState {
	return self.toolhead.pauseState()
}

func (self *toolheadPauseRuntimeAdapter) ApplyPauseState(state motionpkg.ToolheadPauseState) {
	self.toolhead.applyPauseState(state)
}

func (self *toolheadPauseRuntimeAdapter) EnsurePrimingTimer(waketime float64) {
	if self.toolhead.Priming_timer == nil {
		self.toolhead.Priming_timer = self.toolhead.Reactor.Register_timer(self.toolhead.Priming_handler, constants.NEVER)
	}
	self.toolhead.Reactor.Update_timer(self.toolhead.Priming_timer, waketime)
}

func (self *toolheadPrimingRuntimeAdapter) SpecialQueuingState() string {
	return self.toolhead.Special_queuing_state
}

func (self *toolheadPrimingRuntimeAdapter) PrintTime() float64 {
	return self.toolhead.Print_time
}

func (self *toolheadPrimingRuntimeAdapter) ClearPrimingTimer() {
	self.toolhead.Reactor.Unregister_timer(self.toolhead.Priming_timer)
	self.toolhead.Priming_timer = nil
}

func (self *toolheadPrimingRuntimeAdapter) FlushLookahead() {
	self.toolhead._flush_lookahead()
}

func (self *toolheadPrimingRuntimeAdapter) SetCheckStallTime(value float64) {
	self.toolhead.Check_stall_time = value
}

func (self *toolheadFlushRuntimeAdapter) FlushHandlerState() motionpkg.ToolheadFlushHandlerState {
	return motionpkg.ToolheadFlushHandlerState{
		PrintTime:           self.toolhead.Print_time,
		LastFlushTime:       self.toolhead.last_flush_time,
		NeedFlushTime:       self.toolhead.need_flush_time,
		SpecialQueuingState: self.toolhead.Special_queuing_state,
	}
}

func (self *toolheadFlushRuntimeAdapter) PrintTime() float64 {
	return self.toolhead.Print_time
}

func (self *toolheadFlushRuntimeAdapter) FlushLookahead() {
	self.toolhead._flush_lookahead()
}

func (self *toolheadFlushRuntimeAdapter) SetCheckStallTime(value float64) {
	self.toolhead.Check_stall_time = value
}

func (self *toolheadFlushRuntimeAdapter) AdvanceFlushTime(flushTime float64) {
	self.toolhead._advance_flush_time(flushTime)
}

func (self *toolheadFlushRuntimeAdapter) SetDoKickFlushTimer(value bool) {
	self.toolhead.do_kick_flush_timer = value
}

func (self *toolheadDripMoveRuntimeAdapter) KinFlushDelay() float64 {
	return self.toolhead.Kin_flush_delay
}

func (self *toolheadDripMoveRuntimeAdapter) Dwell(delay float64) {
	self.toolhead.Dwell(delay)
}

func (self *toolheadDripMoveRuntimeAdapter) FlushLookaheadQueue(lazy bool) {
	self.toolhead.lookahead.Flush(lazy)
}

func (self *toolheadDripMoveRuntimeAdapter) SetSpecialQueuingState(state string) {
	self.toolhead.Special_queuing_state = state
}

func (self *toolheadDripMoveRuntimeAdapter) SetNeedCheckPause(value float64) {
	self.toolhead.Need_check_pause = value
}

func (self *toolheadDripMoveRuntimeAdapter) UpdateFlushTimer(waketime float64) {
	self.toolhead.Reactor.Update_timer(self.toolhead.Flush_timer, waketime)
}

func (self *toolheadDripMoveRuntimeAdapter) SetDoKickFlushTimer(value bool) {
	self.toolhead.do_kick_flush_timer = value
}

func (self *toolheadDripMoveRuntimeAdapter) SetLookaheadFlushTime(value float64) {
	self.toolhead.lookahead.Set_flush_time(value)
}

func (self *toolheadDripMoveRuntimeAdapter) SetCheckStallTime(value float64) {
	self.toolhead.Check_stall_time = value
}

func (self *toolheadDripMoveRuntimeAdapter) SetDripCompletion(completion motionpkg.DripCompletion) {
	runtimeCompletion, _ := completion.(*ReactorCompletion)
	self.toolhead.Drip_completion = runtimeCompletion
}

func (self *toolheadDripMoveRuntimeAdapter) SubmitMove(newpos []float64, speed float64) {
	self.toolhead.Move(newpos, speed)
}

func (self *toolheadDripMoveRuntimeAdapter) FlushStepGeneration() {
	self.toolhead.Flush_step_generation()
}

func (self *toolheadDripMoveRuntimeAdapter) ResetLookaheadQueue() {
	self.toolhead.lookahead.Reset()
}

func (self *toolheadDripMoveRuntimeAdapter) FinalizeDripMoves() {
	self.toolhead.Trapq_finalize_moves(self.toolhead.Trapq, constants.NEVER, 0)
}

func (self *toolheadDripMoveRuntimeAdapter) IsCommandError(recovered interface{}) bool {
	_, ok := recovered.(*CommandError)
	return ok
}

func (self *toolheadDripRuntimeAdapter) PrintTime() float64 {
	return self.toolhead.Print_time
}

func (self *toolheadDripRuntimeAdapter) KinFlushDelay() float64 {
	return self.toolhead.Kin_flush_delay
}

func (self *toolheadDripRuntimeAdapter) CanPause() bool {
	return self.toolhead.Can_pause
}

func (self *toolheadDripRuntimeAdapter) NoteMovequeueActivity(mqTime float64, setStepGenTime bool) {
	self.toolhead.Note_mcu_movequeue_activity(mqTime, setStepGenTime)
}

func (self *toolheadDripRuntimeAdapter) AdvanceMoveTime(nextPrintTime float64) {
	self.toolhead._advance_move_time(nextPrintTime)
}

func (self *toolheadMoveBatchSink) QueueKinematicMove(printTime float64, move *Move) {
	self.toolhead.Trapq_append(self.toolhead.Trapq, printTime, move.Accel_t, move.Cruise_t, move.Decel_t,
		move.Start_pos[0], move.Start_pos[1], move.Start_pos[2],
		move.Axes_r[0], move.Axes_r[1], move.Axes_r[2],
		move.Start_v, move.Cruise_v, move.Accel)
}

func (self *toolheadMoveBatchSink) QueueExtruderMove(printTime float64, move *Move) {
	self.toolhead.Extruder.Move(printTime, move)
}

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

type toolheadCommandAdapter struct {
	toolhead *Toolhead
}

func (self *toolheadCommandAdapter) Dwell(delay float64) {
	self.toolhead.Dwell(delay)
}

func (self *toolheadCommandAdapter) WaitMoves() {
	self.toolhead.Wait_moves()
}

func (self *toolheadCommandAdapter) VelocitySettings() motionpkg.ToolheadVelocitySettings {
	return motionpkg.ToolheadVelocitySettings{
		MaxVelocity:           self.toolhead.Max_velocity,
		MaxAccel:              self.toolhead.Max_accel,
		RequestedAccelToDecel: self.toolhead.Requested_accel_to_decel,
		SquareCornerVelocity:  self.toolhead.Square_corner_velocity,
	}
}

func (self *toolheadCommandAdapter) ApplyVelocityLimitResult(result motionpkg.ToolheadVelocityLimitResult) {
	self.toolhead.Max_velocity = result.Settings.MaxVelocity
	self.toolhead.Max_accel = result.Settings.MaxAccel
	self.toolhead.Requested_accel_to_decel = result.Settings.RequestedAccelToDecel
	self.toolhead.Square_corner_velocity = result.Settings.SquareCornerVelocity
	self.toolhead.Junction_deviation = result.JunctionDeviation
	self.toolhead.Max_accel_to_decel = result.MaxAccelToDecel
}

func (self *toolheadCommandAdapter) SetRolloverInfo(msg string) {
	self.toolhead.Printer.Set_rollover_info("toolhead", msg, true)
}

type IExtruder = motionpkg.Extruder

const cmd_SET_VELOCITY_LIMIT_help = "Set printer velocity limits"

// Main code to track events (and their timing) on the printer Toolhead
type Toolhead struct {
	Printer                  *Printer
	Reactor                  IReactor
	All_mcus                 []*MCU
	Mcu                      *MCU
	Can_pause                bool
	do_kick_flush_timer      bool
	lookahead                *LookAheadQueue
	Commanded_pos            []float64
	Max_velocity             float64
	Max_accel                float64
	Requested_accel_to_decel float64
	Max_accel_to_decel       float64
	Square_corner_velocity   float64
	Junction_deviation       float64
	Print_time               float64
	Check_stall_time         float64
	Special_queuing_state    string
	Need_check_pause         float64
	Flush_timer              *ReactorTimer
	Priming_timer            *ReactorTimer
	Print_stall              float64
	Drip_completion          *ReactorCompletion
	Kin_flush_delay          float64
	Kin_flush_times          []float64
	last_flush_time          float64
	min_restart_time         float64
	need_flush_time          float64
	step_gen_time            float64
	clear_history_time       float64
	Trapq                    interface{}
	Trapq_append             func(tq interface{}, print_time,
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
	self.lookahead = motionpkg.NewMoveQueue(self)
	self.lookahead.Set_flush_time(BUFFER_TIME_HIGH)
	self.Commanded_pos = []float64{0., 0., 0., 0.}
	maxVelocity := config.Getfloat("max_velocity", object.Sentinel{}, 0.0, 0.0, 0.0, 0.0, true)
	maxAccel := config.Getfloat("max_accel", object.Sentinel{}, 0.0, 0.0, 0.0, 0.0, true)
	requestedAccelToDecel := config.Getfloat("max_accel_to_decel", maxAccel*0.5, 0.0, 0.0, 0., 0.0, true)
	squareCornerVelocity := config.Getfloat("square_corner_velocity", 5., 0.0, 0.0, 0., 0.0, true)
	(&toolheadCommandAdapter{toolhead: self}).ApplyVelocityLimitResult(motionpkg.BuildToolheadInitialVelocityResult(motionpkg.ToolheadVelocitySettings{
		MaxVelocity:           maxVelocity,
		MaxAccel:              maxAccel,
		RequestedAccelToDecel: requestedAccelToDecel,
		SquareCornerVelocity:  squareCornerVelocity,
	}))
	self.Check_stall_time = 0.
	self.Print_stall = 0
	self.Can_pause = true
	if self.Mcu.Is_fileoutput() {
		self.Can_pause = false
	}
	self.Need_check_pause = -1.
	self.Print_time = 0.
	self.Special_queuing_state = "NeedPrime"
	self.Drip_completion = nil
	self.Kin_flush_delay = SDS_CHECK_TIME
	self.Kin_flush_times = []float64{}
	self.Flush_timer = self.Reactor.Register_timer(self._flush_handler, constants.NEVER)
	self.do_kick_flush_timer = true
	self.last_flush_time, self.min_restart_time = 0., 0.
	self.need_flush_time, self.step_gen_time, self.clear_history_time = 0., 0., 0.
	self.Trapq = chelper.Trapq_alloc()
	self.Trapq_append = chelper.Trapq_append
	self.Trapq_finalize_moves = chelper.Trapq_finalize_moves
	self.Step_generators = []func(float64 float64){}
	gcode_obj := self.Printer.Lookup_object("gcode", object.Sentinel{})
	gcode := gcode_obj.(*GCodeDispatch)
	self.Coord = append([]string{}, gcode.Coord...)
	self.Extruder = NewDummyExtruder(self.Printer)
	kin_name := config.Get("kinematics", object.Sentinel{}, true)
	kinematics := Load_kinematics(kin_name.(string))
	self.Kin = kinematics.(func(*Toolhead, *ConfigWrapper) interface{})(self, config).(IKinematics)
	gcode.Register_command("G4", self.Cmd_G4, false, "")
	gcode.Register_command("M400", self.Cmd_M400, false, "")
	gcode.Register_command("SET_VELOCITY_LIMIT",
		self.Cmd_SET_VELOCITY_LIMIT, false,
		cmd_SET_VELOCITY_LIMIT_help)
	gcode.Register_command("M204", self.Cmd_M204, false, "")
	self.Printer.Register_event_handler("project:shutdown",
		self.Handle_shutdown)
	modules := []string{"gcode_move", "homing", "statistics", "idle_timeout",
		"manual_probe", "tuning_tower"}
	for _, module_name := range modules {
		self.Printer.Load_object(config, module_name, object.Sentinel{})
	}
	return self
}

func Add_printer_objects_toolhead(config *ConfigWrapper) {
	config.Get_printer().Add_object("toolhead", NewToolhead(config))
	Add_printer_objects_extruder(config)
}

func (self *Toolhead) _Toolhead() {
	chelper.Trapq_free(self.Trapq)
}

func (self *Toolhead) Get_transform() gcodepkg.LegacyMoveTransform {
	return self.move_transform
}

func (self *Toolhead) timingConfig() motionpkg.ToolheadTimingConfig {
	return motionpkg.DefaultTimingConfig()
}

func (self *Toolhead) timingState() motionpkg.ToolheadTimingState {
	kinFlushTimes := append([]float64{}, self.Kin_flush_times...)
	return motionpkg.ToolheadTimingState{
		PrintTime:        self.Print_time,
		LastFlushTime:    self.last_flush_time,
		MinRestartTime:   self.min_restart_time,
		NeedFlushTime:    self.need_flush_time,
		StepGenTime:      self.step_gen_time,
		ClearHistoryTime: self.clear_history_time,
		KinFlushDelay:    self.Kin_flush_delay,
		KinFlushTimes:    kinFlushTimes,
		DoKickFlushTimer: self.do_kick_flush_timer,
		CanPause:         self.Can_pause,
	}
}

func (self *Toolhead) applyTimingState(state motionpkg.ToolheadTimingState) {
	self.Print_time = state.PrintTime
	self.last_flush_time = state.LastFlushTime
	self.min_restart_time = state.MinRestartTime
	self.need_flush_time = state.NeedFlushTime
	self.step_gen_time = state.StepGenTime
	self.clear_history_time = state.ClearHistoryTime
	self.Kin_flush_delay = state.KinFlushDelay
	self.Kin_flush_times = append([]float64{}, state.KinFlushTimes...)
	self.do_kick_flush_timer = state.DoKickFlushTimer
	self.Can_pause = state.CanPause
}

func (self *Toolhead) timingFlushActions() motionpkg.FlushActions {
	flushDrivers := make([]motionpkg.MoveQueueFlusher, 0, len(self.All_mcus))
	for _, m := range self.All_mcus {
		flushDrivers = append(flushDrivers, &toolheadMoveQueueFlusher{mcu: m})
	}
	return motionpkg.FlushActions{
		StepGenerators: self.Step_generators,
		FinalizeMoves: func(freeTime float64, clearHistoryTime float64) {
			self.Trapq_finalize_moves(self.Trapq, freeTime, clearHistoryTime)
		},
		UpdateExtruderMoveTime: self.Extruder.Update_move_time,
		FlushDrivers:           flushDrivers,
	}
}

func (self *Toolhead) pauseConfig() motionpkg.ToolheadPauseConfig {
	return motionpkg.DefaultPauseConfig()
}

func (self *Toolhead) pauseState() motionpkg.ToolheadPauseState {
	return motionpkg.ToolheadPauseState{
		PrintTime:           self.Print_time,
		CheckStallTime:      self.Check_stall_time,
		PrintStall:          self.Print_stall,
		SpecialQueuingState: self.Special_queuing_state,
		NeedCheckPause:      self.Need_check_pause,
		CanPause:            self.Can_pause,
	}
}

func (self *Toolhead) applyPauseState(state motionpkg.ToolheadPauseState) {
	self.Print_time = state.PrintTime
	self.Check_stall_time = state.CheckStallTime
	self.Print_stall = state.PrintStall
	self.Special_queuing_state = state.SpecialQueuingState
	self.Need_check_pause = state.NeedCheckPause
	self.Can_pause = state.CanPause
}

func (self *Toolhead) flushConfig() motionpkg.ToolheadFlushConfig {
	return motionpkg.DefaultFlushConfig()
}

func (self *Toolhead) dripConfig() motionpkg.ToolheadDripConfig {
	return motionpkg.DefaultDripConfig()
}

func (self *Toolhead) dripMoveConfig() motionpkg.ToolheadDripMoveConfig {
	return motionpkg.DefaultDripMoveConfig()
}

func (self *Toolhead) Cmd_G4(arg interface{}) error {
	return motionpkg.HandleToolheadG4Command(&toolheadCommandAdapter{toolhead: self}, arg.(*GCodeCommand))
}

func (self *Toolhead) Cmd_M400(gcmd interface{}) error {
	_ = gcmd
	return motionpkg.HandleToolheadM400Command(&toolheadCommandAdapter{toolhead: self})
}

func (self *Toolhead) Cmd_SET_VELOCITY_LIMIT(arg interface{}) error {
	result, queryOnly := motionpkg.HandleToolheadSetVelocityLimitCommand(&toolheadCommandAdapter{toolhead: self}, arg.(*GCodeCommand))
	if queryOnly {
		logger.Debugf(result.Summary)
	}
	return nil
}

func (self *Toolhead) Cmd_M204(cmd interface{}) error {
	return motionpkg.HandleToolheadM204Command(&toolheadCommandAdapter{toolhead: self}, cmd.(*GCodeCommand))
}

func (self *Toolhead) M204(accel float64) {
	motionpkg.ApplyToolheadAcceleration(&toolheadCommandAdapter{toolhead: self}, accel)
}

func (self *Toolhead) _advance_flush_time(flush_time float64) {
	state := self.timingState()
	state.AdvanceFlushTime(flush_time, self.timingConfig(), self.timingFlushActions())
	self.applyTimingState(state)
}

func (self *Toolhead) _advance_move_time(next_print_time float64) {
	state := self.timingState()
	state.AdvanceMoveTime(next_print_time, self.timingConfig(), self.timingFlushActions())
	self.applyTimingState(state)
}

func (self *Toolhead) _calc_print_time() {
	state := self.timingState()
	state.CalcPrintTime(self.timingConfig(), &toolheadPrintTimeSource{reactor: self.Reactor, mcu: self.Mcu}, &toolheadPrintTimeNotifier{printer: self.Printer})
	self.applyTimingState(state)
}

func (self *Toolhead) Process_moves(moves []*Move) {
	if len(self.Special_queuing_state) > 0 {
		if self.Special_queuing_state != "Drip" {
			self.Special_queuing_state = ""
			self.Need_check_pause = -1.
		}
		self._calc_print_time()
	}

	next_move_time := motionpkg.QueueMoveBatch(self.Print_time, moves, &toolheadMoveBatchSink{toolhead: self})
	if self.Special_queuing_state != "" {
		err := motionpkg.UpdateToolheadDripTime(
			&toolheadDripRuntimeAdapter{toolhead: self},
			&toolheadPauseSource{reactor: self.Reactor, mcu: self.Mcu},
			self.Drip_completion,
			next_move_time,
			self.dripConfig(),
		)
		if err != nil {
			panic(err)
		}
	}
	self.Note_mcu_movequeue_activity(next_move_time+self.Kin_flush_delay, true)
	self._advance_move_time(next_move_time)
}

func (self *Toolhead) _flush_lookahead() {
	reset := motionpkg.BuildToolheadFlushReset(BUFFER_TIME_HIGH)
	self.lookahead.Flush(false)
	self.Special_queuing_state = reset.SpecialQueuingState
	self.Need_check_pause = reset.NeedCheckPause
	self.lookahead.Set_flush_time(reset.LookaheadFlushTime)
	self.Check_stall_time = reset.CheckStallTime
}

func (self *Toolhead) Flush_step_generation() {
	self._flush_lookahead()
	self._advance_flush_time(self.step_gen_time)
	self.min_restart_time = math.Max(self.min_restart_time, self.Print_time)
}

func (self *Toolhead) Get_last_move_time() float64 {
	if self.Special_queuing_state != "" {
		self._flush_lookahead()
		self._calc_print_time()
	} else {
		self.lookahead.Flush(false)
	}
	return self.Print_time
}

func (self *Toolhead) _Check_pause() {
	motionpkg.CheckToolheadPause(&toolheadPauseRuntimeAdapter{toolhead: self}, &toolheadPauseSource{reactor: self.Reactor, mcu: self.Mcu}, self.pauseConfig())
}

func (self *Toolhead) Priming_handler(eventtime float64) float64 {
	_ = eventtime
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Exception in priming_handler")
			self.Printer.Invoke_shutdown("Exception in priming_handler")
		}
	}()
	return motionpkg.HandleToolheadPrimingCallback(&toolheadPrimingRuntimeAdapter{toolhead: self})
}

func (self *Toolhead) _flush_handler(eventtime float64) float64 {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("Exception in flush_handler")
			self.Printer.Invoke_shutdown("Exception in flush_handler")
		}
	}()
	est_print_time := self.Mcu.Estimated_print_time(eventtime)
	return motionpkg.HandleToolheadFlushCallback(eventtime, est_print_time, &toolheadFlushRuntimeAdapter{toolhead: self}, self.flushConfig())
}

func (self *Toolhead) Get_position() []float64 {
	commanded_pos_back := make([]float64, len(self.Commanded_pos))
	copy(commanded_pos_back, self.Commanded_pos)
	return commanded_pos_back
}

func (self *Toolhead) GetPosition() []float64 {
	return self.Get_position()
}

func (self *Toolhead) Set_position(newpos []float64, homingAxes []int) {
	self.Flush_step_generation()
	chelper.Trapq_set_position(self.Trapq, self.Print_time,
		newpos[0], newpos[1], newpos[2])
	self.Commanded_pos = append([]float64{}, newpos...)
	self.Kin.Set_position(newpos, homingAxes)
	self.Printer.Send_event("toolhead:set_position", nil)
}

func (self *Toolhead) SetPosition(newpos []float64, homingAxes []int) {
	self.Set_position(newpos, homingAxes)
}

func (self *Toolhead) Move(newpos []float64, speed float64) {
	motionpkg.RunToolheadMove(&toolheadMoveControlAdapter{toolhead: self}, newpos, speed)
}

func (self *Toolhead) Manual_move(coord []interface{}, speed float64) {
	curpos := motionpkg.BuildToolheadManualMoveTarget(self.Commanded_pos, coord)
	self.Move(curpos, speed)
	self.Printer.Send_event("toolhead:manual_move", nil)
}

func (self *Toolhead) ManualMove(coord []interface{}, speed float64) {
	self.Manual_move(coord, speed)
}

func (self *Toolhead) Dwell(delay float64) {
	motionpkg.RunToolheadDwell(&toolheadDwellRuntimeAdapter{toolhead: self}, delay)
}

func (self *Toolhead) Wait_moves() {
	motionpkg.HandleToolheadWaitMoves(&toolheadWaitRuntimeAdapter{toolhead: self}, &toolheadPauseSource{reactor: self.Reactor, mcu: self.Mcu}, self.pauseConfig())
}

func (self *Toolhead) Set_extruder(extruder IExtruder, extrude_pos float64) {
	self.Extruder = extruder
	self.Commanded_pos[3] = extrude_pos
}

func (self *Toolhead) Get_extruder() IExtruder {
	return self.Extruder
}

func (self *Toolhead) Drip_move(newpos []float64, speed float64, drip_completion *ReactorCompletion) error {
	motionpkg.RunToolheadDripMove(&toolheadDripMoveRuntimeAdapter{toolhead: self}, newpos, speed, drip_completion, self.dripMoveConfig())
	return nil
}

func (self *Toolhead) Stats(eventtime float64) (bool, string) {
	est_print_time := self.Mcu.Estimated_print_time(eventtime)
	stats := motionpkg.BuildToolheadStats(motionpkg.ToolheadStatsSnapshot{
		PrintTime:           self.Print_time,
		LastFlushTime:       self.last_flush_time,
		EstimatedPrintTime:  est_print_time,
		PrintStall:          self.Print_stall,
		MoveHistoryExpire:   MOVE_HISTORY_EXPIRE,
		SpecialQueuingState: self.Special_queuing_state,
	})
	max_queue_time := stats.MaxQueueTime
	for _, m := range self.All_mcus {
		m.Check_active(max_queue_time, eventtime)
	}
	self.clear_history_time = stats.ClearHistoryTime
	return stats.IsActive, stats.Summary
}

func (self *Toolhead) Check_busy(eventtime float64) (float64, float64, bool) {
	busyState := motionpkg.BuildToolheadBusyState(
		self.Print_time,
		self.Mcu.Estimated_print_time(eventtime),
		self.lookahead.Queue_len() == 0,
	)
	return busyState.PrintTime, busyState.EstimatedPrintTime, busyState.LookaheadEmpty
}

func (self *Toolhead) Get_status(eventtime float64) map[string]interface{} {
	return motionpkg.BuildToolheadStatus(motionpkg.ToolheadStatusSnapshot{
		KinematicsStatus:      self.Kin.Get_status(eventtime),
		PrintTime:             self.Print_time,
		EstimatedPrintTime:    self.Mcu.Estimated_print_time(eventtime),
		PrintStall:            self.Print_stall,
		ExtruderName:          self.Extruder.Get_name(),
		CommandedPosition:     self.Commanded_pos,
		MaxVelocity:           self.Max_velocity,
		MaxAccel:              self.Max_accel,
		RequestedAccelToDecel: self.Requested_accel_to_decel,
		SquareCornerVelocity:  self.Square_corner_velocity,
	})
}

func (self *Toolhead) Handle_shutdown([]interface{}) error {
	self.Can_pause = false
	self.lookahead.Reset()
	return nil
}

func (self *Toolhead) Get_kinematics() interface{} {
	return self.Kin
}

func (self *Toolhead) Get_trapq() interface{} {
	return self.Trapq
}

func (self *Toolhead) Register_step_generator(handler func(float64)) {
	self.Step_generators = append(self.Step_generators, handler)
}

func (self *Toolhead) Note_step_generation_scan_time(delay, old_delay float64) {
	self.Flush_step_generation()
	state := self.timingState()
	state.UpdateStepGenerationScanDelay(delay, old_delay, self.timingConfig())
	self.applyTimingState(state)
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
	state := self.timingState()
	kickFlushTimer := state.NoteMovequeueActivity(mq_time, set_step_gen_time)
	self.applyTimingState(state)
	if kickFlushTimer {
		self.Reactor.Update_timer(self.Flush_timer, constants.NOW)
	}
}

func (self *Toolhead) NoteMCUMovequeueActivity(mqTime float64, setStepGenTime bool) {
	self.Note_mcu_movequeue_activity(mqTime, setStepGenTime)
}

func (self *Toolhead) HomedAxes(eventtime float64) string {
	status := self.Get_kinematics().(IKinematics).Get_status(eventtime)
	axes, _ := status["homed_axes"].(string)
	return axes
}

func (self *Toolhead) NoteZNotHomed() {
	if kin, ok := self.Get_kinematics().(interface{ Note_z_not_homed() }); ok {
		kin.Note_z_not_homed()
	}
}

func (self *Toolhead) Get_max_velocity() (float64, float64) {
	return self.Max_velocity, self.Max_accel
}
