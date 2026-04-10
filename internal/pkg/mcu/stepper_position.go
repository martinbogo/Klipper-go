package mcu

type StepcompressQueue interface {
	FindPastPosition(clock uint64) int64
	Reset(lastStepClock uint64) int
	QueueMessage(data []uint32) int
	SetLastPosition(clock uint64, lastPosition int64) int
}

type StepperPositionState struct {
	StepDist          float64
	MCUPositionOffset float64
	InvertDir         uint32
}

func (self *StepperPositionState) GetMCUPosition(commandedPosition float64) int {
	mcuPosDist := commandedPosition + self.MCUPositionOffset
	mcuPos := mcuPosDist / self.StepDist
	if mcuPos >= 0 {
		return int(mcuPos + 0.5)
	}
	return int(mcuPos - 0.5)
}

func (self *StepperPositionState) SetMCUPosition(mcuPos int, commandedPosition float64) {
	mcuPosDist := float64(mcuPos) * self.StepDist
	self.MCUPositionOffset = mcuPosDist - commandedPosition
}

func (self *StepperPositionState) PastMCUPosition(printTime float64, printTimeToClock func(float64) int64, queue StepcompressQueue) int {
	clock := printTimeToClock(printTime)
	return int(queue.FindPastPosition(uint64(clock)))
}

func (self *StepperPositionState) MCUToCommandedPosition(mcuPos int) float64 {
	return float64(mcuPos)*self.StepDist - self.MCUPositionOffset
}

func (self *StepperPositionState) NoteHomingEnd(queue StepcompressQueue, resetCmdTag int, oid int) error {
	if queue.Reset(0) > 0 {
		return ErrStepcompress
	}
	data := []uint32{uint32(resetCmdTag), uint32(oid), 0}
	if queue.QueueMessage(data) > 0 {
		return ErrStepcompress
	}
	return nil
}

func (self *StepperPositionState) SyncFromQueryResponse(rawPosition int64, receiveTime float64, commandedPosition float64, estimatedPrintTime func(float64) float64, printTimeToClock func(float64) int64, queue StepcompressQueue) (int, error) {
	lastPos := int(int32(rawPosition))
	if self.InvertDir > 0 {
		lastPos = -lastPos
	}
	printTime := estimatedPrintTime(receiveTime)
	clock := printTimeToClock(printTime)
	if queue.SetLastPosition(uint64(clock), int64(lastPos)) > 0 {
		return 0, ErrStepcompress
	}
	self.SetMCUPosition(lastPos, commandedPosition)
	return lastPos, nil
}
