package tmc

import "fmt"

type UARTRegisterTransport interface {
	ReadRegister(addr, reg int64) (int64, bool)
	WriteRegister(addr, reg, val int64, printTime *float64)
}

type UARTTransportRuntime struct {
	name         string
	nameToReg    map[string]int64
	addr         int64
	transport    UARTRegisterTransport
	debugEnabled func() bool
	ifcnt        *int64
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

func (self *UARTTransportRuntime) GetRegister(regName string) (int64, error) {
	if self.debugEnabled != nil && self.debugEnabled() {
		return 0, nil
	}
	return self.readRegister(regName)
}

func (self *UARTTransportRuntime) readRegister(regName string) (int64, error) {
	reg := self.nameToReg[regName]
	for i := 0; i < 5; i++ {
		if val, ok := self.transport.ReadRegister(self.addr, reg); ok {
			return val, nil
		}
	}
	return -1, fmt.Errorf("Unable to read tmc uart '%s' register %s", self.name, regName)
}

func (self *UARTTransportRuntime) SetRegister(regName string, val int64, printTime *float64) error {
	if self.debugEnabled != nil && self.debugEnabled() {
		return nil
	}
	reg := self.nameToReg[regName]
	for i := 0; i < 5; i++ {
		ifcnt := self.ifcnt
		if ifcnt == nil {
			currentIFCNT, _ := self.readRegister("IFCNT")
			ifcnt = &currentIFCNT
			self.ifcnt = ifcnt
		}
		self.transport.WriteRegister(self.addr, reg, val, printTime)
		currentIFCNT, _ := self.readRegister("IFCNT")
		self.ifcnt = &currentIFCNT
		if currentIFCNT == ((*ifcnt + 1) & 0xff) {
			return nil
		}
	}
	return fmt.Errorf("Unable to write tmc uart '%s' register %s", self.name, regName)
}