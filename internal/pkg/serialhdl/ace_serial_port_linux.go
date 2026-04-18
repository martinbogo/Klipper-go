//go:build linux

package serialhdl

import (
	"fmt"
	"os"

	"goklipper/common/logger"

	"golang.org/x/sys/unix"
)

func PrepareACESerialPort(file *os.File, name string, baud int) {
	fd := int(file.Fd())
	if err := unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIOFLUSH); err != nil {
		logger.Errorf("TCIOFLUSH failed on %s: %v", name, err)
	}

	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		logger.Errorf("TCGETS2 failed on %s: %v", name, err)
		return
	}

	termios.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	termios.Oflag &^= unix.OPOST
	termios.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Cflag &^= unix.CSIZE | unix.PARENB | unix.CRTSCTS
	termios.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL
	if err := setTermiosBaud(termios, baud); err != nil {
		logger.Errorf("unable to set baud on %s to %d: %v", name, baud, err)
		return
	}

	if err := unix.IoctlSetTermios(fd, unix.TCSETS2, termios); err != nil {
		logger.Errorf("TCSETS2 failed on %s: %v", name, err)
	}
}

func reconfigureSerialPortBaud(file *os.File, name string, baud int) error {
	if file == nil {
		return fmt.Errorf("nil serial file")
	}
	fd := int(file.Fd())
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		return fmt.Errorf("TCGETS2 failed on %s: %w", name, err)
	}
	if err := setTermiosBaud(termios, baud); err != nil {
		return fmt.Errorf("unable to set baud on %s to %d: %w", name, baud, err)
	}
	if err := unix.IoctlSetTermios(fd, unix.TCSETS2, termios); err != nil {
		return fmt.Errorf("TCSETS2 failed on %s: %w", name, err)
	}
	return nil
}

func setTermiosBaud(termios *unix.Termios, baud int) error {
	if baud <= 0 {
		return fmt.Errorf("invalid baud %d", baud)
	}
	speed, ok := lookupLinuxBaudConstant(baud)
	termios.Cflag &^= uint32(unix.CBAUD)
	if ok {
		termios.Cflag |= speed
		termios.Ispeed = speed
		termios.Ospeed = speed
		return nil
	}
	termios.Cflag |= uint32(unix.BOTHER)
	termios.Ispeed = uint32(baud)
	termios.Ospeed = uint32(baud)
	return nil
}

func lookupLinuxBaudConstant(baud int) (uint32, bool) {
	switch baud {
	case 2400:
		return uint32(unix.B2400), true
	case 9600:
		return uint32(unix.B9600), true
	case 19200:
		return uint32(unix.B19200), true
	case 38400:
		return uint32(unix.B38400), true
	case 57600:
		return uint32(unix.B57600), true
	case 115200:
		return uint32(unix.B115200), true
	case 230400:
		return uint32(unix.B230400), true
	case 460800:
		return uint32(unix.B460800), true
	case 500000:
		return uint32(unix.B500000), true
	case 576000:
		return uint32(unix.B576000), true
	case 921600:
		return uint32(unix.B921600), true
	default:
		return 0, false
	}
}
