//go:build linux

package util

import "golang.org/x/sys/unix"

func clearHUPCLImpl(fd uintptr) {
	termios, err := unix.IoctlGetTermios(int(fd), unix.TCGETS2)
	if err != nil {
		return
	}
	termios.Cflag &^= uint32(unix.HUPCL)
	if err := unix.IoctlSetTermios(int(fd), unix.TCSETS2, termios); err != nil {
		_ = unix.IoctlSetTermios(int(fd), unix.TCSETS, termios)
	}
}
