package webhooks

import (
	"errors"
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	"reflect"
)

type muxEndpoint struct {
	key    string
	values map[string]func(RuntimeRequest)
}

type typedMuxRegistry interface {
	RegisterMuxEndpoint(path, key, value string, callback func(RuntimeRequest))
}

type TypedEndpointBinding[T RuntimeRequest] struct {
	Path    string
	Handler func(T) (interface{}, error)
}

type runtimeRequestValueGetter interface {
	Get(item string, defaultValue interface{}, types []reflect.Kind) interface{}
}

// EndpointRegistry stores generic webhook endpoint, remote method, and mux
// routing state that can be shared by project-specific webhook frontends.
type EndpointRegistry struct {
	endpoints     map[string]func(RuntimeRequest) (interface{}, error)
	remoteMethods map[string]map[RuntimeClient]interface{}
	muxEndpoints  map[string]muxEndpoint
}

func NewEndpointRegistry() *EndpointRegistry {
	registry := &EndpointRegistry{
		endpoints:     map[string]func(RuntimeRequest) (interface{}, error){},
		remoteMethods: map[string]map[RuntimeClient]interface{}{},
		muxEndpoints:  map[string]muxEndpoint{},
	}
	registry.endpoints["list_endpoints"] = registry.handleListEndpoints
	return registry
}

func convertRuntimeRequest[T RuntimeRequest](request RuntimeRequest) (T, error) {
	typed, ok := any(request).(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("invalid webhook request type: %T", request)
	}
	return typed, nil
}

// RegisterTypedEndpoint adapts a concrete RuntimeRequest subtype handler onto a
// generic runtime registry.
func RegisterTypedEndpoint[T RuntimeRequest](registry RuntimeRegistry, path string, handler func(T) (interface{}, error)) error {
	return registry.RegisterEndpoint(path, func(request RuntimeRequest) (interface{}, error) {
		typed, err := convertRuntimeRequest[T](request)
		if err != nil {
			return nil, err
		}
		return handler(typed)
	})
}

func RegisterTypedEndpoints[T RuntimeRequest](registry RuntimeRegistry, bindings []TypedEndpointBinding[T]) error {
	for _, binding := range bindings {
		if err := RegisterTypedEndpoint[T](registry, binding.Path, binding.Handler); err != nil {
			return err
		}
	}
	return nil
}

// RegisterTypedMuxEndpoint adapts a concrete RuntimeRequest subtype handler for
// mux-style webhook endpoints.
func RegisterTypedMuxEndpoint[T RuntimeRequest](registry typedMuxRegistry, path, key, value string, handler func(T)) {
	registry.RegisterMuxEndpoint(path, key, value, func(request RuntimeRequest) {
		typed, err := convertRuntimeRequest[T](request)
		if err != nil {
			panic(err)
		}
		handler(typed)
	})
}

func (r *EndpointRegistry) RegisterEndpoint(path string, handler func(RuntimeRequest) (interface{}, error)) error {
	if _, ok := r.endpoints[path]; ok {
		return errors.New("Path already registered to an endpoint")
	}
	r.endpoints[path] = handler
	return nil
}

func (r *EndpointRegistry) Handler(path string) func(RuntimeRequest) (interface{}, error) {
	cb := r.endpoints[path]
	if cb == nil {
		logger.Error(fmt.Sprintf("webhooks: No registered callback for path '%s'", path))
		return nil
	}
	return cb
}

func (r *EndpointRegistry) HandleRemoteMethodRegistration(request RuntimeRequest) (interface{}, error) {
	template := request.GetDict("response_template", object.Sentinel{})
	if template == nil {
		template = map[string]interface{}{}
	}
	method := request.GetStr("remote_method", object.Sentinel{})
	conn := request.Connection()
	logger.Infof("webhooks: registering remote method '%s' for connection %p", method, conn)
	if _, ok := r.remoteMethods[method]; !ok {
		r.remoteMethods[method] = map[RuntimeClient]interface{}{}
	}
	r.remoteMethods[method][conn] = template
	return nil, nil
}

func (r *EndpointRegistry) CallRemoteMethod(method string, kwargs interface{}) error {
	connMap, ok := r.remoteMethods[method]
	if !ok {
		return fmt.Errorf("Remote method '%s' not registered", method)
	}
	validConns := make(map[RuntimeClient]interface{})
	for conn, template := range connMap {
		if conn.IsClosed() {
			continue
		}
		validConns[conn] = template
		out := map[string]interface{}{
			"params": kwargs,
		}
		for k, v := range template.(map[string]interface{}) {
			out[k] = v
		}
		conn.Send(out)
	}
	if len(validConns) == 0 {
		delete(r.remoteMethods, method)
		return fmt.Errorf("No active connections for method '%s'", method)
	}
	r.remoteMethods[method] = validConns
	return nil
}

func (r *EndpointRegistry) RegisterMuxEndpoint(path, key, value string, callback func(RuntimeRequest)) {
	prev, ok := r.muxEndpoints[path]
	if !ok {
		if _, exists := r.endpoints[path]; exists {
			panic(fmt.Errorf("mux endpoint %s already registered to non-mux handler", path))
		}
		prev = muxEndpoint{
			key:    key,
			values: make(map[string]func(RuntimeRequest)),
		}
		r.muxEndpoints[path] = prev
		r.endpoints[path] = func(request RuntimeRequest) (interface{}, error) {
			return r.handleMuxPath(path, request)
		}
	}
	prevKey, prevValues := prev.key, prev.values
	if prevKey != key {
		panic(fmt.Errorf("mux endpoint %s %s %s may have only one key (%s)", path, key, value, prevKey))
	}
	if _, exists := prevValues[value]; exists {
		panic(fmt.Errorf("mux endpoint %s %s %s already registered (%+v)", path, key, value, prevValues))
	}
	prevValues[value] = callback
}

func (r *EndpointRegistry) handleListEndpoints(request RuntimeRequest) (interface{}, error) {
	request.Send(map[string]interface{}{
		"endpoints": str.MapStringKeys(r.endpoints),
	})
	return nil, nil
}

func (r *EndpointRegistry) handleMuxPath(path string, request RuntimeRequest) (interface{}, error) {
	ep := r.muxEndpoints[path]
	key, values := ep.key, ep.values
	_, hasDefault := values[""]
	keyParam := r.muxKey(request, key, hasDefault)
	callback, ok := values[keyParam]
	if !ok {
		panic(fmt.Errorf("The value '%s' is not valid for %s", keyParam, key))
	}
	callback(request)
	return nil, nil
}

func (r *EndpointRegistry) muxKey(request RuntimeRequest, key string, allowDefault bool) string {
	defaultValue := interface{}(object.Sentinel{})
	if allowDefault {
		defaultValue = ""
	}
	if getter, ok := request.(runtimeRequestValueGetter); ok {
		return cast.ToString(getter.Get(key, defaultValue, nil))
	}
	return request.GetStr(key, defaultValue)
}
