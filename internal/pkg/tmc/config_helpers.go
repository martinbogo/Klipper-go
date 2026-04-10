package tmc

import "goklipper/common/value"

type FieldCarrier interface {
	Get_fields() *FieldHelper
}

func ApplyWaveTableDefaults(config ConfigFieldSource, mcuTMC FieldCarrier) {
	setConfigField := mcuTMC.Get_fields().Set_config_field
	setConfigField(config, "mslut0", int64(0xAAAAB554))
	setConfigField(config, "mslut1", int64(0x4A9554AA))
	setConfigField(config, "mslut2", int64(0x24492929))
	setConfigField(config, "mslut3", int64(0x10104222))
	setConfigField(config, "mslut4", int64(0xFBFFFFFF))
	setConfigField(config, "mslut5", int64(0xB5BB777D))
	setConfigField(config, "mslut6", int64(0x49295556))
	setConfigField(config, "mslut7", int64(0x00404222))
	setConfigField(config, "w0", 2)
	setConfigField(config, "w1", 1)
	setConfigField(config, "w2", 1)
	setConfigField(config, "w3", 1)
	setConfigField(config, "x1", 128)
	setConfigField(config, "x2", 255)
	setConfigField(config, "x3", 255)
	setConfigField(config, "start_sin", 0)
	setConfigField(config, "start_sin90", 247)
}

func ApplyMicrostepSettings(fields *FieldHelper, mres interface{}, interpolate bool) {
	fields.Set_field("mres", mres, value.None, nil)
	fields.Set_field("intpol", interpolate, value.None, nil)
}

func TMCtstepHelper(stepDist float64, mres int, tmcFreq, velocity float64) int {
	if velocity > 0. {
		shift := 1 << mres
		stepDist256 := stepDist / float64(shift)
		threshold := int(tmcFreq*stepDist256/velocity + .5)
		if threshold < 0 {
			return 0
		}
		if threshold > 0xfffff {
			return 0xfffff
		}
		return threshold
	}
	return 0xfffff
}

func ApplyStealthchop(fields *FieldHelper, tmcFreq float64, stepDist float64, velocity float64) {
	enPWMMode := false
	tpwmthrs := 0xfffff

	if velocity == velocity {
		enPWMMode = true
		mres := fields.Get_field("mres", value.None, nil)
		tpwmthrs = TMCtstepHelper(stepDist, int(mres), tmcFreq, velocity)
	}
	fields.Set_field("tpwmthrs", tpwmthrs, value.None, nil)

	reg := fields.Lookup_register("en_pwm_mode", value.None)
	if value.IsNone(reg) == false {
		fields.Set_field("en_pwm_mode", enPWMMode, value.None, nil)
	} else {
		fields.Set_field("en_spreadcycle", !enPWMMode, value.None, nil)
	}
}
