package config

import (
	"bytes"
	"fmt"
	"goklipper/common/configparser"
	"strings"
)

type StatusSnapshot struct {
	RawConfig map[string]interface{}
	Settings  map[string]interface{}
	Warnings  []interface{}
}

type MainConfigBundle struct {
	Regular  *configparser.RawConfigParser
	Autosave *configparser.RawConfigParser
	Combined *configparser.RawConfigParser
}

// ParseConfigText parses config data into a RawConfigParser.
func ParseConfigText(data, filename string) *configparser.RawConfigParser {
	fileconfig := configparser.NewRawConfigParser()
	ParseConfig(data, filename, fileconfig, map[string]string{})
	return fileconfig
}

// BuildMainConfigBundle splits regular and autosave data and returns parsed views for each.
func BuildMainConfigBundle(data, filename string) *MainConfigBundle {
	regularData, autosaveData := FindAutosaveData(data)
	regularConfig := ParseConfigText(regularData, filename)
	autosaveData = StripDuplicates(autosaveData, regularConfig)
	return &MainConfigBundle{
		Regular:  regularConfig,
		Autosave: ParseConfigText(autosaveData, filename),
		Combined: ParseConfigText(regularData+"\n"+autosaveData, filename),
	}
}

// LoadMainConfigBundle reads a config file and returns parsed regular, autosave, and combined views.
func LoadMainConfigBundle(filename string) (*MainConfigBundle, error) {
	data, err := ReadConfigFile(filename)
	if err != nil {
		return nil, err
	}
	return BuildMainConfigBundle(data, filename), nil
}

// RenderConfig serializes a RawConfigParser back to normalized config text.
func RenderConfig(fileconfig *configparser.RawConfigParser) string {
	buf := bytes.NewBuffer(nil)
	fileconfig.Write(buf)
	return strings.TrimSpace(buf.String())
}

// BuildStatusSnapshot converts parsed config plus tracking maps into status payloads.
func BuildStatusSnapshot(fileconfig *configparser.RawConfigParser, accessTracking, deprecated map[string]interface{}) StatusSnapshot {
	rawConfig := make(map[string]interface{})
	for _, section := range fileconfig.Sections() {
		sectionStatus := make(map[string]interface{})
		rawConfig[section] = sectionStatus
		options, err := fileconfig.Options(section)
		if err != nil {
			continue
		}
		for option := range options {
			sectionStatus[option] = fileconfig.Get(section, option)
		}
	}
	return StatusSnapshot{
		RawConfig: rawConfig,
		Settings:  BuildAccessTrackingSettings(accessTracking),
		Warnings:  BuildDeprecationWarnings(deprecated),
	}
}

// ValidateAutosaveConflicts reports autosave entries that would shadow included values.
func ValidateAutosaveConflicts(regularData, cfgname string, autosave *configparser.RawConfigParser) error {
	config := ParseConfigText(regularData, cfgname)
	for _, section := range autosave.Sections() {
		options, err := autosave.Options(section)
		if err != nil {
			continue
		}
		for option := range options {
			if config.Has_option(section, option) {
				return fmt.Errorf("SAVE_CONFIG section '%s' option '%s' conflicts with included value", section, option)
			}
		}
	}
	return nil
}
