package mcu

import (
	"fmt"
	"sort"
	"strings"
)

type EmergencyStopDecision struct {
	Skip bool
}

func BuildEmergencyStopDecision(hasEmergencyStop bool, isShutdown bool, force bool) EmergencyStopDecision {
	return EmergencyStopDecision{Skip: !hasEmergencyStop || (isShutdown && !force)}
}

func BuildMCULogInfo(mcuName string, messageCount int, version string, buildVersions string, constants map[string]interface{}) string {
	constantSummary := BuildMCUConstantSummary(constants)
	return fmt.Sprintf("Loaded MCU '%s' %d commands (%s / %s) MCU '%s' config: %s",
		mcuName, messageCount, version, buildVersions, mcuName, constantSummary)
}

func BuildMCUConstantSummary(constants map[string]interface{}) string {
	if len(constants) == 0 {
		return ""
	}
	keys := make([]string, 0, len(constants))
	for key := range constants {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, constants[key]))
	}
	return strings.Join(parts, " ")
}

type ConfiguredMCUInfo struct {
	MoveMessage  string
	RolloverInfo string
}

func BuildConfiguredMCUInfo(mcuName string, moveCount int64, logInfo string) ConfiguredMCUInfo {
	moveMessage := fmt.Sprintf("Configured MCU '%s' (%d moves)", mcuName, moveCount)
	return ConfiguredMCUInfo{
		MoveMessage:  moveMessage,
		RolloverInfo: logInfo + "\n" + moveMessage,
	}
}
