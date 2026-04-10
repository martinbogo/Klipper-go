package heater

type ScaledADCModuleInit struct {
	Chip   *ScaledADCChip
	MCURef interface{}
}

func InitializeScaledADCModule(name string, smoothTime float64,
	setupNamedADC func(pinName string, callback func(float64, float64)) ADCPin,
	setupMCUADC func(pinParams map[string]interface{}) ADCPin,
	registerADC func(name string, adc interface{})) (*ScaledADCModuleInit, error) {
	chip, err := NewScaledADCChip(name, smoothTime, setupNamedADC, setupMCUADC, registerADC)
	if err != nil {
		return nil, err
	}
	return &ScaledADCModuleInit{Chip: chip, MCURef: chip.MCURef()}, nil
}
