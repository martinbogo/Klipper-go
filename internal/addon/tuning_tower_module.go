package addon

import (
	"fmt"
	printerpkg "goklipper/internal/pkg/printer"
)

const cmdTuningTowerHelp = "Tool to adjust a parameter at each Z height"

type TuningTowerModule struct {
	printer         printerpkg.ModulePrinter
	gcode           printerpkg.GCodeRuntime
	gcodeMove       printerpkg.MoveTransformController
	normalTransform printerpkg.MoveTransform
	core            *TuningTower
}

func LoadTuningTower(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	self := &TuningTowerModule{
		printer:         printer,
		gcode:           printer.GCode(),
		gcodeMove:       printer.GCodeMove(),
		normalTransform: nil,
		core:            NewTuningTower(),
	}
	self.gcode.RegisterCommand("TUNING_TOWER", self.cmdTuningTower, false, cmdTuningTowerHelp)
	if err := printer.AddObject("tuning_tower", self); err != nil {
		panic(err)
	}
	return self
}

func (self *TuningTowerModule) cmdTuningTower(gcmd printerpkg.Command) error {
	if self.normalTransform != nil {
		self.End_test()
	}

	command := gcmd.String("COMMAND", "")
	if command == "" {
		return fmt.Errorf("missing COMMAND parameter")
	}
	parameter := gcmd.String("PARAMETER", "")
	if parameter == "" {
		return fmt.Errorf("missing PARAMETER parameter")
	}
	start := gcmd.Float("START", 0.)
	factor := gcmd.Float("FACTOR", 0.)
	maxval := 0.0
	band := gcmd.Float("BAND", 0.)
	stepDelta := gcmd.Float("STEP_DELTA", 0.)
	stepHeight := gcmd.Float("STEP_HEIGHT", 0.)
	skip := gcmd.Float("SKIP", 0.)
	_ = maxval

	commandFmt := ""
	if self.gcode.IsTraditionalGCode(command) {
		commandFmt = fmt.Sprintf("%s %s%%.9f", command, parameter)
	} else {
		commandFmt = fmt.Sprintf("%s %s=%%.9f", command, parameter)
	}

	nt := self.gcodeMove.SetMoveTransform(self, true)
	self.normalTransform = nt
	message, err := self.core.BeginTest(nt, TuningConfig{
		CommandFmt: commandFmt,
		Start:      start,
		Factor:     factor,
		Band:       band,
		StepHeight: stepHeight,
		StepDelta:  stepDelta,
		Skip:       skip,
	})
	if err != nil {
		self.gcodeMove.SetMoveTransform(nt, true)
		self.normalTransform = nil
		return err
	}
	gcmd.RespondInfo(message, true)
	return nil
}

func (self *TuningTowerModule) GetPosition() []float64 {
	return self.core.GetPosition()
}

func (self *TuningTowerModule) Get_position() []float64 {
	return self.GetPosition()
}

func (self *TuningTowerModule) Calc_value(z float64) float64 {
	return self.core.CalcValue(z)
}

func (self *TuningTowerModule) Move(newpos []float64, speed float64) {
	command, endTest := self.core.Move(newpos, speed, self.gcodeMove.GCodePositionZ())
	if endTest {
		self.End_test()
	}
	if command != "" {
		self.gcode.RunScriptFromCommand(command)
	}
}

func (self *TuningTowerModule) End_test() {
	self.gcode.RespondInfo("Ending tuning test mode", true)
	if self.normalTransform != nil {
		self.gcodeMove.SetMoveTransform(self.normalTransform, true)
	}
	self.normalTransform = nil
	self.core.EndTest()
}

func (self *TuningTowerModule) Is_active() bool {
	return self.core.IsActive()
}