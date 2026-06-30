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

func TestControllerPrepareFetch_MultiKeyCandidates(t *testing.T) {
	t.Run("[E1] all candidates renderable at lookup", func(t *testing.T) {
		cfg := multiProductConfig(30*time.Second, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
		upcKey := renderedCandidateKey(t, cfg, item, 0)
		skuKey := renderedCandidateKey(t, cfg, item, 1)
		store := newTestStore()

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, &resolve.FetchCacheHandle{
			Decision: resolve.DecisionFetch,
			Items: []resolve.ItemCacheState{
				{Item: item, RenderedKeys: []string{upcKey, skuKey}},
			},
		}, handle)
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Get", Key: skuKey},
		}, store.ops)
	})

	t.Run("[E2] non-primary candidate hit is served", func(t *testing.T) {
		cfg := multiProductConfig(30*time.Second, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
		upcKey := renderedCandidateKey(t, cfg, item, 0)
		skuKey := renderedCandidateKey(t, cfg, item, 1)
		store := newTestStore()
		store.Seed(skuKey, []byte(`{"upc":"1","sku":"sku-1","name":"Table"}`), time.Minute)

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, &resolve.FetchCacheHandle{
			Decision:      resolve.DecisionSkipFullHit,
			WasHit:        true,
			MustWriteBack: true,
			Items: []resolve.ItemCacheState{
				{
					Item:                 item,
					FromCache:            handle.Items[0].FromCache,
					RenderedKeys:         []string{upcKey, skuKey},
					FromCacheCandidates:  []resolve.CacheCandidate{{Value: []byte(`{"upc":"1","sku":"sku-1","name":"Table"}`), RemainingTTL: handle.Items[0].SelectedRemainingTTL}},
					SelectedRemainingTTL: handle.Items[0].SelectedRemainingTTL,
				},
			},
		}, handle)
		require.NoError(t, rc.OnFetchSkipped(handle, mergeInput(item, nil, false, false)))
		rc.EndRequest()
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Get", Key: skuKey},
			{Kind: "Set", Key: upcKey, Value: `{"upc":"1","sku":"sku-1","name":"Table"}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})
}

func TestControllerOnFetchResult_MultiKeyBackfill(t *testing.T) {
	t.Run("[E3] rendered lookup key refreshes and pending candidate backfills after response", func(t *testing.T) {
		cfg := multiProductConfig(45*time.Second, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
		upcKey := renderedCandidateKey(t, cfg, parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`), 0)
		skuKey := renderedCandidateKey(t, cfg, parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`), 1)
		store := newTestStore()

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, &resolve.FetchCacheHandle{
			Decision: resolve.DecisionFetch,
			Items: []resolve.ItemCacheState{
				{Item: item, RenderedKeys: []string{upcKey}, PendingCandidates: []resolve.CacheKeyCandidate{cfg.KeySpec.Candidates[1]}},
			},
		}, handle)

		fresh := parseValue(t, `{"upc":"1","sku":"sku-1","name":"Table"}`)
		require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, fresh, false, false)))
		rc.EndRequest()

		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: upcKey, Value: `{"upc":"1","sku":"sku-1","name":"Table"}`, TTL: 45 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: skuKey, Value: `{"upc":"1","sku":"sku-1","name":"Table"}`, TTL: 45 * time.Second, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})

	t.Run("[E4] no candidates render at lookup but render after response", func(t *testing.T) {
		cfg := multiProductConfig(45*time.Second, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product"}`)
		fresh := parseValue(t, `{"upc":"1","sku":"sku-1","name":"Table"}`)
		upcKey := renderedCandidateKey(t, cfg, parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`), 0)
		skuKey := renderedCandidateKey(t, cfg, parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`), 1)
		store := newTestStore()

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, &resolve.FetchCacheHandle{
			Decision: resolve.DecisionFetch,
			Items: []resolve.ItemCacheState{
				{Item: item, PendingCandidates: []resolve.CacheKeyCandidate{cfg.KeySpec.Candidates[0], cfg.KeySpec.Candidates[1]}},
			},
		}, handle)

		require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, fresh, false, false)))
		rc.EndRequest()

		assert.Equal(t, []storeOp{
			{Kind: "Set", Key: upcKey, Value: `{"upc":"1","sku":"sku-1","name":"Table"}`, TTL: 45 * time.Second, Reason: resolve.CacheWriteReasonBackfill},
			{Kind: "Set", Key: skuKey, Value: `{"upc":"1","sku":"sku-1","name":"Table"}`, TTL: 45 * time.Second, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})

	t.Run("[E7] single key entity remains the one-candidate degenerate case", func(t *testing.T) {
		cfg := productConfig(30 * time.Second)
		item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
		key := renderedKey(t, cfg, item)
		store := newTestStore()

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, &resolve.FetchCacheHandle{
			Decision: resolve.DecisionFetch,
			Items: []resolve.ItemCacheState{
				{Item: item, RenderedKeys: []string{key}},
			},
		}, handle)
		assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
	})
}

func TestControllerOnFetchSkipped_MultiKeyBackfill(t *testing.T) {
	t.Run("[E5] pending candidate backfills from a read hit without network", func(t *testing.T) {
		cfg := multiProductConfig(30*time.Second, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
		served := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
		upcKey := renderedCandidateKey(t, cfg, served, 0)
		skuKey := renderedCandidateKey(t, cfg, served, 1)
		store := newTestStore()
		store.Seed(upcKey, []byte(`{"upc":"1","sku":"sku-1","name":"Table"}`), time.Minute)

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)
		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, true, handle.MustWriteBack)

		require.NoError(t, rc.OnFetchSkipped(handle, mergeInput(item, nil, false, false)))
		rc.EndRequest()

		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: skuKey, Value: `{"upc":"1","sku":"sku-1","name":"Table"}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})

	t.Run("[E6] refresh and backfill reasons are observable on store ops", func(t *testing.T) {
		cfg := multiProductConfig(30*time.Second, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product","upc":"1"}`)
		fresh := parseValue(t, `{"upc":"1","sku":"sku-1","name":"Table"}`)
		upcKey := renderedCandidateKey(t, cfg, parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`), 0)
		skuKey := renderedCandidateKey(t, cfg, parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`), 1)
		store := newTestStore()

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		_, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)
		require.NoError(t, rc.OnFetchResult(handle, mergeInput(item, fresh, false, false)))
		rc.EndRequest()

		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Set", Key: upcKey, Value: `{"upc":"1","sku":"sku-1","name":"Table"}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: skuKey, Value: `{"upc":"1","sku":"sku-1","name":"Table"}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonBackfill},
		}, store.ops)
	})
}

func TestControllerPrepareFetch_MultiCandidateFreshness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := multiProductConfig(time.Minute, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
		upcKey := renderedCandidateKey(t, cfg, item, 0)
		skuKey := renderedCandidateKey(t, cfg, item, 1)
		store := newTestStore()
		store.Seed(upcKey, []byte(`{"upc":"1","sku":"sku-1","name":"Older"}`), 20*time.Second)
		store.Seed(skuKey, []byte(`{"upc":"1","sku":"sku-1","name":"Fresh"}`), 50*time.Second)

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, 50*time.Second, handle.Items[0].SelectedRemainingTTL)
		assert.Equal(t, `{"upc":"1","sku":"sku-1","name":"Fresh"}`, valueString(handle.Items[0].FromCache))
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Get", Key: skuKey},
		}, store.ops)
	})
}

func TestControllerPrepareFetch_MultiCandidateTieBreaks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := multiProductConfig(time.Minute, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
		upcKey := renderedCandidateKey(t, cfg, item, 0)
		skuKey := renderedCandidateKey(t, cfg, item, 1)
		store := newTestStore()
		store.SeedRemainingTTL(upcKey, []byte(`{"upc":"1","sku":"sku-1","name":"Known"}`), time.Minute, 30*time.Second)
		store.SeedRemainingTTL(skuKey, []byte(`{"upc":"1","sku":"sku-1","name":"Unknown"}`), time.Minute, 0)

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		_, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)
		assert.Equal(t, `{"upc":"1","sku":"sku-1","name":"Known"}`, valueString(handle.Items[0].FromCache))
		assert.Equal(t, 30*time.Second, handle.Items[0].SelectedRemainingTTL)
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Get", Key: skuKey},
		}, store.ops)
	})

	synctest.Test(t, func(t *testing.T) {
		cfg := multiProductConfig(time.Minute, productProvidesFields("upc", "sku", "name"))
		item := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
		upcKey := renderedCandidateKey(t, cfg, item, 0)
		skuKey := renderedCandidateKey(t, cfg, item, 1)
		store := newTestStore()
		store.Seed(upcKey, []byte(`{"upc":"1","sku":"sku-1","name":"First"}`), 30*time.Second)
		store.Seed(skuKey, []byte(`{"upc":"1","sku":"sku-1","name":"Second"}`), 30*time.Second)

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		_, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)
		assert.Equal(t, `{"upc":"1","sku":"sku-1","name":"First"}`, valueString(handle.Items[0].FromCache))
		assert.Equal(t, 30*time.Second, handle.Items[0].SelectedRemainingTTL)
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Get", Key: skuKey},
		}, store.ops)
	})
}

func TestControllerPrepareFetch_MergeSynthesisAndFallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := multiProductConfig(40*time.Second, productProvidesFields("upc", "name", "price"))
		item := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
		upcKey := renderedCandidateKey(t, cfg, item, 0)
		skuKey := renderedCandidateKey(t, cfg, item, 1)
		store := newTestStore()
		store.Seed(upcKey, []byte(`{"price":10,"upc":"1"}`), 20*time.Second)
		store.Seed(skuKey, []byte(`{"name":"Table","sku":"sku-1"}`), 50*time.Second)

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, true, handle.MustWriteBack)
		assert.Equal(t, true, handle.Items[0].NeedsWriteback)
		assert.Equal(t, `{"upc":"1","name":"Table","price":10,"sku":"sku-1"}`, valueString(handle.Items[0].FromCache))
		require.NoError(t, rc.OnFetchSkipped(handle, mergeInput(item, nil, false, false)))
		rc.EndRequest()
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Get", Key: skuKey},
			{Kind: "Set", Key: upcKey, Value: `{"upc":"1","name":"Table","price":10,"sku":"sku-1"}`, TTL: 40 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
			{Kind: "Set", Key: skuKey, Value: `{"upc":"1","name":"Table","price":10,"sku":"sku-1"}`, TTL: 40 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
		}, store.ops)
	})

	synctest.Test(t, func(t *testing.T) {
		cfg := multiProductConfig(40*time.Second, productProvidesFields("upc", "name", "price"))
		item := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
		upcKey := renderedCandidateKey(t, cfg, item, 0)
		skuKey := renderedCandidateKey(t, cfg, item, 1)
		store := newTestStore()
		store.Seed(upcKey, []byte(`{"upc":"1","name":"Covering","price":10}`), 20*time.Second)
		store.Seed(skuKey, []byte(`{"sku":"sku-1","price":{"bad":true}}`), 50*time.Second)

		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, true, handle.MustWriteBack)
		assert.Equal(t, true, handle.Items[0].NeedsWriteback)
		assert.Equal(t, 20*time.Second, handle.Items[0].SelectedRemainingTTL)
		assert.Equal(t, `{"upc":"1","name":"Covering","price":10}`, valueString(handle.Items[0].FromCache))
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: upcKey},
			{Kind: "Get", Key: skuKey},
		}, store.ops)
	})
}

func TestControllerPrepareFetch_ReorderToProvidesOrder(t *testing.T) {
	cfg := multiProductConfig(30*time.Second, productProvidesFields("name", "upc", "price"))
	item := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
	upcKey := renderedCandidateKey(t, cfg, item, 0)
	skuKey := renderedCandidateKey(t, cfg, item, 1)
	store := newTestStore()
	store.Seed(upcKey, []byte(`{"price":10,"extra":"kept","upc":"1","name":"Table"}`), time.Minute)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(prepareInput(t, cfg, item))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	assert.Equal(t, `{"name":"Table","upc":"1","price":10,"extra":"kept"}`, valueString(handle.Items[0].FromCache))
	require.NoError(t, rc.OnFetchSkipped(handle, mergeInput(item, nil, false, false)))
	assert.Equal(t, `{"__typename":"Product","upc":"1","sku":"sku-1","name":"Table","price":10,"extra":"kept"}`, valueString(item))
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: upcKey},
		{Kind: "Get", Key: skuKey},
	}, store.ops)
}

func TestControllerPrepareFetch_AllOrNothingReduction(t *testing.T) {
	cfg := multiProductConfig(30*time.Second, productProvidesFields("upc", "sku", "name"))
	first := parseValue(t, `{"__typename":"Product","upc":"1","sku":"sku-1"}`)
	second := parseValue(t, `{"__typename":"Product","upc":"2","sku":"sku-2"}`)
	firstUPC := renderedCandidateKey(t, cfg, first, 0)
	firstSKU := renderedCandidateKey(t, cfg, first, 1)
	secondUPC := renderedCandidateKey(t, cfg, second, 0)
	secondSKU := renderedCandidateKey(t, cfg, second, 1)

	t.Run("[D12] all items covered skips full fetch", func(t *testing.T) {
		store := newTestStore()
		store.Seed(firstUPC, []byte(`{"upc":"1","sku":"sku-1","name":"Table"}`), time.Minute)
		store.Seed(secondSKU, []byte(`{"upc":"2","sku":"sku-2","name":"Chair"}`), time.Minute)
		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		in := prepareInput(t, cfg, first)
		in.Items = []*astjson.Value{first, second}

		decision, handle := rc.PrepareFetch(in)
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionSkipFullHit, decision)
		assert.Equal(t, true, handle.WasHit)
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: firstUPC},
			{Kind: "Get", Key: firstSKU},
			{Kind: "Get", Key: secondUPC},
			{Kind: "Get", Key: secondSKU},
		}, store.ops)
	})

	t.Run("[D13] one uncovered item fetches whole request", func(t *testing.T) {
		store := newTestStore()
		store.Seed(firstUPC, []byte(`{"upc":"1","sku":"sku-1","name":"Table"}`), time.Minute)
		rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
		in := prepareInput(t, cfg, first)
		in.Items = []*astjson.Value{first, second}

		decision, handle := rc.PrepareFetch(in)
		require.NotNil(t, handle)

		assert.Equal(t, resolve.DecisionFetch, decision)
		assert.Equal(t, false, handle.WasHit)
		assert.Equal(t, []storeOp{
			{Kind: "Get", Key: firstUPC},
			{Kind: "Get", Key: firstSKU},
			{Kind: "Get", Key: secondUPC},
			{Kind: "Get", Key: secondSKU},
		}, store.ops)
	})
}

func multiProductConfig(ttl time.Duration, provides *resolve.Object) *resolve.FetchCacheConfig {
	return &resolve.FetchCacheConfig{
		L2:        true,
		CacheName: "entity:products",
		TTL:       ttl,
		KeySpec: resolve.CacheKeySpec{
			Scope:    resolve.CacheScopeEntity,
			TypeName: "Product",
			Candidates: []resolve.CacheKeyCandidate{
				{Representation: productRepresentation()},
				{Representation: productSKURepresentation()},
			},
		},
		ProvidesData: provides,
	}
}

func productSKURepresentation() *resolve.Object {
	return &resolve.Object{
		TypeName: "Product",
		Fields: []*resolve.Field{
			{Name: []byte("__typename"), Value: &resolve.Scalar{}},
			{Name: []byte("sku"), Value: &resolve.Scalar{}},
		},
	}
}

func productProvidesFields(names ...string) *resolve.Object {
	fields := make([]*resolve.Field, 0, len(names))
	for _, name := range names {
		fields = append(fields, &resolve.Field{Name: []byte(name), Value: &resolve.Scalar{}})
	}
	return &resolve.Object{Fields: fields}
}

func renderedCandidateKey(t *testing.T, cfg *resolve.FetchCacheConfig, item *astjson.Value, candidate int) string {
	t.Helper()
	arena := newTestMergeArena()
	session := arena.Begin()
	defer session.Close()
	key, ok := renderEntityKey(session, cfg.KeySpec.Candidates[candidate].Representation, item, cacheKeyPrefix(cfg, 0))
	require.Equal(t, true, ok)
	return key
}
