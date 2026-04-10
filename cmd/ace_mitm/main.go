package main

import (
        "encoding/hex"
        "io"
        "log"
        "net"
        "os"
        "path/filepath"
        "time"

        "github.com/tarm/serial"
)

func findTTY() string {
        for {
                matches, _ := filepath.Glob("/dev/ttyACM*")
                if len(matches) > 0 {
                        return matches[len(matches)-1] // return the highest one, usually the most recent
                }
                time.Sleep(1 * time.Second)
        }
}

func main() {
        log.Println("Starting ACE MITM proxy...")

        var s *serial.Port
        var err error

        tty := findTTY()
        c := &serial.Config{Name: tty, Baud: 230400, ReadTimeout: time.Millisecond * 10}

        for {
                s, err = serial.OpenPort(c)
                if err == nil {
                        break
                }
                log.Printf("Waiting for %s: %v", tty, err)
                time.Sleep(2 * time.Second)
                tty = findTTY()
                c.Name = tty
        }
        defer s.Close()
        log.Printf("Opened %s at 230400\n", tty)

        l, err := net.Listen("tcp", "0.0.0.0:9999")
        if err != nil {
                log.Fatalf("Failed to listen on TCP: %v", err)
        }
        defer l.Close()
        log.Println("Listening on tcp :9999")

        for {
                conn, err := l.Accept()
                if err != nil {
                        log.Println("Accept error:", err)
                        continue
                }
                log.Println("Client connected:", conn.RemoteAddr())

                go proxy(conn, s, "TCP->SERIAL", tty)
                go proxy(s, conn, "SERIAL->TCP", tty)
        }
}

func proxy(src io.Reader, dst io.Writer, name string, tty string) {
        buf := make([]byte, 4096)
        for {
                n, err := src.Read(buf)
                if n > 0 {
                        raw := buf[:n]
                        log.Printf("[%s] %d bytes\n%s", name, n, hex.Dump(raw))
                        dst.Write(raw)
                }
                if err != nil {
                        if err == io.EOF {
                            time.Sleep(100 * time.Millisecond) // Don't spin too fast
                            
                            // Check if the device still exists.
                            if _, statErr := os.Stat(tty); os.IsNotExist(statErr) {
                                log.Printf("[%s] %s disappeared! Warning! Exiting goroutine to fix spin.", name, tty)
                                break // Stop spinning if the device is gone
                            }
                            continue
                        }
                        log.Printf("[%s] Read error: %v", name, err)
                        break
                }
        }
}
