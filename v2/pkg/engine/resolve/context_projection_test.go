package resolve

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
)

func TestContextGroupsReuseSharedProjection(t *testing.T) {
	doc := astjson.MustParse(`{"accounts":[
		{"rates":[{"code":"USD"},{"code":"EUR"}],"wallets":[{"kind":"A"},{"kind":"B"},{"kind":"A"}]},
		{"rates":[{"code":"USD"},{"code":"EUR"}],"wallets":[{"kind":"B"}]},
		{"rates":[{"code":"JPY"}],"wallets":[{"kind":"A"}]}
	]}`)
	loader := Loader{dataBuffer: &DataBuffer{data: doc}}
	bindings := []ContextBinding{
		{Variable: "rates", Sources: []ContextSource{{Depth: 1, Selection: []ContextSelection{{Field: "rates", Children: []ContextSelection{{Field: "code"}}}}}}},
		{Variable: "kind", Sources: []ContextSource{{Depth: 2, Selection: []ContextSelection{{Field: "kind"}}}}},
	}
	path := []FetchItemPathElement{ArrayPath("accounts"), ArrayPath("wallets")}
	groups := loader.contextGroups(path, bindings)
	require.Len(t, groups, 3)
	require.Len(t, groups[0].items, 2)
	require.Len(t, groups[1].items, 2)
	require.Len(t, groups[2].items, 1)
	require.JSONEq(t, `{"rates":["USD","EUR"],"kind":"A"}`, string(groups[0].variables.MarshalTo(nil)))
	require.JSONEq(t, `{"rates":["USD","EUR"],"kind":"B"}`, string(groups[1].variables.MarshalTo(nil)))
	require.JSONEq(t, `{"rates":["JPY"],"kind":"A"}`, string(groups[2].variables.MarshalTo(nil)))
	require.Same(t, groups[0].variables.Get("rates"), groups[1].variables.Get("rates"))

	// A later fetch (or subscription event) must read the new response state.
	doc.Get("accounts", "0", "rates", "0").Set(nil, "code", astjson.StringValue(nil, "GBP"))
	next := loader.contextGroups(path, bindings)
	require.Len(t, next, 4)
	require.JSONEq(t, `{"rates":["GBP","EUR"],"kind":"A"}`, string(next[0].variables.MarshalTo(nil)))
}
