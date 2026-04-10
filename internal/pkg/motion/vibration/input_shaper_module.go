package vibration

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"goklipper/common/collections"
	"goklipper/internal/pkg/chelper"
	printerpkg "goklipper/internal/pkg/printer"
)

type inputShaperCommand interface {
	printerpkg.Command
	Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64, above *float64, below *float64) string
	Get_float(name string, _default interface{}, minval *float64, maxval *float64, above *float64, below *float64) float64
}

type inputShaperToolhead interface {
	Get_kinematics() interface{}
	Flush_step_generation()
	Note_step_generation_scan_time(delay, old_delay float64)
}

type inputShaperKinematics interface {
	Get_steppers() []interface{}
}

type inputShaperStepper interface {
	Set_stepper_kinematics(sk interface{}) interface{}
}

func requireInputShaperCommand(gcmd printerpkg.Command) inputShaperCommand {
	cmd, ok := gcmd.(inputShaperCommand)
	if !ok {
		panic(fmt.Sprintf("input shaper command does not implement legacy getters: %T", gcmd))
	}
	return cmd
}

func requireInputShaperToolhead(printer printerpkg.ModulePrinter) inputShaperToolhead {
	toolheadObj := printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(inputShaperToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement inputShaperToolhead: %T", toolheadObj))
	}
	return toolhead
}

func requireInputShaperKinematics(toolhead inputShaperToolhead) inputShaperKinematics {
	kinematicsObj := toolhead.Get_kinematics()
	kinematics, ok := kinematicsObj.(inputShaperKinematics)
	if !ok {
		panic(fmt.Sprintf("toolhead kinematics does not implement inputShaperKinematics: %T", kinematicsObj))
	}
	return kinematics
}

func configFloatRange(config printerpkg.ModuleConfig, option string, defaultValue float64, minval *float64, maxval *float64) float64 {
	value := config.Float(option, defaultValue)
	if minval != nil && value < *minval {
		panic(fmt.Sprintf("Option '%s' in section '%s' must be at least %.6f", option, config.Name(), *minval))
	}
	if maxval != nil && value > *maxval {
		panic(fmt.Sprintf("Option '%s' in section '%s' must be at most %.6f", option, config.Name(), *maxval))
	}
	return value
}

type InputShaperParams struct {
	axis          string
	shapers       map[string]func(shaper_freq, damping_ratio float64) ([]float64, []float64)
	shaper_type   string
	damping_ratio float64
	shaper_freq   float64
}

func NewInputShaperParams(axis string, config printerpkg.ModuleConfig) *InputShaperParams {
	self := &InputShaperParams{}
	self.axis = axis
	self.shapers = make(map[string]func(shaper_freq, damping_ratio float64) ([]float64, []float64))
	for _, shaper := range INPUT_SHAPERS {
		self.shapers[shaper.Name] = shaper.Init_func
	}
	shaperType := config.String("shaper_type", "mzv", true)
	self.shaper_type = config.String("shaper_type_"+axis, shaperType, true)
	if _, ok := self.shapers[self.shaper_type]; !ok {
		panic(fmt.Errorf("supported shaper type: %s", self.shaper_type))
	}
	zero := 0.0
	one := 1.0
	self.damping_ratio = configFloatRange(config, "damping_ratio_"+axis, DEFAULT_DAMPING_RATIO, &zero, &one)
	self.shaper_freq = configFloatRange(config, "shaper_freq_"+axis, 0., &zero, nil)
	return self
}

func (self *InputShaperParams) Update(gcmd printerpkg.Command) {
	cmd := requireInputShaperCommand(gcmd)
	axis := strings.ToUpper(self.axis)
	zero := 0.0
	one := 1.0
	self.damping_ratio = cmd.Get_float("DAMPING_RATIO_"+axis, self.damping_ratio, &zero, &one, nil, nil)
	self.shaper_freq = cmd.Get_float("SHAPER_FREQ_"+axis, self.shaper_freq, &zero, nil, nil, nil)
	shaperType := cmd.Get("SHAPER_TYPE", "", nil, nil, nil, nil, nil)
	if shaperType == "" {
		shaperType = cmd.Get("SHAPER_TYPE_"+axis, self.shaper_type, nil, nil, nil, nil, nil)
	}
	shaperType = strings.ToLower(shaperType)
	if _, ok := self.shapers[shaperType]; !ok {
		panic(fmt.Errorf("Unsupported shaper type: %s", shaperType))
	}
	self.shaper_type = shaperType
}

func (self *InputShaperParams) Get_shaper() (int, []float64, []float64) {
	var A, T []float64
	if self.shaper_freq == 0 {
		A, T = Get_none_shaper()
	} else {
		A, T = self.shapers[self.shaper_type](self.shaper_freq, self.damping_ratio)
	}
	return len(A), A, T
}

func (self *InputShaperParams) get_status() *collections.SortedMap {
	m := map[string]interface{}{
		"shaper_type":   self.shaper_type,
		"shaper_freq":   fmt.Sprintf("%.3f", self.shaper_freq),
		"damping_ratio": fmt.Sprintf("%.6f", self.damping_ratio),
	}
	return collections.NewSortedMap1([]string{"shaper_type", "shaper_freq", "damping_ratio"}, m)
}

type AxisInputShaper struct {
	axis   string
	params *InputShaperParams
	n      int
	A      []float64
	T      []float64
	saved  *AxisInputShaperSaved
}

type AxisInputShaperSaved struct {
	n int
	A []float64
	T []float64
}

func NewAxisInputShaper(axis string, config printerpkg.ModuleConfig) *AxisInputShaper {
	self := &AxisInputShaper{}
	self.axis = axis
	self.params = NewInputShaperParams(axis, config)
	self.n, self.A, self.T = self.params.Get_shaper()
	return self
}

func (self *AxisInputShaper) Get_name() string {
	return "shaper_" + self.axis
}

func (self *AxisInputShaper) Get_shaper() (int, []float64, []float64) {
	return self.n, self.A, self.T
}

func (self *AxisInputShaper) Update(gcmd printerpkg.Command) bool {
	self.params.Update(gcmd)
	old_n, old_A, old_T := self.n, self.A, self.T
	self.n, self.A, self.T = self.params.Get_shaper()
	return old_n != self.n &&
		!reflect.DeepEqual(old_A, self.A) &&
		!reflect.DeepEqual(old_T, self.T)
}

func (self *AxisInputShaper) Set_shaper_kinematics(sk interface{}) bool {
	success := chelper.Input_shaper_set_shaper_params(sk, int8(self.axis[0]), self.n, self.A, self.T) == 0
	if !success {
		self.disable_shaping()
		chelper.Input_shaper_set_shaper_params(sk, int8(self.axis[0]), self.n, self.A, self.T)
	}
	return success
}

func (self *AxisInputShaper) Get_step_generation_window() float64 {
	if len(self.A) == 0 || len(self.T) == 0 {
		return 0.
	}
	return chelper.Input_shaper_get_step_generation_window(self.n, self.A, self.T)
}

func (self *AxisInputShaper) disable_shaping() {
	if self.saved == nil && self.n != 0 {
		self.saved = &AxisInputShaperSaved{n: self.n, A: self.A, T: self.T}
	}
	A, T := Get_none_shaper()
	self.n, self.A, self.T = len(A), A, T
}

func (self *AxisInputShaper) Enable_shaping() {
	if self.saved == nil {
		return
	}
	self.n = self.saved.n
	self.A = self.saved.A
	self.T = self.saved.T
	self.saved = nil
}

func (self *AxisInputShaper) Report(gcmd printerpkg.Command) {
	statusItems := []string{}
	self.params.get_status().Range(func(key string, value interface{}) bool {
		statusItems = append(statusItems, fmt.Sprintf("%s_%s:%s", key, self.axis, value))
		return true
	})
	gcmd.RespondInfo(strings.Join(statusItems, " "), true)
}

type InputShaper struct {
	printer                 printerpkg.ModulePrinter
	toolhead                inputShaperToolhead
	shapers                 []*AxisInputShaper
	stepper_kinematics      []interface{}
	orig_stepper_kinematics []interface{}
	old_delay               float64
}

func NewInputShaper(config printerpkg.ModuleConfig) *InputShaper {
	self := &InputShaper{}
	self.printer = config.Printer()
	self.printer.RegisterEventHandler("project:connect", self.Connect)
	self.shapers = []*AxisInputShaper{NewAxisInputShaper("x", config), NewAxisInputShaper("y", config)}
	self.stepper_kinematics = nil
	self.orig_stepper_kinematics = nil
	self.printer.GCode().RegisterCommand("SET_INPUT_SHAPER", self.Cmd_SET_INPUT_SHAPER, false, Cmd_SET_INPUT_SHAPER_help)
	return self
}

func (self *InputShaper) Get_shapers() []*AxisInputShaper {
	return self.shapers
}

func (self *InputShaper) Connect(_ []interface{}) error {
	self.toolhead = requireInputShaperToolhead(self.printer)
	kinematics := requireInputShaperKinematics(self.toolhead)
	for _, stepperObj := range kinematics.Get_steppers() {
		stepper, ok := stepperObj.(inputShaperStepper)
		if !ok {
			panic(fmt.Sprintf("stepper does not implement inputShaperStepper: %T", stepperObj))
		}
		sk := chelper.Input_shaper_alloc()
		origSK := stepper.Set_stepper_kinematics(sk)
		if chelper.Input_shaper_set_sk(sk, origSK) < 0 {
			stepper.Set_stepper_kinematics(origSK)
			continue
		}
		self.stepper_kinematics = append(self.stepper_kinematics, sk)
		self.orig_stepper_kinematics = append(self.orig_stepper_kinematics, origSK)
	}
	self.old_delay = 0.
	self._update_input_shaping(errors.New("printer config_error"))
	return nil
}

func (self *InputShaper) _update_input_shaping(err error) {
	self.toolhead.Flush_step_generation()
	new_delay := 0.
	for _, shaper := range self.shapers {
		if window := shaper.Get_step_generation_window(); window > new_delay {
			new_delay = window
		}
	}
	self.toolhead.Note_step_generation_scan_time(new_delay, self.old_delay)
	failed := []*AxisInputShaper{}
	for _, sk := range self.stepper_kinematics {
		for _, shaper := range self.shapers {
			if self.contains(failed, shaper) {
				continue
			}
			if !shaper.Set_shaper_kinematics(sk) {
				failed = append(failed, shaper)
			}
		}
	}
	if len(failed) > 0 {
		names := make([]string, 0, len(failed))
		for _, shaper := range failed {
			names = append(names, shaper.Get_name())
		}
		if err == nil {
			err = errors.New("printer command error")
		}
		panic(fmt.Errorf("Failed to configure shaper(s) %s with given parameters, error: %w", strings.Join(names, ", "), err))
	}
}

func (self *InputShaper) contains(elems []*AxisInputShaper, v *AxisInputShaper) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

func (self *InputShaper) Disable_shaping() {
	for _, shaper := range self.shapers {
		shaper.disable_shaping()
	}
	self._update_input_shaping(nil)
}

func (self *InputShaper) Enable_shaping() {
	for _, shaper := range self.shapers {
		shaper.Enable_shaping()
	}
	self._update_input_shaping(nil)
}

const Cmd_SET_INPUT_SHAPER_help = "Set cartesian parameters for input shaper"

func (self *InputShaper) Cmd_SET_INPUT_SHAPER(gcmd printerpkg.Command) error {
	updated := false
	for _, shaper := range self.shapers {
		if shaper.Update(gcmd) {
			updated = true
		}
	}
	if updated {
		self._update_input_shaping(nil)
	}
	for _, shaper := range self.shapers {
		shaper.Report(gcmd)
	}
	return nil
}

func LoadConfigInputShaper(config printerpkg.ModuleConfig) interface{} {
	return NewInputShaper(config)
}
