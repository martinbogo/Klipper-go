//go:build linux

package serialhdl

import (
	"os"

	"golang.org/x/sys/unix"
)

func configureUARTPortRTS(file *os.File, enabled bool) error {
	fd := int(file.Fd())
	status, err := unix.IoctlGetInt(fd, unix.TIOCMGET)
	if err != nil {
		return err
	}
	if enabled {
		status |= unix.TIOCM_RTS
	} else {
		status &^= unix.TIOCM_RTS
	}
	return unix.IoctlSetPointerInt(fd, unix.TIOCMSET, status)
}
