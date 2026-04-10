package heater

import printerpkg "goklipper/internal/pkg/printer"

type CustomThermistor struct {
	Name   string
	Params map[string]float64
}

func NewCustomThermistor(config printerpkg.ModuleConfig) *CustomThermistor {
	self := &CustomThermistor{Name: configSuffixName(config.Name())}
	t1 := config.Float("temperature1", 0.0)
	r1 := config.Float("resistance1", 0.0)
	if beta := config.OptionalFloat("beta"); beta != nil {
		self.Params = map[string]float64{"t1": t1, "r1": r1, "beta": *beta}
		return self
	}
	t2 := config.Float("temperature2", 0.0)
	r2 := config.Float("resistance2", 0.0)
	t3 := config.Float("temperature3", 0.0)
	r3 := config.Float("resistance3", 0.0)
	arr := [][]float64{{t1, r1}, {t2, r2}, {t3, r3}}
	for i := 0; i < len(arr)-1; i++ {
		for j := 0; j < len(arr)-1-i; j++ {
			if arr[j][0] < arr[j+1][0] {
				arr[j], arr[j+1] = arr[j+1], arr[j]
			}
		}
	}
	self.Params = map[string]float64{
		"t1": arr[0][0], "r1": arr[0][1],
		"t2": arr[1][0], "r2": arr[1][1],
		"t3": arr[2][0], "r3": arr[2][1],
	}
	return self
}

func PrinterThermistor(config printerpkg.ModuleConfig, params map[string]float64) printerpkg.TemperatureSensor {
	pullup := config.Float("pullup_resistor", 4700.0)
	inlineResistor := config.Float("inline_resistor", 0.0)
	thermistor := NewThermistor(pullup, inlineResistor)
	if beta, ok := params["beta"]; ok {
		thermistor.Setup_coefficients_beta(params["t1"], params["r1"], beta)
	} else {
		thermistor.Setup_coefficients(
			params["t1"], params["r1"],
			params["t2"], params["r2"],
			params["t3"], params["r3"],
			config.Name(),
		)
	}
	return NewPrinterADCtoTemperature(config, thermistor)
}

func (self *CustomThermistor) Create(config printerpkg.ModuleConfig) printerpkg.TemperatureSensor {
	return PrinterThermistor(config, self.Params)
}

func LoadConfigThermistor(config printerpkg.ModuleConfig) interface{} {
	thermistor := NewCustomThermistor(config)
	config.Printer().TemperatureSensors().AddSensorFactory(thermistor.Name, thermistor.Create)
	return nil
}