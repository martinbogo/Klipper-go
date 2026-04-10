package fan

import (
	"fmt"
	"strconv"
	"strings"

	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

const controllerFanPinMinTime = 0.100

type moduleMotorEnableAdapter struct {
	line printerpkg.StepperEnableLine
}

func (self *moduleMotorEnableAdapter) Is_motor_enabled() bool {
	return self.line.IsMotorEnabled()
}

type ControllerFanModule struct {
	printer     printerpkg.ModulePrinter
	heaterNames []string
	core        *ControllerFan
	fan         *Fan
}

func LoadConfigControllerFan(config printerpkg.ModuleConfig) interface{} {
	return NewControllerFanModule(config)
}

func NewControllerFanModule(config printerpkg.ModuleConfig) *ControllerFanModule {
	_ = config.LoadObject("stepper_enable")
	_ = config.LoadObject("heaters")
	printer := config.Printer()
	fan := newConfiguredFan(config)
	fanSpeed := config.Float("fan_speed", 1.0)
	self := &ControllerFanModule{
		printer:     printer,
		heaterNames: parseConfigList(config.String("heater", "extruder", true)),
		core: NewControllerFan(
			parseConfigList(config.String("stepper", "", true)),
			fan,
			fanSpeed,
			config.Float("idle_speed", fanSpeed),
			parseNonNegativeIntConfig(config, "idle_timeout", 30),
		),
		fan: fan,
	}
	printer.RegisterEventHandler("project:ready", self.handleReady)
	printer.RegisterEventHandler("project:connect", self.handleConnect)
	printer.RegisterEventHandler("gcode:request_restart", self.handleRequestRestart)
	return self
}

func parseNonNegativeIntConfig(config printerpkg.ModuleConfig, option string, defaultValue int) int {
	raw := strings.TrimSpace(config.String(option, strconv.Itoa(defaultValue), true))
	value, err := strconv.Atoi(raw)
	if err != nil {
		panic(fmt.Sprintf("Unable to parse option '%s' in section '%s'", option, config.Name()))
	}
	if value < 0 {
		panic(fmt.Sprintf("Option '%s' in section '%s' must have minimum of %d", option, config.Name(), 0))
	}
	return value
}

func (self *ControllerFanModule) handleConnect([]interface{}) error {
	coreHeaters := make([]Heater, 0, len(self.heaterNames))
	for _, name := range self.heaterNames {
		coreHeaters = append(coreHeaters, &moduleHeaterAdapter{heater: self.printer.LookupHeater(name)})
	}
	self.core.SetHeaters(coreHeaters)
	return self.core.ResolveSteppers(self.printer.StepperEnable().StepperNames())
}

func (self *ControllerFanModule) handleReady([]interface{}) error {
	reactor := self.printer.Reactor()
	reactor.RegisterTimer(self.callback, reactor.Monotonic()+controllerFanPinMinTime)
	return nil
}

func (self *ControllerFanModule) handleRequestRestart(args []interface{}) error {
	if len(args) == 0 {
		panic("missing controller_fan restart print time")
	}
	printTime, ok := args[0].(float64)
	if !ok {
		panic("unexpected controller_fan restart print time type")
	}
	self.fan.HandleRequestRestart(printTime)
	return nil
}

func (self *ControllerFanModule) Get_status(eventtime float64) map[string]float64 {
	return self.core.Get_status(eventtime)
}

func (self *ControllerFanModule) callback(eventtime float64) float64 {
	return self.core.Callback(eventtime,
		func(name string) (MotorEnable, error) {
			line, err := self.printer.StepperEnable().LookupEnable(name)
			if err != nil {
				return nil, err
			}
			return &moduleMotorEnableAdapter{line: line}, nil
		},
		func(name string, err error) {
			logger.Errorf("stepper_enable Lookup_enable %s error: %v\n", name, err)
		},
	)
}