package bedmesh

import (
	"fmt"
	"reflect"

	"goklipper/common/utils/object"
	addonpkg "goklipper/internal/addon"
	printerpkg "goklipper/internal/pkg/printer"
	printpkg "goklipper/internal/print"
)

const cmdAdaptiveBedMeshCalibrateHelp = "Run adaptive bed mesh calibration using detected print area"

type AdaptiveBedMeshModule struct {
	gcode         printerpkg.GCodeRuntime
	excludeObject *addonpkg.ExcludeObjectModule
	printStats    *printpkg.PrintStatsModule
	bedMesh       interface{}
	moduleConfig  AdaptiveCalibrationModuleConfig
	debugMode     bool
}

type reflectiveAdaptiveModuleConfigSource struct {
	root reflect.Value
}

func LoadConfigAdaptiveBedMesh(config printerpkg.ModuleConfig) interface{} {
	module, err := NewAdaptiveBedMeshModule(config)
	if err != nil {
		panic(err)
	}
	return module
}

func NewAdaptiveBedMeshModule(config printerpkg.ModuleConfig) (*AdaptiveBedMeshModule, error) {
	printer := config.Printer()
	source, err := newReflectiveAdaptiveModuleConfigSource(config)
	if err != nil {
		return nil, err
	}
	moduleConfig, err := BuildAdaptiveCalibrationModuleConfig(source)
	if err != nil {
		return nil, err
	}
	self := &AdaptiveBedMeshModule{
		gcode:         printer.GCode(),
		excludeObject: mustLookupAdaptiveExcludeObject(printer, config.Name()),
		printStats:    mustLookupAdaptivePrintStats(printer, config.Name()),
		bedMesh:       mustLookupAdaptiveBedMesh(printer, config.Name()),
		moduleConfig:  moduleConfig,
		debugMode:     config.Bool("debug_mode", false),
	}
	self.gcode.RegisterCommand("ADAPTIVE_BED_MESH_CALIBRATE", self.cmdAdaptiveBedMeshCalibrate, false, cmdAdaptiveBedMeshCalibrateHelp)
	return self, nil
}

func mustLookupAdaptiveExcludeObject(printer printerpkg.ModulePrinter, sectionName string) *addonpkg.ExcludeObjectModule {
	obj := printer.LookupObject("exclude_object", object.Sentinel{})
	excludeObj, ok := obj.(*addonpkg.ExcludeObjectModule)
	if !ok || excludeObj == nil {
		panic(fmt.Sprintf("[adaptive_bed_mesh] requires [exclude_object] to be configured before %s", sectionName))
	}
	return excludeObj
}

func mustLookupAdaptivePrintStats(printer printerpkg.ModulePrinter, sectionName string) *printpkg.PrintStatsModule {
	obj := printer.LookupObject("print_stats", object.Sentinel{})
	printStats, ok := obj.(*printpkg.PrintStatsModule)
	if !ok || printStats == nil {
		panic(fmt.Sprintf("[adaptive_bed_mesh] requires [print_stats] to be configured before %s", sectionName))
	}
	return printStats
}

func mustLookupAdaptiveBedMesh(printer printerpkg.ModulePrinter, sectionName string) interface{} {
	obj := printer.LookupObject("bed_mesh", object.Sentinel{})
	if _, isSentinel := obj.(object.Sentinel); isSentinel || obj == nil {
		panic(fmt.Sprintf("[adaptive_bed_mesh] requires [bed_mesh] to be configured before %s", sectionName))
	}
	return obj
}

func newReflectiveAdaptiveModuleConfigSource(config printerpkg.ModuleConfig) (reflectiveAdaptiveModuleConfigSource, error) {
	root := reflect.ValueOf(config)
	if !root.IsValid() {
		return reflectiveAdaptiveModuleConfigSource{}, fmt.Errorf("[adaptive_bed_mesh] missing config wrapper")
	}
	for _, methodName := range []string{"Fileconfig", "Getsection", "Get_name", "Get", "Getfloatlist", "Getfloat", "Getint", "Getboolean"} {
		if !root.MethodByName(methodName).IsValid() {
			return reflectiveAdaptiveModuleConfigSource{}, fmt.Errorf("[adaptive_bed_mesh] config type %T does not expose %s", config, methodName)
		}
	}
	return reflectiveAdaptiveModuleConfigSource{root: root}, nil
}

func (self reflectiveAdaptiveModuleConfigSource) section(name string) reflect.Value {
	if name == "" {
		return self.root
	}
	return callAdaptiveReflectiveMethod(self.root, "Getsection", name)[0]
}

func (self reflectiveAdaptiveModuleConfigSource) fileConfig() reflect.Value {
	return callAdaptiveReflectiveMethod(self.root, "Fileconfig")[0]
}

func (self reflectiveAdaptiveModuleConfigSource) HasSection(section string) bool {
	return callAdaptiveReflectiveMethod(self.fileConfig(), "Has_section", section)[0].Bool()
}

func (self reflectiveAdaptiveModuleConfigSource) HasOption(section string, option string) bool {
	sectionName := callAdaptiveReflectiveMethod(self.section(section), "Get_name")[0].String()
	return callAdaptiveReflectiveMethod(self.fileConfig(), "Has_option", sectionName, option)[0].Bool()
}

func (self reflectiveAdaptiveModuleConfigSource) FloatSlice(section string, option string, fallback []float64, count int) []float64 {
	value := callAdaptiveReflectiveMethod(self.section(section), "Getfloatlist", option, fallback, ",", count, true)[0]
	if value.IsNil() {
		return nil
	}
	return value.Interface().([]float64)
}

func (self reflectiveAdaptiveModuleConfigSource) Float(section string, option string, fallback float64) float64 {
	return callAdaptiveReflectiveMethod(self.section(section), "Getfloat", option, fallback, 0.0, 0.0, 0.0, 0.0, true)[0].Float()
}

func (self reflectiveAdaptiveModuleConfigSource) Int(section string, option string, fallback int, minValue int) int {
	return int(callAdaptiveReflectiveMethod(self.section(section), "Getint", option, fallback, minValue, 0, true)[0].Int())
}

func (self reflectiveAdaptiveModuleConfigSource) Bool(section string, option string, fallback bool) bool {
	return callAdaptiveReflectiveMethod(self.section(section), "Getboolean", option, fallback, true)[0].Bool()
}

func (self reflectiveAdaptiveModuleConfigSource) String(section string, option string, fallback string) (string, error) {
	sectionValue := self.section(section)
	value := callAdaptiveReflectiveMethod(sectionValue, "Get", option, fallback, true)[0].Interface()
	str, ok := value.(string)
	if !ok {
		sectionName := callAdaptiveReflectiveMethod(sectionValue, "Get_name")[0].String()
		return "", fmt.Errorf("[adaptive_bed_mesh] %s.%s must be a string", sectionName, option)
	}
	return str, nil
}

func callAdaptiveReflectiveMethod(target reflect.Value, methodName string, args ...interface{}) []reflect.Value {
	method := target.MethodByName(methodName)
	if !method.IsValid() {
		panic(fmt.Sprintf("[adaptive_bed_mesh] missing reflective method %s on %T", methodName, target.Interface()))
	}
	callArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		paramType := method.Type().In(i)
		if arg == nil {
			callArgs[i] = reflect.Zero(paramType)
			continue
		}
		value := reflect.ValueOf(arg)
		if value.Type().AssignableTo(paramType) {
			callArgs[i] = value
			continue
		}
		if value.Type().ConvertibleTo(paramType) {
			callArgs[i] = value.Convert(paramType)
			continue
		}
		if paramType.Kind() == reflect.Interface && value.Type().Implements(paramType) {
			callArgs[i] = value
			continue
		}
		panic(fmt.Sprintf("[adaptive_bed_mesh] unable to pass %T to %s.%s", arg, target.Type(), methodName))
	}
	return method.Call(callArgs)
}

func setAdaptiveBedMeshZeroReference(bedMesh interface{}, value *Vec2) error {
	bedMeshValue := reflect.ValueOf(bedMesh)
	if !bedMeshValue.IsValid() || (bedMeshValue.Kind() == reflect.Pointer && bedMeshValue.IsNil()) {
		return fmt.Errorf("[adaptive_bed_mesh] missing bed_mesh runtime")
	}
	if bedMeshValue.Kind() != reflect.Pointer || bedMeshValue.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("[adaptive_bed_mesh] unsupported bed_mesh runtime %T", bedMesh)
	}
	field := bedMeshValue.Elem().FieldByName("Zero_ref_pos")
	if !field.IsValid() || !field.CanSet() || field.Type() != reflect.TypeOf([]float64(nil)) {
		return fmt.Errorf("[adaptive_bed_mesh] bed_mesh runtime %T does not expose writable Zero_ref_pos", bedMesh)
	}
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}
	field.Set(reflect.ValueOf([]float64{value.X, value.Y}))
	return nil
}

func (self *AdaptiveBedMeshModule) cmdAdaptiveBedMeshCalibrate(gcmd printerpkg.Command) (err error) {
	respond := func(msg string) {
		gcmd.RespondInfo("AdaptiveBedMesh: "+msg, true)
	}
	defer func() {
		if r := recover(); r != nil {
			recoveredErr := fmt.Errorf("panic recovered: %v", r)
			respond(recoveredErr.Error())
			if !self.debugMode {
				err = recoveredErr
			}
		}
	}()

	err = RunAdaptiveCalibrationModule(
		AdaptiveMeshAreaInput{
			AreaStart:      gcmd.String("AREA_START", ""),
			AreaEnd:        gcmd.String("AREA_END", ""),
			ExcludeObjects: self.excludeObject.Objects(),
			GCodeFilePath:  gcmd.String("GCODE_FILEPATH", ""),
			ActiveFilename: self.printStats.Filename(),
		},
		self.moduleConfig,
		AdaptiveCalibrationModuleRuntime{
			Log: respond,
			SetZeroReference: func(value *Vec2) {
				if setErr := setAdaptiveBedMeshZeroReference(self.bedMesh, value); setErr != nil {
					respond(setErr.Error())
				}
			},
			RunCommand: self.gcode.RunScriptFromCommand,
		},
	)
	if err != nil {
		respond(err.Error())
		if !self.debugMode {
			return err
		}
	}
	return nil
}
