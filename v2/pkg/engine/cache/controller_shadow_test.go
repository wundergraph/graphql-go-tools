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

// shadowConfig is the entity config in shadow mode.
func shadowConfig(t *testing.T) *resolve.FetchCacheConfig {
	t.Helper()
	cfg := entityConfig(t, time.Minute)
	cfg.ShadowMode = true
	return cfg
}

// recordingShadowObserver records the shadow compares as (key, isFresh, age),
// and nothing else.
type recordingShadowObserver struct {
	noopObserver

	compares []struct {
		key     string
		isFresh bool
		age     time.Duration
	}
}

func (o *recordingShadowObserver) CompareShadow(h *resolve.FetchCacheHandle, fresh *astjson.Value, tx *resolve.CacheTransaction) {
	for _, entry := range h.ShadowStash {
		freshBytes := []byte("null")
		if fresh != nil {
			freshBytes = fresh.MarshalTo(nil)
		}
		o.compares = append(o.compares, struct {
			key     string
			isFresh bool
			age     time.Duration
		}{
			key:     entry.CacheKey,
			isFresh: string(entry.CachedValue.MarshalTo(nil)) == string(freshBytes),
			age:     entry.CacheTTL - entry.RemainingTTL,
		})
	}
}

// TestControllerShadowRows covers the H rows.
func TestControllerShadowRows(t *testing.T) {
	fresh := `{"__typename":"Product","name":"Table","price":100}`

	primeShadow := func(t *testing.T, store *testStore, cfg *resolve.FetchCacheConfig) string {
		t.Helper()
		// A shadow MISS behaves exactly like a plain miss: fetch + write.
		return writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"), fresh)
	}

	t.Run("[H1] shadow hit stashes, never serves, and forces the fetch", func(t *testing.T) {
		store := newTestStore()
		cfg := shadowConfig(t)
		key := primeShadow(t, store, cfg)
		store.ops = nil // the shadow request's ops assert in isolation

		rc := NewController(store, nil).BeginRequest(nil)
		decision, handle := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetchShadow, decision)
		assert.True(t, handle.Shadow)
		assert.False(t, handle.WasHit)
		require.Len(t, handle.Items, 1)
		// Nothing is servable; the read is stashed.
		assert.Nil(t, handle.Items[0].FromCache)
		require.Contains(t, handle.ShadowStash, 0)
		assert.Equal(t, key, handle.ShadowStash[0].CacheKey)
		assert.Equal(t, fresh, string(handle.ShadowStash[0].CachedValue.MarshalTo(nil)))
		// The store shows the read happened and hit (and nothing else).
		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{key}, Hits: []bool{true}},
		}, store.ops)
	})

	t.Run("[H2] compare MATCH: exact CacheAge, compare precedes the L2 overwrite", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := shadowConfig(t)
			key := primeShadow(t, store, cfg)

			time.Sleep(20 * time.Second) // age the entry inside the bubble

			obs := &recordingShadowObserver{}
			rc := NewController(store, obs).BeginRequest(nil)
			item := productItem(t, "1")
			decision, handle := prepare(t, rc, cfg, item)
			require.Equal(t, resolve.DecisionFetchShadow, decision)

			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:        []*astjson.Value{item},
				ResponseData: astjson.MustParseBytes([]byte(fresh)),
				Arena:        beginner(),
			}))
			// The compare ran BEFORE any write (the deferred set flushes at
			// EndRequest; the compare is already recorded here).
			require.Len(t, obs.compares, 1)
			assert.Equal(t, key, obs.compares[0].key)
			assert.True(t, obs.compares[0].isFresh)
			assert.Equal(t, 20*time.Second, obs.compares[0].age)

			rc.EndRequest()
			// L2 was overwritten with the fresh value after the compare; the
			// rewrite's envelope carries the CURRENT write moment.
			value, ok := store.value(key)
			require.True(t, ok)
			assert.Equal(t, `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684820,"scope":"public"}}`, string(value))
		})
	})

	t.Run("[H3] compare MISMATCH: IsFresh false, L2 overwritten with fresh", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := shadowConfig(t)
			key := primeShadow(t, store, cfg)

			obs := &recordingShadowObserver{}
			rc := NewController(store, obs).BeginRequest(nil)
			item := productItem(t, "1")
			_, handle := prepare(t, rc, cfg, item)
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:        []*astjson.Value{item},
				ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Renamed","price":100}`)),
				Arena:        beginner(),
			}))
			require.Len(t, obs.compares, 1)
			assert.False(t, obs.compares[0].isFresh)

			rc.EndRequest()
			value, ok := store.value(key)
			require.True(t, ok)
			assert.Equal(t, `{"data":{"__typename":"Product","name":"Renamed","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`, string(value))
		})
	})

	t.Run("envelope metadata is not staleness: only the data reaches the compare", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := shadowConfig(t)
			key := primeShadow(t, store, cfg)
			// Rewrite the entry with the SAME data under a DIFFERENT write
			// moment and TTL, so the entry is selected and only its metadata
			// differs.
			store.data[key] = testStoreEntry{
				value:     []byte(`{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":600,"created":946600000,"scope":"public"}}`),
				expiresAt: time.Now().Add(time.Minute),
			}

			obs := &recordingShadowObserver{}
			rc := NewController(store, obs).BeginRequest(nil)
			item := productItem(t, "1")
			decision, handle := prepare(t, rc, cfg, item)
			require.Equal(t, resolve.DecisionFetchShadow, decision)
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:        []*astjson.Value{item},
				ResponseData: astjson.MustParseBytes([]byte(fresh)),
				Arena:        beginner(),
			}))
			require.Len(t, obs.compares, 1)
			assert.True(t, obs.compares[0].isFresh)
		})
	})

	t.Run("[H6] nil observer: force-fetch, nothing recorded, writes still land", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := shadowConfig(t)
			key := primeShadow(t, store, cfg)

			rc := NewController(store, nil).BeginRequest(nil)
			item := productItem(t, "1")
			decision, handle := prepare(t, rc, cfg, item)
			require.Equal(t, resolve.DecisionFetchShadow, decision)
			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:        []*astjson.Value{item},
				ResponseData: astjson.MustParseBytes([]byte(fresh)),
				Arena:        beginner(),
			}))
			rc.EndRequest()
			value, ok := store.value(key)
			require.True(t, ok)
			assert.Equal(t, `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`, string(value))
		})
	})

	t.Run("[H7] L2-off shadow never yields DecisionFetchShadow", func(t *testing.T) {
		cfg := shadowConfig(t)
		cfg.L2 = false
		cfg.L1 = false // NO-OP shape: the controller stays out entirely
		rc := NewController(newTestStore(), nil).BeginRequest(nil)
		decision, handle := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
	})

	t.Run("[H7] L1-only shadow is a plain fetch with no stash", func(t *testing.T) {
		cfg := shadowConfig(t)
		cfg.L2 = false // L1 stays true: shadow semantics are L2-read-only
		store := newTestStore()
		rc := NewController(store, nil).BeginRequest(nil)
		decision, handle := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
		require.NotNil(t, handle)
		assert.False(t, handle.Shadow)
		assert.Empty(t, store.ops)
	})

	t.Run("[H] a shadow probe never populates L1: the second read still probes L2", func(t *testing.T) {
		store := newTestStore()
		cfg := shadowConfig(t) // L1 + L2 + shadow
		key := primeShadow(t, store, cfg)
		store.ops = nil // the shadow request's ops assert in isolation

		rc := NewController(store, nil).BeginRequest(nil)
		_, handleA := prepare(t, rc, cfg, productItem(t, "1"))
		stashA, okA := handleA.ShadowStash[0]
		require.True(t, okA)
		assert.Equal(t, fresh, string(stashA.CachedValue.MarshalTo(nil)))
		_, handleB := prepare(t, rc, cfg, productItem(t, "1"))
		stashB, okB := handleB.ShadowStash[0]
		require.True(t, okB)
		assert.Equal(t, fresh, string(stashB.CachedValue.MarshalTo(nil)))
		// BOTH reads hit L2: only SERVED values enter the request's L1, and a
		// shadow probe serves nothing.
		assert.Equal(t, []testStoreOp{
			{Kind: "GetMany", Keys: []string{key}, Hits: []bool{true}},
			{Kind: "GetMany", Keys: []string{key}, Hits: []bool{true}},
		}, store.ops)
	})

	t.Run("[H] shadow miss is a plain fetch (no stash, no shadow decision)", func(t *testing.T) {
		store := newTestStore()
		cfg := shadowConfig(t)
		rc := NewController(store, nil).BeginRequest(nil)
		decision, handle := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.False(t, handle.Shadow)
		assert.Nil(t, handle.ShadowStash)
	})
}
