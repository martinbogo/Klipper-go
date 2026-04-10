package mcu

import "strings"

type ReservedPin struct {
	Pin   string
	Owner string
}

func CollectReservedPins(constants map[string]interface{}) []ReservedPin {
	reserved := []ReservedPin{}
	for name, value := range constants {
		if !strings.HasPrefix(name, "RESERVE_PINS_") {
			continue
		}
		pins, ok := value.(string)
		if !ok || pins == "" {
			continue
		}
		owner := name[13:]
		for _, pin := range strings.Split(pins, ",") {
			reserved = append(reserved, ReservedPin{Pin: pin, Owner: owner})
		}
	}
	return reserved
}

type IdentifyFinalizePlan struct {
	RestartMethod string
	IsMCUBridge   bool
	StatusInfo    map[string]interface{}
	ReservedPins  []ReservedPin
}

func BuildIdentifyFinalizePlan(restartMethod string, hasReset bool, hasConfigReset bool, serialBaudConstant interface{}, canbusBridgeConstant interface{}, version string, buildVersions string, constants map[string]interface{}) IdentifyFinalizePlan {
	extOnly := !hasReset && !hasConfigReset
	finalRestartMethod := restartMethod
	if finalRestartMethod == "" && serialBaudConstant == nil && !extOnly {
		finalRestartMethod = "command"
	}
	statusInfo := map[string]interface{}{
		"mcu_version":        version,
		"mcu_build_versions": buildVersions,
		"mcu_constants":      constants,
	}
	return IdentifyFinalizePlan{
		RestartMethod: finalRestartMethod,
		IsMCUBridge:   canbusBridgeConstant != 0,
		StatusInfo:    statusInfo,
		ReservedPins:  CollectReservedPins(constants),
	}
}
