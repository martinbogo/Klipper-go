package tmc

import (
	"fmt"
	"goklipper/common/utils/cast"
	"goklipper/common/value"
)

const TMC2130TMCFrequency = 13200000.

var TMC2130Registers = map[string]int64{
	"GCONF": 0x00, "GSTAT": 0x01, "IOIN": 0x04, "IHOLD_IRUN": 0x10,
	"TPOWERDOWN": 0x11, "TSTEP": 0x12, "TPWMTHRS": 0x13, "TCOOLTHRS": 0x14,
	"THIGH": 0x15, "XDIRECT": 0x2d, "MSLUT0": 0x60, "MSLUTSEL": 0x68,
	"MSLUTSTART": 0x69, "MSCNT": 0x6a, "MSCURACT": 0x6b, "CHOPCONF": 0x6c,
	"COOLCONF": 0x6d, "DCCTRL": 0x6e, "DRV_STATUS": 0x6f, "PWMCONF": 0x70,
	"PWM_SCALE": 0x71, "ENCM_CTRL": 0x72, "LOST_STEPS": 0x73,
}

var TMC2130ReadRegisters = []string{
	"GCONF", "GSTAT", "IOIN", "TSTEP", "XDIRECT", "MSCNT", "MSCURACT",
	"CHOPCONF", "DRV_STATUS", "PWM_SCALE", "LOST_STEPS",
}

var TMC2130Fields = map[string]map[string]int64{
	"GCONF": {
		"i_scale_analog": 1 << 0, "internal_rsense": 1 << 1, "en_pwm_mode": 1 << 2,
		"enc_commutation": 1 << 3, "shaft": 1 << 4, "diag0_error": 1 << 5,
		"diag0_otpw": 1 << 6, "diag0_stall": 1 << 7, "diag1_stall": 1 << 8,
		"diag1_index": 1 << 9, "diag1_onstate": 1 << 10, "diag1_steps_skipped": 1 << 11,
		"diag0_int_pushpull": 1 << 12, "diag1_pushpull": 1 << 13,
		"small_hysteresis": 1 << 14, "stop_enable": 1 << 15, "direct_mode": 1 << 16,
		"test_mode": 1 << 17,
	},
	"GSTAT": {"reset": 1 << 0, "drv_err": 1 << 1, "uv_cp": 1 << 2},
	"IOIN": {
		"step": 1 << 0, "dir": 1 << 1, "dcen_cfg4": 1 << 2, "dcin_cfg5": 1 << 3,
		"drv_enn_cfg6": 1 << 4, "dco": 1 << 5, "version": 0xff << 24,
	},
	"IHOLD_IRUN": {
		"ihold": 0x1f << 0, "irun": 0x1f << 8, "iholddelay": 0x0f << 16,
	},
	"TPOWERDOWN": {"tpowerdown": 0xff},
	"TSTEP":      {"tstep": 0xfffff},
	"TPWMTHRS":   {"tpwmthrs": 0xfffff},
	"TCOOLTHRS":  {"tcoolthrs": 0xfffff},
	"THIGH":      {"thigh": 0xfffff},
	"MSLUT0":     {"mslut0": 0xffffffff},
	"MSLUT1":     {"mslut1": 0xffffffff},
	"MSLUT2":     {"mslut2": 0xffffffff},
	"MSLUT3":     {"mslut3": 0xffffffff},
	"MSLUT4":     {"mslut4": 0xffffffff},
	"MSLUT5":     {"mslut5": 0xffffffff},
	"MSLUT6":     {"mslut6": 0xffffffff},
	"MSLUT7":     {"mslut7": 0xffffffff},
	"MSLUTSEL": {
		"x3": 0xFF << 24,
		"x2": 0xFF << 16,
		"x1": 0xFF << 8,
		"w3": 0x03 << 6,
		"w2": 0x03 << 4,
		"w1": 0x03 << 2,
		"w0": 0x03 << 0,
	},
	"MSLUTSTART": {
		"start_sin":   0xFF << 0,
		"start_sin90": 0xFF << 16,
	},
	"MSCNT":    {"mscnt": 0x3ff},
	"MSCURACT": {"cur_a": 0x1ff, "cur_b": 0x1ff << 16},
	"CHOPCONF": {
		"toff": 0x0f, "hstrt": 0x07 << 4, "hend": 0x0f << 7, "fd3": 1 << 11,
		"disfdcc": 1 << 12, "rndtf": 1 << 13, "chm": 1 << 14, "tbl": 0x03 << 15,
		"vsense": 1 << 17, "vhighfs": 1 << 18, "vhighchm": 1 << 19, "sync": 0x0f << 20,
		"mres": 0x0f << 24, "intpol": 1 << 28, "dedge": 1 << 29, "diss2g": 1 << 30,
	},
	"COOLCONF": {
		"semin": 0x0f, "seup": 0x03 << 5, "semax": 0x0f << 8, "sedn": 0x03 << 13,
		"seimin": 1 << 15, "sgt": 0x7f << 16, "sfilt": 1 << 24,
	},
	"DRV_STATUS": {
		"sg_result": 0x3ff, "fsactive": 1 << 15, "cs_actual": 0x1f << 16,
		"stallguard": 1 << 24, "ot": 1 << 25, "otpw": 1 << 26, "s2ga": 1 << 27,
		"s2gb": 1 << 28, "ola": 1 << 29, "olb": 1 << 30, "stst": 1 << 31,
	},
	"PWMCONF": {
		"pwm_ampl": 0xff, "pwm_grad": 0xff << 8, "pwm_freq": 0x03 << 16,
		"pwm_autoscale": 1 << 18, "pwm_symmetric": 1 << 19, "freewheel": 0x03 << 20,
	},
	"PWM_SCALE":  {"pwm_scale": 0xff},
	"LOST_STEPS": {"lost_steps": 0xfffff},
}

var TMC2130SignedFields = []string{"cur_a", "cur_b", "sgt"}

var TMC2130FieldFormatters = map[string]func(interface{}) string{
	"i_scale_analog": func(v interface{}) string {
		if value.True(v) {
			return "1(ExtVREF)"
		}
		return ""
	},
	"shaft": func(v interface{}) string {
		if value.True(v) {
			return "1(Reverse)"
		}
		return ""
	},
	"reset": func(v interface{}) string {
		if value.True(v) {
			return "1(Reset)"
		}
		return ""
	},
	"drv_err": func(v interface{}) string {
		if value.True(v) {
			return "1(ErrorShutdown!)"
		}
		return ""
	},
	"uv_cp": func(v interface{}) string {
		if value.True(v) {
			return "1(Undervoltage!)"
		}
		return ""
	},
	"version": func(v interface{}) string { return fmt.Sprintf("%#x", v) },
	"mres":    func(v interface{}) string { return fmt.Sprintf("%d(%dusteps)", v, 0x100>>cast.ToInt(v)) },
	"otpw": func(v interface{}) string {
		if value.True(v) {
			return "1(OvertempWarning!)"
		}
		return ""
	},
	"ot": func(v interface{}) string {
		if value.True(v) {
			return "1(OvertempError!)"
		}
		return ""
	},
	"s2ga": func(v interface{}) string {
		if value.True(v) {
			return "1(ShortToGND_A!)"
		}
		return ""
	},
	"s2gb": func(v interface{}) string {
		if value.True(v) {
			return "1(ShortToGND_B!)"
		}
		return ""
	},
	"ola": func(v interface{}) string {
		if value.True(v) {
			return "1(OpenLoad_A!)"
		}
		return ""
	},
	"olb": func(v interface{}) string {
		if value.True(v) {
			return "1(OpenLoad_B!)"
		}
		return ""
	},
	"cs_actual": func(v interface{}) string {
		if value.True(v) {
			return fmt.Sprintf("%d", v)
		}
		return "0(Reset?)"
	},
}

func ConfigureTMC2130(config ConfigFieldSource, fields *FieldHelper) {
	setConfigField := fields.Set_config_field
	setConfigField(config, "toff", 4)
	setConfigField(config, "hstrt", 0)
	setConfigField(config, "hend", 7)
	setConfigField(config, "tbl", 1)
	setConfigField(config, "vhighfs", 0)
	setConfigField(config, "vhighchm", 0)
	setConfigField(config, "semin", 0)
	setConfigField(config, "seup", 0)
	setConfigField(config, "semax", 0)
	setConfigField(config, "sedn", 0)
	setConfigField(config, "seimin", 0)
	setConfigField(config, "sgt", 0)
	setConfigField(config, "sfilt", 0)
	setConfigField(config, "iholddelay", 8)
	setConfigField(config, "pwm_ampl", 128)
	setConfigField(config, "pwm_grad", 4)
	setConfigField(config, "pwm_freq", 1)
	setConfigField(config, "pwm_autoscale", true)
	setConfigField(config, "freewheel", 0)
	setConfigField(config, "tpowerdown", 0)
}
