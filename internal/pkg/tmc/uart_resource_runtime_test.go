package tmc

import "testing"

type fakeUARTMuxActivator struct {
	activations [][]int64
}

func (self *fakeUARTMuxActivator) Activate(instanceID []int64) {
	self.activations = append(self.activations, append([]int64(nil), instanceID...))
}

type fakeUARTMuxCommand struct {
	sends [][]int64
}

func (self *fakeUARTMuxCommand) Send(data interface{}, minclock, reqclock int64) {
	self.sends = append(self.sends, append([]int64(nil), data.([]int64)...))
	_ = minclock
	_ = reqclock
}

type fakeUARTMuxOwner struct {
	nextOID    int
	configCmds []string
	callback   func()
	command    *fakeUARTMuxCommand
}

func (self *fakeUARTMuxOwner) CreateOID() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeUARTMuxOwner) AddConfigCmd(cmd string, isInit, onRestart bool) {
	self.configCmds = append(self.configCmds, cmd)
	_ = isInit
	_ = onRestart
}

func (self *fakeUARTMuxOwner) RegisterConfigCallback(callback func()) {
	self.callback = callback
}

func (self *fakeUARTMuxOwner) LookupCommand(msgformat string, cmdQueue interface{}) (UARTMuxCommand, error) {
	_ = msgformat
	_ = cmdQueue
	return self.command, nil
}

func TestUARTAnalogMuxRuntimeConfiguresPinsAndOnlySendsChanges(t *testing.T) {
	owner := &fakeUARTMuxOwner{nextOID: 7, command: &fakeUARTMuxCommand{}}
	runtime := NewUARTAnalogMuxRuntime(owner, "cmdq", []interface{}{"p1", "p2"})

	if len(owner.configCmds) != 2 {
		t.Fatalf("expected two config commands, got %d", len(owner.configCmds))
	}
	if owner.callback == nil {
		t.Fatal("expected config callback registration")
	}
	owner.callback()

	runtime.Activate([]int64{1, 0})
	runtime.Activate([]int64{1, 1})

	if len(owner.command.sends) != 3 {
		t.Fatalf("expected three pin updates, got %d", len(owner.command.sends))
	}
	if owner.command.sends[0][0] != 7 || owner.command.sends[0][1] != 1 {
		t.Fatalf("unexpected first send %#v", owner.command.sends[0])
	}
	if owner.command.sends[1][0] != 8 || owner.command.sends[1][1] != 0 {
		t.Fatalf("unexpected second send %#v", owner.command.sends[1])
	}
	if owner.command.sends[2][0] != 8 || owner.command.sends[2][1] != 1 {
		t.Fatalf("unexpected third send %#v", owner.command.sends[2])
	}
	if got := runtime.Pins(); len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
		t.Fatalf("unexpected mux pins %#v", got)
	}
}

func TestUARTMutexCacheReusesEntry(t *testing.T) {
	cache := NewUARTMutexCache()
	factoryCalls := 0
	first := cache.Lookup("mcu", func() interface{} {
		factoryCalls++
		return "mutex"
	})
	second := cache.Lookup("mcu", func() interface{} {
		factoryCalls++
		return "other"
	})

	if first != "mutex" || second != "mutex" {
		t.Fatalf("expected cached mutex, got first=%#v second=%#v", first, second)
	}
	if factoryCalls != 1 {
		t.Fatalf("expected factory to be called once, got %d", factoryCalls)
	}
}

func TestUARTSharedResourceRuntimeRejectsMismatchedPins(t *testing.T) {
	runtime := NewUARTSharedResourceRuntime("rx0", "tx0", nil, nil, nil)
	if _, err := runtime.RegisterInstance("rx1", "tx0", nil, 0); err == nil {
		t.Fatal("expected mismatched pins error")
	}
}

func TestUARTSharedResourceRuntimeValidatesMuxPinsAndUniqueness(t *testing.T) {
	mux := &fakeUARTMuxActivator{}
	runtime := NewUARTSharedResourceRuntime("rx0", "tx0", "mcu0", []interface{}{"p1", "p2"}, mux)
	selectPins := []UARTSelectPin{{Owner: "mcu0", Pin: "p1", Invert: false}, {Owner: "mcu0", Pin: "p2", Invert: true}}

	instanceID, err := runtime.RegisterInstance("rx0", "tx0", selectPins, 2)
	if err != nil {
		t.Fatalf("RegisterInstance returned error: %v", err)
	}
	if len(instanceID) != 2 || instanceID[0] != 1 || instanceID[1] != 0 {
		t.Fatalf("unexpected instance id %#v", instanceID)
	}
	if _, err := runtime.RegisterInstance("rx0", "tx0", selectPins, 2); err == nil {
		t.Fatal("expected duplicate instance error")
	}
	if _, err := runtime.RegisterInstance("rx0", "tx0", []UARTSelectPin{{Owner: "mcu1", Pin: "p1", Invert: false}, {Owner: "mcu0", Pin: "p2", Invert: false}}, 3); err == nil {
		t.Fatal("expected foreign mux owner error")
	}
	if _, err := runtime.RegisterInstance("rx0", "tx0", []UARTSelectPin{{Owner: "mcu0", Pin: "other", Invert: false}, {Owner: "mcu0", Pin: "p2", Invert: false}}, 4); err == nil {
		t.Fatal("expected mismatched mux pins error")
	}
}

func TestUARTSharedResourceRuntimePrepareTransferActivatesMux(t *testing.T) {
	mux := &fakeUARTMuxActivator{}
	runtime := NewUARTSharedResourceRuntime("rx0", "tx0", "mcu0", []interface{}{"p1"}, mux)
	runtime.PrepareTransfer([]int64{1})
	if len(mux.activations) != 1 {
		t.Fatalf("expected one activation, got %d", len(mux.activations))
	}
	if len(mux.activations[0]) != 1 || mux.activations[0][0] != 1 {
		t.Fatalf("unexpected activation payload %#v", mux.activations[0])
	}
}
