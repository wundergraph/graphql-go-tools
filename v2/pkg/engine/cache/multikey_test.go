package cache

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// multiKeyConfig builds a two-candidate entity config over the shared fixture:
// candidates sorted by selection set — index 0 is "sku", index 1 is "upc".
func multiKeyConfig(t *testing.T) *resolve.FetchCacheConfig {
	t.Helper()
	builder := newKeyBuilder(t, parseKeyBuilderDefinition(t), newKeyBuilderFederation(t, "upc", "sku"))
	spec, ok := builder.buildEntitySpec(productEntityInfo())
	require.True(t, ok)
	require.Len(t, spec.Candidates, 2)
	return &resolve.FetchCacheConfig{
		L1:           true,
		L2:           true,
		CacheName:    "entities",
		TTL:          time.Minute,
		KeySpec:      spec,
		ProvidesData: productProvidesData(),
	}
}

// fullProductItem carries BOTH key fields, so both candidates render.
func fullProductItem(t *testing.T) *astjson.Value {
	t.Helper()
	return astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","sku":"S1"}`))
}

// deriveMultiKeys returns (skuKey, upcKey) for the full item by rendering
// through a throwaway request cache — the same templates production uses.
func deriveMultiKeys(t *testing.T, cfg *resolve.FetchCacheConfig) (string, string) {
	t.Helper()
	rc := NewController(newTestStore(), nil).BeginRequest(nil)
	_, handle := prepare(t, rc, cfg, fullProductItem(t))
	require.NotNil(t, handle)
	require.Len(t, handle.Items[0].RenderedKeys, 2)
	return handle.Items[0].RenderedKeys[0], handle.Items[0].RenderedKeys[1]
}

func newMultiKeyRC(store Store) resolve.RequestCache {
	return NewController(store, nil).BeginRequest(nil)
}

// TestMultiKeyRenderRows covers E1/E3/E4/E7: best-effort rendering into
// RenderedKeys vs PendingCandidates and the write-side refresh/backfill split,
// each with the EXACT ordered store-op log.
func TestMultiKeyRenderRows(t *testing.T) {
	t.Run("[E1] both candidates render; write refreshes both keys", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		skuKey, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		rc := newMultiKeyRC(store)
		item := fullProductItem(t)
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, []string{skuKey, upcKey}, handle.Items[0].RenderedKeys)
		assert.Nil(t, handle.Items[0].PendingCandidates)

		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100}`)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: skuKey},
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: skuKey, Value: `{"__typename":"Product","name":"Table","price":100}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: upcKey, Value: `{"__typename":"Product","name":"Table","price":100}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
		}, store.ops)
	})

	t.Run("[E3] unrenderable candidate backfills from the fresh response", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		skuKey, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		rc := newMultiKeyRC(store)
		// The request item lacks sku: the sku candidate is pending, only upc renders.
		item := productItem(t, "1")
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, []string{upcKey}, handle.Items[0].RenderedKeys)
		require.Len(t, handle.Items[0].PendingCandidates, 1)

		// The FRESH data carries sku, so the pending candidate now renders.
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100,"sku":"S1"}`)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: upcKey, Value: `{"__typename":"Product","name":"Table","price":100,"sku":"S1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: skuKey, Value: `{"__typename":"Product","name":"Table","price":100,"sku":"S1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})

	t.Run("[E4] no candidate renderable: zero lookups, write-only post fetch", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		skuKey, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		rc := newMultiKeyRC(store)
		item := astjson.MustParseBytes([]byte(`{"__typename":"Product"}`))
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.Items[0].RenderedKeys)
		require.Len(t, handle.Items[0].PendingCandidates, 2)

		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100,"sku":"S1","upc":"1"}`)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Set", Key: skuKey, Value: `{"__typename":"Product","name":"Table","price":100,"sku":"S1","upc":"1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonBackfill},
			{Kind: "Set", Key: upcKey, Value: `{"__typename":"Product","name":"Table","price":100,"sku":"S1","upc":"1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})

	t.Run("[E7] single-@key degenerate stays a one-element list", func(t *testing.T) {
		cfg := entityConfig(t, time.Minute)
		store := newTestStore()
		rc := newMultiKeyRC(store)
		_, handle := prepare(t, rc, cfg, productItem(t, "1"))
		require.NotNil(t, handle)
		assert.Len(t, handle.Items[0].RenderedKeys, 1)
		assert.Nil(t, handle.Items[0].PendingCandidates)
	})
}

// TestMultiKeyHitRows covers E2/E5/E6: cross-key hits and the read-hit
// write-back paths through OnFetchSkipped.
func TestMultiKeyHitRows(t *testing.T) {
	t.Run("[E2] hit on the non-primary key serves and backfills the missed key", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cfg := multiKeyConfig(t)
			skuKey, upcKey := deriveMultiKeys(t, cfg)
			store := newTestStore()
			// Only the upc (second) candidate is primed.
			store.seed(upcKey, []byte(`{"__typename":"Product","name":"Table","price":100}`), 30*time.Second)

			rc := newMultiKeyRC(store)
			item := fullProductItem(t)
			decision, handle := prepare(t, rc, cfg, item)
			require.Equal(t, resolve.DecisionSkipFullHit, decision)
			assert.True(t, handle.MustWriteBack)
			require.Len(t, handle.Items, 1)
			assert.Equal(t, 30*time.Second, handle.Items[0].SelectedRemainingTTL)
			assert.False(t, handle.Items[0].NeedsWriteback)

			target := astjson.MustParseBytes([]byte(`{}`))
			handle.Items[0].Item = target
			require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
				Items: []*astjson.Value{target},
				Arena: beginner(),
			}))
			rc.EndRequest()
			assert.Equal(t, `{"name":"Table","price":100,"__typename":"Product"}`, string(target.MarshalTo(nil)))
			assert.Equal(t, []testStoreOp{
				{Kind: "Get", Key: skuKey},
				{Kind: "Get", Key: upcKey},
				{Kind: "Set", Key: skuKey, Value: `{"name":"Table","price":100,"__typename":"Product"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonBackfill},
			}, store.ops)
		})
	})

	t.Run("[E5] pending candidate renders from the SERVED value on a hit", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		skuKey, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		// The cached value itself carries sku, so the pending sku candidate can
		// render from it at write-back time.
		store.seed(upcKey, []byte(`{"__typename":"Product","name":"Table","price":100,"sku":"S1"}`), time.Minute)

		rc := newMultiKeyRC(store)
		item := productItem(t, "1") // no sku: sku candidate pending
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		require.Len(t, handle.Items[0].PendingCandidates, 1)

		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{item},
			Arena: beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: skuKey, Value: `{"name":"Table","price":100,"__typename":"Product","sku":"S1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})
}

// TestMultiKeyFreshnessRows covers the D7–D13 selection ladder.
func TestMultiKeyFreshnessRows(t *testing.T) {
	t.Run("[D7] the freshest covering value wins", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cfg := multiKeyConfig(t)
			skuKey, upcKey := deriveMultiKeys(t, cfg)
			store := newTestStore()
			store.seed(skuKey, []byte(`{"__typename":"Product","name":"Fresh","price":1}`), time.Minute)
			store.seed(upcKey, []byte(`{"__typename":"Product","name":"Stale","price":2}`), 10*time.Second)

			rc := newMultiKeyRC(store)
			decision, handle := prepare(t, rc, cfg, fullProductItem(t))
			require.Equal(t, resolve.DecisionSkipFullHit, decision)
			assert.Equal(t, `{"name":"Fresh","price":1,"__typename":"Product"}`, string(handle.Items[0].FromCache.MarshalTo(nil)))
			assert.Equal(t, time.Minute, handle.Items[0].SelectedRemainingTTL)
			assert.False(t, handle.Items[0].NeedsWriteback)
			// Candidates are recorded freshest first.
			assert.Equal(t, []resolve.CacheCandidate{
				{Value: []byte(`{"__typename":"Product","name":"Fresh","price":1}`), RemainingTTL: time.Minute},
				{Value: []byte(`{"__typename":"Product","name":"Stale","price":2}`), RemainingTTL: 10 * time.Second},
			}, handle.Items[0].FromCacheCandidates)
		})
	})

	t.Run("[D8] a known TTL beats an unknown one", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		skuKey, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		store.seedNoTTL(skuKey, []byte(`{"__typename":"Product","name":"Unknown","price":1}`))
		store.seed(upcKey, []byte(`{"__typename":"Product","name":"Known","price":2}`), 10*time.Second)

		rc := newMultiKeyRC(store)
		decision, handle := prepare(t, rc, cfg, fullProductItem(t))
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, `{"name":"Known","price":2,"__typename":"Product"}`, string(handle.Items[0].FromCache.MarshalTo(nil)))
	})

	t.Run("[D9] merge synthesis: the union covers, freshest wins conflicts, canonical rewritten", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cfg := multiKeyConfig(t)
			skuKey, upcKey := deriveMultiKeys(t, cfg)
			store := newTestStore()
			// Fresher value has name (and a conflicting price=1); older has price only.
			store.seed(skuKey, []byte(`{"__typename":"Product","name":"Table","price":1}`), time.Minute)
			store.seed(upcKey, []byte(`{"__typename":"Product","price":2}`), 10*time.Second)
			// Neither covers alone: make name non-nullable and REQUIRE a field
			// only the union has.
			cfg.ProvidesData = &resolve.Object{
				Fields: []*resolve.Field{
					{Name: []byte("name"), Value: &resolve.Scalar{Nullable: false, Path: []string{"name"}}},
					{Name: []byte("price"), Value: &resolve.Scalar{Nullable: false, Path: []string{"price"}}},
					{Name: []byte("weight"), Value: &resolve.Scalar{Nullable: true, Path: []string{"weight"}}},
				},
			}
			store.seed(skuKey, []byte(`{"__typename":"Product","name":"Table","weight":9}`), time.Minute)
			store.seed(upcKey, []byte(`{"__typename":"Product","price":2,"weight":1}`), 10*time.Second)

			rc := newMultiKeyRC(store)
			decision, handle := prepare(t, rc, cfg, fullProductItem(t))
			require.Equal(t, resolve.DecisionSkipFullHit, decision)
			require.NotNil(t, handle.Items[0].FromCache)
			// Union serves; the fresher value's weight=9 wins the conflict.
			assert.Equal(t, `{"name":"Table","price":2,"weight":9,"__typename":"Product"}`, string(handle.Items[0].FromCache.MarshalTo(nil)))
			assert.True(t, handle.Items[0].NeedsWriteback)
			assert.True(t, handle.MustWriteBack)

			// The read-hit write-back REFRESHES both canonical keys with the
			// synthesized value.
			require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
				Items: []*astjson.Value{handle.Items[0].Item},
				Arena: beginner(),
			}))
			rc.EndRequest()
			assert.Equal(t, []testStoreOp{
				{Kind: "Get", Key: skuKey},
				{Kind: "Get", Key: upcKey},
				{Kind: "Set", Key: skuKey, Value: `{"name":"Table","price":2,"weight":9,"__typename":"Product"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
				{Kind: "Set", Key: upcKey, Value: `{"name":"Table","price":2,"weight":9,"__typename":"Product"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			}, store.ops)
		})
	})

	t.Run("[D10] older single covering value is the fallback and owes a refresh", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cfg := multiKeyConfig(t)
			skuKey, upcKey := deriveMultiKeys(t, cfg)
			store := newTestStore()
			// Fresher value is shape-stale (missing non-nullable name; extra
			// conflicting garbage that breaks the union too).
			store.seed(skuKey, []byte(`{"__typename":"Product","price":null}`), time.Minute)
			store.seed(upcKey, []byte(`{"__typename":"Product","name":"Older","price":2}`), 10*time.Second)
			// Union fails: merged value has price=null from the fresher entry.
			cfg.ProvidesData = &resolve.Object{
				Fields: []*resolve.Field{
					{Name: []byte("name"), Value: &resolve.Scalar{Nullable: false, Path: []string{"name"}}},
					{Name: []byte("price"), Value: &resolve.Scalar{Nullable: false, Path: []string{"price"}}},
				},
			}

			rc := newMultiKeyRC(store)
			decision, handle := prepare(t, rc, cfg, fullProductItem(t))
			require.Equal(t, resolve.DecisionSkipFullHit, decision)
			assert.Equal(t, `{"name":"Older","price":2,"__typename":"Product"}`, string(handle.Items[0].FromCache.MarshalTo(nil)))
			assert.Equal(t, 10*time.Second, handle.Items[0].SelectedRemainingTTL)
			assert.True(t, handle.Items[0].NeedsWriteback)
		})
	})

	t.Run("[adversarial] empty union: nothing covers, nothing served", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		skuKey, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		store.seed(skuKey, []byte(`{}`), time.Minute)
		store.seed(upcKey, []byte(`{}`), time.Minute)

		rc := newMultiKeyRC(store)
		decision, handle := prepare(t, rc, cfg, fullProductItem(t))
		require.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.Items[0].FromCache)
		assert.Len(t, handle.Items[0].FromCacheCandidates, 2)
	})

	t.Run("[adversarial] all candidates stale even merged: miss", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		skuKey, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		// Both lack the non-nullable name; the union does too.
		store.seed(skuKey, []byte(`{"__typename":"Product","price":1}`), time.Minute)
		store.seed(upcKey, []byte(`{"__typename":"Product","price":2}`), 30*time.Second)

		rc := newMultiKeyRC(store)
		decision, handle := prepare(t, rc, cfg, fullProductItem(t))
		require.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.Items[0].FromCache)
	})

	t.Run("[D12/D13] AND-reduction across items with multi-key states", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		_, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		store.seed(upcKey, []byte(`{"__typename":"Product","name":"Table","price":100}`), time.Minute)

		rc := newMultiKeyRC(store)
		itemCovered := fullProductItem(t)
		itemMiss := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"2","sku":"S2"}`))
		decision, handle := prepare(t, rc, cfg, itemCovered, itemMiss)
		require.Equal(t, resolve.DecisionFetch, decision)
		assert.NotNil(t, handle.Items[0].FromCache)
		assert.Nil(t, handle.Items[1].FromCache)
		assert.False(t, handle.MustWriteBack) // not a full hit
	})
}
