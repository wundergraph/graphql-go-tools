package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestControllerRootReuse_OnFetchResultBackfillsMappedEntityKeys(t *testing.T) {
	cfg := rootReuseConfig(45*time.Second, rootReuseProvides("name", "sku"), []resolve.EntityKeyMapping{
		rootReuseMapping("Product", "upc"),
		rootReuseMapping("Product", "sku"),
	})
	target := parseValue(t, `{}`)
	upcKey := rootReuseEntityKey(cfg, `{"__typename":"Product","key":{"upc":"1"}}`)
	skuKey := rootReuseEntityKey(cfg, `{"__typename":"Product","key":{"sku":"sku-1"}}`)
	store := newTestStore()

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(rootReusePrepareInput(t, cfg, target, parseValue(t, `{"upc":"1"}`)))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionFetch, decision)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionFetch,
		Items: []resolve.ItemCacheState{
			{
				Item:              target,
				RenderedKeys:      []string{upcKey},
				PendingCandidates: []resolve.CacheKeyCandidate{rootReuseCandidate("Product", "sku")},
			},
		},
	}, handle)

	fresh := parseValue(t, `{"name":"Table","sku":"sku-1"}`)
	require.NoError(t, rc.OnFetchResult(handle, mergeInput(target, fresh, false, false)))
	rc.EndRequest()

	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: upcKey},
		{Kind: "Set", Key: upcKey, Value: `{"name":"Table","sku":"sku-1"}`, TTL: 45 * time.Second, Reason: resolve.CacheWriteReasonRefresh},
		{Kind: "Set", Key: skuKey, Value: `{"name":"Table","sku":"sku-1"}`, TTL: 45 * time.Second, Reason: resolve.CacheWriteReasonBackfill},
	}, store.ops)
}

func TestControllerRootReuse_OnFetchSkippedBackfillsMappedEntityKeys(t *testing.T) {
	cfg := rootReuseConfig(30*time.Second, rootReuseProvides("name", "sku"), []resolve.EntityKeyMapping{
		rootReuseMapping("Product", "upc"),
		rootReuseMapping("Product", "sku"),
	})
	target := parseValue(t, `{}`)
	upcKey := rootReuseEntityKey(cfg, `{"__typename":"Product","key":{"upc":"1"}}`)
	skuKey := rootReuseEntityKey(cfg, `{"__typename":"Product","key":{"sku":"sku-1"}}`)
	store := newTestStore()
	store.Seed(upcKey, []byte(`{"name":"Table","sku":"sku-1"}`), time.Minute)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(rootReusePrepareInput(t, cfg, target, parseValue(t, `{"upc":"1"}`)))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	assert.Equal(t, true, handle.MustWriteBack)

	require.NoError(t, rc.OnFetchSkipped(handle, mergeInput(target, nil, false, false)))
	rc.EndRequest()

	assert.Equal(t, []storeOp{
		{Kind: "Get", Key: upcKey},
		{Kind: "Set", Key: skuKey, Value: `{"name":"Table","sku":"sku-1"}`, TTL: 30 * time.Second, Reason: resolve.CacheWriteReasonBackfill},
	}, store.ops)
}

func TestControllerRootReuse_HitUsesEntityKeySpace(t *testing.T) {
	cfg := rootReuseConfig(30*time.Second, rootReuseProvides("name"), []resolve.EntityKeyMapping{
		rootReuseMapping("Product", "upc"),
	})
	target := parseValue(t, `{}`)
	upcKey := rootReuseEntityKey(cfg, `{"__typename":"Product","key":{"upc":"1"}}`)
	store := newTestStore()
	store.Seed(upcKey, []byte(`{"name":"Table"}`), time.Minute)

	rc := NewController(store, ModeL2, nil).BeginRequest(resolve.NewContext(t.Context()))
	decision, handle := rc.PrepareFetch(rootReusePrepareInput(t, cfg, target, parseValue(t, `{"upc":"1"}`)))
	require.NotNil(t, handle)

	assert.Equal(t, resolve.DecisionSkipFullHit, decision)
	assert.Equal(t, &resolve.FetchCacheHandle{
		Decision: resolve.DecisionSkipFullHit,
		WasHit:   true,
		Items: []resolve.ItemCacheState{
			{
				Item:      target,
				FromCache: parseValue(t, `{"name":"Table"}`),
				RenderedKeys: []string{
					upcKey,
				},
				FromCacheCandidates: []resolve.CacheCandidate{
					{Value: []byte(`{"name":"Table"}`), RemainingTTL: handle.Items[0].SelectedRemainingTTL},
				},
				SelectedRemainingTTL: handle.Items[0].SelectedRemainingTTL,
			},
		},
	}, handle)

	require.NoError(t, rc.OnFetchSkipped(handle, mergeInput(target, nil, false, false)))
	rc.EndRequest()

	assert.Equal(t, parseValue(t, `{"name":"Table"}`), target)
	assert.Equal(t, []storeOp{{Kind: "Get", Key: upcKey}}, store.ops)
}

func rootReuseConfig(ttl time.Duration, provides *resolve.Object, mappings []resolve.EntityKeyMapping) *resolve.FetchCacheConfig {
	return &resolve.FetchCacheConfig{
		L2:        true,
		CacheName: "entity:products",
		TTL:       ttl,
		KeySpec: resolve.CacheKeySpec{
			Scope:             resolve.CacheScopeRootField,
			TypeName:          "Query",
			FieldName:         "product",
			EntityKeyMappings: mappings,
		},
		ProvidesData: provides,
	}
}

func rootReusePrepareInput(t *testing.T, cfg *resolve.FetchCacheConfig, item, variables *astjson.Value) resolve.PrepareFetchInput {
	t.Helper()
	ctx := resolve.NewContext(t.Context())
	ctx.Variables = variables
	return resolve.PrepareFetchInput{
		Ctx:    ctx,
		Items:  []*astjson.Value{item},
		Input:  []byte(`query Product($upc:String!){product(upc:$upc){name sku}}`),
		Config: cfg,
		Arena:  newTestMergeArena(),
	}
}

func rootReuseMapping(typeName, field string) resolve.EntityKeyMapping {
	return resolve.EntityKeyMapping{
		EntityTypeName: typeName,
		FieldMappings: []resolve.EntityFieldMapping{
			{
				EntityKeyField:      field,
				ArgumentPath:        []string{field},
				ArgumentIsEntityKey: true,
			},
		},
	}
}

func rootReuseCandidate(typeName, field string) resolve.CacheKeyCandidate {
	return resolve.CacheKeyCandidate{Representation: &resolve.Object{
		TypeName: typeName,
		Fields: []*resolve.Field{
			{Name: []byte("__typename"), Value: &resolve.Scalar{}},
			{Name: []byte(field), Value: &resolve.Scalar{}},
		},
	}}
}

func rootReuseProvides(names ...string) *resolve.Object {
	fields := make([]*resolve.Field, 0, len(names))
	for _, name := range names {
		fields = append(fields, &resolve.Field{Name: []byte(name), Value: &resolve.Scalar{}})
	}
	return &resolve.Object{Fields: fields}
}

func rootReuseEntityKey(cfg *resolve.FetchCacheConfig, payload string) string {
	return renderCacheKey(cacheKeyPrefix(cfg, 0), []byte(payload))
}
