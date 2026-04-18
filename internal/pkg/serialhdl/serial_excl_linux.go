//go:build linux

package serialhdl

import "golang.org/x/sys/unix"

// setTIOCEXCL sets exclusive mode on the serial port file descriptor,
// matching Python's serial.Serial(exclusive=True). This prevents any other
// process from opening the port while gklib holds it.
func setTIOCEXCL(fd uintptr) error {
	return unix.IoctlSetInt(int(fd), unix.TIOCEXCL, 0)
}
