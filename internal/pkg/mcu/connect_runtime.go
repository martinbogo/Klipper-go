package mcu

import "strings"

type ConnectRuntimeHooks struct {
	QuerySnapshot     func() *ConfigSnapshot
	RestartMethod     string
	StartReason       string
	MCUName           string
	SendConfig        func(prevCRC *uint32) string
	TriggerRestart    func(reason string)
	RestartRequested  func() bool
	IsFileoutput      bool
	ReservedMoveSlots int64
}

type ConnectRuntimeResult struct {
	Snapshot            *ConfigSnapshot
	MoveCount           int64
	ReturnError         string
	PanicMessage        string
	WrapPanicInMCUError bool
	RestartPending      bool
}

func RunConnectRuntime(hooks ConnectRuntimeHooks) ConnectRuntimeResult {
	var snapshot *ConfigSnapshot
	if hooks.QuerySnapshot != nil {
		snapshot = hooks.QuerySnapshot()
	}
	decision := BuildConnectDecision(snapshot, hooks.RestartMethod, hooks.StartReason, hooks.MCUName)
	if decision.ReturnError != "" {
		return ConnectRuntimeResult{ReturnError: decision.ReturnError}
	}
	if decision.PanicMessage != "" {
		return ConnectRuntimeResult{PanicMessage: decision.PanicMessage}
	}
	if decision.NeedsPreConfigReset && hooks.TriggerRestart != nil {
		hooks.TriggerRestart("full reset before config")
		if hooks.RestartRequested != nil && hooks.RestartRequested() {
			return ConnectRuntimeResult{RestartPending: true}
		}
	}
	if decision.SendConfig && hooks.SendConfig != nil {
		var prevCRC *uint32
		if decision.UsePrevCRC {
			prev := decision.PrevCRC
			prevCRC = &prev
		}
		if errorMessage := hooks.SendConfig(prevCRC); errorMessage != "" {
			if hooks.TriggerRestart != nil {
				hooks.TriggerRestart("CRC mismatch")
			}
			return ConnectRuntimeResult{PanicMessage: errorMessage}
		}
	}
	if decision.NeedsRequery && hooks.QuerySnapshot != nil {
		snapshot = hooks.QuerySnapshot()
	}
	result := ConnectRuntimeResult{Snapshot: snapshot}
	if snapshot != nil {
		result.MoveCount = snapshot.MoveCount
	}
	if errorMessage := ValidateConfiguredSnapshot(snapshot, hooks.IsFileoutput, hooks.ReservedMoveSlots, hooks.MCUName); errorMessage != "" {
		result.PanicMessage = errorMessage
		result.WrapPanicInMCUError = strings.HasPrefix(errorMessage, "Too few moves available")
	}
	return result
}
