package cachetesting

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestFakeRequestCacheRecordsPrepareAndResult(t *testing.T) {
	handle := &resolve.FetchCacheHandle{Decision: resolve.DecisionFetch}
	item := &resolve.FetchItem{ResponsePath: "data.product"}
	fake := NewFakeRequestCache(map[string]ScriptedDecision{
		"data.product": {
			Decision: resolve.DecisionFetch,
			Handle:   handle,
		},
	})

	decision, gotHandle := fake.PrepareFetch(resolve.PrepareFetchInput{
		Item:       item,
		Items:      []*astjson.Value{astjson.MustParseBytes([]byte(`{"id":"1"}`))},
		Input:      []byte(`{"query":"{product{id}}"}`),
		HeaderHash: 42,
	})
	if gotHandle != handle {
		t.Fatalf("PrepareFetch returned handle %p, want %p", gotHandle, handle)
	}
	assert.Equal(t, resolve.DecisionFetch, decision)

	response := astjson.MustParseBytes([]byte(`{"id":"1","name":"Table"}`))
	err := fake.OnFetchResult(gotHandle, resolve.MergeInput{
		Item:         item,
		Items:        []*astjson.Value{astjson.MustParseBytes([]byte(`{"id":"1"}`))},
		ResponseData: response,
		HasErrors:    true,
		StatusCode:   206,
	})
	assert.Equal(t, nil, err)
	assert.Equal(t, []*resolve.FetchCacheHandle{handle}, fake.ResultHandles())
	assert.Equal(t, []Call{
		{
			Op:         "Prepare",
			FetchPath:  "data.product",
			Items:      1,
			InputBytes: `{"query":"{product{id}}"}`,
			HeaderHash: 42,
			Decision:   resolve.DecisionFetch,
		},
		{
			Op:           "Result",
			FetchPath:    "data.product",
			Items:        1,
			ResponseData: `{"id":"1","name":"Table"}`,
			HasErrors:    true,
			StatusCode:   206,
		},
	}, fake.Calls())
}

func TestFakeStoreExpiresEntriesWithSynctestClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := NewFakeStore()
		store.Seed("entity:Product:1", []byte(`{"id":"1"}`), 30*time.Second)

		got, ok := store.Get("entity:Product:1")
		assert.Equal(t, true, ok)
		assert.Equal(t, `{"id":"1"}`, string(got.Value))

		time.Sleep(31 * time.Second)
		synctest.Wait()

		got, ok = store.Get("entity:Product:1")
		assert.Equal(t, false, ok)
		assert.Equal(t, StoredEntry{}, got)
		assert.Equal(t, []StoreOp{
			{Kind: "Get", Key: "entity:Product:1"},
			{Kind: "Get", Key: "entity:Product:1"},
		}, store.Ops())
	})
}

func TestGatedDataSourceBlocksUntilReleased(t *testing.T) {
	arrived := make(chan string, 1)
	release := make(chan struct{})
	done := make(chan []byte, 1)
	ds := &GatedDataSource{
		Name:    "products",
		Resp:    []byte(`{"data":{"product":{"id":"1"}}}`),
		Arrived: arrived,
		Release: release,
	}

	go func() {
		resp, err := ds.Load(t.Context(), nil, []byte(`{}`))
		assert.Equal(t, nil, err)
		done <- resp
	}()

	assert.Equal(t, "products", <-arrived)
	select {
	case resp := <-done:
		t.Fatalf("Load returned before release: %s", string(resp))
	default:
	}

	close(release)
	assert.Equal(t, []byte(`{"data":{"product":{"id":"1"}}}`), <-done)
}
