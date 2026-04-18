package mcu

import "fmt"

type LifecycleState struct {
	isShutdown      bool
	shutdownClock   int64
	shutdownMessage string
	isTimeout       bool
}

func NewLifecycleState() *LifecycleState {
	return &LifecycleState{}
}

func (self *LifecycleState) HandleShutdownPlan(plan ShutdownPlan) bool {
	if self.isShutdown {
		return false
	}
	self.isShutdown = true
	if plan.HasShutdownClock {
		self.shutdownClock = plan.ShutdownClock
	}
	self.shutdownMessage = plan.ShutdownMessage
	return true
}

func (self *LifecycleState) StartingShutdownMessage(mcuName string) string {
	return BuildStartingShutdownMessage(self.isShutdown, mcuName)
}

func (self *LifecycleState) ShutdownActive() bool {
	return self.isShutdown
}

func (self *LifecycleState) ShutdownClock() int64 {
	return self.shutdownClock
}

func (self *LifecycleState) ShutdownMessage() string {
	return self.shutdownMessage
}

func (self *LifecycleState) ClearShutdown() {
	self.isShutdown = false
}

func (self *LifecycleState) MarkShutdown() {
	self.isShutdown = true
}

func (self *LifecycleState) TimeoutActive() bool {
	return self.isTimeout
}

func (self *LifecycleState) SetTimeout(active bool) {
	self.isTimeout = active
}

type ShutdownPlan struct {
	HasShutdownClock bool
	ShutdownClock    int64
	ShutdownMessage  string
	LogMessage       string
	AsyncMessage     string
	RespondInfo      string
}

func BuildShutdownPlan(mcuName string, params map[string]interface{}, clockSyncDebug string, serialDebug string) ShutdownPlan {
	plan := ShutdownPlan{}
	if clock, ok := params["clock"].(int64); ok {
		plan.HasShutdownClock = true
		plan.ShutdownClock = clock
	}
	shutdownMessage, _ := params["static_string_id"].(string)
	eventName, _ := params["#name"].(string)
	plan.ShutdownMessage = shutdownMessage
	plan.LogMessage = fmt.Sprintf("MCU '%s' %s: %s\n%s\n%s", mcuName, eventName, shutdownMessage, clockSyncDebug, serialDebug)
	prefix := fmt.Sprintf("MCU '%s' shutdown: ", mcuName)
	if eventName == "is_shutdown" {
		prefix = fmt.Sprintf("Previous MCU '%s' shutdown: ", mcuName)
	}
	plan.AsyncMessage = prefix + shutdownMessage
	plan.RespondInfo = fmt.Sprintf("MCU '%s' %s: %s", mcuName, eventName, shutdownMessage)
	return plan
}

func BuildStartingShutdownMessage(isShutdown bool, mcuName string) string {
	if isShutdown {
		return ""
	}
	return fmt.Sprintf("MCU '%s' spontaneous restart", mcuName)
}

type RestartCheckDecision struct {
	Skip         bool
	LogMessage   string
	ExitReason   string
	PauseSeconds float64
	PanicMessage string
}

func BuildRestartCheckDecision(startReason interface{}, mcuName string, reason string) RestartCheckDecision {
	if startReasonString, ok := startReason.(string); ok && startReasonString == "firmware_restart" {
		return RestartCheckDecision{Skip: true}
	}
	return RestartCheckDecision{
		LogMessage:   fmt.Sprintf("Attempting automated MCU '%s' restart: %s", mcuName, reason),
		ExitReason:   "firmware_restart",
		PauseSeconds: 2.0,
		PanicMessage: fmt.Sprintf("Attempt MCU '%s' restart failed", mcuName),
	}
}

type CommandResetMode string

const (
	CommandResetModeNone        CommandResetMode = ""
	CommandResetModeReset       CommandResetMode = "reset"
	CommandResetModeConfigReset CommandResetMode = "config_reset"
)

type CommandResetPlan struct {
	Mode                 CommandResetMode
	ErrorMessage         string
	LogMessage           string
	MarkShutdown         bool
	NeedsEmergencyStop   bool
	PreSendPauseSeconds  float64
	PostSendPauseSeconds float64
}

func BuildCommandResetPlan(hasResetCommand bool, hasConfigResetCommand bool, clockSyncActive bool, mcuName string) CommandResetPlan {
	if (!hasResetCommand && !hasConfigResetCommand) || !clockSyncActive {
		return CommandResetPlan{ErrorMessage: fmt.Sprintf("Unable to issue reset command on MCU '%s'", mcuName)}
	}
	if !hasResetCommand {
		return CommandResetPlan{
			Mode:                 CommandResetModeConfigReset,
			LogMessage:           fmt.Sprintf("Attempting MCU '%s' config_reset command", mcuName),
			MarkShutdown:         true,
			NeedsEmergencyStop:   true,
			PreSendPauseSeconds:  0.015,
			PostSendPauseSeconds: 0.015,
		}
	}
	return CommandResetPlan{
		Mode:                 CommandResetModeReset,
		LogMessage:           fmt.Sprintf("Attempting MCU '%s' reset command", mcuName),
		PostSendPauseSeconds: 0.015,
	}
}

type FirmwareRestartAction string

const (
	FirmwareRestartActionNone    FirmwareRestartAction = ""
	FirmwareRestartActionRPIUSB  FirmwareRestartAction = "rpi_usb"
	FirmwareRestartActionCommand FirmwareRestartAction = "command"
	FirmwareRestartActionCheetah FirmwareRestartAction = "cheetah"
	FirmwareRestartActionArduino FirmwareRestartAction = "arduino"
)

type FirmwareRestartPlan struct {
	Skip   bool
	Action FirmwareRestartAction
}

func BuildFirmwareRestartPlan(force bool, isMCUBridge bool, restartMethod string) FirmwareRestartPlan {
	if isMCUBridge && !force {
		return FirmwareRestartPlan{Skip: true}
	}
	switch restartMethod {
	case "rpi_usb":
		return FirmwareRestartPlan{Action: FirmwareRestartActionRPIUSB}
	case "command":
		return FirmwareRestartPlan{Action: FirmwareRestartActionCommand}
	case "cheetah":
		return FirmwareRestartPlan{Action: FirmwareRestartActionCheetah}
	default:
		return FirmwareRestartPlan{Action: FirmwareRestartActionArduino}
	}
}
