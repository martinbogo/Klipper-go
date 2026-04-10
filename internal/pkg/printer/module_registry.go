package printer

import (
	"goklipper/common/logger"
	"strings"
)

type ModuleInit func(section interface{}) interface{}

type ModuleRegistry struct {
	modules map[string]ModuleInit
}

func NewModuleRegistry() *ModuleRegistry {
	self := ModuleRegistry{}
	self.modules = map[string]ModuleInit{}
	return &self
}

func (self *ModuleRegistry) Register(name string, init ModuleInit) {
	self.modules[name] = init
}

func (self *ModuleRegistry) Has(name string) bool {
	_, ok := self.modules[name]
	return ok
}

func (self *ModuleRegistry) LookupExact(name string) (ModuleInit, bool) {
	initFunc, ok := self.modules[name]
	return initFunc, ok
}

func (self *ModuleRegistry) baseSection(section string) string {
	return strings.Split(section, " ")[0]
}

func (self *ModuleRegistry) LoadObject(section string, lookupObject func(string) interface{},
	getSection func(string) interface{}, storeObject func(string, interface{})) interface{} {
	if obj := lookupObject(section); obj != nil {
		return obj
	}

	if strings.HasSuffix(section, " default") || strings.HasSuffix(section, " adaptive") {
		baseSection := self.baseSection(section)
		if _, ok := self.LookupExact(baseSection); !ok {
			logger.Errorf("%s depend on %s, should loaded before", section, baseSection)
			return nil
		}
		logger.Debugf("%s only as config, don't use for load object", section)
		initFunc, _ := self.LookupExact(baseSection)
		return initFunc
	}

	if initFunc, ok := self.LookupExact(section); ok {
		storeObject(section, initFunc(getSection(section)))
	} else if initFunc, ok := self.LookupExact(self.baseSection(section)); ok {
		storeObject(section, initFunc(getSection(section)))
	}

	return lookupObject(section)
}

func (self *ModuleRegistry) ReloadObject(section string, getSection func(string) interface{},
	storeObject func(string, interface{}), lookupObject func(string) interface{}) interface{} {
	moduleSection := section
	moduleParts := strings.Split(section, " ")
	if strings.HasPrefix(section, "gcode_macro") && len(moduleParts) > 1 {
		moduleSection = "gcode_macro_1"
	}

	if initFunc, ok := self.LookupExact(moduleSection); ok {
		storeObject(section, initFunc(getSection(section)))
	} else if initFunc, ok := self.LookupExact(self.baseSection(moduleSection)); ok {
		storeObject(section, initFunc(getSection(moduleSection)))
	}

	return lookupObject(section)
}