package tmc

import (
	"fmt"
	"goklipper/common/value"
)

const TMC2240TMCFrequency = 12500000.

var TMC2240Registers = map[string]int64{
	"GCONF":           0x00,
	"GSTAT":           0x01,
	"IFCNT":           0x02,
	"NODECONF":        0x03,
	"IOIN":            0x04,
	"DRV_CONF":        0x0A,
	"GLOBALSCALER":    0x0B,
	"IHOLD_IRUN":      0x10,
	"TPOWERDOWN":      0x11,
	"TSTEP":           0x12,
	"TPWMTHRS":        0x13,
	"TCOOLTHRS":       0x14,
	"THIGH":           0x15,
	"DIRECT_MODE":     0x2D,
	"ENCMODE":         0x38,
	"X_ENC":           0x39,
	"ENC_CONST":       0x3A,
	"ENC_STATUS":      0x3B,
	"ENC_LATCH":       0x3C,
	"ADC_VSUPPLY_AIN": 0x50,
	"ADC_TEMP":        0x51,
	"OTW_OV_VTH":      0x52,
	"MSLUT0":          0x60,
	"MSLUT1":          0x61,
	"MSLUT2":          0x62,
	"MSLUT3":          0x63,
	"MSLUT4":          0x64,
	"MSLUT5":          0x65,
	"MSLUT6":          0x66,
	"MSLUT7":          0x67,
	"MSLUTSEL":        0x68,
	"MSLUTSTART":      0x69,
	"MSCNT":           0x6A,
	"MSCURACT":        0x6B,
	"CHOPCONF":        0x6C,
	"COOLCONF":        0x6D,
	"DRV_STATUS":      0x6F,
	"PWMCONF":         0x70,
	"PWM_SCALE":       0x71,
	"PWM_AUTO":        0x72,
	"SG4_THRS":        0x74,
	"SG4_RESULT":      0x75,
	"SG4_IND":         0x76,
}

var TMC2240ReadRegisters = []string{
	"GCONF", "GSTAT", "IOIN", "DRV_CONF", "GLOBALSCALER", "IHOLD_IRUN",
	"TPOWERDOWN", "TSTEP", "TPWMTHRS", "TCOOLTHRS", "THIGH", "ADC_VSUPPLY_AIN",
	"ADC_TEMP", "MSCNT", "MSCURACT", "CHOPCONF", "COOLCONF", "DRV_STATUS",
	"PWMCONF", "PWM_SCALE", "PWM_AUTO", "SG4_THRS", "SG4_RESULT", "SG4_IND",
}

var TMC2240Fields = map[string]map[string]int64{
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
		"csactual":   0x1F << 16,
		"stallguard": 0x01 << 24,
		"ot":         0x01 << 25,
		"otpw":       0x01 << 26,
		"s2ga":       0x01 << 27,
		"s2gb":       0x01 << 28,
		"ola":        0x01 << 29,
		"olb":        0x01 << 30,
		"stst":       0x01 << 31,
	},
	"GCONF": {
		"faststandstill":   0x01 << 1,
		"en_pwm_mode":      0x01 << 2,
		"multistep_filt":   0x01 << 3,
		"shaft":            0x01 << 4,
		"diag0_error":      0x01 << 5,
		"diag0_otpw":       0x01 << 6,
		"diag0_stall":      0x01 << 7,
		"diag1_stall":      0x01 << 8,
		"diag1_index":      0x01 << 9,
		"diag1_onstate":    0x01 << 10,
		"diag0_pushpull":   0x01 << 12,
		"diag1_pushpull":   0x01 << 13,
		"small_hysteresis": 0x01 << 14,
		"stop_enable":      0x01 << 15,
		"direct_mode":      0x01 << 16,
	},
	"GSTAT": {
		"reset":          0x01 << 0,
		"drv_err":        0x01 << 1,
		"uv_cp":          0x01 << 2,
		"register_reset": 0x01 << 3,
		"vm_uvlo":        0x01 << 4,
	},
	"GLOBALSCALER": {
		"globalscaler": 0xFF << 0,
	},
	"IHOLD_IRUN": {
		"ihold":      0x1F << 0,
		"irun":       0x1F << 8,
		"iholddelay": 0x0F << 16,
		"irundelay":  0x0F << 24,
	},
	"IOIN": {
		"step":        0x01 << 0,
		"dir":         0x01 << 1,
		"encb":        0x01 << 2,
		"enca":        0x01 << 3,
		"drv_enn":     0x01 << 4,
		"encn":        0x01 << 5,
		"uart_en":     0x01 << 6,
		"comp_a":      0x01 << 8,
		"comp_b":      0x01 << 9,
		"comp_a1_a2":  0x01 << 10,
		"comp_b1_b2":  0x01 << 11,
		"output":      0x01 << 12,
		"ext_res_det": 0x01 << 13,
		"ext_clk":     0x01 << 14,
		"adc_err":     0x01 << 15,
		"silicon_rv":  0x07 << 16,
		"version":     0xFF << 24,
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
		"start_sin":    0xFF << 0,
		"start_sin90":  0xFF << 16,
		"offset_sin90": 0xFF << 24,
	},
	"MSCNT":    {"mscnt": 0x3ff << 0},
	"MSCURACT": {"cur_a": 0x1ff << 0, "cur_b": 0x1ff << 16},
	"PWM_AUTO": {"pwm_ofs_auto": 0xff << 0, "pwm_grad_auto": 0xff << 16},
	"PWMCONF": {
		"pwm_ofs":            0xFF << 0,
		"pwm_grad":           0xFF << 8,
		"pwm_freq":           0x03 << 16,
		"pwm_autoscale":      0x01 << 18,
		"pwm_autograd":       0x01 << 19,
		"freewheel":          0x03 << 20,
		"pwm_meas_sd_enable": 0x01 << 22,
		"pwm_dis_reg_stst":   0x01 << 23,
		"pwm_reg":            0x0F << 24,
		"pwm_lim":            0x0F << 28,
	},
	"PWM_SCALE":       {"pwm_scale_sum": 0x3ff << 0, "pwm_scale_auto": 0x1ff << 16},
	"TPOWERDOWN":      {"tpowerdown": 0xff << 0},
	"TPWMTHRS":        {"tpwmthrs": 0xfffff << 0},
	"TCOOLTHRS":       {"tcoolthrs": 0xfffff << 0},
	"TSTEP":           {"tstep": 0xfffff << 0},
	"THIGH":           {"thigh": 0xfffff << 0},
	"DRV_CONF":        {"current_range": 0x03 << 0, "slope_control": 0x03 << 4},
	"ADC_VSUPPLY_AIN": {"adc_vsupply": 0x1fff << 0, "adc_ain": 0x1fff << 16},
	"ADC_TEMP":        {"adc_temp": 0x1fff << 0},
	"OTW_OV_VTH":      {"overvoltage_vth": 0x1fff << 0, "overtempprewarning_vth": 0x1fff << 16},
	"SG4_THRS":        {"sg4_thrs": 0xFF << 0, "sg4_filt_en": 0x01 << 8, "sg4_angle_offset": 0x01 << 9},
	"SG4_RESULT":      {"sg4_result": 0x3FF << 0},
	"SG4_IND":         {"sg4_ind_0": 0xFF << 0, "sg4_ind_1": 0xFF << 8, "sg4_ind_2": 0xFF << 16, "sg4_ind_3": 0xFF << 24},
}

var TMC2240SignedFields = []string{"cur_a", "cur_b", "sgt", "pwm_scale_auto", "offset_sin90"}

var TMC2240FieldFormatters = map[string]func(interface{}) string{
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
	"adc_temp": func(v interface{}) string {
		return fmt.Sprintf("0x%04x(%.1fC)", v, float64(v.(int64)-2038)/7.7)
	},
}

func ConfigureTMC2240(config ConfigFieldSource, fields *FieldHelper) {
	fields.Set_field("multistep_filt", true, nil, nil)
	fields.Set_config_field(config, "offset_sin90", 0)

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
	setConfigField(config, "irundelay", 4)
	setConfigField(config, "pwm_ofs", 29)
	setConfigField(config, "pwm_grad", 0)
	setConfigField(config, "pwm_freq", 0)
	setConfigField(config, "pwm_autoscale", true)
	setConfigField(config, "pwm_autograd", true)
	setConfigField(config, "freewheel", 0)
	setConfigField(config, "pwm_reg", 4)
	setConfigField(config, "pwm_lim", 12)
	setConfigField(config, "tpowerdown", 10)
	setConfigField(config, "sg4_thrs", 0)
	setConfigField(config, "sg4_angle_offset", true)
}
