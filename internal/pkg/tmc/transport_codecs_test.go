package tmc

import (
	"reflect"
	"testing"
)

func TestUARTWriteDecodeRoundTrip(t *testing.T) {
	encoded := EncodeUARTWrite(0x05, 0xff, 0x12, 0x12345678)
	decoded, ok := DecodeUARTRead(0x12, encoded)
	if !ok {
		t.Fatalf("expected UART decode to succeed")
	}
	if decoded != 0x12345678 {
		t.Fatalf("expected decoded value to round-trip, got %#x", decoded)
	}
}

func TestUARTDecodeRejectsCorruption(t *testing.T) {
	encoded := EncodeUARTWrite(0x05, 0xff, 0x12, 0x12345678)
	encoded[len(encoded)-1] ^= 0x01
	if _, ok := DecodeUARTRead(0x12, encoded); ok {
		t.Fatalf("expected corrupted UART packet to fail decode")
	}
}

func TestUARTReadEncodingUsesCRCAndSerialBits(t *testing.T) {
	got := EncodeUARTRead(0xf5, 0x01, 0x02)
	want := AddSerialBits([]int64{0xf5, 0x01, 0x02, CalcCRC8ATM([]int64{0xf5, 0x01, 0x02})})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected UART read encoding to match CRC+serial packing, got %#v want %#v", got, want)
	}
}

func TestBuildSPIChainCommandPositionsPayload(t *testing.T) {
	got := BuildSPIChainCommand([]int64{1, 2, 3, 4, 5}, 3, 2)
	want := []int{0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 0, 0, 0, 0, 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected SPI chain command placement to match, got %#v want %#v", got, want)
	}
}

func TestDecodeSPIChainResponseExtractsSelectedSlot(t *testing.T) {
	response := string([]byte{
		0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x11, 0x22, 0x33, 0x44,
		0x00, 0x00, 0x00, 0x00, 0x00,
	})
	if got := DecodeSPIChainResponse(response, 3, 2); got != 0x11223344 {
		t.Fatalf("expected selected SPI slot to decode, got %#x", got)
	}
}