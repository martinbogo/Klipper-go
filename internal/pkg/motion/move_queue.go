package motion

import (
	"errors"
	"fmt"
	"math"
)

type MoveConfig struct {
	Max_accel          float64
	Junction_deviation float64
	Max_velocity       float64
	Max_accel_to_decel float64
}

type MoveJunctionCalculator interface {
	Calc_junction(prev_move, move *Move) float64
}

type MoveQueueProcessor interface {
	Process_moves(moves []*Move)
}

type Move struct {
	Start_pos          []float64
	End_pos            []float64
	Accel              float64
	Junction_deviation float64
	Timing_callbacks   []func(float64)
	Is_kinematic_move  bool
	Axes_d             []float64
	Move_d             float64
	Axes_r             []float64
	Min_move_t         float64
	Max_start_v2       float64
	Max_cruise_v2      float64
	Delta_v2           float64
	Next_junction_v2   float64
	Max_mcr_start_v2   float64
	Mcr_delta_v2       float64
	Max_smoothed_v2    float64
	Smooth_delta_v2    float64
	Start_v            float64
	Cruise_v           float64
	End_v              float64
	Accel_t            float64
	Cruise_t           float64
	Decel_t            float64
}

func NewMove(config MoveConfig, start_pos, end_pos []float64, speed float64) *Move {
	self := &Move{}
	self.Start_pos = append([]float64{}, start_pos...)
	self.End_pos = append([]float64{}, end_pos...)
	self.Accel = config.Max_accel
	self.Junction_deviation = config.Junction_deviation
	self.Timing_callbacks = []func(float64){}
	velocity := math.Min(speed, config.Max_velocity)
	self.Is_kinematic_move = true
	axes_d := []float64{
		end_pos[0] - start_pos[0],
		end_pos[1] - start_pos[1],
		end_pos[2] - start_pos[2],
		end_pos[3] - start_pos[3],
	}
	self.Axes_d = axes_d
	move_d := math.Sqrt(axes_d[0]*axes_d[0] + axes_d[1]*axes_d[1] + axes_d[2]*axes_d[2])
	self.Move_d = move_d
	inv_move_d := 0.
	if move_d < 0.000000001 {
		self.End_pos = []float64{start_pos[0], start_pos[1], start_pos[2], end_pos[3]}
		axes_d[0], axes_d[1], axes_d[2] = 0., 0., 0.
		move_d = math.Abs(axes_d[3])
		self.Move_d = move_d
		inv_move_d = 0.
		if move_d != 0. {
			inv_move_d = 1. / move_d
		}
		self.Accel = 99999999.9
		velocity = speed
		self.Is_kinematic_move = false
	} else {
		inv_move_d = 1. / self.Move_d
	}
	self.Axes_r = []float64{
		axes_d[0] * inv_move_d,
		axes_d[1] * inv_move_d,
		axes_d[2] * inv_move_d,
		axes_d[3] * inv_move_d,
	}
	self.Min_move_t = self.Move_d / velocity
	self.Max_start_v2 = 0.
	self.Max_cruise_v2 = velocity * velocity
	self.Delta_v2 = 2. * self.Move_d * self.Accel
	self.Next_junction_v2 = 999999999.9
	self.Max_mcr_start_v2 = 0.
	self.Mcr_delta_v2 = 2. * self.Move_d * config.Max_accel_to_decel
	self.Max_smoothed_v2 = 0.
	self.Smooth_delta_v2 = self.Mcr_delta_v2
	return self
}

func (self *Move) Limit_speed(speed float64, accel float64) {
	speed2 := speed * speed
	if speed2 < self.Max_cruise_v2 {
		self.Max_cruise_v2 = speed2
		self.Min_move_t = self.Move_d / speed
	}
	self.Accel = math.Min(self.Accel, accel)
	self.Delta_v2 = 2.0 * self.Move_d * self.Accel
	self.Mcr_delta_v2 = math.Min(self.Mcr_delta_v2, self.Delta_v2)
	self.Smooth_delta_v2 = self.Mcr_delta_v2
}

func (self *Move) Limit_next_junction_speed(speed float64) {
	self.Next_junction_v2 = math.Min(self.Next_junction_v2, speed*speed)
}

func (self *Move) LimitNextJunctionSpeed(speed float64) {
	self.Limit_next_junction_speed(speed)
}

func (self *Move) Move_error(msg string) error {
	ep := self.End_pos
	m := fmt.Sprintf("%s: %.3f %.3f %.3f [%.3f]", msg, ep[0], ep[1], ep[2], ep[3])
	return errors.New(m)
}

func (self *Move) EndPos() []float64 {
	return self.End_pos
}

func (self *Move) AxesD() []float64 {
	return self.Axes_d
}

func (self *Move) MoveD() float64 {
	return self.Move_d
}

func (self *Move) LimitSpeed(speed float64, accel float64) {
	self.Limit_speed(speed, accel)
}

func (self *Move) MoveError(msg string) error {
	return self.Move_error(msg)
}

func customMin(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minValue := values[0]
	for _, v := range values[1:] {
		if v < minValue {
			minValue = v
		}
	}
	return minValue
}

func (self *Move) Calc_junction(prev_move *Move, extruder MoveJunctionCalculator) {
	if !self.Is_kinematic_move || !prev_move.Is_kinematic_move {
		return
	}
	extruder_v2 := self.Max_cruise_v2
	if extruder != nil {
		extruder_v2 = extruder.Calc_junction(prev_move, self)
	}
	max_start_v2 := customMin(extruder_v2, self.Max_cruise_v2,
		prev_move.Max_cruise_v2, prev_move.Next_junction_v2,
		prev_move.Max_start_v2+prev_move.Delta_v2)
	axes_r := self.Axes_r
	prev_axes_r := prev_move.Axes_r
	junction_cos_theta := -(axes_r[0]*prev_axes_r[0] + axes_r[1]*prev_axes_r[1] + axes_r[2]*prev_axes_r[2])
	sin_theta_d2 := math.Sqrt(math.Max(0.5*(1.0-junction_cos_theta), 0.))
	cos_theta_d2 := math.Sqrt(math.Max(0.5*(1.0+junction_cos_theta), 0.))
	one_minus_sin_theta_d2 := 1. - sin_theta_d2
	if one_minus_sin_theta_d2 > 0. && cos_theta_d2 > 0. {
		R_jd := sin_theta_d2 / one_minus_sin_theta_d2
		move_jd_v2 := R_jd * self.Junction_deviation * self.Accel
		pmove_jd_v2 := R_jd * prev_move.Junction_deviation * prev_move.Accel
		quarter_tan_theta_d2 := .25 * sin_theta_d2 / cos_theta_d2
		move_centripetal_v2 := self.Delta_v2 * quarter_tan_theta_d2
		pmove_centripetal_v2 := prev_move.Delta_v2 * quarter_tan_theta_d2
		max_start_v2 = customMin(max_start_v2, move_jd_v2, pmove_jd_v2,
			move_centripetal_v2, pmove_centripetal_v2)
	}
	self.Max_start_v2 = max_start_v2
	self.Max_mcr_start_v2 = math.Min(max_start_v2, prev_move.Max_mcr_start_v2+prev_move.Mcr_delta_v2)
	self.Max_smoothed_v2 = self.Max_mcr_start_v2
}

func (self *Move) Set_junction(start_v2, cruise_v2, end_v2 float64) {
	half_inv_accel := 0.5 / self.Accel
	accel_d := (cruise_v2 - start_v2) * half_inv_accel
	decel_d := (cruise_v2 - end_v2) * half_inv_accel
	cruise_d := self.Move_d - accel_d - decel_d
	start_v := math.Sqrt(start_v2)
	self.Start_v = start_v
	cruise_v := math.Sqrt(cruise_v2)
	self.Cruise_v = cruise_v
	end_v := math.Sqrt(end_v2)
	self.End_v = end_v
	self.Accel_t = accel_d / ((start_v + cruise_v) * 0.5)
	self.Cruise_t = cruise_d / cruise_v
	self.Decel_t = decel_d / ((end_v + cruise_v) * 0.5)
}

const LOOKAHEAD_FLUSH_TIME = 0.150

type junctionNode struct {
	Move        *Move
	StartV2     float64
	CruiseV2    float64
	HasCruiseV2 bool
	NextStartV2 float64
}

type LookAheadQueue struct {
	processor      MoveQueueProcessor
	queue          []*Move
	junction_flush float64
}

func NewMoveQueue(processor MoveQueueProcessor) *LookAheadQueue {
	return &LookAheadQueue{
		processor:      processor,
		queue:          []*Move{},
		junction_flush: LOOKAHEAD_FLUSH_TIME,
	}
}

func (self *LookAheadQueue) Reset() {
	self.queue = []*Move{}
	self.junction_flush = LOOKAHEAD_FLUSH_TIME
}

func (self *LookAheadQueue) Set_flush_time(flush_time float64) {
	self.junction_flush = flush_time
}

func (self *LookAheadQueue) Get_last() *Move {
	if len(self.queue) > 0 {
		return self.queue[len(self.queue)-1]
	}
	return nil
}

func (self *LookAheadQueue) Queue_len() int {
	return len(self.queue)
}

func (self *LookAheadQueue) Flush(lazy bool) {
	self.junction_flush = LOOKAHEAD_FLUSH_TIME
	update_flush_count := lazy
	queue := self.queue
	flush_count := len(queue)
	junctionInfo := make([]junctionNode, flush_count)
	next_start_v2, next_mcr_start_v2, peak_cruise_v2 := 0., 0., 0.
	pending_cruise_assign := 0
	for i := flush_count - 1; i >= 0; i-- {
		move := queue[i]
		reachable_start_v2 := next_start_v2 + move.Delta_v2
		start_v2 := math.Min(move.Max_start_v2, reachable_start_v2)
		cruise_v2 := 0.0
		hasCruiseV2 := false
		pending_cruise_assign += 1
		reachable_mcr_start_v2 := next_mcr_start_v2 + move.Mcr_delta_v2
		mcr_start_v2 := math.Min(move.Max_mcr_start_v2, reachable_mcr_start_v2)
		if mcr_start_v2 < reachable_mcr_start_v2 {
			if mcr_start_v2+move.Mcr_delta_v2 > next_mcr_start_v2 || pending_cruise_assign > 1 {
				if update_flush_count && peak_cruise_v2 != 0 {
					flush_count = i + pending_cruise_assign
					update_flush_count = false
				}
				peak_cruise_v2 = (mcr_start_v2 + reachable_mcr_start_v2) * .5
			}
			cruise_v2 = customMin((start_v2+reachable_start_v2)*.5,
				move.Max_cruise_v2, peak_cruise_v2)
			hasCruiseV2 = true
			pending_cruise_assign = 0
		}
		junctionInfo[i] = junctionNode{
			Move:        move,
			StartV2:     start_v2,
			CruiseV2:    cruise_v2,
			HasCruiseV2: hasCruiseV2,
			NextStartV2: next_start_v2,
		}
		next_start_v2 = start_v2
		next_mcr_start_v2 = mcr_start_v2
	}
	if update_flush_count || flush_count <= 0 {
		return
	}
	prev_cruise_v2 := 0.0
	for i := 0; i < flush_count; i++ {
		node := junctionInfo[i]
		cruise_v2 := node.CruiseV2
		if !node.HasCruiseV2 {
			cruise_v2 = math.Min(prev_cruise_v2, node.StartV2)
		}
		node.Move.Set_junction(math.Min(node.StartV2, cruise_v2), cruise_v2,
			math.Min(node.NextStartV2, cruise_v2))
		prev_cruise_v2 = cruise_v2
	}
	self.processor.Process_moves(queue[:flush_count])
	if flush_count < len(self.queue) && len(self.queue) > 0 {
		self.queue = self.queue[flush_count:]
	} else if len(self.queue) > 0 {
		self.queue = []*Move{}
	}
}

func (self *LookAheadQueue) Add_move(move *Move, extruder MoveJunctionCalculator) bool {
	self.queue = append(self.queue, move)
	if len(self.queue) == 1 {
		return false
	}
	move.Calc_junction(self.queue[len(self.queue)-2], extruder)
	self.junction_flush -= move.Min_move_t
	return self.junction_flush <= 0.
}
