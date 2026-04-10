//go:build linux

package serialhdl

import (
	"os"

	"goklipper/common/logger"

	"golang.org/x/sys/unix"
)

func PrepareACESerialPort(file *os.File, name string, baud int) {
	fd := int(file.Fd())
	if err := unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIOFLUSH); err != nil {
		logger.Errorf("TCIOFLUSH failed on %s: %v", name, err)
	}

	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		logger.Errorf("TCGETS failed on %s: %v", name, err)
		return
	}

	termios.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	termios.Oflag &^= unix.OPOST
	termios.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Cflag &^= unix.CSIZE | unix.PARENB | unix.CRTSCTS
	termios.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL
	switch baud {
	case 115200:
		termios.Ispeed = unix.B115200
		termios.Ospeed = unix.B115200
	case 230400:
		termios.Ispeed = unix.B230400
		termios.Ospeed = unix.B230400
	}

	if err := unix.IoctlSetTermios(fd, unix.TCSETS, termios); err != nil {
		logger.Errorf("TCSETS failed on %s: %v", name, err)
	}
}
