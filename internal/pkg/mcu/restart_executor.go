package mcu

import "time"

type CommandResetExecutionHooks struct {
	DebugLog          func(string)
	MarkShutdown      func()
	SendEmergencyStop func(force bool)
	PauseSeconds      func(float64)
	SendConfigReset   func()
	SendReset         func()
	Sleep             func(time.Duration)
	Disconnect        func()
}

func ExecuteCommandReset(plan CommandResetPlan, hooks CommandResetExecutionHooks) string {
	if plan.ErrorMessage != "" {
		return plan.ErrorMessage
	}
	if hooks.DebugLog != nil {
		hooks.DebugLog(plan.LogMessage)
	}
	if plan.Mode == CommandResetModeConfigReset {
		if plan.MarkShutdown && hooks.MarkShutdown != nil {
			hooks.MarkShutdown()
		}
		if plan.NeedsEmergencyStop && hooks.SendEmergencyStop != nil {
			hooks.SendEmergencyStop(true)
		}
		if hooks.PauseSeconds != nil {
			hooks.PauseSeconds(plan.PreSendPauseSeconds)
		}
		if hooks.SendConfigReset != nil {
			hooks.SendConfigReset()
		}
	} else if hooks.SendReset != nil {
		hooks.SendReset()
	}
	if hooks.Sleep != nil {
		hooks.Sleep(time.Duration(plan.PostSendPauseSeconds * float64(time.Second)))
	}
	if hooks.Disconnect != nil {
		hooks.Disconnect()
	}
	return ""
}

type FirmwareRestartExecutionHooks struct {
	RestartRPIUSB     func()
	RestartViaCommand func()
	RestartCheetah    func()
	RestartArduino    func()
}

func ExecuteFirmwareRestartPlan(plan FirmwareRestartPlan, hooks FirmwareRestartExecutionHooks) {
	if plan.Skip {
		return
	}
	switch plan.Action {
	case FirmwareRestartActionRPIUSB:
		if hooks.RestartRPIUSB != nil {
			hooks.RestartRPIUSB()
		}
	case FirmwareRestartActionCommand:
		if hooks.RestartViaCommand != nil {
			hooks.RestartViaCommand()
		}
	case FirmwareRestartActionCheetah:
		if hooks.RestartCheetah != nil {
			hooks.RestartCheetah()
		}
	default:
		if hooks.RestartArduino != nil {
			hooks.RestartArduino()
		}
	}
}
