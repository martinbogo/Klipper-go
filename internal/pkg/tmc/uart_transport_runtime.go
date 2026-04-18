package tmc

import (
	"fmt"
	"strings"
)

const (
	uartTransportReadRetries      = 10
	uartTransportWriteRetries     = 10
	uartTransportWriteVerifyPolls = 10
)

type UARTRegisterTransport interface {
	ReadRegister(addr, reg int64) (int64, bool)
	WriteRegister(addr, reg, val int64, printTime *float64)
}

type uartRegisterTransportFuncs struct {
	readRegister  func(addr, reg int64) (int64, bool)
	writeRegister func(addr, reg, val int64, printTime *float64)
}

func NewUARTRegisterTransportFuncs(
	readRegister func(addr, reg int64) (int64, bool),
	writeRegister func(addr, reg, val int64, printTime *float64),
) UARTRegisterTransport {
	return &uartRegisterTransportFuncs{
		readRegister:  readRegister,
		writeRegister: writeRegister,
	}
}

func (self *uartRegisterTransportFuncs) ReadRegister(addr, reg int64) (int64, bool) {
	if self.readRegister == nil {
		return 0, false
	}
	return self.readRegister(addr, reg)
}

func (self *uartRegisterTransportFuncs) WriteRegister(addr, reg, val int64, printTime *float64) {
	if self.writeRegister == nil {
		return
	}
	self.writeRegister(addr, reg, val, printTime)
}

type UARTTransportRuntime struct {
	name         string
	nameToReg    map[string]int64
	addr         int64
	transport    UARTRegisterTransport
	debugEnabled func() bool
	ifcnt        int64
	hasIFCNT     bool
}

func NewUARTTransportRuntime(name string, nameToReg map[string]int64, addr int64, transport UARTRegisterTransport, debugEnabled func() bool) *UARTTransportRuntime {
	return &UARTTransportRuntime{
		name:         name,
		nameToReg:    nameToReg,
		addr:         addr,
		transport:    transport,
		debugEnabled: debugEnabled,
	}
}

func NewLockedUARTRegisterAccess(name string, nameToReg map[string]int64, addr int64, transport UARTRegisterTransport, fields *FieldHelper, debugEnabled func() bool, mutex RegisterLocker) RegisterAccess {
	runtime := NewUARTTransportRuntime(name, nameToReg, addr, transport, debugEnabled)
	return NewLockedRegisterAccess(fields, runtime, mutex)
}

func uartTransportPanicMessage(reason interface{}) string {
	if err, ok := reason.(error); ok {
		return err.Error()
	}
	return fmt.Sprint(reason)
}

func isUARTTransportRetryableTimeout(reason interface{}) bool {
	message := uartTransportPanicMessage(reason)
	return strings.HasPrefix(message, "Timeout on wait for 'tmcuart_response'") ||
		strings.HasPrefix(message, "Timeout on send for 'tmcuart_response'")
}

func (self *UARTTransportRuntime) transportReadRegister(reg int64) (_ int64, ok bool) {
	defer func() {
		if reason := recover(); reason != nil {
			if isUARTTransportRetryableTimeout(reason) {
				ok = false
				return
			}
			panic(reason)
		}
	}()
	return self.transport.ReadRegister(self.addr, reg)
}

func (self *UARTTransportRuntime) transportWriteRegister(reg, val int64, printTime *float64) (ok bool) {
	ok = true
	defer func() {
		if reason := recover(); reason != nil {
			if isUARTTransportRetryableTimeout(reason) {
				ok = false
				return
			}
			panic(reason)
		}
	}()
	self.transport.WriteRegister(self.addr, reg, val, printTime)
	return ok
}

func (self *UARTTransportRuntime) GetRegister(regName string) (int64, error) {
	if self.debugEnabled != nil && self.debugEnabled() {
		return 0, nil
	}
	return self.readRegister(regName)
}

func (self *UARTTransportRuntime) readRegister(regName string) (int64, error) {
	reg := self.nameToReg[regName]
	for i := 0; i < uartTransportReadRetries; i++ {
		val, ok := self.transportReadRegister(reg)
		if ok {
			return val, nil
		}
	}
	return -1, fmt.Errorf("Unable to read tmc uart '%s' register %s", self.name, regName)
}

func (self *UARTTransportRuntime) pollIFCNTAfterWrite(expectedIFCNT int64) (currentIFCNT int64, matched bool, sawRead bool) {
	reg := self.nameToReg["IFCNT"]
	currentIFCNT = self.ifcnt
	for i := 0; i < uartTransportWriteVerifyPolls; i++ {
		nextIFCNT, ok := self.transportReadRegister(reg)
		if !ok {
			continue
		}
		sawRead = true
		currentIFCNT = nextIFCNT
		self.ifcnt = currentIFCNT
		self.hasIFCNT = true
		if currentIFCNT == expectedIFCNT {
			return currentIFCNT, true, true
		}
	}
	return currentIFCNT, false, sawRead
}

func (self *UARTTransportRuntime) SetRegister(regName string, val int64, printTime *float64) error {
	if self.debugEnabled != nil && self.debugEnabled() {
		return nil
	}
	reg := self.nameToReg[regName]

	if printTime != nil {
		if !self.transportWriteRegister(reg, val, printTime) {
			return fmt.Errorf("transport timeout on scheduled write")
		}
		self.hasIFCNT = false
		return nil
	}

	var lastErr error
	for i := 0; i < uartTransportWriteRetries; i++ {
		if !self.hasIFCNT {
			currentIFCNT, err := self.readRegister("IFCNT")
			if err != nil {
				lastErr = fmt.Errorf("reading IFCNT before write attempt %d: %w", i+1, err)
				continue
			}
			self.ifcnt = currentIFCNT
			self.hasIFCNT = true
		}
		ifcnt := self.ifcnt
		if !self.transportWriteRegister(reg, val, printTime) {
			self.hasIFCNT = false
			lastErr = fmt.Errorf("transport timeout on write attempt %d", i+1)
			continue
		}
		expectedIFCNT := (ifcnt + 1) & 0xff
		currentIFCNT, matched, sawRead := self.pollIFCNTAfterWrite(expectedIFCNT)
		if matched {
			return nil
		}
		if !sawRead {
			self.hasIFCNT = false
			lastErr = fmt.Errorf(
				"reading IFCNT after write attempt %d: %w",
				i+1,
				fmt.Errorf("Unable to read tmc uart '%s' register IFCNT", self.name),
			)
			continue
		}
		lastErr = fmt.Errorf(
			"IFCNT unchanged after write attempt %d (before=%d after=%d expected=%d)",
			i+1,
			ifcnt,
			currentIFCNT,
			expectedIFCNT,
		)
	}
	if lastErr != nil {
		return fmt.Errorf("Unable to write tmc uart '%s' register %s: %w", self.name, regName, lastErr)
	}
	return fmt.Errorf("Unable to write tmc uart '%s' register %s", self.name, regName)
}
