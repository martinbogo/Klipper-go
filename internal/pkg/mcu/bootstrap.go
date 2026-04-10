package mcu

import (
	"fmt"
	"hash/crc32"
	"strings"
)

type ConfigBuildPlan struct {
	ConfigCmds  []string
	RestartCmds []string
	InitCmds    []string
	ConfigCRC   uint32
}

func BuildConfigPlan(oidCount int, configCmds []string, restartCmds []string, initCmds []string, updateCommand func(string) string) ConfigBuildPlan {
	applyUpdate := func(commands []string) []string {
		updated := make([]string, len(commands))
		for i, cmd := range commands {
			if updateCommand != nil {
				updated[i] = updateCommand(cmd)
			} else {
				updated[i] = cmd
			}
		}
		return updated
	}
	resolvedConfigCmds := append([]string{fmt.Sprintf("allocate_oids count=%d", oidCount)}, applyUpdate(configCmds)...)
	configCRC := crc32.ChecksumIEEE([]byte(strings.Join(resolvedConfigCmds, "\n"))) & 0xffffffff
	resolvedConfigCmds = append(resolvedConfigCmds, fmt.Sprintf("finalize_config crc=%d", configCRC))
	return ConfigBuildPlan{
		ConfigCmds:  resolvedConfigCmds,
		RestartCmds: applyUpdate(restartCmds),
		InitCmds:    applyUpdate(initCmds),
		ConfigCRC:   configCRC,
	}
}

type ConnectionMode string

const (
	ConnectionModeFileoutput ConnectionMode = "fileoutput"
	ConnectionModeCanbus     ConnectionMode = "canbus"
	ConnectionModeRemote     ConnectionMode = "remote"
	ConnectionModeUART       ConnectionMode = "uart"
	ConnectionModePipe       ConnectionMode = "pipe"
)

type ConnectionPlan struct {
	Mode                  ConnectionMode
	RTS                   bool
	NeedsPowerEnableReset bool
	NeedsClockSyncConnect bool
}

func BuildConnectionPlan(isFileoutput bool, restartMethod string, serialPort string, baud int, hasCanbus bool, serialPathExists bool) ConnectionPlan {
	if isFileoutput {
		return ConnectionPlan{Mode: ConnectionModeFileoutput}
	}
	plan := ConnectionPlan{NeedsClockSyncConnect: true, RTS: restartMethod != "cheetah"}
	if restartMethod == "rpi_usb" && !serialPathExists {
		plan.NeedsPowerEnableReset = true
	}
	if hasCanbus {
		plan.Mode = ConnectionModeCanbus
		return plan
	}
	if strings.HasPrefix(serialPort, "tcp@") {
		plan.Mode = ConnectionModeRemote
		return plan
	}
	if baud > 0 {
		plan.Mode = ConnectionModeUART
		return plan
	}
	plan.Mode = ConnectionModePipe
	return plan
}
