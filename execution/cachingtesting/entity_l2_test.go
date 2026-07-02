package cachingtesting

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// inventoryCaching enables entity caching for the inventory subgraph's Product.
func inventoryCaching() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"inventory": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "entities", TTL: time.Minute},
			},
		},
	}
}

// countingController wraps a controller to count BeginRequest calls (B rows).
type countingController struct {
	inner  resolve.CacheController
	begins atomic.Int64
}

func (c *countingController) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	c.begins.Add(1)
	return c.inner.BeginRequest(ctx)
}

// TestEntityL2EndToEnd is the end-to-end L2 entity hit: request 1 misses and
// writes at request end; request 2 serves from L2 with the subgraph double
// proving ZERO network for the cached fetch; complete responses asserted; the
// lifecycle counts (lazy single BeginRequest, single EndRequest) ride along.
// Runs through the REAL ExecutionEngine; the engine exposes no execution
// option for loader hooks, so the C7 LoaderHooks contract (no hooks for the
// skipped fetch) stays pinned by the resolve-level suites.
func TestEntityL2EndToEnd(t *testing.T) {
	store := cachetesting.NewFakeStore()
	query := `{ me { username favoriteProduct { upc stock } } }`
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
	products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
	inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
	executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})
	expected := `{"data":{"me":{"username":"jens","favoriteProduct":{"upc":"1","stock":5}}}}`

	// Request 1: miss + write-through.
	firstObserver := &cachetesting.RecordingObserver{}
	firstController := &countingController{inner: cachetesting.NewRealishCache(store, firstObserver)}
	firstBody := Execute(t, executionEngine, query, firstController)
	assert.Equal(t, expected, firstBody)
	assert.Equal(t, int64(1), users.Requests())
	assert.Equal(t, int64(1), products.Requests())
	assert.Equal(t, int64(1), inventory.Requests())
	assert.Equal(t, int64(1), firstController.begins.Load())
	firstBegins, firstEnds := firstObserver.Counts()
	assert.Equal(t, 1, firstBegins)
	assert.Equal(t, 1, firstEnds)

	firstOps := store.Ops()
	require.Len(t, firstOps, 2)
	key := firstOps[0].Key
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key},
		{Kind: "Set", Key: key, Value: `{"__typename":"Product","stock":5}`, TTL: time.Minute},
	}, firstOps)

	// Request 2: L2 hit; the inventory subgraph is never hit again. The op log
	// resets so request 2's ops assert in isolation; a fresh controller keeps
	// the BeginRequest count per-request.
	store.ResetOps()
	secondController := &countingController{inner: cachetesting.NewRealishCache(store, nil)}
	secondBody := Execute(t, executionEngine, query, secondController)

	assert.Equal(t, expected, secondBody)
	// The uncached subgraphs fetched again (counts accumulate across the two
	// requests through the one engine); inventory stayed at ONE.
	assert.Equal(t, int64(2), users.Requests())
	assert.Equal(t, int64(2), products.Requests())
	assert.Equal(t, int64(1), inventory.Requests())
	assert.Equal(t, int64(1), secondController.begins.Load())
	// Read key == write key (key fidelity): request 2's ONLY op is a Get
	// under the SAME key request 1 wrote.
	assert.Equal(t, []cachetesting.StoreOp{
		{Kind: "Get", Key: key},
	}, store.Ops())
}

// resolveWithContext drives the public sync entry point with a caller-built
// Context (for hook/controller combinations the plain helper cannot express).
func resolveWithContext(t *testing.T, ctx *resolve.Context, resp *resolve.GraphQLResponse) string {
	t.Helper()
	var buf writerBuffer
	r := resolve.New(t.Context(), resolve.ResolverOptions{MaxConcurrency: 16})
	_, err := r.ResolveGraphQLResponse(ctx, resp, nil, &buf)
	require.NoError(t, err)
	return buf.String()
}

type writerBuffer struct {
	data []byte
}

func (w *writerBuffer) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *writerBuffer) String() string {
	return string(w.data)
}

// TestEntityL2DispatchRows covers the C dispatch/lifecycle rows with the
// recording fake: decisions route to the right merge hook, the handle keeps
// pointer identity from prepare to merge (C8), and hook errors propagate (O).
func TestEntityL2DispatchRows(t *testing.T) {
	t.Run("[C] DecisionFetch dispatches to OnFetchResult with handle identity", func(t *testing.T) {
		handle := &resolve.FetchCacheHandle{Decision: resolve.DecisionFetch}
		controller := cachetesting.NewRecordingController(map[string]cachetesting.ScriptedDecision{
			"me.favoriteProduct": {Decision: resolve.DecisionFetch, Handle: handle},
		})
		result := Plan(t, `{ me { username favoriteProduct { upc stock } } }`, inventoryCaching(), map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		})
		body := ResolveResponse(t, result.Response, controller)
		assert.Equal(t, `{"data":{"me":{"username":"jens","favoriteProduct":{"upc":"1","stock":5}}}}`, body)

		calls := controller.Calls()
		require.Len(t, calls, 3) // Prepare + Result + End
		assert.Equal(t, "Prepare", calls[0].Op)
		assert.Equal(t, "me.favoriteProduct", calls[0].FetchPath)
		assert.Equal(t, "Result", calls[1].Op)
		assert.Equal(t, `{"__typename":"Product","stock":5}`, calls[1].ResponseData)
		assert.Equal(t, "End", calls[2].Op)
		assert.Equal(t, []*resolve.FetchCacheHandle{handle}, controller.ResultHandles())
		assert.Equal(t, int64(1), controller.Begins())
	})

	t.Run("[C3/C6] SkipFullHit skips the network with NO spurious error", func(t *testing.T) {
		query := `{ me { username favoriteProduct { upc stock } } }`
		responses := map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		}
		result := Plan(t, query, inventoryCaching(), responses)
		// A real full hit: seed the store through a first request, then replay.
		store := cachetesting.NewFakeStore()
		warmup := Plan(t, query, inventoryCaching(), responses)
		ResolveResponse(t, warmup.Response, cachetesting.NewRealishCache(store, nil))

		body := ResolveResponse(t, result.Response, cachetesting.NewRealishCache(store, nil))
		assert.Equal(t, `{"data":{"me":{"username":"jens","favoriteProduct":{"upc":"1","stock":5}}}}`, body)
		assert.Equal(t, int64(0), result.LoadCount("inventory", "me.favoriteProduct"))
	})

	t.Run("[O] merge-hook errors propagate to the caller", func(t *testing.T) {
		fake := cachetesting.NewFakeRequestCache(map[string]cachetesting.ScriptedDecision{
			"me.favoriteProduct": {Decision: resolve.DecisionFetch, Handle: &resolve.FetchCacheHandle{Decision: resolve.DecisionFetch}},
		})
		fake.SetError("me.favoriteProduct", "Result", errors.New("cache write exploded"))
		controller := cachetesting.NewFakeCacheController(fake)

		result := Plan(t, `{ me { username favoriteProduct { upc stock } } }`, inventoryCaching(), map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		})
		ctx := resolve.NewContext(t.Context())
		ctx.SetCacheController(controller)
		var buf writerBuffer
		r := resolve.New(t.Context(), resolve.ResolverOptions{MaxConcurrency: 16})
		_, err := r.ResolveGraphQLResponse(ctx, result.Response, nil, &buf)
		require.EqualError(t, err, "cache write exploded")
	})
}
