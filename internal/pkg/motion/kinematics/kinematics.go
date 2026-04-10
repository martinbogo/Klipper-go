package kinematics

type Stepper interface {
	Set_trapq(tq interface{}) interface{}
	Generate_steps(flush_time float64)
	Get_name(short bool) string
}

type RailEndstop interface {
	Add_stepper(stepper Stepper)
}

type RailHomingInfo struct {
	Speed             float64
	PositionEndstop   float64
	RetractSpeed      float64
	RetractDist       float64
	PositiveDir       bool
	SecondHomingSpeed float64
}

type Rail interface {
	Setup_itersolve(alloc_func string, params ...interface{})
	Get_steppers() []Stepper
	Primary_endstop() RailEndstop
	Get_range() (float64, float64)
	Set_position(newpos []float64)
	Get_homing_info() *RailHomingInfo
	Set_trapq(tq interface{})
	Get_commanded_position() float64
	Get_name(short bool) string
}

type Toolhead interface {
	Get_trapq() interface{}
	Register_step_generator(handler func(float64))
	Get_max_velocity() (float64, float64)
	Flush_step_generation()
	Get_position() []float64
	Set_position(newpos []float64, homingAxes []int)
}

type Printer interface {
	Register_event_handler(event string, callback func([]interface{}) error)
}

type Move interface {
	EndPos() []float64
	AxesD() []float64
	MoveD() float64
	LimitSpeed(speed float64, accel float64)
	MoveError(msg string) error
}

type HomingState interface {
	GetAxes() []int
	HomeRails(rails []Rail, forcepos []interface{}, homepos []interface{})
}

type Kinematics interface {
	GetSteppers() []Stepper
	CalcPosition(stepperPositions map[string]float64) []float64
	SetPosition(newpos []float64, homingAxes []int)
	NoteZNotHomed()
	Home(homingState HomingState)
	CheckMove(move Move)
	Status(eventtime float64) map[string]interface{}
}

type DualCarriageConfig struct {
	Axis     int
	AxisName string
	Rails    []Rail
}

type CartesianConfig struct {
	Printer      Printer
	Toolhead     Toolhead
	Rails        []Rail
	MaxZVelocity float64
	MaxZAccel    float64
	DualCarriage *DualCarriageConfig
}

type CoreXYConfig struct {
	Printer      Printer
	Toolhead     Toolhead
	Rails        []Rail
	MaxZVelocity float64
	MaxZAccel    float64
}

type NoneConfig struct {
	AxesMinMax []string
}
