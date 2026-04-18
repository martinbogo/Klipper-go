package mcu

import "testing"

type fakeLegacySPIBusMCU struct {
	nextOID             int
	configCmds          []string
	queue               interface{}
	callbacks           []func()
	enumerations        map[string]interface{}
	constants           map[string]interface{}
	name                string
	command             *fakeBusCommandSender
	query               *fakeBusQuerySender
	lookupMsgs          []string
	queryMsgs           []string
	lastPrintTime       float64
	printTimeClockValue int64
}

func (self *fakeLegacySPIBusMCU) CreateOID() int {
	self.nextOID++
	return self.nextOID
}

func (self *fakeLegacySPIBusMCU) AddConfigCmd(cmd string, isInit, onRestart bool) {
	_ = isInit
	_ = onRestart
	self.configCmds = append(self.configCmds, cmd)
}

func (self *fakeLegacySPIBusMCU) AllocCommandQueue() interface{} {
	return self.queue
}

func (self *fakeLegacySPIBusMCU) LookupCommand(msgformat string, cmdQueue interface{}) (BusCommandSender, error) {
	_ = cmdQueue
	self.lookupMsgs = append(self.lookupMsgs, msgformat)
	return self.command, nil
}

func (self *fakeLegacySPIBusMCU) LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) BusQuerySender {
	_ = oid
	_ = cmdQueue
	_ = isAsync
	self.queryMsgs = append(self.queryMsgs, msgformat+"->"+respformat)
	return self.query
}

func (self *fakeLegacySPIBusMCU) Enumerations() map[string]interface{} {
	return self.enumerations
}

func (self *fakeLegacySPIBusMCU) Constants() map[string]interface{} {
	return self.constants
}

func (self *fakeLegacySPIBusMCU) Name() string {
	return self.name
}

func (self *fakeLegacySPIBusMCU) PrintTimeToClock(printTime float64) int64 {
	self.lastPrintTime = printTime
	return self.printTimeClockValue
}

func (self *fakeLegacySPIBusMCU) RegisterConfigCallback(cb func()) {
	self.callbacks = append(self.callbacks, cb)
}

func TestLegacySPIBusBuildConfigAndTransfers(t *testing.T) {
	mcu := &fakeLegacySPIBusMCU{
		queue:               "queue",
		name:                "mcu",
		enumerations:        map[string]interface{}{"spi_bus": map[string]interface{}{"spi0": 0}},
		constants:           map[string]interface{}{"BUS_PINS_spi0": "PA1,PA2"},
		command:             &fakeBusCommandSender{},
		query:               &fakeBusQuerySender{},
		printTimeClockValue: 42,
	}
	reserved := []string{}
	bus := NewLegacySPIBus(mcu, func(pin string, owner string) {
		reserved = append(reserved, pin+"@"+owner)
	}, "", "PA4", 3, 5000000, nil, true)

	if bus.GetOID() != 1 {
		t.Fatalf("unexpected bus oid %d", bus.GetOID())
	}
	if bus.CommandQueue() != "queue" {
		t.Fatalf("unexpected command queue %#v", bus.CommandQueue())
	}
	if len(mcu.configCmds) != 1 || mcu.configCmds[0] != "config_spi oid=1 pin=PA4 cs_active_high=true" {
		t.Fatalf("unexpected initial config commands %#v", mcu.configCmds)
	}
	if len(mcu.callbacks) != 1 {
		t.Fatalf("expected one config callback, got %d", len(mcu.callbacks))
	}

	mcu.callbacks[0]()
	if len(mcu.configCmds) != 2 || mcu.configCmds[1] != "spi_set_bus oid=1 spi_bus=spi0 mode=3 rate=5000000" {
		t.Fatalf("unexpected config commands after build %#v", mcu.configCmds)
	}
	if len(reserved) != 2 || reserved[0] != "PA1@spi0" || reserved[1] != "PA2@spi0" {
		t.Fatalf("unexpected reserved pins %#v", reserved)
	}

	bus.Send([]int{1, 2}, 5, 6)
	if len(mcu.command.calls) != 1 {
		t.Fatalf("expected one send call, got %d", len(mcu.command.calls))
	}

	mcu.query.sendResponse = map[string]interface{}{"response": []int{3, 4}}
	if got := bus.TransferResponse([]int{3}, 7, 8); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("unexpected transfer response %#v", got)
	}

	mcu.query.sendResponse = map[string]interface{}{"response": "pong"}
	if got := bus.Transfer([]int{9}, 10, 11); got != "pong" {
		t.Fatalf("unexpected transfer string %q", got)
	}

	mcu.query.prefaceResponse = map[string]interface{}{"response": "preface"}
	if got := bus.TransferWithPreface([]int{7}, []int{8}, 12, 13); got != "preface" {
		t.Fatalf("unexpected preface transfer %q", got)
	}
	if len(mcu.query.prefaceCalls) != 1 {
		t.Fatalf("expected one preface call, got %d", len(mcu.query.prefaceCalls))
	}

	if got := bus.PrintTimeToClock(1.25); got != 42 {
		t.Fatalf("unexpected print time clock %d", got)
	}
	if mcu.lastPrintTime != 1.25 {
		t.Fatalf("unexpected print time %.2f", mcu.lastPrintTime)
	}
}

func TestLegacySPIBusSetupShutdownAndPreBuildSendFallback(t *testing.T) {
	mcu := &fakeLegacySPIBusMCU{
		queue:        "queue",
		name:         "mcu",
		command:      &fakeBusCommandSender{},
		query:        &fakeBusQuerySender{},
		enumerations: map[string]interface{}{"spi_bus": map[string]interface{}{"spi0": 0}},
	}
	bus := NewLegacySPIBus(mcu, nil, "", nil, 0, 4000000, []interface{}{"MISO", "MOSI", "SCLK"}, false)

	bus.Send([]int{0xaa}, 0, 0)
	if len(mcu.configCmds) != 2 {
		t.Fatalf("expected initial config and fallback send, got %#v", mcu.configCmds)
	}
	if mcu.configCmds[0] != "config_spi_without_cs oid=1" {
		t.Fatalf("unexpected initial config %q", mcu.configCmds[0])
	}
	if mcu.configCmds[1] != "spi_send oid=1 data=aa" {
		t.Fatalf("unexpected fallback send command %q", mcu.configCmds[1])
	}

	bus.SetupShutdownMsg([]int{0x12, 0xab})
	if len(mcu.configCmds) != 3 || mcu.configCmds[2] != "config_spi_shutdown oid=2 spi_oid=1 shutdown_msg=12ab" {
		t.Fatalf("unexpected shutdown command %#v", mcu.configCmds)
	}
	if len(mcu.callbacks) != 1 {
		t.Fatalf("expected config callback registration, got %d", len(mcu.callbacks))
	}
}
