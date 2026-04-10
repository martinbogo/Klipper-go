package mcu

import "testing"

func TestBuildSPIConfigSetupPlanHardwareBus(t *testing.T) {
	plan := BuildSPIConfigSetupPlan(4, "PA4", 3, 5000000, nil, true)
	if len(plan.InitialConfigCmds) != 1 || plan.InitialConfigCmds[0] != "config_spi oid=4 pin=PA4 cs_active_high=true" {
		t.Fatalf("unexpected initial SPI config %#v", plan.InitialConfigCmds)
	}
	if plan.ConfigFormat != "spi_set_bus oid=4 spi_bus=%s mode=3 rate=5000000" {
		t.Fatalf("unexpected SPI config format %q", plan.ConfigFormat)
	}
	if plan.SendLookup != "spi_send oid=%c data=%*s" || plan.TransferRequest != "spi_transfer oid=%c data=%*s" || plan.TransferResponse != "spi_transfer_response oid=%c response=%*s" {
		t.Fatalf("unexpected SPI lookup formats %#v", plan)
	}
}

func TestBuildSPIConfigSetupPlanSoftwareBus(t *testing.T) {
	plan := BuildSPIConfigSetupPlan(7, nil, 1, 1000000, []interface{}{"MISO", "MOSI", "SCLK"}, false)
	if len(plan.InitialConfigCmds) != 1 || plan.InitialConfigCmds[0] != "config_spi_without_cs oid=7" {
		t.Fatalf("unexpected software SPI initial config %#v", plan.InitialConfigCmds)
	}
	if plan.ConfigFormat != "spi_set_software_bus oid=7 miso_pin=MISO mosi_pin=MOSI sclk_pin=SCLK mode=1 rate=1000000" {
		t.Fatalf("unexpected software SPI config format %q", plan.ConfigFormat)
	}
}

func TestBuildI2CConfigSetupPlan(t *testing.T) {
	plan := BuildI2CConfigSetupPlan(5, 400000, 42)
	if plan.ConfigFormat != "config_i2c oid=5 i2c_bus=%s rate=400000 address=42" {
		t.Fatalf("unexpected I2C config format %q", plan.ConfigFormat)
	}
	if plan.WriteLookup != "i2c_write oid=%c data=%*s" || plan.ReadRequest != "i2c_read oid=%c reg=%*s read_len=%u" || plan.ReadResponse != "i2c_read_response oid=%c response=%*s" || plan.ModifyBitsLookup != "i2c_modify_bits oid=%c reg=%*s clear_set_bits=%*s" {
		t.Fatalf("unexpected I2C lookup formats %#v", plan)
	}
}
