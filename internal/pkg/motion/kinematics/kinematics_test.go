package kinematics

import (
	"errors"
	"strings"
	"testing"
)

type fakeStepper struct {
	name          string
	currentTrapq  interface{}
	trapqHistory  []interface{}
	generateCalls []float64
}

func (self *fakeStepper) Set_trapq(tq interface{}) interface{} {
	prev := self.currentTrapq
	self.currentTrapq = tq
	self.trapqHistory = append(self.trapqHistory, tq)
	return prev
}

func (self *fakeStepper) Generate_steps(flush_time float64) {
	self.generateCalls = append(self.generateCalls, flush_time)
}

func (self *fakeStepper) Get_name(short bool) string {
	_ = short
	return self.name
}

type fakeEndstop struct {
	added []Stepper
}

func (self *fakeEndstop) Add_stepper(stepper Stepper) {
	self.added = append(self.added, stepper)
}

type setupCall struct {
	alloc string
	params []interface{}
}

type fakeRail struct {
	name              string
	steppers          []Stepper
	endstop           RailEndstop
	rangeMin          float64
	rangeMax          float64
	homingInfo        *RailHomingInfo
	setupCalls        []setupCall
	setPositions      [][]float64
	trapqHistory      []interface{}
	commandedPosition float64
}

func (self *fakeRail) Setup_itersolve(alloc_func string, params ...interface{}) {
	copiedParams := append([]interface{}{}, params...)
	self.setupCalls = append(self.setupCalls, setupCall{alloc: alloc_func, params: copiedParams})
}

func (self *fakeRail) Get_steppers() []Stepper {
	return append([]Stepper{}, self.steppers...)
}

func (self *fakeRail) Primary_endstop() RailEndstop {
	return self.endstop
}

func (self *fakeRail) Get_range() (float64, float64) {
	return self.rangeMin, self.rangeMax
}

func (self *fakeRail) Set_position(newpos []float64) {
	self.setPositions = append(self.setPositions, append([]float64{}, newpos...))
}

func (self *fakeRail) Get_homing_info() *RailHomingInfo {
	if self.homingInfo == nil {
		return &RailHomingInfo{}
	}
	copied := *self.homingInfo
	return &copied
}

func (self *fakeRail) Set_trapq(tq interface{}) {
	self.trapqHistory = append(self.trapqHistory, tq)
}

func (self *fakeRail) Get_commanded_position() float64 {
	return self.commandedPosition
}

func (self *fakeRail) Get_name(short bool) string {
	_ = short
	return self.name
}

type fakeToolhead struct {
	trapq            interface{}
	registered       []func(float64)
	maxVelocity      float64
	maxAccel         float64
	flushCalls       int
	currentPosition  []float64
	setPositionCalls [][]float64
}

func (self *fakeToolhead) Get_trapq() interface{} {
	return self.trapq
}

func (self *fakeToolhead) Register_step_generator(handler func(float64)) {
	self.registered = append(self.registered, handler)
}

func (self *fakeToolhead) Get_max_velocity() (float64, float64) {
	return self.maxVelocity, self.maxAccel
}

func (self *fakeToolhead) Flush_step_generation() {
	self.flushCalls++
}

func (self *fakeToolhead) Get_position() []float64 {
	return append([]float64{}, self.currentPosition...)
}

func (self *fakeToolhead) Set_position(newpos []float64, homingAxes []int) {
	_ = homingAxes
	self.currentPosition = append([]float64{}, newpos...)
	self.setPositionCalls = append(self.setPositionCalls, append([]float64{}, newpos...))
}

type fakePrinter struct {
	events map[string]int
}

func (self *fakePrinter) Register_event_handler(event string, callback func([]interface{}) error) {
	_ = callback
	if self.events == nil {
		self.events = map[string]int{}
	}
	self.events[event]++
}

type limitCall struct {
	speed float64
	accel float64
}

type fakeMove struct {
	endPos       []float64
	axesD        []float64
	moveD        float64
	limitCalls   []limitCall
	moveErrorMsg string
}

func (self *fakeMove) EndPos() []float64 {
	return append([]float64{}, self.endPos...)
}

func (self *fakeMove) AxesD() []float64 {
	return append([]float64{}, self.axesD...)
}

func (self *fakeMove) MoveD() float64 {
	return self.moveD
}

func (self *fakeMove) LimitSpeed(speed float64, accel float64) {
	self.limitCalls = append(self.limitCalls, limitCall{speed: speed, accel: accel})
}

func (self *fakeMove) MoveError(msg string) error {
	self.moveErrorMsg = msg
	return errors.New(msg)
}

type homeRailsCall struct {
	rails    []Rail
	forcepos []interface{}
	homepos  []interface{}
}

type fakeHomingState struct {
	axes  []int
	calls []homeRailsCall
}

func (self *fakeHomingState) GetAxes() []int {
	return append([]int{}, self.axes...)
}

func (self *fakeHomingState) HomeRails(rails []Rail, forcepos []interface{}, homepos []interface{}) {
	copiedRails := append([]Rail{}, rails...)
	copiedForce := append([]interface{}{}, forcepos...)
	copiedHome := append([]interface{}{}, homepos...)
	self.calls = append(self.calls, homeRailsCall{rails: copiedRails, forcepos: copiedForce, homepos: copiedHome})
}

func TestNewCartesianRegistersSteppersAndReportsStatus(t *testing.T) {
	printer := &fakePrinter{}
	toolhead := &fakeToolhead{trapq: "trapq", maxVelocity: 200, maxAccel: 500}
	x := &fakeStepper{name: "stepper_x"}
	y := &fakeStepper{name: "stepper_y"}
	z := &fakeStepper{name: "stepper_z"}
	rails := []Rail{
		&fakeRail{name: "stepper_x", steppers: []Stepper{x}, rangeMin: 0, rangeMax: 200, homingInfo: &RailHomingInfo{}},
		&fakeRail{name: "stepper_y", steppers: []Stepper{y}, rangeMin: 0, rangeMax: 210, homingInfo: &RailHomingInfo{}},
		&fakeRail{name: "stepper_z", steppers: []Stepper{z}, rangeMin: -2, rangeMax: 250, homingInfo: &RailHomingInfo{}},
	}

	kin := NewCartesian(CartesianConfig{Printer: printer, Toolhead: toolhead, Rails: rails, MaxZVelocity: 25, MaxZAccel: 40})
	kin.SetPosition([]float64{0, 0, 0, 0}, []int{0, 1, 2})
	status := kin.Status(0)

	if len(toolhead.registered) != 3 {
		t.Fatalf("expected 3 registered step generators, got %d", len(toolhead.registered))
	}
	if x.currentTrapq != "trapq" || y.currentTrapq != "trapq" || z.currentTrapq != "trapq" {
		t.Fatalf("expected steppers to receive the toolhead trapq")
	}
	if printer.events["stepper_enable:motor_off"] != 1 {
		t.Fatalf("expected motor_off handler registration, got %#v", printer.events)
	}
	if status["homed_axes"] != "xyz" {
		t.Fatalf("expected homed_axes xyz, got %#v", status["homed_axes"])
	}
}

func TestCartesianCheckMovePanicsBeforeHome(t *testing.T) {
	kin := NewCartesian(CartesianConfig{
		Toolhead: &fakeToolhead{},
		Rails: []Rail{
			&fakeRail{name: "x", rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
			&fakeRail{name: "y", rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
			&fakeRail{name: "z", rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
		},
		MaxZVelocity: 20,
		MaxZAccel:    30,
	})
	move := &fakeMove{endPos: []float64{10, 0, 0}, axesD: []float64{10, 0, 0}, moveD: 10}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic for unhoned axis")
		}
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("expected error panic, got %T", recovered)
		}
		if !strings.Contains(err.Error(), "Must home axis first") {
			t.Fatalf("expected homing panic, got %v", err)
		}
	}()

	kin.CheckMove(move)
}

func TestCartesianCheckMoveScalesZSpeedAfterHome(t *testing.T) {
	kin := NewCartesian(CartesianConfig{
		Toolhead: &fakeToolhead{},
		Rails: []Rail{
			&fakeRail{name: "x", rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
			&fakeRail{name: "y", rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
			&fakeRail{name: "z", rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
		},
		MaxZVelocity: 30,
		MaxZAccel:    45,
	})
	kin.SetPosition([]float64{0, 0, 0, 0}, []int{0, 1, 2})
	move := &fakeMove{endPos: []float64{3, 4, 5}, axesD: []float64{3, 4, 5}, moveD: 10}

	kin.CheckMove(move)

	if len(move.limitCalls) != 1 {
		t.Fatalf("expected one speed limit call, got %d", len(move.limitCalls))
	}
	if move.limitCalls[0].speed != 60 || move.limitCalls[0].accel != 90 {
		t.Fatalf("unexpected z limit scaling: %+v", move.limitCalls[0])
	}
}

func TestCartesianDualCarriageUsesAxisByteAndActivatesRail(t *testing.T) {
	toolhead := &fakeToolhead{trapq: "trapq", currentPosition: []float64{10, 20, 30, 40}}
	baseY := &fakeRail{name: "stepper_y", steppers: []Stepper{&fakeStepper{name: "y0"}}, rangeMin: 0, rangeMax: 200, homingInfo: &RailHomingInfo{}, commandedPosition: 20}
	altY := &fakeRail{name: "dual_y", steppers: []Stepper{&fakeStepper{name: "y1"}}, rangeMin: 10, rangeMax: 210, homingInfo: &RailHomingInfo{}, commandedPosition: 55}
	kin := NewCartesian(CartesianConfig{
		Toolhead: toolhead,
		Rails: []Rail{
			&fakeRail{name: "stepper_x", steppers: []Stepper{&fakeStepper{name: "x"}}, rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
			baseY,
			&fakeRail{name: "stepper_z", steppers: []Stepper{&fakeStepper{name: "z"}}, rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
		},
		MaxZVelocity: 20,
		MaxZAccel:    20,
		DualCarriage: &DualCarriageConfig{
			Axis:     1,
			AxisName: "y",
			Rails:    []Rail{baseY, altY},
		},
	})
	kin.SetPosition([]float64{0, 0, 0, 0}, []int{0, 1, 2})
	kin.ActivateCarriage(1)

	if len(altY.setupCalls) != 1 || altY.setupCalls[0].alloc != "cartesian_stepper_alloc" || altY.setupCalls[0].params[0] != byte('y') {
		t.Fatalf("expected dual carriage rail to be configured for y axis, got %#v", altY.setupCalls)
	}
	if toolhead.flushCalls != 1 {
		t.Fatalf("expected one flush before carriage switch, got %d", toolhead.flushCalls)
	}
	if len(toolhead.setPositionCalls) != 1 || toolhead.setPositionCalls[0][1] != 55 {
		t.Fatalf("expected toolhead Y position to update to dual carriage commanded position, got %#v", toolhead.setPositionCalls)
	}
	if len(baseY.trapqHistory) == 0 || baseY.trapqHistory[len(baseY.trapqHistory)-1] != nil {
		t.Fatalf("expected active rail trapq to be cleared, got %#v", baseY.trapqHistory)
	}
	if len(altY.trapqHistory) == 0 || altY.trapqHistory[len(altY.trapqHistory)-1] != "trapq" {
		t.Fatalf("expected new carriage trapq to be set, got %#v", altY.trapqHistory)
	}
}

func TestNewCoreXYSharesEndstops(t *testing.T) {
	xEndstop := &fakeEndstop{}
	yEndstop := &fakeEndstop{}
	xStepper := &fakeStepper{name: "x"}
	yStepper := &fakeStepper{name: "y"}
	zStepper := &fakeStepper{name: "z"}
	toolhead := &fakeToolhead{trapq: "trapq"}

	kin := NewCoreXY(CoreXYConfig{
		Printer:  &fakePrinter{},
		Toolhead: toolhead,
		Rails: []Rail{
			&fakeRail{name: "stepper_x", steppers: []Stepper{xStepper}, endstop: xEndstop, rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
			&fakeRail{name: "stepper_y", steppers: []Stepper{yStepper}, endstop: yEndstop, rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
			&fakeRail{name: "stepper_z", steppers: []Stepper{zStepper}, endstop: &fakeEndstop{}, rangeMin: 0, rangeMax: 100, homingInfo: &RailHomingInfo{}},
		},
		MaxZVelocity: 20,
		MaxZAccel:    30,
	})

	_ = kin
	if len(xEndstop.added) != 1 || xEndstop.added[0] != yStepper {
		t.Fatalf("expected X endstop to share Y stepper, got %#v", xEndstop.added)
	}
	if len(yEndstop.added) != 1 || yEndstop.added[0] != xStepper {
		t.Fatalf("expected Y endstop to share X stepper, got %#v", yEndstop.added)
	}
	if len(toolhead.registered) != 3 {
		t.Fatalf("expected three step generators, got %d", len(toolhead.registered))
	}
}

func TestNoneStatusReturnsAxesMinMax(t *testing.T) {
	kin := NewNone(NoneConfig{AxesMinMax: []string{"1", "2", "3", "4"}})
	status := kin.Status(0)
	if status["homed_axes"] != "" {
		t.Fatalf("expected no homed axes, got %#v", status["homed_axes"])
	}
	if got := status["axis_minimum"].([]string); len(got) != 4 || got[2] != "3" {
		t.Fatalf("unexpected axis_minimum %#v", got)
	}
}
