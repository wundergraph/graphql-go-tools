package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestControllerRootField_MissWritesWholeResponseUnderInputKey(t *testing.T) {
	cfg := rootFieldConfig(25 * time.Second)
	input := []byte(`query TopProducts { topProducts { upc name } }`)
	target := parseValue(t, `{}`)
	key := rootFieldKeyForTest(cfg, input, 0)
	store := newTestStore()

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(rootFieldPrepareInput(t, cfg, target, input, 0))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionFetch,
		Items: []resolve.ItemCacheState{
			{Item: target, RenderedKeys: []string{key}},
		},
	}, handle)

	fresh := parseValue(t, `{"topProducts":[{"upc":"1","name":"Table"},{"upc":"2","name":"Chair"}]}`)
	require.NoError(t, rc.OnFetchResult(handle, mergeInput(target, fresh, false, false)))
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)

	rc.EndRequest()
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"topProducts":[{"upc":"1","name":"Table"},{"upc":"2","name":"Chair"}]}`, TTL: 25 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
	}, store.ops)
}

func TestControllerRootField_HitSkipsAndSplicesWholeResponse(t *testing.T) {
	cfg := rootFieldConfig(30 * time.Second)
	input := []byte(`query TopProducts { topProducts { upc name } }`)
	target := parseValue(t, `{}`)
	cached := []byte(`{"topProducts":[{"name":"Table","upc":"1"}]}`)
	key := rootFieldKeyForTest(cfg, input, 0)
	store := newTestStore()
	store.Seed(key, cached, time.Minute)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(rootFieldPrepareInput(t, cfg, target, input, 0))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionSkipFullHit,
		WasHit:   true,
		Items: []resolve.ItemCacheState{
			{
				Item:                 target,
				FromCache:            handle.Items[0].FromCache,
				RenderedKeys:         []string{key},
				FromCacheCandidates:  []resolve.CacheCandidate{{Value: cached, RemainingTTL: handle.Items[0].SelectedRemainingTTL}},
				SelectedRemainingTTL: handle.Items[0].SelectedRemainingTTL,
			},
		},
	}, handle)

	require.NoError(t, rc.OnFetchSkipped(handle, mergeInput(target, nil, false, false)))
	assert.Equal(t, `{"topProducts":[{"upc":"1","name":"Table"}]}`, valueString(target))
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
}

func TestControllerRootField_CoverageFailFetches(t *testing.T) {
	cfg := rootFieldConfig(time.Minute)
	input := []byte(`query TopProducts { topProducts { upc name } }`)
	target := parseValue(t, `{}`)
	key := rootFieldKeyForTest(cfg, input, 0)
	store := newTestStore()
	store.Seed(key, []byte(`{"topProducts":[{"upc":"1"}]}`), time.Minute)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(rootFieldPrepareInput(t, cfg, target, input, 0))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Equal(t, (*astjson.Value)(nil), handle.Items[0].FromCache)
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
}

func TestControllerRootField_OnFetchResultWriteGate(t *testing.T) {
	tests := []struct {
		name        string
		response    *astjson.Value
		fetchFailed bool
		hasErrors   bool
	}{
		{
			name:     "clean success writes",
			response: parseValue(t, `{"topProducts":[{"upc":"1","name":"Table"}]}`),
		},
		{
			name:        "fetch failure writes nothing",
			fetchFailed: true,
		},
		{
			name:      "GraphQL errors write nothing",
			response:  parseValue(t, `{"topProducts":[{"upc":"1","name":"Table"}]}`),
			hasErrors: true,
		},
		{
			name:     "JSON null writes nothing",
			response: parseValue(t, `null`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := rootFieldConfig(20 * time.Second)
			input := []byte(`query TopProducts { topProducts { upc name } }`)
			target := parseValue(t, `{}`)
			key := rootFieldKeyForTest(cfg, input, 0)
			store := newTestStore()

			rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
			_, handle := rc.PrepareFetch(rootFieldPrepareInput(t, cfg, target, input, 0))
			require.NoError(t, rc.OnFetchResult(handle, mergeInput(target, tt.response, tt.fetchFailed, tt.hasErrors)))
			rc.EndRequest()

			expected := []storeOp{{Kind: "Get", Key: key}}
			if tt.name == "clean success writes" {
				expected = append(expected, storeOp{Kind: "Set", Key: key, Value: `{"topProducts":[{"upc":"1","name":"Table"}]}`, TTL: 20 * time.Second, Reason: resolve.CacheWriteReasonRefresh})
			}
			assert.Equal(t, expected, store.ops)
		})
	}
}

func TestControllerRootField_ShadowForceRefetchOverwritesWithoutCompare(t *testing.T) {
	cfg := rootFieldConfig(30 * time.Second)
	cfg.ShadowMode = true
	input := []byte(`query TopProducts { topProducts { upc name } }`)
	target := parseValue(t, `{}`)
	key := rootFieldKeyForTest(cfg, input, 0)
	store := newTestStore()
	store.Seed(key, []byte(`{"topProducts":[{"upc":"1","name":"cached"}]}`), cfg.TTL)
	observer := &shadowRecordingObserver{}

	rc := NewController(store, ModeL2, observer).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(rootFieldPrepareInput(t, cfg, target, input, 0))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionFetchShadow, decision)
	assert.Equal(t, resolve.DecisionFetchShadow, handle.Decision)
	assert.Equal(t, false, handle.WasHit)
	assert.Equal(t, true, handle.Shadow)
	assert.Equal(t, map[int]resolve.ShadowCacheEntry{
		0: {
			CachedValue:  handle.ShadowStash[0].CachedValue,
			CacheKey:     key,
			RemainingTTL: handle.ShadowStash[0].RemainingTTL,
			CacheTTL:     cfg.TTL,
		},
	}, handle.ShadowStash)
	assert.Equal(t, `{"topProducts":[{"upc":"1","name":"cached"}]}`, valueString(handle.ShadowStash[0].CachedValue))

	fresh := parseValue(t, `{"topProducts":[{"upc":"1","name":"fresh"}]}`)
	require.NoError(t, rc.OnFetchResult(handle, mergeInput(target, fresh, false, false)))
	assert.Equal(t, []shadowCompare(nil), observer.Compares())

	rc.EndRequest()
	assert.Equal(t, []shadowCompare(nil), observer.Compares())
	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"topProducts":[{"upc":"1","name":"fresh"}]}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
	}, store.ops)
}

func TestControllerRootField_ModeMatrixNoopAndL2HitAreDataEqual(t *testing.T) {
	cfg := rootFieldConfig(time.Minute)
	input := []byte(`query TopProducts { topProducts { upc name } }`)
	cached := []byte(`{"topProducts":[{"upc":"1","name":"Table"}]}`)
	key := rootFieldKeyForTest(cfg, input, 0)

	noopStore := newTestStore()
	noopTarget := parseValue(t, `{}`)
	noop := NewController(noopStore, ModeNoop, nil).BeginRequest(resolve.NewContext(t.Context()))
	noopDecision, noopHandle := noop.PrepareFetch(rootFieldPrepareInput(t, cfg, noopTarget, input, 0))
	assert.Equal(t, resolve.DecisionFetch, noopDecision)
	assert.Equal(t, (*resolve.FetchCacheHandle)(nil), noopHandle)
	assert.Equal(t, []storeOp(nil), noopStore.ops)

	l2Store := newTestStore()
	l2Store.Seed(key, cached, time.Minute)
	l2Target := parseValue(t, `{}`)
	l2 := NewController(l2Store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	l2Decision, l2Handle := l2.PrepareFetch(rootFieldPrepareInput(t, cfg, l2Target, input, 0))
	require.NotNil(t, l2Handle)
	require.NoError(t, l2.OnFetchSkipped(l2Handle, mergeInput(l2Target, nil, false, false)))

	assert.Equal(t, resolve.DecisionSkipFullHit, l2Decision)
	assert.Equal(t, `{"topProducts":[{"upc":"1","name":"Table"}]}`, valueString(l2Target))
	assert.Equal(t, valueString(parseValue(t, string(cached))), valueString(l2Target))
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, l2Store.ops)
}

func TestControllerRootField_KeyDerivesFromCanonicalInputAndHeaderHash(t *testing.T) {
	cfg := rootFieldConfig(time.Minute)
	cfg.CacheName = "root:products"
	cfg.IncludeSubgraphHeaderPrefix = true
	input := []byte(`query TopProducts { topProducts { upc name } }`)
	otherInput := []byte(`query TopProducts{topProducts{upc name}}`)
	headerHash := uint64(0x0102030405060708)
	key := rootFieldKeyForTest(cfg, input, headerHash)
	otherKey := rootFieldKeyForTest(cfg, otherInput, headerHash)
	target := parseValue(t, `{}`)
	store := newTestStore()

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(rootFieldPrepareInput(t, cfg, target, input, headerHash))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Equal(t, []string{key}, handle.Items[0].RenderedKeys)
	assert.Equal(t, "root:products:h0102030405060708:"+hashHex(append([]byte("root:products:h0102030405060708:"), input...)), key)
	assert.Equal(t, false, key == otherKey)
	assert.Equal(t, []storeOp{{Kind: "Get", Key: key}}, store.ops)
}

func rootFieldConfig(ttl time.Duration) *resolve.FetchCacheConfig {
	return &resolve.FetchCacheConfig{
		L2:        true,
		CacheName: "root:products",
		TTL:       ttl,
		KeySpec: resolve.CacheKeySpec{
			Scope:     resolve.CacheScopeRootField,
			TypeName:  "Query",
			FieldName: "topProducts",
		},
		ProvidesData: rootFieldProvides(),
	}
}

func rootFieldProvides() *resolve.Object {
	return &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name: []byte("topProducts"),
				Value: &resolve.Array{
					Item: &resolve.Object{
						Fields: []*resolve.Field{
							{Name: []byte("upc"), Value: &resolve.Scalar{}},
							{Name: []byte("name"), Value: &resolve.Scalar{}},
						},
					},
				},
			},
		},
	}
}

func rootFieldPrepareInput(t *testing.T, cfg *resolve.FetchCacheConfig, item *astjson.Value, input []byte, headerHash uint64) resolve.PrepareFetchInput {
	t.Helper()
	return resolve.PrepareFetchInput{
		Ctx:        resolve.NewContext(t.Context()),
		Items:      []*astjson.Value{item},
		Input:      append([]byte(nil), input...),
		HeaderHash: headerHash,
		Config:     cfg,
		Arena:      newTestMergeArena(),
	}
}

func rootFieldKeyForTest(cfg *resolve.FetchCacheConfig, input []byte, headerHash uint64) string {
	prefix := cacheKeyPrefix(cfg, headerHash)
	preimage := make([]byte, 0, len(prefix)+1+len(input))
	preimage = append(preimage, prefix...)
	preimage = append(preimage, ':')
	preimage = append(preimage, input...)
	return prefix + ":" + hashHex(preimage)
}
