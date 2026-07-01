package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// rootFieldConfig is a root-field-scope config over Query.products.
func rootFieldConfig(ttl time.Duration) *resolve.FetchCacheConfig {
	cfg := &resolve.FetchCacheConfig{
		L2:        ttl > 0,
		CacheName: "root-fields",
		TTL:       ttl,
		KeySpec: resolve.CacheKeySpec{
			Scope:     resolve.CacheScopeRootField,
			TypeName:  "Query",
			FieldName: "products",
		},
		ProvidesData: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("products"),
					Value: &resolve.Array{
						Nullable: false,
						Path:     []string{"products"},
						Item: &resolve.Object{
							Nullable: false,
							Fields: []*resolve.Field{
								{Name: []byte("name"), Value: &resolve.Scalar{Nullable: false, Path: []string{"name"}}},
							},
						},
					},
				},
			},
		},
	}
	resolve.ComputeHasAliases(cfg.ProvidesData)
	return cfg
}

func rootItem() *astjson.Value {
	return astjson.MustParseBytes([]byte(`{}`))
}

// TestControllerRootFieldRows covers the runtime root-field arms of the D and
// F rows plus the flush and shadow-asymmetry rows.
func TestControllerRootFieldRows(t *testing.T) {
	newRC := func(store Store, ctx *resolve.Context) resolve.RequestCache {
		return NewController(store, nil).BeginRequest(ctx)
	}
	responseData := `{"products":[{"name":"Table"},{"name":"Chair"}]}`

	primeRoot := func(t *testing.T, store *testStore, cfg *resolve.FetchCacheConfig, ctx *resolve.Context, data string) string {
		t.Helper()
		rc := newRC(store, ctx)
		item := rootItem()
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionFetch, decision)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(data)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		return handle.Items[0].RenderedKeys[0]
	}

	t.Run("[D-root] miss then hit: read key == write key, splice serves the whole value", func(t *testing.T) {
		store := newTestStore()
		cfg := rootFieldConfig(time.Minute)
		key := primeRoot(t, store, cfg, nil, responseData)

		rc := newRC(store, nil)
		item := rootItem()
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, []string{key}, handle.Items[0].RenderedKeys)
		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{item},
			Arena: beginner(),
		}))
		assert.Equal(t, responseData, string(item.MarshalTo(nil)))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: key},
			{Kind: "Set", Key: key, Value: responseData, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Get", Key: key},
		}, store.ops)
	})

	t.Run("[D-root] coverage failure: a smaller cached value never serves a bigger selection", func(t *testing.T) {
		store := newTestStore()
		cfg := rootFieldConfig(time.Minute)
		// The cached value lacks the products field entirely.
		primeRoot(t, store, cfg, nil, `{"promotions":[]}`)

		decision, handle := prepare(t, newRC(store, nil), cfg, rootItem())
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.Items[0].FromCache)
		assert.Len(t, handle.Items[0].FromCacheCandidates, 1)
	})

	t.Run("[key] different variables produce different keys; order does not matter", func(t *testing.T) {
		cfg := rootFieldConfig(time.Minute)
		ctxA := variableContext(t, `{"a":1,"b":2}`)
		ctxAReordered := variableContext(t, `{"b":2,"a":1}`)
		ctxB := variableContext(t, `{"a":9,"b":2}`)
		keyA := rootFieldCacheKey(cfg, 0, ctxA)
		keyAReordered := rootFieldCacheKey(cfg, 0, ctxAReordered)
		keyB := rootFieldCacheKey(cfg, 0, ctxB)
		assert.Equal(t, keyA, keyAReordered)
		assert.NotEqual(t, keyA, keyB)
	})

	t.Run("[F-root] the write gate blocks failed fetches", func(t *testing.T) {
		store := newTestStore()
		cfg := rootFieldConfig(time.Minute)
		rc := newRC(store, nil)
		item := rootItem()
		_, handle := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:       []*astjson.Value{item},
			FetchFailed: true,
			Arena:       beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{{Kind: "Get", Key: handle.Items[0].RenderedKeys[0]}}, store.ops)
	})

	t.Run("[H5] root-field shadow: hit force-refetches, ZERO compares, L2 overwritten", func(t *testing.T) {
		store := newTestStore()
		cfg := rootFieldConfig(time.Minute)
		cfg.ShadowMode = true
		key := primeRoot(t, store, cfg, nil, responseData)

		obs := &recordingShadowObserver{}
		rc := NewController(store, obs).BeginRequest(nil)
		item := rootItem()
		decision, handle := prepare(t, rc, cfg, item)
		// The asymmetry: a plain Fetch — no stash, no Shadow flag, so a
		// compare is structurally impossible.
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.False(t, handle.Shadow)
		assert.Nil(t, handle.ShadowStash)

		changed := `{"products":[{"name":"Renamed"}]}`
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(changed)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		assert.Empty(t, obs.compares)
		value, _, ok := store.Get(key)
		require.True(t, ok)
		assert.Equal(t, changed, string(value))
	})

	t.Run("[J] NO-OP vs L2 produce identical data through the splice", func(t *testing.T) {
		store := newTestStore()
		cfg := rootFieldConfig(time.Minute)
		primeRoot(t, store, cfg, nil, responseData)

		// L2: served from cache.
		rc := newRC(store, nil)
		cachedItem := rootItem()
		decision, handle := prepare(t, rc, cfg, cachedItem)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{Items: []*astjson.Value{cachedItem}, Arena: beginner()}))

		// NO-OP (nil config): the loader would merge the network response; the
		// resulting item data is identical.
		networkItem := rootItem()
		tx := beginner().Begin()
		_, err := tx.MergeValues(networkItem, astjson.MustParseBytes([]byte(responseData)))
		tx.Commit()
		require.NoError(t, err)
		assert.Equal(t, string(networkItem.MarshalTo(nil)), string(cachedItem.MarshalTo(nil)))
	})
}
