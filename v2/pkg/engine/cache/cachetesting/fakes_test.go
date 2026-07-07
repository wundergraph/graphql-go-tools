package cachetesting

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestFakeStoreTTLExpiry proves TTL semantics against the real clock inside a
// synctest bubble: a value is served before its TTL and is a miss after the
// bubble's fake time passes it, with the full op log recorded.
func TestFakeStoreTTLExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := NewFakeStore()
		store.Set("product:1", []byte(`{"name":"Table"}`), time.Second)

		entry, ok := store.Get("product:1")
		require.True(t, ok)
		assert.Equal(t, []byte(`{"name":"Table"}`), entry.Value)

		time.Sleep(2 * time.Second) // advances fake time past the TTL

		_, ok = store.Get("product:1")
		assert.False(t, ok)

		assert.Equal(t, []StoreOp{
			{Kind: "Set", Key: "product:1", Value: `{"name":"Table"}`, TTL: time.Second},
			{Kind: "Get", Key: "product:1"},
			{Kind: "Get", Key: "product:1"},
		}, store.Ops())
	})
}

// TestFakeStoreSeedDoesNotLog pins that Seed arranges state without polluting
// the op log.
func TestFakeStoreSeedDoesNotLog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := NewFakeStore()
		store.Seed("user:1", []byte(`{"username":"me"}`), time.Minute)

		entry, ok := store.Get("user:1")
		require.True(t, ok)
		assert.Equal(t, []byte(`{"username":"me"}`), entry.Value)
		assert.Equal(t, []StoreOp{
			{Kind: "Get", Key: "user:1"},
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
