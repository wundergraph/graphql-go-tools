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

// negativeConfig is the entity config with negative caching enabled.
func negativeConfig(t *testing.T, negativeTTL time.Duration) *resolve.FetchCacheConfig {
	t.Helper()
	cfg := entityConfig(t, time.Minute)
	cfg.NegativeCacheTTL = negativeTTL
	return cfg
}

// emptyEntityInput is the loader's post-merge view of a SUCCESSFUL fetch that
// returned no entity: parsed response, null data, EmptyEntity set.
func emptyEntityInput(item *astjson.Value) resolve.MergeInput {
	return resolve.MergeInput{
		Items:        []*astjson.Value{item},
		ResponseData: astjson.MustParseBytes([]byte(`null`)),
		EmptyEntity:  true,
		StatusCode:   200,
		Arena:        beginner(),
	}
}

// TestControllerNegativeRows covers the G rows.
func TestControllerNegativeRows(t *testing.T) {
	newRC := func(store Store) resolve.RequestCache {
		return NewController(store, nil).BeginRequest(nil)
	}

	t.Run("[G1] empty entity writes the exact sentinel with NegativeCacheTTL", func(t *testing.T) {
		store := newTestStore()
		cfg := negativeConfig(t, 5*time.Second)
		rc := newRC(store)
		item := productItem(t, "404")
		_, handle := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, emptyEntityInput(item)))
		rc.EndRequest()
		key := handle.Items[0].RenderedKeys[0]
		assert.Equal(t, []testStoreOp{
			{Kind: "Get", Key: key},
			{Kind: "Set", Key: key, Value: "null", TTL: 5 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
		}, store.ops)
		assert.True(t, handle.Items[0].NegativeHit)
		require.NotNil(t, handle.Items[0].FromCache)
		assert.Equal(t, astjson.TypeNull, handle.Items[0].FromCache.Type())
	})

	t.Run("[G2] NegativeCacheTTL == 0 disables the L2 path: zero store writes", func(t *testing.T) {
		store := newTestStore()
		cfg := negativeConfig(t, 0)
		cfg.L1 = false // isolate the L2 rule; the L1 sentinel has no TTL knob (task 17)
		rc := newRC(store)
		item := productItem(t, "404")
		_, handle := prepare(t, rc, cfg, item)
		require.NoError(t, rc.OnFetchResult(handle, emptyEntityInput(item)))
		rc.EndRequest()
		key := handle.Items[0].RenderedKeys[0]
		assert.Equal(t, []testStoreOp{{Kind: "Get", Key: key}}, store.ops)
		assert.False(t, handle.Items[0].NegativeHit)
	})

	t.Run("[G3] a sentinel hit serves null with zero network and NegativeHit recorded", func(t *testing.T) {
		store := newTestStore()
		cfg := negativeConfig(t, 5*time.Second)
		// Prime the sentinel through the controller itself (read key == write key).
		primeRC := newRC(store)
		primeItem := productItem(t, "404")
		_, primeHandle := prepare(t, primeRC, cfg, primeItem)
		require.NoError(t, primeRC.OnFetchResult(primeHandle, emptyEntityInput(primeItem)))
		primeRC.EndRequest()

		rc := newRC(store)
		item := productItem(t, "404")
		decision, handle := prepare(t, rc, cfg, item)
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		require.Len(t, handle.Items, 1)
		assert.True(t, handle.Items[0].NegativeHit)
		require.NotNil(t, handle.Items[0].FromCache)
		assert.Equal(t, astjson.TypeNull, handle.Items[0].FromCache.Type())

		// The splice leaves the target UNTOUCHED (exactly like a real
		// successful-but-empty fetch, whose mergeResult early-returns), so the
		// resolvable renders the identical null bubble and error either way.
		require.NoError(t, rc.OnFetchSkipped(handle, resolve.MergeInput{
			Items: []*astjson.Value{item},
			Arena: beginner(),
		}))
		assert.Equal(t, `{"__typename":"Product","upc":"404"}`, string(item.MarshalTo(nil)))
	})

	t.Run("[G4] the sentinel expires after NegativeCacheTTL and the network runs again", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := negativeConfig(t, 5*time.Second)
			primeRC := newRC(store)
			primeItem := productItem(t, "404")
			_, primeHandle := prepare(t, primeRC, cfg, primeItem)
			require.NoError(t, primeRC.OnFetchResult(primeHandle, emptyEntityInput(primeItem)))
			primeRC.EndRequest()

			decision, _ := prepare(t, newRC(store), cfg, productItem(t, "404"))
			assert.Equal(t, resolve.DecisionSkipFullHit, decision)

			time.Sleep(5*time.Second + time.Second)
			decision, handle := prepare(t, newRC(store), cfg, productItem(t, "404"))
			assert.Equal(t, resolve.DecisionFetch, decision)
			assert.False(t, handle.Items[0].NegativeHit)
		})
	})

	t.Run("[G5] EmptyEntity with a failure signal never writes", func(t *testing.T) {
		failureRows := []struct {
			name   string
			mutate func(in *resolve.MergeInput)
		}{
			{"FetchFailed wins over EmptyEntity", func(in *resolve.MergeInput) { in.FetchFailed = true }},
			{"HasErrors wins over EmptyEntity", func(in *resolve.MergeInput) { in.HasErrors = true }},
			{"both signals", func(in *resolve.MergeInput) { in.FetchFailed = true; in.HasErrors = true }},
		}
		for _, row := range failureRows {
			t.Run(row.name, func(t *testing.T) {
				store := newTestStore()
				cfg := negativeConfig(t, 5*time.Second)
				rc := newRC(store)
				item := productItem(t, "404")
				_, handle := prepare(t, rc, cfg, item)
				in := emptyEntityInput(item)
				row.mutate(&in)
				require.NoError(t, rc.OnFetchResult(handle, in))
				rc.EndRequest()
				key := handle.Items[0].RenderedKeys[0]
				assert.Equal(t, []testStoreOp{{Kind: "Get", Key: key}}, store.ops)
				assert.False(t, handle.Items[0].NegativeHit)
			})
		}
	})

	t.Run("[G6] a positive value with a null FIELD is not a sentinel", func(t *testing.T) {
		store := newTestStore()
		cfg := negativeConfig(t, 5*time.Second)
		// A positive cached value whose nullable field is null: served as a
		// normal hit, NEVER routed through the negative path.
		writeThrough(t, newRC(store), cfg, productItem(t, "1"), `{"__typename":"Product","name":"Table","price":null}`)

		decision, handle := prepare(t, newRC(store), cfg, productItem(t, "1"))
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.False(t, handle.Items[0].NegativeHit)
		require.NotNil(t, handle.Items[0].FromCache)
		assert.Equal(t, astjson.TypeObject, handle.Items[0].FromCache.Type())
	})
}
