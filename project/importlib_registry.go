package project

import (
	moduleinitpkg "goklipper/internal/pkg/moduleinit"
	printerpkg "goklipper/internal/pkg/printer"
)

func LoadMainModule() *printerpkg.ModuleRegistry {
	module := printerpkg.NewModuleRegistry()
	registerProjectModules(module)
	moduleinitpkg.RegisterInternalModules(module)
	return module
}

func registerConfigWrapperModule(module *printerpkg.ModuleRegistry, name string, init func(*ConfigWrapper) interface{}) {
	module.Register(name, func(section interface{}) interface{} {
		return init(section.(*ConfigWrapper))
	})
}

func registerProjectModules(module *printerpkg.ModuleRegistry) {
	for _, item := range []struct {
		name string
		init func(*ConfigWrapper) interface{}
	}{
		{name: "homing", init: Load_config_homing},
		{name: "probe", init: Load_config_probe},
		{name: "gcode_macro_1", init: Load_config_printer_gcode_macro},
		{name: "gcode_macro", init: Load_config_gcode_macro},
		{name: "bed_mesh", init: Load_config_bed_mesh},
		{name: "leviq3", init: Load_config_LeviQ3},
		{name: "adaptive_bed_mesh", init: Load_config_adaptive_bed_mesh},
		{name: "tmc2209", init: Load_config_TMC2209},
		{name: "tmc2240", init: Load_config_TMC2240},
		{name: "adxl345", init: Load_config_ADXL345},
		{name: "lis2dw12", init: Load_config_LIS2DW12},
		{name: "manual_stepper", init: Load_config_manual_stepper},
		{name: "manual_probe", init: Load_config_ManualProbe},
		{name: "ace", init: Load_config_ace},
		{name: "filament_hub", init: Load_config_filament_hub},
	} {
		registerConfigWrapperModule(module, item.name, item.init)
	}
}
