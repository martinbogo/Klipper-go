package mcu

import "fmt"

type ConfigSnapshot struct {
	IsConfig   bool
	CRC        uint32
	IsShutdown bool
	MoveCount  int64
}

func DefaultFileoutputConfigSnapshot() *ConfigSnapshot {
	return &ConfigSnapshot{IsConfig: false, CRC: 0, IsShutdown: false, MoveCount: 500}
}

func ParseConfigSnapshot(params map[string]interface{}) *ConfigSnapshot {
	if params == nil {
		return nil
	}
	snapshot := &ConfigSnapshot{}
	if isConfig, ok := params["is_config"].(int64); ok {
		snapshot.IsConfig = isConfig != 0
	}
	switch crc := params["crc"].(type) {
	case int64:
		snapshot.CRC = uint32(crc)
	case uint32:
		snapshot.CRC = crc
	}
	if isShutdown, ok := params["is_shutdown"].(int64); ok {
		snapshot.IsShutdown = isShutdown != 0
	}
	if moveCount, ok := params["move_count"].(int64); ok {
		snapshot.MoveCount = moveCount
	}
	return snapshot
}

type ConfigQueryDecision struct {
	NeedsClearShutdown bool
	ErrorMessage       string
}

func EvaluateConfigQuery(snapshot *ConfigSnapshot, isShutdown bool, shutdownMsg string, mcuName string, hasClearShutdown bool) ConfigQueryDecision {
	if snapshot == nil {
		return ConfigQueryDecision{}
	}
	if isShutdown {
		return ConfigQueryDecision{ErrorMessage: fmt.Sprintf("MCU '%s' error during config: %s", mcuName, shutdownMsg)}
	}
	if !snapshot.IsShutdown {
		return ConfigQueryDecision{}
	}
	if !hasClearShutdown {
		return ConfigQueryDecision{ErrorMessage: fmt.Sprintf("Can not update MCU '%s' config as it is shutdown", mcuName)}
	}
	return ConfigQueryDecision{NeedsClearShutdown: true}
}

func EvaluateClearedShutdownSnapshot(snapshot *ConfigSnapshot, mcuName string) string {
	if snapshot != nil && snapshot.IsShutdown {
		return fmt.Sprintf("Can not update MCU '%s' config as it is STUCK in shutdown", mcuName)
	}
	return ""
}

type ConnectDecision struct {
	ReturnError         string
	PanicMessage        string
	NeedsPreConfigReset bool
	SendConfig          bool
	UsePrevCRC          bool
	PrevCRC             uint32
	NeedsRequery        bool
}

func BuildConnectDecision(snapshot *ConfigSnapshot, restartMethod string, startReason string, mcuName string) ConnectDecision {
	if snapshot == nil {
		return ConnectDecision{ReturnError: "MCU is shudown,please firmware restart"}
	}
	if !snapshot.IsConfig {
		return ConnectDecision{
			NeedsPreConfigReset: restartMethod == "rpi_usb",
			SendConfig:          true,
			NeedsRequery:        true,
		}
	}
	if startReason == "firmware_restart" {
		return ConnectDecision{PanicMessage: fmt.Sprintf("Failed automated reset of MCU '%s'", mcuName)}
	}
	return ConnectDecision{SendConfig: true, UsePrevCRC: true, PrevCRC: snapshot.CRC}
}

func ValidateConfiguredSnapshot(snapshot *ConfigSnapshot, isFileoutput bool, reservedMoveSlots int64, mcuName string) string {
	if (snapshot == nil || !snapshot.IsConfig) && !isFileoutput {
		return fmt.Sprintf("Unable to configure MCU '%s'", mcuName)
	}
	if snapshot != nil && snapshot.MoveCount < reservedMoveSlots {
		return fmt.Sprintf("Too few moves available on MCU '%s'", mcuName)
	}
	return ""
}
