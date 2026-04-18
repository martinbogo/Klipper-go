package mcu

import "testing"

func TestRunConnectRuntimeUnconfiguredSnapshotRequeriesAndConfigures(t *testing.T) {
	queries := 0
	sendConfigCalls := 0
	result := RunConnectRuntime(ConnectRuntimeHooks{
		QuerySnapshot: func() *ConfigSnapshot {
			queries++
			if queries == 1 {
				return &ConfigSnapshot{IsConfig: false}
			}
			return &ConfigSnapshot{IsConfig: true, MoveCount: 8}
		},
		RestartMethod:     "command",
		StartReason:       "start",
		MCUName:           "mcu",
		ReservedMoveSlots: 2,
		SendConfig: func(prevCRC *uint32) string {
			sendConfigCalls++
			if prevCRC != nil {
				t.Fatalf("expected new config send to omit previous CRC, got %v", *prevCRC)
			}
			return ""
		},
	})
	if result.ReturnError != "" || result.PanicMessage != "" {
		t.Fatalf("unexpected connect result %#v", result)
	}
	if queries != 2 {
		t.Fatalf("expected one initial query and one requery, got %d", queries)
	}
	if sendConfigCalls != 1 {
		t.Fatalf("expected one config send, got %d", sendConfigCalls)
	}
	if result.MoveCount != 8 || result.Snapshot == nil || !result.Snapshot.IsConfig {
		t.Fatalf("unexpected configured snapshot %#v", result)
	}
}

func TestRunConnectRuntimeConfiguredSnapshotSendsPreviousCRC(t *testing.T) {
	result := RunConnectRuntime(ConnectRuntimeHooks{
		QuerySnapshot: func() *ConfigSnapshot {
			return &ConfigSnapshot{IsConfig: true, CRC: 42, MoveCount: 6}
		},
		RestartMethod:     "command",
		StartReason:       "start",
		MCUName:           "mcu",
		ReservedMoveSlots: 1,
		SendConfig: func(prevCRC *uint32) string {
			if prevCRC == nil || *prevCRC != 42 {
				t.Fatalf("expected previous CRC 42, got %#v", prevCRC)
			}
			return ""
		},
	})
	if result.ReturnError != "" || result.PanicMessage != "" {
		t.Fatalf("unexpected connect result %#v", result)
	}
	if result.MoveCount != 6 {
		t.Fatalf("expected move count 6, got %#v", result)
	}
}

func TestRunConnectRuntimePreConfigResetAbortsWhenRestartRequested(t *testing.T) {
	restartRequested := false
	sendConfigCalls := 0
	queries := 0
	result := RunConnectRuntime(ConnectRuntimeHooks{
		QuerySnapshot: func() *ConfigSnapshot {
			queries++
			return &ConfigSnapshot{IsConfig: false}
		},
		RestartMethod: "rpi_usb",
		StartReason:   "start",
		MCUName:       "nozzle_mcu",
		TriggerRestart: func(reason string) {
			if reason != "full reset before config" {
				t.Fatalf("unexpected restart reason %q", reason)
			}
			restartRequested = true
		},
		RestartRequested: func() bool {
			return restartRequested
		},
		SendConfig: func(prevCRC *uint32) string {
			sendConfigCalls++
			return ""
		},
	})
	if !result.RestartPending {
		t.Fatalf("expected restart-pending result, got %#v", result)
	}
	if result.PanicMessage != "" || result.ReturnError != "" {
		t.Fatalf("expected clean restart handoff, got %#v", result)
	}
	if queries != 1 {
		t.Fatalf("expected a single pre-restart query, got %d", queries)
	}
	if sendConfigCalls != 0 {
		t.Fatalf("expected config send to be skipped once restart was requested, got %d calls", sendConfigCalls)
	}
}

func TestRunConnectRuntimeConfigSendFailureRequestsRestart(t *testing.T) {
	restartReasons := []string{}
	result := RunConnectRuntime(ConnectRuntimeHooks{
		QuerySnapshot: func() *ConfigSnapshot {
			return &ConfigSnapshot{IsConfig: true, CRC: 7, MoveCount: 6}
		},
		RestartMethod: "command",
		StartReason:   "start",
		MCUName:       "mcu",
		SendConfig: func(prevCRC *uint32) string {
			return "crc mismatch"
		},
		TriggerRestart: func(reason string) {
			restartReasons = append(restartReasons, reason)
		},
	})
	if result.PanicMessage != "crc mismatch" {
		t.Fatalf("expected config send panic message, got %#v", result)
	}
	if len(restartReasons) != 1 || restartReasons[0] != "CRC mismatch" {
		t.Fatalf("expected CRC mismatch restart request, got %#v", restartReasons)
	}
}

func TestRunConnectRuntimeFlagsTooFewMoveSlotsAsMCUError(t *testing.T) {
	result := RunConnectRuntime(ConnectRuntimeHooks{
		QuerySnapshot: func() *ConfigSnapshot {
			return &ConfigSnapshot{IsConfig: true, CRC: 7, MoveCount: 2}
		},
		RestartMethod:     "command",
		StartReason:       "start",
		MCUName:           "mcu",
		ReservedMoveSlots: 3,
		SendConfig: func(prevCRC *uint32) string {
			return ""
		},
	})
	if result.PanicMessage == "" {
		t.Fatalf("expected validation panic result, got %#v", result)
	}
	if !result.WrapPanicInMCUError {
		t.Fatalf("expected too-few-moves panic to be wrapped as MCU error, got %#v", result)
	}
}
