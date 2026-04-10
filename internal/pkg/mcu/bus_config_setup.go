package mcu

import "fmt"

type SPIConfigSetupPlan struct {
	InitialConfigCmds []string
	ConfigFormat      string
	SendLookup        string
	TransferRequest   string
	TransferResponse  string
}

func BuildSPIConfigSetupPlan(oid int, pin interface{}, mode int, speed int, swPins []interface{}, csActiveHigh bool) SPIConfigSetupPlan {
	initial := []string{}
	if pin == nil {
		initial = append(initial, fmt.Sprintf("config_spi_without_cs oid=%d", oid))
	} else {
		initial = append(initial, fmt.Sprintf("config_spi oid=%d pin=%s cs_active_high=%v", oid, pin, csActiveHigh))
	}
	configFormat := "spi_set_bus oid=%d spi_bus=%%s mode=%d rate=%d"
	if swPins != nil {
		configFormat = "spi_set_software_bus oid=%d miso_pin=%s mosi_pin=%s sclk_pin=%s mode=%d rate=%d"
		return SPIConfigSetupPlan{
			InitialConfigCmds: initial,
			ConfigFormat:      fmt.Sprintf(configFormat, oid, swPins[0], swPins[1], swPins[2], mode, speed),
			SendLookup:        "spi_send oid=%c data=%*s",
			TransferRequest:   "spi_transfer oid=%c data=%*s",
			TransferResponse:  "spi_transfer_response oid=%c response=%*s",
		}
	}
	return SPIConfigSetupPlan{
		InitialConfigCmds: initial,
		ConfigFormat:      fmt.Sprintf(configFormat, oid, mode, speed),
		SendLookup:        "spi_send oid=%c data=%*s",
		TransferRequest:   "spi_transfer oid=%c data=%*s",
		TransferResponse:  "spi_transfer_response oid=%c response=%*s",
	}
}

type I2CConfigSetupPlan struct {
	ConfigFormat     string
	WriteLookup      string
	ReadRequest      string
	ReadResponse     string
	ModifyBitsLookup string
}

func BuildI2CConfigSetupPlan(oid int, speed int, address int) I2CConfigSetupPlan {
	return I2CConfigSetupPlan{
		ConfigFormat:     fmt.Sprintf("config_i2c oid=%d i2c_bus=%%s rate=%d address=%d", oid, speed, address),
		WriteLookup:      "i2c_write oid=%c data=%*s",
		ReadRequest:      "i2c_read oid=%c reg=%*s read_len=%u",
		ReadResponse:     "i2c_read_response oid=%c response=%*s",
		ModifyBitsLookup: "i2c_modify_bits oid=%c reg=%*s clear_set_bits=%*s",
	}
}
