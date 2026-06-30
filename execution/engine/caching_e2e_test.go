package engine_test

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	engine "github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache/cachetesting"
)

func TestCaching_EndToEnd_L2EntityHit(t *testing.T) {
	query := "{ topProducts { upc name reviews { body } } }"
	responses := map[string]string{
		"1":             `{"data":{"topProducts":[{"__typename":"Product","upc":"1","name":"Table"},{"__typename":"Product","upc":"2","name":"Chair"}]}}`,
		"2:topProducts": `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Stable"}]}]}}`,
	}
	wantBody := cachetesting.Compact(t, `{"data":{"topProducts":[{"upc":"1","name":"Table","reviews":[{"body":"Solid"}]},{"upc":"2","name":"Chair","reviews":[{"body":"Stable"}]}]}}`)
	store := cachetesting.NewFakeStore()

	firstPlan := engine.Plan(t, cachetesting.StageL2Entities, query, responses)
	firstRegistry := firstPlan.Fakes
	firstEndCount := &atomic.Int64{}
	firstController := newCountingCacheController(cachetesting.NewRealishCache(t, cachetesting.ModeL2, store, nil), firstEndCount)
	firstBody := resolveGraphQLResponse(t, firstPlan.Response, firstController)
	firstOps := store.Ops()
	require.Equal(t, 4, len(firstOps))
	firstExpectedOps := []cachetesting.StoreOp{
		{Kind: "Get", Key: firstOps[0].Key},
		{Kind: "Get", Key: firstOps[1].Key},
		{Kind: "Set", Key: firstOps[0].Key, Value: `{"__typename":"Product","reviews":[{"body":"Solid"}]}`, TTL: time.Minute},
		{Kind: "Set", Key: firstOps[1].Key, Value: `{"__typename":"Product","reviews":[{"body":"Stable"}]}`, TTL: time.Minute},
	}

	assert.Equal(t, wantBody, firstBody)
	assert.Equal(t, int64(1), firstRegistry.LoadCount("1", ""))
	assert.Equal(t, int64(1), firstRegistry.LoadCount("2", "topProducts"))
	assert.Equal(t, int64(1), firstEndCount.Load())
	assert.Equal(t, firstExpectedOps, firstOps)

	secondPlan := engine.Plan(t, cachetesting.StageL2Entities, query, responses)
	secondRegistry := secondPlan.Fakes
	secondEndCount := &atomic.Int64{}
	secondController := newCountingCacheController(cachetesting.NewRealishCache(t, cachetesting.ModeL2, store, nil), secondEndCount)
	secondBody := resolveGraphQLResponse(t, secondPlan.Response, secondController)
	secondOps := store.Ops()
	secondExpectedOps := append(firstExpectedOps,
		cachetesting.StoreOp{Kind: "Get", Key: firstOps[0].Key},
		cachetesting.StoreOp{Kind: "Get", Key: firstOps[1].Key},
	)

	assert.Equal(t, wantBody, secondBody)
	assert.Equal(t, int64(1), secondRegistry.LoadCount("1", ""))
	assert.Equal(t, int64(0), secondRegistry.LoadCount("2", "topProducts"))
	assert.Equal(t, int64(1), secondEndCount.Load())
	assert.Equal(t, secondExpectedOps, secondOps)
}

type countingCacheController struct {
	inner    resolve.CacheController
	endCount *atomic.Int64
}

func newCountingCacheController(inner resolve.CacheController, endCount *atomic.Int64) *countingCacheController {
	return &countingCacheController{inner: inner, endCount: endCount}
}

func (c *countingCacheController) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	return &countingRequestCache{
		inner:    c.inner.BeginRequest(ctx),
		endCount: c.endCount,
	}
}

type countingRequestCache struct {
	inner    resolve.RequestCache
	endCount *atomic.Int64
}

func (c *countingRequestCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	return c.inner.PrepareFetch(in)
}

func (c *countingRequestCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	return c.inner.OnFetchSkipped(h, in)
}

func (c *countingRequestCache) OnFetchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	return c.inner.OnFetchResult(h, in)
}

func (c *countingRequestCache) EndRequest() {
	c.endCount.Add(1)
	c.inner.EndRequest()
}

func resolveGraphQLResponse(t *testing.T, resp *resolve.GraphQLResponse, controller resolve.CacheController) string {
	t.Helper()

	ctx := resolve.NewContext(t.Context())
	ctx.SetCacheController(controller)
	var buf bytes.Buffer
	r := resolve.New(t.Context(), resolve.ResolverOptions{})
	_, err := r.ResolveGraphQLResponse(ctx, resp, nil, &buf)
	require.NoError(t, err)
	return buf.String()
}
