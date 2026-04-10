package report

import "testing"

type fakeAPIDumpReactor struct {
	now              float64
	unregistered     int
	registeredWake   float64
	registeredHandle interface{}
	callback         func(float64) float64
}

func (self *fakeAPIDumpReactor) Monotonic() float64 { return self.now }
func (self *fakeAPIDumpReactor) RegisterTimer(callback func(float64) float64, waketime float64) interface{} {
	self.callback = callback
	self.registeredWake = waketime
	self.registeredHandle = "timer"
	return self.registeredHandle
}
func (self *fakeAPIDumpReactor) UnregisterTimer(timer interface{}) {
	_ = timer
	self.unregistered++
}

type fakeAPIDumpClient struct {
	closed bool
	sent   []map[string]interface{}
}

func (self *fakeAPIDumpClient) IsClosed() bool { return self.closed }
func (self *fakeAPIDumpClient) Send(msg map[string]interface{}) {
	copyMsg := map[string]interface{}{}
	for key, value := range msg {
		copyMsg[key] = value
	}
	self.sent = append(self.sent, copyMsg)
}

func TestAPIDumpHelperAddClientAndUpdate(t *testing.T) {
	reactor := &fakeAPIDumpReactor{now: 10.0}
	helper := NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		return map[string]interface{}{"time": eventtime}
	}, nil, 0.5)
	client := &fakeAPIDumpClient{}
	helper.AddClient(client, map[string]interface{}{"header": map[string]interface{}{"kind": "sample"}})
	if reactor.registeredWake != 10.5 {
		t.Fatalf("unexpected wake time %v", reactor.registeredWake)
	}
	next := reactor.callback(11.0)
	if next != 11.5 {
		t.Fatalf("unexpected next wake %v", next)
	}
	if len(client.sent) != 1 {
		t.Fatalf("expected one client message, got %d", len(client.sent))
	}
	params := client.sent[0]["params"].(map[string]interface{})
	if params["time"].(float64) != 11.0 {
		t.Fatalf("unexpected params %#v", params)
	}
	if client.sent[0]["header"].(map[string]interface{})["kind"].(string) != "sample" {
		t.Fatalf("unexpected template merge %#v", client.sent[0])
	}
}

func TestAPIDumpHelperInternalClientCapturesMessages(t *testing.T) {
	reactor := &fakeAPIDumpReactor{now: 1.0}
	helper := NewAPIDumpHelper(reactor, func(eventtime float64) map[string]interface{} {
		_ = eventtime
		return map[string]interface{}{"data": map[string]interface{}{"value": 42}}
	}, nil, 0.1)
	client := helper.AddInternalClient()
	reactor.callback(1.2)
	msgs := client.Get_messages()
	if len(msgs) != 1 {
		t.Fatalf("expected one internal client message, got %d", len(msgs))
	}
	if msgs[0]["params"]["data"].(map[string]interface{})["value"].(int) != 42 {
		t.Fatalf("unexpected internal client message %#v", msgs[0])
	}
	client.Finalize()
	if !client.IsClosed() {
		t.Fatal("expected finalized client to be closed")
	}
}
