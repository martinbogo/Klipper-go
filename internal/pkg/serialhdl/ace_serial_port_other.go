//go:build !linux

package serialhdl

import "os"

func PrepareACESerialPort(file *os.File, name string, baud int) {
	_ = file
	_ = name
	_ = baud
}
