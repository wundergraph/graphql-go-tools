package cachingtesting

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
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

// TestEntityL2DispatchRows covers the C dispatch/lifecycle rows through the
// REAL ExecutionEngine: decisions route to the right merge hook, the handle
// keeps pointer identity from prepare to merge (C8), and hook errors
// propagate (O).
func TestEntityL2DispatchRows(t *testing.T) {
	t.Run("[C] DecisionFetch dispatches to OnFetchResult with handle identity", func(t *testing.T) {
		handle := &resolve.FetchCacheHandle{Decision: resolve.DecisionFetch}
		controller := cachetesting.NewRecordingController(map[string]cachetesting.ScriptedDecision{
			"me.favoriteProduct": {Decision: resolve.DecisionFetch, Handle: handle},
		})
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		subgraphs := Subgraphs{"users": users, "products": products, "inventory": inventory}
		executionEngine := NewEngine(t, inventoryCaching(), subgraphs)

		body := Execute(t, executionEngine, `{ me { username favoriteProduct { upc stock } } }`, controller)
		assert.Equal(t, `{"data":{"me":{"username":"jens","favoriteProduct":{"upc":"1","stock":5}}}}`, body)

		// Only the cached inventory fetch hits the hooks: Prepare + Result + End.
		// InputBytes carries the double's ephemeral URL; normalize it so the pin
		// stays literal. EmptyEntity is the loader's RAW signal (entity fetch
		// whose data._entities is a non-null array); the controller only treats
		// it as a negative result when ResponseData is also null.
		calls := controller.Calls()
		for i := range calls {
			calls[i].InputBytes = subgraphs.NormalizeURLs(calls[i].InputBytes)
		}
		assert.Equal(t, []cachetesting.Call{
			{
				Op:         "Prepare",
				FetchPath:  "me.favoriteProduct",
				Items:      1,
				InputBytes: `{"method":"POST","url":"http://inventory.service","header":{},"body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"}]}}}`,
				Decision:   resolve.DecisionFetch,
			},
			{
				Op:           "Result",
				FetchPath:    "me.favoriteProduct",
				Items:        1,
				ResponseData: `{"__typename":"Product","stock":5}`,
				EmptyEntity:  true,
				StatusCode:   200,
			},
			{Op: "End"},
		}, calls)
		assert.Equal(t, []*resolve.FetchCacheHandle{handle}, controller.ResultHandles())
		assert.Equal(t, int64(1), controller.Begins())
	})

	t.Run("[C3/C6] SkipFullHit skips the network with NO spurious error", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		controller := cachetesting.NewRealishCache(store, nil)
		query := `{ me { username favoriteProduct { upc stock } } }`
		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})
		expected := `{"data":{"me":{"username":"jens","favoriteProduct":{"upc":"1","stock":5}}}}`

		// A real full hit: seed the store through a first request, then replay.
		warmupBody := Execute(t, executionEngine, query, controller)
		assert.Equal(t, expected, warmupBody)
		warmupOps := store.Ops()
		require.Len(t, warmupOps, 2)
		key := warmupOps[0].Key
		assert.Equal(t, []cachetesting.StoreOp{
			{Kind: "Get", Key: key},
			{Kind: "Set", Key: key, Value: `{"__typename":"Product","stock":5}`, TTL: time.Minute},
		}, warmupOps)

		// The replay resolves cleanly (Execute fails the test on any resolver
		// error) with ZERO further network to inventory: a single Get, no writes.
		store.ResetOps()
		body := Execute(t, executionEngine, query, controller)
		assert.Equal(t, expected, body)
		assert.Equal(t, int64(1), inventory.Requests())
		assert.Equal(t, []cachetesting.StoreOp{
			{Kind: "Get", Key: key},
		}, store.Ops())
	})

	t.Run("[O] merge-hook errors propagate to the caller", func(t *testing.T) {
		fake := cachetesting.NewFakeRequestCache(map[string]cachetesting.ScriptedDecision{
			"me.favoriteProduct": {Decision: resolve.DecisionFetch, Handle: &resolve.FetchCacheHandle{Decision: resolve.DecisionFetch}},
		})
		fake.SetError("me.favoriteProduct", "Result", errors.New("cache write exploded"))
		controller := cachetesting.NewFakeCacheController(fake)

		users := Respond(`{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`)
		products := Respond(`{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`)
		inventory := Respond(`{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
		executionEngine := NewEngine(t, inventoryCaching(), Subgraphs{"users": users, "products": products, "inventory": inventory})

		// Execute requires success, so drive the engine directly for the error.
		writer := graphql.NewEngineResultWriter()
		err := executionEngine.Execute(context.Background(), &graphql.Request{Query: `{ me { username favoriteProduct { upc stock } } }`}, &writer, engine.WithCacheController(controller))
		require.EqualError(t, err, "cache write exploded")
	})
}
