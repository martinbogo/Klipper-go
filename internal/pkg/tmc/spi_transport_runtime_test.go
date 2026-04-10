package tmc

import "testing"

type fakeSPIBusTransport struct {
	sends                 [][]int
	transfers             [][]int
	transferResponses     []string
	prefaceTransfers      []fakeSPIPrefaceCall
	prefaceResponses      []string
	printTimeToClockCalls []float64
	clockResult           int64
}

type fakeSPIPrefaceCall struct {
	preface  []int
	data     []int
	minclock int64
	reqclock int64
}

func (self *fakeSPIBusTransport) Send(data []int, minclock, reqclock int64) {
	_ = minclock
	_ = reqclock
	self.sends = append(self.sends, append([]int(nil), data...))
}

func (self *fakeSPIBusTransport) Transfer(data []int, minclock, reqclock int64) string {
	_ = minclock
	_ = reqclock
	self.transfers = append(self.transfers, append([]int(nil), data...))
	if len(self.transferResponses) == 0 {
		return ""
	}
	response := self.transferResponses[0]
	self.transferResponses = self.transferResponses[1:]
	return response
}

func (self *fakeSPIBusTransport) TransferWithPreface(prefaceData, data []int, minclock, reqclock int64) string {
	self.prefaceTransfers = append(self.prefaceTransfers, fakeSPIPrefaceCall{
		preface:  append([]int(nil), prefaceData...),
		data:     append([]int(nil), data...),
		minclock: minclock,
		reqclock: reqclock,
	})
	if len(self.prefaceResponses) == 0 {
		return ""
	}
	response := self.prefaceResponses[0]
	self.prefaceResponses = self.prefaceResponses[1:]
	return response
}

func (self *fakeSPIBusTransport) PrintTimeToClock(printTime float64) int64 {
	self.printTimeToClockCalls = append(self.printTimeToClockCalls, printTime)
	return self.clockResult
}

func TestSPIChainRuntimeRegisterPositionRejectsDuplicates(t *testing.T) {
	runtime := NewSPIChainRuntime(3, &fakeSPIBusTransport{}, nil)
	if err := runtime.RegisterPosition(2); err != nil {
		t.Fatalf("RegisterPosition returned error: %v", err)
	}
	if err := runtime.RegisterPosition(2); err == nil {
		t.Fatal("expected duplicate position error")
	}
}

func TestSPIChainRuntimeReadRegisterUsesChainCodec(t *testing.T) {
	transport := &fakeSPIBusTransport{transferResponses: []string{"\x00\x00\x00\x00\x00\x00\x11\x22\x33\x44\x00\x00\x00\x00\x00"}}
	runtime := NewSPIChainRuntime(3, transport, func() bool { return false })

	got := runtime.ReadRegister(0x6, 2)
	if got != 0x11223344 {
		t.Fatalf("expected 0x11223344, got %#x", got)
	}
	if len(transport.sends) != 1 || len(transport.transfers) != 1 {
		t.Fatalf("expected one send and one transfer, got sends=%d transfers=%d", len(transport.sends), len(transport.transfers))
	}
}

func TestSPIRegisterTransportRuntimeRetriesUntilEchoMatches(t *testing.T) {
	transport := &fakeSPIBusTransport{prefaceResponses: []string{
		"\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00",
		"\x00\x00\x00\x00\x2a\x00\x00\x00\x00\x00",
	}}
	chain := NewSPIChainRuntime(2, transport, func() bool { return false })
	runtime := NewSPIRegisterTransportRuntime("stepper_x", map[string]int64{"GCONF": 0x00}, 2, chain)

	if err := runtime.SetRegister("GCONF", 0x2a, nil); err != nil {
		t.Fatalf("SetRegister returned error: %v", err)
	}
	if len(transport.prefaceTransfers) != 2 {
		t.Fatalf("expected 2 write attempts, got %d", len(transport.prefaceTransfers))
	}
}

func TestTMC2660SPITransportRuntimeChangesRdselOnlyWhenNeeded(t *testing.T) {
	transport := &fakeSPIBusTransport{transferResponses: []string{"\x01\x02\x03", "\x04\x05\x06"}}
	fields := NewFieldHelper(map[string]map[string]int64{"DRVCONF": {"rdsel": 0x03 << 4}}, nil, nil, nil)
	runtime := NewTMC2660SPITransportRuntime(map[string]int64{"DRVCONF": 0xE}, fields, transport, func() bool { return false })

	got, err := runtime.GetRegister("READRSP@RDSEL1")
	if err != nil {
		t.Fatalf("GetRegister returned error: %v", err)
	}
	if got != 0x010203 {
		t.Fatalf("expected 0x010203, got %#x", got)
	}
	if len(transport.sends) != 1 {
		t.Fatalf("expected rdsel pre-write send, got %d", len(transport.sends))
	}
	if fields.Get_field("rdsel", nil, nil) != 1 {
		t.Fatalf("expected rdsel to be updated to 1, got %d", fields.Get_field("rdsel", nil, nil))
	}

	got, err = runtime.GetRegister("READRSP@RDSEL1")
	if err != nil {
		t.Fatalf("second GetRegister returned error: %v", err)
	}
	if got != 0x040506 {
		t.Fatalf("expected 0x040506, got %#x", got)
	}
	if len(transport.sends) != 2 {
		t.Fatalf("expected repeated rdsel pre-write behavior to be preserved, got %d sends", len(transport.sends))
	}
}