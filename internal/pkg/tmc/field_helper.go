package tmc

import (
	"fmt"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/maths"
	"goklipper/common/utils/str"
	"goklipper/common/value"
	"math/bits"
	"sort"
	"strconv"
	"strings"
)

type ConfigFieldSource interface {
	Getboolean(option string, default1 interface{}, noteValid bool) bool
	Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int
	Getint64(option string, default1 interface{}, minval, maxval int64, noteValid bool) int64
}

// Return the position of the first bit set in a mask.
func ffs(mask int64) int {
	return bits.TrailingZeros64(uint64(mask))
}

type FieldHelper struct {
	allFields       map[string]map[string]int64
	signedFields    map[string]int
	fieldFormatters map[string]func(interface{}) string
	registers       map[string]interface{}
	registerOrder   []string
	registerSeen    map[string]struct{}
	fieldToRegister map[string]string
}

func NewFieldHelper(allFields map[string]map[string]int64, signedFields []string, fieldFormatters map[string]func(interface{}) string,
	registers *map[string]interface{}) *FieldHelper {
	self := new(FieldHelper)
	self.allFields = allFields
	self.signedFields = make(map[string]int)
	for _, field := range signedFields {
		self.signedFields[field] = 1
	}

	self.fieldFormatters = fieldFormatters
	self.registers = make(map[string]interface{})
	self.registerSeen = make(map[string]struct{})
	if registers != nil {
		keys := str.MapStringKeys(*registers)
		sort.Strings(keys)
		for _, k := range keys {
			v := (*registers)[k]
			self.registers[k] = v
			self.registerOrder = append(self.registerOrder, k)
			self.registerSeen[k] = struct{}{}
		}
	}

	self.fieldToRegister = make(map[string]string)
	for registerName, fields := range self.allFields {
		for fieldName := range fields {
			self.fieldToRegister[fieldName] = registerName
		}
	}
	return self
}

func (self *FieldHelper) Lookup_register(fieldName string, defaultValue interface{}) interface{} {
	if val, ok := self.fieldToRegister[fieldName]; ok {
		return val
	}
	return defaultValue
}

func (self *FieldHelper) Get_field(fieldName string, regValue interface{}, regName *string) int64 {
	resolvedRegName := cast.String(regName)
	if value.IsNone(regName) {
		resolvedRegName = self.fieldToRegister[fieldName]
	}

	if value.IsNone(regValue) {
		regValue = self.registers[resolvedRegName]
	}

	mask := self.allFields[resolvedRegName][fieldName]
	fieldValue := (cast.ToInt64(regValue) & mask) >> ffs(mask)
	if _, ok := self.signedFields[fieldName]; ok && ((cast.ToInt64(regValue)&mask)<<1) > mask {
		fieldValue -= 1 << bits.Len(uint(fieldValue))
	}

	return fieldValue
}

func (self *FieldHelper) Set_field(fieldName string, fieldValue interface{}, regValue interface{}, regName interface{}) int64 {
	if value.IsNone(regName) {
		regName = self.fieldToRegister[fieldName]
	}

	if value.IsNone(regValue) {
		regValue = self.registers[regName.(string)]
	}
	resolvedRegName := regName.(string)
	mask := self.allFields[resolvedRegName][fieldName]
	newValue := (cast.ToInt64(regValue) & ^mask) | ((cast.ToInt64(fieldValue) << ffs(mask)) & mask)
	self.trackRegister(resolvedRegName)
	self.registers[resolvedRegName] = newValue
	return newValue
}

func (self *FieldHelper) trackRegister(regName string) {
	if _, ok := self.registerSeen[regName]; ok {
		return
	}
	self.registerOrder = append(self.registerOrder, regName)
	self.registerSeen[regName] = struct{}{}
}

func (self *FieldHelper) orderedRegisterNames() []string {
	ordered := append([]string(nil), self.registerOrder...)
	if len(ordered) == len(self.registers) {
		return ordered
	}
	extra := make([]string, 0, len(self.registers)-len(ordered))
	for regName := range self.registers {
		if _, ok := self.registerSeen[regName]; ok {
			continue
		}
		extra = append(extra, regName)
	}
	sort.Strings(extra)
	return append(ordered, extra...)
}

func (self *FieldHelper) Set_config_field(config ConfigFieldSource, fieldName string, defaultValue interface{}) int64 {
	configName := "driver_" + strings.ToUpper(fieldName)
	regName := self.fieldToRegister[fieldName]
	mask := self.allFields[regName][fieldName]
	maxVal := mask >> ffs(mask)

	var val interface{}
	if maxVal == 1 {
		if config.Getboolean(configName, defaultValue, true) {
			val = 1
		} else {
			val = 0
		}
	} else if _, ok := self.signedFields[fieldName]; ok {
		if _, ok := defaultValue.(int); ok {
			val = config.Getint(configName, defaultValue, -(maths.FloorDiv(cast.ForceInt(maxVal), 2) + 1), maths.FloorDiv(cast.ForceInt(maxVal), 2), true)
		} else {
			val = config.Getint64(configName, defaultValue, -1, maxVal, true)
		}
	} else {
		if _, ok := defaultValue.(int); ok {
			val = config.Getint(configName, defaultValue, 0, cast.ForceInt(maxVal), true)
		} else {
			val = config.Getint64(configName, defaultValue, 0, maxVal, true)
		}
	}
	return self.Set_field(fieldName, val, nil, nil)
}

func (self *FieldHelper) Pretty_format(regName string, regValue interface{}) string {
	regFields := self.allFields[regName]

	keys := str.MapStringKeys(regFields)
	sort.Strings(keys)
	fields := make([]string, 0, len(keys))
	for _, fieldName := range keys {
		fieldValue := self.Get_field(fieldName, regValue, cast.StringP(regName))
		var stringValue string
		if self.fieldFormatters[fieldName] != nil {
			// Use the custom formatter if one is registered.
			stringValue = self.fieldFormatters[fieldName](fieldValue)
		} else {
			// Match Python's str() default: show every non-zero field even
			// if no custom formatter is registered for it.
			stringValue = strconv.FormatInt(fieldValue, 10)
		}
		if len(stringValue) != 0 && stringValue != "0" {
			fields = append(fields, fmt.Sprintf(" %s=%s", fieldName, stringValue))
		}
	}

	return fmt.Sprintf("%-11s %08x%s", regName+":", regValue, strings.Join(fields, ""))
}

func (self *FieldHelper) Get_reg_fields(regName string, regValue interface{}) map[string]int64 {
	regFields := self.allFields[regName]
	resolved := make(map[string]int64)
	for fieldName := range regFields {
		resolved[fieldName] = self.Get_field(fieldName, regValue, cast.StringP(regName))
	}
	return resolved
}

func (self *FieldHelper) All_fields() map[string]map[string]int64 {
	return self.allFields
}

func (self *FieldHelper) Registers() map[string]interface{} {
	return self.registers
}
