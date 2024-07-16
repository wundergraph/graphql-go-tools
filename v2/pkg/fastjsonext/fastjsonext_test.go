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

func TestSetNull(t *testing.T) {
	a := fastjson.MustParse(`{"name":"Jens"}`)
	SetNull(a, "name")
	out := a.MarshalTo(nil)
	require.Equal(t, `{"name":null}`, string(out))

	b := fastjson.MustParse(`{"person":{"name":"Jens"}}`)
	SetNull(b, "person", "name")
	out = b.MarshalTo(nil)
	require.Equal(t, `{"person":{"name":null}}`, string(out))
}

func TestSetWithNonExistingPath(t *testing.T) {
	a := fastjson.MustParse(`{}`)
	SetValue(a, fastjson.MustParse(`1`), "a", "b")
	out := a.MarshalTo(nil)
	require.Equal(t, `{"a":{"b":1}}`, string(out))
}

func TestAppendErrorWithMessage(t *testing.T) {
	a := fastjson.MustParse(`[]`)
	AppendErrorToArray(a, "error", nil)
	out := a.MarshalTo(nil)
	require.Equal(t, `[{"message":"error"}]`, string(out))

	AppendErrorToArray(a, "error2", []PathElement{{Name: "a"}})
	out = a.MarshalTo(nil)
	require.Equal(t, `[{"message":"error"},{"message":"error2","path":["a"]}]`, string(out))
}

func TestCreateErrorObjectWithPath(t *testing.T) {
	v := CreateErrorObjectWithPath("my error message", []PathElement{
		{Name: "a"},
	})
	out := v.MarshalTo(nil)
	require.Equal(t, `{"message":"my error message","path":["a"]}`, string(out))
	v = CreateErrorObjectWithPath("my error message", []PathElement{
		{Name: "a"},
		{Idx: 1},
		{Name: "b"},
	})
	out = v.MarshalTo(nil)
	require.Equal(t, `{"message":"my error message","path":["a",1,"b"]}`, string(out))
	v = CreateErrorObjectWithPath("my error message", []PathElement{
		{Name: "a"},
		{Name: "b"},
	})
	out = v.MarshalTo(nil)
	require.Equal(t, `{"message":"my error message","path":["a","b"]}`, string(out))
}

func TestAppendToArray(t *testing.T) {
	a := fastjson.MustParse(`[1,2]`)
	AppendToArray(a, fastjson.MustParse(`3`))
	out := a.MarshalTo(nil)
	require.Equal(t, `[1,2,3]`, string(out))
}
