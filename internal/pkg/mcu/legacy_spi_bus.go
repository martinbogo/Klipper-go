package mcu

import "goklipper/common/utils/cast"

type LegacySPIBusMCU interface {
	CreateOID() int
	AddConfigCmd(cmd string, isInit, onRestart bool)
	AllocCommandQueue() interface{}
	LookupCommand(msgformat string, cmdQueue interface{}) (BusCommandSender, error)
	LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) BusQuerySender
	Enumerations() map[string]interface{}
	Constants() map[string]interface{}
	Name() string
	PrintTimeToClock(printTime float64) int64
	RegisterConfigCallback(cb func())
}

type LegacySPIBus struct {
	mcu      LegacySPIBusMCU
	oid      int
	cmdQueue interface{}
	runtime  *SPIBusRuntime
}

func NewLegacySPIBus(mcu LegacySPIBusMCU, reservePin func(pin string, owner string), bus string, pin interface{}, mode, speed int, swPins []interface{}, csActiveHigh bool) *LegacySPIBus {
	self := &LegacySPIBus{mcu: mcu, oid: mcu.CreateOID()}
	plan := BuildSPIConfigSetupPlan(self.oid, pin, mode, speed, swPins, csActiveHigh)
	for _, cmd := range plan.InitialConfigCmds {
		mcu.AddConfigCmd(cmd, false, false)
	}

	self.cmdQueue = mcu.AllocCommandQueue()
	if reservePin == nil {
		reservePin = func(string, string) {}
	}
	self.runtime = NewSPIBusRuntime(
		BusCommandOwnerFuncs{
			AddConfigCmdFunc:       mcu.AddConfigCmd,
			LookupCommandFunc:      mcu.LookupCommand,
			LookupQueryCommandFunc: mcu.LookupQueryCommand,
		},
		BusNameResolverFuncs{
			EnumerationsFunc: mcu.Enumerations,
			ConstantsFunc:    mcu.Constants,
			MCUNameFunc:      mcu.Name,
			ReservePinFunc:   reservePin,
		},
		self.oid,
		bus,
		plan.ConfigFormat,
		self.cmdQueue,
	)
	mcu.RegisterConfigCallback(self.BuildConfig)
	return self
}

func (self *LegacySPIBus) SetupShutdownMsg(shutdownSeq []int) {
	self.mcu.AddConfigCmd(BuildSPIShutdownConfigCommand(self.mcu.CreateOID(), self.oid, shutdownSeq), false, false)
}

func (self *LegacySPIBus) GetOID() int {
	return self.oid
}

func (self *LegacySPIBus) CommandQueue() interface{} {
	return self.cmdQueue
}

func (self *LegacySPIBus) BuildConfig() {
	self.runtime.BuildConfig()
}

func (self *LegacySPIBus) Send(data []int, minclock, reqclock int64) {
	self.runtime.Send(data, minclock, reqclock)
}

func (self *LegacySPIBus) TransferResponse(data []int, minclock, reqclock int64) []int {
	params := self.runtime.Transfer(data, minclock, reqclock).(map[string]interface{})
	return params["response"].([]int)
}

func (self *LegacySPIBus) Transfer(data []int, minclock, reqclock int64) string {
	params := self.runtime.Transfer(data, minclock, reqclock).(map[string]interface{})
	return cast.ToString(params["response"])
}

func (self *LegacySPIBus) TransferWithPreface(prefaceData, data []int, minclock, reqclock int64) string {
	params := self.runtime.TransferWithPreface(prefaceData, data, minclock, reqclock).(map[string]interface{})
	return cast.ToString(params["response"])
}

func (self *LegacySPIBus) PrintTimeToClock(printTime float64) int64 {
	return self.mcu.PrintTimeToClock(printTime)
}
