package caching

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Emitted verbatim in a header a CDN purges by, so the exact string is the contract.
func TestSurrogateKeys(t *testing.T) {
	t.Run("derived surrogate keys carry their tier prefix", func(t *testing.T) {
		require.Equal(t, SurrogateKey{Value: "subgraph-accounts", Tier: SurrogateKeyTierSubgraph}, SubgraphSurrogateKey("accounts"))
		require.Equal(t, SurrogateKey{Value: "type-accounts-User", Tier: SurrogateKeyTierType}, TypeSurrogateKey("accounts", "User"))
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
		surrogateKeys := []SurrogateKey{
			DeclaredSurrogateKey("zeta"), TypeSurrogateKey("b", "X"), SubgraphSurrogateKey("b"),
			DeclaredSurrogateKey("subgraph-c"), TypeSurrogateKey("a", "X"), SubgraphSurrogateKey("a"),
		}
		SortSurrogateKeys(surrogateKeys)
		require.Equal(t, []string{"subgraph-b", "subgraph-a", "type-b-X", "type-a-X", "zeta", "subgraph-c"}, SurrogateKeyValues(surrogateKeys))
	})

	t.Run("merge dedupes by tier and value and keeps first seen order", func(t *testing.T) {
		require.Equal(t, declared("a", "b", "c"), MergeSurrogateKeys(nil, declared("a", "b"), declared("b", "c"), nil, declared("a")))
		require.Equal(t, declared("a", "b"), MergeSurrogateKeys(declared("a"), declared("a", "b")))
		require.Nil(t, MergeSurrogateKeys(nil, nil, []SurrogateKey{}))
		require.Equal(t,
			[]SurrogateKey{SubgraphSurrogateKey("a"), DeclaredSurrogateKey("subgraph-a")},
			MergeSurrogateKeys(nil, []SurrogateKey{SubgraphSurrogateKey("a")}, declared("subgraph-a")))
	})

	t.Run("values dedupe across tiers", func(t *testing.T) {
		require.Equal(t, []string{"subgraph-a", "b"}, SurrogateKeyValues([]SurrogateKey{SubgraphSurrogateKey("a"), DeclaredSurrogateKey("subgraph-a"), DeclaredSurrogateKey("b")}))
		require.Nil(t, SurrogateKeyValues(nil))
	})
}
