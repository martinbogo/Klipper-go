package webhooks

import (
	"encoding/json"
	"fmt"
	printerpkg "goklipper/internal/pkg/printer"
	"math"
	"reflect"
	"testing"
)

func TestSanitizeJSONValueReplacesNonFiniteFloats(t *testing.T) {
	payload := map[string]interface{}{
		"ok":  1.25,
		"bad": math.NaN(),
		"nested": []interface{}{
			math.Inf(1),
			map[string]interface{}{"inner": float32(math.Inf(-1))},
		},
	}

	sanitized := sanitizeJSONValue(payload)
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("marshal failed after sanitization: %v", err)
	}

	decoded := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["bad"] != nil {
		t.Fatalf("expected bad field to be null, got %#v", decoded["bad"])
	}
	nested, ok := decoded["nested"].([]interface{})
	if !ok || len(nested) != 2 {
		t.Fatalf("unexpected nested payload: %#v", decoded["nested"])
	}
	if nested[0] != nil {
		t.Fatalf("expected nested[0] to be null, got %#v", nested[0])
	}
	inner, ok := nested[1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested[1] map, got %#v", nested[1])
	}
	if inner["inner"] != nil {
		t.Fatalf("expected nested inner field to be null, got %#v", inner["inner"])
	}
}

func TestParseRequestExtractsEnvelopeAndParams(t *testing.T) {
	req, err := ParseRequest(`{"id":1,"method":"objects/query","params":{"name":"toolhead","count":2}}`)
	if err != nil {
		t.Fatalf("ParseRequest returned error: %v", err)
	}

	if req.ID != 1 {
		t.Fatalf("expected ID 1, got %v", req.ID)
	}
	if req.Method != "objects/query" {
		t.Fatalf("expected method objects/query, got %q", req.Method)
	}
	if got := req.Get_str("name", ""); got != "toolhead" {
		t.Fatalf("expected name toolhead, got %q", got)
	}
	if got := req.Get_int("count", 0); got != 2 {
		t.Fatalf("expected count 2, got %d", got)
	}
}

func TestParseRequestRejectsInvalidEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "missing numeric id", raw: `{"id":"bad","method":"x"}`, wantErr: "id is Not a number"},
		{name: "invalid params type", raw: `{"id":1,"method":"x","params":[]}`, wantErr: "Invalid request type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRequest(tt.raw)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("expected error %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRequestSendAndFinish(t *testing.T) {
	req := &Request{ID: 7, RequestParams: RequestParams{Method: "info", Params: map[string]interface{}{}}}
	if err := req.Send(map[string]interface{}{"ok": true}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if err := req.Send(map[string]interface{}{"ok": false}); err == nil || err.Error() != "Multiple calls to send not allowed" {
		t.Fatalf("expected duplicate send error, got %v", err)
	}

	got := req.Finish()
	want := map[string]interface{}{"id": float64(7), "result": map[string]interface{}{"ok": true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected finish payload: got %#v want %#v", got, want)
	}
	if req.Finish()["result"] == nil {
		t.Fatalf("expected result payload to be present")
	}

	errReq := &Request{ID: 3, RequestParams: RequestParams{Method: "info", Params: map[string]interface{}{}}}
	errReq.SetErrorResponse(map[string]interface{}{"message": "boom"})
	errPayload := errReq.Finish()
	if _, ok := errPayload["error"]; !ok {
		t.Fatalf("expected error payload, got %#v", errPayload)
	}

	empty := &Request{ID: 9, RequestParams: RequestParams{Method: "info", Params: map[string]interface{}{}}}
	finished := empty.Finish()
	if !reflect.DeepEqual(finished["result"], map[string]string{}) {
		t.Fatalf("expected default empty result, got %#v", finished["result"])
	}
}

func TestNewConnectedRequestWrapsClientAndAliases(t *testing.T) {
	client := &fakeRuntimeClient{}
	req, err := NewConnectedRequest(client, `{"id":9,"method":"info","params":{"name":"toolhead","count":3,"meta":{"state":"ready"}}}`)
	if err != nil {
		t.Fatalf("NewConnectedRequest returned error: %v", err)
	}
	if req.Connection() != client {
		t.Fatalf("expected original client connection")
	}
	if got := req.GetStr("name", ""); got != "toolhead" {
		t.Fatalf("expected alias GetStr to return toolhead, got %q", got)
	}
	if got := req.String("name", ""); got != "toolhead" {
		t.Fatalf("expected String helper to return toolhead, got %q", got)
	}
	if got := req.Int("count", 0); got != 3 {
		t.Fatalf("expected Int helper to return 3, got %d", got)
	}
	if got := req.GetDict("meta", nil); !reflect.DeepEqual(got, map[string]interface{}{"state": "ready"}) {
		t.Fatalf("unexpected GetDict payload: %#v", got)
	}

	req.SetError(fmt.Errorf("boom"))
	payload := req.Finish()
	errorPayload, ok := payload["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error payload, got %#v", payload)
	}
	if errorPayload["message"] != "boom" {
		t.Fatalf("expected boom message, got %#v", errorPayload)
	}
}

func TestNewConnectedEnvelopeKeepsConcreteClient(t *testing.T) {
	client := &ClientConnection{}
	envelope, err := NewConnectedEnvelope(client, `{"id":1,"method":"info","params":{"name":"toolhead"}}`)
	if err != nil {
		t.Fatalf("NewConnectedEnvelope returned error: %v", err)
	}
	request, ok := envelope.(*ConnectedRequest)
	if !ok {
		t.Fatalf("expected ConnectedRequest envelope, got %T", envelope)
	}
	if request.ClientConnection() != client {
		t.Fatalf("expected concrete client connection to be preserved")
	}
}

func TestNewPrinterRegistryRegistersWebhookHandlers(t *testing.T) {
	runtimeRegistry := &fakeRuntimeRegistry{}
	printerRegistry := NewPrinterRegistry(runtimeRegistry)
	if err := printerRegistry.RegisterEndpointWithRequest("printer/info", func(request printerpkg.WebhookRequest) (interface{}, error) {
		if request.String("name", "") != "toolhead" {
			t.Fatalf("unexpected webhook request payload")
		}
		return map[string]interface{}{"ok": true}, nil
	}); err != nil {
		t.Fatalf("RegisterEndpointWithRequest returned error: %v", err)
	}
	if err := printerRegistry.RegisterEndpoint("printer/ping", func() (interface{}, error) {
		return map[string]interface{}{"pong": true}, nil
	}); err != nil {
		t.Fatalf("RegisterEndpoint returned error: %v", err)
	}

	request, err := NewConnectedRequest(&fakeRuntimeClient{}, `{"id":2,"method":"printer/info","params":{"name":"toolhead"}}`)
	if err != nil {
		t.Fatalf("NewConnectedRequest returned error: %v", err)
	}
	response, err := runtimeRegistry.handlers["printer/info"](request)
	if err != nil {
		t.Fatalf("printer/info handler returned error: %v", err)
	}
	if !reflect.DeepEqual(response, map[string]interface{}{"ok": true}) {
		t.Fatalf("unexpected printer/info response: %#v", response)
	}

	pingRequest, err := NewConnectedRequest(&fakeRuntimeClient{}, `{"id":3,"method":"printer/ping","params":{}}`)
	if err != nil {
		t.Fatalf("NewConnectedRequest returned error: %v", err)
	}
	pingResponse, err := runtimeRegistry.handlers["printer/ping"](pingRequest)
	if err != nil {
		t.Fatalf("printer/ping handler returned error: %v", err)
	}
	if !reflect.DeepEqual(pingResponse, map[string]interface{}{"pong": true}) {
		t.Fatalf("unexpected printer/ping response: %#v", pingResponse)
	}
}

func TestReqItemsNormalizesSlices(t *testing.T) {
	fromInterfaces := ReqItems([]interface{}{"a", "b"})
	if !reflect.DeepEqual(fromInterfaces, []string{"a", "b"}) {
		t.Fatalf("unexpected interface normalization: %#v", fromInterfaces)
	}
	fromStrings := ReqItems([]string{"x", "y"})
	if !reflect.DeepEqual(fromStrings, []string{"x", "y"}) {
		t.Fatalf("unexpected string normalization: %#v", fromStrings)
	}
}

type fakeRuntimeRegistry struct {
	handlers map[string]func(RuntimeRequest) (interface{}, error)
}

func (r *fakeRuntimeRegistry) RegisterEndpoint(path string, handler func(RuntimeRequest) (interface{}, error)) error {
	if r.handlers == nil {
		r.handlers = map[string]func(RuntimeRequest) (interface{}, error){}
	}
	r.handlers[path] = handler
	return nil
}

type fakeRuntimeClient struct {
	closed bool
	sent   []interface{}
}

func (c *fakeRuntimeClient) IsClosed() bool {
	return c.closed
}

func (c *fakeRuntimeClient) Send(data interface{}) {
	c.sent = append(c.sent, data)
}

type fakeRuntimeRequest struct {
	params map[string]interface{}
	client RuntimeClient
	sent   []interface{}
}

func (r *fakeRuntimeRequest) Send(data interface{}) error {
	r.sent = append(r.sent, data)
	return nil
}

func (r *fakeRuntimeRequest) GetDict(item string, defaultValue interface{}) map[string]interface{} {
	if value, ok := r.params[item]; ok {
		if typed, ok := value.(map[string]interface{}); ok {
			return typed
		}
	}
	if typed, ok := defaultValue.(map[string]interface{}); ok {
		return typed
	}
	return nil
}

func (r *fakeRuntimeRequest) GetStr(item string, defaultValue interface{}) string {
	if value, ok := r.params[item]; ok {
		if typed, ok := value.(string); ok {
			return typed
		}
	}
	if typed, ok := defaultValue.(string); ok {
		return typed
	}
	return ""
}

func (r *fakeRuntimeRequest) Connection() RuntimeClient {
	return r.client
}

type fakeWebhookGCode struct {
	help           map[string]string
	scripts        []string
	panicValue     interface{}
	outputHandlers []func(string)
}

func (g *fakeWebhookGCode) GetCommandHelp() map[string]string {
	return g.help
}

func (g *fakeWebhookGCode) RunScript(script string) {
	g.scripts = append(g.scripts, script)
	if g.panicValue != nil {
		panic(g.panicValue)
	}
}

func (g *fakeWebhookGCode) RegisterOutputHandler(cb func(string)) {
	g.outputHandlers = append(g.outputHandlers, cb)
}

func TestRegisterGCodeEndpointsRoutesHelpAndOutput(t *testing.T) {
	registry := &fakeRuntimeRegistry{}
	gcode := &fakeWebhookGCode{help: map[string]string{"HELP": "show help"}}
	RegisterGCodeEndpoints(registry, gcode)

	helpRequest := &fakeRuntimeRequest{params: map[string]interface{}{}, client: &fakeRuntimeClient{}}
	if _, err := registry.handlers["gcode/help"](helpRequest); err != nil {
		t.Fatalf("help handler returned error: %v", err)
	}
	if !reflect.DeepEqual(helpRequest.sent, []interface{}{map[string]string{"HELP": "show help"}}) {
		t.Fatalf("unexpected help payload: %#v", helpRequest.sent)
	}

	client := &fakeRuntimeClient{}
	subscribeRequest := &fakeRuntimeRequest{
		params: map[string]interface{}{"response_template": map[string]interface{}{"method": "notify_gcode_response"}},
		client: client,
	}
	if _, err := registry.handlers["gcode/subscribe_output"](subscribeRequest); err != nil {
		t.Fatalf("subscribe handler returned error: %v", err)
	}
	if len(gcode.outputHandlers) != 1 {
		t.Fatalf("expected one output handler registration, got %d", len(gcode.outputHandlers))
	}
	gcode.outputHandlers[0]("ok")
	if len(client.sent) != 1 {
		t.Fatalf("expected one client notification, got %#v", client.sent)
	}
	want := map[string]interface{}{
		"method": "notify_gcode_response",
		"params": map[string]interface{}{"response": "ok"},
	}
	if !reflect.DeepEqual(client.sent[0], want) {
		t.Fatalf("unexpected client notification: got %#v want %#v", client.sent[0], want)
	}
	if len(gcode.scripts) != 0 {
		t.Fatalf("subscribe should not run scripts, got %#v", gcode.scripts)
	}
}

func TestRegisterGCodeEndpointsReturnsRecoveredScriptErrors(t *testing.T) {
	registry := &fakeRuntimeRegistry{}
	gcode := &fakeWebhookGCode{panicValue: fmt.Errorf("boom")}
	RegisterGCodeEndpoints(registry, gcode)

	request := &fakeRuntimeRequest{params: map[string]interface{}{"script": "STATUS"}, client: &fakeRuntimeClient{}}
	if _, err := registry.handlers["gcode/script"](request); err == nil || err.Error() != "boom" {
		t.Fatalf("expected recovered error boom, got %v", err)
	}
	if !reflect.DeepEqual(gcode.scripts, []string{"STATUS"}) {
		t.Fatalf("unexpected scripts %#v", gcode.scripts)
	}
}

type fakeQueryTimer struct {
	updates []float64
}

func (t *fakeQueryTimer) Update(waketime float64) {
	t.updates = append(t.updates, waketime)
}

type fakeQueryCompletion struct {
	reactor *fakeQueryReactor
	result  interface{}
	waits   []float64
}

func (c *fakeQueryCompletion) Complete(result interface{}) {
	c.result = result
}

func (c *fakeQueryCompletion) Wait(waketime float64, waketimeResult interface{}) interface{} {
	c.waits = append(c.waits, waketime)
	if c.result == nil && c.reactor.timerCallback != nil {
		c.reactor.timerCallback(c.reactor.monotonic)
	}
	if c.result == nil {
		return waketimeResult
	}
	return c.result
}

type fakeQueryReactor struct {
	monotonic     float64
	timerCallback func(float64) float64
	timerWake     float64
	completion    *fakeQueryCompletion
	timer         *fakeQueryTimer
}

func (r *fakeQueryReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	r.timerCallback = callback
	r.timerWake = waketime
	r.timer = &fakeQueryTimer{}
	return r.timer
}

func (r *fakeQueryReactor) Monotonic() float64 {
	return r.monotonic
}

func (r *fakeQueryReactor) Completion() interface{} {
	r.completion = &fakeQueryCompletion{reactor: r}
	return r.completion
}

type fakeStatusObject struct {
	status map[string]interface{}
	called []float64
}

func (o *fakeStatusObject) Get_status(eventtime float64) map[string]interface{} {
	o.called = append(o.called, eventtime)
	return o.status
}

type fakeQueryPrinter struct {
	lookup  map[string]interface{}
	objects []interface{}
	reactor printerpkg.ModuleReactor
}

func (p *fakeQueryPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	if value, ok := p.lookup[name]; ok {
		return value
	}
	return defaultValue
}

func (p *fakeQueryPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {}
func (p *fakeQueryPrinter) SendEvent(event string, params []interface{})                          {}
func (p *fakeQueryPrinter) CurrentExtruderName() string                                           { return "extruder" }
func (p *fakeQueryPrinter) AddObject(name string, obj interface{}) error                          { return nil }
func (p *fakeQueryPrinter) LookupObjects(module string) []interface{}                             { return p.objects }
func (p *fakeQueryPrinter) HasStartArg(name string) bool                                          { return false }
func (p *fakeQueryPrinter) LookupHeater(name string) printerpkg.HeaterRuntime                     { return nil }
func (p *fakeQueryPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry              { return nil }
func (p *fakeQueryPrinter) LookupMCU(name string) printerpkg.MCURuntime                           { return nil }
func (p *fakeQueryPrinter) InvokeShutdown(msg string)                                             {}
func (p *fakeQueryPrinter) IsShutdown() bool                                                      { return false }
func (p *fakeQueryPrinter) Reactor() printerpkg.ModuleReactor                                     { return p.reactor }
func (p *fakeQueryPrinter) StepperEnable() printerpkg.StepperEnableRuntime                        { return nil }
func (p *fakeQueryPrinter) GCode() printerpkg.GCodeRuntime                                        { return nil }
func (p *fakeQueryPrinter) GCodeMove() printerpkg.MoveTransformController                         { return nil }
func (p *fakeQueryPrinter) Webhooks() printerpkg.WebhookRegistry                                  { return nil }

func TestNewQueryStatusHelperHandlesObjectQuery(t *testing.T) {
	registry := &fakeRuntimeRegistry{}
	toolhead := &fakeStatusObject{status: map[string]interface{}{"state": "ready", "x": 12.5}}
	helper := NewQueryStatusHelper(&fakeQueryPrinter{
		lookup:  map[string]interface{}{"toolhead": toolhead},
		reactor: &fakeQueryReactor{monotonic: 10.0},
	}, registry, func() float64 { return 42.5 })
	if helper == nil {
		t.Fatal("expected helper to be created")
	}

	request := &fakeRuntimeRequest{
		params: map[string]interface{}{"objects": map[string]interface{}{"toolhead": nil}},
		client: &fakeRuntimeClient{},
	}
	if _, err := registry.handlers["objects/object_query"](request); err != nil {
		t.Fatalf("object_query handler returned error: %v", err)
	}
	if len(request.sent) != 1 {
		t.Fatalf("expected one object_query response, got %#v", request.sent)
	}
	want := map[string]interface{}{
		"eventtime": 42.5,
		"status": map[string]interface{}{
			"toolhead": map[string]interface{}{"state": "ready", "x": 12.5},
		},
	}
	if !reflect.DeepEqual(request.sent[0], want) {
		t.Fatalf("unexpected object_query response: got %#v want %#v", request.sent[0], want)
	}
	if !reflect.DeepEqual(toolhead.called, []float64{42.5}) {
		t.Fatalf("unexpected object_query eventtimes %#v", toolhead.called)
	}
}

func TestNewQueryStatusHelperHandlesQuery(t *testing.T) {
	registry := &fakeRuntimeRegistry{}
	toolhead := &fakeStatusObject{status: map[string]interface{}{"state": "ready", "x": 12.5}}
	reactor := &fakeQueryReactor{monotonic: 10.0}
	printer := &fakeQueryPrinter{
		lookup:  map[string]interface{}{"toolhead": toolhead},
		reactor: reactor,
	}
	NewQueryStatusHelper(printer, registry, nil)

	request := &fakeRuntimeRequest{
		params: map[string]interface{}{"objects": map[string]interface{}{"toolhead": []interface{}{"state"}}},
		client: &fakeRuntimeClient{},
	}
	if _, err := registry.handlers["objects/query"](request); err != nil {
		t.Fatalf("query handler returned error: %v", err)
	}
	if len(request.sent) != 1 {
		t.Fatalf("expected one query response, got %#v", request.sent)
	}
	want := map[string]interface{}{
		"eventtime": 10.0,
		"status": map[string]interface{}{
			"toolhead": map[string]interface{}{"state": "ready"},
		},
	}
	if !reflect.DeepEqual(request.sent[0], want) {
		t.Fatalf("unexpected query response: got %#v want %#v", request.sent[0], want)
	}
	if reactor.timerWake != 0 {
		t.Fatalf("expected timer wake NOW, got %v", reactor.timerWake)
	}
	if !reflect.DeepEqual(toolhead.called, []float64{10.0}) {
		t.Fatalf("unexpected query eventtimes %#v", toolhead.called)
	}
	if reactor.completion == nil || len(reactor.completion.waits) != 1 {
		t.Fatalf("expected query wait to occur, got %#v", reactor.completion)
	}
}
