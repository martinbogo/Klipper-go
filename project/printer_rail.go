package project

import (
	"container/list"
	"fmt"
	"goklipper/common/utils/object"
	"goklipper/common/value"
	mcupkg "goklipper/internal/pkg/mcu"
	kinematicspkg "goklipper/internal/pkg/motion/kinematics"
	probepkg "goklipper/internal/pkg/motion/probe"
	printerpkg "goklipper/internal/pkg/printer"
	"strconv"
)

/////////////////////////////////////////////////////////////////////
// Stepper controlled rails
/////////////////////////////////////////////////////////////////////

// A motor control "rail" with one (or more) steppers and one (or more)
// endstops.
type PrinterRail struct {
	stepper_units_in_radians bool
	steppers                 []*MCU_stepper
	endstops                 []list.List
	endstop_map              map[string]interface{}
	Get_name                 func(bool) string
	Get_commanded_position   func() float64
	calc_position_from_coord func(coord []float64) float64
	Position_endstop         float64
	position_min             float64
	position_max             float64
	homing_speed             float64
	second_homing_speed      float64
	homing_retract_speed     float64
	homing_retract_dist      float64
	homing_positive_dir      bool
}

func NewPrinterRail(config *ConfigWrapper, need_position_minmax bool,
	default_position_endstop interface{}, units_in_radians bool) *PrinterRail {
	self := PrinterRail{}
	self.stepper_units_in_radians = units_in_radians
	self.steppers = []*MCU_stepper{}
	self.endstops = []list.List{}
	self.endstop_map = map[string]interface{}{}
	self.Add_extra_stepper(config)
	mcu_stepper := self.steppers[0]
	self.Get_name = mcu_stepper.Get_name
	self.Get_commanded_position = mcu_stepper.Get_commanded_position
	self.calc_position_from_coord = mcu_stepper.Calc_position_from_coord
	mcu_endstop := self.endstops[0].Front().Value
	settings := kinematicspkg.BuildLegacyRailSettings(config, mcu_endstop, need_position_minmax, default_position_endstop)
	self.Position_endstop = settings.PositionEndstop
	self.position_min = settings.PositionMin
	self.position_max = settings.PositionMax
	self.homing_speed = settings.HomingSpeed
	self.second_homing_speed = settings.SecondHomingSpeed
	self.homing_retract_speed = settings.HomingRetractSpeed
	self.homing_retract_dist = settings.HomingRetractDist
	self.homing_positive_dir = settings.HomingPositiveDir
	return &self
}

func (self *PrinterRail) Get_range() (float64, float64) {
	return self.position_min, self.position_max
}

type HomingInfo struct {
	Speed               float64
	Position_endstop    float64
	Retract_speed       float64
	Retract_dist        float64
	Positive_dir        bool
	Second_homing_speed float64
}

func (self *PrinterRail) Get_homing_info() *HomingInfo {
	h := &HomingInfo{
		Speed:               self.homing_speed,
		Position_endstop:    self.Position_endstop,
		Retract_speed:       self.homing_retract_speed,
		Retract_dist:        self.homing_retract_dist,
		Positive_dir:        self.homing_positive_dir,
		Second_homing_speed: self.second_homing_speed,
	}
	return h
}

func (self *PrinterRail) Get_steppers() []*MCU_stepper {
	steppersBack := make([]*MCU_stepper, len(self.steppers))
	copy(steppersBack, self.steppers)
	return steppersBack
}
func (self *PrinterRail) Get_endstops() []list.List {
	endstopsBack := make([]list.List, len(self.endstops))
	copy(endstopsBack, self.endstops)
	return endstopsBack
}

func railEndstopEntriesFromProject(endstopMap map[string]interface{}) map[string]mcupkg.RailEndstopEntry {
	entries := make(map[string]mcupkg.RailEndstopEntry, len(endstopMap))
	for pinName, rawEntry := range endstopMap {
		entry := rawEntry.(map[string]interface{})
		entries[pinName] = mcupkg.RailEndstopEntry{
			Endstop: entry["endstop"],
			Invert:  entry["invert"],
			Pullup:  entry["pullup"],
		}
	}
	return entries
}

func addStepperToRailEndstop(endstop interface{}, stepper *MCU_stepper) {
	if mcuEndstop, ok := endstop.(*MCU_endstop); ok {
		mcuEndstop.Add_stepper(stepper)
	}
	if probeEndstop, ok := endstop.(*ProbeEndstopWrapper); ok {
		probeEndstop.Add_stepper(stepper)
	}
}

func (self *PrinterRail) Add_extra_stepper(config *ConfigWrapper) {
	stepper := PrinterStepper(config, self.stepper_units_in_radians)
	self.steppers = append(self.steppers, stepper)
	if len(self.endstops) > 0 && config.Get("endstop_pin", value.None, false) == nil {
		self.endstops[0].Front().Value.(*MCU_endstop).Add_stepper(stepper)
		return
	}
	endstopPin := config.Get("endstop_pin", object.Sentinel{}, false)
	printer := config.Get_printer()
	pins := printer.Lookup_object("pins", object.Sentinel{})
	pinParams := pins.(*printerpkg.PrinterPins).Parse_pin(endstopPin.(string), true, true)
	name := stepper.Get_name(true)
	queryEndstops := printer.Load_object(config, "query_endstops", object.Sentinel{}).(*probepkg.QueryEndstopsModule)
	result, err := mcupkg.ResolveLegacyRailEndstop(
		railEndstopEntriesFromProject(self.endstop_map),
		pinParams["chip_name"].(string),
		pinParams["pin"],
		pinParams["invert"],
		pinParams["pullup"],
		name,
		func() interface{} {
			return pins.(*printerpkg.PrinterPins).Setup_pin("endstop", endstopPin.(string))
		},
		queryEndstops.Register_endstop,
	)
	if err != nil {
		panic(fmt.Errorf("pinter rail %s %w", self.Get_name(false), err))
	}
	if result.Created {
		self.endstop_map[result.PinName] = map[string]interface{}{
			"endstop": result.Entry.Endstop,
			"invert":  result.Entry.Invert,
			"pullup":  result.Entry.Pullup,
		}
		var list list.List
		list.PushBack(result.Endstop)
		list.PushBack(name)
		self.endstops = append(self.endstops, list)
	}
	addStepperToRailEndstop(result.Endstop, stepper)
}

func (self *PrinterRail) Setup_itersolve(alloc_func string, params ...interface{}) {
	for _, stepper := range self.steppers {
		stepper.Setup_itersolve(alloc_func, params)
	}
}

func (self *PrinterRail) Generate_steps(flush_time float64) {
	for _, stepper := range self.steppers {
		stepper.Generate_steps(flush_time)
	}
}

func (self *PrinterRail) Set_trapq(trapq interface{}) {
	for _, stepper := range self.steppers {
		stepper.Set_trapq(trapq)
	}
}

func (self *PrinterRail) Set_position(coord []float64) {
	for _, stepper := range self.steppers {
		stepper.Set_position(coord)
	}
}

func LookupMultiRail(config *ConfigWrapper, need_position_minmax bool, default_position_endstop interface{}, units_in_radians bool) *PrinterRail {
	dpe, ok := default_position_endstop.(*float64)
	if !ok {
		dpe = nil
	}
	rail := NewPrinterRail(config, need_position_minmax, dpe, units_in_radians)
	for i := 1; i < 99; i++ {
		if !config.Has_section(config.Get_name() + strconv.Itoa(i)) {
			break
		}
		rail.Add_extra_stepper(config.Getsection(config.Get_name() + strconv.Itoa(i)))
	}
	return rail
}
