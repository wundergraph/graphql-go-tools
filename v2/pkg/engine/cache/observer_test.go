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

// noopObserver is the do-nothing resolve.CacheObserver the package's observer
// doubles embed (cachetesting cannot be imported here), so a double spells out
// only the hooks it actually records.
type noopObserver struct{}

func (noopObserver) BeginRequest(*resolve.Context)             {}
func (noopObserver) EndRequest(*resolve.Context)               {}
func (noopObserver) OnFetchObserved(*resolve.FetchCacheHandle) {}
func (noopObserver) CompareShadow(*resolve.FetchCacheHandle, *astjson.Value, *resolve.CacheTransaction) {
}
func (noopObserver) OnStoreError(string, string, int, error)                  {}
func (noopObserver) OnUncacheablePrivate(string, string)                      {}
func (noopObserver) OnScopeMismatch(string, string)                           {}
func (noopObserver) OnEntity(*resolve.FetchCacheHandle, *astjson.Value)       {}
func (noopObserver) OnFieldValue(resolve.GraphCoordinate, resolve.FieldValue) {}

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

	t.Run("miss + fetch: decision and the item's key", func(t *testing.T) {
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
					Key: handle.Items[0].RenderedKey,
					Hit: false,
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
						Key:              key,
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
					Key:        handle.Items[0].RenderedKey,
					ServedFrom: "l1",
					Hit:        true,
				},
			},
		}, trace.CacheTrace)
	})

	t.Run("L1-only fetch: the item traces under the HASH of its preimage", func(t *testing.T) {
		obs := NewTraceObserver()
		rc := NewController(newTestStore(), obs).BeginRequest(nil)
		cfg := entityConfig(t, 0) // TTL 0: L1 true, L2 false
		l1Fetch(t, rc, cfg, productItem(t, "1"), astjson.MustParseBytes([]byte(fresh)))

		item := productItem(t, "1")
		in, trace := tracedInput(cfg, item)
		decision, handle := rc.PrepareFetch(in)
		require.Equal(t, resolve.DecisionSkipFullHit, decision)
		rc.EndRequest()
		// No store key exists to trace, and the raw preimage carries the
		// entity's @key values — so the trace identifies the entry by the
		// preimage's hash.
		assert.Equal(t, `products:{"__typename":"Product","representation":{"upc":"1"}}`, handle.Items[0].L1Key)
		assert.Equal(t, &resolve.CacheTrace{
			Decision: "SkipFullHit",
			Hit:      true,
			Items: []resolve.CacheItemTrace{
				{
					Key:        hashHex([]byte(`products:{"__typename":"Product","representation":{"upc":"1"}}`)),
					ServedFrom: "l1",
					Hit:        true,
				},
			},
		}, trace.CacheTrace)
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
		assert.Equal(t, key, trace.CacheTrace.Items[0].Key)
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
		require.Len(t, trace.CacheTrace.Items, 1)
		hashed := trace.CacheTrace.Items[0].Key
		raw := handle.Items[0].RenderedKey
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
	return handle.Items[0].RenderedKey
}

// countingObserver counts OnFetchObserved calls around the real TraceObserver.
type countingObserver struct {
	*TraceObserver

	observed int
}

func (c *countingObserver) OnFetchObserved(h *resolve.FetchCacheHandle) {
	c.observed++
	c.TraceObserver.OnFetchObserved(h)
}

// TestFlushTracesBeforeEndRequest: the resolver flushes cache traces BEFORE
// the response renders (extensions.trace serializes during Resolve, EndRequest
// runs after the response is written). The trace must be attached by
// FlushTraces, and the handle observed exactly ONCE — EndRequest skips
// already-flushed handles.
func TestFlushTracesBeforeEndRequest(t *testing.T) {
	obs := &countingObserver{TraceObserver: NewTraceObserver()}
	rc := NewController(newTestStore(), obs).BeginRequest(nil)
	cfg := entityConfig(t, time.Minute)
	item := productItem(t, "1")
	in, trace := tracedInput(cfg, item)
	decision, handle := rc.PrepareFetch(in)
	require.Equal(t, resolve.DecisionFetch, decision)
	require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
		Items:        []*astjson.Value{item},
		ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100}`)),
		Arena:        beginner(),
	}))

	flusher, ok := rc.(resolve.CacheTraceFlusher)
	require.True(t, ok, "requestCache must implement CacheTraceFlusher")
	flusher.FlushTraces()
	assert.Equal(t, &resolve.CacheTrace{
		Decision: "Fetch",
		Hit:      false,
		Items: []resolve.CacheItemTrace{
			{
				Key: handle.Items[0].RenderedKey,
				Hit: false,
			},
		},
	}, trace.CacheTrace)
	assert.Equal(t, 1, obs.observed)

	rc.EndRequest()
	assert.Equal(t, 1, obs.observed)
}
