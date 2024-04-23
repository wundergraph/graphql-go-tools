package resolve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubgraphError(t *testing.T) {
	t.Run("Simple", func(t *testing.T) {
		e := NewSubgraphError("subgraphName", "path", "", 500)

		require.Equal(t, e.SubgraphName, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "")
		require.Equal(t, e.ResponseCode, 500)

		require.Equal(t, len(e.DownstreamErrors), 0)
		require.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path'.")
	})

	t.Run("With a reason", func(t *testing.T) {
		e := NewSubgraphError("subgraphName", "path", "reason", 500)

		require.Equal(t, e.SubgraphName, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "reason")
		require.Equal(t, e.ResponseCode, 500)

		require.Equal(t, len(e.DownstreamErrors), 0)
		require.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path', Reason: reason.")
	})

	t.Run("With downstream errors", func(t *testing.T) {
		e := NewSubgraphError("subgraphName", "path", "reason", 500)

		require.Equal(t, e.SubgraphName, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "reason")
		require.Equal(t, e.ResponseCode, 500)

		e.AppendDownstreamError(&GraphQLError{
			Message: "errorMessage",
			Path:    []any{"path"},
			Extensions: map[string]interface{}{
				"code": "code",
			},
		})

		require.Equal(t, len(e.DownstreamErrors), 1)
		require.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path', Reason: reason.\nDownstream errors:\n1. Subgraph error at Path 'path', Message: errorMessage, Extension Code: code.\n")
	})

	t.Run("With multi segment downstream errors", func(t *testing.T) {
		e := NewSubgraphError("subgraphName", "path", "reason", 500)

		require.Equal(t, e.SubgraphName, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "reason")
		require.Equal(t, e.ResponseCode, 500)

		e.AppendDownstreamError(&GraphQLError{
			Message: "errorMessage",
			Path:    []any{"path", "to", "success"},
			Extensions: map[string]interface{}{
				"code": "code",
			},
		})

		require.Equal(t, len(e.DownstreamErrors), 1)
		require.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path', Reason: reason.\nDownstream errors:\n1. Subgraph error at Path 'path.to.success', Message: errorMessage, Extension Code: code.\n")
	})

	t.Run("With mixed multi segment downstream errors", func(t *testing.T) {
		e := NewSubgraphError("subgraphName", "path", "reason", 500)

		require.Equal(t, e.SubgraphName, "subgraphName")
		require.Equal(t, e.Path, "path")
		require.Equal(t, e.Reason, "reason")
		require.Equal(t, e.ResponseCode, 500)

		e.AppendDownstreamError(&GraphQLError{
			Message: "errorMessage",
			Path:    []any{"path", 1, "to", "success"},
			Extensions: map[string]interface{}{
				"code": "code",
			},
		})

		require.Equal(t, len(e.DownstreamErrors), 1)
		require.EqualError(t, e, "Failed to fetch from Subgraph 'subgraphName' at Path: 'path', Reason: reason.\nDownstream errors:\n1. Subgraph error at Path 'path.1.to.success', Message: errorMessage, Extension Code: code.\n")
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
