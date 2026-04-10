package report

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

const NeverTime = 9999999999999999.

type StepQueueEntry struct {
	FirstClock    uint64
	LastClock     uint64
	StartPosition int
	Interval      int
	StepCount     int
	Add           int
}

type StepperSource interface {
	Name() string
	MCUName() string
	DumpSteps(count int, startClock uint64, endClock uint64) ([]StepQueueEntry, int)
	ClockToPrintTime(clock int64) float64
	MCUToCommandedPosition(mcuPos int) float64
	StepDistance() float64
}

type StepperDump struct {
	source       StepperSource
	lastAPIClock uint64
}

func NewStepperDump(source StepperSource) *StepperDump {
	return &StepperDump{source: source}
}

func (self *StepperDump) StepQueue(startClock, endClock uint64) []StepQueueEntry {
	batches := make([][]StepQueueEntry, 0)
	for {
		data, count := self.source.DumpSteps(128, startClock, endClock)
		if count == 0 {
			break
		}
		if count > len(data) {
			count = len(data)
		}
		batch := append([]StepQueueEntry(nil), data[:count]...)
		batches = append(batches, batch)
		if count < len(data) {
			break
		}
		endClock = batch[count-1].FirstClock
	}

	steps := make([]StepQueueEntry, 0)
	for batchIndex := len(batches) - 1; batchIndex >= 0; batchIndex-- {
		batch := batches[batchIndex]
		for entryIndex := len(batch) - 1; entryIndex >= 0; entryIndex-- {
			steps = append(steps, batch[entryIndex])
		}
	}
	return steps
}

func (self *StepperDump) LogMessage(data []StepQueueEntry) string {
	if len(data) == 0 {
		return ""
	}
	out := []string{fmt.Sprintf("Dumping stepper '%s' (%s) %d queue_step:", self.source.Name(), self.source.MCUName(), len(data))}
	for i, entry := range data {
		out = append(out, fmt.Sprintf("queue_step %d: t=%d p=%d i=%d c=%d a=%d", i, entry.FirstClock, entry.StartPosition, entry.Interval, entry.StepCount, entry.Add))
	}
	return strings.Join(out, "\n")
}

func (self *StepperDump) APIUpdate() map[string]interface{} {
	data := self.StepQueue(self.lastAPIClock, uint64(1)<<63)
	if len(data) == 0 {
		return map[string]interface{}{}
	}
	first := data[0]
	firstClock := first.FirstClock
	firstTime := self.source.ClockToPrintTime(int64(firstClock))
	self.lastAPIClock = data[len(data)-1].LastClock
	lastClock := int64(self.lastAPIClock)
	lastTime := self.source.ClockToPrintTime(lastClock)
	mcuPos := first.StartPosition
	startPosition := self.source.MCUToCommandedPosition(mcuPos)
	stepDist := self.source.StepDistance()
	dumpData := make([][]int, 0, len(data))
	for _, entry := range data {
		dumpData = append(dumpData, []int{entry.Interval, entry.StepCount, entry.Add})
	}
	return map[string]interface{}{
		"data":               dumpData,
		"start_position":     startPosition,
		"start_mcu_position": mcuPos,
		"step_distance":      stepDist,
		"first_clock":        firstClock,
		"first_step_time":    firstTime,
		"last_clock":         lastClock,
		"last_step_time":     lastTime,
	}
}

type TrapQMove struct {
	PrintTime     float64
	MoveTime      float64
	StartVelocity float64
	Acceleration  float64
	StartPosition [3]float64
	Direction     [3]float64
}

type TrapQSource interface {
	ExtractMoves(limit int, startTime float64, endTime float64) ([]TrapQMove, int)
}

type TrapQDump struct {
	name           string
	source         TrapQSource
	lastAPIMessage []interface{}
}

func NewTrapQDump(name string, source TrapQSource) *TrapQDump {
	return &TrapQDump{
		name:           name,
		source:         source,
		lastAPIMessage: []interface{}{0.0, 0.0},
	}
}

func (self *TrapQDump) Moves(startTime, endTime float64) []TrapQMove {
	batches := make([][]TrapQMove, 0)
	for {
		data, count := self.source.ExtractMoves(128, startTime, endTime)
		if count == 0 {
			break
		}
		if count > len(data) {
			count = len(data)
		}
		batch := append([]TrapQMove(nil), data[:count]...)
		batches = append(batches, batch)
		if count < len(data) {
			break
		}
		endTime = batch[count-1].PrintTime
	}

	trapq := make([]TrapQMove, 0)
	for batchIndex := len(batches) - 1; batchIndex >= 0; batchIndex-- {
		batch := batches[batchIndex]
		for entryIndex := len(batch) - 1; entryIndex >= 0; entryIndex-- {
			trapq = append(trapq, batch[entryIndex])
		}
	}
	return trapq
}

func (self *TrapQDump) LogMessage(data []TrapQMove) string {
	if len(data) == 0 {
		return ""
	}
	out := []string{fmt.Sprintf("Dumping trapq '%s' %d moves:", self.name, len(data))}
	for i, move := range data {
		out = append(out, fmt.Sprintf("move %d: pt=%.6f mt=%.6f sv=%.6f a=%.6f sp=(%.6f,%.6f,%.6f) ar=(%.6f,%.6f,%.6f)",
			i,
			move.PrintTime,
			move.MoveTime,
			move.StartVelocity,
			move.Acceleration,
			move.StartPosition[0], move.StartPosition[1], move.StartPosition[2],
			move.Direction[0], move.Direction[1], move.Direction[2],
		))
	}
	return strings.Join(out, "\n")
}

func (self *TrapQDump) PositionAt(printTime float64) ([]float64, float64) {
	data, count := self.source.ExtractMoves(1, 0.0, printTime)
	if count == 0 || len(data) == 0 {
		return nil, -1
	}
	move := data[0]
	moveTime := math.Max(0.0, math.Min(move.MoveTime, printTime-move.PrintTime))
	dist := (move.StartVelocity + .5*move.Acceleration*moveTime) * moveTime
	pos := []float64{
		move.StartPosition[0] + move.Direction[0]*dist,
		move.StartPosition[1] + move.Direction[1]*dist,
		move.StartPosition[2] + move.Direction[2]*dist,
	}
	velocity := move.StartVelocity + move.Acceleration*moveTime
	return pos, velocity
}

func (self *TrapQDump) APIUpdate() map[string]interface{} {
	qtime := self.lastAPIMessage[0].(float64) + math.Min(self.lastAPIMessage[1].(float64), 0.100)
	data := self.Moves(qtime, NeverTime)
	formatted := make([][]interface{}, 0, len(data))
	for _, move := range data {
		formatted = append(formatted, []interface{}{
			move.PrintTime,
			move.MoveTime,
			move.StartVelocity,
			move.Acceleration,
			[]float64{move.StartPosition[0], move.StartPosition[1], move.StartPosition[2]},
			[]float64{move.Direction[0], move.Direction[1], move.Direction[2]},
		})
	}
	if len(formatted) > 0 && reflect.DeepEqual(formatted[0], self.lastAPIMessage) {
		formatted = formatted[1:]
	}
	if len(formatted) == 0 {
		return map[string]interface{}{}
	}
	self.lastAPIMessage = formatted[len(formatted)-1]
	return map[string]interface{}{"data": formatted}
}
