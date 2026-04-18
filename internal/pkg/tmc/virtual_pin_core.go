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
	dirtyRegs    map[string]int64
	dirtyOrder   []string
	prevFields   map[string]int64
	prevOrder    []string
}

func NewVirtualPinHelperCore(config VirtualPinConfig, mcuTMC FieldCarrier) *VirtualPinHelperCore {
	self := &VirtualPinHelperCore{
		fields:     mcuTMC.Get_fields(),
		dirtyRegs:  map[string]int64{},
		prevFields: map[string]int64{},
	}
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

func (self *VirtualPinHelperCore) queueField(fieldName string, fieldValue interface{}) {
	regName := self.fields.Lookup_register(fieldName, value.None)
	if value.IsNone(regName) {
		return
	}
	if _, ok := self.prevFields[fieldName]; !ok {
		self.prevFields[fieldName] = self.fields.Get_field(fieldName, value.None, nil)
		self.prevOrder = append(self.prevOrder, fieldName)
	}
	resolvedRegName := cast.ToString(regName)
	regValue := self.fields.Set_field(fieldName, fieldValue, value.None, nil)
	if _, ok := self.dirtyRegs[resolvedRegName]; !ok {
		self.dirtyOrder = append(self.dirtyOrder, resolvedRegName)
	}
	self.dirtyRegs[resolvedRegName] = regValue
}

func (self *VirtualPinHelperCore) sendQueuedFields(mcuTMC RegisterAccess, printTime *float64) error {
	for _, regName := range self.dirtyOrder {
		if err := mcuTMC.Set_register(regName, self.dirtyRegs[regName], printTime); err != nil {
			return err
		}
	}
	self.dirtyRegs = map[string]int64{}
	self.dirtyOrder = nil
	return nil
}

func (self *VirtualPinHelperCore) BeginHoming(mcuTMC RegisterAccess, tcoolthrsIfZero int64) error {
	sg4Thrs := int64(0)
	if value.IsNotNone(self.fields.Lookup_register("sg4_thrs", value.None)) {
		sg4Thrs = self.fields.Get_field("sg4_thrs", value.None, nil)
	}

	reg := self.fields.Lookup_register("en_pwm_mode", value.None)
	if value.IsNone(reg) {
		self.queueField("tpwmthrs", 0)
		self.queueField("en_spreadcycle", 0)
	} else if sg4Thrs != 0 {
		self.queueField("en_pwm_mode", 1)
		self.queueField("tpwmthrs", 0)
		if value.IsNotNone(self.diagPinField) {
			self.queueField(cast.ToString(self.diagPinField), 1)
		}
	} else {
		self.queueField("en_pwm_mode", 0)
		if value.IsNotNone(self.diagPinField) {
			self.queueField(cast.ToString(self.diagPinField), 1)
		}
	}
	if self.fields.Get_field("tcoolthrs", value.None, nil) == 0 {
		self.queueField("tcoolthrs", tcoolthrsIfZero)
	}
	if value.IsNotNone(self.fields.Lookup_register("thigh", value.None)) {
		self.queueField("thigh", 0)
	}
	return self.sendQueuedFields(mcuTMC, nil)
}

func (self *VirtualPinHelperCore) BeginMoveHoming(mcuTMC RegisterAccess) error {
	self.queueField("tcoolthrs", 0)
	if err := self.sendQueuedFields(mcuTMC, nil); err != nil {
		return err
	}
	return self.BeginHoming(mcuTMC, 500)
}

func (self *VirtualPinHelperCore) EndHoming(mcuTMC RegisterAccess, printTime *float64) error {
	for _, fieldName := range self.prevOrder {
		self.queueField(fieldName, self.prevFields[fieldName])
	}
	if err := self.sendQueuedFields(mcuTMC, printTime); err != nil {
		return err
	}
	self.prevFields = map[string]int64{}
	self.prevOrder = nil
	return nil
}
