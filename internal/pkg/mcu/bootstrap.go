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
	resolvedRestartCmds := applyUpdate(restartCmds)
	resolvedInitCmds := applyUpdate(initCmds)
	configCRC := crc32.ChecksumIEEE([]byte(strings.Join(resolvedConfigCmds, "\n"))) & 0xffffffff
	resolvedConfigCmds = append(resolvedConfigCmds, fmt.Sprintf("finalize_config crc=%d", configCRC))
	return ConfigBuildPlan{
		ConfigCmds:  resolvedConfigCmds,
		RestartCmds: resolvedRestartCmds,
		InitCmds:    resolvedInitCmds,
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

type ConnectionExecutionHooks struct {
	ConnectFileoutput func()
	ConnectCanbus     func()
	ConnectRemote     func(serialPort string)
	ConnectUART       func(serialPort string, baud int, rts bool)
	ConnectPipe       func(serialPort string)
	ConnectClockSync  func()
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

func ExecuteConnectionPlan(plan ConnectionPlan, serialPort string, baud int, hooks ConnectionExecutionHooks) {
	switch plan.Mode {
	case ConnectionModeFileoutput:
		if hooks.ConnectFileoutput != nil {
			hooks.ConnectFileoutput()
		}
	case ConnectionModeCanbus:
		if hooks.ConnectCanbus != nil {
			hooks.ConnectCanbus()
		}
	case ConnectionModeRemote:
		if hooks.ConnectRemote != nil {
			hooks.ConnectRemote(serialPort)
		}
	case ConnectionModeUART:
		if hooks.ConnectUART != nil {
			hooks.ConnectUART(serialPort, baud, plan.RTS)
		}
	default:
		if hooks.ConnectPipe != nil {
			hooks.ConnectPipe(serialPort)
		}
	}
	if plan.NeedsClockSyncConnect && hooks.ConnectClockSync != nil {
		hooks.ConnectClockSync()
	}
}
