package cachetesting

import (
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestFakeStoreTTLExpiry proves TTL semantics against the real clock inside a
// synctest bubble: a value is served before its TTL and is a miss after the
// bubble's fake time passes it, with the full op log recorded — one entry per
// call, carrying every key and its hit outcome.
func TestFakeStoreTTLExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := NewFakeStore()
		require.NoError(t, store.SetMany(t.Context(), []cache.Item{
			{Key: "product:1", Value: []byte(`{"name":"Table"}`), TTL: time.Second},
			{Key: "product:2", Value: []byte(`{"name":"Chair"}`), TTL: time.Minute},
		}))

		entries, err := store.GetMany(t.Context(), []string{"product:1", "product:2"})
		require.NoError(t, err)
		assert.Equal(t, []cache.Entry{
			{Value: []byte(`{"name":"Table"}`), RemainingTTL: time.Second, OK: true},
			{Value: []byte(`{"name":"Chair"}`), RemainingTTL: time.Minute, OK: true},
		}, entries)

		time.Sleep(2 * time.Second) // advances fake time past product:1's TTL

		entries, err = store.GetMany(t.Context(), []string{"product:1", "product:2"})
		require.NoError(t, err)
		assert.Equal(t, []cache.Entry{
			{},
			{Value: []byte(`{"name":"Chair"}`), RemainingTTL: 58 * time.Second, OK: true},
		}, entries)

		assert.Equal(t, []StoreOp{
			{
				Kind: "SetMany",
				Items: []StoreOpItem{
					{Key: "product:1", Value: `{"name":"Table"}`, TTL: time.Second},
					{Key: "product:2", Value: `{"name":"Chair"}`, TTL: time.Minute},
				},
			},
			{
				Kind: "GetMany",
				Keys: []string{"product:1", "product:2"},
				Hits: []bool{true, true},
			},
			{
				Kind: "GetMany",
				Keys: []string{"product:1", "product:2"},
				Hits: []bool{false, true},
			},
		}, store.Ops())
	})
}

// TestFakeStoreSeedDoesNotLog pins that Seed arranges state without polluting
// the op log.
func TestFakeStoreSeedDoesNotLog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := NewFakeStore()
		store.Seed("user:1", []byte(`{"username":"me"}`), time.Minute)

		entries, err := store.GetMany(t.Context(), []string{"user:1"})
		require.NoError(t, err)
		assert.Equal(t, []cache.Entry{
			{Value: []byte(`{"username":"me"}`), RemainingTTL: time.Minute, OK: true},
		}, entries)
		assert.Equal(t, []StoreOp{
			{
				Kind: "GetMany",
				Keys: []string{"user:1"},
				Hits: []bool{true},
			},
		}, store.Ops())
	})
}

// TestFakeStoreFailureInjection pins the injection knobs: exactly the next N
// calls fail, they touch no data, they still log (marked Failed), and the call
// after the budget behaves normally again.
func TestFakeStoreFailureInjection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := NewFakeStore()
		store.Seed("product:1", []byte(`{"name":"Table"}`), time.Minute)
		readErr := errors.New("store read down")
		writeErr := errors.New("store write down")
		store.FailGetMany(1, readErr)
		store.FailSetMany(1, writeErr)

		entries, err := store.GetMany(t.Context(), []string{"product:1"})
		assert.Equal(t, readErr, err)
		assert.Nil(t, entries)

		assert.Equal(t, writeErr, store.SetMany(t.Context(), []cache.Item{
			{Key: "product:2", Value: []byte(`{"name":"Chair"}`), TTL: time.Minute},
		}))
		_, stored := store.Value("product:2")
		assert.False(t, stored) // the failed write touched no data

		// The budget is spent: both ops work again.
		entries, err = store.GetMany(t.Context(), []string{"product:1"})
		require.NoError(t, err)
		assert.Equal(t, []cache.Entry{
			{Value: []byte(`{"name":"Table"}`), RemainingTTL: time.Minute, OK: true},
		}, entries)
		require.NoError(t, store.SetMany(t.Context(), []cache.Item{
			{Key: "product:2", Value: []byte(`{"name":"Chair"}`), TTL: time.Minute},
		}))
		value, stored := store.Value("product:2")
		require.True(t, stored)
		assert.Equal(t, []byte(`{"name":"Chair"}`), value)

		assert.Equal(t, []StoreOp{
			{
				Kind:   "GetMany",
				Keys:   []string{"product:1"},
				Failed: true,
			},
			{
				Kind: "SetMany",
				Items: []StoreOpItem{
					{Key: "product:2", Value: `{"name":"Chair"}`, TTL: time.Minute},
				},
				Failed: true,
			},
			{
				Kind: "GetMany",
				Keys: []string{"product:1"},
				Hits: []bool{true},
			},
			{
				Kind: "SetMany",
				Items: []StoreOpItem{
					{Key: "product:2", Value: `{"name":"Chair"}`, TTL: time.Minute},
				},
			},
		}, store.Ops())
	})
}

// TestGatedDataSourceOrdering proves the gate semantics: Load announces
// arrival, blocks until released (observable via synctest.Wait, never via
// latency), and then returns the canned response.
func TestGatedDataSourceOrdering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived := make(chan string, 1)
		release := make(chan struct{})
		counter := &atomic.Int64{}
		ds := &GatedDataSource{
			Name:        "products",
			Resp:        []byte(`{"data":{}}`),
			Arrived:     arrived,
			Release:     release,
			LoadCounter: counter,
		}

		var done atomic.Bool
		var out []byte
		var err error
		go func() {
			out, err = ds.Load(t.Context(), nil, []byte(`{}`))
			done.Store(true)
		}()

		synctest.Wait()
		assert.Equal(t, "products", <-arrived)
		assert.Equal(t, int64(1), counter.Load())
		assert.False(t, done.Load()) // still gated

		close(release)
		synctest.Wait()
		assert.True(t, done.Load())
		require.NoError(t, err)
		assert.Equal(t, []byte(`{"data":{}}`), out)
	})
}

// TestFakeRequestCacheRecordsCalls pins the full normalized Call log across
// all four hooks, including scripted decisions and the merge-path carrier.
func TestFakeRequestCacheRecordsCalls(t *testing.T) {
	handle := &resolve.FetchCacheHandle{Decision: resolve.DecisionSkipFullHit}
	fake := NewFakeRequestCache(map[string]ScriptedDecision{
		"topProducts": {Decision: resolve.DecisionSkipFullHit, Handle: handle},
	})

	item := &resolve.FetchItem{ResponsePath: "topProducts"}
	items := []*astjson.Value{astjson.MustParseBytes([]byte(`{"upc":"1"}`))}

	decision, gotHandle := fake.PrepareFetch(resolve.PrepareFetchInput{
		Item:       item,
		Items:      items,
		Input:      []byte(`{"query":"{topProducts{upc}}"}`),
		HeaderHash: 42,
	})
	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	assert.Same(t, handle, gotHandle)

	responseData := astjson.MustParseBytes([]byte(`{"topProducts":[{"upc":"1"}]}`))
	require.NoError(t, fake.OnFetchSkipped(handle, resolve.MergeInput{
		Item:  item,
		Items: items,
	}))
	require.NoError(t, fake.OnFetchResult(handle, resolve.MergeInput{
		Item:         item,
		Items:        items,
		ResponseData: responseData,
		MergePath:    []string{"nested"},
		HasErrors:    true,
		FetchFailed:  true,
		EmptyEntity:  true,
		StatusCode:   500,
	}))
	fake.EndRequest()

	assert.Equal(t, []Call{
		{
			Op:         "Prepare",
			FetchPath:  "topProducts",
			Items:      1,
			InputBytes: `{"query":"{topProducts{upc}}"}`,
			HeaderHash: 42,
			Decision:   resolve.DecisionSkipFullHit,
		},
		{
			Op:        "Skipped",
			FetchPath: "topProducts",
			Items:     1,
		},
		{
			Op:           "Result",
			FetchPath:    "topProducts",
			Items:        1,
			ResponseData: `{"topProducts":[{"upc":"1"}]}`,
			MergePath:    []string{"nested"},
			HasErrors:    true,
			FetchFailed:  true,
			EmptyEntity:  true,
			StatusCode:   500,
		},
		{Op: "End"},
	}, fake.Calls())

	assert.Equal(t, []*resolve.FetchCacheHandle{handle}, fake.ResultHandles())
}

// TestFakeRegistrySwapDataSources pins the response-key fallback order and the
// per-fetch load counting of swapped datasources.
func TestFakeRegistrySwapDataSources(t *testing.T) {
	reg := NewFakeRegistry(map[string]string{
		"products:topProducts": `{"data":{"topProducts":[]}}`,
		"reviews":              `{"data":{"_entities":[]}}`,
		"*":                    `{"data":{}}`,
	})

	single := &resolve.SingleFetch{
		Info: &resolve.FetchInfo{DataSourceName: "products"},
	}
	entity := &resolve.EntityFetch{
		Info: &resolve.FetchInfo{DataSourceName: "reviews"},
	}
	other := &resolve.BatchEntityFetch{
		Info: &resolve.FetchInfo{DataSourceName: "inventory"},
	}
	tree := &resolve.FetchTreeNode{
		Kind: resolve.FetchTreeNodeKindSequence,
		ChildNodes: []*resolve.FetchTreeNode{
			{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: single, ResponsePath: "topProducts"}},
			{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: entity, ResponsePath: "topProducts.reviews"}},
			{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: other, ResponsePath: "other"}},
		},
	}
	SwapDataSources(tree, reg)

	load := func(ds resolve.DataSource) string {
		t.Helper()
		out, err := ds.Load(t.Context(), nil, nil)
		require.NoError(t, err)
		return string(out)
	}

	assert.Equal(t, `{"data":{"topProducts":[]}}`, load(single.FetchConfiguration.DataSource)) // name:path key
	assert.Equal(t, `{"data":{"_entities":[]}}`, load(entity.DataSource))                      // name key
	assert.Equal(t, `{"data":{}}`, load(other.DataSource))                                     // "*" fallback

	assert.Equal(t, int64(1), reg.LoadCount("products", "topProducts"))
	assert.Equal(t, int64(1), reg.LoadCount("reviews", "topProducts.reviews"))
	assert.Equal(t, int64(1), reg.LoadCount("inventory", "other"))
	// A never-swapped name/path pair is -1, never 0: a typo cannot satisfy a
	// zero-loads assertion vacuously.
	assert.Equal(t, int64(-1), reg.LoadCount("products", "unknown"))
}
