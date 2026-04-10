package motion

import "goklipper/internal/pkg/chelper"

type ManualStepperCore struct {
	nextCmdTime        float64
	trapq              interface{}
	trapqAppend        func(tq interface{}, print_time, accel_t, cruise_t, decel_t,
		start_pos_x, start_pos_y, start_pos_z, axes_r_x, axes_r_y,
		axes_r_z, start_v, cruise_v, accel float64)
	trapqFinalizeMoves func(interface{}, float64, float64)
}

func NewManualStepperCore() *ManualStepperCore {
	return &ManualStepperCore{
		trapq:              chelper.Trapq_alloc(),
		trapqAppend:        chelper.Trapq_append,
		trapqFinalizeMoves: chelper.Trapq_finalize_moves,
	}
}

func (self *ManualStepperCore) Trapq() interface{} {
	if self == nil {
		return nil
	}
	return self.trapq
}

func (self *ManualStepperCore) NextCmdTime() float64 {
	if self == nil {
		return 0
	}
	return self.nextCmdTime
}

func (self *ManualStepperCore) SyncPrintTime(printTime float64) float64 {
	if self == nil {
		return 0
	}
	if self.nextCmdTime > printTime {
		return self.nextCmdTime - printTime
	}
	self.nextCmdTime = printTime
	return 0
}

func (self *ManualStepperCore) QueueMove(currentPos, movePos, speed, accel float64) float64 {
	dist := movePos - currentPos
	axisR, accelT, cruiseT, cruiseV := CalcMoveTime(dist, speed, &accel)
	self.trapqAppend(self.trapq, self.nextCmdTime, accelT, cruiseT, accelT,
		currentPos, 0, 0, axisR, 0, 0,
		0, cruiseV, accel)
	self.nextCmdTime += accelT + cruiseT + accelT
	return self.nextCmdTime
}

func (self *ManualStepperCore) FinalizeMoves() {
	if self == nil {
		return
	}
	self.trapqFinalizeMoves(self.trapq, self.nextCmdTime+99999.9, self.nextCmdTime+99999.9)
}

func (self *ManualStepperCore) Dwell(delay float64) {
	if self == nil || delay <= 0 {
		return
	}
	self.nextCmdTime += delay
}