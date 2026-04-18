package mcu

import (
	"fmt"
	"time"

	"goklipper/common/logger"
)

type ConfigSession struct {
	callbacks   []func()
	configCmds  []string
	restartCmds []string
	initCmds    []string
}

func NewConfigSession() *ConfigSession {
	return &ConfigSession{
		callbacks:   []func(){},
		configCmds:  []string{},
		restartCmds: []string{},
		initCmds:    []string{},
	}
}

func (self *ConfigSession) RegisterCallback(cb func()) {
	self.callbacks = append(self.callbacks, cb)
}

func (self *ConfigSession) RunCallbacks() {
	for _, cb := range self.callbacks {
		cb()
	}
}

func (self *ConfigSession) AddCommand(cmd string, isInit bool, onRestart bool) {
	if isInit {
		self.initCmds = append(self.initCmds, cmd)
	} else if onRestart {
		self.restartCmds = append(self.restartCmds, cmd)
	} else {
		self.configCmds = append(self.configCmds, cmd)
	}
}

func (self *ConfigSession) ConfigCmds() []string {
	return append([]string{}, self.configCmds...)
}

func (self *ConfigSession) RestartCmds() []string {
	return append([]string{}, self.restartCmds...)
}

func (self *ConfigSession) InitCmds() []string {
	return append([]string{}, self.initCmds...)
}

func (self *ConfigSession) ApplyResolvedCommands(configCmds []string, restartCmds []string, initCmds []string) {
	self.configCmds = append([]string{}, configCmds...)
	self.restartCmds = append([]string{}, restartCmds...)
	self.initCmds = append([]string{}, initCmds...)
}

type ConfigSendHooks struct {
	RunCallbacks            func()
	OIDCount                int
	ConfigCmds              []string
	RestartCmds             []string
	InitCmds                []string
	UpdateCommand           func(string) string
	SetCommands             func([]string, []string, []string)
	RegisterStartingHandler func(func(map[string]interface{}) error)
	SendCommand             func(string)
	MCUName                 string
}

func SendConfig(hooks ConfigSendHooks, prevCRC *uint32, startingHandler func(map[string]interface{}) error) string {
	if hooks.RunCallbacks != nil {
		hooks.RunCallbacks()
	}
	plan := BuildConfigPlan(hooks.OIDCount, hooks.ConfigCmds, hooks.RestartCmds, hooks.InitCmds, hooks.UpdateCommand)
	if hooks.SetCommands != nil {
		hooks.SetCommands(plan.ConfigCmds, plan.RestartCmds, plan.InitCmds)
	}
	if prevCRC != nil {
		logger.Debug("config_crc:", plan.ConfigCRC, " prev_crc:", *prevCRC)
	} else {
		logger.Debug("config_crc:", plan.ConfigCRC)
	}
	if prevCRC != nil && plan.ConfigCRC != *prevCRC {
		return fmt.Sprintf("MCU '%s' CRC does not match config", hooks.MCUName)
	}
	if hooks.RegisterStartingHandler != nil {
		hooks.RegisterStartingHandler(startingHandler)
	}
	if prevCRC == nil {
		logger.Debugf("Sending MCU '%s' printer configuration...",
			hooks.MCUName)
		for _, command := range plan.ConfigCmds {
			hooks.SendCommand(command)
		}
	} else {
		for _, command := range plan.RestartCmds {
			hooks.SendCommand(command)
		}
	}
	for _, command := range plan.InitCmds {
		hooks.SendCommand(command)
	}
	return ""
}

func SendConfigSession(session *ConfigSession, oidCount int, oidCountGetter func() int, updateCommand func(string) string,
	registerStartingHandler func(func(map[string]interface{}) error),
	sendCommand func(string), mcuName string, prevCRC *uint32,
	startingHandler func(map[string]interface{}) error) string {
	if session == nil {
		session = NewConfigSession()
	}
	session.RunCallbacks()
	if oidCountGetter != nil {
		oidCount = oidCountGetter()
	}
	return SendConfig(ConfigSendHooks{
		OIDCount:                oidCount,
		ConfigCmds:              session.ConfigCmds(),
		RestartCmds:             session.RestartCmds(),
		InitCmds:                session.InitCmds(),
		UpdateCommand:           updateCommand,
		RegisterStartingHandler: registerStartingHandler,
		SendCommand:             sendCommand,
		SetCommands: func(configCmds []string, restartCmds []string, initCmds []string) {
			session.ApplyResolvedCommands(configCmds, restartCmds, initCmds)
		},
		MCUName: mcuName,
	}, prevCRC, startingHandler)
}

type ConfigQueryHooks struct {
	IsFileoutput       bool
	QueryConfig        func() map[string]interface{}
	IsShutdown         bool
	ShutdownMessage    string
	MCUName            string
	HasClearShutdown   bool
	SendClearShutdown  func()
	ClearLocalShutdown func()
	Sleep              func(time.Duration)
}

type ConfigQueryResult struct {
	Snapshot     *ConfigSnapshot
	ErrorMessage string
}

func QueryConfigSnapshot(hooks ConfigQueryHooks) ConfigQueryResult {
	if hooks.IsFileoutput {
		return ConfigQueryResult{Snapshot: DefaultFileoutputConfigSnapshot()}
	}
	queryConfig := hooks.QueryConfig
	if queryConfig == nil {
		return ConfigQueryResult{}
	}
	snapshot := ParseConfigSnapshot(queryConfig())
	decision := EvaluateConfigQuery(snapshot, hooks.IsShutdown, hooks.ShutdownMessage, hooks.MCUName, hooks.HasClearShutdown)
	if decision.ErrorMessage != "" {
		return ConfigQueryResult{Snapshot: snapshot, ErrorMessage: decision.ErrorMessage}
	}
	if !decision.NeedsClearShutdown {
		return ConfigQueryResult{Snapshot: snapshot}
	}
	logger.Warnf("Attempting to send clear_shutdown to reset '%s' MCU state", hooks.MCUName)
	if hooks.SendClearShutdown != nil {
		hooks.SendClearShutdown()
	}
	sleep := hooks.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	sleep(100 * time.Millisecond)
	snapshot = ParseConfigSnapshot(queryConfig())
	if errorMessage := EvaluateClearedShutdownSnapshot(snapshot, hooks.MCUName); errorMessage != "" {
		return ConfigQueryResult{Snapshot: snapshot, ErrorMessage: errorMessage}
	}
	logger.Infof("Successfully cleared shutdown on MCU '%s'", hooks.MCUName)
	if hooks.ClearLocalShutdown != nil {
		hooks.ClearLocalShutdown()
	}
	return ConfigQueryResult{Snapshot: snapshot}
}
