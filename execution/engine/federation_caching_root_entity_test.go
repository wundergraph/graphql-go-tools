package engine_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestRootFieldEntityKeyMappingCacheSharing tests that a root field with EntityKeyMappings
// shares cache keys with entity fetches from another subgraph.
//
// Scenario (mirrors failing cosmo router test):
//   - "products" subgraph: root field product(upc: "top-1") → {upc, name, price}
//   - "reviews" subgraph: entity fetch Product._entities(upc: "top-1") → {reviews: [...]}
//   - Root field uses EntityKeyMappings so L2 key = {"__typename":"Product","key":{"upc":"top-1"}}
//   - Second request should hit L2 cache for both fetches (no subgraph calls)
//
// Root cause: EntityKeyMapping.ArgumentPath used the schema argument name ("upc"),
// but after variable extraction the actual variable in ctx.Variables has a normalized
// sequential name ("a"). The planner resolves this mismatch by looking up the actual
// ContextVariable path from the root field's tracked arguments.
func TestRootFieldEntityKeyMappingCacheSharing(t *testing.T) {
	t.Parallel()

	t.Run("root field with EntityKeyMappings L2 hit on second request", func(t *testing.T) {
		t.Parallel()

		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: true,
			}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "product",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{
									EntityTypeName: "Product",
									FieldMappings: []plan.FieldMapping{
										{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
									},
								},
							},
						},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
					},
				},
			}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// Request 1: cache miss → both subgraphs called
		defaultCache.ClearLog()
		tracker.Reset()
		resp, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { product(upc: "top-1") { upc name reviews { body } } }`, nil, t)
		assert.Equal(t, `{"data":{"product":{"upc":"top-1","name":"Trilby","reviews":[{"body":"A highly effective form of birth control."}]}}}`, string(resp))

		assert.Equal(t, 1, tracker.GetCount(productsHost), "first request should call products subgraph once")
		assert.Equal(t, 1, tracker.GetCount(reviewsHost), "first request should call reviews subgraph once")

		// Request 2: should hit cache → neither subgraph called
		defaultCache.ClearLog()
		tracker.Reset()
		resp2, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { product(upc: "top-1") { upc name reviews { body } } }`, nil, t)
		assert.Equal(t, string(resp), string(resp2), "both requests should return identical responses")

		assert.Equal(t, 0, tracker.GetCount(productsHost), "second request should NOT call products subgraph (root field entity cache hit)")
		assert.Equal(t, 0, tracker.GetCount(reviewsHost), "second request should NOT call reviews subgraph (entity cache hit)")
	})
}
