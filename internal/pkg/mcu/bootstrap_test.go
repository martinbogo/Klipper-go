package mcu

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildConfigPlanPrependsAllocateAndFinalize(t *testing.T) {
	plan := BuildConfigPlan(3, []string{"config_a pin=PA1", "config_b pin=PA2"}, []string{"restart_a"}, []string{"init_a"}, func(cmd string) string {
		return strings.ReplaceAll(cmd, "PA", "PB")
	})
	if len(plan.ConfigCmds) != 4 {
		t.Fatalf("expected 4 config commands, got %d", len(plan.ConfigCmds))
	}
	if plan.ConfigCmds[0] != "allocate_oids count=3" {
		t.Fatalf("unexpected allocate command %q", plan.ConfigCmds[0])
	}
	if plan.ConfigCmds[1] != "config_a pin=PB1" || plan.ConfigCmds[2] != "config_b pin=PB2" {
		t.Fatalf("expected updated config commands, got %#v", plan.ConfigCmds)
	}
	if plan.ConfigCmds[3] != fmt.Sprintf("finalize_config crc=%d", plan.ConfigCRC) {
		t.Fatalf("unexpected finalize command %q", plan.ConfigCmds[3])
	}
	if len(plan.RestartCmds) != 1 || plan.RestartCmds[0] != "restart_a" {
		t.Fatalf("unexpected restart commands %#v", plan.RestartCmds)
	}
	if len(plan.InitCmds) != 1 || plan.InitCmds[0] != "init_a" {
		t.Fatalf("unexpected init commands %#v", plan.InitCmds)
	}
}

func TestBuildConfigPlanSupportsNilUpdater(t *testing.T) {
	plan := BuildConfigPlan(1, []string{"config_a"}, nil, nil, nil)
	if plan.ConfigCmds[1] != "config_a" {
		t.Fatalf("expected command to remain unchanged, got %#v", plan.ConfigCmds)
	}
}

func TestBuildConnectionPlanModes(t *testing.T) {
	tests := []struct {
		name      string
		plan      ConnectionPlan
		expected  ConnectionMode
		clockSync bool
	}{
		{"fileoutput", BuildConnectionPlan(true, "command", "/tmp/out", 0, false, true), ConnectionModeFileoutput, false},
		{"canbus", BuildConnectionPlan(false, "command", "canbus", 0, true, true), ConnectionModeCanbus, true},
		{"remote", BuildConnectionPlan(false, "command", "tcp@host", 0, false, true), ConnectionModeRemote, true},
		{"uart", BuildConnectionPlan(false, "command", "/dev/ttyUSB0", 250000, false, true), ConnectionModeUART, true},
		{"pipe", BuildConnectionPlan(false, "command", "/tmp/pipe", 0, false, true), ConnectionModePipe, true},
	}
	for _, test := range tests {
		if test.plan.Mode != test.expected {
			t.Fatalf("%s: expected mode %s, got %s", test.name, test.expected, test.plan.Mode)
		}
		if test.plan.NeedsClockSyncConnect != test.clockSync {
			t.Fatalf("%s: expected clock sync %v, got %v", test.name, test.clockSync, test.plan.NeedsClockSyncConnect)
		}
	}
}

func TestBuildConnectionPlanTracksResetAndRTS(t *testing.T) {
	plan := BuildConnectionPlan(false, "rpi_usb", "/dev/ttyUSB0", 250000, false, false)
	if !plan.NeedsPowerEnableReset {
		t.Fatalf("expected missing rpi_usb path to request power-enable reset")
	}
	if !plan.RTS {
		t.Fatalf("expected non-cheetah UART plan to keep RTS enabled")
	}
	cheetahPlan := BuildConnectionPlan(false, "cheetah", "/dev/ttyUSB0", 250000, false, true)
	if cheetahPlan.RTS {
		t.Fatalf("expected cheetah UART plan to disable RTS")
	}
}
