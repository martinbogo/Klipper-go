package tmc

import (
	"goklipper/common/utils/cast"
	"goklipper/common/value"
	"strings"
)

type VirtualPinConfig interface {
	Get_name() string
	Get(option string, default1 interface{}, noteValid bool) interface{}
}

type VirtualPinHelperCore struct {
	fields       *FieldHelper
	diagPin      interface{}
	diagPinField interface{}
	enPWM        bool
	pwmthrs      int64
	coolthrs     int64
}

func NewVirtualPinHelperCore(config VirtualPinConfig, mcuTMC FieldCarrier) *VirtualPinHelperCore {
	self := &VirtualPinHelperCore{fields: mcuTMC.Get_fields()}
	if value.IsNotNone(self.fields.Lookup_register("diag0_stall", nil)) {
		if value.IsNotNone(config.Get("diag0_pin", value.None, true)) {
			self.diagPin = config.Get("diag0_pin", value.None, true)
			self.diagPinField = "diag0_stall"
		} else {
			self.diagPin = config.Get("diag1_pin", value.None, true)
			self.diagPinField = "diag1_stall"
		}
	} else {
		self.diagPin = config.Get("diag_pin", value.None, true)
		self.diagPinField = value.None
	}
	return self
}

func (self *VirtualPinHelperCore) ChipName(config VirtualPinConfig) string {
	nameParts := strings.Split(config.Get_name(), " ")
	return nameParts[0] + "_" + nameParts[len(nameParts)-1]
}

func (self *VirtualPinHelperCore) DiagPin() interface{} {
	return self.diagPin
}

func (self *VirtualPinHelperCore) BeginHoming(mcuTMC RegisterAccess, tcoolthrsIfZero int64) error {
	self.pwmthrs = self.fields.Get_field("tpwmthrs", value.None, nil)
	self.coolthrs = self.fields.Get_field("tcoolthrs", value.None, nil)

	reg := self.fields.Lookup_register("en_pwm_mode", value.None)
	var val int64
	if value.IsNone(reg) {
		self.enPWM = self.fields.Get_field("en_spreadcycle", value.None, nil) == 0
		tpVal := self.fields.Set_field("tpwmthrs", 0, value.None, nil)
		if err := mcuTMC.Set_register("TPWMTHRS", tpVal, nil); err != nil {
			return err
		}
		val = self.fields.Set_field("en_spreadcycle", 0, value.None, nil)
	} else {
		self.enPWM = self.fields.Get_field("en_pwm_mode", value.None, nil) == 0
		self.fields.Set_field("en_pwm_mode", 0, value.None, nil)
		val = self.fields.Set_field(cast.ToString(self.diagPinField), 1, value.None, nil)
	}
	if err := mcuTMC.Set_register("GCONF", val, nil); err != nil {
		return err
	}
	if self.coolthrs == 0 {
		tcVal := self.fields.Set_field("tcoolthrs", tcoolthrsIfZero, value.None, nil)
		if err := mcuTMC.Set_register("TCOOLTHRS", tcVal, nil); err != nil {
			return err
		}
	}
	return nil
}

func (self *VirtualPinHelperCore) BeginMoveHoming(mcuTMC RegisterAccess) error {
	tcVal := self.fields.Set_field("tcoolthrs", 0, value.None, nil)
	if err := mcuTMC.Set_register("TCOOLTHRS", tcVal, nil); err != nil {
		return err
	}
	return self.BeginHoming(mcuTMC, 500)
}

func (self *VirtualPinHelperCore) EndHoming(mcuTMC RegisterAccess, printTime *float64) error {
	var val int64
	reg := self.fields.Lookup_register("en_pwm_mode", value.None)
	if value.IsNone(reg) {
		tpVal := self.fields.Set_field("tpwmthrs", self.pwmthrs, value.None, nil)
		if err := mcuTMC.Set_register("TPWMTHRS", tpVal, printTime); err != nil {
			return err
		}
		val = self.fields.Set_field("en_spreadcycle", self.enPWM, value.None, nil)
	} else {
		self.fields.Set_field("en_pwm_mode", self.enPWM, value.None, nil)
		val = self.fields.Set_field(cast.ToString(self.diagPinField), 0, value.None, nil)
	}
	if err := mcuTMC.Set_register("GCONF", val, printTime); err != nil {
		return err
	}
	tcVal := self.fields.Set_field("tcoolthrs", 0, value.None, nil)
	return mcuTMC.Set_register("TCOOLTHRS", tcVal, nil)
}
