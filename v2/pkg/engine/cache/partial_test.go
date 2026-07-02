package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestFilterBatchInput pins the reduced-input bytes and the fallback cases.
func TestFilterBatchInput(t *testing.T) {
	input := []byte(`{"method":"POST","url":"http://x","body":{"query":"...","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`)

	t.Run("filters covered representations, keeping order", func(t *testing.T) {
		reduced, ok := filterBatchInput(input, []bool{true, false, true})
		require.True(t, ok)
		assert.Equal(t,
			`{"method":"POST","url":"http://x","body":{"query":"...","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"3"}]}}}`,
			string(reduced))
	})

	t.Run("unexpected shape falls back", func(t *testing.T) {
		_, ok := filterBatchInput([]byte(`{"body":{"variables":{}}}`), []bool{true})
		assert.False(t, ok)
	})

	t.Run("length mismatch falls back", func(t *testing.T) {
		_, ok := filterBatchInput(input, []bool{true, false})
		assert.False(t, ok)
	})

	t.Run("degenerate all-keep and none-keep fall back", func(t *testing.T) {
		_, ok := filterBatchInput(input, []bool{true, true, true})
		assert.False(t, ok)
		_, ok = filterBatchInput(input, []bool{false, false, false})
		assert.False(t, ok)
	})
}

// partialConfig is the batch entity config with partial loading enabled.
func partialConfig(t *testing.T) *resolve.FetchCacheConfig {
	t.Helper()
	cfg := entityConfig(t, time.Minute)
	cfg.EnablePartialCacheLoad = true
	return cfg
}

// batchPrepareInput builds a PrepareFetchInput with BatchStats buckets (one
// target per bucket) and the rendered representations input matching them.
func batchPrepareInput(t *testing.T, cfg *resolve.FetchCacheConfig, upcs ...string) (resolve.PrepareFetchInput, [][]*astjson.Value) {
	t.Helper()
	buckets := make([][]*astjson.Value, 0, len(upcs))
	representations := ""
	for i, upc := range upcs {
		if i > 0 {
			representations += ","
		}
		representations += `{"__typename":"Product","upc":"` + upc + `"}`
		buckets = append(buckets, []*astjson.Value{astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"` + upc + `"}`))})
	}
	in := resolve.PrepareFetchInput{
		Config:     cfg,
		BatchStats: buckets,
		Input:      []byte(`{"body":{"query":"...","variables":{"representations":[` + representations + `]}}}`),
		Arena:      beginner(),
	}
	return in, buckets
}

// TestPartialBatchRows covers the split, realign, adversarial, and failure rows.
func TestPartialBatchRows(t *testing.T) {
	fresh := func(upc string) string {
		return `{"__typename":"Product","name":"P` + upc + `","price":100}`
	}

	t.Run("partial split: exact partition, reduced input, lookups for all", func(t *testing.T) {
		store := newTestStore()
		cfg := partialConfig(t)
		key1 := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "2"), fresh("2"))
		storeOpsBefore := len(store.ops)

		rc := NewController(store, nil).BeginRequest(nil)
		in, _ := batchPrepareInput(t, cfg, "1", "2", "3")
		decision, handle := rc.PrepareFetch(in)
		assert.Equal(t, resolve.DecisionFetchPartial, decision)
		require.NotNil(t, handle)
		// Exact partition: bucket 1 covered, buckets 0 and 2 missing.
		assert.Nil(t, handle.Items[0].FromCache)
		require.NotNil(t, handle.Items[1].FromCache)
		assert.Equal(t, fresh("2"), string(handle.Items[1].FromCache.MarshalTo(nil)))
		assert.Nil(t, handle.Items[2].FromCache)
		// The reduced input carries EXACTLY the missing representations.
		assert.Equal(t,
			`{"body":{"query":"...","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"3"}]}}}`,
			string(handle.PartialInput))
		// Lookups ran for ALL buckets (three Gets; key1 among them).
		gets := store.ops[storeOpsBefore:]
		require.Len(t, gets, 3)
		assert.Equal(t, "Get", gets[0].Kind)
		assert.Equal(t, key1, gets[1].Key)
	})

	t.Run("realign: fetched entities land at their ORIGINAL positions; writes only for fetched", func(t *testing.T) {
		store := newTestStore()
		cfg := partialConfig(t)
		writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "2"), fresh("2"))
		store.ops = nil

		rc := NewController(store, nil).BeginRequest(nil)
		in, buckets := batchPrepareInput(t, cfg, "1", "2", "3")
		decision, handle := rc.PrepareFetch(in)
		require.Equal(t, resolve.DecisionFetchPartial, decision)

		// The REDUCED response: entities for upc 1 and 3 only, in order.
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			BatchStats:   buckets,
			ResponseData: astjson.MustParseBytes([]byte(`[` + fresh("1") + `,` + fresh("3") + `]`)),
			Arena:        beginner(),
		}))
		// Full merged targets: cached bucket 1 spliced, fetched 0 and 2 merged
		// at their original positions.
		assert.Equal(t, `{"__typename":"Product","upc":"1","name":"P1","price":100}`, string(buckets[0][0].MarshalTo(nil)))
		assert.Equal(t, `{"__typename":"Product","upc":"2","name":"P2","price":100}`, string(buckets[1][0].MarshalTo(nil)))
		assert.Equal(t, `{"__typename":"Product","upc":"3","name":"P3","price":100}`, string(buckets[2][0].MarshalTo(nil)))

		rc.EndRequest()
		// Writes ONLY for the fetched buckets (0 and 2), none for the covered one.
		var sets []testStoreOp
		for _, op := range store.ops {
			if op.Kind == "Set" {
				sets = append(sets, op)
			}
		}
		require.Len(t, sets, 2)
		assert.Equal(t, fresh("1"), sets[0].Value)
		assert.Equal(t, fresh("3"), sets[1].Value)
	})

	t.Run("duplicated representations: one bucket, many targets, all served", func(t *testing.T) {
		store := newTestStore()
		cfg := partialConfig(t)
		writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "2"), fresh("2"))

		rc := NewController(store, nil).BeginRequest(nil)
		in, buckets := batchPrepareInput(t, cfg, "1", "2")
		// The "1" bucket has TWO merge targets (a duplicated representation).
		buckets[0] = append(buckets[0], astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`)))
		in.BatchStats = buckets
		decision, handle := rc.PrepareFetch(in)
		require.Equal(t, resolve.DecisionFetchPartial, decision)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			BatchStats:   buckets,
			ResponseData: astjson.MustParseBytes([]byte(`[` + fresh("1") + `]`)),
			Arena:        beginner(),
		}))
		assert.Equal(t, `{"__typename":"Product","upc":"1","name":"P1","price":100}`, string(buckets[0][0].MarshalTo(nil)))
		assert.Equal(t, `{"__typename":"Product","upc":"1","name":"P1","price":100}`, string(buckets[0][1].MarshalTo(nil)))
		assert.Equal(t, `{"__typename":"Product","upc":"2","name":"P2","price":100}`, string(buckets[1][0].MarshalTo(nil)))
	})

	t.Run("all-hit degenerates to SkipFullHit; all-miss to Fetch", func(t *testing.T) {
		store := newTestStore()
		cfg := partialConfig(t)
		writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), fresh("1"))
		writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "2"), fresh("2"))

		rc := NewController(store, nil).BeginRequest(nil)
		inHit, _ := batchPrepareInput(t, cfg, "1", "2")
		decision, handle := rc.PrepareFetch(inHit)
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Nil(t, handle.PartialInput)

		inMiss, _ := batchPrepareInput(t, cfg, "8", "9")
		decision, handle = rc.PrepareFetch(inMiss)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.PartialInput)
	})

	t.Run("single-element batch never goes partial", func(t *testing.T) {
		store := newTestStore()
		cfg := partialConfig(t)
		rc := NewController(store, nil).BeginRequest(nil)
		in, _ := batchPrepareInput(t, cfg, "1")
		decision, handle := rc.PrepareFetch(in)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.PartialInput)
	})

	t.Run("failure in the fetched subset: spliced subset intact, zero writes", func(t *testing.T) {
		store := newTestStore()
		cfg := partialConfig(t)
		writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "2"), fresh("2"))
		store.ops = nil

		rc := NewController(store, nil).BeginRequest(nil)
		in, buckets := batchPrepareInput(t, cfg, "1", "2")
		decision, handle := rc.PrepareFetch(in)
		require.Equal(t, resolve.DecisionFetchPartial, decision)
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			BatchStats:  buckets,
			FetchFailed: true,
			Arena:       beginner(),
		}))
		rc.EndRequest()
		// The covered bucket is spliced; the failed bucket's target untouched.
		assert.Equal(t, `{"__typename":"Product","upc":"1"}`, string(buckets[0][0].MarshalTo(nil)))
		assert.Equal(t, `{"__typename":"Product","upc":"2","name":"P2","price":100}`, string(buckets[1][0].MarshalTo(nil)))
		for _, op := range store.ops {
			assert.NotEqual(t, "Set", op.Kind)
		}
	})

	t.Run("config gate: partial disabled keeps all-or-nothing", func(t *testing.T) {
		store := newTestStore()
		cfg := entityConfig(t, time.Minute) // EnablePartialCacheLoad false
		writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "2"), fresh("2"))

		rc := NewController(store, nil).BeginRequest(nil)
		in, _ := batchPrepareInput(t, cfg, "1", "2")
		decision, handle := rc.PrepareFetch(in)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle.PartialInput)
	})

	t.Run("shadow wins over partial", func(t *testing.T) {
		store := newTestStore()
		cfg := partialConfig(t)
		cfg.ShadowMode = true
		writeThrough(t, NewController(store, nil).BeginRequest(nil), func() *resolve.FetchCacheConfig {
			plain := partialConfig(t)
			return plain
		}(), productItem(t, "2"), fresh("2"))

		rc := NewController(store, nil).BeginRequest(nil)
		in, _ := batchPrepareInput(t, cfg, "1", "2")
		decision, handle := rc.PrepareFetch(in)
		assert.Equal(t, resolve.DecisionFetchShadow, decision)
		assert.Nil(t, handle.PartialInput)
	})
}
