package engine_test

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	engine "github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache/cachetesting"
)

func TestCaching_EndToEnd_RootReusesEntity(t *testing.T) {
	query := `query ProductByUPC($upc: String!) { product(upc: $upc) { name } }`
	variables := astjson.MustParseBytes([]byte(`{"upc":"1"}`))
	responses := map[string]string{
		"*": `{"data":{"product":{"name":"Table"}}}`,
	}
	wantBody := cachetesting.Compact(t, `{"data":{"product":{"name":"Table"}}}`)
	store := cachetesting.NewFakeStore()

	firstPlan := engine.Plan(t, cachetesting.StageL2RootReusesEntity, query, responses)
	firstRegistry := firstPlan.Fakes
	firstEndCount := &atomic.Int64{}
	firstController := newCountingCacheController(cachetesting.NewRealishCache(t, cachetesting.ModeL2, store, nil), firstEndCount)
	firstBody := resolveGraphQLResponseWithVariables(t, firstPlan.Response, firstController, variables)
	firstOps := store.Ops()
	require.Equal(t, 2, len(firstOps))
	firstExpectedOps := []cachetesting.StoreOp{
		{Kind: "Get", Key: firstOps[0].Key},
		{Kind: "Set", Key: firstOps[0].Key, Value: `{"product":{"name":"Table"}}`, TTL: time.Minute},
	}

	assert.Equal(t, wantBody, firstBody)
	assert.Equal(t, int64(1), firstRegistry.LoadCount("1", ""))
	assert.Equal(t, int64(1), firstEndCount.Load())
	assert.Equal(t, firstExpectedOps, firstOps)

	secondPlan := engine.Plan(t, cachetesting.StageL2RootReusesEntity, query, responses)
	secondRegistry := secondPlan.Fakes
	secondEndCount := &atomic.Int64{}
	secondController := newCountingCacheController(cachetesting.NewRealishCache(t, cachetesting.ModeL2, store, nil), secondEndCount)
	secondBody := resolveGraphQLResponseWithVariables(t, secondPlan.Response, secondController, variables)
	secondOps := store.Ops()
	secondExpectedOps := append(firstExpectedOps, cachetesting.StoreOp{Kind: "Get", Key: firstOps[0].Key})

	assert.Equal(t, wantBody, secondBody)
	assert.Equal(t, int64(0), secondRegistry.LoadCount("1", ""))
	assert.Equal(t, int64(1), secondEndCount.Load())
	assert.Equal(t, secondExpectedOps, secondOps)
}

func resolveGraphQLResponseWithVariables(t *testing.T, resp *resolve.GraphQLResponse, controller resolve.CacheController, variables *astjson.Value) string {
	t.Helper()

	ctx := resolve.NewContext(t.Context())
	ctx.Variables = variables
	ctx.SetCacheController(controller)
	var buf bytes.Buffer
	r := resolve.New(t.Context(), resolve.ResolverOptions{})
	_, err := r.ResolveGraphQLResponse(ctx, resp, nil, &buf)
	require.NoError(t, err)
	return buf.String()
}
