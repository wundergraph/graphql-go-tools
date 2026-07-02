package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// batchInput builds a PrepareFetchInput with one BatchStats bucket per item;
// each bucket's targets alias the representative (the common loader shape).
func batchInput(cfg *resolve.FetchCacheConfig, buckets ...[]*astjson.Value) resolve.PrepareFetchInput {
	in := prepareInput(cfg)
	in.BatchStats = buckets
	return in
}

// TestControllerBatchRows covers the I rows: per-item keying, full-batch
// serve/refetch semantics, per-element backfill, batch splice targets.
func TestControllerBatchRows(t *testing.T) {
	newRC := func(store Store) resolve.RequestCache {
		return NewController(store, nil).BeginRequest(nil)
	}

	primeBatch := func(t *testing.T, store *testStore, cfg *resolve.FetchCacheConfig, upcs []string, entities string) []string {
		t.Helper()
		rc := newRC(store)
		buckets := make([][]*astjson.Value, 0, len(upcs))
		for _, upc := range upcs {
			buckets = append(buckets, []*astjson.Value{productItem(t, upc)})
		}
		decision, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		require.Equal(t, resolve.DecisionFetch, decision)
		require.NotNil(t, handle)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			BatchStats:   buckets,
			ResponseData: astjson.MustParseBytes([]byte(entities)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		keys := make([]string, 0, len(handle.Items))
		for _, item := range handle.Items {
			require.Len(t, item.RenderedKeys, 1)
			keys = append(keys, item.RenderedKeys[0])
		}
		return keys
	}

	t.Run("[I1] per-item keying: N entities produce N distinct Gets and Sets", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		keys := primeBatch(t, store, cfg, []string{"1", "2"},
			`[{"__typename":"Product","name":"Table","price":100},{"__typename":"Product","name":"Chair","price":50}]`)
		require.Len(t, keys, 2)
		assert.NotEqual(t, keys[0], keys[1])
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: keys[0]},
			{Kind: "Get", Key: keys[1]},
			{Kind: "Set", Key: keys[0], Value: `{"__typename":"Product","name":"Table","price":100}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: keys[1], Value: `{"__typename":"Product","name":"Chair","price":50}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
		}, store.ops)
	})

	t.Run("[I2] full-batch hit serves every merge target at the merge path", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		primeBatch(t, store, cfg, []string{"1", "2"},
			`[{"__typename":"Product","name":"Table","price":100},{"__typename":"Product","name":"Chair","price":50}]`)

		rc := newRC(store)
		// Bucket 0 has TWO merge targets (a deduplicated representation).
		target0a := productItem(t, "1")
		target0b := productItem(t, "1")
		target1 := productItem(t, "2")
		buckets := [][]*astjson.Value{{target0a, target0b}, {target1}}
		decision, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.True(t, handle.BatchEntityKey)
		assert.Equal(t, []int{0, 1}, []int{handle.Items[0].BatchIndex, handle.Items[1].BatchIndex})

		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			BatchStats: buckets,
			MergePath:  []string{"details"},
			Arena:      beginner(),
		}))
		assert.Equal(t, `{"__typename":"Product","upc":"1","details":{"name":"Table","price":100,"__typename":"Product"}}`, string(target0a.MarshalTo(nil)))
		assert.Equal(t, `{"__typename":"Product","upc":"1","details":{"name":"Table","price":100,"__typename":"Product"}}`, string(target0b.MarshalTo(nil)))
		assert.Equal(t, `{"__typename":"Product","upc":"2","details":{"name":"Chair","price":50,"__typename":"Product"}}`, string(target1.MarshalTo(nil)))
	})

	t.Run("[I3] mixed batch never partially serves: full refetch with lookups logged", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		keys := primeBatch(t, store, cfg, []string{"1"}, `[{"__typename":"Product","name":"Table","price":100}]`)
		store.ops = nil // the mixed batch's ops assert in isolation

		rc := newRC(store)
		buckets := [][]*astjson.Value{{productItem(t, "1")}, {productItem(t, "3")}}
		decision, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		require.Equal(t, resolve.DecisionFetch, decision)
		assert.False(t, handle.WasHit)
		// Item 0 is covered, item 1 not — but full-batch semantics refetch all.
		assert.NotNil(t, handle.Items[0].FromCache)
		assert.Nil(t, handle.Items[1].FromCache)

		// The full refetch writes EVERY entity afterwards.
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			BatchStats:   buckets,
			ResponseData: astjson.MustParseBytes([]byte(`[{"__typename":"Product","name":"Table","price":100},{"__typename":"Product","name":"Desk","price":80}]`)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		missKey := handle.Items[1].RenderedKeys[0]
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: keys[0]},
			{Kind: "Get", Key: missKey},
			{Kind: "Set", Key: keys[0], Value: `{"__typename":"Product","name":"Table","price":100}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: missKey, Value: `{"__typename":"Product","name":"Desk","price":80}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
		}, store.ops)
	})

	t.Run("[I4] per-element multi-key backfill from the batch response", func(t *testing.T) {
		cfg := multiKeyConfig(t)
		skuKey, upcKey := deriveMultiKeys(t, cfg)
		store := newTestStore()
		rc := newRC(store)
		// Items lack sku, so the sku candidate is pending per element.
		buckets := [][]*astjson.Value{{productItem(t, "1")}}
		decision, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		require.Equal(t, resolve.DecisionFetch, decision)
		require.Len(t, handle.Items[0].PendingCandidates, 1)

		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			BatchStats:   buckets,
			ResponseData: astjson.MustParseBytes([]byte(`[{"__typename":"Product","name":"Table","price":100,"sku":"S1"}]`)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: upcKey, Value: `{"__typename":"Product","name":"Table","price":100,"sku":"S1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: skuKey, Value: `{"__typename":"Product","name":"Table","price":100,"sku":"S1"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})

	t.Run("[I5] a non-array batch response writes nothing", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute)
		rc := newRC(store)
		buckets := [][]*astjson.Value{{productItem(t, "1")}}
		_, handle := rc.PrepareFetch(batchInput(cfg, buckets...))
		require.NotNil(t, handle)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			BatchStats:   buckets,
			ResponseData: astjson.MustParseBytes([]byte(`{"not":"an array"}`)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		key := handle.Items[0].RenderedKeys[0]
		assert.Equal(t, []testStoreOp{{Kind: "Get", Key: key}}, store.ops)
	})
}
