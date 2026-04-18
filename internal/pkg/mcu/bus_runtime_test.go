package mcu

import "testing"

func TestBusQuerySenderAdapterSendWithPrefaceUsesWrappedRawSender(t *testing.T) {
	preface := &BusCommandSenderAdapter{Raw: "preface-raw"}
	adapter := &BusQuerySenderAdapter{
		SendWithPrefaceFunc: func(prefaceRaw interface{}, prefaceData interface{}, data interface{}, minclock, reqclock int64) interface{} {
			if prefaceRaw != "preface-raw" {
				t.Fatalf("unexpected preface raw %#v", prefaceRaw)
			}
			if prefaceData != "preface-data" || data != "payload" || minclock != 11 || reqclock != 22 {
				t.Fatalf("unexpected forwarded arguments %#v %#v %d %d", prefaceData, data, minclock, reqclock)
			}
			return "ok"
		},
	}

	if got := adapter.SendWithPreface(preface, "preface-data", "payload", 11, 22); got != "ok" {
		t.Fatalf("unexpected send-with-preface result %#v", got)
	}
}

func TestBuildSPIShutdownConfigCommand(t *testing.T) {
	cmd := BuildSPIShutdownConfigCommand(3, 5, []int{0x12, 0xab, 0x00})
	if cmd != "config_spi_shutdown oid=3 spi_oid=5 shutdown_msg=12ab00" {
		t.Fatalf("unexpected SPI shutdown config command %q", cmd)
	}
}

func TestBuildSPISendConfigCommand(t *testing.T) {
	cmd := BuildSPISendConfigCommand(7, []int{1, 2, 255})
	if cmd != "spi_send oid=7 data=0102ff" {
		t.Fatalf("unexpected SPI send config command %q", cmd)
	}
}

func TestBuildI2CWriteConfigCommand(t *testing.T) {
	cmd := BuildI2CWriteConfigCommand(9, []int{0, 16, 255})
	if cmd != "i2c_write oid=9 data=0010ff" {
		t.Fatalf("unexpected I2C write config command %q", cmd)
	}
}

func TestBuildI2CModifyBitsConfigCommand(t *testing.T) {
	cmd := BuildI2CModifyBitsConfigCommand(4, "A", "\x01", "\x02")
	if cmd != "i2c_modify_bits oid=4 reg=41 clear_set_bits=0102" {
		t.Fatalf("unexpected I2C modify-bits config command %q", cmd)
	}
}
