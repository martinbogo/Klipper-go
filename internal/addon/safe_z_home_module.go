package addon

import (
	"fmt"
	"strconv"
	"strings"

	printerpkg "goklipper/internal/pkg/printer"
)

type safeZConfigSectionChecker interface {
	HasSection(section string) bool
}

type safeZGCode interface {
	printerpkg.GCodeRuntime
	CreateCommand(cmd string, raw string, params map[string]string) printerpkg.Command
}

type safeZToolhead interface {
	GetPosition() []float64
	SetPosition(newpos []float64, homingAxes []int)
	ManualMove(coord []interface{}, speed float64)
	HomedAxes(eventtime float64) string
	NoteZNotHomed()
}

type SafeZHomingModule struct {
	printer        printerpkg.ModulePrinter
	reactor        printerpkg.ModuleReactor
	gcode          safeZGCode
	homeXPos       float64
	homeYPos       float64
	zHop           float64
	zHopSpeed      float64
	speed          float64
	moveToPrevious bool
	prevG28        func(printerpkg.Command) error
}

const (
	homeAllAlias = "HOME_ALL"
	homeXYAlias  = "HOME_XY"
	homeZAlias   = "HOME_Z"
)

func LoadConfigSafeZHoming(config printerpkg.ModuleConfig) interface{} {
	homeXPos, homeYPos := parseRequiredFloatPair(config.String("home_xy_position", "", true), "home_xy_position", config.Name())
	gcodeObj := config.Printer().GCode()
	gcode, ok := gcodeObj.(safeZGCode)
	if !ok {
		panic(fmt.Sprintf("gcode runtime does not implement safeZGCode: %T", gcodeObj))
	}
	self := &SafeZHomingModule{
		printer:        config.Printer(),
		reactor:        config.Printer().Reactor(),
		gcode:          gcode,
		homeXPos:       homeXPos,
		homeYPos:       homeYPos,
		zHop:           config.Float("z_hop", 0.0),
		zHopSpeed:      config.Float("z_hop_speed", 15.0),
		speed:          config.Float("speed", 50.0),
		moveToPrevious: config.Bool("move_to_previous", false),
	}
	config.LoadObject("homing")
	self.prevG28 = self.gcode.ReplaceCommand("G28", self.cmdG28, false, "")
	self.gcode.RegisterCommand("H28", self.cmdH28, false, "")
	self.gcode.RegisterCommand(homeAllAlias, self.cmdHomeAll, false, "Home all axes")
	self.gcode.RegisterCommand(homeXYAlias, self.cmdHomeXY, false, "Home X and Y axes")
	self.gcode.RegisterCommand(homeZAlias, self.cmdHomeZ, false, "Home Z axis")
	if hasSafeZConfigSection(config, "homing_override") {
		panic("homing_override and safe_z_homing cannot be used simultaneously")
	}
	return self
}

func hasSafeZConfigSection(config printerpkg.ModuleConfig, section string) bool {
	typed, ok := config.(safeZConfigSectionChecker)
	if !ok {
		return false
	}
	return typed.HasSection(section)
}

func parseRequiredFloatPair(raw string, option string, section string) (float64, float64) {
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		panic(fmt.Sprintf("Option '%s' in section '%s' must contain two comma-separated floats", option, section))
	}
	values := make([]float64, 0, 2)
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			panic(fmt.Sprintf("invalid float in option '%s' in section '%s': %v", option, section, err))
		}
		values = append(values, value)
	}
	return values[0], values[1]
}

func axisRequested(gcmd printerpkg.Command, axis string) bool {
	return gcmd.String(axis, "") != ""
}

func axisHomed(homedAxes string, axis string) bool {
	return strings.Contains(homedAxes, strings.ToLower(axis))
}

func (self *SafeZHomingModule) lookupToolhead() safeZToolhead {
	toolheadObj := self.printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(safeZToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement safeZToolhead: %T", toolheadObj))
	}
	return toolhead
}

func (self *SafeZHomingModule) invokePrevG28(params map[string]string) error {
	if self.prevG28 == nil {
		return nil
	}
	return self.prevG28(self.gcode.CreateCommand("G28", "G28", params))
}

func requestedXYHomeParams(homeX bool, homeY bool, homedAxes string) map[string]string {
	params := map[string]string{}
	if homeX {
		params["X"] = "0"
	}
	if homeY || (homeX && !axisHomed(homedAxes, "y")) {
		params["Y"] = "0"
	}
	return params
}

func (self *SafeZHomingModule) invokeG28Alias(raw string, params map[string]string) error {
	return self.cmdG28(self.gcode.CreateCommand("G28", raw, params))
}

func (self *SafeZHomingModule) performZHopIfNeeded(toolhead safeZToolhead) {
	if self.zHop == 0.0 {
		return
	}
	curtime := self.reactor.Monotonic()
	homedAxes := toolhead.HomedAxes(curtime)
	pos := toolhead.GetPosition()
	if !axisHomed(homedAxes, "z") {
		pos[2] = 0
		toolhead.SetPosition(pos, []int{2})
		toolhead.ManualMove([]interface{}{nil, nil, self.zHop}, self.zHopSpeed)
		toolhead.NoteZNotHomed()
		return
	}
	if pos[2] < self.zHop {
		toolhead.ManualMove([]interface{}{nil, nil, self.zHop}, self.zHopSpeed)
	}
}

func (self *SafeZHomingModule) cmdG28(gcmd printerpkg.Command) error {
	return self.cmdG28WithZHop(gcmd, true)
}

func (self *SafeZHomingModule) cmdG28WithZHop(gcmd printerpkg.Command, withZHop bool) error {
	toolhead := self.lookupToolhead()
	if withZHop {
		self.performZHopIfNeeded(toolhead)
	}

	needX := axisRequested(gcmd, "X")
	needY := axisRequested(gcmd, "Y")
	needZ := axisRequested(gcmd, "Z")
	if !needX && !needY && !needZ {
		needX, needY, needZ = true, true, true
	}

	homedAxes := toolhead.HomedAxes(self.reactor.Monotonic())
	xyParams := requestedXYHomeParams(needX, needY, homedAxes)
	if len(xyParams) > 0 {
		if err := self.invokePrevG28(xyParams); err != nil {
			return err
		}
	}
	if !needZ {
		return nil
	}

	homedAxes = toolhead.HomedAxes(self.reactor.Monotonic())
	if !axisHomed(homedAxes, "x") || !axisHomed(homedAxes, "y") {
		panic("Must home X and Y axes first")
	}
	prevpos := toolhead.GetPosition()
	toolhead.ManualMove([]interface{}{self.homeXPos, self.homeYPos}, self.speed)
	if err := self.invokePrevG28(map[string]string{"Z": "0"}); err != nil {
		return err
	}
	if self.zHop != 0.0 {
		toolhead.ManualMove([]interface{}{nil, nil, self.zHop}, self.zHopSpeed)
	}
	if self.moveToPrevious {
		toolhead.ManualMove([]interface{}{prevpos[0], prevpos[1]}, self.speed)
	}
	return nil
}

func (self *SafeZHomingModule) cmdHomeAll(printerpkg.Command) error {
	return self.invokeG28Alias("G28", nil)
}

func (self *SafeZHomingModule) cmdHomeXY(printerpkg.Command) error {
	return self.cmdG28WithZHop(self.gcode.CreateCommand("G28", "G28 X Y", map[string]string{"X": "0", "Y": "0"}), false)
}

func (self *SafeZHomingModule) cmdHomeZ(printerpkg.Command) error {
	return self.invokeG28Alias("G28 Z", map[string]string{"Z": "0"})
}

func (self *SafeZHomingModule) cmdH28(gcmd printerpkg.Command) error {
	toolhead := self.lookupToolhead()
	self.performZHopIfNeeded(toolhead)

	needX := axisRequested(gcmd, "X")
	needY := axisRequested(gcmd, "Y")
	needZ := axisRequested(gcmd, "Z")
	if !needX && !needY && !needZ {
		needX, needY, needZ = true, true, true
	}

	homedAxes := toolhead.HomedAxes(self.reactor.Monotonic())
	xyParams := requestedXYHomeParams(
		needX && !axisHomed(homedAxes, "x"),
		needY && !axisHomed(homedAxes, "y"),
		homedAxes,
	)
	if len(xyParams) > 0 {
		if err := self.invokePrevG28(xyParams); err != nil {
			return err
		}
	}
	if !needZ {
		return nil
	}

	homedAxes = toolhead.HomedAxes(self.reactor.Monotonic())
	if axisHomed(homedAxes, "z") {
		return nil
	}
	if !axisHomed(homedAxes, "x") || !axisHomed(homedAxes, "y") {
		panic("Must home X and Y axes first")
	}
	prevpos := toolhead.GetPosition()
	toolhead.ManualMove([]interface{}{self.homeXPos, self.homeYPos}, self.speed)
	if err := self.invokePrevG28(map[string]string{"Z": "0"}); err != nil {
		return err
	}
	if self.zHop != 0.0 {
		toolhead.ManualMove([]interface{}{nil, nil, self.zHop}, self.zHopSpeed)
	}
	if self.moveToPrevious {
		toolhead.ManualMove([]interface{}{prevpos[0], prevpos[1]}, self.speed)
	}
	return nil
}
