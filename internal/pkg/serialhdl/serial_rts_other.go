//go:build !linux

package serialhdl

import "os"

func configureUARTPortRTS(file *os.File, enabled bool) error {
	return nil
}
