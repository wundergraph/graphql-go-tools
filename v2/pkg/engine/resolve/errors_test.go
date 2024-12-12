package resolve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
