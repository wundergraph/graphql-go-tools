package fastjsonext

import (
	"fmt"
	"strconv"

	"github.com/wundergraph/astjson"
)

func AppendErrorToArray(v *astjson.Value, msg string, path []PathElement) {
	if v.Type() != astjson.TypeArray {
		return
	}
	errorObject := CreateErrorObjectWithPath(msg, path)
	items, _ := v.Array()
	v.SetArrayItem(len(items), errorObject)
}

type PathElement struct {
	Name string
	Idx  int
}

func CreateErrorObjectWithPath(message string, path []PathElement) *astjson.Value {
	errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"%s"}`, message))
	if len(path) == 0 {
		return errorObject
	}
	errorPath := astjson.MustParse(`[]`)
	for i := range path {
		if path[i].Name != "" {
			errorPath.SetArrayItem(i, astjson.MustParse(fmt.Sprintf(`"%s"`, path[i].Name)))
		} else {
			errorPath.SetArrayItem(i, astjson.MustParse(strconv.FormatInt(int64(path[i].Idx), 10)))
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
