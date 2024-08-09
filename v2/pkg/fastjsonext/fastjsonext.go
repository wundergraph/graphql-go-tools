package fastjsonext

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/valyala/fastjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

var (
	NullValue = fastjson.MustParse(`null`)
)

func MergeValues(a, b *fastjson.Value) (*fastjson.Value, bool) {
	if a == nil {
		return b, true
	}
	if b == nil {
		return a, false
	}
	if a.Type() != b.Type() {
		return a, false
	}
	switch a.Type() {
	case fastjson.TypeObject:
		ao, _ := a.Object()
		bo, _ := b.Object()
		ao.Visit(func(key []byte, l *fastjson.Value) {
			sKey := unsafebytes.BytesToString(key)
			r := bo.Get(sKey)
			if r == nil {
				return
			}
			merged, changed := MergeValues(l, r)
			if changed {
				ao.Set(unsafebytes.BytesToString(key), merged)
			}
		})
		bo.Visit(func(key []byte, r *fastjson.Value) {
			sKey := unsafebytes.BytesToString(key)
			if ao.Get(sKey) != nil {
				return
			}
			ao.Set(sKey, r)
		})
		return a, false
	case fastjson.TypeArray:
		aa, _ := a.Array()
		ba, _ := b.Array()
		for i := 0; i < len(ba); i++ {
			a.SetArrayItem(len(aa)+i, ba[i])
		}
		return a, false
	case fastjson.TypeFalse:
		if b.Type() == fastjson.TypeTrue {
			return b, true
		}
		return a, false
	case fastjson.TypeTrue:
		if b.Type() == fastjson.TypeFalse {
			return b, true
		}
		return a, false
	case fastjson.TypeNull:
		if b.Type() != fastjson.TypeNull {
			return b, true
		}
		return a, false
	case fastjson.TypeNumber:
		af, _ := a.Float64()
		bf, _ := b.Float64()
		if af != bf {
			return b, true
		}
		return a, false
	case fastjson.TypeString:
		as, _ := a.StringBytes()
		bs, _ := b.StringBytes()
		if !bytes.Equal(as, bs) {
			return b, true
		}
		return a, false
	default:
		return b, true
	}
}

func MergeValuesWithPath(a, b *fastjson.Value, path ...string) (*fastjson.Value, bool) {
	if len(path) == 0 {
		return MergeValues(a, b)
	}
	root := fastjson.MustParseBytes([]byte(`{}`))
	current := root
	for i := 0; i < len(path)-1; i++ {
		current.Set(path[i], fastjson.MustParseBytes([]byte(`{}`)))
		current = current.Get(path[i])
	}
	current.Set(path[len(path)-1], b)
	return MergeValues(a, root)
}

func AppendToArray(array, value *fastjson.Value) {
	if array.Type() != fastjson.TypeArray {
		return
	}
	items, _ := array.Array()
	array.SetArrayItem(len(items), value)
}

func AppendErrorToArray(v *fastjson.Value, msg string, path []PathElement) {
	if v.Type() != fastjson.TypeArray {
		return
	}
	errorObject := CreateErrorObjectWithPath(msg, path)
	items, _ := v.Array()
	v.SetArrayItem(len(items), errorObject)
}

func SetValue(v *fastjson.Value, value *fastjson.Value, path ...string) {
	for i := 0; i < len(path)-1; i++ {
		parent := v
		v = v.Get(path[i])
		if v == nil {
			child := fastjson.MustParse(`{}`)
			parent.Set(path[i], child)
			v = child
		}
	}
	v.Set(path[len(path)-1], value)
}

func SetNull(v *fastjson.Value, path ...string) {
	SetValue(v, fastjson.MustParse(`null`), path...)
}

func ValueIsNonNull(v *fastjson.Value) bool {
	if v == nil {
		return false
	}
	if v.Type() == fastjson.TypeNull {
		return false
	}
	return true
}

func ValueIsNull(v *fastjson.Value) bool {
	return !ValueIsNonNull(v)
}

type PathElement struct {
	Name string
	Idx  int
}

func CreateErrorObjectWithPath(message string, path []PathElement) *fastjson.Value {
	errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"%s"}`, message))
	if len(path) == 0 {
		return errorObject
	}
	errorPath := fastjson.MustParse(`[]`)
	for i := range path {
		if path[i].Name != "" {
			errorPath.SetArrayItem(i, fastjson.MustParse(fmt.Sprintf(`"%s"`, path[i].Name)))
		} else {
			errorPath.SetArrayItem(i, fastjson.MustParse(strconv.FormatInt(int64(path[i].Idx), 10)))
		}
	}
	errorObject.Set("path", errorPath)
	return errorObject
}

func PrintGraphQLResponse(data, errors *fastjson.Value) string {
	out := fastjson.MustParse(`{}`)
	if ValueIsNonNull(errors) {
		out.Set("errors", errors)
	}
	out.Set("data", data)
	return string(out.MarshalTo(nil))
}

func DeduplicateObjectKeysRecursively(v *fastjson.Value) {
	if v.Type() != fastjson.TypeObject {
		return
	}
	o, _ := v.Object()
	seen := make(map[string]struct{})
	o.Visit(func(k []byte, v *fastjson.Value) {
		key := string(k)
		if _, ok := seen[key]; ok {
			o.Del(key)
			return
		} else {
			seen[key] = struct{}{}
		}
		DeduplicateObjectKeysRecursively(v)
	})
}
