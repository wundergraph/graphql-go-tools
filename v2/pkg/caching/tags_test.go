package caching

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The router builds these to look an index up by, and the engine builds them to
// write it, from separate modules. What they have to agree on is the exact
// string, so that is what is asserted rather than the shape of it.
func TestTags(t *testing.T) {
	t.Run("each kind carries its own namespace", func(t *testing.T) {
		require.Equal(t, "subgraph:accounts", SubgraphTag("accounts"))
		require.Equal(t, "type:accounts:User", TypeTag("accounts", "User"))
		require.Equal(t, "declared:accounts:user-42", DeclaredTag("accounts", "user-42"))
	})

	t.Run("a subgraph's tags cannot reach another subgraph's", func(t *testing.T) {
		require.NotEqual(t, TypeTag("accounts", "User"), TypeTag("products", "User"))
		require.NotEqual(t, DeclaredTag("accounts", "shared"), DeclaredTag("products", "shared"))
	})

	t.Run("a declared tag cannot spell its way into a derived namespace", func(t *testing.T) {
		// The namespace is applied to what the subgraph said rather than taken
		// from it, so a tag named after one of the router's own lands beside it.
		require.Equal(t, "declared:accounts:subgraph:there", DeclaredTag("accounts", "subgraph:there"))
		require.NotEqual(t, SubgraphTag("there"), DeclaredTag("accounts", "subgraph:there"))
	})

	t.Run("a separator in either half keeps the halves apart", func(t *testing.T) {
		// Both spell "a:b:c" unescaped, and an invalidation for one would then
		// drop the other's entries.
		require.NotEqual(t, DeclaredTag("a", "b:c"), DeclaredTag("a:b", "c"))
		require.Equal(t, "declared:a:b:c", DeclaredTag("a", "b:c"))
		require.Equal(t, "declared:a::b:c", DeclaredTag("a:b", "c"))
	})
}
