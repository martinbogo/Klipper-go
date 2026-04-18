package mcu

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConfigSessionTracksCallbacksAndCommands(t *testing.T) {
	session := NewConfigSession()
	callbackRuns := 0
	session.RegisterCallback(func() {
		callbackRuns++
	})
	session.AddCommand("config_a", false, false)
	session.AddCommand("restart_a", false, true)
	session.AddCommand("init_a", true, false)
	session.RunCallbacks()
	if callbackRuns != 1 {
		t.Fatalf("expected one callback run, got %d", callbackRuns)
	}
	if !reflect.DeepEqual(session.ConfigCmds(), []string{"config_a"}) {
		t.Fatalf("unexpected config commands %#v", session.ConfigCmds())
	}
	if !reflect.DeepEqual(session.RestartCmds(), []string{"restart_a"}) {
		t.Fatalf("unexpected restart commands %#v", session.RestartCmds())
	}
	if !reflect.DeepEqual(session.InitCmds(), []string{"init_a"}) {
		t.Fatalf("unexpected init commands %#v", session.InitCmds())
	}
}

func TestSendConfigSessionBuildsAndStoresCommands(t *testing.T) {
	session := NewConfigSession()
	session.RegisterCallback(func() {})
	session.AddCommand("config_a pin=PA1", false, false)
	session.AddCommand("restart_a pin=PA1", false, true)
	session.AddCommand("init_a pin=PA1", true, false)
	sent := []string{}
	registered := 0
	errorMessage := SendConfigSession(session, 2,
		func() int { return 2 },
		func(command string) string { return strings.ReplaceAll(command, "PA", "PB") },
		func(func(map[string]interface{}) error) {
			registered++
		},
		func(command string) {
			sent = append(sent, command)
		},
		"mcu", nil, nil)
	if errorMessage != "" {
		t.Fatalf("unexpected error %q", errorMessage)
	}
	if registered != 1 {
		t.Fatalf("expected starting handler registration, got %d", registered)
	}
	configCmds := session.ConfigCmds()
	if len(configCmds) != 3 || configCmds[1] != "config_a pin=PB1" {
		t.Fatalf("unexpected stored config commands %#v", configCmds)
	}
	expectedSent := []string{configCmds[0], configCmds[1], configCmds[2], "init_a pin=PB1"}
	if !reflect.DeepEqual(sent, expectedSent) {
		t.Fatalf("unexpected sent commands %#v", sent)
	}
	if !reflect.DeepEqual(session.RestartCmds(), []string{"restart_a pin=PB1"}) {
		t.Fatalf("unexpected stored restart commands %#v", session.RestartCmds())
	}
}

func TestSendConfigSessionIncludesCommandsAddedByCallbacks(t *testing.T) {
	session := NewConfigSession()
	callbackRuns := 0
	session.RegisterCallback(func() {
		callbackRuns++
		session.AddCommand("config_from_callback pin=PA2", false, false)
		session.AddCommand("restart_from_callback pin=PA2", false, true)
		session.AddCommand("init_from_callback pin=PA2", true, false)
	})
	sent := []string{}
	errorMessage := SendConfigSession(session, 1,
		func() int { return 1 },
		func(command string) string { return strings.ReplaceAll(command, "PA", "PB") },
		nil,
		func(command string) {
			sent = append(sent, command)
		},
		"mcu", nil, nil)
	if errorMessage != "" {
		t.Fatalf("unexpected error %q", errorMessage)
	}
	if callbackRuns != 1 {
		t.Fatalf("expected one callback run, got %d", callbackRuns)
	}
	configCmds := session.ConfigCmds()
	if len(configCmds) != 3 || configCmds[1] != "config_from_callback pin=PB2" {
		t.Fatalf("expected callback-generated config commands, got %#v", configCmds)
	}
	if !reflect.DeepEqual(session.RestartCmds(), []string{"restart_from_callback pin=PB2"}) {
		t.Fatalf("unexpected callback-generated restart commands %#v", session.RestartCmds())
	}
	if !reflect.DeepEqual(session.InitCmds(), []string{"init_from_callback pin=PB2"}) {
		t.Fatalf("unexpected callback-generated init commands %#v", session.InitCmds())
	}
	expectedSent := []string{configCmds[0], configCmds[1], configCmds[2], "init_from_callback pin=PB2"}
	if !reflect.DeepEqual(sent, expectedSent) {
		t.Fatalf("unexpected sent commands %#v", sent)
	}
}

func TestSendConfigSessionUsesUpdatedOIDCountAfterCallbacks(t *testing.T) {
	session := NewConfigSession()
	oidCount := 1
	session.RegisterCallback(func() {
		oidCount = 4
		session.AddCommand("config_from_callback pin=PA3", false, false)
	})
	sent := []string{}
	errorMessage := SendConfigSession(session, oidCount,
		func() int { return oidCount },
		func(command string) string { return strings.ReplaceAll(command, "PA", "PB") },
		nil,
		func(command string) {
			sent = append(sent, command)
		},
		"mcu", nil, nil)
	if errorMessage != "" {
		t.Fatalf("unexpected error %q", errorMessage)
	}
	configCmds := session.ConfigCmds()
	if len(configCmds) != 3 {
		t.Fatalf("unexpected config commands %#v", configCmds)
	}
	if configCmds[0] != "allocate_oids count=4" {
		t.Fatalf("expected updated oid count, got %#v", configCmds)
	}
	expectedSent := []string{configCmds[0], configCmds[1], configCmds[2]}
	if !reflect.DeepEqual(sent, expectedSent) {
		t.Fatalf("unexpected sent commands %#v", sent)
	}
}

func TestSendConfigBuildsAndSendsCommands(t *testing.T) {
	sent := []string{}
	registered := 0
	callbackRuns := 0
	configCmds := []string{"config_a pin=PA1"}
	restartCmds := []string{"restart_a pin=PA1"}
	initCmds := []string{"init_a pin=PA1"}
	hooks := ConfigSendHooks{
		RunCallbacks: func() {
			callbackRuns++
		},
		OIDCount:      2,
		ConfigCmds:    configCmds,
		RestartCmds:   restartCmds,
		InitCmds:      initCmds,
		UpdateCommand: func(command string) string { return strings.ReplaceAll(command, "PA", "PB") },
		SetCommands: func(config []string, restart []string, init []string) {
			configCmds = append([]string{}, config...)
			restartCmds = append([]string{}, restart...)
			initCmds = append([]string{}, init...)
		},
		RegisterStartingHandler: func(func(map[string]interface{}) error) {
			registered++
		},
		SendCommand: func(command string) {
			sent = append(sent, command)
		},
		MCUName: "mcu",
	}

	if errorMessage := SendConfig(hooks, nil, nil); errorMessage != "" {
		t.Fatalf("unexpected error %q", errorMessage)
	}
	if callbackRuns != 1 {
		t.Fatalf("expected one callback run, got %d", callbackRuns)
	}
	if registered != 1 {
		t.Fatalf("expected starting handler registration, got %d", registered)
	}
	if len(configCmds) != 3 {
		t.Fatalf("expected built config command list, got %#v", configCmds)
	}
	if configCmds[0] != "allocate_oids count=2" {
		t.Fatalf("unexpected first config command %q", configCmds[0])
	}
	if configCmds[1] != "config_a pin=PB1" {
		t.Fatalf("expected updated config command, got %#v", configCmds)
	}
	expectedSent := []string{configCmds[0], configCmds[1], configCmds[2], "init_a pin=PB1"}
	if !reflect.DeepEqual(sent, expectedSent) {
		t.Fatalf("unexpected sent commands %#v", sent)
	}
	if len(restartCmds) != 1 || restartCmds[0] != "restart_a pin=PB1" {
		t.Fatalf("unexpected restart commands %#v", restartCmds)
	}
	if len(initCmds) != 1 || initCmds[0] != "init_a pin=PB1" {
		t.Fatalf("unexpected init commands %#v", initCmds)
	}
}

func TestSendConfigReturnsCRCMismatch(t *testing.T) {
	sent := 0
	registered := 0
	hooks := ConfigSendHooks{
		OIDCount:    1,
		ConfigCmds:  []string{"config_a"},
		SetCommands: func([]string, []string, []string) {},
		RegisterStartingHandler: func(func(map[string]interface{}) error) {
			registered++
		},
		SendCommand: func(string) {
			sent++
		},
		MCUName: "mcu",
	}
	prevCRC := uint32(1)
	errorMessage := SendConfig(hooks, &prevCRC, nil)
	if errorMessage == "" {
		t.Fatalf("expected CRC mismatch error")
	}
	if sent != 0 {
		t.Fatalf("expected no commands to be sent on mismatch")
	}
	if registered != 0 {
		t.Fatalf("expected no handler registration on mismatch")
	}
}

func TestQueryConfigSnapshotClearsShutdown(t *testing.T) {
	queries := []map[string]interface{}{
		{"is_config": int64(1), "crc": int64(12), "is_shutdown": int64(1), "move_count": int64(6)},
		{"is_config": int64(1), "crc": int64(12), "is_shutdown": int64(0), "move_count": int64(6)},
	}
	queryIndex := 0
	clearSent := 0
	shutdownCleared := 0
	slept := time.Duration(0)
	result := QueryConfigSnapshot(ConfigQueryHooks{
		QueryConfig: func() map[string]interface{} {
			current := queries[queryIndex]
			if queryIndex < len(queries)-1 {
				queryIndex++
			}
			return current
		},
		MCUName:          "mcu",
		HasClearShutdown: true,
		SendClearShutdown: func() {
			clearSent++
		},
		ClearLocalShutdown: func() {
			shutdownCleared++
		},
		Sleep: func(duration time.Duration) {
			slept = duration
		},
	})
	if result.ErrorMessage != "" {
		t.Fatalf("unexpected query error %q", result.ErrorMessage)
	}
	if result.Snapshot == nil || result.Snapshot.IsShutdown {
		t.Fatalf("expected cleared snapshot, got %#v", result.Snapshot)
	}
	if clearSent != 1 {
		t.Fatalf("expected clear_shutdown send, got %d", clearSent)
	}
	if shutdownCleared != 1 {
		t.Fatalf("expected local shutdown clear, got %d", shutdownCleared)
	}
	if slept != 100*time.Millisecond {
		t.Fatalf("expected 100ms sleep, got %s", slept)
	}
}

func TestQueryConfigSnapshotReturnsShutdownError(t *testing.T) {
	result := QueryConfigSnapshot(ConfigQueryHooks{
		QueryConfig: func() map[string]interface{} {
			return map[string]interface{}{"is_config": int64(1), "is_shutdown": int64(0), "move_count": int64(3)}
		},
		IsShutdown:      true,
		ShutdownMessage: "boom",
		MCUName:         "mcu",
	})
	if result.ErrorMessage == "" {
		t.Fatalf("expected shutdown error")
	}
	if result.Snapshot == nil || !result.Snapshot.IsConfig {
		t.Fatalf("expected parsed snapshot in result, got %#v", result.Snapshot)
	}
	if !strings.Contains(result.ErrorMessage, "boom") {
		t.Fatalf("expected shutdown message in error, got %q", result.ErrorMessage)
	}
}
