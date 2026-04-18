package mcu

import (
	"reflect"
	"testing"

	motionpkg "goklipper/internal/pkg/motion"
	printerpkg "goklipper/internal/pkg/printer"
)

type fakeLegacyStepperPinLookupCall struct {
	pinDesc   string
	canInvert bool
	canPullup bool
	shareType interface{}
}

type fakeLegacyStepperPins struct {
	responses   map[string]map[string]interface{}
	lookupCalls []fakeLegacyStepperPinLookupCall
}

func (self *fakeLegacyStepperPins) LookupPin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{} {
	self.lookupCalls = append(self.lookupCalls, fakeLegacyStepperPinLookupCall{
		pinDesc:   pinDesc,
		canInvert: canInvert,
		canPullup: canPullup,
		shareType: shareType,
	})
	if response, ok := self.responses[pinDesc]; ok {
		return response
	}
	return nil
}

type fakeLegacyStepperPrinter struct {
	objects            map[string]interface{}
	registerEventCalls []string
	sendEventCalls     []string
}

func (self *fakeLegacyStepperPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.objects[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeLegacyStepperPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	self.registerEventCalls = append(self.registerEventCalls, event)
	_ = callback
}

func (self *fakeLegacyStepperPrinter) SendEvent(event string, params []interface{}) {
	self.sendEventCalls = append(self.sendEventCalls, event)
	_ = params
}

type fakeLegacyStepperController struct {
	nextOID                 int
	registeredConfigActions []func()
	registeredStepqueues    []interface{}
	constants               map[string]interface{}
}

func (self *fakeLegacyStepperController) CreateOID() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeLegacyStepperController) RegisterConfigCallback(cb func()) {
	self.registeredConfigActions = append(self.registeredConfigActions, cb)
}

func (self *fakeLegacyStepperController) Register_stepqueue(stepqueue interface{}) {
	self.registeredStepqueues = append(self.registeredStepqueues, stepqueue)
}

func (self *fakeLegacyStepperController) Get_constants() map[string]interface{} {
	if self.constants == nil {
		return map[string]interface{}{}
	}
	return self.constants
}

func (self *fakeLegacyStepperController) Seconds_to_clock(time float64) int64 {
	return int64(time * 1000)
}

func (self *fakeLegacyStepperController) Get_max_stepper_error() float64 {
	return 0.000025
}

func (self *fakeLegacyStepperController) Add_config_cmd(cmd string, is_init bool, on_restart bool) {
	_, _, _ = cmd, is_init, on_restart
}

func (self *fakeLegacyStepperController) Lookup_command_tag(msgformat string) interface{} {
	_ = msgformat
	return 0
}

func (self *fakeLegacyStepperController) LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{} {
	_, _, _, _, _ = msgformat, respformat, oid, cq, isAsync
	return nil
}

func (self *fakeLegacyStepperController) Is_fileoutput() bool {
	return false
}

func (self *fakeLegacyStepperController) Estimated_print_time(eventtime float64) float64 {
	return eventtime
}

func (self *fakeLegacyStepperController) Print_time_to_clock(print_time float64) int64 {
	return int64(print_time * 1000)
}

func (self *fakeLegacyStepperPrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeLegacyStepperPrinter) AddObject(name string, obj interface{}) error {
	if self.objects == nil {
		self.objects = map[string]interface{}{}
	}
	self.objects[name] = obj
	return nil
}

func (self *fakeLegacyStepperPrinter) LookupObjects(module string) []interface{} {
	_ = module
	return nil
}

func (self *fakeLegacyStepperPrinter) HasStartArg(name string) bool {
	_ = name
	return false
}

func (self *fakeLegacyStepperPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	_ = name
	return nil
}

func (self *fakeLegacyStepperPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeLegacyStepperPrinter) LookupMCU(name string) printerpkg.MCURuntime {
	_ = name
	return nil
}

func (self *fakeLegacyStepperPrinter) InvokeShutdown(msg string) {
	_ = msg
}

func (self *fakeLegacyStepperPrinter) IsShutdown() bool { return false }

func (self *fakeLegacyStepperPrinter) Reactor() printerpkg.ModuleReactor { return nil }

func (self *fakeLegacyStepperPrinter) StepperEnable() printerpkg.StepperEnableRuntime { return nil }

func (self *fakeLegacyStepperPrinter) GCode() printerpkg.GCodeRuntime { return nil }

func (self *fakeLegacyStepperPrinter) GCodeMove() printerpkg.MoveTransformController { return nil }

func (self *fakeLegacyStepperPrinter) Webhooks() printerpkg.WebhookRegistry { return nil }

type fakeLegacyStepperFloatNoneCall struct {
	option    string
	default1  interface{}
	minval    float64
	maxval    float64
	above     float64
	below     float64
	noteValid bool
}

type fakeLegacyStepperConfig struct {
	printer        *fakeLegacyStepperPrinter
	name           string
	values         map[string]interface{}
	floatNoneCalls []fakeLegacyStepperFloatNoneCall
}

type fakeLegacyStepperEnableModule struct {
	config  interface{}
	stepper stepperEnableStepper
}

func (self *fakeLegacyStepperEnableModule) Register_stepper(config interface{}, stepper stepperEnableStepper) {
	self.config = config
	self.stepper = stepper
}

type fakeLegacyForceMoveModule struct {
	stepper motionpkg.ForceMoveStepperDriver
}

func (self *fakeLegacyForceMoveModule) RegisterStepper(stepper motionpkg.ForceMoveStepperDriver) {
	self.stepper = stepper
}

func (self *fakeLegacyStepperConfig) Name() string {
	return self.name
}

func (self *fakeLegacyStepperConfig) String(option string, defaultValue string, noteValid bool) string {
	_ = noteValid
	if value, ok := self.values[option]; ok {
		return value.(string)
	}
	return defaultValue
}

func (self *fakeLegacyStepperConfig) Bool(option string, defaultValue bool) bool {
	return defaultValue
}

func (self *fakeLegacyStepperConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.values[option]; ok {
		return value.(float64)
	}
	return defaultValue
}

func (self *fakeLegacyStepperConfig) OptionalFloat(option string) *float64 {
	if value, ok := self.values[option]; ok {
		floatValue := value.(float64)
		return &floatValue
	}
	return nil
}

func (self *fakeLegacyStepperConfig) LoadObject(section string) interface{} { return nil }

func (self *fakeLegacyStepperConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeLegacyStepperConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeLegacyStepperConfig) Printer() printerpkg.ModulePrinter {
	return self.printer
}

func (self *fakeLegacyStepperConfig) Get(option string, default1 interface{}, noteValid bool) interface{} {
	_ = noteValid
	if value, ok := self.values[option]; ok {
		return value
	}
	return default1
}

func (self *fakeLegacyStepperConfig) Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	_ = default1
	_ = minval
	_ = maxval
	_ = above
	_ = below
	_ = noteValid
	return self.values[option].(float64)
}

func (self *fakeLegacyStepperConfig) Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int {
	_ = default1
	_ = minval
	_ = maxval
	_ = noteValid
	return self.values[option].(int)
}

func (self *fakeLegacyStepperConfig) Get_name() string {
	return self.name
}

func (self *fakeLegacyStepperConfig) GetfloatNone(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) interface{} {
	self.floatNoneCalls = append(self.floatNoneCalls, fakeLegacyStepperFloatNoneCall{
		option:    option,
		default1:  default1,
		minval:    minval,
		maxval:    maxval,
		above:     above,
		below:     below,
		noteValid: noteValid,
	})
	if value, ok := self.values[option]; ok {
		return value
	}
	return default1
}

func TestBuildLegacyStepperFactoryPlanBuildsStepAndDirectionSetup(t *testing.T) {
	pins := &fakeLegacyStepperPins{responses: map[string]map[string]interface{}{
		"PA0": {"pin": "PA0", "chip_name": "mcu", "invert": 0, "pullup": 0},
		"PB1": {"pin": "PB1", "chip_name": "mcu", "invert": 0, "pullup": 0},
	}}
	config := &fakeLegacyStepperConfig{
		printer: &fakeLegacyStepperPrinter{objects: map[string]interface{}{"pins": pins}},
		name:    "stepper_x",
		values: map[string]interface{}{
			"step_pin":                "PA0",
			"dir_pin":                 "PB1",
			"rotation_distance":       40.0,
			"microsteps":              16,
			"full_steps_per_rotation": 200,
			"step_pulse_duration":     0.002,
		},
	}

	plan := BuildLegacyStepperFactoryPlan(config, false)

	if plan.Name != "stepper_x" {
		t.Fatalf("unexpected stepper name %q", plan.Name)
	}
	if !reflect.DeepEqual(plan.StepPinParams, pins.responses["PA0"]) {
		t.Fatalf("unexpected step pin params %#v", plan.StepPinParams)
	}
	if !reflect.DeepEqual(plan.DirPinParams, pins.responses["PB1"]) {
		t.Fatalf("unexpected dir pin params %#v", plan.DirPinParams)
	}
	if plan.RotationDist != 40.0 || plan.StepsPerRotation != 3200 {
		t.Fatalf("unexpected distance plan %#v", plan)
	}
	if plan.StepPulseDuration != 0.002 {
		t.Fatalf("unexpected step pulse duration %#v", plan.StepPulseDuration)
	}
	if got, want := pins.lookupCalls, []fakeLegacyStepperPinLookupCall{
		{pinDesc: "PA0", canInvert: true, canPullup: false, shareType: nil},
		{pinDesc: "PB1", canInvert: true, canPullup: false, shareType: nil},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected pin lookup calls %#v", got)
	}
	if len(config.floatNoneCalls) != 1 {
		t.Fatalf("expected one GetfloatNone call, got %#v", config.floatNoneCalls)
	}
	call := config.floatNoneCalls[0]
	if call.option != "step_pulse_duration" || call.above != 0.001 || call.noteValid {
		t.Fatalf("unexpected GetfloatNone call %#v", call)
	}
}

func TestBuildLegacyStepperFactoryPlanPanicsWithoutPinResolver(t *testing.T) {
	config := &fakeLegacyStepperConfig{
		printer: &fakeLegacyStepperPrinter{objects: map[string]interface{}{"pins": struct{}{}}},
		name:    "stepper_bad",
		values: map[string]interface{}{
			"step_pin":                "PA0",
			"dir_pin":                 "PB1",
			"rotation_distance":       40.0,
			"microsteps":              16,
			"full_steps_per_rotation": 200,
		},
	}
	defer func() {
		if recover() == nil {
			t.Fatalf("expected missing pin resolver to panic")
		}
	}()
	BuildLegacyStepperFactoryPlan(config, false)
}

func TestRegisterLegacyStepperModulesLoadsKnownModulesInOrder(t *testing.T) {
	loadCalls := []string{}
	registrationCalls := []string{}
	registeredModules := []interface{}{}
	modules := map[string]interface{}{
		"stepper_enable": "enable-module",
	}

	RegisterLegacyStepperModules(func(moduleName string) interface{} {
		loadCalls = append(loadCalls, moduleName)
		return modules[moduleName]
	}, func(module interface{}) {
		registrationCalls = append(registrationCalls, "stepper_enable")
		registeredModules = append(registeredModules, module)
	}, func(module interface{}) {
		registrationCalls = append(registrationCalls, "force_move")
		registeredModules = append(registeredModules, module)
	})

	if got, want := loadCalls, []string{"stepper_enable", "force_move"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected module load order %v, want %v", got, want)
	}
	if got, want := registrationCalls, []string{"stepper_enable"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected registration calls %v, want %v", got, want)
	}
	if got, want := registeredModules, []interface{}{"enable-module"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected registered modules %#v, want %#v", got, want)
	}
}

func TestNewLegacyPrinterStepperRegistersConnectHandlerAndBindsSelf(t *testing.T) {
	controller := &fakeLegacyStepperController{}
	printer := &fakeLegacyStepperPrinter{}
	stepper := NewLegacyPrinterStepper(
		"stepper_x",
		map[string]interface{}{"chip": controller, "pin": "PA0", "invert": 0},
		map[string]interface{}{"chip": controller, "pin": "PA1", "invert": 0},
		40.0,
		200,
		nil,
		false,
		printer,
	)

	if got, want := printer.registerEventCalls, []string{"project:connect"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected registered events %v, want %v", got, want)
	}
	if stepper.Raw() != stepper {
		t.Fatalf("expected bound event self to be the stepper itself")
	}
	if len(controller.registeredConfigActions) != 1 {
		t.Fatalf("expected one config callback registration, got %d", len(controller.registeredConfigActions))
	}
	if len(controller.registeredStepqueues) != 1 {
		t.Fatalf("expected one stepqueue registration, got %d", len(controller.registeredStepqueues))
	}
}

func TestLoadLegacyPrinterStepperBuildsAndRegistersModules(t *testing.T) {
	controller := &fakeLegacyStepperController{}
	pins := &fakeLegacyStepperPins{responses: map[string]map[string]interface{}{
		"PA0": {"chip": controller, "pin": "PA0", "invert": 0, "pullup": 0},
		"PB1": {"chip": controller, "pin": "PB1", "invert": 0, "pullup": 0},
	}}
	printer := &fakeLegacyStepperPrinter{objects: map[string]interface{}{"pins": pins}}
	config := &fakeLegacyStepperConfig{
		printer: printer,
		name:    "stepper_x",
		values: map[string]interface{}{
			"step_pin":                "PA0",
			"dir_pin":                 "PB1",
			"rotation_distance":       40.0,
			"microsteps":              16,
			"full_steps_per_rotation": 200,
		},
	}
	loadCalls := []string{}
	var enableStepper *LegacyStepper
	var forceMoveStepper *LegacyStepper

	stepper := LoadLegacyPrinterStepper(config, false, printer, func(moduleName string) interface{} {
		loadCalls = append(loadCalls, moduleName)
		return moduleName + "-module"
	}, func(module interface{}, stepper *LegacyStepper) {
		if module != "stepper_enable-module" {
			t.Fatalf("unexpected stepper_enable module %#v", module)
		}
		enableStepper = stepper
	}, func(module interface{}, stepper *LegacyStepper) {
		if module != "force_move-module" {
			t.Fatalf("unexpected force_move module %#v", module)
		}
		forceMoveStepper = stepper
	})

	if stepper == nil {
		t.Fatal("expected stepper to be created")
	}
	if enableStepper != stepper || forceMoveStepper != stepper {
		t.Fatalf("expected module registrations to receive the created stepper")
	}
	if got, want := loadCalls, []string{"stepper_enable", "force_move"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected module load calls %v, want %v", got, want)
	}
	if got, want := printer.registerEventCalls, []string{"project:connect"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected registered events %v, want %v", got, want)
	}
}

func TestLoadLegacyPrinterStepperWithDefaultModulesRegistersStandardModules(t *testing.T) {
	controller := &fakeLegacyStepperController{}
	pins := &fakeLegacyStepperPins{responses: map[string]map[string]interface{}{
		"PA0": {"chip": controller, "pin": "PA0", "invert": 0, "pullup": 0},
		"PB1": {"chip": controller, "pin": "PB1", "invert": 0, "pullup": 0},
	}}
	printer := &fakeLegacyStepperPrinter{objects: map[string]interface{}{"pins": pins}}
	config := &fakeLegacyStepperConfig{
		printer: printer,
		name:    "manual_stepper select_stepper",
		values: map[string]interface{}{
			"step_pin":                "PA0",
			"dir_pin":                 "PB1",
			"rotation_distance":       40.0,
			"microsteps":              16,
			"full_steps_per_rotation": 200,
		},
	}
	stepperEnableModule := &fakeLegacyStepperEnableModule{}
	forceMoveModule := &fakeLegacyForceMoveModule{}
	loadCalls := []string{}

	stepper := LoadLegacyPrinterStepperWithDefaultModules(config, false, printer, func(moduleName string) interface{} {
		loadCalls = append(loadCalls, moduleName)
		switch moduleName {
		case "stepper_enable":
			return stepperEnableModule
		case "force_move":
			return forceMoveModule
		default:
			return nil
		}
	})

	if stepper == nil {
		t.Fatal("expected stepper to be created")
	}
	if stepperEnableModule.config != config {
		t.Fatalf("expected stepper_enable registration to receive config %p, got %p", config, stepperEnableModule.config)
	}
	if registeredStepper, ok := stepperEnableModule.stepper.(*LegacyStepper); !ok || registeredStepper != stepper {
		t.Fatalf("expected stepper_enable registration to receive the created stepper, got %#v", stepperEnableModule.stepper)
	}
	if registeredStepper, ok := forceMoveModule.stepper.(*LegacyStepper); !ok || registeredStepper != stepper {
		t.Fatalf("expected force_move registration to receive the created stepper, got %#v", forceMoveModule.stepper)
	}
	if got, want := loadCalls, []string{"stepper_enable", "force_move"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected module load calls %v, want %v", got, want)
	}
	if got, want := printer.registerEventCalls, []string{"project:connect"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected registered events %v, want %v", got, want)
	}
}
