package probe

import (
	"fmt"
	"strings"
)

type Endstop interface {
	QueryEndstop(printTime float64) int
}

type endstopEntry struct {
	name    string
	endstop Endstop
}

type QueryEndstops struct {
	endstops  []endstopEntry
	lastState map[string]int
}

func NewQueryEndstops() *QueryEndstops {
	self := &QueryEndstops{}
	self.endstops = []endstopEntry{}
	self.lastState = map[string]int{}
	return self
}

func (self *QueryEndstops) RegisterEndstop(endstop Endstop, name string) {
	self.endstops = append(self.endstops, endstopEntry{name: name, endstop: endstop})
	if _, ok := self.lastState[name]; !ok {
		self.lastState[name] = 0
	}
}

func (self *QueryEndstops) Query(printTime float64) map[string]int {
	statuses := make(map[string]int, len(self.endstops))
	for _, entry := range self.endstops {
		statuses[entry.name] = entry.endstop.QueryEndstop(printTime)
	}
	self.lastState = statuses
	return statuses
}

func (self *QueryEndstops) Get_status(eventtime interface{}) map[string]map[string]interface{} {
	data := make(map[string]interface{}, len(self.lastState))
	for name, value := range self.lastState {
		data[name] = value
	}
	return map[string]map[string]interface{}{
		"last_query": data,
	}
}

func (self *QueryEndstops) StatusTextMap(statuses map[string]int) map[string]string {
	result := make(map[string]string, len(statuses))
	for _, entry := range self.endstops {
		result[entry.name] = self.statusCode(statuses[entry.name])
	}
	return result
}

func (self *QueryEndstops) StatusMessage(statuses map[string]int) string {
	parts := make([]string, 0, len(self.endstops))
	for _, entry := range self.endstops {
		parts = append(parts, fmt.Sprintf("%s:%s", entry.name, self.statusCode(statuses[entry.name])))
	}
	return strings.Join(parts, " ")
}

func (self *QueryEndstops) statusCode(value int) string {
	if value == 0 {
		return "open"
	}
	return "TRIGGERED"
}