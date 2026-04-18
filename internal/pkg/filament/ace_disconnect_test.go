package filament

import (
	"testing"

	"goklipper/internal/pkg/queue"
)

type fakeACESerialDevice struct {
	fd         uintptr
	closeCalls int
}

func (self *fakeACESerialDevice) Close() {
	self.closeCalls++
}

func (self *fakeACESerialDevice) GetFd() uintptr {
	return self.fd
}

func TestAceCommunDisconnectIsIdempotent(t *testing.T) {
	device := &fakeACESerialDevice{fd: 42}
	commun := NewAceCommunication("/tmp/fake-ace", 115200)
	commun.dev = device
	commun.is_connected = true
	commun.queue = queue.NewQueue()
	commun.send_time = 12.5
	commun.read_buffer = []byte{1, 2, 3}
	commun.request_id = 7
	commun.callback_map[7] = func(map[string]interface{}) {}

	commun.Disconnect()

	if device.closeCalls != 1 {
		t.Fatalf("expected first disconnect to close device once, got %d", device.closeCalls)
	}
	if commun.dev != nil {
		t.Fatalf("expected device to be cleared after disconnect")
	}
	if commun.queue != nil {
		t.Fatalf("expected queue to be cleared after disconnect")
	}
	if commun.is_connected {
		t.Fatalf("expected disconnect to clear connection state")
	}
	if commun.send_time != 0 {
		t.Fatalf("expected send time reset, got %v", commun.send_time)
	}
	if commun.read_buffer != nil {
		t.Fatalf("expected read buffer reset")
	}
	if commun.request_id != 0 {
		t.Fatalf("expected request id reset, got %d", commun.request_id)
	}
	if len(commun.callback_map) != 0 {
		t.Fatalf("expected callback map reset, got %d entries", len(commun.callback_map))
	}

	commun.Disconnect()

	if device.closeCalls != 1 {
		t.Fatalf("expected repeated disconnect to avoid closing nil device again, got %d closes", device.closeCalls)
	}
	if got := commun.Fd(); got != -1 {
		t.Fatalf("expected nil device fd sentinel, got %d", got)
	}
	if !commun.Is_send_queue_empty() {
		t.Fatalf("expected nil queue to be treated as empty")
	}
	if len(commun.callback_map) != 0 {
		t.Fatalf("expected callback map to remain empty after repeated disconnect")
	}
}

func TestACEHandleDisconnectIsSafeAfterCommunicationAlreadyClosed(t *testing.T) {
	printer := newFakeACEPrinter()
	config := &fakeACEConfig{
		name:    "ace",
		printer: printer,
		values: map[string]interface{}{
			"serial": "/tmp/fake-ace",
		},
	}

	ace := LoadConfigACE(config).(*ACE)
	ace.ace_commun.dev = &fakeACESerialDevice{fd: 99}
	ace.ace_commun.queue = queue.NewQueue()
	ace.ace_commun.is_connected = true
	ace.ace_commun.Disconnect()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected disconnect handler to tolerate already closed communication, recovered %v", recovered)
		}
	}()

	if err := ace._handle_disconnect(nil); err != nil {
		t.Fatalf("expected nil disconnect handler error, got %v", err)
	}
}
