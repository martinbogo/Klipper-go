package util

import (
	"fmt"
	"math"
	"sort"
	"strings"

	printerpkg "goklipper/internal/pkg/printer"
)

type ADCReader = printerpkg.ADCQueryReader

type ADCQueryRegistry struct {
	adc map[string]ADCReader
}

func NewADCQueryRegistry() *ADCQueryRegistry {
	self := &ADCQueryRegistry{}
	self.adc = map[string]ADCReader{}
	return self
}

func (self *ADCQueryRegistry) RegisterADC(name string, adc ADCReader) {
	self.adc[name] = adc
}

func (self *ADCQueryRegistry) Lookup(name string) (ADCReader, bool) {
	adc, ok := self.adc[name]
	return adc, ok
}

func (self *ADCQueryRegistry) AvailableNames() []string {
	keys := make([]string, 0, len(self.adc))
	for key := range self.adc {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (self *ADCQueryRegistry) FormatAvailableMessage() string {
	return fmt.Sprintf("Available ADC objects: %s", strings.Join(self.AvailableNames(), ", "))
}

func (self *ADCQueryRegistry) BuildValueMessage(name string, pullup float64) (string, bool) {
	adc, ok := self.Lookup(name)
	if !ok {
		return "", false
	}
	arr := adc.GetLastValue()
	value, timestamp := arr[0], arr[1]
	msg := fmt.Sprintf("ADC object \"%s\" has value %.6f (timestamp %.3f)",
		name, value, timestamp)

	if pullup != 0. {
		v := math.Max(0.00001, math.Min(0.99999, value))
		r := pullup * v / (1.0 - v)
		msg += fmt.Sprintf("\n resistance %.3f (with %.0f pullup)", r, pullup)
	}
	return msg, true
}
