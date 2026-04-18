package webhooks

import (
	"reflect"
	"testing"
)

type fakeTypedRuntimeRequest struct {
	*fakeRuntimeRequest
	marker string
}

func TestEndpointRegistryListEndpointsIncludesRegisteredPaths(t *testing.T) {
	registry := NewEndpointRegistry()
	if err := registry.RegisterEndpoint("info", func(RuntimeRequest) (interface{}, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("RegisterEndpoint returned error: %v", err)
	}

	request := &fakeRuntimeRequest{params: map[string]interface{}{}, client: &fakeRuntimeClient{}}
	handler := registry.Handler("list_endpoints")
	if handler == nil {
		t.Fatal("expected list_endpoints handler")
	}
	if _, err := handler(request); err != nil {
		t.Fatalf("list_endpoints handler returned error: %v", err)
	}
	if len(request.sent) != 1 {
		t.Fatalf("expected one response, got %#v", request.sent)
	}

	response, ok := request.sent[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got %T", request.sent[0])
	}
	endpoints, ok := response["endpoints"].([]string)
	if !ok {
		t.Fatalf("expected []string endpoints, got %#v", response["endpoints"])
	}
	if !containsString(endpoints, "list_endpoints") || !containsString(endpoints, "info") {
		t.Fatalf("expected registered endpoints in %#v", endpoints)
	}
}

func TestEndpointRegistryRemoteMethodRegistrationAndDispatch(t *testing.T) {
	registry := NewEndpointRegistry()
	client := &fakeRuntimeClient{}
	request := &fakeRuntimeRequest{
		params: map[string]interface{}{
			"remote_method":     "notify_status",
			"response_template": map[string]interface{}{"method": "notify_status"},
		},
		client: client,
	}

	if _, err := registry.HandleRemoteMethodRegistration(request); err != nil {
		t.Fatalf("HandleRemoteMethodRegistration returned error: %v", err)
	}
	if err := registry.CallRemoteMethod("notify_status", map[string]interface{}{"ready": true}); err != nil {
		t.Fatalf("CallRemoteMethod returned error: %v", err)
	}

	want := []interface{}{
		map[string]interface{}{
			"method": "notify_status",
			"params": map[string]interface{}{"ready": true},
		},
	}
	if !reflect.DeepEqual(client.sent, want) {
		t.Fatalf("unexpected remote call payload: got %#v want %#v", client.sent, want)
	}
}

func TestEndpointRegistryCallRemoteMethodDropsClosedClients(t *testing.T) {
	registry := NewEndpointRegistry()
	client := &fakeRuntimeClient{closed: true}
	request := &fakeRuntimeRequest{
		params: map[string]interface{}{
			"remote_method":     "notify_status",
			"response_template": map[string]interface{}{"method": "notify_status"},
		},
		client: client,
	}

	if _, err := registry.HandleRemoteMethodRegistration(request); err != nil {
		t.Fatalf("HandleRemoteMethodRegistration returned error: %v", err)
	}
	if err := registry.CallRemoteMethod("notify_status", map[string]interface{}{"ready": true}); err == nil || err.Error() != "No active connections for method 'notify_status'" {
		t.Fatalf("expected no active connections error, got %v", err)
	}
	if _, ok := registry.remoteMethods["notify_status"]; ok {
		t.Fatalf("expected closed client registration to be pruned")
	}
}

func TestEndpointRegistryRegisterMuxEndpointDispatchesByKey(t *testing.T) {
	registry := NewEndpointRegistry()
	called := 0
	registry.RegisterMuxEndpoint("dump", "sensor", "probe", func(request RuntimeRequest) {
		called++
		if got := request.GetStr("sensor", ""); got != "probe" {
			t.Fatalf("expected mux request sensor probe, got %q", got)
		}
	})

	handler := registry.Handler("dump")
	if handler == nil {
		t.Fatal("expected mux handler")
	}
	request := &fakeRuntimeRequest{
		params: map[string]interface{}{"sensor": "probe"},
		client: &fakeRuntimeClient{},
	}
	if _, err := handler(request); err != nil {
		t.Fatalf("mux handler returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected mux callback once, got %d", called)
	}
}

func TestRegisterTypedEndpointCastsConcreteRequest(t *testing.T) {
	registry := NewEndpointRegistry()
	if err := RegisterTypedEndpoint[*fakeTypedRuntimeRequest](registry, "typed", func(request *fakeTypedRuntimeRequest) (interface{}, error) {
		return map[string]interface{}{"marker": request.marker}, nil
	}); err != nil {
		t.Fatalf("RegisterTypedEndpoint returned error: %v", err)
	}

	handler := registry.Handler("typed")
	if handler == nil {
		t.Fatal("expected typed handler")
	}
	response, err := handler(&fakeTypedRuntimeRequest{
		fakeRuntimeRequest: &fakeRuntimeRequest{params: map[string]interface{}{}, client: &fakeRuntimeClient{}},
		marker:             "typed-request",
	})
	if err != nil {
		t.Fatalf("typed handler returned error: %v", err)
	}
	if !reflect.DeepEqual(response, map[string]interface{}{"marker": "typed-request"}) {
		t.Fatalf("unexpected typed response: %#v", response)
	}
}

func TestRegisterTypedEndpointRejectsWrongRequestType(t *testing.T) {
	registry := NewEndpointRegistry()
	if err := RegisterTypedEndpoint[*fakeTypedRuntimeRequest](registry, "typed", func(request *fakeTypedRuntimeRequest) (interface{}, error) {
		return request.marker, nil
	}); err != nil {
		t.Fatalf("RegisterTypedEndpoint returned error: %v", err)
	}

	handler := registry.Handler("typed")
	if handler == nil {
		t.Fatal("expected typed handler")
	}
	if _, err := handler(&fakeRuntimeRequest{params: map[string]interface{}{}, client: &fakeRuntimeClient{}}); err == nil || err.Error() != "invalid webhook request type: *webhooks.fakeRuntimeRequest" {
		t.Fatalf("expected invalid type error, got %v", err)
	}
}

func TestRegisterTypedEndpointsRegistersAllBindings(t *testing.T) {
	registry := NewEndpointRegistry()
	called := []string{}
	err := RegisterTypedEndpoints[*fakeTypedRuntimeRequest](registry, []TypedEndpointBinding[*fakeTypedRuntimeRequest]{
		{
			Path: "typed/one",
			Handler: func(request *fakeTypedRuntimeRequest) (interface{}, error) {
				called = append(called, "one:"+request.marker)
				return nil, nil
			},
		},
		{
			Path: "typed/two",
			Handler: func(request *fakeTypedRuntimeRequest) (interface{}, error) {
				called = append(called, "two:"+request.marker)
				return nil, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("RegisterTypedEndpoints returned error: %v", err)
	}

	for _, path := range []string{"typed/one", "typed/two"} {
		handler := registry.Handler(path)
		if handler == nil {
			t.Fatalf("expected handler for %s", path)
		}
		if _, err := handler(&fakeTypedRuntimeRequest{
			fakeRuntimeRequest: &fakeRuntimeRequest{params: map[string]interface{}{}, client: &fakeRuntimeClient{}},
			marker:             "ok",
		}); err != nil {
			t.Fatalf("handler %s returned error: %v", path, err)
		}
	}

	if !reflect.DeepEqual(called, []string{"one:ok", "two:ok"}) {
		t.Fatalf("unexpected call order: %#v", called)
	}
}

func TestRegisterTypedMuxEndpointCastsConcreteRequest(t *testing.T) {
	registry := NewEndpointRegistry()
	called := ""
	RegisterTypedMuxEndpoint[*fakeTypedRuntimeRequest](registry, "typed_mux", "sensor", "probe", func(request *fakeTypedRuntimeRequest) {
		called = request.marker
	})

	handler := registry.Handler("typed_mux")
	if handler == nil {
		t.Fatal("expected typed mux handler")
	}
	if _, err := handler(&fakeTypedRuntimeRequest{
		fakeRuntimeRequest: &fakeRuntimeRequest{params: map[string]interface{}{"sensor": "probe"}, client: &fakeRuntimeClient{}},
		marker:             "mux-request",
	}); err != nil {
		t.Fatalf("typed mux handler returned error: %v", err)
	}
	if called != "mux-request" {
		t.Fatalf("unexpected typed mux marker: %q", called)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
