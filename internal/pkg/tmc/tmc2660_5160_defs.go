package tmc

import (
	"fmt"
	"goklipper/common/utils/cast"
	"goklipper/common/value"
)

const TMC5160TMCFrequency = 12000000.

var TMC2660Registers = map[string]int64{
	"DRVCONF": 0xE, "SGCSCONF": 0xC, "SMARTEN": 0xA,
	"CHOPCONF": 0x8, "DRVCTRL": 0x0,
}

var TMC2660ReadRegisters = []string{"READRSP@RDSEL0", "READRSP@RDSEL1", "READRSP@RDSEL2"}

var TMC2660Fields = map[string]map[string]int64{
	"DRVCTRL": {
		"mres":   0x0f,
		"dedge":  0x01 << 8,
		"intpol": 0x01 << 9,
	},
	"CHOPCONF": {
		"toff":  0x0f,
		"hstrt": 0x7 << 4,
		"hend":  0x0f << 7,
		"hdec":  0x03 << 11,
		"rndtf": 0x01 << 13,
		"chm":   0x01 << 14,
		"tbl":   0x03 << 15,
	},
	"SMARTEN": {
		"semin":  0x0f,
		"seup":   0x03 << 5,
		"semax":  0x0f << 8,
		"sedn":   0x03 << 13,
		"seimin": 0x01 << 15,
	},
	"SGCSCONF": {
		"cs":    0x1f,
		"sgt":   0x7F << 8,
		"sfilt": 0x01 << 16,
	},
	"DRVCONF": {
		"rdsel":  0x03 << 4,
		"vsense": 0x01 << 6,
		"sdoff":  0x01 << 7,
		"ts2g":   0x03 << 8,
		"diss2g": 0x01 << 10,
		"slpl":   0x03 << 12,
		"slph":   0x03 << 14,
		"tst":    0x01 << 16,
	},
	"READRSP@RDSEL0": {
		"stallguard": 0x01 << 4,
		"ot":         0x01 << 5,
		"otpw":       0x01 << 6,
		"s2ga":       0x01 << 7,
		"s2gb":       0x01 << 8,
		"ola":        0x01 << 9,
		"olb":        0x01 << 10,
		"stst":       0x01 << 11,
		"mstep":      0x3ff << 14,
	},
	"READRSP@RDSEL1": {
		"stallguard": 0x01 << 4,
		"ot":         0x01 << 5,
		"otpw":       0x01 << 6,
		"s2ga":       0x01 << 7,
		"s2gb":       0x01 << 8,
		"ola":        0x01 << 9,
		"olb":        0x01 << 10,
		"stst":       0x01 << 11,
		"sg_result":  0x3ff << 14,
	},
	"READRSP@RDSEL2": {
		"stallguard":       0x01 << 4,
		"ot":               0x01 << 5,
		"otpw":             0x01 << 6,
		"s2ga":             0x01 << 7,
		"s2gb":             0x01 << 8,
		"ola":              0x01 << 9,
		"olb":              0x01 << 10,
		"stst":             0x01 << 11,
		"se":               0x1f << 14,
		"sg_result@rdsel2": 0x1f << 19,
	},
}

var TMC2660SignedFields = []string{"sgt"}

var TMC2660FieldFormatters = map[string]func(interface{}) string{
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
	"chm": func(v interface{}) string {
		if value.True(v) {
			return "1(constant toff)"
		}
		return "0(spreadCycle)"
	},
	"vsense": func(v interface{}) string {
		if value.True(v) {
			return "1(165mV)"
		}
		return "0(305mV)"
	},
	"sdoff": func(v interface{}) string {
		if value.True(v) {
			return "1(Step/Dir disabled!)"
		}
		return ""
	},
	"diss2g": func(v interface{}) string {
		if value.True(v) {
			return "1(Short to GND disabled!)"
		}
		return ""
	},
	"se": func(v interface{}) string {
		if value.True(v) {
			return fmt.Sprintf("%d", v)
		}
		return "0(Reset?)"
	},
}

func ConfigureTMC2660(config ConfigFieldSource, fields *FieldHelper) {
	fields.Set_field("sdoff", 0, nil, nil)
	setConfigField := fields.Set_config_field
	setConfigField(config, "tbl", 2)
	setConfigField(config, "rndtf", 0)
	setConfigField(config, "hdec", 0)
	setConfigField(config, "chm", 0)
	setConfigField(config, "hend", 3)
	setConfigField(config, "hstrt", 3)
	setConfigField(config, "toff", 4)
	if value.False(fields.Get_field("chm", 0, nil)) {
		if (fields.Get_field("hstrt", 0, nil) + fields.Get_field("hend", 0, nil)) > 15 {
			panic("driver_HEND + driver_HSTRT must be <= 15")
		}
	}
	setConfigField(config, "seimin", 0)
	setConfigField(config, "sedn", 0)
	setConfigField(config, "semax", 0)
	setConfigField(config, "seup", 0)
	setConfigField(config, "semin", 0)
	setConfigField(config, "sfilt", 0)
	setConfigField(config, "sgt", 0)
	setConfigField(config, "slph", 0)
	setConfigField(config, "slpl", 0)
	setConfigField(config, "diss2g", 0)
	setConfigField(config, "ts2g", 3)
}

var TMC5160Registers = map[string]int64{
	"GCONF":         0x00,
	"GSTAT":         0x01,
	"IFCNT":         0x02,
	"SLAVECONF":     0x03,
	"IOIN":          0x04,
	"X_COMPARE":     0x05,
	"OTP_READ":      0x07,
	"FACTORY_CONF":  0x08,
	"SHORT_CONF":    0x09,
	"DRV_CONF":      0x0A,
	"GLOBALSCALER":  0x0B,
	"OFFSET_READ":   0x0C,
	"IHOLD_IRUN":    0x10,
	"TPOWERDOWN":    0x11,
	"TSTEP":         0x12,
	"TPWMTHRS":      0x13,
	"TCOOLTHRS":     0x14,
	"THIGH":         0x15,
	"RAMPMODE":      0x20,
	"XACTUAL":       0x21,
	"VACTUAL":       0x22,
	"VSTART":        0x23,
	"A1":            0x24,
	"V1":            0x25,
	"AMAX":          0x26,
	"VMAX":          0x27,
	"DMAX":          0x28,
	"D1":            0x2A,
	"VSTOP":         0x2B,
	"TZEROWAIT":     0x2C,
	"XTARGET":       0x2D,
	"VDCMIN":        0x33,
	"SW_MODE":       0x34,
	"RAMP_STAT":     0x35,
	"XLATCH":        0x36,
	"ENCMODE":       0x38,
	"X_ENC":         0x39,
	"ENC_CONST":     0x3A,
	"ENC_STATUS":    0x3B,
	"ENC_LATCH":     0x3C,
	"ENC_DEVIATION": 0x3D,
	"MSLUT0":        0x60,
	"MSLUT1":        0x61,
	"MSLUT2":        0x62,
	"MSLUT3":        0x63,
	"MSLUT4":        0x64,
	"MSLUT5":        0x65,
	"MSLUT6":        0x66,
	"MSLUT7":        0x67,
	"MSLUTSEL":      0x68,
	"MSLUTSTART":    0x69,
	"MSCNT":         0x6A,
	"MSCURACT":      0x6B,
	"CHOPCONF":      0x6C,
	"COOLCONF":      0x6D,
	"DCCTRL":        0x6E,
	"DRV_STATUS":    0x6F,
	"PWMCONF":       0x70,
	"PWM_SCALE":     0x71,
	"PWM_AUTO":      0x72,
	"LOST_STEPS":    0x73,
}

var TMC5160ReadRegisters = []string{
	"GCONF", "CHOPCONF", "GSTAT", "DRV_STATUS", "FACTORY_CONF", "IOIN",
	"LOST_STEPS", "MSCNT", "MSCURACT", "OTP_READ", "PWM_SCALE",
	"PWM_AUTO", "TSTEP",
}

var TMC5160Fields = map[string]map[string]int64{
	"COOLCONF": {
		"semin":  0x0F << 0,
		"seup":   0x03 << 5,
		"semax":  0x0F << 8,
		"sedn":   0x03 << 13,
		"seimin": 0x01 << 15,
		"sgt":    0x7F << 16,
		"sfilt":  0x01 << 24,
	},
	"CHOPCONF": {
		"toff":     0x0F << 0,
		"hstrt":    0x07 << 4,
		"hend":     0x0F << 7,
		"fd3":      0x01 << 11,
		"disfdcc":  0x01 << 12,
		"chm":      0x01 << 14,
		"tbl":      0x03 << 15,
		"vhighfs":  0x01 << 18,
		"vhighchm": 0x01 << 19,
		"tpfd":     0x0F << 20,
		"mres":     0x0F << 24,
		"intpol":   0x01 << 28,
		"dedge":    0x01 << 29,
		"diss2g":   0x01 << 30,
		"diss2vs":  0x01 << 31,
	},
	"DRV_STATUS": {
		"sg_result":  0x3FF << 0,
		"s2vsa":      0x01 << 12,
		"s2vsb":      0x01 << 13,
		"stealth":    0x01 << 14,
		"fsactive":   0x01 << 15,
		"csactual":   0xFF << 16,
		"stallguard": 0x01 << 24,
		"ot":         0x01 << 25,
		"otpw":       0x01 << 26,
		"s2ga":       0x01 << 27,
		"s2gb":       0x01 << 28,
		"ola":        0x01 << 29,
		"olb":        0x01 << 30,
		"stst":       0x01 << 31,
	},
	"FACTORY_CONF": {
		"factory_conf": 0x1F << 0,
	},
	"GCONF": {
		"recalibrate":            0x01 << 0,
		"faststandstill":         0x01 << 1,
		"en_pwm_mode":            0x01 << 2,
		"multistep_filt":         0x01 << 3,
		"shaft":                  0x01 << 4,
		"diag0_error":            0x01 << 5,
		"diag0_otpw":             0x01 << 6,
		"diag0_stall":            0x01 << 7,
		"diag1_stall":            0x01 << 8,
		"diag1_index":            0x01 << 9,
		"diag1_onstate":          0x01 << 10,
		"diag1_steps_skipped":    0x01 << 11,
		"diag0_int_pushpull":     0x01 << 12,
		"diag1_poscomp_pushpull": 0x01 << 13,
		"small_hysteresis":       0x01 << 14,
		"stop_enable":            0x01 << 15,
		"direct_mode":            0x01 << 16,
		"test_mode":              0x01 << 17,
	},
	"GSTAT": {
		"reset":   0x01 << 0,
		"drv_err": 0x01 << 1,
		"uv_cp":   0x01 << 2,
	},
	"GLOBALSCALER": {
		"globalscaler": 0xFF << 0,
	},
	"IHOLD_IRUN": {
		"ihold":      0x1F << 0,
		"irun":       0x1F << 8,
		"iholddelay": 0x0F << 16,
	},
	"IOIN": {
		"refl_step":      0x01 << 0,
		"refr_dir":       0x01 << 1,
		"encb_dcen_cfg4": 0x01 << 2,
		"enca_dcin_cfg5": 0x01 << 3,
		"drv_enn":        0x01 << 4,
		"enc_n_dco_cfg6": 0x01 << 5,
		"sd_mode":        0x01 << 6,
		"swcomp_in":      0x01 << 7,
		"version":        0xFF << 24,
	},
	"LOST_STEPS": {
		"lost_steps": 0xfffff << 0,
	},
	"MSLUT0": {"mslut0": 0xffffffff},
	"MSLUT1": {"mslut1": 0xffffffff},
	"MSLUT2": {"mslut2": 0xffffffff},
	"MSLUT3": {"mslut3": 0xffffffff},
	"MSLUT4": {"mslut4": 0xffffffff},
	"MSLUT5": {"mslut5": 0xffffffff},
	"MSLUT6": {"mslut6": 0xffffffff},
	"MSLUT7": {"mslut7": 0xffffffff},
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
	"MSCNT": {
		"mscnt": 0x3ff << 0,
	},
	"MSCURACT": {
		"cur_a": 0x1ff << 0,
		"cur_b": 0x1ff << 16,
	},
	"OTP_READ": {
		"otp_fclktrim": 0x1f << 0,
		"otp_s2_level": 0x01 << 5,
		"otp_bbm":      0x01 << 6,
		"otp_tbl":      0x01 << 7,
	},
	"PWM_AUTO": {
		"pwm_ofs_auto":  0xff << 0,
		"pwm_grad_auto": 0xff << 16,
	},
	"PWMCONF": {
		"pwm_ofs":       0xFF << 0,
		"pwm_grad":      0xFF << 8,
		"pwm_freq":      0x03 << 16,
		"pwm_autoscale": 0x01 << 18,
		"pwm_autograd":  0x01 << 19,
		"freewheel":     0x03 << 20,
		"pwm_reg":       0x0F << 24,
		"pwm_lim":       0x0F << 28,
	},
	"PWM_SCALE": {
		"pwm_scale_sum":  0xff << 0,
		"pwm_scale_auto": 0x1ff << 16,
	},
	"TPOWERDOWN": {
		"tpowerdown": 0xff << 0,
	},
	"TPWMTHRS": {
		"tpwmthrs": 0xfffff << 0,
	},
	"TCOOLTHRS": {
		"tcoolthrs": 0xfffff << 0,
	},
	"THIGH": {
		"thigh": 0xfffff << 0,
	},
	"TSTEP": {
		"tstep": 0xfffff << 0,
	},
}

var TMC5160SignedFields = []string{"cur_a", "cur_b", "sgt", "xactual", "vactual", "pwm_scale_auto"}

var TMC5160FieldFormatters = map[string]func(interface{}) string{
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
	"s2vsa": func(v interface{}) string {
		if value.True(v) {
			return "1(ShortToSupply_A!)"
		}
		return ""
	},
	"s2vsb": func(v interface{}) string {
		if value.True(v) {
			return "1(ShortToSupply_B!)"
		}
		return ""
	},
}

func ConfigureTMC5160(config ConfigFieldSource, fields *FieldHelper) {
	fields.Set_field("multistep_filt", true, nil, nil)
	setConfigField := fields.Set_config_field
	setConfigField(config, "toff", 3)
	setConfigField(config, "hstrt", 5)
	setConfigField(config, "hend", 2)
	setConfigField(config, "fd3", 0)
	setConfigField(config, "disfdcc", 0)
	setConfigField(config, "chm", 0)
	setConfigField(config, "tbl", 2)
	setConfigField(config, "vhighfs", 0)
	setConfigField(config, "vhighchm", 0)
	setConfigField(config, "tpfd", 4)
	setConfigField(config, "diss2g", 0)
	setConfigField(config, "diss2vs", 0)
	setConfigField(config, "semin", 0)
	setConfigField(config, "seup", 0)
	setConfigField(config, "semax", 0)
	setConfigField(config, "sedn", 0)
	setConfigField(config, "seimin", 0)
	setConfigField(config, "sgt", 0)
	setConfigField(config, "sfilt", 0)
	setConfigField(config, "iholddelay", 6)
	setConfigField(config, "pwm_ofs", 30)
	setConfigField(config, "pwm_grad", 0)
	setConfigField(config, "pwm_freq", 0)
	setConfigField(config, "pwm_autoscale", true)
	setConfigField(config, "pwm_autograd", true)
	setConfigField(config, "freewheel", 0)
	setConfigField(config, "pwm_reg", 4)
	setConfigField(config, "pwm_lim", 12)
	setConfigField(config, "tpowerdown", 10)
}
