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

// tracedInput wires a fetch with an ART trace destination into the prepare
// input, exactly as the loader does when tracing is enabled.
func tracedInput(cfg *resolve.FetchCacheConfig, items ...*astjson.Value) (resolve.PrepareFetchInput, *resolve.DataSourceLoadTrace) {
	trace := &resolve.DataSourceLoadTrace{}
	in := prepareInput(cfg, items...)
	in.Item = &resolve.FetchItem{Fetch: &resolve.EntityFetch{Trace: trace}}
	return in, trace
}

// TestTraceObserverRows pins the COMPLETE assembled CacheTrace per scenario.
func TestTraceObserverRows(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	t.Run("miss + fetch: decision, keys, refresh write", func(t *testing.T) {
		obs := NewTraceObserver()
		rc := NewController(newTestStore(), obs).BeginRequest(nil)
		cfg := entityConfig(t, time.Minute)
		item := productItem(t, "1")
		in, trace := tracedInput(cfg, item)
		decision, handle := rc.PrepareFetch(in)
		require.Equal(t, resolve.DecisionFetch, decision)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(fresh)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, &resolve.CacheTrace{
			Decision: "Fetch",
			Hit:      false,
			Items: []resolve.CacheItemTrace{
				{
					Keys:        handle.Items[0].RenderedKeys,
					Hit:         false,
					WriteReason: "refresh",
				},
			},
		}, trace.CacheTrace)
	})

	t.Run("L2 hit: served_from l2 with the EXACT remaining TTL", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			key := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), fresh)

			time.Sleep(20 * time.Second) // age inside the bubble

			obs := NewTraceObserver()
			rc := NewController(store, obs).BeginRequest(nil)
			item := productItem(t, "1")
			in, trace := tracedInput(cfg, item)
			decision, handle := rc.PrepareFetch(in)
			require.Equal(t, resolve.DecisionSkipFullHit, decision)
			require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
				Items: []*astjson.Value{item},
				Arena: beginner(),
			}))
			rc.EndRequest()
			assert.Equal(t, &resolve.CacheTrace{
				Decision: "SkipFullHit",
				Hit:      true,
				Items: []resolve.CacheItemTrace{
					{
						Keys:             []string{key},
						ServedFrom:       "l2",
						Hit:              true,
						RemainingTTLNano: int64(40 * time.Second),
					},
				},
			}, trace.CacheTrace)
		})
	})

	t.Run("L1 hit: served_from l1, no TTL", func(t *testing.T) {
		obs := NewTraceObserver()
		rc := NewController(newTestStore(), obs).BeginRequest(nil)
		cfg := entityConfig(t, time.Minute)
		l1Fetch(t, rc, cfg, productItem(t, "1"), astjson.MustParseBytes([]byte(fresh)))

		item := productItem(t, "1")
		in, trace := tracedInput(cfg, item)
		decision, handle := rc.PrepareFetch(in)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{item},
			Arena: beginner(),
		}))
		rc.EndRequest()
		assert.Equal(t, &resolve.CacheTrace{
			Decision: "SkipFullHit",
			Hit:      true,
			Items: []resolve.CacheItemTrace{
				{
					Keys:       handle.Items[0].RenderedKeys,
					ServedFrom: "l1",
					Hit:        true,
				},
			},
		}, trace.CacheTrace)
	})

	t.Run("backfill: a hit with a missed sibling key records the backfill write", func(t *testing.T) {
		store := newTestStore()
		cfg := multiKeyConfig(t)
		skuKey, upcKey := entityWriteKeys(t, cfg.CacheName, `{"__typename":"Product","upc":"1","sku":"S1"}`)
		store.seed(skuKey, []byte(`{"__typename":"Product","name":"Table","price":100,"upc":"1","sku":"S1"}`), time.Minute)

		obs := NewTraceObserver()
		rc := NewController(store, obs).BeginRequest(nil)
		item := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","sku":"S1"}`))
		in, trace := tracedInput(cfg, item)
		decision, handle := rc.PrepareFetch(in)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{item},
			Arena: beginner(),
		}))
		rc.EndRequest()
		require.NotNil(t, trace.CacheTrace)
		require.Len(t, trace.CacheTrace.Items, 1)
		got := trace.CacheTrace.Items[0]
		assert.Equal(t, []string{skuKey, upcKey}, got.Keys)
		assert.Equal(t, "l2", got.ServedFrom)
		assert.Equal(t, "backfill", got.WriteReason)
	})

	t.Run("negative hit is marked", func(t *testing.T) {
		store := newTestStore()
		cfg := negativeConfig(t, 5*time.Second)
		key := writeNegativeThrough(t, store, cfg, "404")

		obs := NewTraceObserver()
		rc := NewController(store, obs).BeginRequest(nil)
		item := productItem(t, "404")
		in, trace := tracedInput(cfg, item)
		decision, handle := rc.PrepareFetch(in)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{item},
			Arena: beginner(),
		}))
		rc.EndRequest()
		require.NotNil(t, trace.CacheTrace)
		require.Len(t, trace.CacheTrace.Items, 1)
		assert.True(t, trace.CacheTrace.Items[0].NegativeHit)
		assert.True(t, trace.CacheTrace.Items[0].Hit)
		assert.Equal(t, []string{key}, trace.CacheTrace.Items[0].Keys)
		_ = handle
	})

	t.Run("shadow: compares recorded with the EXACT cache age", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := shadowConfig(t)
			key := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), fresh)

			time.Sleep(15 * time.Second)

			obs := NewTraceObserver()
			rc := NewController(store, obs).BeginRequest(nil)
			item := productItem(t, "1")
			in, trace := tracedInput(cfg, item)
			decision, handle := rc.PrepareFetch(in)
			require.Equal(t, resolve.DecisionFetchShadow, decision)
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:        []*astjson.Value{item},
				ResponseData: astjson.MustParseBytes([]byte(fresh)),
				Arena:        beginner(),
			}))
			rc.EndRequest()
			require.NotNil(t, trace.CacheTrace)
			assert.Equal(t, "FetchShadow", trace.CacheTrace.Decision)
			assert.True(t, trace.CacheTrace.Shadow)
			assert.Equal(t, []resolve.CacheShadowCompareTrace{
				{Key: key, IsFresh: true, CacheAgeNano: int64(15 * time.Second)},
			}, trace.CacheTrace.ShadowCompares)
		})
	})

	t.Run("HashAnalyticsKeys hashes key material in trace output", func(t *testing.T) {
		obs := NewTraceObserver()
		rc := NewController(newTestStore(), obs).BeginRequest(nil)
		cfg := entityConfig(t, time.Minute)
		cfg.HashAnalyticsKeys = true
		item := productItem(t, "1")
		in, trace := tracedInput(cfg, item)
		_, handle := rc.PrepareFetch(in)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(fresh)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		require.NotNil(t, trace.CacheTrace)
		require.Len(t, trace.CacheTrace.Items[0].Keys, 1)
		hashed := trace.CacheTrace.Items[0].Keys[0]
		raw := handle.Items[0].RenderedKeys[0]
		assert.NotEqual(t, raw, hashed)
		assert.Equal(t, hashHex([]byte(raw)), hashed)
		assert.Len(t, hashed, 16)
	})

	t.Run("tracing off: nothing attached, compares drained", func(t *testing.T) {
		obs := NewTraceObserver()
		rc := NewController(newTestStore(), obs).BeginRequest(nil)
		cfg := entityConfig(t, time.Minute)
		item := productItem(t, "1")
		// No Item on the input: the loader with tracing disabled leaves
		// handle.Trace nil.
		decision, handle := prepare(t, rc, cfg, item)
		require.Equal(t, resolve.DecisionFetch, decision)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(fresh)),
			Arena:        beginner(),
		}))
		rc.EndRequest()
		assert.Nil(t, handle.Trace)
		assert.Empty(t, obs.compares)
	})
}

// writeNegativeThrough primes the negative sentinel for one item and returns
// its key.
func writeNegativeThrough(t *testing.T, store *testStore, cfg *resolve.FetchCacheConfig, upc string) string {
	t.Helper()
	rc := NewController(store, nil).BeginRequest(nil)
	item := productItem(t, upc)
	_, handle := prepare(t, rc, cfg, item)
	require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
		Items:        []*astjson.Value{item},
		ResponseData: astjson.MustParseBytes([]byte(`null`)),
		EmptyEntity:  true,
		Arena:        beginner(),
	}))
	rc.EndRequest()
	return handle.Items[0].RenderedKeys[0]
}
