package fastjsonext

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/valyala/fastjson"
)

func TestMergeValues(t *testing.T) {
	a, b := fastjson.MustParse(`{"a":1}`), fastjson.MustParse(`{"b":2}`)
	merged, changed := MergeValues(a, b)
	require.Equal(t, false, changed)
	out := merged.MarshalTo(nil)
	require.Equal(t, `{"a":1,"b":2}`, string(out))
	out = merged.Get("b").MarshalTo(out[:0])
	require.Equal(t, `2`, string(out))
}

func TestMergeValuesArray(t *testing.T) {
	a, b := fastjson.MustParse(`[1,2]`), fastjson.MustParse(`[3,4]`)
	merged, changed := MergeValues(a, b)
	require.Equal(t, false, changed)
	out := merged.MarshalTo(nil)
	require.Equal(t, `[1,2,3,4]`, string(out))
}

func TestMergeValuesNestedObjects(t *testing.T) {
	a, b := fastjson.MustParse(`{"a":{"b":1}}`), fastjson.MustParse(`{"a":{"c":2}}`)
	merged, changed := MergeValues(a, b)
	require.Equal(t, false, changed)
	out := merged.MarshalTo(nil)
	require.Equal(t, `{"a":{"b":1,"c":2}}`, string(out))
}

func TestMergeValuesWithPath(t *testing.T) {
	a, b := fastjson.MustParse(`{"a":{"b":1}}`), fastjson.MustParse(`{"c":2}`)
	merged, changed := MergeValuesWithPath(a, b, "a")
	require.Equal(t, false, changed)
	out := merged.MarshalTo(nil)
	require.Equal(t, `{"a":{"b":1,"c":2}}`, string(out))
	e := fastjson.MustParse(`{"e":true}`)
	merged, changed = MergeValuesWithPath(merged, e, "a", "d")
	require.Equal(t, false, changed)
	out = merged.MarshalTo(out[:0])
	require.Equal(t, `{"a":{"b":1,"c":2,"d":{"e":true}}}`, string(out))
}

func TestGetArray(t *testing.T) {
	a := fastjson.MustParse(`[{"name":"Jens"},{"name":"Jannik"}]`)
	arr, err := a.Array()
	require.NoError(t, err)
	require.Equal(t, 2, len(arr))
	jens := arr[0].MarshalTo(nil)
	require.Equal(t, `{"name":"Jens"}`, string(jens))
	jannik := arr[1].MarshalTo(nil)
	require.Equal(t, `{"name":"Jannik"}`, string(jannik))
}
