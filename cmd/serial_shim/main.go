package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/tarm/serial"
)

const (
	defaultListenAddr     = "127.0.0.1:9105"
	defaultSerialPort     = "/dev/ttyS5"
	defaultBaud           = 576000
	readBufferSize        = 4096
	attachDrainQuietPolls = 1
	attachDrainMaxPolls   = 8
)

func drainPendingInput(src io.Reader, quietPolls, maxPolls int) (discarded int, polls int, drainedToQuiet bool, err error) {
	if quietPolls < 1 {
		quietPolls = 1
	}
	if maxPolls < quietPolls {
		maxPolls = quietPolls
	}
	buf := make([]byte, readBufferSize)
	quietCount := 0
	for polls = 0; polls < maxPolls; polls++ {
		n, readErr := src.Read(buf)
		if n > 0 {
			discarded += n
			quietCount = 0
		} else {
			quietCount++
		}
		if readErr != nil {
			if readErr == io.EOF {
				return discarded, polls + 1, true, nil
			}
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				if quietCount >= quietPolls {
					return discarded, polls + 1, true, nil
				}
				continue
			}
			return discarded, polls + 1, false, readErr
		}
		if n == 0 && quietCount >= quietPolls {
			return discarded, polls + 1, true, nil
		}
	}
	return discarded, polls, false, nil
}

type sessionLogger struct {
	logger    *log.Logger
	maxHexLen int
}

func newSessionLogger(logPath string, maxHexLen int) (*sessionLogger, func() error, error) {
	writers := []io.Writer{os.Stdout}
	closers := []io.Closer{}
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, f)
		closers = append(closers, f)
	}
	mw := io.MultiWriter(writers...)
	closeFn := func() error {
		var firstErr error
		for _, closer := range closers {
			if err := closer.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	return &sessionLogger{
		logger:    log.New(mw, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.LUTC),
		maxHexLen: maxHexLen,
	}, closeFn, nil
}

func (l *sessionLogger) event(format string, args ...interface{}) {
	l.logger.Printf(format, args...)
}

func (l *sessionLogger) frame(sessionID uint64, direction string, payload []byte) {
	hexPayload := fmt.Sprintf("%x", payload)
	truncated := false
	if l.maxHexLen > 0 && len(hexPayload) > l.maxHexLen {
		hexPayload = hexPayload[:l.maxHexLen]
		truncated = true
	}
	if truncated {
		l.logger.Printf("session=%d dir=%s bytes=%d hex=%s...", sessionID, direction, len(payload), hexPayload)
		return
	}
	l.logger.Printf("session=%d dir=%s bytes=%d hex=%s", sessionID, direction, len(payload), hexPayload)
}

func copyLoop(done <-chan struct{}, sessionID uint64, direction string, src io.Reader, dst io.Writer, logger *sessionLogger, idlePoll bool) error {
	buf := make([]byte, readBufferSize)
	for {
		select {
		case <-done:
			return nil
		default:
		}
		n, err := src.Read(buf)
		if n > 0 {
			payload := append([]byte(nil), buf[:n]...)
			logger.frame(sessionID, direction, payload)
			if _, writeErr := dst.Write(payload); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return err
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return err
		}
		if idlePoll && n == 0 {
			continue
		}
	}
}

func main() {
	listenAddr := flag.String("listen", defaultListenAddr, "TCP listen address")
	serialPath := flag.String("serial", defaultSerialPort, "serial device path")
	baud := flag.Int("baud", defaultBaud, "serial baud rate")
	logPath := flag.String("log", "", "optional path for captured traffic log")
	maxHexLen := flag.Int("max-hex", 0, "optional maximum number of hex characters to log per frame (0 = no truncation)")
	flag.Parse()

	logger, closeLog, err := newSessionLogger(*logPath, *maxHexLen)
	if err != nil {
		log.Fatalf("open log: %v", err)
	}
	defer func() {
		if err := closeLog(); err != nil {
			log.Printf("close log: %v", err)
		}
	}()

	cfg := &serial.Config{
		Name:        *serialPath,
		Baud:        *baud,
		ReadTimeout: 50 * time.Millisecond,
	}
	serialPort, err := serial.OpenPort(cfg)
	if err != nil {
		log.Fatalf("open serial port %s: %v", *serialPath, err)
	}
	defer serialPort.Close()

	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("listen on %s: %v", *listenAddr, err)
	}
	defer listener.Close()

	logger.event("serial shim listening on %s and forwarding to %s at %d baud", *listenAddr, *serialPath, *baud)

	var nextSessionID uint64
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.event("accept error: %v", err)
			continue
		}
		tcpConn, _ := conn.(*net.TCPConn)
		if tcpConn != nil {
			_ = tcpConn.SetNoDelay(true)
		}
		sessionID := atomic.AddUint64(&nextSessionID, 1)
		logger.event("session=%d connected from %s", sessionID, conn.RemoteAddr())
		drainedBytes, drainPolls, drainedToQuiet, drainErr := drainPendingInput(serialPort, attachDrainQuietPolls, attachDrainMaxPolls)
		if drainErr != nil {
			logger.event("session=%d pre-attach serial drain error after %d polls: %v", sessionID, drainPolls, drainErr)
		} else if drainedBytes > 0 {
			if drainedToQuiet {
				logger.event("session=%d drained %d stale serial bytes before attach (%d polls)", sessionID, drainedBytes, drainPolls)
			} else {
				logger.event("session=%d drained %d stale serial bytes before attach; input remained active after %d polls", sessionID, drainedBytes, drainPolls)
			}
		}

		done := make(chan struct{})
		serialResult := make(chan error, 1)
		go func() {
			serialResult <- copyLoop(done, sessionID, "serial->tcp", serialPort, conn, logger, true)
		}()

		tcpErr := copyLoop(done, sessionID, "tcp->serial", conn, serialPort, logger, false)
		close(done)
		_ = conn.Close()
		serialErr := <-serialResult

		if tcpErr != nil && tcpErr != io.EOF {
			logger.event("session=%d tcp->serial ended with error: %v", sessionID, tcpErr)
		} else {
			logger.event("session=%d tcp->serial ended", sessionID)
		}
		if serialErr != nil && serialErr != io.EOF {
			logger.event("session=%d serial->tcp ended with error: %v", sessionID, serialErr)
		} else {
			logger.event("session=%d serial->tcp ended", sessionID)
		}
		logger.event("session=%d disconnected", sessionID)
	}
}
