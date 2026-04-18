package msgproto

import (
	"reflect"
	"testing"
)

func TestCrc16CcittMatchesPythonReference(t *testing.T) {
	buf := []int{5, 16, 1, 2, 3, 4}
	got := Crc16_ccitt(buf)
	want := []int{109, 248}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Crc16_ccitt(%v) = %v, want %v", buf, got, want)
	}
}
