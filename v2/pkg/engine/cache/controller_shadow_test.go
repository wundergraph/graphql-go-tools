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

// recordingShadowObserver is the in-package observer double (cachetesting
// cannot be imported here); it records compares as (key, isFresh, age).
type recordingShadowObserver struct {
	compares []struct {
		key     string
		isFresh bool
		age     time.Duration
	}
}

func (o *recordingShadowObserver) BeginRequest(*resolve.Context) {}
func (o *recordingShadowObserver) EndRequest(*resolve.Context)   {}
func (o *recordingShadowObserver) OnFetchObserved(*resolve.FetchCacheHandle) {
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

func (o *recordingShadowObserver) OnEntity(*resolve.FetchCacheHandle, *astjson.Value)       {}
func (o *recordingShadowObserver) OnFieldValue(resolve.GraphCoordinate, resolve.FieldValue) {}

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
		// The store shows the read happened.
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: key},
			{Kind: "Set", Key: key, Value: fresh, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Get", Key: key},
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
			// L2 was overwritten with the fresh value after the compare.
			value, _, ok := store.Get(key)
			require.True(t, ok)
			assert.Equal(t, fresh, string(value))
		})
	})

	t.Run("[H3] compare MISMATCH: IsFresh false, L2 overwritten with fresh", func(t *testing.T) {
		store := newTestStore()
		cfg := shadowConfig(t)
		key := primeShadow(t, store, cfg)

		obs := &recordingShadowObserver{}
		rc := NewController(store, obs).BeginRequest(nil)
		item := productItem(t, "1")
		_, handle := prepare(t, rc, cfg, item)
		changed := `{"__typename":"Product","name":"Renamed","price":100}`
		require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
			Items:        []*astjson.Value{item},
			ResponseData: astjson.MustParseBytes([]byte(changed)),
			Arena:        beginner(),
		}))
		require.Len(t, obs.compares, 1)
		assert.False(t, obs.compares[0].isFresh)

		rc.EndRequest()
		value, _, ok := store.Get(key)
		require.True(t, ok)
		assert.Equal(t, changed, string(value))
	})

	t.Run("[H6] nil observer: force-fetch, nothing recorded, writes still land", func(t *testing.T) {
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
		value, _, ok := store.Get(key)
		require.True(t, ok)
		assert.Equal(t, fresh, string(value))
	})

	t.Run("[H7] L2-off shadow never yields DecisionFetchShadow", func(t *testing.T) {
		cfg := shadowConfig(t)
		cfg.L2 = false // NO-OP / L1-only shapes: the L2 controller stays out entirely
		rc := NewController(newTestStore(), nil).BeginRequest(nil)
		decision, handle := prepare(t, rc, cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Nil(t, handle)
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
