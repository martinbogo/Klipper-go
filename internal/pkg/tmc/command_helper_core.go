package tmc

import (
	"fmt"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/collections"
	"goklipper/common/value"
)

type RegisterAccess interface {
	Get_fields() *FieldHelper
	Get_register(string) (int64, error)
	Set_register(string, int64, *float64) error
}

type CurrentControl interface {
	Get_current() []float64
	Set_current(float64, float64, float64)
}

type CommandHelperCore struct {
	mcuTMC        RegisterAccess
	currentHelper CurrentControl
	fields        *FieldHelper
	readRegisters []string
	readTranslate func(string, int64) (string, int64)
	toff          interface{}
	phaseOffset   *int
}

func NewCommandHelperCore(mcuTMC RegisterAccess, currentHelper CurrentControl) *CommandHelperCore {
	return &CommandHelperCore{
		mcuTMC:        mcuTMC,
		currentHelper: currentHelper,
		fields:        mcuTMC.Get_fields(),
	}
}

func (self *CommandHelperCore) Fields() *FieldHelper {
	return self.fields
}

func (self *CommandHelperCore) InitRegisters(printTime *float64) error {
	for _, regName := range self.fields.orderedRegisterNames() {
		val := self.fields.Registers()[regName]
		if err := self.mcuTMC.Set_register(regName, cast.ToInt64(val), printTime); err != nil {
			return err
		}
	}
	return nil
}

func (self *CommandHelperCore) SetField(fieldName string, fieldValue int64, printTime float64) error {
	regName := self.fields.Lookup_register(fieldName, nil)
	if value.IsNone(regName) {
		return fmt.Errorf("Unknown field name '%s'", fieldName)
	}
	regVal := self.fields.Set_field(fieldName, fieldValue, nil, nil)
	return self.mcuTMC.Set_register(cast.ToString(regName), regVal, &printTime)
}

func (self *CommandHelperCore) UpdateCurrent(runCurrent, holdCurrent *float64, printTime float64) []float64 {
	current := self.currentHelper.Get_current()
	prevCur := current[0]
	reqHoldCur := current[2]

	if value.IsNotNone(runCurrent) || value.IsNotNone(holdCurrent) {
		newRunCurrent := prevCur
		newHoldCurrent := reqHoldCur
		if value.IsNotNone(runCurrent) {
			newRunCurrent = cast.Float64(runCurrent)
		}
		if value.IsNotNone(holdCurrent) {
			newHoldCurrent = cast.Float64(holdCurrent)
		}
		self.currentHelper.Set_current(newRunCurrent, newHoldCurrent, printTime)
		current = self.currentHelper.Get_current()
	}
	return current
}

func (self *CommandHelperCore) GetPhases() int {
	shift := self.fields.Get_field("mres", nil, nil)
	return (256 >> shift) * 4
}

func (self *CommandHelperCore) GetPhaseOffset() (*int, int) {
	return self.phaseOffset, self.GetPhases()
}

func (self *CommandHelperCore) SetPhaseOffset(phaseOffset *int) {
	self.phaseOffset = phaseOffset
}

func (self *CommandHelperCore) ClearPhaseOffset() {
	self.phaseOffset = nil
}

func (self *CommandHelperCore) QueryPhase() (int64, error) {
	fieldName := "mscnt"
	if value.IsNone(self.fields.Lookup_register(fieldName, nil)) {
		fieldName = "mstep"
	}
	reg, err := self.mcuTMC.Get_register(cast.ToString(self.fields.Lookup_register(fieldName, "")))
	if err != nil {
		return 0, err
	}
	return self.fields.Get_field(fieldName, reg, nil), nil
}

func (self *CommandHelperCore) SetupRegisterDump(readRegisters []string, readTranslate func(string, int64) (string, int64)) {
	self.readRegisters = readRegisters
	self.readTranslate = readTranslate
}

func (self *CommandHelperCore) DumpRegister(regName string) (string, error) {
	val := self.fields.Registers()[regName]
	hasRegName := collections.Contains(self.readRegisters, regName)
	if val != nil && !hasRegName {
		return self.fields.Pretty_format(regName, val), nil
	}
	if hasRegName {
		readVal, err := self.mcuTMC.Get_register(regName)
		if err != nil {
			return "", err
		}
		if self.readTranslate != nil {
			regName, readVal = self.readTranslate(regName, readVal)
		}
		return self.fields.Pretty_format(regName, readVal), nil
	}
	return "", fmt.Errorf("Unknown register name '%s'", regName)
}

func (self *CommandHelperCore) DumpAllRegisters() ([]string, error) {
	lines := []string{"========== Write-only registers =========="}
	for _, regName := range self.fields.orderedRegisterNames() {
		val := self.fields.Registers()[regName]
		if !collections.Contains(self.readRegisters, regName) {
			lines = append(lines, self.fields.Pretty_format(regName, val))
		}
	}
	lines = append(lines, "========== Queried registers ==========")
	for _, regName := range self.readRegisters {
		val, err := self.mcuTMC.Get_register(regName)
		if err != nil {
			return nil, err
		}
		if self.readTranslate != nil {
			regName, val = self.readTranslate(regName, val)
		}
		lines = append(lines, self.fields.Pretty_format(regName, val))
	}
	return lines, nil
}

func (self *CommandHelperCore) EnableVirtualEnable() {
	self.toff = self.fields.Get_field("toff", nil, nil)
	self.fields.Set_field("toff", 0, nil, nil)
}

func (self *CommandHelperCore) HasVirtualEnable() bool {
	return value.IsNotNone(self.toff)
}

func (self *CommandHelperCore) ApplyEnableRegisters(printTime *float64) error {
	if self.HasVirtualEnable() {
		self.fields.Set_field("toff", cast.ToInt64(self.toff), nil, nil)
	}
	return self.InitRegisters(printTime)
}

func (self *CommandHelperCore) ApplyDisableRegisters(printTime *float64) error {
	if !self.HasVirtualEnable() {
		return nil
	}
	val := self.fields.Set_field("toff", 0, nil, nil)
	regName := cast.ToString(self.fields.Lookup_register("toff", ""))
	return self.mcuTMC.Set_register(regName, val, printTime)
}
