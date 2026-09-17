package caching

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Emitted verbatim in a header a CDN purges by, so the exact string is the contract.
func TestSurrogateKeys(t *testing.T) {
	t.Run("derived surrogate keys carry their tier prefix", func(t *testing.T) {
		require.Equal(t, "subgraph-accounts", SubgraphSurrogateKey("accounts"))
		require.Equal(t, "type-accounts-User", TypeSurrogateKey("accounts", "User"))
	})

	t.Run("sort groups by tier and keeps the order within one", func(t *testing.T) {
		surrogateKeys := []string{"zeta", "type-b-X", "subgraph-b", "alpha", "type-a-X", "subgraph-a"}
		SortSurrogateKeys(surrogateKeys)
		require.Equal(t, []string{"subgraph-b", "subgraph-a", "type-b-X", "type-a-X", "zeta", "alpha"}, surrogateKeys)
	})

	t.Run("merge dedupes and keeps first seen order", func(t *testing.T) {
		require.Equal(t, []string{"a", "b", "c"}, MergeSurrogateKeys(nil, []string{"a", "b"}, []string{"b", "c"}, nil, []string{"a"}))
		require.Equal(t, []string{"a", "b"}, MergeSurrogateKeys([]string{"a"}, []string{"a", "b"}))
		require.Nil(t, MergeSurrogateKeys(nil, nil, []string{}))
	})
}
