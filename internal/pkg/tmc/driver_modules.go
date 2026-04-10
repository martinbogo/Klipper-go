package tmc

type DriverConfig interface {
	CurrentHelperConfig
	TMC2660CurrentHelperConfig
	ConfigFieldSource
	Get(option string, default1 interface{}, noteValid bool) interface{}
}

type DriverCommandHelper interface {
	SetupRegisterDump([]string, func(string, int64) (string, int64))
	GetPhaseOffset() (*int, int)
	GetStatus(float64) map[string]interface{}
}

type DriverAdapter interface {
	NewUART(config DriverConfig, nameToReg map[string]int64, fields *FieldHelper, maxAddr int64, tmcFrequency float64) RegisterAccess
	NewSPI(config DriverConfig, nameToReg map[string]int64, fields *FieldHelper) RegisterAccess
	NewTMC2660SPI(config DriverConfig, nameToReg map[string]int64, fields *FieldHelper) RegisterAccess
	AttachVirtualPin(config DriverConfig, mcuTMC RegisterAccess)
	NewCommandHelper(config DriverConfig, mcuTMC RegisterAccess, currentHelper CurrentControl) DriverCommandHelper
	ApplyStealthchop(config DriverConfig, mcuTMC RegisterAccess, tmcFrequency float64)
	NewTMC2660CurrentHelper(config DriverConfig, mcuTMC RegisterAccess) CurrentControl
}

type DriverModule struct {
	Get_phase_offset func() (*int, int)
	Get_status       func(float64) map[string]interface{}
}

func newDriverModule(helper DriverCommandHelper) *DriverModule {
	return &DriverModule{
		Get_phase_offset: helper.GetPhaseOffset,
		Get_status:       helper.GetStatus,
	}
}

func tmc2208ReadTranslate(fields *FieldHelper) func(string, int64) (string, int64) {
	return func(regName string, val int64) (string, int64) {
		if regName == "IOIN" {
			regName = "IOIN@TMC220x"
			if fields.Get_field("sel_a", val, nil) == 0 {
				regName = "IOIN@TMC222x"
			}
		}
		return regName, val
	}
}

func NewTMC2130(config DriverConfig, adapter DriverAdapter) *DriverModule {
	fields := NewFieldHelper(TMC2130Fields, TMC2130SignedFields, TMC2130FieldFormatters, nil)
	mcuTMC := adapter.NewSPI(config, TMC2130Registers, fields)
	adapter.AttachVirtualPin(config, mcuTMC)
	currentHelper := NewTMCCurrentHelper(config, mcuTMC)
	cmdHelper := adapter.NewCommandHelper(config, mcuTMC, currentHelper)
	cmdHelper.SetupRegisterDump(TMC2130ReadRegisters, nil)
	ConfigureTMC2130(config, fields)
	adapter.ApplyStealthchop(config, mcuTMC, TMC2130TMCFrequency)
	return newDriverModule(cmdHelper)
}

func NewTMC2208(config DriverConfig, adapter DriverAdapter) *DriverModule {
	fields := NewFieldHelper(TMC2208Fields, TMC2208SignedFields, TMC2208FieldFormatters, nil)
	mcuTMC := adapter.NewUART(config, TMC2208Registers, fields, 0, TMC2208TMCFrequency)
	currentHelper := NewTMCCurrentHelper(config, mcuTMC)
	cmdHelper := adapter.NewCommandHelper(config, mcuTMC, currentHelper)
	cmdHelper.SetupRegisterDump(TMC2208ReadRegisters, tmc2208ReadTranslate(fields))
	ConfigureTMC2208(config, fields)
	adapter.ApplyStealthchop(config, mcuTMC, TMC2208TMCFrequency)
	return newDriverModule(cmdHelper)
}

func NewTMC2209(config DriverConfig, adapter DriverAdapter) *DriverModule {
	fields := NewFieldHelper(TMC2209Fields, TMC2208SignedFields, TMC2209FieldFormatters, nil)
	mcuTMC := adapter.NewUART(config, TMC2209Registers, fields, 3, TMC2209TMCFrequency)
	adapter.AttachVirtualPin(config, mcuTMC)
	currentHelper := NewTMCCurrentHelper(config, mcuTMC)
	cmdHelper := adapter.NewCommandHelper(config, mcuTMC, currentHelper)
	cmdHelper.SetupRegisterDump(TMC2209ReadRegisters, nil)
	ConfigureTMC2209(config, fields)
	adapter.ApplyStealthchop(config, mcuTMC, TMC2209TMCFrequency)
	return newDriverModule(cmdHelper)
}

func NewTMC2240(config DriverConfig, adapter DriverAdapter) *DriverModule {
	fields := NewFieldHelper(TMC2240Fields, TMC2240SignedFields, TMC2240FieldFormatters, nil)
	var mcuTMC RegisterAccess
	if config.Get("uart_pin", nil, true) != nil {
		mcuTMC = adapter.NewUART(config, TMC2240Registers, fields, 3, TMC2240TMCFrequency)
	} else {
		mcuTMC = adapter.NewSPI(config, TMC2240Registers, fields)
	}
	adapter.AttachVirtualPin(config, mcuTMC)
	currentHelper := NewTMC2240CurrentHelper(config, mcuTMC)
	cmdHelper := adapter.NewCommandHelper(config, mcuTMC, currentHelper)
	cmdHelper.SetupRegisterDump(TMC2240ReadRegisters, nil)
	ApplyWaveTableDefaults(config, mcuTMC)
	ConfigureTMC2240(config, fields)
	adapter.ApplyStealthchop(config, mcuTMC, TMC2240TMCFrequency)
	return newDriverModule(cmdHelper)
}

func NewTMC2660(config DriverConfig, adapter DriverAdapter) *DriverModule {
	fields := NewFieldHelper(TMC2660Fields, TMC2660SignedFields, TMC2660FieldFormatters, nil)
	mcuTMC := adapter.NewTMC2660SPI(config, TMC2660Registers, fields)
	currentHelper := adapter.NewTMC2660CurrentHelper(config, mcuTMC)
	cmdHelper := adapter.NewCommandHelper(config, mcuTMC, currentHelper)
	cmdHelper.SetupRegisterDump(TMC2660ReadRegisters, nil)
	ConfigureTMC2660(config, fields)
	return newDriverModule(cmdHelper)
}

func NewTMC5160(config DriverConfig, adapter DriverAdapter) *DriverModule {
	fields := NewFieldHelper(TMC5160Fields, TMC5160SignedFields, TMC5160FieldFormatters, nil)
	mcuTMC := adapter.NewSPI(config, TMC5160Registers, fields)
	adapter.AttachVirtualPin(config, mcuTMC)
	currentHelper := NewTMC5160CurrentHelper(config, mcuTMC)
	cmdHelper := adapter.NewCommandHelper(config, mcuTMC, currentHelper)
	cmdHelper.SetupRegisterDump(TMC5160ReadRegisters, nil)
	ConfigureTMC5160(config, fields)
	adapter.ApplyStealthchop(config, mcuTMC, TMC5160TMCFrequency)
	return newDriverModule(cmdHelper)
}

func LoadConfigTMC2130(config DriverConfig, adapter DriverAdapter) interface{} {
	return NewTMC2130(config, adapter)
}
func LoadConfigTMC2208(config DriverConfig, adapter DriverAdapter) interface{} {
	return NewTMC2208(config, adapter)
}
func LoadConfigTMC2209(config DriverConfig, adapter DriverAdapter) interface{} {
	return NewTMC2209(config, adapter)
}
func LoadConfigTMC2240(config DriverConfig, adapter DriverAdapter) interface{} {
	return NewTMC2240(config, adapter)
}
func LoadConfigTMC2660(config DriverConfig, adapter DriverAdapter) interface{} {
	return NewTMC2660(config, adapter)
}
func LoadConfigTMC5160(config DriverConfig, adapter DriverAdapter) interface{} {
	return NewTMC5160(config, adapter)
}
