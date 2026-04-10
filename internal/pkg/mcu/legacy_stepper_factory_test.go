package mcu

import (
	"reflect"
	"testing"

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

func (self *fakeLegacyStepperPins) Lookup_pin(pinDesc string, canInvert bool, canPullup bool, shareType interface{}) map[string]interface{} {
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
	objects map[string]interface{}
}

func (self *fakeLegacyStepperPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := self.objects[name]; ok {
		return value
	}
	return defaultValue
}

func (self *fakeLegacyStepperPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
}

func (self *fakeLegacyStepperPrinter) SendEvent(event string, params []interface{}) {}

func (self *fakeLegacyStepperPrinter) CurrentExtruderName() string { return "extruder" }

func (self *fakeLegacyStepperPrinter) AddObject(name string, obj interface{}) error { return nil }

func (self *fakeLegacyStepperPrinter) LookupObjects(module string) []interface{} { return nil }

func (self *fakeLegacyStepperPrinter) HasStartArg(name string) bool { return false }

func (self *fakeLegacyStepperPrinter) LookupHeater(name string) printerpkg.HeaterRuntime { return nil }

func (self *fakeLegacyStepperPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeLegacyStepperPrinter) LookupMCU(name string) printerpkg.MCURuntime { return nil }

func (self *fakeLegacyStepperPrinter) InvokeShutdown(msg string) {}

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

type fakeLegacyStepperModuleRegistrar struct {
	registrationCalls []string
	modules           []interface{}
}

func (self *fakeLegacyStepperModuleRegistrar) RegisterStepperEnable(module interface{}) {
	self.registrationCalls = append(self.registrationCalls, "stepper_enable")
	self.modules = append(self.modules, module)
}

func (self *fakeLegacyStepperModuleRegistrar) RegisterForceMove(module interface{}) {
	self.registrationCalls = append(self.registrationCalls, "force_move")
	self.modules = append(self.modules, module)
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
	modules := map[string]interface{}{
		"stepper_enable": "enable-module",
	}
	registrar := &fakeLegacyStepperModuleRegistrar{}

	RegisterLegacyStepperModules(func(moduleName string) interface{} {
		loadCalls = append(loadCalls, moduleName)
		return modules[moduleName]
	}, registrar)

	if got, want := loadCalls, []string{"stepper_enable", "force_move"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected module load order %v, want %v", got, want)
	}
	if got, want := registrar.registrationCalls, []string{"stepper_enable"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected registration calls %v, want %v", got, want)
	}
	if got, want := registrar.modules, []interface{}{"enable-module"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected registered modules %#v, want %#v", got, want)
	}
}
