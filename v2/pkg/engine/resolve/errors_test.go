package resolve

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphQLError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload string
	}{
		{name: "required fields", payload: `{"message":"error"}`},
		{name: "empty optional fields", payload: `{"message":"error","path":[],"locations":[],"extensions":{}}`},
		{name: "extensions and unknown fields", payload: `{"message":"error","path":["field",2],"locations":[{"line":1,"column":2}],"extensions":{"id":9007199254740993,"nested":{"value":0.123456789123456789},"errors":[{"message":"nested"}]},"Message":"custom","custom":9007199254740993}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var value GraphQLError
			require.NoError(t, json.Unmarshal([]byte(tc.payload), &value))
			assert.EqualError(t, &value, "error")
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			assert.JSONEq(t, tc.payload, string(encoded))
			if value.Extensions["id"] != nil {
				assert.Equal(t, json.Number("9007199254740993"), value.Extensions["id"])
				assert.Contains(t, string(encoded), `"custom":9007199254740993`)
				assert.Contains(t, string(encoded), "0.123456789123456789")
			}
		})
	}
}

func TestGraphQLErrorPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	var value GraphQLError
	require.NoError(t, json.Unmarshal([]byte(`{"message":"error","custom":{"id":9007199254740993,"value":0.123456789123456789},"Message":"case-sensitive"}`), &value))
	value.Message = "rewritten"
	value.Extensions = map[string]any{"code": "CUSTOM"}
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"message":"rewritten","extensions":{"code":"CUSTOM"},"custom":{"id":9007199254740993,"value":0.123456789123456789},"Message":"case-sensitive"}`, string(encoded))
	assert.Contains(t, string(encoded), `"id":9007199254740993`)
	assert.Contains(t, string(encoded), `"value":0.123456789123456789`)

	// Reusing a value must not retain unknown fields from the previous error.
	require.NoError(t, json.Unmarshal([]byte(`{"message":"next"}`), &value))
	encoded, err = json.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"message":"next"}`, string(encoded))
}

func TestSubgraphErrorCodes(t *testing.T) {
	t.Parallel()

	err := &SubgraphError{DownstreamErrors: []*GraphQLError{
		nil,
		{Message: "no extensions"},
		{Extensions: map[string]any{"code": "A"}},
		{Extensions: map[string]any{"code": "B"}},
		{Extensions: map[string]any{"code": "A"}},
		{Extensions: map[string]any{"code": 42}},
	}}
	assert.Equal(t, []string{"A", "B"}, err.Codes())
}

func TestSubgraphError(t *testing.T) {
	t.Parallel()

	t.Run("Simple", func(t *testing.T) {
		t.Parallel()

		e := NewSubgraphError(DataSourceInfo{
			Name: "subgraphName",
			ID:   "subgraphID",
		}, "path", "", 500)

		assert.Equal(t, "subgraphName", e.DataSourceInfo.Name)
		assert.Equal(t, "path", e.Path)
		assert.Equal(t, "", e.Reason)
		assert.Equal(t, 500, e.ResponseCode)

		assert.Empty(t, e.DownstreamErrors)
		assert.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path'.")
	})

	t.Run("With a reason", func(t *testing.T) {
		t.Parallel()

		e := NewSubgraphError(DataSourceInfo{
			Name: "subgraphName",
			ID:   "subgraphID",
		}, "path", "reason", 500)

		assert.Equal(t, "subgraphName", e.DataSourceInfo.Name)
		assert.Equal(t, "path", e.Path)
		assert.Equal(t, "reason", e.Reason)
		assert.Equal(t, 500, e.ResponseCode)

		assert.Empty(t, e.DownstreamErrors)
		assert.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path', Reason: reason.")
	})
}

func TestRateLimitError(t *testing.T) {
	t.Parallel()

	t.Run("Without a reason", func(t *testing.T) {
		t.Parallel()

		e := NewRateLimitError("subgraphName", "path", "")

		assert.Equal(t, "subgraphName", e.SubgraphName)
		assert.Equal(t, "path", e.Path)
		assert.Equal(t, "", e.Reason)

		assert.EqualError(t, e, "Rate limit rejected for Subgraph 'subgraphName' at Path 'path'.")
	})

	t.Run("With a reason", func(t *testing.T) {
		t.Parallel()

		e := NewRateLimitError("subgraphName", "path", "limit")

		assert.Equal(t, "subgraphName", e.SubgraphName)
		assert.Equal(t, "path", e.Path)
		assert.Equal(t, "limit", e.Reason)

		assert.EqualError(t, e, "Rate limit rejected for Subgraph 'subgraphName' at Path 'path', Reason: limit.")
	})
}
