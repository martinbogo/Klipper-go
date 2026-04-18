package motion

type Extruder interface {
	Update_move_time(flush_time float64, clear_history_time float64)
	Check_move(move *Move) error
	Find_past_position(print_time float64) float64
	Calc_junction(prev_move, move *Move) float64
	Move(print_time float64, move *Move)
	Get_name() string
	Get_heater() interface{}
	Get_trapq() interface{}
}

type LegacyExtruderRuntime struct {
	Name              string
	Last_position     float64
	Heater            interface{}
	Nozzle_diameter   float64
	Filament_area     float64
	Max_extrude_ratio float64
	Max_e_velocity    float64
	Max_e_accel       float64
	Max_e_dist        float64
	Instant_corner_v  float64
	Trapq             interface{}
	Trapq_append      func(tq interface{}, print_time,
		accel_t, cruise_t, decel_t,
		start_pos_x, start_pos_y, start_pos_z,
		axes_r_x, axes_r_y, axes_r_z,
		start_v, cruise_v, accel float64)
	Trapq_finalize_moves       func(interface{}, float64, float64)
	Can_extrude                func() bool
	Heater_status              func(float64) map[string]float64
	Heater_stats               func(float64) (bool, string)
	Stepper_status             func(float64) map[string]float64
	Find_stepper_past_position func(float64) float64
}

func (self *LegacyExtruderRuntime) Update_move_time(flush_time float64, clear_history_time float64) {
	if self.Trapq_finalize_moves == nil {
		return
	}
	self.Trapq_finalize_moves(self.Trapq, flush_time, clear_history_time)
}

func (self *LegacyExtruderRuntime) Get_status(eventtime float64) map[string]interface{} {
	status := make(map[string]interface{})
	if self.Heater_status != nil {
		for k, v := range self.Heater_status(eventtime) {
			status[k] = v
		}
	}
	status["can_extrude"] = self.canExtrude()
	status["Nozzle_diameter"] = self.Nozzle_diameter
	if self.Stepper_status != nil {
		for k, v := range self.Stepper_status(eventtime) {
			status[k] = v
		}
	}
	return status
}

func (self *LegacyExtruderRuntime) Get_name() string {
	return self.Name
}

func (self *LegacyExtruderRuntime) Get_heater() interface{} {
	return self.Heater
}

func (self *LegacyExtruderRuntime) Get_trapq() interface{} {
	return self.Trapq
}

func (self *LegacyExtruderRuntime) LegacyLastPosition() float64 {
	return self.Last_position
}

func (self *LegacyExtruderRuntime) Stats(eventtime float64) (bool, string) {
	if self.Heater_stats == nil {
		return false, ""
	}
	return self.Heater_stats(eventtime)
}

func (self *LegacyExtruderRuntime) Check_move(move *Move) error {
	return CheckExtrusionMove(move, ExtrusionLimits{
		CanExtrude:      self.canExtrude(),
		NozzleDiameter:  self.Nozzle_diameter,
		FilamentArea:    self.Filament_area,
		MaxExtrudeRatio: self.Max_extrude_ratio,
		MaxEVelocity:    self.Max_e_velocity,
		MaxEAccel:       self.Max_e_accel,
		MaxEDistance:    self.Max_e_dist,
		InstantCornerV:  self.Instant_corner_v,
	})
}

func (self *LegacyExtruderRuntime) Calc_junction(prev_move, move *Move) float64 {
	return CalcExtrusionJunction(prev_move, move, self.Instant_corner_v)
}

func (self *LegacyExtruderRuntime) Move(print_time float64, move *Move) {
	planned := BuildExtrusionMove(move)
	if self.Trapq_append != nil {
		self.Trapq_append(self.Trapq, print_time,
			planned.AccelT, planned.CruiseT, planned.DecelT,
			planned.StartPosition, 0., 0.,
			1., planned.CanPressureAdvance, 0.,
			planned.StartV, planned.CruiseV, planned.Accel)
	}
	self.Last_position = move.End_pos[3]
}

func (self *LegacyExtruderRuntime) Find_past_position(print_time float64) float64 {
	if self.Find_stepper_past_position == nil {
		return 0.
	}
	return self.Find_stepper_past_position(print_time)
}

func (self *LegacyExtruderRuntime) canExtrude() bool {
	if self.Can_extrude == nil {
		return false
	}
	return self.Can_extrude()
}

type DummyExtruder struct{}

func NewDummyExtruder() *DummyExtruder {
	return &DummyExtruder{}
}

func (self *DummyExtruder) Update_move_time(flush_time float64, clear_history_time float64) {}

func (self *DummyExtruder) Check_move(move *Move) error {
	return NoExtruderMoveError(move)
}

func (self *DummyExtruder) Find_past_position(print_time float64) float64 {
	return 0.
}

func (self *DummyExtruder) Calc_junction(prev_move, move *Move) float64 {
	return DefaultExtrusionJunction(move)
}

func (self *DummyExtruder) Move(print_time float64, move *Move) {}

func (self *DummyExtruder) Get_name() string {
	return ""
}

func (self *DummyExtruder) Get_heater() interface{} {
	panic("Extruder not configured")
}

func (self *DummyExtruder) Get_trapq() interface{} {
	panic("Extruder not configured")
}
