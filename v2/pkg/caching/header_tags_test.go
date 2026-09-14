package caching

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Emitted verbatim in a header a CDN purges by, so the exact string is the contract.
func TestHeaderTags(t *testing.T) {
	t.Run("derived headerTags carry their tier prefix", func(t *testing.T) {
		require.Equal(t, "subgraph-accounts", SubgraphHeaderTag("accounts"))
		require.Equal(t, "type-accounts-User", TypeHeaderTag("accounts", "User"))
	})

	t.Run("a declared tag is visible ASCII and never a derived tier", func(t *testing.T) {
		for _, tag := range []string{"users", "user-42", "a/b:c", "subgraph_x", "typed"} {
			require.True(t, ValidDeclaredHeaderTag(tag), tag)
		}
		for _, tag := range []string{"", "subgraph-accounts", "type-accounts-User", "subgraph-", "type-",
			"a b", "a\tb", "a\nb", "a\x00b", "a\x7fb", "\u00fcber", "用户"} {
			require.False(t, ValidDeclaredHeaderTag(tag), "%q", tag)
		}
	})

	t.Run("sort groups by tier and keeps the order within one", func(t *testing.T) {
		headerTags := []string{"zeta", "type-b-X", "subgraph-b", "alpha", "type-a-X", "subgraph-a"}
		SortHeaderTags(headerTags)
		require.Equal(t, []string{"subgraph-b", "subgraph-a", "type-b-X", "type-a-X", "zeta", "alpha"}, headerTags)
	})

	t.Run("merge dedupes and keeps first seen order", func(t *testing.T) {
		require.Equal(t, []string{"a", "b", "c"}, MergeHeaderTags(nil, []string{"a", "b"}, []string{"b", "c"}, nil, []string{"a"}))
		require.Equal(t, []string{"a", "b"}, MergeHeaderTags([]string{"a"}, []string{"a", "b"}))
		require.Nil(t, MergeHeaderTags(nil, nil, []string{}))
	})
}
