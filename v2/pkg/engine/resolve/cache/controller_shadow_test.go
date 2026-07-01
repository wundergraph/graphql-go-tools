package cache

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type shadowCompare struct {
	CacheKey   string
	EntityType string
	IsFresh    bool
	CacheAge   time.Duration
}

type shadowRecordingObserver struct {
	compares []shadowCompare
}

func (o *shadowRecordingObserver) BeginRequest(ctx *resolve.Context) {}

func (o *shadowRecordingObserver) EndRequest(ctx *resolve.Context) {}

func (o *shadowRecordingObserver) OnFetchObserved(h *resolve.FetchCacheHandle) {}

func (o *shadowRecordingObserver) CompareShadow(h *resolve.FetchCacheHandle, fresh *astjson.Value, s resolve.MergeSession) {
	if h == nil {
		return
	}
	compares := make([]shadowCompare, 0, len(h.ShadowStash))
	for itemIndex, entry := range h.ShadowStash {
		freshValue := shadowTestFreshValue(h, itemIndex, fresh)
		compares = append(compares, shadowCompare{
			CacheKey:   entry.CacheKey,
			EntityType: "Product",
			IsFresh:    string(entry.CachedValue.MarshalTo(nil)) == string(freshValue.MarshalTo(nil)),
			CacheAge:   entry.CacheTTL - entry.RemainingTTL,
		})
	}
	o.compares = append(o.compares, compares...)
}

func (o *shadowRecordingObserver) OnEntity(h *resolve.FetchCacheHandle, entity *astjson.Value) {}

func (o *shadowRecordingObserver) OnFieldValue(coordinate resolve.GraphCoordinate, value resolve.FieldValue) {
}

func (o *shadowRecordingObserver) Compares() []shadowCompare {
	return slices.Clone(o.compares)
}

func shadowTestFreshValue(h *resolve.FetchCacheHandle, itemIndex int, fresh *astjson.Value) *astjson.Value {
	item := resolve.ItemCacheState{}
	if itemIndex >= 0 && itemIndex < len(h.Items) {
		item = h.Items[itemIndex]
	}
	freshValue := fresh
	if h.BatchEntityKey {
		if batch := fresh.GetArray(); batch != nil {
			batchIndex := itemIndex
			if item.BatchIndex >= 0 {
				batchIndex = item.BatchIndex
			}
			if batchIndex >= 0 && batchIndex < len(batch) {
				freshValue = batch[batchIndex]
			}
		}
	}
	if len(item.EntityMergePath) > 0 {
		if entity := freshValue.Get(item.EntityMergePath...); entity != nil {
			freshValue = entity
		}
	}
	return freshValue
}

func TestControllerShadow_PrepareFetchStashesL2HitAndForcesFetch(t *testing.T) {
	cfg := productConfig(30 * time.Second)
	cfg.ShadowMode = true
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()
	store.Seed(key, []byte(`{"upc":"1","name":"cached"}`), cfg.TTL)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))

	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))

	require.NotNil(t, handle)
	require.Equal(t, resolve.DecisionFetchShadow, decision)
	assert.Equal(t, resolve.DecisionFetchShadow, handle.Decision)
	assert.Equal(t, false, handle.WasHit)
	assert.Equal(t, true, handle.Shadow)
	assert.Equal(t, (*astjson.Value)(nil), handle.Items[0].FromCache)
	assert.Equal(t, map[int]resolve.ShadowCacheEntry{
		0: {
			CachedValue:  handle.ShadowStash[0].CachedValue,
			CacheKey:     key,
			RemainingTTL: handle.ShadowStash[0].RemainingTTL,
			CacheTTL:     cfg.TTL,
		},
	}, handle.ShadowStash)
	assert.Equal(t, `{"upc":"1","name":"cached"}`, valueString(handle.ShadowStash[0].CachedValue))
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)

	fresh := parseValue(t, `{"upc":"1","name":"fresh"}`)
	require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, fresh, false, false)))
	rc.EndRequest()

	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"upc":"1","name":"fresh"}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
	}, store.ops)
}

func TestControllerShadow_CompareMatchRunsBeforeWriteAndRecordsAge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := productConfig(time.Minute)
		cfg.ShadowMode = true
		item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
		key := renderedKey(t, cfg, item)
		store := newTestStore()
		store.Seed(key, []byte(`{"upc":"1","name":"Table"}`), cfg.TTL)
		time.Sleep(17 * time.Second)
		observer := &shadowRecordingObserver{}
		rc := NewController(store, ModeL2, observer).BeginRequest(resolve.NewContext(t.Context()))

		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.Equal(t, resolve.DecisionFetchShadow, decision)

		fresh := parseValue(t, `{"upc":"1","name":"Table"}`)
		require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, fresh, false, false)))

		assert.Equal(t, []shadowCompare{{
			CacheKey:   key,
			EntityType: "Product",
			IsFresh:    true,
			CacheAge:   17 * time.Second,
		}}, observer.Compares())
		assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)

		rc.EndRequest()

		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: key},
			{Kind: "Set", Key: key, Value: `{"upc":"1","name":"Table"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
		}, store.ops)
	})
}

func TestControllerShadow_CompareMismatchStillOverwritesL2WithFresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := productConfig(time.Minute)
		cfg.ShadowMode = true
		item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
		key := renderedKey(t, cfg, item)
		store := newTestStore()
		store.Seed(key, []byte(`{"upc":"1","name":"stale"}`), cfg.TTL)
		time.Sleep(11 * time.Second)
		observer := &shadowRecordingObserver{}
		rc := NewController(store, ModeL2, observer).BeginRequest(resolve.NewContext(t.Context()))

		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.Equal(t, resolve.DecisionFetchShadow, decision)

		fresh := parseValue(t, `{"upc":"1","name":"fresh"}`)
		require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, fresh, false, false)))
		rc.EndRequest()

		assert.Equal(t, []shadowCompare{{
			CacheKey:   key,
			EntityType: "Product",
			IsFresh:    false,
			CacheAge:   11 * time.Second,
		}}, observer.Compares())
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: key},
			{Kind: "Set", Key: key, Value: `{"upc":"1","name":"fresh"}`, TTL: time.Minute, Reason: resolve.CacheWriteReasonRefresh},
		}, store.ops)
	})
}

func TestControllerShadow_NilObserverForceFetchesAndOverwritesWithoutCompare(t *testing.T) {
	cfg := productConfig(30 * time.Second)
	cfg.ShadowMode = true
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()
	store.Seed(key, []byte(`{"upc":"1","name":"cached"}`), cfg.TTL)
	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))

	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.Equal(t, resolve.DecisionFetchShadow, decision)
	require.NotNil(t, handle)

	require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, parseValue(t, `{"upc":"1","name":"fresh"}`), false, false)))
	rc.EndRequest()

	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"upc":"1","name":"fresh"}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
	}, store.ops)
}

func TestControllerShadow_NilProvidesDataForceFetchesAndSkipsCompare(t *testing.T) {
	cfg := productConfig(30 * time.Second)
	cfg.ShadowMode = true
	cfg.ProvidesData = nil
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)
	store := newTestStore()
	store.Seed(key, []byte(`{"upc":"1","name":"cached"}`), cfg.TTL)
	observer := &shadowRecordingObserver{}
	rc := NewController(store, ModeL2, observer).BeginRequest(resolve.NewContext(t.Context()))

	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.Equal(t, resolve.DecisionFetchShadow, decision)
	require.NotNil(t, handle)

	require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, parseValue(t, `{"upc":"1","name":"fresh"}`), false, false)))
	rc.EndRequest()

	assert.Equal(t, []shadowCompare(nil), observer.Compares())
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"upc":"1","name":"fresh"}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
	}, store.ops)
}

func TestControllerShadow_NoopAndL1ModesNeverReturnFetchShadow(t *testing.T) {
	cfg := productConfig(30 * time.Second)
	cfg.ShadowMode = true
	item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
	key := renderedKey(t, cfg, item)

	tests := []struct {
		name     string
		mode     Mode
		expected []storeOp
	}{
		{name: "noop", mode: ModeNoop, expected: nil},
		{name: "l1", mode: ModeL1, expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore()
			store.Seed(key, []byte(`{"upc":"1","name":"cached"}`), cfg.TTL)
			rc := NewController(store, tt.mode, nil).BeginRequest(resolve.NewContext(t.Context()))

			decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))

			assert.Equal(t, resolve.DecisionFetch, decision)
			assert.Equal(t, (*resolve.FetchCacheHandle)(nil), handle)
			assert.Equal(t, tt.expected, store.ops)
		})
	}
}
