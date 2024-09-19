package fastjsonext

import (
	"github.com/wundergraph/astjson"
)

func AppendErrorToArray(arena *astjson.Arena, v *astjson.Value, msg string, path []PathElement) {
	if v.Type() != astjson.TypeArray {
		return
	}
	errorObject := CreateErrorObjectWithPath(arena, msg, path)
	items, _ := v.Array()
	v.SetArrayItem(len(items), errorObject)
}

func AppendErrorWithExtensionsCodeToArray(arena *astjson.Arena, v *astjson.Value, msg, code string, path []PathElement) {
	if v.Type() != astjson.TypeArray {
		return
	}
	errorObject := CreateErrorObjectWithPath(arena, msg, path)
	extensions := arena.NewObject()
	extensions.Set("code", arena.NewString(code))
	errorObject.Set("extensions", extensions)
	items, _ := v.Array()
	v.SetArrayItem(len(items), errorObject)
}

type PathElement struct {
	Name string
	Idx  int
}

func CreateErrorObjectWithPath(arena *astjson.Arena, message string, path []PathElement) *astjson.Value {
	errorObject := arena.NewObject()
	errorObject.Set("message", arena.NewString(message))
	if len(path) == 0 {
		return errorObject
	}
	errorPath := arena.NewArray()
	for i := range path {
		if path[i].Name != "" {
			errorPath.SetArrayItem(i, arena.NewString(path[i].Name))
		} else {
			errorPath.SetArrayItem(i, arena.NewNumberInt(path[i].Idx))
		}
	}
	errorObject.Set("path", errorPath)
	return errorObject
}

func PrintGraphQLResponse(data, errors *astjson.Value) string {
	out := astjson.MustParse(`{}`)
	if astjson.ValueIsNonNull(errors) {
		out.Set("errors", errors)
	}
	out.Set("data", data)
	return string(out.MarshalTo(nil))
}
