package cachingtesting

import (
	"context"
	"errors"
	"slices"
	"strings"
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

// inventoryL1Caching enables the inventory Product policy (ttl 0 = L1-only).
func inventoryL1Caching(ttl time.Duration) map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{
		"inventory": {
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "inventory", TTL: ttl},
			},
		},
	}
}

// executeDeferOnFlush is ExecuteDefer with a per-flushed-frame callback (called
// with all frames flushed so far), so tests can close subgraph gates off
// deterministic frame progress instead of latency.
func executeDeferOnFlush(tb testing.TB, executionEngine *engine.ExecutionEngine, query string, controller resolve.CacheController, onFlush func(frames []string)) []string {
	tb.Helper()
	var frames []string
	writer := graphql.NewEngineResultWriter()
	writer.SetFlushCallback(func(data []byte) {
		frames = append(frames, string(data))
		onFlush(frames)
	})
	require.NoError(tb, executionEngine.Execute(context.Background(), &graphql.Request{Query: query}, &writer, engine.WithCacheController(controller)))
	if writer.Len() > 0 {
		frames = append(frames, writer.String())
	}
	return frames
}

// TestDeferL1CrossGroupServing (N1 + M3): an entity cached by the INITIAL
// fetch serves a same-entity DEFERRED fetch in a later group — the deferred
// group's subgraph is never hit, both frames are complete, and the per-group
// loaders share ONE L1 through the by-reference Context (one BeginRequest).
func TestDeferL1CrossGroupServing(t *testing.T) {
	query := `{ me { favoriteProduct { upc stock warehouse { id location } } } products(first: 1) { upc ... @defer { stock } } }`
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
	products := Rules(
		Rule(`_entities`, `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`),
		Rule(`products(first: $a)`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`),
	)
	// TAMPERED fallback: the deferred group's stock-only request misses the
	// warehouse rule, so if it ever touches the network the frame shows 999
	// and the assertion fails loudly.
	tampered := Rule(``, `{"data":{"_entities":[{"__typename":"Product","stock":999}]}}`)
	inventory := Rules(
		Rule(`warehouse`, `{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`),
		tampered,
	)
	executionEngine := NewEngine(t, inventoryL1Caching(0), Subgraphs{"users": users, "products": products, "inventory": inventory})

	store := cachetesting.NewFakeStore()
	controller := &countingController{inner: cachetesting.NewRealishCache(store, nil)}
	frames := ExecuteDefer(t, executionEngine, query, controller)
	assert.Equal(t, []string{
		`{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5,"warehouse":{"id":"w1","location":"Berlin"}}},"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"stock":5},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, frames)
	// The deferred group NEVER hit the network: inventory saw exactly the one
	// initial superset fetch; L1-only means zero store traffic.
	assert.Equal(t, int64(0), tampered.Count.Load())
	assert.Equal(t, int64(1), inventory.Requests())
	assert.Empty(t, store.Ops())
	// M3: one BeginRequest — every group's loader worked on the same L1.
	assert.Equal(t, int64(1), controller.begins.Load())
}

// TestDeferL1GroupToLaterGroup (N2): a DEFERRED fetch populates L1 that a
// LATER (nested) group is served from — ordering comes purely from the
// defer-group ancestry (no dependency edge links the two inventory fetches).
func TestDeferL1GroupToLaterGroup(t *testing.T) {
	query := `{ products(first: 1) { upc ... @defer { stock warehouse { id location } ... @defer { reviews { product { stock } } } } } }`
	products := Respond(`{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`)
	// TAMPERED fallback: the nested group's stock-only inventory fetch must
	// never fire.
	tampered := Rule(``, `{"data":{"_entities":[{"__typename":"Product","stock":999}]}}`)
	inventory := Rules(
		Rule(`warehouse`, `{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`),
		tampered,
	)
	reviews := Respond(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}]}]}}`)
	executionEngine := NewEngine(t, inventoryL1Caching(0), Subgraphs{"products": products, "inventory": inventory, "reviews": reviews})

	store := cachetesting.NewFakeStore()
	frames := ExecuteDefer(t, executionEngine, query, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, []string{
		`{"data":{"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"stock":5,"warehouse":{"id":"w1","location":"Berlin"}},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"reviews":[{"product":{"stock":5}}]},"id":"2","subPath":[0]}],"completed":[{"id":"2"}],"hasNext":false}`,
	}, frames)
	// The nested group NEVER hit the network: inventory saw exactly the OUTER
	// group's superset fetch; L1-only means zero store traffic.
	assert.Equal(t, int64(0), tampered.Count.Load())
	assert.Equal(t, int64(1), inventory.Requests())
	assert.Empty(t, store.Ops())
}

// endCountingController wraps a controller and counts EndRequest calls on the
// request caches it hands out.
type endCountingController struct {
	inner resolve.CacheController
	ends  atomic.Int64
}

func (c *endCountingController) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	return &endCountingCache{inner: c.inner.BeginRequest(ctx), ends: &c.ends}
}

type endCountingCache struct {
	inner resolve.RequestCache
	ends  *atomic.Int64
}

func (c *endCountingCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	return c.inner.PrepareFetch(in)
}

func (c *endCountingCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	return c.inner.OnFetchSkipped(h, in)
}

func (c *endCountingCache) OnFetchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	return c.inner.OnFetchResult(h, in)
}

func (c *endCountingCache) EndRequest() {
	c.ends.Add(1)
	c.inner.EndRequest()
}

// TestDeferSingleFlush (N3): exactly ONE EndRequest after ALL groups, and the
// L2 flush carries the deferred writes from the initial fetch AND the group —
// every Set sits AFTER every Get in the op log.
func TestDeferSingleFlush(t *testing.T) {
	// The deferred selection is NOT covered by the initial fetch, so the group
	// genuinely fetches and owes an L2 write.
	query := `{ me { favoriteProduct { upc stock } } products(first: 1) { upc ... @defer { warehouse { id location } } } }`
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
	products := Rules(
		Rule(`_entities`, `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`),
		Rule(`products(first: $a)`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`),
	)
	inventory := Rules(
		Rule(`stock`, `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`),
		Rule(`warehouse`, `{"data":{"_entities":[{"__typename":"Product","warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`),
	)
	executionEngine := NewEngine(t, inventoryL1Caching(time.Minute), Subgraphs{"users": users, "products": products, "inventory": inventory})

	store := cachetesting.NewFakeStore()
	controller := &endCountingController{inner: cachetesting.NewRealishCache(store, nil)}
	frames := ExecuteDefer(t, executionEngine, query, controller)
	assert.Equal(t, []string{
		`{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}},"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"warehouse":{"id":"w1","location":"Berlin"}},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, frames)
	assert.Equal(t, int64(1), controller.ends.Load())

	ops := store.Ops()
	require.Len(t, ops, 4)
	// Both lookups (initial + deferred group) precede BOTH writes: the single
	// request-end flush carries the initial fetch's write and the group's.
	assert.Equal(t, []string{"Get", "Get", "Set", "Set"}, []string{ops[0].Kind, ops[1].Kind, ops[2].Kind, ops[3].Kind})
	// The two flush writes drain a map, so their order is free — sort, then
	// pin the complete value set.
	values := []string{ops[2].Value, ops[3].Value}
	slices.Sort(values)
	assert.Equal(t, []string{
		`{"__typename":"Product","stock":5}`,
		`{"__typename":"Product","warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}`,
	}, values)
}

// TestDeferSkipDoesNotReorderFrames (N4): a SkipFullHit on one sibling group
// flushes its frame while the OTHER sibling is still gated on its subgraph —
// the skip neither waits for nor reorders the fetching sibling.
func TestDeferSkipDoesNotReorderFrames(t *testing.T) {
	query := `{ me { favoriteProduct { upc stock } } products(first: 1) { upc ... @defer { stock } ... @defer { warehouse { id } } } }`
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
	products := Rules(
		Rule(`_entities`, `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`),
		Rule(`products(first: $a)`, `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`),
	)
	// Only the warehouse group LOADS (the stock group is L1-served); its HTTP
	// response stays gated until `release` closes.
	release := make(chan struct{})
	gated := &SubgraphRule{
		Match:    `warehouse`,
		Response: `{"data":{"_entities":[{"__typename":"Product","warehouse":{"__typename":"Warehouse","id":"w1"}}]}}`,
		Gate:     release,
	}
	stockRule := Rule(`stock`, `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`)
	inventory := Rules(gated, stockRule)
	executionEngine := NewEngine(t, inventoryL1Caching(0), Subgraphs{"users": users, "products": products, "inventory": inventory})

	// Pure gate ordering, no latency dependence: the gated sibling's frame can
	// only flush after `release` closes, and `release` closes only once BOTH
	// earlier frames (initial + the L1-served skip sibling's) have flushed — so
	// the skip demonstrably did not wait for the gated fetch, and the full
	// frame order is deterministic.
	frames := executeDeferOnFlush(t, executionEngine, query, cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil), func(frames []string) {
		if len(frames) == 2 {
			close(release)
		}
	})
	assert.Equal(t, []string{
		`{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}},"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]},{"id":"2","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"stock":5},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":true}`,
		`{"incremental":[{"data":{"warehouse":{"id":"w1"}},"id":"2","subPath":[0]}],"completed":[{"id":"2"}],"hasNext":false}`,
	}, frames)
	// The skip group never touched the network: inventory saw exactly the
	// initial stock fetch and the gated warehouse fetch.
	assert.Equal(t, int64(1), stockRule.Count.Load())
	assert.Equal(t, int64(1), gated.Count.Load())
	assert.Equal(t, int64(2), inventory.Requests())
}

// erroringResultCache fails OnFetchResult when the fresh response contains
// the marker, leaving every other hook untouched.
type erroringResultController struct {
	inner  resolve.CacheController
	marker string
}

func (c *erroringResultController) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	return &erroringResultCache{inner: c.inner.BeginRequest(ctx), marker: c.marker}
}

type erroringResultCache struct {
	inner  resolve.RequestCache
	marker string
}

func (c *erroringResultCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	return c.inner.PrepareFetch(in)
}

func (c *erroringResultCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	return c.inner.OnFetchSkipped(h, in)
}

func (c *erroringResultCache) OnFetchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	if in.ResponseData != nil && strings.Contains(string(in.ResponseData.MarshalTo(nil)), c.marker) {
		return errFromHook
	}
	return c.inner.OnFetchResult(h, in)
}

func (c *erroringResultCache) EndRequest() {
	c.inner.EndRequest()
}

var errFromHook = errors.New("cache hook failed")

// TestDeferHookErrorIsolation (M5): one group's cache-hook error surfaces in
// THAT group's outcome; the sibling group's frame is complete and unaffected.
func TestDeferHookErrorIsolation(t *testing.T) {
	query := `{ me { favoriteProduct { upc ... @defer { stock } } } products(first: 1) { upc ... @defer { warehouse { id } } } }`
	users := Respond(`{"data":{"me":{"__typename":"User","id":"u1"}}}`)
	products := Rules(
		Rule(`_entities`, `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`),
		Rule(`products(first: $a)`, `{"data":{"products":[{"__typename":"Product","upc":"2"}]}}`),
	)
	// Parallel sibling groups flush in either order — gate the ERRORING group's
	// subgraph fetch until the healthy sibling's frame has flushed, so the frame
	// order is gate-deterministic and every frame can be pinned in full.
	release := make(chan struct{})
	gated := &SubgraphRule{
		Match:    `warehouse`,
		Response: `{"data":{"_entities":[{"__typename":"Product","warehouse":{"__typename":"Warehouse","id":"w1"}}]}}`,
		Gate:     release,
	}
	inventory := Rules(
		gated,
		Rule(`stock`, `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`),
	)
	executionEngine := NewEngine(t, inventoryL1Caching(time.Minute), Subgraphs{"users": users, "products": products, "inventory": inventory})

	controller := &erroringResultController{
		inner:  cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil),
		marker: "warehouse",
	}
	frames := executeDeferOnFlush(t, executionEngine, query, controller, func(frames []string) {
		if len(frames) == 2 {
			close(release)
		}
	})
	// The erroring group's frame carries ONLY the hook failure (no warehouse
	// data); the sibling's frame is complete and unaffected.
	assert.Equal(t, []string{
		`{"data":{"me":{"favoriteProduct":{"upc":"1"}},"products":[{"upc":"2"}]},"pending":[{"id":"1","path":["me","favoriteProduct"]},{"id":"2","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"stock":5},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}`,
		`{"completed":[{"id":"2","errors":[{"message":"cache hook failed"}]}],"hasNext":false}`,
	}, frames)
}
