package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLookupProfileAcceptsAliases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "ks1", want: "Kobra S1"},
		{input: "Kobra S1", want: "Kobra S1"},
		{input: "ks1m", want: "Kobra S1 Max"},
		{input: "Kobra-S1-Max", want: "Kobra S1 Max"},
	}

	for _, tt := range tests {
		profile, err := lookupProfile(tt.input)
		if err != nil {
			t.Fatalf("lookupProfile(%q) error = %v", tt.input, err)
		}
		if profile.Name != tt.want {
			t.Fatalf("lookupProfile(%q) name = %q, want %q", tt.input, profile.Name, tt.want)
		}
		if profile.LinuxGPIO != 116 {
			t.Fatalf("lookupProfile(%q) gpio = %d, want 116", tt.input, profile.LinuxGPIO)
		}
	}
}

func TestGPIOBasePathUsesSysfsRoot(t *testing.T) {
	if got := gpioBasePath("/"); got != "/sys/class/gpio" {
		t.Fatalf("gpioBasePath(/) = %q, want %q", got, "/sys/class/gpio")
	}
	if got := gpioBasePath("/tmp/test-root"); got != "/tmp/test-root/sys/class/gpio" {
		t.Fatalf("gpioBasePath(custom) = %q", got)
	}
}

func TestPulseGPIOWritesDirectionValueAndCleanup(t *testing.T) {
	root := t.TempDir()
	gpioDir := filepath.Join(root, "sys", "class", "gpio", "gpio116")
	if err := os.MkdirAll(gpioDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for _, relativePath := range []string{
		"sys/class/gpio/export",
		"sys/class/gpio/unexport",
		"sys/class/gpio/gpio116/direction",
		"sys/class/gpio/gpio116/value",
	} {
		path := filepath.Join(root, relativePath)
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	var sleeps []time.Duration
	if err := pulseGPIO(root, 116, 25*time.Millisecond, 50*time.Millisecond, true, func(d time.Duration) {
		sleeps = append(sleeps, d)
	}); err != nil {
		t.Fatalf("pulseGPIO() error = %v", err)
	}

	directionBytes, err := os.ReadFile(filepath.Join(root, "sys", "class", "gpio", "gpio116", "direction"))
	if err != nil {
		t.Fatalf("ReadFile(direction) error = %v", err)
	}
	if string(directionBytes) != "out" {
		t.Fatalf("direction = %q, want %q", string(directionBytes), "out")
	}

	valueBytes, err := os.ReadFile(filepath.Join(root, "sys", "class", "gpio", "gpio116", "value"))
	if err != nil {
		t.Fatalf("ReadFile(value) error = %v", err)
	}
	if string(valueBytes) != "0" {
		t.Fatalf("final value = %q, want %q", string(valueBytes), "0")
	}

	unexportBytes, err := os.ReadFile(filepath.Join(root, "sys", "class", "gpio", "unexport"))
	if err != nil {
		t.Fatalf("ReadFile(unexport) error = %v", err)
	}
	if string(unexportBytes) != "116" {
		t.Fatalf("unexport = %q, want %q", string(unexportBytes), "116")
	}

	if len(sleeps) != 2 || sleeps[0] != 25*time.Millisecond || sleeps[1] != 50*time.Millisecond {
		t.Fatalf("sleeps = %#v, want [25ms 50ms]", sleeps)
	}
}
