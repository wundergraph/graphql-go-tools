package resolve

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGraphQLError(t *testing.T) {
	for _, payload := range []string{
		`{"message":"error"}`,
		`{"message":"error","path":[],"locations":[],"extensions":{}}`,
		`{"message":"error","path":["field",2],"locations":[{"line":1,"column":2}],"extensions":{"id":9007199254740993,"nested":{"value":0.123456789123456789},"errors":[{"message":"nested"}]},"Message":"custom","custom":9007199254740993}`,
	} {
		var value GraphQLError
		require.NoError(t, json.Unmarshal([]byte(payload), &value))
		require.EqualError(t, &value, "error")
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		require.JSONEq(t, payload, string(encoded))
		if value.Extensions["id"] != nil {
			require.Equal(t, json.Number("9007199254740993"), value.Extensions["id"])
			require.Equal(t, json.Number("9007199254740993"), value.AdditionalFields["custom"])
			require.Contains(t, string(encoded), "9007199254740993")
			require.Contains(t, string(encoded), "0.123456789123456789")
		}
	}

	value := GraphQLError{Message: "error", AdditionalFields: map[string]any{
		"message": 42, "path": "invalid", "locations": "invalid", "extensions": "invalid", "custom": true,
	}}
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	require.JSONEq(t, `{"message":"error","custom":true}`, string(encoded))
	require.NoError(t, json.Unmarshal([]byte(`{"message":"next"}`), &value))
	require.Nil(t, value.AdditionalFields, "reusing a value must not retain old fields")
}

func TestSubgraphErrorCodes(t *testing.T) {
	err := &SubgraphError{DownstreamErrors: []*GraphQLError{
		nil,
		{Message: "no extensions"},
		{Extensions: map[string]any{"code": "A"}},
		{Extensions: map[string]any{"code": "B"}},
		{Extensions: map[string]any{"code": "A"}},
		{Extensions: map[string]any{"code": 42}},
	}}
	require.Equal(t, []string{"A", "B"}, err.Codes())
}

func TestSubgraphError(t *testing.T) {
	t.Run("Simple", func(t *testing.T) {
		e := NewSubgraphError(DataSourceInfo{
			Name: "subgraphName",
			ID:   "subgraphID",
		}, "path", "", 500)

		require.Equal(t, e.DataSourceInfo.Name, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "")
		require.Equal(t, e.ResponseCode, 500)

		require.Equal(t, len(e.DownstreamErrors), 0)
		require.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path'.")
	})

	t.Run("With a reason", func(t *testing.T) {
		e := NewSubgraphError(DataSourceInfo{
			Name: "subgraphName",
			ID:   "subgraphID",
		}, "path", "reason", 500)

		require.Equal(t, e.DataSourceInfo.Name, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "reason")
		require.Equal(t, e.ResponseCode, 500)

		require.Equal(t, len(e.DownstreamErrors), 0)
		require.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path', Reason: reason.")
	})
}

func TestRateLimitError(t *testing.T) {
	t.Run("Without a reason", func(t *testing.T) {
		e := NewRateLimitError("subgraphName", "path", "")

		require.Equal(t, e.SubgraphName, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "")

		require.EqualError(t, e, "Rate limit rejected for Subgraph 'subgraphName' at Path 'path'.")
	})

	t.Run("With a reason", func(t *testing.T) {
		e := NewRateLimitError("subgraphName", "path", "limit")

		require.Equal(t, e.SubgraphName, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "limit")

		require.EqualError(t, e, "Rate limit rejected for Subgraph 'subgraphName' at Path 'path', Reason: limit.")
	})
}
