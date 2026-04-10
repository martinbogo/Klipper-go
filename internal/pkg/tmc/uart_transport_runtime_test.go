package tmc

import "testing"

type uartReadResult struct {
	value int64
	ok    bool
}

type fakeUARTRegisterTransport struct {
	reads     map[int64][]uartReadResult
	readCalls map[int64]int
	writes    []uartWriteCall
}

type uartWriteCall struct {
	addr      int64
	reg       int64
	value     int64
	printTime *float64
}

func (self *fakeUARTRegisterTransport) ReadRegister(addr, reg int64) (int64, bool) {
	_ = addr
	if self.readCalls == nil {
		self.readCalls = map[int64]int{}
	}
	self.readCalls[reg]++
	queue := self.reads[reg]
	if len(queue) == 0 {
		return 0, false
	}
	result := queue[0]
	self.reads[reg] = queue[1:]
	return result.value, result.ok
}

func (self *fakeUARTRegisterTransport) WriteRegister(addr, reg, value int64, printTime *float64) {
	self.writes = append(self.writes, uartWriteCall{addr: addr, reg: reg, value: value, printTime: printTime})
}

func TestUARTTransportRuntimeGetRegisterRetriesUntilSuccess(t *testing.T) {
	transport := &fakeUARTRegisterTransport{reads: map[int64][]uartReadResult{
		0x10: {{ok: false}, {ok: false}, {value: 42, ok: true}},
	}}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x10, "IFCNT": 0x02}, 3, transport, func() bool { return false })

	got, err := runtime.GetRegister("GCONF")
	if err != nil {
		t.Fatalf("GetRegister returned error: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if transport.readCalls[0x10] != 3 {
		t.Fatalf("expected 3 read attempts, got %d", transport.readCalls[0x10])
	}
}

func TestUARTTransportRuntimeSetRegisterCachesIFCNTAcrossWrites(t *testing.T) {
	transport := &fakeUARTRegisterTransport{reads: map[int64][]uartReadResult{
		0x02: {{value: 7, ok: true}, {value: 8, ok: true}, {value: 9, ok: true}},
	}}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x10, "IFCNT": 0x02}, 1, transport, func() bool { return false })

	if err := runtime.SetRegister("GCONF", 11, nil); err != nil {
		t.Fatalf("first SetRegister returned error: %v", err)
	}
	if err := runtime.SetRegister("GCONF", 12, nil); err != nil {
		t.Fatalf("second SetRegister returned error: %v", err)
	}
	if transport.readCalls[0x02] != 3 {
		t.Fatalf("expected IFCNT to be read 3 times, got %d", transport.readCalls[0x02])
	}
	if len(transport.writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(transport.writes))
	}
	if transport.writes[0].value != 11 || transport.writes[1].value != 12 {
		t.Fatalf("unexpected write values: %#v", transport.writes)
	}
}

func TestUARTTransportRuntimeSetRegisterRetriesUntilIFCNTAdvances(t *testing.T) {
	transport := &fakeUARTRegisterTransport{reads: map[int64][]uartReadResult{
		0x02: {{value: 3, ok: true}, {value: 3, ok: true}, {value: 4, ok: true}},
	}}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x10, "IFCNT": 0x02}, 5, transport, func() bool { return false })

	if err := runtime.SetRegister("GCONF", 99, nil); err != nil {
		t.Fatalf("SetRegister returned error: %v", err)
	}
	if len(transport.writes) != 2 {
		t.Fatalf("expected 2 write attempts, got %d", len(transport.writes))
	}
	if transport.readCalls[0x02] != 3 {
		t.Fatalf("expected 3 IFCNT reads, got %d", transport.readCalls[0x02])
	}
}

func TestUARTTransportRuntimeDebugOutputSkipsIO(t *testing.T) {
	transport := &fakeUARTRegisterTransport{reads: map[int64][]uartReadResult{
		0x10: {{value: 55, ok: true}},
		0x02: {{value: 9, ok: true}},
	}}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x10, "IFCNT": 0x02}, 7, transport, func() bool { return true })

	got, err := runtime.GetRegister("GCONF")
	if err != nil {
		t.Fatalf("GetRegister returned error: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected debug read to return 0, got %d", got)
	}
	if err := runtime.SetRegister("GCONF", 12, nil); err != nil {
		t.Fatalf("SetRegister returned error: %v", err)
	}
	if len(transport.writes) != 0 {
		t.Fatalf("expected no writes in debug mode, got %d", len(transport.writes))
	}
	if len(transport.readCalls) != 0 {
		t.Fatalf("expected no reads in debug mode, got %#v", transport.readCalls)
	}
}