package moduleinit

import (
	addonpkg "goklipper/internal/addon"
	fanpkg "goklipper/internal/pkg/fan"
	gcodepkg "goklipper/internal/pkg/gcode"
	heaterpkg "goklipper/internal/pkg/heater"
	iopkg "goklipper/internal/pkg/io"
	mcupkg "goklipper/internal/pkg/mcu"
	motionpkg "goklipper/internal/pkg/motion"
	bedmeshpkg "goklipper/internal/pkg/motion/bed_mesh"
	probepkg "goklipper/internal/pkg/motion/probe"
	vibrationpkg "goklipper/internal/pkg/motion/vibration"
	printerpkg "goklipper/internal/pkg/printer"
	utilpkg "goklipper/internal/pkg/util"
	printpkg "goklipper/internal/print"
)

func BuildRegistry(registerProject func(*printerpkg.ModuleRegistry)) *printerpkg.ModuleRegistry {
	module := printerpkg.NewModuleRegistry()
	if registerProject != nil {
		registerProject(module)
	}
	RegisterInternalModules(module)
	return module
}

func RegisterInternalModules(module *printerpkg.ModuleRegistry) {
	for _, item := range []struct {
		name string
		init func(printerpkg.ModuleConfig) interface{}
	}{
		{name: "query_adc", init: utilpkg.LoadConfigQueryADC},
		{name: "query_endstops", init: probepkg.LoadConfigQueryEndstops},
		{name: "force_move", init: motionpkg.LoadConfigForceMove},
		{name: "buttons", init: iopkg.LoadConfigButtons},
		{name: "stepper_enable", init: mcupkg.LoadConfigStepperEnable},
		{name: "fan", init: fanpkg.LoadConfigFan},
		{name: "fan_generic", init: fanpkg.LoadConfigGenericFan},
		{name: "heater_fan extruder_fan", init: fanpkg.LoadConfigHeaterFan},
		{name: "controller_fan controller_fan", init: fanpkg.LoadConfigControllerFan},
		{name: "heaters", init: heaterpkg.LoadConfigHeaters},
		{name: "heater_generic", init: heaterpkg.LoadConfigGenericHeater},
		{name: "adc_temperature", init: heaterpkg.LoadConfigADCTemperature},
		{name: "thermistor", init: heaterpkg.LoadConfigThermistor},
		{name: "customLinear", init: heaterpkg.LoadConfigPrefixCustomLinear},
		{name: "heater_bed", init: heaterpkg.LoadConfigHeaterBed},
		{name: "homing_override", init: addonpkg.LoadConfigHomingOverride},
		{name: "print_stats", init: printpkg.LoadConfigPrintStats},
		{name: "idle_timeout", init: printpkg.LoadConfigIdleTimeout},
		{name: "exclude_object", init: addonpkg.LoadConfigExcludeObject},
		{name: "adaptive_bed_mesh", init: bedmeshpkg.LoadConfigAdaptiveBedMesh},
		{name: "safe_z_home", init: addonpkg.LoadConfigSafeZHoming},
		{name: "virtual_sdcard", init: addonpkg.LoadConfigVirtualSD},
		{name: "statistics", init: printerpkg.LoadConfigStatsModule},
		{name: "save_variables", init: addonpkg.LoadConfigSaveVariables},
		{name: "mcu_ota", init: addonpkg.LoadConfigMCUOTA},
		{name: "led_pin", init: addonpkg.LoadConfigLedDigitalOut},
		{name: "pause_resume", init: gcodepkg.LoadConfigPauseResume},
		{name: "gcode_macro", init: gcodepkg.LoadConfigGCodeMacro},
		{name: "gcode_move", init: gcodepkg.LoadConfigGCodeMove},
		{name: "input_shaper", init: vibrationpkg.LoadConfigInputShaper},
		{name: "resonance_tester", init: vibrationpkg.LoadConfigResonanceTester},
		{name: "firmware_retraction", init: gcodepkg.LoadConfigFirmwareRetraction},
		{name: "pid_calibrate", init: heaterpkg.LoadConfigPIDCalibrate},
		{name: "verify_heater", init: heaterpkg.LoadConfigVerifyHeater},
		{name: "gcode_arcs", init: gcodepkg.LoadConfigArcSupport},
		{name: "ds18b20", init: heaterpkg.LoadConfigDS18B20},
		{name: "cs1237", init: iopkg.LoadConfigCS1237},
		{name: "filament_tracker", init: iopkg.LoadConfigFilamentTracker},
		{name: "encoder_sensor", init: iopkg.LoadConfigPrefixEncoderSensor},
		{name: "filament_switch_sensor filament_sensor", init: iopkg.LoadConfigSwitchSensor},
		{name: "output_pin", init: iopkg.LoadConfigPrefixDigitalOut},
	} {
		module.Register(item.name, func(section interface{}) interface{} {
			return item.init(section.(printerpkg.ModuleConfig))
		})
	}
}
