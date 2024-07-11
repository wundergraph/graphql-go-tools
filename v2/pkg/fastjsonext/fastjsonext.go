package fastjsonext

import (
	"bytes"

	"github.com/valyala/fastjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
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

func AppendErrorWithMessage(v *fastjson.Value, msg string) {
	if v.Type() != fastjson.TypeArray {
		return
	}
	items, err := v.Array()
	if err != nil {
		return
	}
	v.SetArrayItem(len(items), fastjson.MustParse(`{"message":"`+msg+`"}`))
}
