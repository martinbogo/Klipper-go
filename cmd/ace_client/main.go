//go:build linux

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func calcCrc(buf []byte) uint16 {
	var _crc uint16 = 0xffff
	for i := 0; i < len(buf); i++ {
		data := uint16(buf[i])
		data ^= _crc & 0xff
		data ^= (data & 0x0f) << 4
		_crc = ((data << 8) | (_crc >> 8)) ^ (data >> 4) ^ (data << 3)
	}
	return _crc
}

func buildACEPacket(payload string) []byte {
	bts := []byte(payload)
	l := len(bts)

	packet := make([]byte, 4+l+3)
	packet[0] = 0xFF
	packet[1] = 0xAA
	packet[2] = byte(l & 0xFF)
	packet[3] = byte((l >> 8) & 0xFF)
	copy(packet[4:], bts)

	crc := calcCrc(bts)
	packet[4+l] = byte(crc & 0xFF)
	packet[4+l+1] = byte((crc >> 8) & 0xFF)
	packet[4+l+2] = 0xFE
	return packet
}

func main() {
	var port = "/dev/ttyACM0"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	fd, err := unix.Open(port, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_LARGEFILE|unix.O_CLOEXEC, 0666)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", port, err)
	}
	defer unix.Close(fd)

	log.Printf("Opened %s with fd %d", port, fd)

	fmt.Println("Configuring termios to match gklib exactly...")
	t, _ := unix.IoctlGetTermios(fd, unix.TCGETS)

	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8

	t.Ispeed = unix.B115200
	t.Ospeed = unix.B115200

	t.Cflag |= unix.CLOCAL | unix.CREAD

	t.Cc[unix.VTIME] = 0
	t.Cc[unix.VMIN] = 1

	if err := unix.IoctlSetTermios(fd, unix.TCSETS, t); err != nil {
		log.Fatalf("TCSETS failed: %v", err)
	}

	// Flush input/output buffers immediately after config to appease ACE hardware
	if err := unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIOFLUSH); err != nil {
		log.Printf("TCIOFLUSH failed: %v", err)
	} else {
		log.Printf("TCIOFLUSH successful")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := unix.Read(fd, buf)
			if err != nil {
				if err == unix.EAGAIN {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				log.Printf("Read error: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}
			if n > 0 {
				log.Printf("RECV (%d): %x", n, buf[:n])
			}
		}
	}()

	id := 2
	for {
		reqStr := fmt.Sprintf(`{"id":%d,"method":"get_info"}`, id)
		pkt := buildACEPacket(reqStr)

		log.Printf("Sending (id=%d, len=%d): %x", id, len(pkt), pkt)
		n, err := unix.Write(fd, pkt)
		if err != nil {
			log.Printf("Write error: %v", err)
		} else if n != len(pkt) {
			log.Printf("Short write: %d < %d", n, len(pkt))
		}

		id++
		time.Sleep(1 * time.Second)

		select {
		case <-sigChan:
			log.Println("Exiting")
			return
		default:
		}
	}
}
