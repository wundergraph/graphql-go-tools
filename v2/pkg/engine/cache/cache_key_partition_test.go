package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// partitionTemplate builds the key template of a Product fetch under the given
// privacy/header configuration and store access, exactly as PrepareFetch does.
func partitionTemplate(t *testing.T, private, includeHeaders bool, headerHash uint64, access l2Access) cacheKeyTemplate {
	t.Helper()
	template, ok := newCacheKeyTemplate(nil, &resolve.FetchCacheConfig{
		SubgraphName:           "products",
		Private:                private,
		IncludeSubgraphHeaders: includeHeaders,
		KeySpec: resolve.CacheKeySpec{
			Scope:          resolve.CacheScopeEntity,
			TypeName:       "Product",
			Representation: productRepresentation(t, "upc"),
		},
	}, headerHash, access)
	require.True(t, ok)
	return template
}

// TestCacheKeyPartitionMatrix walks scope x identity source x header inclusion
// and pins what each combination puts in the key: a partition segment in the L2
// preimage, a header hash in the visible prefix, or — for every public fetch —
// neither.
func TestCacheKeyPartitionMatrix(t *testing.T) {
	item := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`))
	body := `{"__typename":"Product","representation":{"upc":"1"}}`
	partition := sha256Hex("i:user-a")

	t.Run("public without header inclusion: the pre-privacy key, byte for byte", func(t *testing.T) {
		keys, ok := partitionTemplate(t, false, false, 42, l2Access{enabled: true}).render(item)
		require.True(t, ok)
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1"}}`,
			L2: "v1:products:f5d10aa05957718e",
		}, keys)
		// The literal above is the derivation spelled out, so a silent change to
		// either the preimage or the digest fails this row.
		assert.Equal(t, "v1:products:"+hashHex([]byte("v1:products:"+body)), keys.L2)
	})

	t.Run("public with header inclusion: the hash varies the visible prefix", func(t *testing.T) {
		keys, ok := partitionTemplate(t, false, true, 42, l2Access{enabled: true}).render(item)
		require.True(t, ok)
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1"}}`,
			L2: "v1:products:h" + hex64(42) + ":" + hashHex([]byte("v1:products:h"+hex64(42)+":"+body)),
		}, keys)
	})

	t.Run("private with an identity: the segment joins the L2 preimage only", func(t *testing.T) {
		keys, ok := partitionTemplate(t, true, false, 42, l2Access{enabled: true, partition: partition}).render(item)
		require.True(t, ok)
		assert.Equal(t, itemKeys{
			// The L1 key is the public one: the request-lifetime map is
			// per-requester already.
			L1: `products:{"__typename":"Product","representation":{"upc":"1"}}`,
			L2: "v1:products:" + hashHex([]byte(`v1:products:{"__typename":"Product","representation":{"upc":"1"},"partition":"`+partition+`"}`)),
		}, keys)
	})

	t.Run("private with header inclusion: partitioned ONCE, never prefix and segment both", func(t *testing.T) {
		template := partitionTemplate(t, true, true, 42, l2Access{enabled: true, partition: partition})
		// The header hash is the IDENTITY here, so it reaches the key through the
		// partition segment and leaves the visible prefix plain.
		assert.Equal(t, "v1:products", template.prefix)
		keys, ok := template.render(item)
		require.True(t, ok)
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1"}}`,
			L2: "v1:products:" + hashHex([]byte(`v1:products:{"__typename":"Product","representation":{"upc":"1"},"partition":"`+partition+`"}`)),
		}, keys)
	})

	t.Run("private without store access: L1 only, and the L1 key is unchanged", func(t *testing.T) {
		keys, ok := partitionTemplate(t, true, true, 42, l2Access{}).render(item)
		require.True(t, ok)
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1"}}`,
		}, keys)
	})

	t.Run("two identities are two entries, and neither is the public one", func(t *testing.T) {
		userA, ok := partitionTemplate(t, true, false, 0, l2Access{enabled: true, partition: sha256Hex("i:user-a")}).render(item)
		require.True(t, ok)
		userB, ok := partitionTemplate(t, true, false, 0, l2Access{enabled: true, partition: sha256Hex("i:user-b")}).render(item)
		require.True(t, ok)
		public, ok := partitionTemplate(t, false, false, 0, l2Access{enabled: true}).render(item)
		require.True(t, ok)
		assert.NotEqual(t, userA.L2, userB.L2)
		assert.NotEqual(t, userA.L2, public.L2)
		assert.NotEqual(t, userB.L2, public.L2)
		// The shared L1 key is exactly what makes the unpartitioned map safe to
		// reason about: one request only ever holds ONE of these identities.
		assert.Equal(t, userA.L1, userB.L1)
		assert.Equal(t, userA.L1, public.L1)
	})
}

// TestRootFieldCacheKeyPartition pins the root-field arm of the partition
// segment: the same identity trails the coordinate+variables preimage, and a
// public coordinate's key is untouched.
func TestRootFieldCacheKeyPartition(t *testing.T) {
	ctx := resolve.NewContext(t.Context())
	ctx.Variables = astjson.MustParseBytes([]byte(`{"first":5}`))
	partition := sha256Hex("i:user-a")

	t.Run("a public coordinate carries no segment", func(t *testing.T) {
		cfg := &resolve.FetchCacheConfig{
			SubgraphName: "products",
			KeySpec:      resolve.CacheKeySpec{Scope: resolve.CacheScopeRootField, TypeName: "Query", FieldName: "topProducts"},
		}
		assert.Equal(t,
			"v1:products:"+hashHex([]byte(`v1:products:Query.topProducts:{"first":5}`)),
			rootFieldCacheKey(cfg, 0, ctx, ""),
		)
	})

	t.Run("a private coordinate keys under its requester", func(t *testing.T) {
		cfg := &resolve.FetchCacheConfig{
			SubgraphName: "products",
			Private:      true,
			KeySpec:      resolve.CacheKeySpec{Scope: resolve.CacheScopeRootField, TypeName: "Query", FieldName: "me"},
		}
		assert.Equal(t,
			"v1:products:"+hashHex([]byte(`v1:products:Query.me:{"first":5},"partition":"`+partition+`"`)),
			rootFieldCacheKey(cfg, 0, ctx, partition),
		)
	})

	t.Run("the header hash of a private coordinate stays out of the prefix", func(t *testing.T) {
		cfg := &resolve.FetchCacheConfig{
			SubgraphName:           "products",
			Private:                true,
			IncludeSubgraphHeaders: true,
			KeySpec:                resolve.CacheKeySpec{Scope: resolve.CacheScopeRootField, TypeName: "Query", FieldName: "me"},
		}
		assert.Equal(t,
			"v1:products:"+hashHex([]byte(`v1:products:Query.me:{"first":5},"partition":"`+partition+`"`)),
			rootFieldCacheKey(cfg, 42, ctx, partition),
		)
	})
}

// TestPartitionHashing pins how an identity becomes a key segment: sha256 (not
// the 64-bit key hash — a forgeable collision here is a cross-requester leak),
// and each source tagged so one can never be mistaken for the other.
func TestPartitionHashing(t *testing.T) {
	t.Run("the segment is the hex sha256 of the tagged identity", func(t *testing.T) {
		assert.Equal(t,
			"5c10d62d060122fe2e5c2e88d47555725da6e8cc22b609099d3b7e6e07c28bf5",
			sha256Hex("i:user-a"),
		)
		assert.Len(t, sha256Hex("i:user-a"), 64)
	})

	t.Run("a provider identity spelling a header hash lands in a different partition", func(t *testing.T) {
		// The forgery attempt: a provider hands back the exact text the header
		// source would produce for header hash 1.
		forged := "h:" + hex64(1)
		assert.NotEqual(t, sha256Hex("i:"+forged), sha256Hex(forged))
	})
}
