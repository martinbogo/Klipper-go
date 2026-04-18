package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultGPIOBasePath = "/sys/class/gpio"
)

type printerProfile struct {
	Name      string
	LinuxGPIO int
}

var printerProfiles = map[string]printerProfile{
	"ks1":        {Name: "Kobra S1", LinuxGPIO: 116},
	"kobras1":    {Name: "Kobra S1", LinuxGPIO: 116},
	"ks1m":       {Name: "Kobra S1 Max", LinuxGPIO: 116},
	"kobras1max": {Name: "Kobra S1 Max", LinuxGPIO: 116},
}

func normalizeProfile(name string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	return replacer.Replace(strings.ToLower(strings.TrimSpace(name)))
}

func lookupProfile(name string) (printerProfile, error) {
	profile, ok := printerProfiles[normalizeProfile(name)]
	if !ok {
		return printerProfile{}, fmt.Errorf("unsupported printer profile %q", name)
	}
	return profile, nil
}

func gpioBasePath(sysfsRoot string) string {
	if sysfsRoot == "" || sysfsRoot == "/" {
		return defaultGPIOBasePath
	}
	return filepath.Join(sysfsRoot, "sys", "class", "gpio")
}

func gpioPaths(base string, gpio int) (gpioDir string, directionPath string, valuePath string, exportPath string, unexportPath string) {
	gpioDir = filepath.Join(base, fmt.Sprintf("gpio%d", gpio))
	return gpioDir,
		filepath.Join(gpioDir, "direction"),
		filepath.Join(gpioDir, "value"),
		filepath.Join(base, "export"),
		filepath.Join(base, "unexport")
}

func ensureExported(base string, gpio int) error {
	gpioDir, _, _, exportPath, _ := gpioPaths(base, gpio)
	if _, err := os.Stat(gpioDir); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat gpio directory: %w", err)
	}

	gpioValue := strconv.Itoa(gpio)
	if err := os.WriteFile(exportPath, []byte(gpioValue), 0o644); err != nil {
		if _, statErr := os.Stat(gpioDir); statErr == nil {
			return nil
		}
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "device or resource busy") || strings.Contains(errMsg, "busy") {
			return nil
		}
		return fmt.Errorf("export gpio%d via %s: %w", gpio, exportPath, err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(gpioDir); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(gpioDir); err == nil {
		return nil
	}
	return fmt.Errorf("gpio%d did not appear at %s after export", gpio, gpioDir)
}

func writeGPIOFile(path string, value string) error {
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return fmt.Errorf("write %q to %s: %w", value, path, err)
	}
	return nil
}

func pulseGPIO(sysfsRoot string, gpio int, preDelay time.Duration, holdDuration time.Duration, cleanup bool, sleeper func(time.Duration)) error {
	if sleeper == nil {
		sleeper = time.Sleep
	}
	base := gpioBasePath(sysfsRoot)
	gpioDir, directionPath, valuePath, _, unexportPath := gpioPaths(base, gpio)
	if err := ensureExported(base, gpio); err != nil {
		return err
	}
	if err := writeGPIOFile(directionPath, "out"); err != nil {
		return err
	}
	if err := writeGPIOFile(valuePath, "0"); err != nil {
		return err
	}
	if preDelay > 0 {
		sleeper(preDelay)
	}
	if err := writeGPIOFile(valuePath, "1"); err != nil {
		return err
	}
	if holdDuration > 0 {
		sleeper(holdDuration)
	}
	if err := writeGPIOFile(valuePath, "0"); err != nil {
		return err
	}
	if cleanup {
		if err := os.WriteFile(unexportPath, []byte(strconv.Itoa(gpio)), 0o644); err != nil {
			return fmt.Errorf("unexport gpio%d from %s: %w", gpio, unexportPath, err)
		}
	}
	log.Printf("MCU reset pulse complete on gpio%d (%s)", gpio, gpioDir)
	return nil
}

func main() {
	if runtime.GOOS != "linux" {
		log.Fatal("mcu_reset must run on Linux")
	}

	printerName := flag.String("printer", "ks1", "printer profile: ks1 or ks1m")
	gpioOverride := flag.Int("gpio", -1, "override Linux sysfs GPIO number")
	preDelay := flag.Duration("pre-delay", time.Second, "delay after export/direction setup before asserting the line")
	holdDuration := flag.Duration("hold", time.Second, "time to hold the reset line high")
	cleanup := flag.Bool("cleanup", false, "unexport the GPIO after pulsing it")
	sysfsRoot := flag.String("sysfs-root", "/", "root prefix used to locate /sys/class/gpio")
	flag.Parse()

	profile, err := lookupProfile(*printerName)
	if err != nil {
		log.Fatal(err)
	}
	gpio := profile.LinuxGPIO
	if *gpioOverride >= 0 {
		gpio = *gpioOverride
	}

	log.Printf("Resetting MCU for %s via Linux sysfs gpio%d", profile.Name, gpio)
	if err := pulseGPIO(*sysfsRoot, gpio, *preDelay, *holdDuration, *cleanup, time.Sleep); err != nil {
		log.Fatal(err)
	}
}
