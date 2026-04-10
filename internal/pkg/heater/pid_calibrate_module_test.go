package heater

import (
	"strings"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakePIDCalibrateHeater struct {
	maxPower        float64
	pwmDelay        float64
	setPWMCalls     [][2]float64
	alterTargets    []float64
	controls        []interface{}
	controlToReturn interface{}
}

func (self *fakePIDCalibrateHeater) Get_max_power() float64 {
	return self.maxPower
}

func (self *fakePIDCalibrateHeater) Get_pwm_delay() float64 {
	return self.pwmDelay
}

func (self *fakePIDCalibrateHeater) Set_pwm(read_time float64, value float64) {
	self.setPWMCalls = append(self.setPWMCalls, [2]float64{read_time, value})
}

func (self *fakePIDCalibrateHeater) Alter_target(target_temp float64) {
	self.alterTargets = append(self.alterTargets, target_temp)
}

func (self *fakePIDCalibrateHeater) Set_control(control interface{}) interface{} {
	self.controls = append(self.controls, control)
	if self.controlToReturn != nil {
		return self.controlToReturn
	}
	return "old-control"
}

type fakePIDCalibrateHeaterManager struct {
	heater       interface{}
	lookupNames  []string
	setTempCalls []struct {
		heater interface{}
		temp   float64
		wait   bool
	}
	setTempFunc func(interface{}, float64, bool) error
}

func (self *fakePIDCalibrateHeaterManager) LookupHeater(name string) interface{} {
	self.lookupNames = append(self.lookupNames, name)
	return self.heater
}

func (self *fakePIDCalibrateHeaterManager) Set_temperature(heater interface{}, temp float64, wait bool) error {
	self.setTempCalls = append(self.setTempCalls, struct {
		heater interface{}
		temp   float64
		wait   bool
	}{heater: heater, temp: temp, wait: wait})
	if self.setTempFunc != nil {
		return self.setTempFunc(heater, temp, wait)
	}
	return nil
}

type fakePIDCalibrateToolhead struct {
	moveCalls int
}

func (self *fakePIDCalibrateToolhead) Get_last_move_time() float64 {
	self.moveCalls++
	return 0
}

type fakePIDCalibrateConfigfile struct {
	sets [][3]string
}

func (self *fakePIDCalibrateConfigfile) Set(section string, option string, val string) {
	self.sets = append(self.sets, [3]string{section, option, val})
}

type fakePIDCalibrateGCode struct {
	handlers map[string]func(printerpkg.Command) error
	descs    map[string]string
}

func (self *fakePIDCalibrateGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
	if self.handlers == nil {
		self.handlers = map[string]func(printerpkg.Command) error{}
		self.descs = map[string]string{}
	}
	self.handlers[cmd] = handler
	self.descs[cmd] = desc
}

func (self *fakePIDCalibrateGCode) IsTraditionalGCode(cmd string) bool { return false }
func (self *fakePIDCalibrateGCode) RunScriptFromCommand(script string) {}
func (self *fakePIDCalibrateGCode) RunScript(script string)            {}
func (self *fakePIDCalibrateGCode) IsBusy() bool                       { return false }
func (self *fakePIDCalibrateGCode) Mutex() printerpkg.Mutex            { return nil }
func (self *fakePIDCalibrateGCode) RespondInfo(msg string, log bool)   {}
func (self *fakePIDCalibrateGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakePIDCalibratePrinter struct {
	lookup map[string]interface{}
	gcode  printerpkg.GCodeRuntime
}

func (self *fakePIDCalibratePrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakePIDCalibratePrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}
func (self *fakePIDCalibratePrinter) SendEvent(event string, params []interface{})      {}
func (self *fakePIDCalibratePrinter) CurrentExtruderName() string                       { return "extruder" }
func (self *fakePIDCalibratePrinter) AddObject(name string, obj interface{}) error      { return nil }
func (self *fakePIDCalibratePrinter) LookupObjects(module string) []interface{}         { return nil }
func (self *fakePIDCalibratePrinter) HasStartArg(name string) bool                      { return false }
func (self *fakePIDCalibratePrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }
func (self *fakePIDCalibratePrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}
func (self *fakePIDCalibratePrinter) LookupMCU(name string) printerpkg.MCURuntime    { return nil }
func (self *fakePIDCalibratePrinter) InvokeShutdown(msg string)                      {}
func (self *fakePIDCalibratePrinter) IsShutdown() bool                               { return false }
func (self *fakePIDCalibratePrinter) Reactor() printerpkg.ModuleReactor              { return nil }
func (self *fakePIDCalibratePrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }
func (self *fakePIDCalibratePrinter) GCode() printerpkg.GCodeRuntime                 { return self.gcode }
func (self *fakePIDCalibratePrinter) GCodeMove() printerpkg.MoveTransformController  { return nil }
func (self *fakePIDCalibratePrinter) Webhooks() printerpkg.WebhookRegistry           { return nil }

type fakePIDCalibrateConfig struct {
	printer printerpkg.ModulePrinter
	name    string
}

func (self *fakePIDCalibrateConfig) Name() string { return self.name }
func (self *fakePIDCalibrateConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}
func (self *fakePIDCalibrateConfig) Bool(option string, defaultValue bool) bool { return defaultValue }
func (self *fakePIDCalibrateConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}
func (self *fakePIDCalibrateConfig) OptionalFloat(option string) *float64  { return nil }
func (self *fakePIDCalibrateConfig) LoadObject(section string) interface{} { return nil }
func (self *fakePIDCalibrateConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}
func (self *fakePIDCalibrateConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}
func (self *fakePIDCalibrateConfig) Printer() printerpkg.ModulePrinter { return self.printer }

type fakePIDCalibrateCommand struct {
	strings map[string]string
	floats  map[string]float64
	ints    map[string]int
	infos   []string
	raws    []string
}

func (self *fakePIDCalibrateCommand) String(name string, defaultValue string) string {
	if value, ok := self.strings[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakePIDCalibrateCommand) Float(name string, defaultValue float64) float64 {
	if value, ok := self.floats[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakePIDCalibrateCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	if value, ok := self.ints[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakePIDCalibrateCommand) Parameters() map[string]string { return self.strings }

func (self *fakePIDCalibrateCommand) RespondInfo(msg string, log bool) {
	self.infos = append(self.infos, msg)
}

func (self *fakePIDCalibrateCommand) RespondRaw(msg string) {
	self.raws = append(self.raws, msg)
}

func TestLoadConfigPIDCalibrateRegistersCommand(t *testing.T) {
	gcode := &fakePIDCalibrateGCode{}
	printer := &fakePIDCalibratePrinter{gcode: gcode}
	module := LoadConfigPIDCalibrate(&fakePIDCalibrateConfig{printer: printer, name: "pid_calibrate"}).(*PIDCalibrateModule)
	if module == nil {
		t.Fatalf("expected module instance")
	}
	if gcode.handlers["PID_CALIBRATE"] == nil {
		t.Fatalf("expected PID_CALIBRATE handler registration")
	}
	if gcode.descs["PID_CALIBRATE"] != cmdPIDCalibrateHelp {
		t.Fatalf("unexpected command help: %q", gcode.descs["PID_CALIBRATE"])
	}
}

func TestPIDCalibrateCommandRunsAndStoresResults(t *testing.T) {
	heater := &fakePIDCalibrateHeater{maxPower: 100.0, pwmDelay: 0.2, controlToReturn: "old-control"}
	heaterManager := &fakePIDCalibrateHeaterManager{
		heater: heater,
		setTempFunc: func(heaterObj interface{}, temp float64, wait bool) error {
			control, ok := heater.controls[len(heater.controls)-1].(*ControlAutoTune)
			if !ok {
				t.Fatalf("expected ControlAutoTune control, got %T", heater.controls[len(heater.controls)-1])
			}
			for i := 0; i < 12; i++ {
				if i%2 == 0 {
					control.Temperature_update(float64(i)+0.1, temp-TunePIDDelta-1.0, targetOr(temp, 200.0))
				} else {
					control.Temperature_update(float64(i)+0.1, temp+TunePIDDelta, targetOr(temp, 200.0)-TunePIDDelta)
				}
			}
			return nil
		},
	}
	toolhead := &fakePIDCalibrateToolhead{}
	configfile := &fakePIDCalibrateConfigfile{}
	gcode := &fakePIDCalibrateGCode{}
	printer := &fakePIDCalibratePrinter{
		gcode: gcode,
		lookup: map[string]interface{}{
			"heaters":    heaterManager,
			"toolhead":   toolhead,
			"configfile": configfile,
		},
	}
	LoadConfigPIDCalibrate(&fakePIDCalibrateConfig{printer: printer, name: "pid_calibrate"})
	command := &fakePIDCalibrateCommand{
		strings: map[string]string{"HEATER": "extruder"},
		floats:  map[string]float64{"TARGET": 200.0},
		ints:    map[string]int{"WRITE_FILE": 0},
	}
	if err := gcode.handlers["PID_CALIBRATE"](command); err != nil {
		t.Fatalf("PID_CALIBRATE returned error: %v", err)
	}
	if len(heaterManager.lookupNames) != 1 || heaterManager.lookupNames[0] != "extruder" {
		t.Fatalf("unexpected heater lookups: %#v", heaterManager.lookupNames)
	}
	if len(heaterManager.setTempCalls) != 1 {
		t.Fatalf("expected one set temperature call, got %#v", heaterManager.setTempCalls)
	}
	if heaterManager.setTempCalls[0].temp != 200.0 || !heaterManager.setTempCalls[0].wait {
		t.Fatalf("unexpected set temperature call: %#v", heaterManager.setTempCalls[0])
	}
	if toolhead.moveCalls != 1 {
		t.Fatalf("expected one toolhead move-time query, got %d", toolhead.moveCalls)
	}
	if len(heater.controls) != 2 {
		t.Fatalf("expected calibrate control install and restore, got %#v", heater.controls)
	}
	if heater.controls[1] != "old-control" {
		t.Fatalf("expected old control restore, got %#v", heater.controls[1])
	}
	if len(configfile.sets) != 4 {
		t.Fatalf("expected configfile updates, got %#v", configfile.sets)
	}
	if configfile.sets[0] != [3]string{"extruder", "control", "pid"} {
		t.Fatalf("unexpected first configfile set: %#v", configfile.sets[0])
	}
	if len(command.infos) != 1 || !strings.Contains(command.infos[0], "PID parameters:") {
		t.Fatalf("expected PID response info, got %#v", command.infos)
	}
	if len(heater.setPWMCalls) == 0 {
		t.Fatalf("expected control to drive pwm during calibration")
	}
	if len(heater.alterTargets) == 0 {
		t.Fatalf("expected control to alter targets during calibration")
	}
}

func TestPIDCalibrateRestoresControlOnFailure(t *testing.T) {
	heater := &fakePIDCalibrateHeater{maxPower: 50.0, pwmDelay: 0.1, controlToReturn: "old-control"}
	heaterManager := &fakePIDCalibrateHeaterManager{
		heater: heater,
		setTempFunc: func(heater interface{}, temp float64, wait bool) error {
			panic("boom")
		},
	}
	gcode := &fakePIDCalibrateGCode{}
	printer := &fakePIDCalibratePrinter{
		gcode: gcode,
		lookup: map[string]interface{}{
			"heaters":    heaterManager,
			"toolhead":   &fakePIDCalibrateToolhead{},
			"configfile": &fakePIDCalibrateConfigfile{},
		},
	}
	LoadConfigPIDCalibrate(&fakePIDCalibrateConfig{printer: printer, name: "pid_calibrate"})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic from failed calibration")
		}
		if len(heater.controls) != 2 || heater.controls[1] != "old-control" {
			t.Fatalf("expected old control restore after panic, got %#v", heater.controls)
		}
	}()

	_ = gcode.handlers["PID_CALIBRATE"](&fakePIDCalibrateCommand{
		strings: map[string]string{"HEATER": "extruder"},
		floats:  map[string]float64{"TARGET": 200.0},
	})
}

func targetOr(value float64, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}
