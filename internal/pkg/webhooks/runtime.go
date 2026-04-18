package webhooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"goklipper/common/constants"
	kerror "goklipper/common/errors"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	"goklipper/common/utils/reflects"
	"goklipper/common/utils/sys"
	printerpkg "goklipper/internal/pkg/printer"
	"reflect"
	"runtime/debug"
)

// Request is a reusable JSON webhook request envelope with typed parameter
// accessors and response lifecycle state.
type Request struct {
	ID float64
	RequestParams

	Response interface{}
	IsError  bool
}

func (r *Request) MethodName() string {
	return r.Method
}

// ParseRequest decodes a raw webhook request string into a reusable request
// envelope.
func ParseRequest(raw string) (*Request, error) {
	baseRequest := make(map[string]interface{})
	if err := json.Unmarshal([]byte(raw), &baseRequest); err != nil {
		return nil, err
	}

	id, ok := baseRequest["id"].(float64)
	if !ok {
		return nil, errors.New("id is Not a number")
	}
	method, ok := baseRequest["method"].(string)
	if !ok {
		return nil, errors.New("Invalid request type")
	}

	params := make(map[string]interface{})
	if rawParams, ok := baseRequest["params"]; ok {
		parsedParams, ok := rawParams.(map[string]interface{})
		if !ok {
			return nil, errors.New("Invalid request type")
		}
		params = parsedParams
	}

	return &Request{
		ID: id,
		RequestParams: RequestParams{
			Method: method,
			Params: params,
		},
	}, nil
}

// Send stores the successful response payload for the request.
func (r *Request) Send(data interface{}) error {
	if r.Response != nil {
		return errors.New("Multiple calls to send not allowed")
	}
	r.Response = data
	return nil
}

// SetErrorResponse records an error response payload on the request envelope.
func (r *Request) SetErrorResponse(response interface{}) {
	r.IsError = true
	r.Response = response
}

// Finish builds the final JSON-RPC style response payload.
func (r *Request) Finish() map[string]interface{} {
	if r.ID == 0 {
		return nil
	}
	rtype := "result"
	if r.IsError {
		rtype = "error"
	}
	if r.Response == nil {
		r.Response = map[string]string{}
	}
	return map[string]interface{}{"id": r.ID, rtype: r.Response}
}

type RuntimeClient interface {
	IsClosed() bool
	Send(data interface{})
}

// ConnectedRequest binds a parsed webhook request to the runtime client
// connection that issued it.
type ConnectedRequest struct {
	client RuntimeClient
	*Request
}

var _ RequestEnvelope = (*ConnectedRequest)(nil)

func NewConnectedEnvelope(client *ClientConnection, raw string) (RequestEnvelope, error) {
	return NewConnectedRequest(client, raw)
}

func NewConnectedRequest(client RuntimeClient, raw string) (*ConnectedRequest, error) {
	request, err := ParseRequest(raw)
	if err != nil {
		return nil, err
	}
	return WrapRequest(client, request), nil
}

func WrapRequest(client RuntimeClient, request *Request) *ConnectedRequest {
	return &ConnectedRequest{
		client:  client,
		Request: request,
	}
}

func (r *ConnectedRequest) Connection() RuntimeClient {
	return r.client
}

func (r *ConnectedRequest) ClientConnection() *ClientConnection {
	client, _ := r.client.(*ClientConnection)
	return client
}

func (r *ConnectedRequest) SetError(err error) {
	kerr := kerror.FromError(err)
	r.SetErrorResponse(map[string]interface{}{
		"error":   kerr.Typ,
		"code":    kerr.Code,
		"message": kerr.Message,
	})
}

type RuntimeRequest interface {
	Send(data interface{}) error
	GetDict(item string, defaultValue interface{}) map[string]interface{}
	GetStr(item string, defaultValue interface{}) string
	Connection() RuntimeClient
}

type RuntimeRegistry interface {
	RegisterEndpoint(path string, handler func(RuntimeRequest) (interface{}, error)) error
}

type printerRuntimeRequest interface {
	RuntimeRequest
	printerpkg.WebhookRequest
}

type printerWebhookRegistry struct {
	runtime RuntimeRegistry
}

var _ printerpkg.WebhookRegistry = (*printerWebhookRegistry)(nil)

func NewPrinterRegistry(runtime RuntimeRegistry) printerpkg.WebhookRegistry {
	return &printerWebhookRegistry{runtime: runtime}
}

func (r *printerWebhookRegistry) RegisterEndpoint(path string, handler func() (interface{}, error)) error {
	return RegisterTypedEndpoint[*ConnectedRequest](r.runtime, path, func(_ *ConnectedRequest) (interface{}, error) {
		return handler()
	})
}

func (r *printerWebhookRegistry) RegisterEndpointWithRequest(path string, handler func(printerpkg.WebhookRequest) (interface{}, error)) error {
	return RegisterTypedEndpoint[*ConnectedRequest](r.runtime, path, func(request *ConnectedRequest) (interface{}, error) {
		var typed printerRuntimeRequest = request
		return handler(typed)
	})
}

type gcodeRuntime interface {
	GetCommandHelp() map[string]string
	RunScript(script string)
	RegisterOutputHandler(cb func(string))
}

type GCodeHelper struct {
	gcode gcodeRuntime

	isOutputRegistered bool
	clients            map[RuntimeClient]interface{}
}

func RegisterGCodeEndpoints(registry RuntimeRegistry, gcode gcodeRuntime) *GCodeHelper {
	helper := &GCodeHelper{
		gcode:              gcode,
		isOutputRegistered: false,
		clients:            map[RuntimeClient]interface{}{},
	}
	_ = registry.RegisterEndpoint("gcode/help", helper.handleHelp)
	_ = registry.RegisterEndpoint("gcode/script", helper.handleScript)
	_ = registry.RegisterEndpoint("gcode/restart", helper.handleRestart)
	_ = registry.RegisterEndpoint("gcode/firmware_restart", helper.handleFirmwareRestart)
	_ = registry.RegisterEndpoint("gcode/subscribe_output", helper.handleSubscribeOutput)
	return helper
}

func (h *GCodeHelper) handleHelp(request RuntimeRequest) (interface{}, error) {
	request.Send(h.gcode.GetCommandHelp())
	return nil, nil
}

func (h *GCodeHelper) handleScript(request RuntimeRequest) (_ interface{}, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if msg, ok := recovered.(string); ok && msg == "exit" {
				panic(recovered)
			}
			logger.Error("panic:", sys.GetGID(), recovered, string(debug.Stack()))
			if typed, ok := recovered.(error); ok {
				err = typed
				return
			}
			err = fmt.Errorf("%v", recovered)
		}
	}()
	script := request.GetStr("script", object.Sentinel{})
	logger.Debug("web hook do script:", script)
	h.gcode.RunScript(script)
	return nil, nil
}

func (h *GCodeHelper) handleRestart(request RuntimeRequest) (interface{}, error) {
	h.gcode.RunScript("restart")
	return nil, nil
}

func (h *GCodeHelper) handleFirmwareRestart(request RuntimeRequest) (interface{}, error) {
	h.gcode.RunScript("firmware_restart")
	return nil, nil
}

func (h *GCodeHelper) outputCallback(msg string) {
	for conn, template := range h.clients {
		if conn.IsClosed() {
			h.clients[conn] = nil
			continue
		}
		tmp := template.(map[string]interface{})
		tmp["params"] = map[string]interface{}{"response": msg}
		conn.Send(tmp)
	}
}

func (h *GCodeHelper) handleSubscribeOutput(request RuntimeRequest) (interface{}, error) {
	conn := request.Connection()
	template := request.GetDict("response_template", map[string]interface{}{})
	h.clients[conn] = template
	if !h.isOutputRegistered {
		h.gcode.RegisterOutputHandler(h.outputCallback)
		h.isOutputRegistered = true
	}
	return nil, nil
}

const SubscriptionRefreshTime = 0.25

type queryStatusReactor interface {
	printerpkg.ModuleReactor
	Completion() interface{}
}

type queryStatusCompletion interface {
	Complete(result interface{})
	Wait(waketime float64, waketimeResult interface{}) interface{}
}

type queryStatusEnvelope struct {
	conn         RuntimeClient
	subscription map[string]interface{}
	send         func(interface{})
	template     map[string]interface{}
}

type QueryStatusHelper struct {
	printer        printerpkg.ModulePrinter
	clients        map[RuntimeClient]queryStatusEnvelope
	pendingQueries []queryStatusEnvelope
	queryTimer     printerpkg.TimerHandle
	lastQuery      map[string]interface{}
	monotonic      func() float64
}

func NewQueryStatusHelper(printer printerpkg.ModulePrinter, registry RuntimeRegistry, monotonic func() float64) *QueryStatusHelper {
	helper := &QueryStatusHelper{
		printer:        printer,
		clients:        map[RuntimeClient]queryStatusEnvelope{},
		pendingQueries: []queryStatusEnvelope{},
		lastQuery:      map[string]interface{}{},
		monotonic:      monotonic,
	}
	if helper.monotonic == nil {
		helper.monotonic = func() float64 {
			return helper.reactor().Monotonic()
		}
	}
	_ = registry.RegisterEndpoint("objects/list", helper.handleList)
	_ = registry.RegisterEndpoint("objects/query", helper.handleQuery)
	_ = registry.RegisterEndpoint("objects/subscribe", helper.handleSubscribe)
	_ = registry.RegisterEndpoint("objects/object_query", helper.handleObjectQuery)
	return helper
}

func (h *QueryStatusHelper) reactor() queryStatusReactor {
	reactorObj := h.printer.Reactor()
	reactor, ok := reactorObj.(queryStatusReactor)
	if !ok {
		panic(fmt.Sprintf("reactor does not implement queryStatusReactor: %T", reactorObj))
	}
	return reactor
}

func (h *QueryStatusHelper) newCompletion() queryStatusCompletion {
	completionObj := h.reactor().Completion()
	completion, ok := completionObj.(queryStatusCompletion)
	if !ok {
		panic(fmt.Sprintf("completion does not implement queryStatusCompletion: %T", completionObj))
	}
	return completion
}

func (h *QueryStatusHelper) handleList(request RuntimeRequest) (interface{}, error) {
	objectList := h.printer.LookupObjects("")
	objects := []interface{}{}
	for _, obj := range objectList {
		if obj == "Get_status" {
			objects = append(objects, obj)
		}
	}
	request.Send(map[string][]interface{}{"objects": objects})
	return nil, nil
}

func (h *QueryStatusHelper) doQuery(eventtime float64) float64 {
	lastQuery := h.lastQuery
	h.lastQuery = map[string]interface{}{}
	query := h.lastQuery
	msglist := append([]queryStatusEnvelope{}, h.pendingQueries...)
	h.pendingQueries = []queryStatusEnvelope{}
	for _, subscription := range h.clients {
		msglist = append(msglist, subscription)
	}

	for _, envelope := range msglist {
		conn := envelope.conn
		subscription := envelope.subscription
		sendFunc := envelope.send
		template := envelope.template
		isQuery := conn == nil
		if !isQuery && conn.IsClosed() {
			delete(h.clients, conn)
			continue
		}

		cquery := map[string]interface{}{}
		for objName, reqItems := range subscription {
			res := query[objName]
			if res == nil {
				printerObject := h.printer.LookupObject(objName, nil)
				if printerObject == nil {
					res = map[string]interface{}{}
					query[objName] = res
				} else {
					method := reflects.GetMethod(printerObject, "Get_status")
					if method == nil || method.(reflect.Value).IsNil() {
						res = map[string]interface{}{}
						query[objName] = res
					} else {
						argv := []reflect.Value{reflect.ValueOf(eventtime)}
						status := method.(reflect.Value).Call(argv)
						if len(status) > 0 {
							res = status[0].Interface()
							query[objName] = res
						}
					}
				}
			}
			if reqItems == nil {
				keys := statusKeys(res)
				reqItems = keys
				if len(keys) != 0 {
					subscription[objName] = reqItems
				}
			}

			lres := lastQuery[objName]
			if lres == nil {
				lres = map[string]interface{}{}
			}
			cres := map[string]interface{}{}
			for _, reqItem := range ReqItems(reqItems) {
				current := statusValue(res, reqItem)
				previous := statusValue(lres, reqItem)
				if isQuery || !reflect.DeepEqual(current, previous) {
					cres[reqItem] = current
				}
			}
			if len(cres) != 0 || isQuery {
				cquery[objName] = cres
			}
		}

		if len(cquery) != 0 || isQuery {
			if template != nil {
				tmp := template
				tmp["params"] = map[string]interface{}{"eventtime": eventtime, "status": cquery}
				sendFunc(tmp)
			}
		}
	}

	if query == nil {
		return constants.NEVER
	}
	return eventtime + SubscriptionRefreshTime
}

func statusKeys(status interface{}) []string {
	keys := []string{}
	switch typed := status.(type) {
	case map[string]float64:
		for key := range typed {
			keys = append(keys, key)
		}
	case map[string]string:
		for key := range typed {
			keys = append(keys, key)
		}
	case map[string]interface{}:
		for key := range typed {
			keys = append(keys, key)
		}
	}
	return keys
}

func statusValue(status interface{}, key string) interface{} {
	switch typed := status.(type) {
	case map[string]float64:
		return typed[key]
	case map[string]string:
		return typed[key]
	case map[string]interface{}:
		return typed[key]
	default:
		return nil
	}
}

func (h *QueryStatusHelper) handleQuery(request RuntimeRequest) (interface{}, error) {
	return h.handleQueryRequest(request, false)
}

func (h *QueryStatusHelper) handleQueryRequest(request RuntimeRequest, isSubscribe bool) (interface{}, error) {
	objects := request.GetDict("objects", object.Sentinel{})
	for key, value := range objects {
		if reflect.TypeOf(key).Kind() != reflect.String {
			return nil, errors.New("Invalid argument")
		}
		if value != nil {
			items := value.([]interface{})
			for _, reqItem := range items {
				if reflect.TypeOf(reqItem).Kind() != reflect.String {
					return nil, errors.New("Invalid argument")
				}
			}
		}
	}

	conn := request.Connection()
	template := request.GetDict("response_template", map[string]interface{}{})
	if isSubscribe {
		delete(h.clients, conn)
	}
	completion := h.newCompletion()
	h.pendingQueries = append(h.pendingQueries, queryStatusEnvelope{
		subscription: objects,
		send:         completion.Complete,
		template:     map[string]interface{}{},
	})
	if h.queryTimer == nil {
		h.queryTimer = h.reactor().RegisterTimer(h.doQuery, constants.NOW)
	}
	msg := completion.Wait(constants.NEVER, nil)
	if msg == nil {
		msg = map[string]interface{}{}
	}
	request.Send(msg.(map[string]interface{})["params"])
	if isSubscribe {
		h.clients[conn] = queryStatusEnvelope{
			conn:         conn,
			subscription: objects,
			send:         conn.Send,
			template:     template,
		}
	}
	return nil, nil
}

func (h *QueryStatusHelper) handleObjectQuery(request RuntimeRequest) (interface{}, error) {
	objects := request.GetDict("objects", object.Sentinel{})
	if len(objects) == 0 {
		return nil, errors.New("objects empty")
	}

	objectNames := make([]string, 0, len(objects))
	for key := range objects {
		objectNames = append(objectNames, key)
	}
	status := make(map[string]interface{})
	eventtime := h.monotonic()
	for _, name := range objectNames {
		obj := h.printer.LookupObject(name, nil)
		if obj == nil {
			return nil, fmt.Errorf("object: %s not found", name)
		}

		method := reflects.GetMethod(obj, "Get_status")
		if method == nil || method.(reflect.Value).IsNil() {
			return nil, fmt.Errorf("object: %s method Get_status not found", name)
		}

		argv := []reflect.Value{reflect.ValueOf(eventtime)}
		rv := method.(reflect.Value).Call(argv)
		if len(rv) != 1 {
			return nil, fmt.Errorf("object: %s method Get_status return value should only one", name)
		}
		status[name] = rv[0].Interface()
	}

	request.Send(map[string]interface{}{
		"eventtime": eventtime,
		"status":    status,
	})
	return nil, nil
}

func (h *QueryStatusHelper) handleSubscribe(request RuntimeRequest) (interface{}, error) {
	_, err := h.handleQueryRequest(request, true)
	return nil, err
}
