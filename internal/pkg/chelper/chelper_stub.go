//go:build !linux || !cgo
// +build !linux !cgo

package chelper

import "unsafe"

const (
	GCC_CMD    = "gcc"
	messageMax = 64
)

var NULL = unsafe.Pointer(nil)

type pullQueueMessage struct {
	Msg          [messageMax]uint8
	Len          int
	Sent_time    float64
	Receive_time float64
	Notify_id    uint64
}

type pullHistorySteps struct {
	First_clock    uint64
	Last_clock     uint64
	Start_position int64
	Step_count     int
	Interval       int
	Add            int
}

type pullMove struct {
	Print_time float64
	Move_t     float64
	Start_v    float64
	Accel      float64
	Start_x    float64
	Start_y    float64
	Start_z    float64
	X_r        float64
	Y_r        float64
	Z_r        float64
}

type serialqueue struct{}
type commandQueue struct{}
type trdispatch struct{}
type trdispatchMCU struct{}
type steppersync struct{}

type stepcompress struct {
	oid            uint32
	invertSdir     uint32
	lastPosition   int64
	lastStepClock  uint64
	queuedMessages [][]uint32
}

type stepperKinematics struct {
	kind         string
	axis         int8
	commandedPos float64
	trapq        interface{}
	stepqueue    interface{}
	stepDist     float64
	pressureAdv  float64
	smoothTime   float64
	shaperSource interface{}
	shaperN      int
	shaperA      []float64
	shaperT      []float64
}

type trapq struct {
	position [3]float64
}

type FFI_lib struct{}

func (self *FFI_lib) Get_monotonic() float64 {
	return 0
}

func Free(p interface{}) {}

func get_abs_files(srcdir string, filelist []string) []string {
	_, _ = srcdir, filelist
	return nil
}

func get_mtimes(filelist []string) []float64 {
	_ = filelist
	return nil
}

func check_build_code(sources []string, target string) bool {
	_, _ = sources, target
	return false
}

func check_gcc_option(option string) bool {
	_ = option
	return false
}

var stubLib = &FFI_lib{}

func Get_ffi() *FFI_lib {
	return stubLib
}

func New_pull_queue_message() *pullQueueMessage {
	msg := &pullQueueMessage{}
	msg.Len = -1
	return msg
}

func Serialqueue_pull(serialqueue interface{}, response interface{}) {
	_, _ = serialqueue, response
	if response == nil {
		return
	}
	resp, ok := response.(*pullQueueMessage)
	if !ok {
		return
	}
	resp.Len = -1
}

func Serialqueue_alloc(fileno uintptr, serial_fd_type byte, client_id int) *serialqueue {
	_, _, _ = fileno, serial_fd_type, client_id
	return &serialqueue{}
}

func Serialqueue_free(serialqueue interface{}) {}

func Serialqueue_alloc_commandqueue() *commandQueue {
	return &commandQueue{}
}

func Serialqueue_free_commandqueue(commandqueue interface{}) {}

func Serialqueue_exit(serialqueue interface{}) {}

func Serialqueue_set_wire_frequency(serialqueue interface{}, wire_freq float64) {}

func Serialqueue_set_receive_window(serialqueue interface{}, receive_window int) {}

func Serialqueue_set_clock_est(serialqueue interface{}, freq float64, conv_time float64, conv_clock uint64, last_clock uint64) {
	_, _, _, _, _ = serialqueue, freq, conv_time, conv_clock, last_clock
}

func Serialqueue_send(serialqueue interface{}, cmd_queue interface{}, cmd []int, count int, minclock, reqclock, last int64) {
	_, _, _, _, _, _, _ = serialqueue, cmd_queue, cmd, count, minclock, reqclock, last
}

func Serialqueue_get_stats(serialqueue interface{}, stats_buf []byte) {
	_, _ = serialqueue, stats_buf
}

func Steppersync_alloc(serialqueue interface{}, sc_list interface{}, sc_num int, move_num int) *steppersync {
	_, _, _, _ = serialqueue, sc_list, sc_num, move_num
	return &steppersync{}
}

func Steppersync_free(syc interface{}) {}

func Steppersync_set_time(syc interface{}, time_offset float64, mcu_freq float64) {}

func Steppersync_flush(syc interface{}, move_clock uint64, clear_history_clock uint64) int {
	_, _, _ = syc, move_clock, clear_history_clock
	return 0
}

func Trdispatch_mcu_alloc(td interface{}, sq interface{}, cq interface{}, trsync_oid int, set_timeout_msgtag uint32, trigger_msgtag uint32, state_msgtag uint32) *trdispatchMCU {
	_, _, _, _, _, _, _ = td, sq, cq, trsync_oid, set_timeout_msgtag, trigger_msgtag, state_msgtag
	return &trdispatchMCU{}
}

func Trdispatch_alloc() *trdispatch {
	return &trdispatch{}
}

func Trdispatch_start(td interface{}, dispatch_reason uint32) {}

func Trdispatch_stop(td interface{}) {}

func Trdispatch_mcu_setup(tdm interface{}, last_status_clock uint64, expire_clock uint64, expire_ticks uint64, min_extend_ticks uint64) {
	_, _, _, _, _ = tdm, last_status_clock, expire_clock, expire_ticks, min_extend_ticks
}

func Stepcompress_alloc(oid uint32) *stepcompress {
	return &stepcompress{oid: oid}
}

func Stepcompress_free(p interface{}) {}

func Stepcompress_fill(sc interface{}, oid uint32, max_error uint32, queue_step_msgtag int32, set_next_step_dir_msgtag int32) {
	typed, ok := sc.(*stepcompress)
	if ok && typed != nil {
		typed.oid = oid
	}
	_, _, _, _ = max_error, queue_step_msgtag, set_next_step_dir_msgtag, ok
}

func Stepcompress_set_invert_sdir(sc interface{}, invert_sdir uint32) {
	typed, ok := sc.(*stepcompress)
	if !ok || typed == nil {
		return
	}
	typed.invertSdir = invert_sdir
}

func Stepcompress_find_past_position(sc interface{}, clock uint64) int64 {
	_, _ = sc, clock
	typed, ok := sc.(*stepcompress)
	if !ok || typed == nil {
		return 0
	}
	return typed.lastPosition
}

func Stepcompress_extract_old(sc interface{}, p interface{}, max int, start_clock uint64, end_clock uint64) int {
	_, _, _, _, _ = sc, p, max, start_clock, end_clock
	return 0
}

func Stepcompress_reset(sc interface{}, last_step_clock uint64) int {
	typed, ok := sc.(*stepcompress)
	if ok && typed != nil {
		typed.lastStepClock = last_step_clock
	}
	return 0
}

func Stepcompress_queue_msg(sc interface{}, data interface{}, length int) int {
	typed, ok := sc.(*stepcompress)
	if !ok || typed == nil {
		return 0
	}
	if rows, ok := data.([]uint32); ok {
		copyRow := append([]uint32(nil), rows...)
		typed.queuedMessages = append(typed.queuedMessages, copyRow)
	}
	_ = length
	return 0
}

func Stepcompress_set_last_position(sc interface{}, clock uint64, last_position int64) int {
	typed, ok := sc.(*stepcompress)
	if ok && typed != nil {
		typed.lastStepClock = clock
		typed.lastPosition = last_position
	}
	return 0
}

func New_pull_history_steps() []pullHistorySteps {
	return []pullHistorySteps{}
}

func Itersolve_calc_position_from_coord(sk interface{}, x float64, y float64, z float64) float64 {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return 0
	}
	switch typed.kind {
	case "corexy":
		if typed.axis == 'x' {
			return x + y
		}
		return x - y
	default:
		_ = y
		_ = z
		return x
	}
}

func Itersolve_set_position(sk interface{}, x float64, y float64, z float64) {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return
	}
	typed.commandedPos = Itersolve_calc_position_from_coord(typed, x, y, z)
}

func Itersolve_get_commanded_pos(sk interface{}) float64 {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return 0
	}
	return typed.commandedPos
}

func Itersolve_set_stepcompress(sk interface{}, sc interface{}, step_dist float64) {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return
	}
	typed.stepqueue = sc
	typed.stepDist = step_dist
}

func Itersolve_set_trapq(sk interface{}, tq interface{}) {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return
	}
	typed.trapq = tq
}

func Itersolve_is_active_axis(sk interface{}, axis byte) int32 {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return 0
	}
	if byte(typed.axis) == axis {
		return 1
	}
	if typed.kind == "extruder" {
		return 1
	}
	return 0
}

func Itersolve_generate_steps(sk interface{}, flush_time float64) int32 {
	_, _ = sk, flush_time
	return 0
}

func Itersolve_check_active(sk interface{}, flush_time float64) float64 {
	_, _ = sk, flush_time
	return 0
}

func Trapq_alloc() *trapq {
	return &trapq{}
}

func Trapq_free(tq interface{}) {}

func Trapq_append(tq interface{}, print_time float64, accel_t float64, cruise_t float64, decel_t float64, start_pos_x float64, start_pos_y float64, start_pos_z float64, axes_r_x float64, axes_r_y float64, axes_r_z float64, start_v float64, cruise_v float64, accel float64) {
	_, _, _, _, _, _, _, _, _, _, _, _, _, _ = tq, print_time, accel_t, cruise_t, decel_t, start_pos_x, start_pos_y, start_pos_z, axes_r_x, axes_r_y, axes_r_z, start_v, cruise_v, accel
}

func Trapq_finalize_moves(tq interface{}, print_time float64, clear_history_time float64) {
	_, _, _ = tq, print_time, clear_history_time
}

func Trapq_set_position(tq interface{}, print_time float64, pos_x float64, pos_y float64, pos_z float64) {
	_, _ = tq, print_time
	typed, ok := tq.(*trapq)
	if !ok || typed == nil {
		return
	}
	typed.position = [3]float64{pos_x, pos_y, pos_z}
}

func Input_shaper_set_shaper_params(sk interface{}, axis int8, n int, a []float64, t []float64) int {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return -1
	}
	typed.axis = axis
	typed.shaperN = n
	typed.shaperA = append([]float64(nil), a...)
	typed.shaperT = append([]float64(nil), t...)
	return 0
}

func Input_shaper_get_step_generation_window(n int, a []float64, t []float64) float64 {
	_, _, _ = n, a, t
	return 0
}

func Input_shaper_alloc() *stepperKinematics {
	return &stepperKinematics{kind: "input_shaper"}
}

func Input_shaper_set_sk(sk interface{}, orig_sk interface{}) int {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return -1
	}
	typed.shaperSource = orig_sk
	return 0
}

func Extruder_stepper_alloc() *stepperKinematics {
	return &stepperKinematics{kind: "extruder", axis: 'e'}
}

func Extruder_set_pressure_advance(sk interface{}, pressure_advance float64, smooth_time float64) {
	typed, ok := sk.(*stepperKinematics)
	if !ok || typed == nil {
		return
	}
	typed.pressureAdv = pressure_advance
	typed.smoothTime = smooth_time
}

func CdoubleTofloat64(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	default:
		return 0
	}
}

func Cartesian_stepper_alloc(axis int8) *stepperKinematics {
	return &stepperKinematics{kind: "cartesian", axis: axis}
}

func Corexy_stepper_alloc(axis int8) *stepperKinematics {
	return &stepperKinematics{kind: "corexy", axis: axis}
}

func Cartesian_reverse_stepper_alloc(axis int8) *stepperKinematics {
	return &stepperKinematics{kind: "cartesian_reverse", axis: axis}
}

type CStruct_pull_move = pullMove

func Trapq_extract_old(tq interface{}, data []CStruct_pull_move, max int, start_time, end_time float64) int {
	_, _, _, _, _ = tq, data, max, start_time, end_time
	return 0
}

func Pull_move_alloc(size int) []CStruct_pull_move {
	return make([]pullMove, size)
}

type CStructPullHistorySteps = pullHistorySteps
type CStructPullMove = pullMove
type CStructPullQueueMessage = pullQueueMessage
