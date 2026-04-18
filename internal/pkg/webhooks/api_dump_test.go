package webhooks

import (
	"reflect"
	"testing"
)

func TestNewAPIDumpClientDelegatesToRuntimeClient(t *testing.T) {
	runtimeClient := &fakeRuntimeClient{}
	client := NewAPIDumpClient(runtimeClient)

	if client.IsClosed() {
		t.Fatalf("expected open client")
	}

	payload := map[string]interface{}{"params": map[string]interface{}{"x": 1.25}}
	client.Send(payload)
	if !reflect.DeepEqual(runtimeClient.sent, []interface{}{payload}) {
		t.Fatalf("unexpected forwarded payloads %#v", runtimeClient.sent)
	}

	runtimeClient.closed = true
	if !client.IsClosed() {
		t.Fatalf("expected closed client")
	}
}
