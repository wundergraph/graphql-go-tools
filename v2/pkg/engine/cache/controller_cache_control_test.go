package cache

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// cacheControlHeader builds the response header a subgraph answered with; ""
// means the subgraph sent none.
func cacheControlHeader(value string) http.Header {
	if value == "" {
		return nil
	}
	return http.Header{"Cache-Control": []string{value}}
}

// privateObserver records the uncacheable-private hints and the discarded
// scope mismatches, and nothing else.
type privateObserver struct {
	noopObserver

	hints           []UncacheablePrivate
	scopeMismatches []ScopeMismatch
}

func (o *privateObserver) OnUncacheablePrivate(subgraph string, reason string) {
	o.hints = append(o.hints, UncacheablePrivate{Subgraph: subgraph, Reason: reason})
}

func (o *privateObserver) OnScopeMismatch(subgraph string, storedScope string) {
	o.scopeMismatches = append(o.scopeMismatches, ScopeMismatch{Subgraph: subgraph, StoredScope: storedScope})
}

// fetchWithCacheControl runs one miss -> fetch-result -> flush cycle whose
// subgraph response carried the given Cache-Control value, and returns the
// handle so rows can read the key both layers were written under.
func fetchWithCacheControl(t *testing.T, rc resolve.RequestCache, cfg *resolve.FetchCacheConfig, item *astjson.Value, responseData, cacheControlValue string) *resolve.FetchCacheHandle {
	t.Helper()
	decision, handle := prepare(t, rc, cfg, item)
	require.Equal(t, resolve.DecisionFetch, decision)
	require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
		Items:           []*astjson.Value{item},
		ResponseData:    astjson.MustParseBytes([]byte(responseData)),
		ResponseHeaders: cacheControlHeader(cacheControlValue),
		Arena:           beginner(),
	}))
	rc.EndRequest()
	return handle
}

// TestControllerCacheControlTTLRows drives the two-tier TTL ladder through the
// real write path: what the store entry is written with and what the envelope
// records.
func TestControllerCacheControlTTLRows(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("header max-age wins over the configured TTL", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, 5*time.Minute)
			rc := NewController(store, nil).BeginRequest(nil)
			handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "max-age=60")

			key := handle.Items[0].RenderedKey
			// The envelope's cc.ttl and the store item's TTL both carry the
			// resolved 60s, not the configured 5m.
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`,
							TTL:   time.Minute,
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

	t.Run("a silent header leaves the configured TTL in charge", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, 5*time.Minute)
			rc := NewController(store, nil).BeginRequest(nil)
			handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "")

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":300,"created":946684800,"scope":"public"}}`,
							TTL:   5 * time.Minute,
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

	t.Run("a malformed max-age falls through to the configured TTL", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, 5*time.Minute)
			rc := NewController(store, nil).BeginRequest(nil)
			handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "max-age=whenever")

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":300,"created":946684800,"scope":"public"}}`,
							TTL:   5 * time.Minute,
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

	t.Run("max-age zero writes nothing to the store but still feeds L1", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, 5*time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)
		handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "max-age=0")

		key := handle.Items[0].RenderedKey
		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
		}, store.ops)
		// L1 is request-lifetime and untouched by a merely-zero TTL, so a
		// sibling fetch of the same entity still serves without a network hop.
		decision, _ := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	})

	t.Run("MaxTTL clamps the header tier", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			cfg.MaxTTL = 30 * time.Second
			rc := NewController(store, nil).BeginRequest(nil)
			handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "max-age=600")

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":30,"created":946684800,"scope":"public"}}`,
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
		})
	})

	t.Run("MaxTTL clamps the static tier", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, 10*time.Minute)
			cfg.MaxTTL = time.Minute
			rc := NewController(store, nil).BeginRequest(nil)
			handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "")

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`,
							TTL:   time.Minute,
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

	t.Run("a root field takes its TTL from the header too", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := rootFieldConfig(5 * time.Minute)
			rc := NewController(store, nil).BeginRequest(nil)
			item := rootItem()
			decision, handle := prepare(t, rc, cfg, item)
			require.Equal(t, resolve.DecisionFetch, decision)
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:           []*astjson.Value{item},
				ResponseData:    astjson.MustParseBytes([]byte(`{"products":[{"name":"Table"}]}`)),
				ResponseHeaders: cacheControlHeader("max-age=90"),
				Arena:           beginner(),
			}))
			rc.EndRequest()

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"products":[{"name":"Table"}]},"cc":{"ttl":90,"created":946684800,"scope":"public"}}`,
							TTL:   90 * time.Second,
							Tags: []string{
								"subgraph:products",
								"type:products:Query", // a root-field entry is not one entity: no entity tag
							},
						},
					},
				},
			}, store.ops)
		})
	})

	t.Run("ONE header governs every entity entry written from a batch response", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, 5*time.Minute)
			rc := NewController(store, nil).BeginRequest(nil)
			buckets := [][]*astjson.Value{
				{productItem(t, "1")},
				{productItem(t, "2")},
			}
			decision, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
			require.Equal(t, resolve.DecisionFetch, decision)
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				BatchStats:      buckets,
				ResponseData:    astjson.MustParseBytes([]byte(`[{"__typename":"Product","name":"Table","price":100},{"__typename":"Product","name":"Chair","price":50}]`)),
				ResponseHeaders: cacheControlHeader("max-age=45"),
				Arena:           beginner(),
			}))
			rc.EndRequest()

			// The batch is ONE HTTP response, so its single Cache-Control drives
			// both entries' TTLs.
			assert.Equal(t, []testStoreOp{
				{
					Kind: "GetMany",
					Keys: []string{
						handle.Items[0].RenderedKey,
						handle.Items[1].RenderedKey,
					},
					Hits: []bool{false, false},
				},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   handle.Items[0].RenderedKey,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":45,"created":946684800,"scope":"public"}}`,
							TTL:   45 * time.Second,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:d3cc039c7a9789e7", // upc "1"
							},
						},
						{
							Key:   handle.Items[1].RenderedKey,
							Value: `{"data":{"__typename":"Product","name":"Chair","price":50},"cc":{"ttl":45,"created":946684800,"scope":"public"}}`,
							TTL:   45 * time.Second,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:4f93140518d68e67", // upc "2"
							},
						},
					},
				},
			}, store.ops)
		})
	})
}

// TestControllerCacheControlStorabilityRows covers the directives that decide
// WHETHER a result is kept at all: no-store, no-cache, and runtime private.
func TestControllerCacheControlStorabilityRows(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("no-store writes to NEITHER layer", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, 5*time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)
		handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "no-store")

		// The lookup happened; nothing was written back.
		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{handle.Items[0].RenderedKey}, Hits: []bool{false}},
		}, store.ops)
		assert.Equal(t, map[string]*astjson.Value(nil), rc.(*requestCache).l1)
		// And the request-lifetime layer really is empty: a sibling fetch of the
		// same entity misses and goes to the network.
		decision, _ := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
	})

	t.Run("no-cache is treated exactly like no-store", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, 5*time.Minute)
		rc := NewController(store, nil).BeginRequest(nil)
		handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "no-cache")

		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{handle.Items[0].RenderedKey}, Hits: []bool{false}},
		}, store.ops)
		assert.Equal(t, map[string]*astjson.Value(nil), rc.(*requestCache).l1)
		decision, _ := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
	})

	t.Run("a runtime-private result skips the store, keeps L1, and reports once", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := entityConfig(t, 5*time.Minute)
		rc := NewController(store, obs).BeginRequest(nil)
		handle := fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "private, max-age=60")

		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{handle.Items[0].RenderedKey}, Hits: []bool{false}},
		}, store.ops)
		// The value is per-requester and the request IS the requester, so L1
		// keeps it — and one hint names the subgraph that made it uncacheable.
		assert.Equal(t, []UncacheablePrivate{
			{Subgraph: "products", Reason: UncacheablePrivateResponseHeader},
		}, obs.hints)
		decision, _ := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	})

	t.Run("a public result reports no private hint", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := entityConfig(t, 5*time.Minute)
		rc := NewController(store, obs).BeginRequest(nil)
		fetchWithCacheControl(t, rc, cfg, productItem(t, "1"), fresh, "public, max-age=60")

		assert.Equal(t, []UncacheablePrivate(nil), obs.hints)
	})
}

// TestControllerCacheControlNegativeRows: the empty-entity sentinel runs the
// same storability pipeline as a value, but never takes its lifetime from
// max-age.
func TestControllerCacheControlNegativeRows(t *testing.T) {
	sentinelFetch := func(t *testing.T, rc resolve.RequestCache, cfg *resolve.FetchCacheConfig, cacheControlValue string) *resolve.FetchCacheHandle {
		t.Helper()
		item := productItem(t, "404")
		_, handle := prepare(t, rc, cfg, item)
		in := emptyEntityInput(item)
		in.ResponseHeaders = cacheControlHeader(cacheControlValue)
		require.NoError(t, rc.OnFetchResult(handle, in))
		rc.EndRequest()
		return handle
	}

	t.Run("max-age does NOT extend the sentinel: NegativeCacheTTL still decides", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := negativeConfig(t, 5*time.Second)
			rc := NewController(store, nil).BeginRequest(nil)
			handle := sentinelFetch(t, rc, cfg, "max-age=3600")

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":null,"cc":{"ttl":5,"created":946684800,"scope":"public"}}`,
							TTL:   5 * time.Second,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:0256cf786a3c1be7", // upc "404": the sentinel is tagged like a value
							},
						},
					},
				},
			}, store.ops)
		})
	})

	t.Run("MaxTTL clamps the sentinel lifetime", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := negativeConfig(t, time.Hour)
			cfg.MaxTTL = time.Minute
			rc := NewController(store, nil).BeginRequest(nil)
			handle := sentinelFetch(t, rc, cfg, "")

			key := handle.Items[0].RenderedKey
			assert.Equal(t, []testStoreOp{
				{Kind: "GetMany", Keys: []string{key}, Hits: []bool{false}},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":null,"cc":{"ttl":60,"created":946684800,"scope":"public"}}`,
							TTL:   time.Minute,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:0256cf786a3c1be7", // upc "404"
							},
						},
					},
				},
			}, store.ops)
		})
	})

	t.Run("no-store suppresses the sentinel in BOTH layers", func(t *testing.T) {
		store := newTestStore()
		cfg := negativeConfig(t, 5*time.Second)
		rc := NewController(store, nil).BeginRequest(nil)
		handle := sentinelFetch(t, rc, cfg, "no-store")

		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{handle.Items[0].RenderedKey}, Hits: []bool{false}},
		}, store.ops)
		assert.Equal(t, map[string]*astjson.Value(nil), rc.(*requestCache).l1)
	})

	t.Run("a private sentinel skips the store but stays known-missing in L1", func(t *testing.T) {
		store := newTestStore()
		obs := &privateObserver{}
		cfg := negativeConfig(t, 5*time.Second)
		rc := NewController(store, obs).BeginRequest(nil)
		handle := sentinelFetch(t, rc, cfg, "private")

		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{handle.Items[0].RenderedKey}, Hits: []bool{false}},
		}, store.ops)
		assert.Equal(t, []UncacheablePrivate{
			{Subgraph: "products", Reason: UncacheablePrivateResponseHeader},
		}, obs.hints)
		// Nonexistence is a fact WITHIN the request, so the L1 sentinel serves a
		// sibling lookup for the same entity.
		_, second := prepare(t, rc, cfg, productItem(t, "404"))
		require.NotNil(t, second)
		assert.True(t, second.Items[0].NegativeHit)
	})
}

// TestControllerCacheControlExpiryRows: the resolved TTL is what the entry
// actually lives by, verified by sleeping past it in a synctest bubble.
func TestControllerCacheControlExpiryRows(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("an entry written from max-age expires when that max-age elapses", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Hour)
			fetchWithCacheControl(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), fresh, "max-age=2")

			// Still fresh one second in, despite the configured hour being far
			// from over.
			time.Sleep(time.Second)
			decision, _ := prepare(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"))
			assert.Equal(t, resolve.DecisionSkipFullHit, decision)

			time.Sleep(2 * time.Second)
			decision, _ = prepare(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"))
			assert.Equal(t, resolve.DecisionFetch, decision)
		})
	})

	t.Run("a clamped entry expires at MaxTTL, not at the header value", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Hour)
			cfg.MaxTTL = 10 * time.Second
			fetchWithCacheControl(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), fresh, "max-age=600")

			time.Sleep(9 * time.Second)
			decision, _ := prepare(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"))
			assert.Equal(t, resolve.DecisionSkipFullHit, decision)

			// Past the clamp but far inside the header's 600s.
			time.Sleep(2 * time.Second)
			decision, _ = prepare(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"))
			assert.Equal(t, resolve.DecisionFetch, decision)
		})
	})
}
