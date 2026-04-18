package serialhdl

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tarm/serial"

	"goklipper/internal/pkg/msgproto"
)

type fakeSerialCompletion struct {
	result interface{}
}

type fakeTickingCompletion struct {
	reactor *fakeSerialReactor
	result  interface{}
}

func (self *fakeSerialCompletion) Wait(waketime float64, waketimeResult interface{}) interface{} {
	_ = waketime
	_ = waketimeResult
	return self.result
}

func (self *fakeSerialCompletion) Complete(result interface{}) {
	self.result = result
}

func (self *fakeSerialCompletion) CreatedAtUnix() int64 {
	return 0
}

func (self *fakeTickingCompletion) Wait(waketime float64, waketimeResult interface{}) interface{} {
	self.reactor.now = waketime
	if self.result == nil {
		return waketimeResult
	}
	return self.result
}

func (self *fakeTickingCompletion) Complete(result interface{}) {
	self.result = result
}

func (self *fakeTickingCompletion) CreatedAtUnix() int64 {
	return 0
}

type fakeSerialReactor struct {
	now float64
}

type fakeSerialPort struct {
	calls *[]string
}

func (self *fakeSerialPort) Read(p []byte) (int, error) {
	_ = p
	if self.calls != nil {
		*self.calls = append(*self.calls, "read")
	}
	return 0, nil
}

func (self *fakeSerialPort) Write(p []byte) (int, error) {
	if self.calls != nil {
		*self.calls = append(*self.calls, "write")
	}
	return len(p), nil
}

func (self *fakeSerialPort) Close() error {
	if self.calls != nil {
		*self.calls = append(*self.calls, "close")
	}
	return nil
}

func (self *fakeSerialPort) Flush() error {
	if self.calls != nil {
		*self.calls = append(*self.calls, "flush")
	}
	return nil
}

func (self *fakeSerialReactor) Monotonic() float64 {
	return self.now
}

func (self *fakeSerialReactor) Pause(waketime float64) float64 {
	self.now = waketime
	return waketime
}

func (self *fakeSerialReactor) RegisterTimer(callback func(float64) float64, waketime float64) Timer {
	_ = callback
	_ = waketime
	return nil
}

func (self *fakeSerialReactor) UpdateTimer(timer Timer, waketime float64) {
	_ = timer
	_ = waketime
}

func (self *fakeSerialReactor) RegisterCallback(callback func(interface{}) interface{}, waketime float64) Completion {
	_ = callback
	_ = waketime
	return self.NewCompletion()
}

func (self *fakeSerialReactor) NewCompletion() Completion {
	return &fakeSerialCompletion{}
}

func (self *fakeSerialReactor) AsyncComplete(completion Completion, result map[string]interface{}) {
	completion.Complete(result)
}

func (self *fakeSerialReactor) IsRunning() bool {
	return true
}

func TestIdentifySessionTimeoutMatchesOriginalTransportBehavior(t *testing.T) {
	if identifySessionTimeout != 5*time.Second {
		t.Fatalf("identify session timeout = %v, want 5s", identifySessionTimeout)
	}
	if serialAckTimeoutSafety != 30.0 {
		t.Fatalf("serial ack timeout safety = %v, want 30.0", serialAckTimeoutSafety)
	}
}

func TestPrepareUARTSerialPortRunsUpstreamPrimerBeforeIdentify(t *testing.T) {
	reactor := &fakeSerialReactor{}
	reader := &SerialReader{reactor: reactor, warn_prefix: "test: "}
	calls := []string{}
	serialDev := NewSerialDev(&fakeSerialPort{calls: &calls}, nil, nil)

	oldConfigure := configureUARTPortRTSFunc
	oldLeave := stk500v2LeaveFunc
	defer func() {
		configureUARTPortRTSFunc = oldConfigure
		stk500v2LeaveFunc = oldLeave
	}()

	configureUARTPortRTSFunc = func(file *os.File, enabled bool) error {
		if file != nil {
			t.Fatalf("configureUARTPortRTSFunc file = %#v, want nil in test", file)
		}
		calls = append(calls, "rts=true")
		if !enabled {
			t.Fatalf("configureUARTPortRTSFunc enabled = %t, want true", enabled)
		}
		return nil
	}
	stk500v2LeaveFunc = func(got *SerialDev) {
		if got != serialDev {
			t.Fatalf("stk500v2LeaveFunc serial dev = %#v, want %#v", got, serialDev)
		}
		calls = append(calls, "stk500v2_leave")
	}

	reader.prepareUARTSerialPort(serialDev, true)

	want := []string{"rts=true", "stk500v2_leave", "flush"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for i, wantCall := range want {
		if calls[i] != wantCall {
			t.Fatalf("calls[%d] = %q, want %q (all calls: %#v)", i, calls[i], wantCall, calls)
		}
	}
}

func TestStk500v2LeaveUsesLivePortBaudReconfiguration(t *testing.T) {
	calls := []string{}
	tempFile, err := os.CreateTemp("", "serialhdl-stk500v2")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	serialDev := NewSerialDev(&fakeSerialPort{calls: &calls}, &serial.Config{Name: "/dev/ttyS5", Baud: 576000}, tempFile)

	oldClear := clearHUPCLFunc
	oldReconfigure := reconfigureSerialPortBaudFunc
	defer func() {
		clearHUPCLFunc = oldClear
		reconfigureSerialPortBaudFunc = oldReconfigure
	}()

	clearHUPCLFunc = func(fd uintptr) {
		if fd != tempFile.Fd() {
			t.Fatalf("clearHUPCLFunc fd = %d, want %d", fd, tempFile.Fd())
		}
		calls = append(calls, "clear_hupcl")
	}
	reconfigureSerialPortBaudFunc = func(file *os.File, name string, baud int) error {
		if file != tempFile {
			t.Fatalf("reconfigureSerialPortBaudFunc file = %#v, want %#v", file, tempFile)
		}
		if name != "/dev/ttyS5" {
			t.Fatalf("reconfigureSerialPortBaudFunc name = %q, want /dev/ttyS5", name)
		}
		calls = append(calls, fmt.Sprintf("baud=%d", baud))
		return nil
	}

	Stk500v2Leave(serialDev)

	want := []string{
		"clear_hupcl",
		"baud=2400",
		"read",
		"baud=115200",
		"read",
		"write",
		"read",
		"baud=576000",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for i, wantCall := range want {
		if calls[i] != wantCall {
			t.Fatalf("calls[%d] = %q, want %q (all calls: %#v)", i, calls[i], wantCall, calls)
		}
	}
	if serialDev.Config.Baud != 576000 {
		t.Fatalf("serialDev.Config.Baud = %d, want 576000", serialDev.Config.Baud)
	}
}

func TestUnknownHandlersReturnNil(t *testing.T) {
	reader := &SerialReader{warn_prefix: "test: ", identifyUnknownCounts: map[int]uint32{}}
	params := map[string]interface{}{"#msgid": int64(77), "#msg": []int{1, 2, 3}, "#name": "#output"}
	if err := reader._handle_unknown_init(params); err != nil {
		t.Fatalf("_handle_unknown_init returned error: %v", err)
	}
	if err := reader.Handle_unknown(params); err != nil {
		t.Fatalf("Handle_unknown returned error: %v", err)
	}
}

func TestOwnedResponseUnregisterKeepsNewerHandler(t *testing.T) {
	reader := &SerialReader{
		lock:     &sync.Mutex{},
		handlers: map[string]interface{}{},
	}

	firstID := reader.Register_owned_response(func(map[string]interface{}) error { return nil }, "identify_response", nil)
	secondID := reader.Register_owned_response(func(map[string]interface{}) error { return nil }, "identify_response", nil)

	reader.Unregister_owned_response("identify_response", nil, firstID)
	handler, ok := reader.handlers["identify_response"]
	if !ok {
		t.Fatalf("newer owned handler was removed by stale unregister")
	}
	owned, ok := handler.(ownedResponseHandler)
	if !ok {
		t.Fatalf("stored handler type = %T, want ownedResponseHandler", handler)
	}
	if owned.id != secondID {
		t.Fatalf("remaining handler id = %d, want %d", owned.id, secondID)
	}

	reader.Unregister_owned_response("identify_response", nil, secondID)
	if _, ok := reader.handlers["identify_response"]; ok {
		t.Fatalf("owned handler still registered after matching unregister")
	}
	if firstID == secondID {
		t.Fatalf("owned handler ids must be unique")
	}
}

func TestTrackIdentifyUnknownLimitsLoggingPerMessage(t *testing.T) {
	reader := &SerialReader{identifyUnknownCounts: map[int]uint32{}}

	for i := 1; i <= maxIdentifyUnknownLogs+5; i++ {
		count, shouldLog := reader.trackIdentifyUnknown(77)
		if count != uint32(i) {
			t.Fatalf("count = %d, want %d", count, i)
		}
		wantLog := i <= maxIdentifyUnknownLogs
		if shouldLog != wantLog {
			t.Fatalf("iteration %d shouldLog = %t, want %t", i, shouldLog, wantLog)
		}
	}

	count, shouldLog := reader.trackIdentifyUnknown(70)
	if count != 1 {
		t.Fatalf("second message count = %d, want 1", count)
	}
	if !shouldLog {
		t.Fatalf("second message id should log on first occurrence")
	}
}

func TestRetryAsyncCommandReturnsLateResponseAfterAckTimeout(t *testing.T) {
	reactor := &fakeSerialReactor{now: 10.0}
	reader := &SerialReader{
		reactor:  reactor,
		lock:     &sync.Mutex{},
		handlers: map[string]interface{}{},
	}
	retry := NewRetryAsyncCommand(reader, "tmcuart_response", 0)
	want := map[string]interface{}{
		"#sent_time": 10.0,
		"read":       []int64{1, 2, 3},
	}
	sendWaitAckCalls := 0
	sendRawCalls := 0
	retry.sendWaitAck = func(cmd []int, minclock, reqclock int64, cmd_queue interface{}) map[string]interface{} {
		_ = cmd
		_ = minclock
		_ = reqclock
		_ = cmd_queue
		sendWaitAckCalls++
		if sendWaitAckCalls != 1 {
			t.Fatalf("sendWaitAck called %d times, want 1", sendWaitAckCalls)
		}
		if err := retry.Handle_callback(want); err != nil {
			t.Fatalf("Handle_callback returned error: %v", err)
		}
		return nil
	}
	retry.sendRaw = func(cmd []int, minclock, reqclock int64, cmd_queue interface{}) {
		_ = cmd
		_ = minclock
		_ = reqclock
		_ = cmd_queue
		sendRawCalls++
	}

	got, err := retry.get_response([]interface{}{[]int{1, 2, 3}}, nil, 0, 0)
	if err != nil {
		t.Fatalf("get_response returned error: %v", err)
	}
	if got["read"].([]int64)[0] != 1 {
		t.Fatalf("unexpected read payload: %#v", got["read"])
	}
	if sendRawCalls != 0 {
		t.Fatalf("sendRaw called %d times, want 0", sendRawCalls)
	}
	if _, ok := reader.handlers["tmcuart_response0"]; ok {
		t.Fatalf("owned handler still registered after returning late response")
	}
}

func TestSerialRetryCommandReturnsResponseWithoutAckNotify(t *testing.T) {
	reader := &SerialReader{
		reactor:  &fakeSerialReactor{now: 10.0},
		lock:     &sync.Mutex{},
		handlers: map[string]interface{}{},
	}
	retry := NewSerialRetryCommand(reader, "identify_response", nil)
	ackCalls := 0
	retry.sendWaitAck = func(cmd []int, minclock, reqclock int64, cmd_queue interface{}, shouldStop func() bool) map[string]interface{} {
		_ = cmd
		_ = minclock
		_ = reqclock
		_ = cmd_queue
		ackCalls++
		if err := retry.handle_callback(map[string]interface{}{"offset": int64(40), "data": []int{1, 2, 3}}); err != nil {
			t.Fatalf("handle_callback returned error: %v", err)
		}
		if !shouldStop() {
			t.Fatalf("shouldStop() = false, want true after response callback")
		}
		return map[string]interface{}{}
	}

	params, err := retry.get_response([]interface{}{[]int{1, 2, 3}}, nil, 0, 0)
	if err != nil {
		t.Fatalf("get_response returned error: %v", err)
	}
	if ackCalls != 1 {
		t.Fatalf("sendWaitAck calls = %d, want 1", ackCalls)
	}
	if params["offset"].(int64) != 40 {
		t.Fatalf("offset = %v, want 40", params["offset"])
	}
	if _, ok := reader.handlers["identify_response"]; ok {
		t.Fatalf("owned handler still registered after response return")
	}
}

func TestRetryAsyncCommandNoRetryTimeoutMatchesPythonSemantics(t *testing.T) {
	reactor := &fakeSerialReactor{now: 10.0}
	reader := &SerialReader{
		reactor:  reactor,
		lock:     &sync.Mutex{},
		handlers: map[string]interface{}{},
	}
	retry := NewRetryAsyncCommand(reader, "tmcuart_response", 0)
	retry.Completion = &fakeTickingCompletion{reactor: reactor}
	ackCalls := 0
	sendRawCalls := 0
	retry.sendWaitAck = func(cmd []int, minclock, reqclock int64, cmd_queue interface{}) map[string]interface{} {
		_ = cmd
		_ = minclock
		_ = reqclock
		_ = cmd_queue
		ackCalls++
		return map[string]interface{}{"#sent_time": 10.0}
	}
	retry.sendRaw = func(cmd []int, minclock, reqclock int64, cmd_queue interface{}) {
		_ = cmd
		_ = minclock
		_ = reqclock
		_ = cmd_queue
		sendRawCalls++
	}

	defer func() {
		reason := recover()
		if reason == nil {
			t.Fatalf("expected timeout panic when no response arrives")
		}
		if got := fmt.Sprint(reason); got != "Timeout on wait for 'tmcuart_response' response '0'" {
			t.Fatalf("unexpected panic: %v", reason)
		}
		if ackCalls != 1 {
			t.Fatalf("sendWaitAck calls = %d, want 1", ackCalls)
		}
		if sendRawCalls == 0 {
			t.Fatalf("expected at least one raw retry before timeout")
		}
		if _, ok := reader.handlers["tmcuart_response0"]; ok {
			t.Fatalf("owned handler still registered after timeout")
		}
	}()

	_, _ = retry.get_response([]interface{}{[]int{1, 2, 3}}, nil, 0, 0)
}

func TestCommandQueryWrapperPropagatesTransportError(t *testing.T) {
	reader := &SerialReader{
		lock:      &sync.Mutex{},
		handlers:  map[string]interface{}{},
		Msgparser: msgproto.NewMessageParser("test: "),
	}
	query := NewCommandQueryWrapper(
		reader,
		"identify offset=%u count=%c",
		"identify_response offset=%u data=%.*s",
		0,
		nil,
		false,
		func(message string) interface{} { return fmt.Sprintf("wrapped: %s", message) },
	)

	retryErr := "transport blew up"
	oldNewSerialRetryCommand := newSerialRetryCommandFunc
	defer func() { newSerialRetryCommandFunc = oldNewSerialRetryCommand }()
	newSerialRetryCommandFunc = func(serial *SerialReader, name string, oid interface{}) RetryCommand {
		_ = serial
		_ = name
		_ = oid
		return retryCommandFunc(func(cmds []interface{}, cmd_queue interface{}, minclock, reqclock int64) (map[string]interface{}, error) {
			_ = cmds
			_ = cmd_queue
			_ = minclock
			_ = reqclock
			return nil, errors.New(retryErr)
		})
	}

	defer func() {
		reason := recover()
		if reason == nil {
			t.Fatalf("expected wrapped transport error panic")
		}
		if got := fmt.Sprint(reason); got != "wrapped: "+retryErr {
			t.Fatalf("unexpected panic: %v", reason)
		}
	}()

	query.Do_send([][]int{{1, 2, 3}}, 0, 0)
}

func TestDispatchParsedParamsIgnoresMissingNamedHandler(t *testing.T) {
	reader := &SerialReader{
		lock:     &sync.Mutex{},
		handlers: map[string]interface{}{},
	}
	params := map[string]interface{}{
		"#name": "analog_in_state",
		"oid":   int64(15),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dispatchParsedParams panicked: %v", r)
		}
	}()
	reader.dispatchParsedParams(params)
}
