package addon

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeSafeZTimer struct{}

func (self *fakeSafeZTimer) Update(waketime float64) {}

type fakeSafeZReactor struct {
	monotonic float64
}

func (self *fakeSafeZReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	return &fakeSafeZTimer{}
}

func (self *fakeSafeZReactor) Monotonic() float64 {
	return self.monotonic
}

type fakeSafeZCommand struct {
	strings   map[string]string
	params    map[string]string
	responses []string
	raws      []string
}

func (self *fakeSafeZCommand) String(name string, defaultValue string) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	if value, ok := self.params[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeSafeZCommand) Float(name string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeSafeZCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeSafeZCommand) Parameters() map[string]string {
	params := map[string]string{}
	for key, value := range self.params {
		params[key] = value
	}
	return params
}

func (self *fakeSafeZCommand) RespondInfo(msg string, log bool) {
	self.responses = append(self.responses, msg)
}

func (self *fakeSafeZCommand) RespondRaw(msg string) {
	self.raws = append(self.raws, msg)
}

type fakeSafeZMutex struct{}

func (self *fakeSafeZMutex) Lock()   {}
func (self *fakeSafeZMutex) Unlock() {}

type fakeSafeZGCode struct {
	commands map[string]func(printerpkg.Command) error
	mutex    printerpkg.Mutex
	prevG28  func(printerpkg.Command) error
	created  []map[string]string
}

func (self *fakeSafeZGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.commands == nil {
		self.commands = map[string]func(printerpkg.Command) error{}
	}
	self.commands[cmd] = handler
}

func (self *fakeSafeZGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeSafeZGCode) RunScriptFromCommand(script string) {}

func (self *fakeSafeZGCode) RunScript(script string) {}

func (self *fakeSafeZGCode) IsBusy() bool {
	return false
}

func (self *fakeSafeZGCode) Mutex() printerpkg.Mutex {
	if self.mutex == nil {
		self.mutex = &fakeSafeZMutex{}
	}
	return self.mutex
}

func (self *fakeSafeZGCode) RespondInfo(msg string, log bool) {}

func (self *fakeSafeZGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	old := self.commands[cmd]
	self.RegisterCommand(cmd, handler, whenNotReady, desc)
	return old
}

func (self *fakeSafeZGCode) CreateCommand(cmd string, raw string, params map[string]string) printerpkg.Command {
	copyParams := map[string]string{}
	for key, value := range params {
		copyParams[key] = value
	}
	self.created = append(self.created, copyParams)
	return &fakeSafeZCommand{params: copyParams}
}

type fakeSafeZToolhead struct {
	position         []float64
	homedAxes        string
	setPositions     [][]float64
	setHomingAxes    [][]int
	manualMoves      [][]interface{}
	manualMoveSpeeds []float64
	noteZNotHomed    int
}

func (self *fakeSafeZToolhead) GetPosition() []float64 {
	pos := make([]float64, len(self.position))
	copy(pos, self.position)
	return pos
}

func (self *fakeSafeZToolhead) SetPosition(newpos []float64, homingAxes []int) {
	pos := make([]float64, len(newpos))
	copy(pos, newpos)
	self.position = pos
	recordedPos := make([]float64, len(pos))
	copy(recordedPos, pos)
	self.setPositions = append(self.setPositions, recordedPos)
	axes := make([]int, len(homingAxes))
	copy(axes, homingAxes)
	self.setHomingAxes = append(self.setHomingAxes, axes)
}

func (self *fakeSafeZToolhead) ManualMove(coord []interface{}, speed float64) {
	move := make([]interface{}, len(coord))
	copy(move, coord)
	self.manualMoves = append(self.manualMoves, move)
	self.manualMoveSpeeds = append(self.manualMoveSpeeds, speed)
	for i, value := range coord {
		if value == nil {
			continue
		}
		self.position[i] = value.(float64)
	}
}

func (self *fakeSafeZToolhead) HomedAxes(eventtime float64) string {
	return self.homedAxes
}

func (self *fakeSafeZToolhead) NoteZNotHomed() {
	self.noteZNotHomed++
}

type fakeSafeZPrinter struct {
	gcode   printerpkg.GCodeRuntime
	reactor printerpkg.ModuleReactor
	lookup  map[string]interface{}
}

func (self *fakeSafeZPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeSafeZPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {}

func (self *fakeSafeZPrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeSafeZPrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeSafeZPrinter) AddObject(name string, obj interface{}) error {
	if self.lookup == nil {
		self.lookup = map[string]interface{}{}
	}
	self.lookup[name] = obj
	return nil
}

func (self *fakeSafeZPrinter) LookupObjects(module string) []interface{} { return nil }

func (self *fakeSafeZPrinter) HasStartArg(name string) bool { return false }

func (self *fakeSafeZPrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeSafeZPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry { return nil }

func (self *fakeSafeZPrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }

func (self *fakeSafeZPrinter) InvokeShutdown(msg string) {}

func (self *fakeSafeZPrinter) IsShutdown() bool { return false }

func (self *fakeSafeZPrinter) Reactor() printerpkg.ModuleReactor { return self.reactor }

func (self *fakeSafeZPrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeSafeZPrinter) GCode() printerpkg.GCodeRuntime { return self.gcode }

func (self *fakeSafeZPrinter) GCodeMove() printerpkg.MoveTransformController { return nil }

func (self *fakeSafeZPrinter) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeSafeZConfig struct {
	printer       printerpkg.ModulePrinter
	strings       map[string]string
	floats        map[string]float64
	bools         map[string]bool
	loadedObjects []string
	sections      map[string]bool
}

func (self *fakeSafeZConfig) Name() string { return "safe_z_home" }

func (self *fakeSafeZConfig) String(option string, defaultValue string, noteValid bool) string {
	if value, ok := self.strings[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeSafeZConfig) Bool(option string, defaultValue bool) bool {
	if value, ok := self.bools[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeSafeZConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.floats[option]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeSafeZConfig) OptionalFloat(option string) *float64 { return nil }

func (self *fakeSafeZConfig) LoadObject(section string) interface{} {
	self.loadedObjects = append(self.loadedObjects, section)
	return nil
}

func (self *fakeSafeZConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeSafeZConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeSafeZConfig) Printer() printerpkg.ModulePrinter { return self.printer }

func (self *fakeSafeZConfig) HasSection(section string) bool { return self.sections[section] }

func TestLoadConfigSafeZHomingRegistersCommands(t *testing.T) {
	toolhead := &fakeSafeZToolhead{position: []float64{0, 0, 0, 0}}
	gcode := &fakeSafeZGCode{commands: map[string]func(printerpkg.Command) error{"G28": func(printerpkg.Command) error { return nil }}}
	printer := &fakeSafeZPrinter{
		gcode:   gcode,
		reactor: &fakeSafeZReactor{monotonic: 10},
		lookup:  map[string]interface{}{"toolhead": toolhead},
	}
	config := &fakeSafeZConfig{
		printer: printer,
		strings: map[string]string{"home_xy_position": "100,200"},
		floats:  map[string]float64{"speed": 50, "z_hop_speed": 15},
		bools:   map[string]bool{},
		sections: map[string]bool{},
	}

	loaded := LoadConfigSafeZHoming(config)
	module, ok := loaded.(*SafeZHomingModule)
	if !ok || module == nil {
		t.Fatalf("expected safe z homing module, got %#v", loaded)
	}
	if got, want := config.loadedObjects, []string{"homing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded objects = %v, want %v", got, want)
	}
	commands := make([]string, 0, len(gcode.commands))
	for command := range gcode.commands {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	if got, want := commands, []string{"G28", "H28"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
	if module.prevG28 == nil {
		t.Fatalf("expected previous G28 handler to be captured")
	}
}

func TestLoadConfigSafeZHomingRejectsHomingOverride(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("expected panic when homing_override section exists")
		}
	}()
	printer := &fakeSafeZPrinter{gcode: &fakeSafeZGCode{commands: map[string]func(printerpkg.Command) error{"G28": func(printerpkg.Command) error { return nil }}}, reactor: &fakeSafeZReactor{}}
	config := &fakeSafeZConfig{
		printer:  printer,
		strings:  map[string]string{"home_xy_position": "0,0"},
		floats:   map[string]float64{},
		bools:    map[string]bool{},
		sections: map[string]bool{"homing_override": true},
	}
	LoadConfigSafeZHoming(config)
}

func TestSafeZHomingCmdG28PerformsSafeMoveAndRestore(t *testing.T) {
	toolhead := &fakeSafeZToolhead{position: []float64{1, 2, 0, 0}, homedAxes: ""}
	gcode := &fakeSafeZGCode{commands: map[string]func(printerpkg.Command) error{}}
	gcode.commands["G28"] = func(gcmd printerpkg.Command) error {
		params := gcmd.Parameters()
		if _, ok := params["X"]; ok && !strings.Contains(toolhead.homedAxes, "x") {
			toolhead.homedAxes += "x"
		}
		if _, ok := params["Y"]; ok && !strings.Contains(toolhead.homedAxes, "y") {
			toolhead.homedAxes += "y"
		}
		if _, ok := params["Z"]; ok && !strings.Contains(toolhead.homedAxes, "z") {
			toolhead.homedAxes += "z"
		}
		return nil
	}
	printer := &fakeSafeZPrinter{
		gcode:   gcode,
		reactor: &fakeSafeZReactor{monotonic: 10},
		lookup:  map[string]interface{}{"toolhead": toolhead},
	}
	config := &fakeSafeZConfig{
		printer: printer,
		strings: map[string]string{"home_xy_position": "10,20"},
		floats: map[string]float64{
			"z_hop":       5,
			"z_hop_speed": 15,
			"speed":       50,
		},
		bools:    map[string]bool{"move_to_previous": true},
		sections: map[string]bool{},
	}
	module := LoadConfigSafeZHoming(config).(*SafeZHomingModule)

	if err := module.cmdG28(&fakeSafeZCommand{}); err != nil {
		t.Fatalf("cmdG28() unexpected error: %v", err)
	}
	if got, want := len(gcode.created), 2; got != want {
		t.Fatalf("created G28 commands = %d, want %d", got, want)
	}
	if got, want := gcode.created[0], map[string]string{"X": "0", "Y": "0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first created command = %v, want %v", got, want)
	}
	if got, want := gcode.created[1], map[string]string{"Z": "0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second created command = %v, want %v", got, want)
	}
	if got, want := toolhead.noteZNotHomed, 1; got != want {
		t.Fatalf("NoteZNotHomed calls = %d, want %d", got, want)
	}
	if got, want := toolhead.setPositions, [][]float64{{1, 2, 0, 0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("set positions = %v, want %v", got, want)
	}
	if got, want := toolhead.setHomingAxes, [][]int{{2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("set homing axes = %v, want %v", got, want)
	}
	if got, want := toolhead.manualMoves, [][]interface{}{{nil, nil, 5.0}, {10.0, 20.0}, {nil, nil, 5.0}, {1.0, 2.0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("manual moves = %v, want %v", got, want)
	}
	if got, want := toolhead.manualMoveSpeeds, []float64{15, 50, 15, 50}; !reflect.DeepEqual(got, want) {
		t.Fatalf("manual move speeds = %v, want %v", got, want)
	}
	if got, want := toolhead.GetPosition(), []float64{1, 2, 5, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("final position = %v, want %v", got, want)
	}
}

func TestSafeZHomingCmdH28SkipsAlreadyHomedXY(t *testing.T) {
	toolhead := &fakeSafeZToolhead{position: []float64{5, 6, 1, 0}, homedAxes: "xy"}
	gcode := &fakeSafeZGCode{commands: map[string]func(printerpkg.Command) error{}}
	gcode.commands["G28"] = func(gcmd printerpkg.Command) error {
		if _, ok := gcmd.Parameters()["Z"]; ok && !strings.Contains(toolhead.homedAxes, "z") {
			toolhead.homedAxes += "z"
		}
		return nil
	}
	printer := &fakeSafeZPrinter{
		gcode:   gcode,
		reactor: &fakeSafeZReactor{monotonic: 20},
		lookup:  map[string]interface{}{"toolhead": toolhead},
	}
	config := &fakeSafeZConfig{
		printer: printer,
		strings: map[string]string{"home_xy_position": "11,22"},
		floats: map[string]float64{
			"z_hop":       2,
			"z_hop_speed": 12,
			"speed":       30,
		},
		bools:    map[string]bool{"move_to_previous": false},
		sections: map[string]bool{},
	}
	module := LoadConfigSafeZHoming(config).(*SafeZHomingModule)

	if err := module.cmdH28(&fakeSafeZCommand{}); err != nil {
		t.Fatalf("cmdH28() unexpected error: %v", err)
	}
	if got, want := gcode.created, []map[string]string{{"Z": "0"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("created commands = %v, want %v", got, want)
	}
	if got, want := toolhead.manualMoves, [][]interface{}{{nil, nil, 2.0}, {11.0, 22.0}, {nil, nil, 2.0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("manual moves = %v, want %v", got, want)
	}
}