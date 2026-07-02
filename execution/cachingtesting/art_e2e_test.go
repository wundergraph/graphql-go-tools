package cachingtesting

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// resolveTraced resolves with ART enabled and the production TraceObserver
// composed into the controller, returning the response body.
func resolveTraced(t *testing.T, resp *resolve.GraphQLResponse, store *cachetesting.FakeStore) string {
	t.Helper()
	ctx := resolve.NewContext(t.Context())
	ctx.TracingOptions.Enable = true
	ctx.SetCacheController(cache.NewController(cachetesting.StoreAdapter{Store: store}, cache.NewTraceObserver()))
	return resolveWithContext(t, ctx, resp)
}

// cacheTraces walks the fetch tree and renders every attached CacheTrace as
// canonical JSON (TTL/age nanos normalized to -1 when positive: real-clock
// values; exactness is pinned by the synctest observer unit rows).
func cacheTraces(t *testing.T, node *resolve.FetchTreeNode) []string {
	t.Helper()
	if node == nil {
		return nil
	}
	var out []string
	if node.Item != nil && node.Item.Fetch != nil {
		if trace := loadTraceOf(node.Item.Fetch); trace != nil && trace.CacheTrace != nil {
			normalized := *trace.CacheTrace
			normalized.Items = append([]resolve.CacheItemTrace(nil), trace.CacheTrace.Items...)
			for i := range normalized.Items {
				if normalized.Items[i].RemainingTTLNano > 0 {
					normalized.Items[i].RemainingTTLNano = -1
				}
			}
			normalized.ShadowCompares = append([]resolve.CacheShadowCompareTrace(nil), trace.CacheTrace.ShadowCompares...)
			for i := range normalized.ShadowCompares {
				if normalized.ShadowCompares[i].CacheAgeNano > 0 {
					normalized.ShadowCompares[i].CacheAgeNano = -1
				}
			}
			raw, err := json.Marshal(&normalized)
			require.NoError(t, err)
			out = append(out, fmt.Sprintf("path:%q %s", node.Item.ResponsePath, raw))
		}
	}
	for _, child := range node.ChildNodes {
		out = append(out, cacheTraces(t, child)...)
	}
	return out
}

func loadTraceOf(fetch resolve.Fetch) *resolve.DataSourceLoadTrace {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return f.Trace
	case *resolve.EntityFetch:
		return f.Trace
	case *resolve.BatchEntityFetch:
		return f.Trace
	default:
		return nil
	}
}

// TestARTCacheTraceEndToEnd asserts the COMPLETE cache sections of the ART
// trace for the representative scenarios.
func TestARTCacheTraceEndToEnd(t *testing.T) {
	t.Run("L2 miss then hit", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		query := `{ me { favoriteProduct { upc stock } } }`
		caching := map[string]cacheconfig.CachingConfiguration{
			"inventory": {Entities: []cacheconfig.EntityCachePolicy{{TypeName: "Product", CacheName: "inventory", TTL: time.Minute}}},
		}
		responses := map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		}

		first := Plan(t, query, caching, responses)
		firstBody := resolveTraced(t, first.Response, store)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, firstBody)
		key := store.Ops()[0].Key
		assert.Equal(t, []string{
			`path:"me.favoriteProduct" {"decision":"Fetch","hit":false,"items":[{"keys":["` + key + `"],"hit":false,"write_reason":"refresh"}]}`,
		}, cacheTraces(t, first.Response.Fetches))

		second := Plan(t, query, caching, responses)
		secondBody := resolveTraced(t, second.Response, store)
		assert.Equal(t, firstBody, secondBody)
		assert.Equal(t, []string{
			`path:"me.favoriteProduct" {"decision":"SkipFullHit","hit":true,"items":[{"keys":["` + key + `"],"served_from":"l2","hit":true,"remaining_ttl_nanoseconds":-1}]}`,
		}, cacheTraces(t, second.Response.Fetches))
	})

	t.Run("L1 hit within one request", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		result := Plan(t, l1ChainQuery, productL1Caching(0), l1ChainResponses())
		body := resolveTraced(t, result.Response, store)
		assert.Equal(t, l1ChainExpected, body)
		// The key hashes are deterministic (xxhash64 of the canonical key
		// preimage), so the WHOLE cache sections pin as literals: fetch A is a
		// plain miss+refresh (its sku-derived key; the upc candidate pending),
		// fetch B is the in-request L1 hit under the upc-derived key.
		assert.Equal(t, []string{
			`path:"deal.product" {"decision":"Fetch","hit":false,"items":[{"keys":["entities:f58e5d95ce10e65e"],"hit":false,"write_reason":"refresh","pending_candidates":1}]}`,
			`path:"deal.product.reviews.@.product" {"decision":"SkipFullHit","hit":true,"items":[{"keys":["entities:153cc511c85713fb"],"served_from":"l1","hit":true,"pending_candidates":1}]}`,
		}, cacheTraces(t, result.Response.Fetches))
	})

	t.Run("shadow compare", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		query := `{ me { favoriteProduct { upc stock } } }`
		responses := map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		}
		prime := Plan(t, query, inventoryShadowCaching(), responses)
		resolveTraced(t, prime.Response, store)
		key := store.Ops()[0].Key

		second := Plan(t, query, inventoryShadowCaching(), responses)
		secondBody := resolveTraced(t, second.Response, store)
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, secondBody)
		assert.Equal(t, []string{
			`path:"me.favoriteProduct" {"decision":"FetchShadow","hit":false,"shadow":true,"items":[{"keys":["` + key + `"],"hit":false,"write_reason":"refresh"}],"shadow_compares":[{"key":"` + key + `","is_fresh":true,"cache_age_nanoseconds":-1}]}`,
		}, cacheTraces(t, second.Response.Fetches))
	})

	t.Run("partial batch", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		prime := Plan(t, `{ products(first: 1) { upc reviews { body } } }`, reviewsPartialCaching(), map[string]string{
			"products": `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
			"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"great table"}]}]}}`,
		})
		resolveTraced(t, prime.Response, store)

		second := Plan(t, `{ products(first: 2) { upc reviews { body } } }`, reviewsPartialCaching(), map[string]string{
			"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`,
			"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","body":"sturdy chair"}]}]}}`,
		})
		body := resolveTraced(t, second.Response, store)
		assert.Equal(t,
			`{"data":{"products":[{"upc":"1","reviews":[{"body":"great table"}]},{"upc":"2","reviews":[{"body":"sturdy chair"}]}]}}`,
			body)
		// One partial fetch: bucket 1 (upc "1") served from L2 under the primed
		// key, bucket 2 (upc "2") fetched over the reduced batch and refreshed.
		assert.Equal(t, []string{
			`path:"products" {"decision":"FetchPartial","hit":false,"items":[{"keys":["reviews:aa052522cb64dff0"],"served_from":"l2","hit":true,"remaining_ttl_nanoseconds":-1},{"keys":["reviews:258590a53d3c528e"],"hit":false,"write_reason":"refresh"}]}`,
		}, cacheTraces(t, second.Response.Fetches))
	})

	t.Run("regression: tracing off leaves fetch traces nil", func(t *testing.T) {
		store := cachetesting.NewFakeStore()
		result := Plan(t, `{ me { favoriteProduct { upc stock } } }`, map[string]cacheconfig.CachingConfiguration{
			"inventory": {Entities: []cacheconfig.EntityCachePolicy{{TypeName: "Product", CacheName: "inventory", TTL: time.Minute}}},
		}, map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		})
		body := ResolveResponse(t, result.Response, cachetesting.NewRealishCache(store, nil))
		assert.Equal(t, `{"data":{"me":{"favoriteProduct":{"upc":"1","stock":5}}}}`, body)
		assert.Empty(t, cacheTraces(t, result.Response.Fetches))
	})
}
