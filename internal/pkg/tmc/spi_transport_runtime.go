package tmc

import "fmt"

type SPIBusTransport interface {
	Send(data []int, minclock, reqclock int64)
	Transfer(data []int, minclock, reqclock int64) string
	TransferWithPreface(prefaceData, data []int, minclock, reqclock int64) string
	PrintTimeToClock(printTime float64) int64
}

type SPIChainRuntime struct {
	chainLen       int64
	transport      SPIBusTransport
	debugEnabled   func() bool
	takenPositions map[int64]bool
}

func NewSPIChainRuntime(chainLen int64, transport SPIBusTransport, debugEnabled func() bool) *SPIChainRuntime {
	return &SPIChainRuntime{
		chainLen:       chainLen,
		transport:      transport,
		debugEnabled:   debugEnabled,
		takenPositions: make(map[int64]bool),
	}
}

func (self *SPIChainRuntime) ChainLen() int64 {
	return self.chainLen
}

func (self *SPIChainRuntime) RegisterPosition(chainPos int64) error {
	if self.takenPositions[chainPos] {
		return fmt.Errorf("TMC SPI chain can not have duplicate position")
	}
	self.takenPositions[chainPos] = true
	return nil
}

func (self *SPIChainRuntime) buildCmd(data []int64, chainPos int64) []int {
	return BuildSPIChainCommand(data, self.chainLen, chainPos)
}

func (self *SPIChainRuntime) ReadRegister(reg, chainPos int64) int64 {
	cmd := self.buildCmd([]int64{reg, 0x00, 0x00, 0x00, 0x00}, chainPos)
	self.transport.Send(cmd, 0, 0)
	if self.debugEnabled != nil && self.debugEnabled() {
		return 0
	}
	response := self.transport.Transfer(cmd, 0, 0)
	return DecodeSPIChainResponse(response, self.chainLen, chainPos)
}

func (self *SPIChainRuntime) WriteRegister(reg, val, chainPos int64, printTime *float64) int64 {
	minclock := int64(0)
	if printTime != nil {
		minclock = self.transport.PrintTimeToClock(*printTime)
	}
	data := []int64{(reg | 0x80) & 0xff, (val >> 24) & 0xff, (val >> 16) & 0xff, (val >> 8) & 0xff, val & 0xff}
	if self.debugEnabled != nil && self.debugEnabled() {
		self.transport.Send(self.buildCmd(data, chainPos), minclock, 0)
		return val
	}
	writeCmd := self.buildCmd(data, chainPos)
	dummyRead := self.buildCmd([]int64{0x00, 0x00, 0x00, 0x00, 0x00}, chainPos)
	response := self.transport.TransferWithPreface(writeCmd, dummyRead, minclock, 0)
	return DecodeSPIChainResponse(response, self.chainLen, chainPos)
}

type SPIRegisterTransportRuntime struct {
	name      string
	nameToReg map[string]int64
	chainPos  int64
	chain     *SPIChainRuntime
}

func NewSPIRegisterTransportRuntime(name string, nameToReg map[string]int64, chainPos int64, chain *SPIChainRuntime) *SPIRegisterTransportRuntime {
	return &SPIRegisterTransportRuntime{name: name, nameToReg: nameToReg, chainPos: chainPos, chain: chain}
}

func NewLockedSPIRegisterAccess(name string, nameToReg map[string]int64, chainPos int64, chain *SPIChainRuntime, fields *FieldHelper, mutex RegisterLocker) RegisterAccess {
	runtime := NewSPIRegisterTransportRuntime(name, nameToReg, chainPos, chain)
	return NewLockedRegisterAccess(fields, runtime, mutex)
}

func (self *SPIRegisterTransportRuntime) GetRegister(regName string) (int64, error) {
	return self.chain.ReadRegister(self.nameToReg[regName], self.chainPos), nil
}

func (self *SPIRegisterTransportRuntime) SetRegister(regName string, val int64, printTime *float64) error {
	reg := self.nameToReg[regName]
	for i := 0; i < 5; i++ {
		if self.chain.WriteRegister(reg, val, self.chainPos, printTime) == val {
			return nil
		}
	}
	return fmt.Errorf("Unable to write tmc spi '%s' register %s", self.name, regName)
}

type TMC2660SPITransportRuntime struct {
	nameToReg    map[string]int64
	fields       *FieldHelper
	transport    SPIBusTransport
	debugEnabled func() bool
}

func NewTMC2660SPITransportRuntime(nameToReg map[string]int64, fields *FieldHelper, transport SPIBusTransport, debugEnabled func() bool) *TMC2660SPITransportRuntime {
	return &TMC2660SPITransportRuntime{nameToReg: nameToReg, fields: fields, transport: transport, debugEnabled: debugEnabled}
}

func NewLockedTMC2660SPIRegisterAccess(nameToReg map[string]int64, fields *FieldHelper, transport SPIBusTransport, debugEnabled func() bool, mutex RegisterLocker) RegisterAccess {
	runtime := NewTMC2660SPITransportRuntime(nameToReg, fields, transport, debugEnabled)
	return NewLockedRegisterAccess(fields, runtime, mutex)
}

func (self *TMC2660SPITransportRuntime) GetRegister(regName string) (int64, error) {
	newRdsel := indexOfString(TMC2660ReadRegisters, regName)
	reg := self.nameToReg["DRVCONF"]
	if self.debugEnabled != nil && self.debugEnabled() {
		return 0, nil
	}
	oldRdsel := self.fields.Get_field("rdsel", 0, nil)
	val := self.fields.Set_field("rdsel", newRdsel, nil, nil)
	msg := []int{int(((val >> 16) | reg) & 0xff), int((val >> 8) & 0xff), int(val & 0xff)}
	if int64(newRdsel) != oldRdsel {
		self.transport.Send(msg, 0, 0)
	}
	response := self.transport.Transfer(msg, 0, 0)
	pr := []byte(response)
	return (int64(pr[0]) << 16) | (int64(pr[1]) << 8) | int64(pr[2]), nil
}

func (self *TMC2660SPITransportRuntime) SetRegister(regName string, val int64, printTime *float64) error {
	minclock := int64(0)
	if printTime != nil {
		minclock = self.transport.PrintTimeToClock(*printTime)
	}
	reg := self.nameToReg[regName]
	msg := []int{int(((val >> 16) | reg) & 0xff), int((val >> 8) & 0xff), int(val & 0xff)}
	self.transport.Send(msg, minclock, 0)
	return nil
}

func indexOfString(data []string, element string) int {
	for idx, value := range data {
		if value == element {
			return idx
		}
	}
	return -1
}
