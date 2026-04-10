package mcu

import "testing"

type fakeBusResolver struct {
	enumerations map[string]interface{}
	constants    map[string]interface{}
	name         string
	reserved     []string
}

func (self *fakeBusResolver) Enumerations() map[string]interface{} { return self.enumerations }
func (self *fakeBusResolver) Constants() map[string]interface{}    { return self.constants }
func (self *fakeBusResolver) MCUName() string                      { return self.name }
func (self *fakeBusResolver) ReservePin(pin string, owner string) {
	self.reserved = append(self.reserved, pin+"@"+owner)
}

type fakeBusCommandSender struct {
	calls []fakeBusSendCall
}

type fakeBusSendCall struct {
	data     interface{}
	minclock int64
	reqclock int64
}

func (self *fakeBusCommandSender) Send(data interface{}, minclock, reqclock int64) {
	self.calls = append(self.calls, fakeBusSendCall{data: data, minclock: minclock, reqclock: reqclock})
}

type fakeBusQuerySender struct {
	sendCalls       []fakeBusSendCall
	prefaceCalls    []fakeBusPrefaceCall
	sendResponse    interface{}
	prefaceResponse interface{}
}

type fakeBusPrefaceCall struct {
	preface     BusCommandSender
	prefaceData interface{}
	data        interface{}
	minclock    int64
	reqclock    int64
}

func (self *fakeBusQuerySender) Send(data interface{}, minclock, reqclock int64) interface{} {
	self.sendCalls = append(self.sendCalls, fakeBusSendCall{data: data, minclock: minclock, reqclock: reqclock})
	return self.sendResponse
}

func (self *fakeBusQuerySender) SendWithPreface(preface BusCommandSender, prefaceData interface{}, data interface{}, minclock, reqclock int64) interface{} {
	self.prefaceCalls = append(self.prefaceCalls, fakeBusPrefaceCall{preface: preface, prefaceData: prefaceData, data: data, minclock: minclock, reqclock: reqclock})
	return self.prefaceResponse
}

type fakeBusOwner struct {
	configCmds []string
	cmd        *fakeBusCommandSender
	query      *fakeBusQuerySender
	lookupMsgs []string
	queryMsgs  []string
}

func (self *fakeBusOwner) AddConfigCmd(cmd string, isInit, onRestart bool) {
	_ = isInit
	_ = onRestart
	self.configCmds = append(self.configCmds, cmd)
}

func (self *fakeBusOwner) LookupCommand(msgformat string, cmdQueue interface{}) (BusCommandSender, error) {
	_ = cmdQueue
	self.lookupMsgs = append(self.lookupMsgs, msgformat)
	return self.cmd, nil
}

func (self *fakeBusOwner) LookupQueryCommand(msgformat string, respformat string, oid int, cmdQueue interface{}, isAsync bool) BusQuerySender {
	_ = oid
	_ = cmdQueue
	_ = isAsync
	self.queryMsgs = append(self.queryMsgs, msgformat+"->"+respformat)
	return self.query
}

func TestResolveBusNameUsesDefaultEnumAndReservesPins(t *testing.T) {
	resolver := &fakeBusResolver{
		enumerations: map[string]interface{}{"spi_bus": map[string]interface{}{"spi0": 0, "spi1": 1}},
		constants:    map[string]interface{}{"BUS_PINS_spi0": "PA1,PA2"},
		name:         "mcu",
	}
	if got := ResolveBusName(resolver, "spi_bus", nil); got != "spi0" {
		t.Fatalf("expected spi0, got %q", got)
	}
	if len(resolver.reserved) != 2 || resolver.reserved[0] != "PA1@spi0" || resolver.reserved[1] != "PA2@spi0" {
		t.Fatalf("unexpected reserved pins %#v", resolver.reserved)
	}
}

func TestSPIBusRuntimeBuildConfigAndTransfers(t *testing.T) {
	owner := &fakeBusOwner{cmd: &fakeBusCommandSender{}, query: &fakeBusQuerySender{sendResponse: "response", prefaceResponse: "preface"}}
	resolver := &fakeBusResolver{enumerations: map[string]interface{}{"spi_bus": map[string]interface{}{"spi0": 0}}, name: "mcu"}
	runtime := NewSPIBusRuntime(owner, resolver, 7, "", "spi_set_bus oid=7 spi_bus=%s mode=3 rate=5000000", "queue")

	runtime.BuildConfig()
	if len(owner.configCmds) != 1 || owner.configCmds[0] != "spi_set_bus oid=7 spi_bus=spi0 mode=3 rate=5000000" {
		t.Fatalf("unexpected config commands %#v", owner.configCmds)
	}
	runtime.Send([]int{1, 2}, 5, 6)
	if len(owner.cmd.calls) != 1 {
		t.Fatalf("expected send call, got %d", len(owner.cmd.calls))
	}
	if got := runtime.Transfer([]int{3}, 7, 8); got != "response" {
		t.Fatalf("unexpected transfer response %#v", got)
	}
	if got := runtime.TransferWithPreface([]int{4}, []int{5}, 9, 10); got != "preface" {
		t.Fatalf("unexpected preface transfer response %#v", got)
	}
}

func TestSPIBusRuntimeSendFallsBackToConfigCommandBeforeBuild(t *testing.T) {
	owner := &fakeBusOwner{cmd: &fakeBusCommandSender{}, query: &fakeBusQuerySender{}}
	runtime := NewSPIBusRuntime(owner, &fakeBusResolver{}, 3, "", "spi_set_software_bus oid=3 ...", "queue")
	runtime.Send([]int{0xaa}, 0, 0)
	if len(owner.configCmds) != 1 || owner.configCmds[0] != "spi_send oid=3 data=aa" {
		t.Fatalf("unexpected fallback config commands %#v", owner.configCmds)
	}
}

func TestI2CBusRuntimeBuildConfigAndCommands(t *testing.T) {
	owner := &fakeBusOwner{cmd: &fakeBusCommandSender{}, query: &fakeBusQuerySender{sendResponse: "read"}}
	resolver := &fakeBusResolver{enumerations: map[string]interface{}{"i2c_bus": map[string]interface{}{"i2c0": 0}}, name: "mcu"}
	runtime := NewI2CBusRuntime(owner, resolver, 9, "", 42, "config_i2c oid=9 i2c_bus=%s rate=400000 address=42", "queue")

	runtime.BuildConfig()
	if len(owner.configCmds) != 1 || owner.configCmds[0] != "config_i2c oid=9 i2c_bus=i2c0 rate=400000 address=42" {
		t.Fatalf("unexpected config commands %#v", owner.configCmds)
	}
	runtime.Write([]int{1, 2}, 3, 4)
	if len(owner.cmd.calls) != 1 {
		t.Fatalf("expected I2C write send, got %d calls", len(owner.cmd.calls))
	}
	if got := runtime.Read([]int{5}, 2); got != "read" {
		t.Fatalf("unexpected I2C read response %#v", got)
	}
	runtime.ModifyBits("A", "\x01", "\x02", 6, 7)
	if len(owner.cmd.calls) != 2 {
		t.Fatalf("expected I2C modify-bits send, got %d calls", len(owner.cmd.calls))
	}
}

func TestI2CBusRuntimeModifyBitsFallsBackToConfigCommandBeforeBuild(t *testing.T) {
	owner := &fakeBusOwner{cmd: &fakeBusCommandSender{}, query: &fakeBusQuerySender{}}
	runtime := NewI2CBusRuntime(owner, &fakeBusResolver{}, 4, "", 33, "config_i2c oid=4 i2c_bus=%s rate=400000 address=33", "queue")
	runtime.ModifyBits("A", "\x01", "\x02", 0, 0)
	if len(owner.configCmds) != 1 || owner.configCmds[0] != "i2c_modify_bits oid=4 reg=41 clear_set_bits=0102" {
		t.Fatalf("unexpected fallback config commands %#v", owner.configCmds)
	}
}
