package caching

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Emitted verbatim in a header a CDN purges by, so the exact string is the contract.
func TestLabels(t *testing.T) {
	t.Run("derived labels carry their tier prefix", func(t *testing.T) {
		require.Equal(t, "subgraph-accounts", SubgraphLabel("accounts"))
		require.Equal(t, "type-accounts-User", TypeLabel("accounts", "User"))
	})

	t.Run("sort is coarsest first then lexical", func(t *testing.T) {
		labels := []string{"zeta", "type-b-X", "subgraph-b", "alpha", "type-a-X", "subgraph-a"}
		SortLabels(labels)
		require.Equal(t, []string{"subgraph-a", "subgraph-b", "type-a-X", "type-b-X", "alpha", "zeta"}, labels)
	})

	t.Run("merge dedupes and keeps first seen order", func(t *testing.T) {
		require.Equal(t, []string{"a", "b", "c"}, MergeLabels(nil, []string{"a", "b"}, []string{"b", "c"}, nil, []string{"a"}))
		require.Equal(t, []string{"a", "b"}, MergeLabels([]string{"a"}, []string{"a", "b"}))
		require.Nil(t, MergeLabels(nil, nil, []string{}))
	})
}
