package addon

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

type LEDModule struct {
	printer      printerpkg.ModulePrinter
	name         string
	gpioNumber   int // -1 by default
	pinName      string
	currentState bool
	initialized  bool
	enabled      bool
	timeout      time.Duration
	lastCommand  string
	lastError    string
	lastUpdated  time.Time
}

func LoadConfigLedDigitalOut(config printerpkg.ModuleConfig) interface{} {
	module := &LEDModule{
		printer:    config.Printer(),
		name:       config.Name(),
		gpioNumber: int(config.Float("gpio", -1.0)),
		timeout:    3 * time.Second,
	}

	module.pinName = config.String("pin", "", true)
	valueStr := config.String("initial_state", "true", true)
	module.currentState = parse_led_state(valueStr)

	if module.gpioNumber < 0 && module.pinName == "" {
		panic(fmt.Sprintf("%s: led config missing gpio identifier", config.Name()))
	}

	if module.gpioNumber < 0 && module.pinName != "" {
		// some heuristic or extraction if needed, but the original config used gpio directly.
	}

	module.initModule()

	return module
}

func (l *LEDModule) runShellCommand(command string) error {
	l.lastCommand = command
	l.lastUpdated = time.Now()

	cmd := exec.Command("sh", "-c", command)
	if err := cmd.Run(); err != nil {
		l.lastError = err.Error()
		logger.Errorf("led shell command failed: %s: %v", command, err)
		return err
	}
	l.lastError = ""
	return nil
}

func (l *LEDModule) setLed(state bool) error {
	if l.gpioNumber < 0 {
		return fmt.Errorf("invalid gpio number: %d", l.gpioNumber)
	}

	l.currentState = state
	value := 0
	if state {
		value = 1
	}

	command := fmt.Sprintf("echo %d > /sys/class/gpio/gpio%d/value", value, l.gpioNumber)
	if err := l.runShellCommand(command); err != nil {
		return err
	}

	logger.Infof("setLed: gpio=%d state=%t", l.gpioNumber, state)
	return nil
}

func (l *LEDModule) rv1106_set_led_gpio() error {
	if l.gpioNumber < 0 {
		return fmt.Errorf("invalid gpio number: %d", l.gpioNumber)
	}

	commands := []string{
		fmt.Sprintf("echo %d > /sys/class/gpio/export", l.gpioNumber),
		fmt.Sprintf("echo out > /sys/class/gpio/gpio%d/direction", l.gpioNumber),
	}
	for _, command := range commands {
		// Ignore errors for export, it might already be exported
		_ = l.runShellCommand(command)
	}

	l.initialized = true
	l.enabled = true
	return l.setLed(l.currentState)
}

func (l *LEDModule) cmd_M1011(gcmd printerpkg.Command) error {
	value := gcmd.Int("P", 0, nil, nil)
	return l.setLed(value != 0)
}

func (l *LEDModule) setLedEndpoint(request printerpkg.WebhookRequest) (interface{}, error) {
	params := request.GetParams()
	logger.Infof("setLedEndpoint called: full params = %#v", params)

	// Original args parser might have passed just the array if JSONRPC.
	stateStr := request.String("state", "")
	logger.Infof("setLedEndpoint: extracted stateStr=%q", stateStr)

	var state bool
	if val, ok := params["state"]; ok && val != nil && val != "" {
		state = parse_led_state(val)
	} else {
		// Try reading as int as an alternative debug step
		stateInt := request.Int("state", -1)
		logger.Infof("setLedEndpoint: fallback request.Int stateInt=%d", stateInt)
		if stateInt != -1 {
			state = (stateInt != 0)
		} else {
			// Toggle state if called with no arguments
			state = !l.currentState
		}
	}

	logger.Infof("setLedEndpoint: final state determined as %v", state)
	err := l.setLed(state)
	return nil, err
}

func (l *LEDModule) initModule() {
	gcode := l.printer.GCode().(printerpkg.GCodeRuntime)
	gcode.RegisterCommand("M1011", l.cmd_M1011, false, "Set Chamber LED")

	// Register webhook
	_ = l.printer.Webhooks().RegisterEndpointWithRequest("led/set_led", l.setLedEndpoint)

	l.rv1106_set_led_gpio()
}

func (l *LEDModule) Get_status(eventtime float64) map[string]interface{} {
	return map[string]interface{}{
		"gpio":        l.gpioNumber,
		"pin":         l.pinName,
		"initialized": l.initialized,
		"enabled":     l.enabled,
		"state":       l.currentState,
	}
}

func parse_led_state(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		if s == "" {
			return false
		}
		if parsed, err := strconv.Atoi(s); err == nil {
			return parsed != 0
		}
		return s == "on" || s == "true" || s == "enable" || s == "enabled"
	default:
		return false
	}
}
