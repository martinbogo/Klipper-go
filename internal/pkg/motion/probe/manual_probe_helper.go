package probe

import (
	"fmt"
	"math"
	"strconv"
)

const (
	ZBobMinimum = 0.500
	BisectMax   = 0.200
)

type ManualProbeCommand interface {
	Parameters() map[string]string
	RespondInfo(msg string, log bool)
}

type ManualProbeGCode interface {
	RegisterCommand(cmd string, handler func(ManualProbeCommand) error, desc string)
	ClearCommand(cmd string)
	RespondInfo(msg string, log bool)
}

type ManualProbeRuntime interface {
	ResetStatus()
	SetStatus(status map[string]interface{})
	ToolheadPosition() []float64
	KinematicsPosition() []float64
	ManualMove(coord []interface{}, speed float64)
}

type ManualProbeSession struct {
	finalizeCallback func([]float64)
	gcode            ManualProbeGCode
	runtime          ManualProbeRuntime
	speed            float64
	pastPositions    []float64
	startPosition    []float64
}

func BisectLeft(nums []float64, target float64) int {
	low := 0
	high := len(nums) - 1
	for low <= high {
		mid := low + (high-low)/2
		value := nums[mid]
		if math.Abs(value-target) < 0.001 {
			return mid
		}
		if value < target {
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return low
}

func NewManualProbeSession(gcode ManualProbeGCode, runtime ManualProbeRuntime, speed float64, finalizeCallback func([]float64)) *ManualProbeSession {
	self := &ManualProbeSession{
		finalizeCallback: finalizeCallback,
		gcode:            gcode,
		runtime:          runtime,
		speed:            speed,
		pastPositions:    []float64{},
		startPosition:    runtime.ToolheadPosition(),
	}
	gcode.RegisterCommand("ACCEPT", self.cmdAccept, self.cmdAcceptHelp())
	gcode.RegisterCommand("NEXT", self.cmdAccept, "")
	gcode.RegisterCommand("ABORT", self.cmdAbort, self.cmdAbortHelp())
	gcode.RegisterCommand("TESTZ", self.cmdTestZ, self.cmdTestZHelp())
	gcode.RespondInfo("Starting manual Z probe. Use TESTZ to adjust position.\nFinish with ACCEPT or ABORT command.", true)
	self.ReportZStatus(false, 0)
	return self
}

func (self *ManualProbeSession) MoveZ(zPos float64) {
	currentPos := self.runtime.ToolheadPosition()
	func() {
		defer func() {
			if recover() != nil {
				self.Finalize(false)
			}
		}()
		zBobPos := zPos + ZBobMinimum
		if currentPos[2] < zBobPos {
			self.runtime.ManualMove([]interface{}{nil, nil, zBobPos}, self.speed)
		}
		self.runtime.ManualMove([]interface{}{nil, nil, zPos}, self.speed)
	}()
}

func (self *ManualProbeSession) ReportZStatus(warnNoChange bool, prevPos float64) {
	kinPos := self.runtime.KinematicsPosition()
	zPos := kinPos[2]
	if warnNoChange && zPos == prevPos {
		self.gcode.RespondInfo("WARNING: No change in position (reached stepper resolution)", true)
		pp := self.pastPositions
		nextPos := BisectLeft(pp, zPos)
		prevIdx := nextPos - 1
		if nextPos < len(pp) && pp[nextPos] == zPos {
			nextPos++
		}
		prevPosVal := 0.0
		nextPosVal := 0.0
		prevStr := "??????"
		nextStr := "??????"
		if prevIdx >= 0 {
			prevPosVal = pp[prevIdx]
			prevStr = fmt.Sprintf("%.3f", prevPosVal)
		}
		if nextPos < len(pp) {
			nextPosVal = pp[nextPos]
			nextStr = fmt.Sprintf("%.3f", nextPosVal)
		}
		self.runtime.SetStatus(map[string]interface{}{
			"is_active":        true,
			"z_position":       zPos,
			"z_position_lower": prevPosVal,
			"z_position_upper": nextPosVal,
			"isActive":         true,
			"zPosition":        zPos,
			"zPositionLower":   prevPosVal,
			"zPositionUpper":   nextPosVal,
		})
		self.gcode.RespondInfo(fmt.Sprintf("Z position: %s --> %.3f <-- %s", prevStr, zPos, nextStr), true)
	}
}

func (self *ManualProbeSession) cmdAcceptHelp() string {
	return "Accept the current Z position"
}

func (self *ManualProbeSession) cmdAccept(command ManualProbeCommand) error {
	position := self.runtime.ToolheadPosition()
	startPos := self.startPosition
	if len(position) < 3 || len(startPos) < 3 || position[0] != startPos[0] || position[1] != startPos[1] || position[2] >= startPos[2] {
		command.RespondInfo("Manual probe failed! Use TESTZ commands to position the\nnozzle prior to running ACCEPT.", true)
		self.Finalize(false)
		return nil
	}
	self.Finalize(true)
	return nil
}

func (self *ManualProbeSession) cmdAbortHelp() string {
	return "Abort manual Z probing tool"
}

func (self *ManualProbeSession) cmdAbort(command ManualProbeCommand) error {
	_ = command
	self.Finalize(false)
	return nil
}

func (self *ManualProbeSession) cmdTestZHelp() string {
	return "Move to new Z height"
}

func (self *ManualProbeSession) cmdTestZ(command ManualProbeCommand) error {
	kinPos := self.runtime.KinematicsPosition()
	zPos := kinPos[2]
	insertPos := BisectLeft(self.pastPositions, zPos)
	if insertPos >= len(self.pastPositions) || self.pastPositions[insertPos] != zPos {
		self.pastPositions = append(self.pastPositions, 0)
		copy(self.pastPositions[insertPos+1:], self.pastPositions[insertPos:])
		self.pastPositions[insertPos] = zPos
	}
	params := command.Parameters()
	request, ok := params["Z"]
	if !ok {
		panic("Error on manual probe TESTZ: missing Z")
	}
	nextZPos := 0.0
	if request == "+" || request == "++" {
		checkZ := 9999999999999.9
		if insertPos < len(self.pastPositions)-1 {
			checkZ = self.pastPositions[insertPos+1]
		}
		if request == "+" {
			checkZ = (checkZ + zPos) / 2.
		}
		nextZPos = math.Min(checkZ, zPos+BisectMax)
	} else if request == "-" || request == "--" {
		checkZ := -9999999999999.9
		if insertPos > 0 {
			checkZ = self.pastPositions[insertPos-1]
		}
		if request == "-" {
			checkZ = (checkZ + zPos) / 2.
		}
		nextZPos = math.Max(checkZ, zPos-BisectMax)
	} else {
		delta, err := strconv.ParseFloat(request, 64)
		if err != nil {
			panic(err)
		}
		nextZPos = zPos + delta
	}
	self.MoveZ(nextZPos)
	self.ReportZStatus(nextZPos != zPos, zPos)
	return nil
}

func (self *ManualProbeSession) Finalize(success bool) {
	self.runtime.ResetStatus()
	self.gcode.ClearCommand("ACCEPT")
	self.gcode.ClearCommand("NEXT")
	self.gcode.ClearCommand("ABORT")
	self.gcode.ClearCommand("TESTZ")
	kinPos := []float64{}
	if success {
		kinPos = self.runtime.KinematicsPosition()
	}
	self.finalizeCallback(kinPos)
}
