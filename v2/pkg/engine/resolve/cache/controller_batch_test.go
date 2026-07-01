package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestControllerBatch_FullHitFansOutToAllTargets(t *testing.T) {
	store := newTestStore()
	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	cfg := productConfig(time.Minute)
	first := parseValue(t, `{"upc":"1"}`)
	firstDuplicate := parseValue(t, `{"upc":"1","alias":"copy"}`)
	second := parseValue(t, `{"upc":"2"}`)
	firstKey := renderedKey(t, cfg, first)
	secondKey := renderedKey(t, cfg, second)
	store.Seed(firstKey, []byte(`{"upc":"1","name":"Table"}`), time.Minute)
	store.Seed(secondKey, []byte(`{"upc":"2","name":"Chair"}`), time.Minute)

	in := batchPrepareInput(t, cfg, [][]*astjson.Value{
		{first, firstDuplicate},
		{second},
	})
	decision, handle := rc.PrepareFetch(in)

	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	require.NoError(t, rc.OnFetchSkipped(handle, batchMergeInput(in.BatchStats, nil, false, false)))
	assert.Equal(t, `{"upc":"1","name":"Table"}`, valueString(first))
	assert.Equal(t, `{"upc":"1","alias":"copy","name":"Table"}`, valueString(firstDuplicate))
	assert.Equal(t, `{"upc":"2","name":"Chair"}`, valueString(second))
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: firstKey},
		{Kind: "Get", Key: secondKey},
	}, store.ops)
}

func TestControllerBatch_AllMissWritesEachBucketFromResponseArray(t *testing.T) {
	store := newTestStore()
	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	cfg := productConfig(time.Minute)
	first := parseValue(t, `{"upc":"1"}`)
	second := parseValue(t, `{"upc":"2"}`)
	firstKey := renderedKey(t, cfg, first)
	secondKey := renderedKey(t, cfg, second)

	in := batchPrepareInput(t, cfg, [][]*astjson.Value{{first}, {second}})
	decision, handle := rc.PrepareFetch(in)

	assert.Equal(t, resolve.DecisionFetch, decision)
	require.NoError(t, rc.OnFetchResult(handle, batchMergeInput(in.BatchStats, parseValue(t, `[
		{"upc":"1","name":"Table"},
		{"upc":"2","name":"Chair"}
	]`), false, false)))
	rc.EndRequest()
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: firstKey},
		{Kind: "Get", Key: secondKey},
		{Kind: "Set", Key: firstKey, Value: `{"upc":"1","name":"Table"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
		{Kind: "Set", Key: secondKey, Value: `{"upc":"2","name":"Chair"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
	}, store.ops)
}

func TestControllerBatch_MixedHitDoesFullRefetchAndWritesEveryBucket(t *testing.T) {
	store := newTestStore()
	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	cfg := productConfig(time.Minute)
	first := parseValue(t, `{"upc":"1"}`)
	second := parseValue(t, `{"upc":"2"}`)
	firstKey := renderedKey(t, cfg, first)
	secondKey := renderedKey(t, cfg, second)
	store.Seed(firstKey, []byte(`{"upc":"1","name":"Stale Table"}`), time.Minute)

	in := batchPrepareInput(t, cfg, [][]*astjson.Value{{first}, {second}})
	decision, handle := rc.PrepareFetch(in)

	assert.Equal(t, resolve.DecisionFetch, decision)
	require.NoError(t, rc.OnFetchResult(handle, batchMergeInput(in.BatchStats, parseValue(t, `[
		{"upc":"1","name":"Fresh Table"},
		{"upc":"2","name":"Chair"}
	]`), false, false)))
	rc.EndRequest()
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: firstKey},
		{Kind: "Get", Key: secondKey},
		{Kind: "Set", Key: firstKey, Value: `{"upc":"1","name":"Fresh Table"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
		{Kind: "Set", Key: secondKey, Value: `{"upc":"2","name":"Chair"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
	}, store.ops)
}

func TestControllerBatch_EmptyBatchShortCircuitsWithoutGet(t *testing.T) {
	store := newTestStore()
	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	cfg := productConfig(time.Minute)

	decision, handle := rc.PrepareFetch(resolve.PrepareFetchInput{
		Ctx:        resolve.NewContext(t.Context()),
		Config:     cfg,
		BatchStats: [][]*astjson.Value{},
		Arena:      newTestMergeArena(),
	})

	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Equal(t, (*resolve.FetchCacheHandle)(nil), handle)
	assert.Equal(t, []storeOp(nil), store.ops)
}

func TestControllerBatch_StateMarksBatchEntityKeyAndIndex(t *testing.T) {
	store := newTestStore()
	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	cfg := productConfig(time.Minute)
	first := parseValue(t, `{"upc":"1"}`)
	second := parseValue(t, `{"upc":"2"}`)

	in := batchPrepareInput(t, cfg, [][]*astjson.Value{{first}, {second}})
	_, handle := rc.PrepareFetch(in)

	require.NotNil(t, handle)
	assert.Equal(t, true, handle.BatchEntityKey)
	assert.Equal(t, []resolve.ItemCacheState{
		{
			Item:              first,
			RenderedKeys:      []string{renderedKey(t, cfg, first)},
			BatchEntityKey:    true,
			BatchIndex:        0,
			EntityMergePath:   nil,
			PendingCandidates: nil,
		},
		{
			Item:              second,
			RenderedKeys:      []string{renderedKey(t, cfg, second)},
			BatchEntityKey:    true,
			BatchIndex:        1,
			EntityMergePath:   nil,
			PendingCandidates: nil,
		},
	}, handle.Items)
}

func batchPrepareInput(t *testing.T, cfg *resolve.FetchCacheConfig, batchStats [][]*astjson.Value) resolve.PrepareFetchInput {
	t.Helper()
	return resolve.PrepareFetchInput{
		Ctx:        resolve.NewContext(t.Context()),
		Config:     cfg,
		BatchStats: batchStats,
		Arena:      newTestMergeArena(),
	}
}

func batchMergeInput(batchStats [][]*astjson.Value, responseData *astjson.Value, fetchFailed, hasErrors bool) resolve.MergeInput {
	return resolve.MergeInput{
		BatchStats:   batchStats,
		ResponseData: responseData,
		FetchFailed:  fetchFailed,
		HasErrors:    hasErrors,
		Arena:        newTestMergeArena(),
	}
}
