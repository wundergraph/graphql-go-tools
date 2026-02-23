package fastjsonext

import (
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

func AppendErrorToArray(a arena.Arena, v *astjson.Value, msg string, path []PathElement) {
	if v.Type() != astjson.TypeArray {
		return
	}
	errorObject := CreateErrorObjectWithPath(a, msg, path)
	items, _ := v.Array()
	v.SetArrayItem(a, len(items), errorObject)
}

func AppendErrorWithExtensionsCodeToArray(a arena.Arena, v *astjson.Value, msg, code string, path []PathElement) {
	if v.Type() != astjson.TypeArray {
		return
	}
	errorObject := CreateErrorObjectWithPath(a, msg, path)
	extensions := astjson.ObjectValue(a)
	extensions.Set(a, "code", astjson.StringValue(a, code))
	errorObject.Set(a, "extensions", extensions)
	items, _ := v.Array()
	v.SetArrayItem(a, len(items), errorObject)
}

type PathElement struct {
	Name string
	Idx  int
}

func CreateErrorObjectWithPath(a arena.Arena, message string, path []PathElement) *astjson.Value {
	errorObject := astjson.ObjectValue(a)
	errorObject.Set(a, "message", astjson.StringValue(a, message))
	if len(path) == 0 {
		return errorObject
	}
	errorPath := astjson.ArrayValue(a)
	for i := range path {
		if path[i].Name != "" {
			errorPath.SetArrayItem(a, i, astjson.StringValue(a, path[i].Name))
		} else {
			errorPath.SetArrayItem(a, i, astjson.IntValue(a, path[i].Idx))
		}
	}
	errorObject.Set(a, "path", errorPath)
	return errorObject
}

func PrintGraphQLResponse(data, errors *astjson.Value) string {
	out := astjson.MustParse(`{}`)
	if astjson.ValueIsNonNull(errors) {
		out.Set(nil, "errors", errors)
	}
	out.Set(nil, "data", data)
	return string(out.MarshalTo(nil))
}
