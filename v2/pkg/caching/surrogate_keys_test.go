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

	t.Run("a declared key is visible ASCII, derived spelling included", func(t *testing.T) {
		for _, tag := range []string{"users", "user-42", "a/b:c", "subgraph_x", "typed", "subgraph-accounts", "type-accounts-User"} {
			require.True(t, ValidDeclaredSurrogateKey(tag), tag)
		}
		for _, tag := range []string{"", "a b", "a\tb", "a\nb", "a\x00b", "a\x7fb", "\u00fcber", "用户"} {
			require.False(t, ValidDeclaredSurrogateKey(tag), "%q", tag)
		}
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
