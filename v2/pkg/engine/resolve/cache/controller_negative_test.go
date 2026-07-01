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

func TestControllerNegative_OnFetchResultWritesNullSentinelForEmptyEntity(t *testing.T) {
	cfg := productConfig(time.Minute)
	cfg.NegativeCacheTTL = 7 * time.Second
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	_, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	require.NoError(t, rc.OnFetchResult(handle, negativeMergeInput(item, parseValue(t, `null`), false, false, true)))
	assert.Equal(t, true, handle.Items[0].NegativeHit)
	require.NotNil(t, handle.Items[0].FromCache)
	assert.Equal(t, astjson.TypeNull, handle.Items[0].FromCache.Type())
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)

	rc.EndRequest()
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `null`, TTL: 7 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
	}, store.ops)
}

func TestControllerNegative_OnFetchResultSkipsWriteWhenNegativeTTLZero(t *testing.T) {
	cfg := productConfig(time.Minute)
	cfg.NegativeCacheTTL = 0
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	_, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	require.NoError(t, rc.OnFetchResult(handle, negativeMergeInput(item, parseValue(t, `null`), false, false, true)))
	rc.EndRequest()

	assert.Equal(t, false, handle.Items[0].NegativeHit)
	assert.Equal(t, (*astjson.Value)(nil), handle.Items[0].FromCache)
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
}

func TestControllerNegative_PrepareFetchServesNullSentinel(t *testing.T) {
	cfg := productConfig(time.Minute)
	cfg.NegativeCacheTTL = 9 * time.Second
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()
	store.Seed(key, []byte(`null`), cfg.NegativeCacheTTL)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	require.NotNil(t, handle.Items[0].FromCache)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionSkipFullHit,
		WasHit:   true,
		Items: []resolve.ItemCacheState{
			{
				Item:                 item,
				FromCache:            handle.Items[0].FromCache,
				RenderedKeys:         []string{key},
				FromCacheCandidates:  []resolve.CacheCandidate{{Value: []byte(`null`), RemainingTTL: handle.Items[0].SelectedRemainingTTL}},
				SelectedRemainingTTL: handle.Items[0].SelectedRemainingTTL,
				NegativeHit:          true,
			},
		},
	}, handle)
	assert.Equal(t, astjson.TypeNull, handle.Items[0].FromCache.Type())

	require.NoError(t, rc.OnFetchSkipped(handle, negativeMergeInput(item, nil, false, false, false)))
	assert.Equal(t, `null`, valueString(item))
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
}

func TestControllerNegative_ExpiredNullSentinelIsMiss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := productConfig(time.Minute)
		cfg.NegativeCacheTTL = 11 * time.Second
		item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
		key := renderedKey(t, cfg, item)
		store := newTestStore()
		store.Seed(key, []byte(`null`), cfg.NegativeCacheTTL)

		time.Sleep(cfg.NegativeCacheTTL + time.Nanosecond)
		synctest.Wait()

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, false, handle.Items[0].NegativeHit)
		assert.Equal(t, (*astjson.Value)(nil), handle.Items[0].FromCache)
		assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
	})
}

func TestControllerNegative_OnFetchResultFetchFailedWinsOverEmptyEntity(t *testing.T) {
	cfg := productConfig(time.Minute)
	cfg.NegativeCacheTTL = 13 * time.Second
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	_, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	require.NoError(t, rc.OnFetchResult(handle, negativeMergeInput(item, parseValue(t, `null`), true, false, true)))
	rc.EndRequest()

	assert.Equal(t, false, handle.Items[0].NegativeHit)
	assert.Equal(t, (*astjson.Value)(nil), handle.Items[0].FromCache)
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
}

func TestControllerNegative_OnFetchSkippedKeepsNullBubbleSuppression(t *testing.T) {
	cfg := productConfig(time.Minute)
	cfg.NegativeCacheTTL = 15 * time.Second
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()
	store.Seed(key, []byte(`null`), cfg.NegativeCacheTTL)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	require.NoError(t, rc.OnFetchSkipped(handle, negativeMergeInput(item, nil, false, false, false)))
	assert.Equal(t, `null`, valueString(item))
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
}

func negativeMergeInput(item, responseData *astjson.Value, fetchFailed, hasErrors, emptyEntity bool) resolve.MergeInput {
	in := mergeInput(item, responseData, fetchFailed, hasErrors)
	in.EmptyEntity = emptyEntity
	return in
}
