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

// TestAnalyzeRawUARTResponses decodes raw response data captured from
// the printer to understand why decode validation fails.
func TestAnalyzeRawUARTResponses(t *testing.T) {
	// These are actual raw responses from the printer logs for IFCNT (reg=0x02)
	samples := []struct {
		name string
		addr int64
		raw  []int64
	}{
		{"stepper_z attempt A", 2, []int64{10, 250, 79, 32, 128, 0, 2, 0, 0, 254}},
		{"stepper_z attempt B", 2, []int64{0, 0, 192, 254, 19, 8, 32, 128, 0, 146}},
		{"stepper_z attempt C", 2, []int64{10, 250, 79, 32, 128, 0, 2, 72, 162, 249}},
		{"stepper_z attempt D", 2, []int64{240, 255, 255, 255, 4, 2, 8, 32, 128, 62}},
		{"stepper_z attempt E", 2, []int64{10, 248, 1, 0, 128, 0, 2, 232, 35, 213}},
		{"stepper_z attempt F", 2, []int64{10, 250, 79, 32, 128, 0, 2, 104, 240, 255}},
		{"stepper_x attempt A", 3, []int64{10, 250, 79, 32, 128, 0, 2, 232, 255, 191}},
		{"stepper_x attempt B", 3, []int64{10, 250, 79, 32, 0, 14, 0, 248, 163, 254}},
		{"stepper_x attempt C", 3, []int64{250, 31, 0, 0, 128, 0, 2, 40, 162, 254}},
		{"stepper_x attempt D", 3, []int64{4, 2, 8, 32, 128, 34, 234, 255, 255, 255}},
		{"stepper_x attempt E", 3, []int64{10, 250, 79, 32, 0, 0, 58, 0, 160, 254}},
		{"stepper_y attempt A", 1, []int64{10, 250, 79, 32, 128, 0, 242, 255, 255, 255}},
	}

	var reg int64 = 0x02 // IFCNT

	// Generate expected encoding for some known IFCNT values
	t.Log("=== Expected encodings for known IFCNT values ===")
	for _, ifcnt := range []int64{0, 1, 2, 16, 17, 18, 19, 20, 31, 32} {
		expected := EncodeUARTWrite(0x05, 0xff, reg, ifcnt)
		t.Logf("  IFCNT=%2d → %v", ifcnt, expected)
	}

	t.Log("\n=== Raw response analysis ===")
	for _, s := range samples {
		val, ok := DecodeUARTRead(reg, s.raw)
		if ok {
			t.Logf("  %s: DECODED OK, value=%d (0x%x)", s.name, val, val)
		} else {
			// Extract the value anyway to see what we would get
			mval := int64(0)
			for i, d := range s.raw {
				mval |= d << (i * 8)
			}
			extractedVal := (((mval >> 31) & 0xff) << 24) |
				(((mval >> 41) & 0xff) << 16) |
				(((mval >> 51) & 0xff) << 8) |
				((mval >> 61) & 0xff)

			expected := EncodeUARTWrite(0x05, 0xff, reg, extractedVal)
			t.Logf("  %s: DECODE FAILED", s.name)
			t.Logf("    raw:       %v", s.raw)
			t.Logf("    expected:  %v (if value=%d/0x%x)", expected, extractedVal, extractedVal)

			// Show byte-by-byte diff
			diffs := ""
			for i := 0; i < 10; i++ {
				if s.raw[i] != expected[i] {
					diffs += "X"
				} else {
					diffs += "."
				}
			}
			t.Logf("    diff:      %s", diffs)
		}
	}
}
