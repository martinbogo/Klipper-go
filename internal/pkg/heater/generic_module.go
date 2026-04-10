package heater

import (
	"fmt"

	printerpkg "goklipper/internal/pkg/printer"
)

type genericHeaterManager interface {
	SetupHeater(config printerpkg.ModuleConfig, gcodeID string) interface{}
}

func LoadConfigGenericHeater(config printerpkg.ModuleConfig) interface{} {
	config.LoadObject("heaters")
	printer := config.Printer()
	heatersObj := printer.LookupObject("heaters", nil)
	heaters, ok := heatersObj.(genericHeaterManager)
	if !ok {
		panic(fmt.Sprintf("heaters object does not implement genericHeaterManager: %T", heatersObj))
	}
	return heaters.SetupHeater(config, "")
}