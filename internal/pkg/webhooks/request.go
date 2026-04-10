// RequestParams provides typed access to a JSON API request parameter map.
//
// It is a pure, project-independent value type embedded by project.WebRequest.
package webhooks

import (
	"goklipper/common/logger"
	"goklipper/common/utils/collections"
	"goklipper/common/utils/object"
	"reflect"
)

// RequestParams holds the method name and parameter map of a JSON API request
// and exposes typed accessor helpers. It has no dependency on project types.
type RequestParams struct {
	Method string
	Params map[string]interface{}
}

func (r *RequestParams) Get_dict(item string, default1 interface{}) map[string]interface{} {
	obj := r.Get(item, default1, []reflect.Kind{reflect.Map})
	if obj == nil {
		return nil
	}
	v, ok := obj.(map[string]interface{})
	if ok {
		return v
	}
	return nil
}

func (r *RequestParams) Get_str(item string, default1 interface{}) string {
	obj := r.Get(item, default1, []reflect.Kind{reflect.String})
	if obj == nil {
		return ""
	}
	v, ok := obj.(string)
	if ok {
		return v
	}
	return ""
}

func (r *RequestParams) Get_int(item string, default_value int) int {
	obj, ok := r.Params[item]
	if !ok || obj == nil {
		return default_value
	}
	v, ok := obj.(float64)
	if ok {
		return int(v)
	}
	return 0
}

func (r *RequestParams) GetBool(item string, default_value bool) bool {
	obj, ok := r.Params[item]
	if !ok || obj == nil {
		return default_value
	}
	v, ok := obj.(bool)
	if ok {
		return v
	}
	return false
}

func (r *RequestParams) Get_float(item string, default1 interface{}) float64 {
	obj := r.Get(item, default1, []reflect.Kind{reflect.Int, reflect.Float64})
	if obj == nil {
		return 0
	}
	v, ok := obj.(float64)
	if ok {
		return v
	}
	return 0
}

func (r *RequestParams) Get(item string, default1 interface{}, types []reflect.Kind) interface{} {
	value, ok := r.Params[item]
	if !ok {
		if object.IsSentinel(default1) {
			logger.Error("Missing Argument [%s]", item)
			return nil
		} else {
			value = default1
		}
	}
	if types != nil && collections.NotInKind(types, value) && collections.InStringMap(r.Params, item) {
		logger.Error("Invalid Argument Type [%s]", item)
		return nil
	}
	return value
}

// ReqItems converts a reqItems value (which may be []interface{} or []string)
// to a normalised []string slice. Used when iterating subscription key lists.
func ReqItems(reqItems interface{}) []string {
	elems := make([]string, 0)
	if items, ok := reqItems.([]interface{}); ok {
		for _, key := range items {
			elems = append(elems, key.(string))
		}
	}
	if items, ok := reqItems.([]string); ok {
		elems = append(elems, items...)
	}
	return elems
}
