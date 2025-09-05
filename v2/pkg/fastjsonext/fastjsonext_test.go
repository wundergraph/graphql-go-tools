package fastjsonext

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
)

func TestGetArray(t *testing.T) {
	a := astjson.MustParse(`[{"name":"Jens"},{"name":"Jannik"}]`)
	arr, err := a.Array()
	require.NoError(t, err)
	require.Equal(t, 2, len(arr))
	jens := arr[0].MarshalTo(nil)
	require.Equal(t, `{"name":"Jens"}`, string(jens))
	jannik := arr[1].MarshalTo(nil)
	require.Equal(t, `{"name":"Jannik"}`, string(jannik))
}

func TestAppendErrorWithMessage(t *testing.T) {
	a := astjson.MustParse(`[]`)
	AppendErrorToArray(&astjson.Arena{}, a, "error", nil)
	out := a.MarshalTo(nil)
	require.Equal(t, `[{"message":"error"}]`, string(out))
	AppendErrorToArray(&astjson.Arena{}, a, "error2", []PathElement{{Name: "a"}})
	out = a.MarshalTo(nil)
	require.Equal(t, `[{"message":"error"},{"message":"error2","path":["a"]}]`, string(out))
}

func TestCreateErrorObjectWithPath(t *testing.T) {
	v := CreateErrorObjectWithPath(&astjson.Arena{}, "my error message", []PathElement{
		{Name: "a"},
	})
	out := v.MarshalTo(nil)
	require.Equal(t, `{"message":"my error message","path":["a"]}`, string(out))
	v = CreateErrorObjectWithPath(&astjson.Arena{}, "my error message", []PathElement{
		{Name: "a"},
		{Idx: 1},
		{Name: "b"},
	})
	out = v.MarshalTo(nil)
	require.Equal(t, `{"message":"my error message","path":["a",1,"b"]}`, string(out))
	v = CreateErrorObjectWithPath(&astjson.Arena{}, "my error message", []PathElement{
		{Name: "a"},
		{Name: "b"},
	})
	out = v.MarshalTo(nil)
	require.Equal(t, `{"message":"my error message","path":["a","b"]}`, string(out))
}
