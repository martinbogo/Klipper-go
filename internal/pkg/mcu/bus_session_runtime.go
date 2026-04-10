package mcu

import (
	"fmt"
	"goklipper/common/utils/cast"
	"strings"
)

type BusNameResolver interface {
	Enumerations() map[string]interface{}
	Constants() map[string]interface{}
	MCUName() string
	ReservePin(pin string, owner string)
}

func ResolveBusName(resolver BusNameResolver, param string, bus *string) string {
	busName := ""
	if bus != nil {
		busName = *bus
	}

	enumerations := resolver.Enumerations()
	var enums interface{}
	if value, ok := enumerations[param]; ok {
		enums = value
	} else if value, ok := enumerations["bus"]; ok {
		enums = value
	} else {
		if bus == nil || busName == "" {
			return ""
		}
		return busName
	}

	enumsMap := enums.(map[string]interface{})
	if bus == nil || busName == "" {
		reverseEnums := make(map[int]string, len(enumsMap))
		for key, value := range enumsMap {
			reverseEnums[cast.ToInt(value)] = key
		}
		defaultBus, ok := reverseEnums[0]
		if !ok {
			panic(fmt.Errorf("Must specify %s on mcu '%s'", param, resolver.MCUName()))
		}
		busName = defaultBus
	}

	if _, ok := enumsMap[busName]; !ok {
		panic(fmt.Errorf("Unknown %s '%s'", param, busName))
	}

	if reservePins, ok := resolver.Constants()[fmt.Sprintf("BUS_PINS_%s", busName)]; ok {
		for _, pin := range strings.Split(cast.ToString(reservePins), ",") {
			resolver.ReservePin(pin, busName)
		}
	}

	return busName
}

type BusCommandSender interface {
	Send(data interface{}, minclock, reqclock int64)
}

type BusQuerySender interface {
	Send(data interface{}, minclock, reqclock int64) interface{}
	SendWithPreface(preface BusCommandSender, prefaceData interface{}, data interface{}, minclock, reqclock int64) interface{}
}

type BusCommandOwner interface {
	AddConfigCmd(cmd string, isInit, onRestart bool)
	LookupCommand(msgformat string, cmdQueue interface{}) (BusCommandSender, error)
	LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) BusQuerySender
}

type SPIBusRuntime struct {
	owner       BusCommandOwner
	resolver    BusNameResolver
	oid         int
	bus         string
	configFmt   string
	cmdQueue    interface{}
	spiSendCmd  BusCommandSender
	spiQueryCmd BusQuerySender
}

func NewSPIBusRuntime(owner BusCommandOwner, resolver BusNameResolver, oid int, bus string, configFmt string, cmdQueue interface{}) *SPIBusRuntime {
	return &SPIBusRuntime{owner: owner, resolver: resolver, oid: oid, bus: bus, configFmt: configFmt, cmdQueue: cmdQueue}
}

func (self *SPIBusRuntime) BuildConfig() {
	if strings.Contains(self.configFmt, "%") {
		bus := ResolveBusName(self.resolver, "spi_bus", cast.StringP(self.bus))
		self.configFmt = fmt.Sprintf(self.configFmt, bus)
	}

	self.owner.AddConfigCmd(self.configFmt, false, false)
	plan := BuildSPIConfigSetupPlan(self.oid, nil, 0, 0, nil, false)
	self.spiSendCmd, _ = self.owner.LookupCommand(plan.SendLookup, self.cmdQueue)
	self.spiQueryCmd = self.owner.LookupQueryCommand(plan.TransferRequest, plan.TransferResponse, self.oid, self.cmdQueue, false)
}

func (self *SPIBusRuntime) Send(data []int, minclock, reqclock int64) {
	if self.spiSendCmd == nil {
		self.owner.AddConfigCmd(BuildSPISendConfigCommand(self.oid, data), true, false)
		return
	}
	self.spiSendCmd.Send([]interface{}{self.oid, data}, minclock, reqclock)
}

func (self *SPIBusRuntime) Transfer(data []int, minclock, reqclock int64) interface{} {
	return self.spiQueryCmd.Send([]interface{}{self.oid, data}, minclock, reqclock)
}

func (self *SPIBusRuntime) TransferWithPreface(prefaceData []int, data []int, minclock, reqclock int64) interface{} {
	return self.spiQueryCmd.SendWithPreface(self.spiSendCmd, []interface{}{self.oid, prefaceData}, []interface{}{self.oid, data}, minclock, reqclock)
}

type I2CBusRuntime struct {
	owner            BusCommandOwner
	resolver         BusNameResolver
	oid              int
	bus              string
	i2cAddress       int
	configFmt        string
	cmdQueue         interface{}
	i2cWriteCmd      BusCommandSender
	i2cReadCmd       BusQuerySender
	i2cModifyBitsCmd BusCommandSender
}

func NewI2CBusRuntime(owner BusCommandOwner, resolver BusNameResolver, oid int, bus string, i2cAddress int, configFmt string, cmdQueue interface{}) *I2CBusRuntime {
	return &I2CBusRuntime{owner: owner, resolver: resolver, oid: oid, bus: bus, i2cAddress: i2cAddress, configFmt: configFmt, cmdQueue: cmdQueue}
}

func (self *I2CBusRuntime) BuildConfig() {
	bus := ResolveBusName(self.resolver, "i2c_bus", cast.StringP(self.bus))
	self.owner.AddConfigCmd(fmt.Sprintf(self.configFmt, bus), false, false)
	plan := BuildI2CConfigSetupPlan(self.oid, 0, self.i2cAddress)
	self.i2cWriteCmd, _ = self.owner.LookupCommand(plan.WriteLookup, self.cmdQueue)
	self.i2cReadCmd = self.owner.LookupQueryCommand(plan.ReadRequest, plan.ReadResponse, self.oid, self.cmdQueue, false)
	self.i2cModifyBitsCmd, _ = self.owner.LookupCommand(plan.ModifyBitsLookup, self.cmdQueue)
}

func (self *I2CBusRuntime) Write(data []int, minclock, reqclock int64) {
	if self.i2cWriteCmd == nil {
		self.owner.AddConfigCmd(BuildI2CWriteConfigCommand(self.oid, data), true, false)
		return
	}
	self.i2cWriteCmd.Send([]interface{}{self.oid, data}, minclock, reqclock)
}

func (self *I2CBusRuntime) Read(write []int, readLen int) interface{} {
	return self.i2cReadCmd.Send([]interface{}{self.oid, write, readLen}, 0, 0)
}

func (self *I2CBusRuntime) ModifyBits(reg string, clearBits string, setBits string, minclock, reqclock int64) {
	clearset := clearBits + setBits
	if self.i2cModifyBitsCmd == nil {
		self.owner.AddConfigCmd(BuildI2CModifyBitsConfigCommand(self.oid, reg, clearBits, setBits), true, true)
		return
	}
	self.i2cModifyBitsCmd.Send([]int64{int64(self.oid), int64(len(reg)), int64(len(clearset))}, minclock, reqclock)
}
