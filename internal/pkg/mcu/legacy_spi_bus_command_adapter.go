package mcu

import serialpkg "goklipper/internal/pkg/serialhdl"

// LegacySPIBusCommandHost adapts a legacy MCU runtime that still exposes the
// older project-side command lookup helpers used by SPI transport bootstraps.
type LegacySPIBusCommandHost interface {
	Create_oid() int
	Add_config_cmd(cmd string, isInit, onRestart bool)
	Alloc_command_queue() interface{}
	Lookup_command(msgformat string, cmdQueue interface{}) (*serialpkg.CommandWrapper, error)
	Lookup_query_command(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) *serialpkg.CommandQueryWrapper
	Get_enumerations() map[string]interface{}
	Get_constants() map[string]interface{}
	Get_name() string
	PrintTimeToClock(printTime float64) int64
	Register_config_callback(cb interface{})
}

type legacySPIBusCommandAdapter struct {
	raw LegacySPIBusCommandHost
}

func NewLegacySPIBusCommandAdapter(raw LegacySPIBusCommandHost) LegacySPIBusMCU {
	return &legacySPIBusCommandAdapter{raw: raw}
}

func (self *legacySPIBusCommandAdapter) CreateOID() int {
	return self.raw.Create_oid()
}

func (self *legacySPIBusCommandAdapter) AddConfigCmd(cmd string, isInit, onRestart bool) {
	self.raw.Add_config_cmd(cmd, isInit, onRestart)
}

func (self *legacySPIBusCommandAdapter) AllocCommandQueue() interface{} {
	return self.raw.Alloc_command_queue()
}

func (self *legacySPIBusCommandAdapter) LookupCommand(msgformat string, cmdQueue interface{}) (BusCommandSender, error) {
	cmd, err := self.raw.Lookup_command(msgformat, cmdQueue)
	if err != nil {
		return nil, err
	}
	return &BusCommandSenderAdapter{
		Raw: cmd,
		SendFunc: func(data interface{}, minclock, reqclock int64) {
			cmd.Send(data, minclock, reqclock)
		},
	}, nil
}

func (self *legacySPIBusCommandAdapter) LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) BusQuerySender {
	cmd := self.raw.Lookup_query_command(msgformat, respformat, oid, cmdQueue, isAsync)
	return &BusQuerySenderAdapter{
		SendFunc: func(data interface{}, minclock, reqclock int64) interface{} {
			return cmd.Send(data, minclock, reqclock)
		},
		SendWithPrefaceFunc: func(prefaceRaw interface{}, prefaceData interface{}, data interface{}, minclock, reqclock int64) interface{} {
			prefaceCmd, ok := prefaceRaw.(*serialpkg.CommandWrapper)
			if !ok {
				panic("unexpected preface sender raw type")
			}
			return cmd.Send_with_preface(prefaceCmd, prefaceData, data, minclock, reqclock)
		},
	}
}

func (self *legacySPIBusCommandAdapter) Enumerations() map[string]interface{} {
	return self.raw.Get_enumerations()
}

func (self *legacySPIBusCommandAdapter) Constants() map[string]interface{} {
	return self.raw.Get_constants()
}

func (self *legacySPIBusCommandAdapter) Name() string {
	return self.raw.Get_name()
}

func (self *legacySPIBusCommandAdapter) PrintTimeToClock(printTime float64) int64 {
	return self.raw.PrintTimeToClock(printTime)
}

func (self *legacySPIBusCommandAdapter) RegisterConfigCallback(cb func()) {
	self.raw.Register_config_callback(cb)
}
