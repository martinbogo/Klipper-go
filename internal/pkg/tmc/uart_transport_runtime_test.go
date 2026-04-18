package tmc

import (
	"fmt"
	"strings"
	"testing"
)

type uartReadResult struct {
	value int64
	ok    bool
}

type fakeUARTRegisterTransport struct {
	reads      map[int64][]uartReadResult
	readCalls  map[int64]int
	readHook   func(addr, reg int64) (int64, bool)
	writeHook  func(addr, reg, value int64, printTime *float64)
	writes     []uartWriteCall
	writeCalls int
}

type uartWriteCall struct {
	addr      int64
	reg       int64
	value     int64
	printTime *float64
}

func TestNewUARTRegisterTransportFuncsDelegatesReadAndWrite(t *testing.T) {
	readCalls := 0
	writeCalls := 0
	printTime := 12.5
	transport := NewUARTRegisterTransportFuncs(
		func(addr, reg int64) (int64, bool) {
			readCalls++
			if addr != 7 || reg != 0x10 {
				t.Fatalf("unexpected read args: addr=%d reg=%d", addr, reg)
			}
			return 33, true
		},
		func(addr, reg, value int64, gotPrintTime *float64) {
			writeCalls++
			if addr != 7 || reg != 0x11 || value != 44 {
				t.Fatalf("unexpected write args: addr=%d reg=%d value=%d", addr, reg, value)
			}
			if gotPrintTime == nil || *gotPrintTime != printTime {
				t.Fatalf("unexpected print time: %v", gotPrintTime)
			}
		},
	)

	value, ok := transport.ReadRegister(7, 0x10)
	if !ok || value != 33 {
		t.Fatalf("unexpected read result: value=%d ok=%v", value, ok)
	}
	transport.WriteRegister(7, 0x11, 44, &printTime)
	if readCalls != 1 || writeCalls != 1 {
		t.Fatalf("expected one delegated read and write, got reads=%d writes=%d", readCalls, writeCalls)
	}
}

func (self *fakeUARTRegisterTransport) ReadRegister(addr, reg int64) (int64, bool) {
	if self.readHook != nil {
		return self.readHook(addr, reg)
	}
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
	self.writeCalls++
	if self.writeHook != nil {
		self.writeHook(addr, reg, value, printTime)
	}
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

func TestUARTTransportRuntimeGetRegisterRetriesTimeoutPanics(t *testing.T) {
	attempts := 0
	transport := &fakeUARTRegisterTransport{
		readHook: func(addr, reg int64) (int64, bool) {
			_ = addr
			if reg != 0x10 {
				return 0, false
			}
			attempts++
			if attempts < 3 {
				panic("Timeout on wait for 'tmcuart_response' response '0'")
			}
			return 42, true
		},
	}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x10, "IFCNT": 0x02}, 3, transport, func() bool { return false })

	got, err := runtime.GetRegister("GCONF")
	if err != nil {
		t.Fatalf("GetRegister returned error: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 read attempts, got %d", attempts)
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
	if !runtime.hasIFCNT {
		t.Fatal("expected IFCNT cache to be populated after writes")
	}
	if runtime.ifcnt != 9 {
		t.Fatalf("expected latest cached IFCNT to be 9, got %d", runtime.ifcnt)
	}
}

func TestUARTTransportRuntimeSetRegisterWaitsForDelayedIFCNTAdvanceBeforeRetryingWrite(t *testing.T) {
	transport := &fakeUARTRegisterTransport{reads: map[int64][]uartReadResult{
		0x02: {
			{value: 3, ok: true},
			{ok: false},
			{value: 3, ok: true},
			{value: 4, ok: true},
		},
	}}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x10, "IFCNT": 0x02}, 5, transport, func() bool { return false })

	if err := runtime.SetRegister("GCONF", 99, nil); err != nil {
		t.Fatalf("SetRegister returned error: %v", err)
	}
	if len(transport.writes) != 1 {
		t.Fatalf("expected delayed IFCNT polling to avoid a second write, got %d writes", len(transport.writes))
	}
	if transport.readCalls[0x02] != 4 {
		t.Fatalf("expected 4 IFCNT reads, got %d", transport.readCalls[0x02])
	}
}

func TestUARTTransportRuntimeSetRegisterRetriesWriteAfterVerificationPollsExhausted(t *testing.T) {
	reads := []uartReadResult{{value: 3, ok: true}}
	for i := 0; i < uartTransportWriteVerifyPolls; i++ {
		reads = append(reads, uartReadResult{value: 3, ok: true})
	}
	reads = append(reads, uartReadResult{value: 4, ok: true})
	transport := &fakeUARTRegisterTransport{reads: map[int64][]uartReadResult{0x02: reads}}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x10, "IFCNT": 0x02}, 5, transport, func() bool { return false })

	if err := runtime.SetRegister("GCONF", 99, nil); err != nil {
		t.Fatalf("SetRegister returned error: %v", err)
	}
	if len(transport.writes) != 2 {
		t.Fatalf("expected 2 write attempts after verification polls were exhausted, got %d", len(transport.writes))
	}
	if transport.readCalls[0x02] != uartTransportWriteVerifyPolls+2 {
		t.Fatalf("expected %d IFCNT reads, got %d", uartTransportWriteVerifyPolls+2, transport.readCalls[0x02])
	}
}

func TestUARTTransportRuntimeSetRegisterRetriesWriteTimeoutPanics(t *testing.T) {
	writeAttempts := 0
	transport := &fakeUARTRegisterTransport{
		reads: map[int64][]uartReadResult{
			0x02: {{value: 7, ok: true}, {value: 7, ok: true}, {value: 8, ok: true}},
		},
		writeHook: func(addr, reg, value int64, printTime *float64) {
			_ = addr
			_ = reg
			_ = value
			_ = printTime
			writeAttempts++
			if writeAttempts == 1 {
				panic("Timeout on wait for 'tmcuart_response' response '0'")
			}
		},
	}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x10, "IFCNT": 0x02}, 5, transport, func() bool { return false })

	if err := runtime.SetRegister("GCONF", 99, nil); err != nil {
		t.Fatalf("SetRegister returned error: %v", err)
	}
	if writeAttempts != 2 {
		t.Fatalf("expected 2 write attempts, got %d", writeAttempts)
	}
	if transport.writeCalls != 2 {
		t.Fatalf("expected 2 write calls, got %d", transport.writeCalls)
	}
	if len(transport.writes) != 1 {
		t.Fatalf("expected 1 completed write after retry, got %d", len(transport.writes))
	}
	if transport.readCalls[0x02] != 3 {
		t.Fatalf("expected 3 IFCNT reads, got %d", transport.readCalls[0x02])
	}
}

func TestUARTTransportRuntimeSetRegisterErrorIncludesIFCNTMismatchDetail(t *testing.T) {
	reads := []uartReadResult{{value: 3, ok: true}}
	for i := 0; i < uartTransportWriteRetries*uartTransportWriteVerifyPolls; i++ {
		reads = append(reads, uartReadResult{value: 3, ok: true})
	}
	transport := &fakeUARTRegisterTransport{reads: map[int64][]uartReadResult{0x02: reads}}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"SLAVECONF": 0x03, "IFCNT": 0x02}, 5, transport, func() bool { return false })

	err := runtime.SetRegister("SLAVECONF", 0x0200, nil)
	if err == nil {
		t.Fatal("expected SetRegister to fail when IFCNT never advances")
	}
	if got := err.Error(); !strings.Contains(got, "Unable to write tmc uart 'stepper_x' register SLAVECONF") {
		t.Fatalf("expected write error prefix, got %q", got)
	}
	if got := err.Error(); !strings.Contains(got, fmt.Sprintf("IFCNT unchanged after write attempt %d (before=3 after=3 expected=4)", uartTransportWriteRetries)) {
		t.Fatalf("expected IFCNT mismatch detail, got %q", got)
	}
}

func TestUARTTransportRuntimeSetRegisterErrorIncludesTransportTimeoutDetail(t *testing.T) {
	reads := make([]uartReadResult, 0, uartTransportWriteRetries)
	for i := 0; i < uartTransportWriteRetries; i++ {
		reads = append(reads, uartReadResult{value: 7, ok: true})
	}
	transport := &fakeUARTRegisterTransport{
		reads: map[int64][]uartReadResult{
			0x02: reads,
		},
		writeHook: func(addr, reg, value int64, printTime *float64) {
			_ = addr
			_ = reg
			_ = value
			_ = printTime
			panic("Timeout on wait for 'tmcuart_response' response '0'")
		},
	}
	runtime := NewUARTTransportRuntime("stepper_x", map[string]int64{"SLAVECONF": 0x03, "IFCNT": 0x02}, 5, transport, func() bool { return false })

	err := runtime.SetRegister("SLAVECONF", 0x0200, nil)
	if err == nil {
		t.Fatal("expected SetRegister to fail after repeated transport timeouts")
	}
	if got := err.Error(); !strings.Contains(got, "Unable to write tmc uart 'stepper_x' register SLAVECONF") {
		t.Fatalf("expected write error prefix, got %q", got)
	}
	if got := err.Error(); !strings.Contains(got, fmt.Sprintf("transport timeout on write attempt %d", uartTransportWriteRetries)) {
		t.Fatalf("expected timeout detail, got %q", got)
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

func TestNewLockedUARTRegisterAccessUsesLockerForReadsAndWrites(t *testing.T) {
	transport := &fakeUARTRegisterTransport{reads: map[int64][]uartReadResult{
		0x10: {{value: 77, ok: true}},
		0x02: {{value: 4, ok: true}, {value: 5, ok: true}},
	}}
	locker := &fakeRegisterLocker{}
	access := NewLockedUARTRegisterAccess(
		"stepper_x",
		map[string]int64{"GCONF": 0x10, "IFCNT": 0x02},
		3,
		transport,
		NewFieldHelper(TMC2209Fields, TMC2208SignedFields, TMC2209FieldFormatters, nil),
		func() bool { return false },
		locker,
	)

	got, err := access.Get_register("GCONF")
	if err != nil {
		t.Fatalf("Get_register returned error: %v", err)
	}
	if got != 77 {
		t.Fatalf("expected readback 77, got %d", got)
	}
	if err := access.Set_register("GCONF", 12, nil); err != nil {
		t.Fatalf("Set_register returned error: %v", err)
	}
	if locker.lockCount != 2 || locker.unlockCount != 2 {
		t.Fatalf("expected one lock/unlock pair per access, got lock=%d unlock=%d", locker.lockCount, locker.unlockCount)
	}
	if len(transport.writes) != 1 || transport.writes[0].value != 12 {
		t.Fatalf("unexpected writes: %#v", transport.writes)
	}
}
