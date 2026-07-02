package cachingtesting

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// TestDeferL1CrossGroupServing (N1 + M3): an entity cached by the INITIAL
// fetch serves a same-entity DEFERRED fetch in a later group — the deferred
// group's subgraph is never hit, both frames are complete, and the per-group
// loaders share ONE L1 through the by-reference Context (one BeginRequest).
func TestDeferL1CrossGroupServing(t *testing.T) {
	query := `{ me { favoriteProduct { upc stock warehouse { id location } } } products(first: 1) { upc ... @defer { stock } } }`
	responses := map[string]string{
		"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
		"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
		"products":                     `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
		"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`,
		// TAMPERED: if the deferred group ever touches the network, the frame
		// shows 999 and the assertion fails loudly.
		"inventory:products": `{"data":{"_entities":[{"__typename":"Product","stock":999}]}}`,
	}
	result := Plan(t, query, inventoryL1Caching(0), responses)

	// The reviewer-guidance plan inspection: the initial tree really carries a
	// configured inventory fetch (the superset provider) and the DEFER GROUP
	// really carries a configured same-entity fetch — L1 kept on BOTH.
	assert.Equal(t, `QueryPlan {
  Sequence {
    Parallel {
      Fetch(service: "3") {
        {
            me {
                __typename
                id
            }
        }
      }
      Fetch(service: "0") {
        {
            products(first: $a){
                upc
                __typename
            }
        }
      }
    }
    Fetch(service: "0") {
      {
        fragment Key on User {
            __typename
            id
        }
      } =>
      {
          _entities(representations: $representations){
              ... on User {
                  __typename
                  favoriteProduct {
                      upc
                      __typename
                  }
              }
          }
      }
    }
    Flatten(path: "me.favoriteProduct") {
      Fetch(service: "1") {
        {
          fragment Key on Product {
              __typename
              upc
          }
        } =>
        Cache: {l1:true l2:false cacheName:inventory ttl:0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
        {
            _entities(representations: $representations){
                ... on Product {
                    __typename
                    stock
                    warehouse {
                        id
                        location
                    }
                }
            }
        }
      }
    }
  }
}
Deferred (id: 1) QueryPlan {
  Fetch(service: "1") {
    {
      fragment Key on Product {
          __typename
          upc
      }
    } =>
    Cache: {l1:true l2:false cacheName:inventory ttl:0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
    {
        _entities(representations: $representations){
            ... on Product {
                __typename
                stock
            }
        }
    }
  }
}
`, PrettyPlan(result))

	store := cachetesting.NewFakeStore()
	controller := &countingController{inner: cachetesting.NewRealishCache(store, nil)}
	frames := ResolveDeferResponse(t, result.DeferResponse, controller)
	assert.Equal(t, []string{
		`{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5,"warehouse":{"id":"w1","location":"Berlin"}}},"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"stock":5},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, frames)
	// The deferred subgraph was NEVER hit; L1-only means zero store traffic.
	assert.Equal(t, int64(0), result.LoadCount("inventory", "products"))
	assert.Empty(t, store.Ops())
	// M3: one BeginRequest — every group's loader worked on the same L1.
	assert.Equal(t, int64(1), controller.begins.Load())
}

// TestDeferL1GroupToLaterGroup (N2): a DEFERRED fetch populates L1 that a
// LATER (nested) group is served from — ordering comes purely from the
// defer-group ancestry (no dependency edge links the two inventory fetches).
func TestDeferL1GroupToLaterGroup(t *testing.T) {
	query := `{ products(first: 1) { upc ... @defer { stock warehouse { id location } ... @defer { reviews { product { stock } } } } } }`
	responses := map[string]string{
		"products":           `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
		"inventory:products": `{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`,
		"reviews":            `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}]}]}}`,
		// TAMPERED: the nested group's inventory fetch must never fire.
		"inventory:products.@.reviews.@.product": `{"data":{"_entities":[{"__typename":"Product","stock":999}]}}`,
	}
	result := Plan(t, query, inventoryL1Caching(0), responses)

	// Plan inspection: the OUTER group's inventory fetch (id 1) is the
	// provider; the NESTED group (id 2, parent = outer) carries the
	// same-entity consumer, kept L1 although NO dependency edge links it to
	// the provider.
	assert.Equal(t, `QueryPlan {
  Fetch(service: "0") {
    {
        products(first: $a){
            upc
            __typename
        }
    }
  }
}
Deferred (id: 1) QueryPlan {
  Fetch(service: "1") {
    {
      fragment Key on Product {
          __typename
          upc
      }
    } =>
    Cache: {l1:true l2:false cacheName:inventory ttl:0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
    {
        _entities(representations: $representations){
            ... on Product {
                __typename
                stock
                warehouse {
                    id
                    location
                }
            }
        }
    }
  }
}
Deferred (id: 2) QueryPlan {
  Sequence {
    Fetch(service: "2") {
      {
        fragment Key on Product {
            __typename
            upc
        }
      } =>
      {
          _entities(representations: $representations){
              ... on Product {
                  __typename
                  reviews {
                      product {
                          __typename
                          upc
                      }
                  }
              }
          }
      }
    }
    Flatten(path: "products.@.reviews.@.product") {
      Fetch(service: "1") {
        {
          fragment Key on Product {
              __typename
              upc
          }
        } =>
        Cache: {l1:true l2:false cacheName:inventory ttl:0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}
        {
            _entities(representations: $representations){
                ... on Product {
                    __typename
                    stock
                }
            }
        }
      }
    }
  }
}
`, PrettyPlan(result))
	groups := DeferGroups(result.DeferResponse)
	require.Len(t, groups, 2)
	assert.Equal(t, 1, result.DeferResponse.DeferDescriptors[groups[1].DeferID].ParentID)

	store := cachetesting.NewFakeStore()
	frames := ResolveDeferResponse(t, result.DeferResponse, cachetesting.NewRealishCache(store, nil))
	assert.Equal(t, []string{
		`{"data":{"products":[{"upc":"1"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"stock":5,"warehouse":{"id":"w1","location":"Berlin"}},"id":"1","subPath":[0]}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["products"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"reviews":[{"product":{"stock":5}}]},"id":"2","subPath":[0]}],"completed":[{"id":"2"}],"hasNext":false}`,
	}, frames)
	assert.Equal(t, int64(0), result.LoadCount("inventory", "products.@.reviews.@.product"))
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
	responses := map[string]string{
		"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
		"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
		"products":                     `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
		"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		"inventory:products":           `{"data":{"_entities":[{"__typename":"Product","warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}]}}`,
	}
	result := Plan(t, query, inventoryL1Caching(time.Minute), responses)
	store := cachetesting.NewFakeStore()
	controller := &endCountingController{inner: cachetesting.NewRealishCache(store, nil)}
	frames := ResolveDeferResponse(t, result.DeferResponse, controller)
	require.Len(t, frames, 2)
	assert.Equal(t, int64(1), controller.ends.Load())

	ops := store.Ops()
	require.Len(t, ops, 4)
	// Both lookups (initial + deferred group) precede BOTH writes: the single
	// request-end flush carries the initial fetch's write and the group's.
	assert.Equal(t, "Get", ops[0].Kind)
	assert.Equal(t, "Get", ops[1].Kind)
	assert.Equal(t, "Set", ops[2].Kind)
	assert.Equal(t, "Set", ops[3].Kind)
	values := []string{ops[2].Value, ops[3].Value}
	assert.Contains(t, values, `{"__typename":"Product","stock":5}`)
	assert.Contains(t, values, `{"__typename":"Product","warehouse":{"__typename":"Warehouse","id":"w1","location":"Berlin"}}`)
}

// TestDeferSkipDoesNotReorderFrames (N4): a SkipFullHit on one sibling group
// flushes its frame while the OTHER sibling is still gated on its subgraph —
// the skip neither waits for nor reorders the fetching sibling.
func TestDeferSkipDoesNotReorderFrames(t *testing.T) {
	query := `{ me { favoriteProduct { upc stock } } products(first: 1) { upc ... @defer { stock } ... @defer { warehouse { id } } } }`
	responses := map[string]string{
		"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
		"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
		"products":                     `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
		"products:products":            `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
		"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		// Only the warehouse group LOADS (the stock group is L1-served and
		// never reaches its gated datasource).
		"inventory:products": `{"data":{"_entities":[{"__typename":"Product","warehouse":{"__typename":"Warehouse","id":"w1"}}]}}`,
	}
	result := Plan(t, query, inventoryL1Caching(0), responses)

	arrived := make(chan string, 4)
	release := make(chan struct{})
	result.Gate("inventory", "products", cachetesting.DataSourceGate{Arrived: arrived, Release: release})

	writer := &deferFrameWriter{Flushed: make(chan struct{}, 8)}
	done := make(chan error, 1)
	ctx := resolve.NewContext(t.Context())
	ctx.SetCacheController(cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil))
	r := resolve.New(t.Context(), resolve.ResolverOptions{MaxConcurrency: 16})
	go func() {
		_, err := r.ResolveGraphQLDeferResponse(ctx, result.DeferResponse, writer)
		done <- err
	}()

	// Pure channel ordering, no latency dependence: every receive carries a
	// failsafe timeout so a resolver regression fails the test instead of
	// hanging it (the timeout can only fire on regression, never orders).
	waitFor := func(what string, ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(30 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
		}
	}
	// The gated group's fetch is IN FLIGHT (blocked on the gate) ...
	select {
	case <-arrived:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the gated inventory fetch to arrive")
	}
	// ... and the initial frame plus the L1-served sibling's frame still flush
	// without waiting for it.
	waitFor("the initial frame flush", writer.Flushed)
	waitFor("the L1-served sibling's frame flush", writer.Flushed)
	framesBeforeRelease := writer.Frames()
	require.Len(t, framesBeforeRelease, 2)
	assert.Contains(t, framesBeforeRelease[1], `"data":{"stock":5}`)

	close(release)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the defer resolve to complete")
	}
	waitFor("the gated sibling's frame flush", writer.Flushed)
	frames := writer.Frames()
	require.Len(t, frames, 3)
	assert.Contains(t, frames[2], `"data":{"warehouse":{"id":"w1"}}`)
	assert.True(t, strings.HasSuffix(frames[2], `"hasNext":false}`))
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
	responses := map[string]string{
		"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
		"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
		"products":                     `{"data":{"products":[{"__typename":"Product","upc":"2"}]}}`,
		"products:products":            `{"data":{"products":[{"__typename":"Product","upc":"2"}]}}`,
		"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		"inventory:products":           `{"data":{"_entities":[{"__typename":"Product","warehouse":{"__typename":"Warehouse","id":"w1"}}]}}`,
	}
	result := Plan(t, query, inventoryL1Caching(time.Minute), responses)
	controller := &erroringResultController{
		inner:  cachetesting.NewRealishCache(cachetesting.NewFakeStore(), nil),
		marker: "warehouse",
	}
	frames := ResolveDeferResponse(t, result.DeferResponse, controller)
	require.Len(t, frames, 3)
	assert.Equal(t,
		`{"data":{"me":{"favoriteProduct":{"upc":"1"}},"products":[{"upc":"2"}]},"pending":[{"id":"1","path":["me","favoriteProduct"]},{"id":"2","path":["products"]}],"hasNext":true}`,
		frames[0])
	// One frame carries the erroring group's hook failure; the OTHER carries
	// the sibling's complete data — unaffected. (Parallel siblings flush in
	// either order; match by content.)
	joined := strings.Join(frames[1:], " ")
	assert.Contains(t, joined, `"errors":[{"message":"cache hook failed"}]`)
	assert.Contains(t, joined, `"data":{"stock":5}`)
	assert.NotContains(t, joined, `"warehouse"`)
	assert.True(t, strings.HasSuffix(frames[2], `"hasNext":false}`))
}
