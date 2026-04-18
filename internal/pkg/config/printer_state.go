package config

import (
	"fmt"
	"goklipper/common/configparser"
	"goklipper/common/value"
)

type RuntimeStatus struct {
	deprecated        map[string]interface{}
	rawConfig         map[string]interface{}
	savePending       map[string]interface{}
	settings          map[string]interface{}
	warnings          []interface{}
	saveConfigPending bool
}

func NewRuntimeStatus() *RuntimeStatus {
	return &RuntimeStatus{
		deprecated:  map[string]interface{}{},
		rawConfig:   map[string]interface{}{},
		savePending: map[string]interface{}{},
		settings:    map[string]interface{}{},
		warnings:    []interface{}{},
	}
}

func (self *RuntimeStatus) Deprecate(section, option, deprecatedValue, msg string) {
	key := fmt.Sprintf("%s:%s:%s", section, option, deprecatedValue)
	self.deprecated[key] = msg
}

func (self *RuntimeStatus) Rebuild(fileconfig *configparser.RawConfigParser, accessTracking map[string]interface{}) {
	snapshot := BuildStatusSnapshot(fileconfig, accessTracking, self.deprecated)
	self.rawConfig = snapshot.RawConfig
	self.settings = snapshot.Settings
	self.warnings = snapshot.Warnings
}

func (self *RuntimeStatus) Snapshot() map[string]interface{} {
	return map[string]interface{}{
		"config":                    self.rawConfig,
		"settings":                  self.settings,
		"warnings":                  self.warnings,
		"save_config_pending":       self.saveConfigPending,
		"save_config_pending_items": self.savePending,
	}
}

func (self *RuntimeStatus) NotePendingSet(section, option, val string) {
	pending := self.savePending
	if _, ok := pending[section]; !ok || value.IsNone(pending[section]) {
		pending[section] = map[string]interface{}{}
	}
	options := pending[section].(map[string]interface{})
	options[option] = val
	pending[section] = options
	self.savePending = pending
	self.saveConfigPending = true
}

func (self *RuntimeStatus) NotePendingRemoval(section string, removedAutosaveSection bool) {
	if removedAutosaveSection {
		delete(self.savePending, section)
		self.saveConfigPending = true
		return
	}
	if _, ok := self.savePending[section]; ok {
		delete(self.savePending, section)
		self.saveConfigPending = true
	}
}