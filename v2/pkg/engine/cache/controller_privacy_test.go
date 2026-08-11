package cache

import (
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// partitionProvider is the integrator hook double: one identity per subgraph
// plus a call counter, so rows can pin that the hook runs at most once per
// subgraph per request. A subgraph without an entry answers ok=false.
type partitionProvider struct {
	identities map[string]string
	calls      int
}

func (p *partitionProvider) PrivatePartition(_ *resolve.Context, subgraphName string) (string, bool) {
	p.calls++
	identity, ok := p.identities[subgraphName]
	return identity, ok
}

// flippingProvider hands out a NEW identity on every call, so rows can prove
// the request cache pins the first one.
type flippingProvider struct {
	calls int
}

func (p *flippingProvider) PrivatePartition(*resolve.Context, string) (string, bool) {
	p.calls++
	return "identity-" + strconv.Itoa(p.calls), true
}

// privateContext is a request context whose private cache entries are
// partitioned by the given provider (nil = no provider wired).
func privateContext(t *testing.T, provider resolve.PrivatePartitionProvider) *resolve.Context {
	t.Helper()
	ctx := resolve.NewContext(t.Context())
	if provider != nil {
		ctx.SetPrivatePartitionProvider(provider)
	}
	return ctx
}

// privateEntityConfig is the shared entity config declared PRIVATE.
func privateEntityConfig(t *testing.T, ttl time.Duration) *resolve.FetchCacheConfig {
	t.Helper()
	cfg := entityConfig(t, ttl)
	cfg.Private = true
	return cfg
}

// prepareWithHeaderHash runs one PrepareFetch for an item whose subgraph
// request carries the given forwarded-header hash.
func prepareWithHeaderHash(rc resolve.RequestCache, cfg *resolve.FetchCacheConfig, headerHash uint64, item *astjson.Value) (resolve.Decision, *resolve.FetchCacheHandle) {
	in := prepareInput(cfg, item)
	in.HeaderHash = headerHash
	return rc.PrepareFetch(in)
}

// TestControllerPrivatePartitionSources covers where the requester identity
// comes from and what happens when there is none.
func TestControllerPrivatePartitionSources(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("the provider identity keys the entry, and the envelope records the scope", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := privateEntityConfig(t, time.Minute)
			ctx := privateContext(t, &partitionProvider{identities: map[string]string{"products": "user-a"}})
			rc := NewController(store, nil).BeginRequest(ctx)
			handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "")

			key := handle.Items[0].RenderedKey
			assert.Equal(t,
				"v1:products:"+hashHex([]byte(`v1:products:{"__typename":"Product","representation":{"upc":"1"},"partition":"`+sha256Hex("i:user-a")+`"}`)),
				key)
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"private"}}`,
							TTL:   time.Minute,
							// A private entry's tags are what a public entry of
							// the same entity would carry — no identity material
							// anywhere — so one purge reaches every partition.
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:d3cc039c7a9789e7", // upc "1"
							},
						},
					},
				},
			}, store.ops)
		})
	})

	t.Run("without a provider the forwarded-header hash is the identity", func(t *testing.T) {
		store := newTestStore()
		cfg := privateEntityConfig(t, time.Minute)
		cfg.IncludeSubgraphHeaders = true
		rc := NewController(store, nil).BeginRequest(privateContext(t, nil))
		_, handle := prepareWithHeaderHash(rc, cfg, 42, productItem(t, "1"))
		require.NotNil(t, handle)

		assert.Equal(t,
			"v1:products:"+hashHex([]byte(`v1:products:{"__typename":"Product","representation":{"upc":"1"},"partition":"`+sha256Hex("h:"+hex64(42))+`"}`)),
			handle.Items[0].RenderedKey)
	})

	t.Run("the provider wins over the header hash", func(t *testing.T) {
		store := newTestStore()
		cfg := privateEntityConfig(t, time.Minute)
		cfg.IncludeSubgraphHeaders = true
		ctx := privateContext(t, &partitionProvider{identities: map[string]string{"products": "user-a"}})
		rc := NewController(store, nil).BeginRequest(ctx)
		_, handle := prepareWithHeaderHash(rc, cfg, 42, productItem(t, "1"))
		require.NotNil(t, handle)

		assert.Equal(t,
			"v1:products:"+hashHex([]byte(`v1:products:{"__typename":"Product","representation":{"upc":"1"},"partition":"`+sha256Hex("i:user-a")+`"}`)),
			handle.Items[0].RenderedKey)
	})

	t.Run("an empty provider value is no identity at all", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := privateEntityConfig(t, time.Minute)
		// The provider answers ok=true with an empty identity — every requester
		// it cannot name would otherwise share one partition.
		ctx := privateContext(t, &partitionProvider{identities: map[string]string{"products": ""}})
		rc := NewController(store, obs).BeginRequest(ctx)
		fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "")

		assert.Equal(t, []testStoreOp(nil), store.ops)
		assert.Equal(t, []UncacheablePrivate{
			{Subgraph: "products", Reason: UncacheablePrivateNoIdentity},
		}, obs.hints)
	})

	t.Run("the provider is consulted once per subgraph per request", func(t *testing.T) {
		store := newTestStore()
		cfg := privateEntityConfig(t, time.Minute)
		provider := &partitionProvider{identities: map[string]string{"products": "user-a"}}
		rc := NewController(store, nil).BeginRequest(privateContext(t, provider))
		// Three fetches of the same subgraph: the hook is an integrator callback
		// (JWT parse, tenant lookup) and must not run per fetch.
		_, first := prepare(t, rc, cfg, productItem(t, "1"))
		_, second := prepare(t, rc, cfg, productItem(t, "2"))
		_, third := prepare(t, rc, cfg, productItem(t, "3"))
		require.NotNil(t, first)
		require.NotNil(t, second)
		require.NotNil(t, third)

		assert.Equal(t, 1, provider.calls)
	})

	t.Run("one request keys under ONE identity even if the provider changes its mind", func(t *testing.T) {
		store := newTestStore()
		cfg := privateEntityConfig(t, time.Minute)
		// A provider that answers differently on every call is the structural
		// worst case: the request must still hold exactly one partition, which is
		// what makes the UNPARTITIONED L1 key of every private value safe.
		flipping := &flippingProvider{}
		rc := NewController(store, nil).BeginRequest(privateContext(t, flipping))
		_, first := prepare(t, rc, cfg, productItem(t, "1"))
		_, second := prepare(t, rc, cfg, productItem(t, "2"))
		require.NotNil(t, first)
		require.NotNil(t, second)

		firstPartition := sha256Hex("i:identity-1")
		assert.Equal(t,
			"v1:products:"+hashHex([]byte(`v1:products:{"__typename":"Product","representation":{"upc":"1"},"partition":"`+firstPartition+`"}`)),
			first.Items[0].RenderedKey)
		assert.Equal(t,
			"v1:products:"+hashHex([]byte(`v1:products:{"__typename":"Product","representation":{"upc":"2"},"partition":"`+firstPartition+`"}`)),
			second.Items[0].RenderedKey)
	})
}

// TestControllerPrivateWithoutIdentity is the no-identity rule: the store is
// skipped ENTIRELY — reads, value writes, and the negative sentinel — while the
// request-lifetime layer keeps working.
func TestControllerPrivateWithoutIdentity(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("no store op at all, and the fetch still serves its sibling from L1", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := privateEntityConfig(t, time.Minute)
		rc := NewController(store, obs).BeginRequest(privateContext(t, nil))
		handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "")

		// No key was derived, so nothing could be read or written.
		assert.Equal(t, "", handle.Items[0].RenderedKey)
		assert.Equal(t, []testStoreOp(nil), store.ops)
		// L1 keyed the item all the same: a sibling fetch of the same entity
		// serves without a network hop.
		decision, sibling := prepare(t, rc, cfg, productItem(t, "1"))
		require.NotNil(t, sibling)
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, `products:{"__typename":"Product","representation":{"upc":"1"}}`, sibling.Items[0].L1Key)
		// One hint per fetch that lost its store access — two fetches here.
		assert.Equal(t, []UncacheablePrivate{
			{Subgraph: "products", Reason: UncacheablePrivateNoIdentity},
			{Subgraph: "products", Reason: UncacheablePrivateNoIdentity},
		}, obs.hints)
	})

	t.Run("the negative sentinel is skipped too, and stays known-missing in L1", func(t *testing.T) {
		store := newTestStore()
		cfg := privateEntityConfig(t, time.Minute)
		cfg.NegativeCacheTTL = 5 * time.Second
		rc := NewController(store, nil).BeginRequest(privateContext(t, nil))
		item := productItem(t, "404")
		_, handle := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, emptyEntityInput(item)))
		rc.EndRequest()

		// A "this entity does not exist" answer can be requester-specific, so it
		// never reaches a shared keyspace either.
		assert.Equal(t, []testStoreOp(nil), store.ops)
		_, sibling := prepare(t, rc, cfg, productItem(t, "404"))
		require.NotNil(t, sibling)
		assert.True(t, sibling.Items[0].NegativeHit)
	})

	t.Run("a private root field has no cache at all without an identity", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := rootFieldConfig(time.Minute)
		cfg.Private = true
		rc := NewController(store, obs).BeginRequest(privateContext(t, nil))
		item := rootItem()
		decision, handle := rc.PrepareFetch(prepareInput(cfg, item))

		// Root fields never carry L1, so nothing is left to prepare.
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
		assert.Equal(t, []testStoreOp(nil), store.ops)
		assert.Equal(t, []UncacheablePrivate{
			{Subgraph: "products", Reason: UncacheablePrivateNoIdentity},
		}, obs.hints)
	})
}

// TestControllerPrivateIsolation is the cross-requester property: two requests
// with different identities never see each other's entries, in either
// direction.
func TestControllerPrivateIsolation(t *testing.T) {
	fresh := func(name string) string {
		return `{"__typename":"Product","name":"` + name + `","price":100}`
	}

	t.Run("two identities warm two entries, each a miss for the other", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := privateEntityConfig(t, time.Minute)

			userA := NewController(store, nil).BeginRequest(privateContext(t, &partitionProvider{identities: map[string]string{"products": "user-a"}}))
			keyA := fetchWithCacheControl(t, userA, cfg, productItem(t, "1"), fresh("Table for A"), "").Items[0].RenderedKey
			store.ops = nil

			// User B asks for the SAME entity: a different key, so a miss and its
			// own write, with user A's value untouched.
			userB := NewController(store, nil).BeginRequest(privateContext(t, &partitionProvider{identities: map[string]string{"products": "user-b"}}))
			keyB := fetchWithCacheControl(t, userB, cfg, productItem(t, "1"), fresh("Table for B"), "").Items[0].RenderedKey
			assert.NotEqual(t, keyA, keyB)
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{keyB}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   keyB,
							Value: `{"data":{"__typename":"Product","name":"Table for B","price":100},"cc":{"ttl":60,"created":946684800,"scope":"private"}}`,
							TTL:   time.Minute,
							// Different key, IDENTICAL tags: user B's entry is
							// addressable by the same entity tag as user A's.
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:d3cc039c7a9789e7", // upc "1"
							},
						},
					},
				},
			}, store.ops)

			// Each identity's repeat request hits its OWN entry and reads its OWN
			// value back.
			store.ops = nil
			repeatA := NewController(store, nil).BeginRequest(privateContext(t, &partitionProvider{identities: map[string]string{"products": "user-a"}}))
			decision, handle := prepare(t, repeatA, cfg, productItem(t, "1"))
			assert.Equal(t, resolve.DecisionSkipFullHit, decision)
			require.NotNil(t, handle)
			assert.Equal(t, fresh("Table for A"), string(handle.Items[0].FromCache.MarshalTo(nil)))
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{keyA}, Hits: []bool{true}},
			}, store.ops)
		})
	})

	t.Run("two header-hash identities are two entries as well", func(t *testing.T) {
		store := newTestStore()
		cfg := privateEntityConfig(t, time.Minute)
		cfg.IncludeSubgraphHeaders = true

		first := NewController(store, nil).BeginRequest(privateContext(t, nil))
		_, firstHandle := prepareWithHeaderHash(first, cfg, 1, productItem(t, "1"))
		require.NotNil(t, firstHandle)
		second := NewController(store, nil).BeginRequest(privateContext(t, nil))
		_, secondHandle := prepareWithHeaderHash(second, cfg, 2, productItem(t, "1"))
		require.NotNil(t, secondHandle)

		assert.NotEqual(t, firstHandle.Items[0].RenderedKey, secondHandle.Items[0].RenderedKey)
	})
}

// TestControllerScopeMismatch is the defense-in-depth read guard: an entry
// carrying the other privacy scope was written by a differently-configured
// deployment and is discarded as a miss, in BOTH directions.
func TestControllerScopeMismatch(t *testing.T) {
	privateEnvelope := `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"private"}}`
	publicEnvelope := `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`

	t.Run("a private entry found on a public path is never served", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := entityConfig(t, time.Minute)
		// Config drift: the subgraph was PRIVATE when this entry was written and
		// is PUBLIC now, so the entry sits under a key the public path derives.
		_, seeding := prepare(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"))
		require.NotNil(t, seeding)
		key := seeding.Items[0].RenderedKey
		store.data[key] = testStoreEntry{value: []byte(privateEnvelope), expiresAt: time.Now().Add(time.Hour)}
		store.ops = nil

		decision, handle := prepare(t, NewController(store, obs).BeginRequest(nil), cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
		require.NotNil(t, handle)
		assert.Nil(t, handle.Items[0].FromCache)
		// The entry WAS read and hit — the scope guard, not the lookup, rejected
		// it, and the fetch falls back to the origin.
		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{key}, Hits: []bool{true}},
		}, store.ops)
		assert.Equal(t, []ScopeMismatch{
			{Subgraph: "products", StoredScope: "private"},
		}, obs.scopeMismatches)
	})

	t.Run("a public entry found on a private path is never served either", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := privateEntityConfig(t, time.Minute)
		ctx := privateContext(t, &partitionProvider{identities: map[string]string{"products": "user-a"}})
		_, seeding := prepare(t, NewController(store, nil).BeginRequest(ctx), cfg, productItem(t, "1"))
		require.NotNil(t, seeding)
		key := seeding.Items[0].RenderedKey
		store.data[key] = testStoreEntry{value: []byte(publicEnvelope), expiresAt: time.Now().Add(time.Hour)}
		store.ops = nil

		decision, handle := prepare(t, NewController(store, obs).BeginRequest(ctx), cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
		require.NotNil(t, handle)
		assert.Nil(t, handle.Items[0].FromCache)
		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{key}, Hits: []bool{true}},
		}, store.ops)
		assert.Equal(t, []ScopeMismatch{
			{Subgraph: "products", StoredScope: "public"},
		}, obs.scopeMismatches)
	})

	t.Run("a private negative sentinel on a public path is discarded, not served as missing", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := entityConfig(t, time.Minute)
		_, seeding := prepare(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "404"))
		require.NotNil(t, seeding)
		key := seeding.Items[0].RenderedKey
		store.data[key] = testStoreEntry{
			value:     []byte(`{"data":null,"cc":{"ttl":5,"created":946684800,"scope":"private"}}`),
			expiresAt: time.Now().Add(time.Hour),
		}
		store.ops = nil

		decision, handle := prepare(t, NewController(store, obs).BeginRequest(nil), cfg, productItem(t, "404"))
		assert.Equal(t, resolve.DecisionFetch, decision)
		require.NotNil(t, handle)
		assert.False(t, handle.Items[0].NegativeHit)
		assert.Equal(t, []ScopeMismatch{
			{Subgraph: "products", StoredScope: "private"},
		}, obs.scopeMismatches)
	})

	t.Run("a private root-field entry on a public path is discarded", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := rootFieldConfig(time.Minute)
		ctx := variableContext(t, `{"first":1}`)
		key := rootFieldCacheKey(cfg, 0, ctx, "")
		store.data[key] = testStoreEntry{
			value:     []byte(`{"data":{"products":[{"name":"Table"}]},"cc":{"ttl":60,"created":946684800,"scope":"private"}}`),
			expiresAt: time.Now().Add(time.Hour),
		}

		decision, handle := NewController(store, obs).BeginRequest(ctx).PrepareFetch(prepareInput(cfg, rootItem()))
		assert.Equal(t, resolve.DecisionFetch, decision)
		require.NotNil(t, handle)
		assert.Nil(t, handle.Items[0].FromCache)
		assert.Equal(t, []ScopeMismatch{
			{Subgraph: "products", StoredScope: "private"},
		}, obs.scopeMismatches)
	})
}

// TestControllerPrivateHeaderInterplay: on a statically-private fetch the
// runtime directives meet a keyspace that is ALREADY partitioned.
func TestControllerPrivateHeaderInterplay(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("a private response header confirms the declaration and writes anyway", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			obs := &privateObserver{}
			cfg := privateEntityConfig(t, time.Minute)
			ctx := privateContext(t, &partitionProvider{identities: map[string]string{"products": "user-a"}})
			rc := NewController(store, obs).BeginRequest(ctx)
			handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "private, max-age=30")

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":30,"created":946684800,"scope":"private"}}`,
							TTL:   30 * time.Second,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:d3cc039c7a9789e7", // upc "1"
							},
						},
					},
				},
			}, store.ops)
			// Nothing to report: the configuration already caches this
			// partitioned, which is exactly what the header asks for.
			assert.Equal(t, []UncacheablePrivate(nil), obs.hints)
		})
	})

	t.Run("no-store still kills the write of a partitioned entry", func(t *testing.T) {
		store := newTestStore()
		cfg := privateEntityConfig(t, time.Minute)
		ctx := privateContext(t, &partitionProvider{identities: map[string]string{"products": "user-a"}})
		rc := NewController(store, nil).BeginRequest(ctx)
		handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "no-store")

		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{handle.Items[0].RenderedKey}, Hits: []bool{false}},
		}, store.ops)
	})
}
