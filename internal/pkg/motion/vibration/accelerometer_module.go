package vibration

import (
	"fmt"

	reportpkg "goklipper/internal/pkg/motion/report"
	printerpkg "goklipper/internal/pkg/printer"
)

type accelerometerModuleConfig interface {
	printerpkg.ModuleConfig
	Get_name() string
	Getlist(option string, defaultValue interface{}, sep string, count int, noteValid bool) interface{}
	Getint(option string, defaultValue interface{}, minval, maxval int, noteValid bool) int
}

type AccelerometerModuleSPITransport interface {
	accelerometerRuntimeSPI
	Get_oid() int
	MCU() AccelerometerModuleMCUTransport
	CommandQueue() interface{}
}

type AccelerometerModuleMCUTransport interface {
	accelerometerRuntimeMCU
	accelerometerCommandLookup
	Create_oid() int
	Add_config_cmd(cmd string, isInit bool, onRestart bool)
	Register_config_callback(cb interface{})
	Register_response(cb interface{}, msg string, oid interface{})
}

type accelerometerTimerAdapterProvider interface {
	TimerAdapter() *printerpkg.TimerReactorAdapter
}

type accelerometerModuleOptions struct {
	configCommand string
	queryCommand  string
	responseName  string
	apiUpdate     func(float64) map[string]interface{}
	apiStartStop  func(bool)
}

type accelerometerModule struct {
	printer printerpkg.ModulePrinter
	runtime *AccelerometerRuntime
}

func requireAccelerometerModuleConfig(config printerpkg.ModuleConfig) accelerometerModuleConfig {
	legacy, ok := config.(accelerometerModuleConfig)
	if !ok {
		panic(fmt.Sprintf("accelerometer config does not implement legacy getters: %T", config))
	}
	return legacy
}

func requireAccelTimerAdapter(printer printerpkg.ModulePrinter) *printerpkg.TimerReactorAdapter {
	provider, ok := printer.Reactor().(accelerometerTimerAdapterProvider)
	if !ok {
		panic(fmt.Sprintf("printer reactor does not expose timer adapter: %T", printer.Reactor()))
	}
	return provider.TimerAdapter()
}

func requireAccelQueryToolhead(printer printerpkg.ModulePrinter) accelQueryToolhead {
	toolheadObj := printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(accelQueryToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement accelQueryToolhead: %T", toolheadObj))
	}
	return toolhead
}

func accelAxesList(config accelerometerModuleConfig) []string {
	rawAxes := config.Getlist("axes_map", []string{"x", "y", "z"}, ",", 3, true)
	switch axes := rawAxes.(type) {
	case []string:
		return axes
	case []interface{}:
		values := make([]string, len(axes))
		for i, axis := range axes {
			values[i] = axis.(string)
		}
		return values
	default:
		panic(fmt.Errorf("unexpected axes_map type: %T", rawAxes))
	}
}

func (self *accelerometerModule) configure(
	config printerpkg.ModuleConfig,
	runtimeConfig AccelerometerRuntimeConfig,
	moduleConfig accelerometerModuleOptions,
	spi AccelerometerModuleSPITransport,
	registerConfig func(),
) (AccelerometerModuleSPITransport, AccelerometerModuleMCUTransport, int) {
	self.printer = config.Printer()
	mcu := spi.MCU()
	oid := mcu.Create_oid()
	runtimeConfig.SPI = spi
	runtimeConfig.MCU = mcu
	runtimeConfig.OID = oid
	self.runtime = NewAccelerometerRuntime(runtimeConfig)
	mcu.Add_config_cmd(fmt.Sprintf(moduleConfig.configCommand, oid, spi.Get_oid()), false, false)
	mcu.Add_config_cmd(fmt.Sprintf(moduleConfig.queryCommand, oid), false, true)
	mcu.Register_config_callback(registerConfig)
	mcu.Register_response(self.runtime.HandleSensorData, moduleConfig.responseName, oid)
	self.runtime.SetAPIDump(reportpkg.NewAPIDumpHelper(requireAccelTimerAdapter(self.printer), moduleConfig.apiUpdate, moduleConfig.apiStartStop, 0.100))
	return spi, mcu, oid
}

func (self *accelerometerModule) build_config(mcu AccelerometerModuleMCUTransport, cmdQueue interface{}, oid int, queryFormat string, statusQueryFormat string, statusFormat string) {
	queryCmd, queryEndCmd, queryStatusCmd := LookupAccelerometerCommands(
		mcu,
		cmdQueue,
		oid,
		queryFormat,
		statusQueryFormat,
		statusFormat,
	)
	self.runtime.SetCommands(queryCmd, queryEndCmd, queryStatusCmd)
}

func (self *accelerometerModule) Is_measuring() bool {
	return self.runtime.IsMeasuring()
}

func (self *accelerometerModule) AddClient(client reportpkg.APIDumpClient, template map[string]interface{}) {
	self.runtime.AddClient(client, template)
}

func (self *accelerometerModule) Start_internal_client() IAclient {
	return self.runtime.StartInternalClient(requireAccelQueryToolhead(self.printer))
}

func (self *accelerometerModule) Get_name() string {
	return self.runtime.Name()
}
