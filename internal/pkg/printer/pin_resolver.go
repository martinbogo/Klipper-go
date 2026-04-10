package printer

import (
	"fmt"
	"goklipper/common/logger"
	"strings"
)

type PinError struct {
}

type PinResolver struct {
	validate_aliases bool
	reserved         map[string]string
	aliases          map[string]string
	active_pins      map[string]string
}

func NewPinResolver(validate_aliases bool) *PinResolver {
	self := PinResolver{}

	self.validate_aliases = validate_aliases
	self.reserved = map[string]string{}
	self.aliases = map[string]string{}
	self.active_pins = map[string]string{}

	return &self
}
func (self *PinResolver) Reserve_pin(pin, reserve_name string) {
	v, ok := self.reserved[pin]
	if ok {
		if v != reserve_name {
			logger.Error(fmt.Sprintf("Pin %s reserved for %s - can't reserve for %s",
				pin, self.reserved[pin], reserve_name))
		}
	}
	self.reserved[pin] = reserve_name
}
func (self *PinResolver) alias_pin(alias, pin string) {
	v, ok := self.active_pins[alias]
	if ok {
		if v != pin {
			logger.Error(fmt.Sprintf("Alias %s mapped to %s - can't alias to %s",
				alias, self.aliases[alias], pin))
		}
	}
	if strings.Contains(pin, "^~!:") || strings.TrimSpace(pin) != pin {
		logger.Errorf("Invalid pin alias '%s'\n", pin)
	}
	_, ok1 := self.active_pins[alias]
	if ok1 {
		pin = self.aliases[pin]
	}
	self.aliases[alias] = pin
	for existing_alias, existing_pin := range self.aliases {
		if existing_pin == alias {
			self.aliases[existing_alias] = pin
		}
	}
}

// 'config_endstop oid=0 pin=PB8 pull_up=0' get value of pin
func (self *PinResolver) Update_command(cmd string) string {
	//def pin_fixup(m):
	//name = m.group('name')
	//pin_id = self.aliases.get(name, name)
	//if (name != self.active_pins.setdefault(pin_id, name)
	//	and self.validate_aliases):
	//raise error("pin %s is an alias for %s" % (
	//	name, self.active_pins[pin_id]))
	//if pin_id in self.reserved:
	//raise error("pin %s is reserved for %s" % (
	//	name, self.reserved[pin_id]))
	//return m.group('prefix') + str(pin_id)
	return strings.TrimSpace(cmd)
}
