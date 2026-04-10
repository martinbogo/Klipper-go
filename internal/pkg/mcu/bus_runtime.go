package mcu

import (
	"fmt"
	"strings"
)

func hexEncodeInts(data []int) string {
	if len(data) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, value := range data {
		builder.WriteString(fmt.Sprintf("%02x", value))
	}
	return builder.String()
}

func hexEncodeStringBytes(data string) string {
	if data == "" {
		return ""
	}
	var builder strings.Builder
	for _, value := range []byte(data) {
		builder.WriteString(fmt.Sprintf("%02x", value))
	}
	return builder.String()
}

func BuildSPIShutdownConfigCommand(configOID int, spiOID int, shutdownSeq []int) string {
	return fmt.Sprintf("config_spi_shutdown oid=%d spi_oid=%d shutdown_msg=%s", configOID, spiOID, hexEncodeInts(shutdownSeq))
}

func BuildSPISendConfigCommand(oid int, data []int) string {
	return fmt.Sprintf("spi_send oid=%d data=%s", oid, hexEncodeInts(data))
}

func BuildI2CWriteConfigCommand(oid int, data []int) string {
	return fmt.Sprintf("i2c_write oid=%d data=%s", oid, hexEncodeInts(data))
}

func BuildI2CModifyBitsConfigCommand(oid int, reg string, clearBits string, setBits string) string {
	return fmt.Sprintf("i2c_modify_bits oid=%d reg=%s clear_set_bits=%s", oid, hexEncodeStringBytes(reg), hexEncodeStringBytes(clearBits+setBits))
}
