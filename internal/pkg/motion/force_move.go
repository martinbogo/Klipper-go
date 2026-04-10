package motion

import (
	"goklipper/internal/pkg/chelper"
	"math"
)

type ForceMoveStepper interface {
	Set_stepper_kinematics(sk interface{}) interface{}
	Set_trapq(tq interface{}) interface{}
	Set_position(coord []float64)
	Generate_steps(flush_time float64)
}

type ForceMoveToolhead interface {
	Flush_step_generation()
	Get_last_move_time() float64
	Note_mcu_movequeue_activity(mq_time float64, set_step_gen_time bool)
	Dwell(delay float64)
}

type ForceMover struct {
	trapq       interface{}
	trapqAppend func(tq interface{}, print_time, accel_t, cruise_t, decel_t,
		start_pos_x, start_pos_y, start_pos_z, axes_r_x, axes_r_y,
		axes_r_z, start_v, cruise_v, accel float64)
	trapqFinalizeMoves func(interface{}, float64, float64)
	stepperKinematics  interface{}
}

func NewForceMover() *ForceMover {
	return &ForceMover{
		trapq:              chelper.Trapq_alloc(),
		trapqAppend:        chelper.Trapq_append,
		trapqFinalizeMoves: chelper.Trapq_finalize_moves,
		stepperKinematics:  chelper.Cartesian_stepper_alloc('x'),
	}
}

func CalcMoveTime(dist float64, speed float64, accel *float64) (float64, float64, float64, float64) {
	axisR := 1.
	if dist < 0 {
		axisR = -1.
		dist = -dist
	}

	if accel == nil || dist == 0 {
		return axisR, 0., dist / speed, speed
	}

	accelValue := *accel
	if accelValue <= 0 {
		return axisR, 0., dist / speed, speed
	}

	maxCruiseV2 := dist * accelValue
	if maxCruiseV2 < math.Pow(speed, 2) {
		speed = math.Sqrt(maxCruiseV2)
	}

	accelT := speed / accelValue
	accelDecelD := accelT * speed
	cruiseT := (dist - accelDecelD) / speed
	return axisR, accelT, cruiseT, speed
}

func (self *ForceMover) ManualMove(toolhead ForceMoveToolhead, stepper ForceMoveStepper, dist, speed float64, accel *float64) {
	toolhead.Flush_step_generation()

	prevSK := stepper.Set_stepper_kinematics(self.stepperKinematics)
	prevTrapq := stepper.Set_trapq(self.trapq)
	stepper.Set_position([]float64{0, 0, 0})

	accelValue := 0.0
	if accel != nil {
		accelValue = *accel
	}
	axisR, accelT, cruiseT, cruiseV := CalcMoveTime(dist, speed, accel)

	printTime := toolhead.Get_last_move_time()
	self.trapqAppend(self.trapq, printTime, accelT, cruiseT, accelT,
		0., 0., 0., axisR, 0., 0., 0., cruiseV, accelValue)
	printTime = printTime + accelT + cruiseT + accelT

	stepper.Generate_steps(printTime)
	self.trapqFinalizeMoves(self.trapq, printTime+99999.9, printTime+99999.9)
	stepper.Set_trapq(prevTrapq)
	stepper.Set_stepper_kinematics(prevSK)
	toolhead.Note_mcu_movequeue_activity(printTime, false)
	toolhead.Dwell(accelT + cruiseT + accelT)
	toolhead.Flush_step_generation()
}
