package engine_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// subgraphRequestTracker tracks requests to subgraphs and captures their bodies
type subgraphRequestTracker struct {
	mu       sync.RWMutex
	requests map[string][]string // host -> list of request bodies
	original http.RoundTripper
}

func newSubgraphRequestTracker(original http.RoundTripper) *subgraphRequestTracker {
	return &subgraphRequestTracker{
		requests: make(map[string][]string),
		original: original,
	}
}

func (t *subgraphRequestTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	// Capture request body
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	t.mu.Lock()
	host := req.URL.Host
	t.requests[host] = append(t.requests[host], string(bodyBytes))
	t.mu.Unlock()

	return t.original.RoundTrip(req)
}

func (t *subgraphRequestTracker) GetRequests(host string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]string, len(t.requests[host]))
	copy(result, t.requests[host])
	return result
}

func (t *subgraphRequestTracker) GetRequestCount(host string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.requests[host])
}

func (t *subgraphRequestTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests = make(map[string][]string)
}

// TestPartialCacheLoading tests the EnablePartialCacheLoad feature for entity caching.
// When enabled, only cache-missed entities are fetched from subgraphs.
// When disabled (default), all entities are fetched if any are missing.
// TestFederationCaching_PartialLoading verifies partial cache loading end-to-end: when some
// entities in a batch are cached, only the uncached ones are fetched from the subgraph.
func TestFederationCaching_PartialLoading(t *testing.T) {
	t.Parallel()
	t.Run("L2 partial cache loading enabled - only missing entities fetched", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with request body tracking
		tracker := newSubgraphRequestTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		setup := federationtesting.NewFederationSetup(func(setup *federationtesting.FederationSetup) *httptest.Server {
			poller := gateway.NewDatasource([]gateway.ServiceConfig{
				{Name: "accounts", URL: setup.AccountsUpstreamServer.URL},
				{Name: "products", URL: setup.ProductsUpstreamServer.URL, WS: strings.ReplaceAll(setup.ProductsUpstreamServer.URL, "http:", "ws:")},
				{Name: "reviews", URL: setup.ReviewsUpstreamServer.URL},
			}, trackingClient)
			gtw := gateway.HandlerWithCaching(abstractlogger.NoopLogger, poller, trackingClient, false, caches, nil, resolve.CachingOptions{EnableL2Cache: true}, engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
					},
				},
				{
					SubgraphName: "accounts",
					EntityCaching: plan.EntityCacheConfigurations{
						// KEY: EnablePartialCacheLoad is TRUE
						{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false, EnablePartialCacheLoad: true},
					},
				},
			}, false)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			poller.Run(ctx)
			return httptest.NewServer(gtw)
		})
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Pre-populate cache with User entity for id "1234"
		// The query will need this user (same user for both reviews via authorWithoutProvides)
		userData := `{"__typename":"User","id":"1234","username":"Me"}`
		err := defaultCache.Set(context.Background(), []*resolve.CacheEntry{
			{Key: `{"__typename":"User","key":{"id":"1234"}}`, Value: []byte(userData), TTL: 30 * time.Second},
		})
		require.NoError(t, err)
		seedLog := defaultCache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}, seedLog)

		// First query - User is already cached, so accounts subgraph should NOT be called
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query MultipleServersWithoutProvides {
    topProducts {
        name
        reviews {
            body
            authorWithoutProvides {
                username
            }
        }
    }
}`, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify accounts subgraph was NOT called (all Users were cached)
		accountsRequests := tracker.GetRequests(accountsHost)
		assert.Equal(t, 0, len(accountsRequests), "accounts subgraph should not be called when all User entities are cached")
	})

	t.Run("L2 partial cache loading enabled - partial cache hit fetches only missing", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with request body tracking
		tracker := newSubgraphRequestTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		setup := federationtesting.NewFederationSetup(func(setup *federationtesting.FederationSetup) *httptest.Server {
			poller := gateway.NewDatasource([]gateway.ServiceConfig{
				{Name: "accounts", URL: setup.AccountsUpstreamServer.URL},
				{Name: "products", URL: setup.ProductsUpstreamServer.URL, WS: strings.ReplaceAll(setup.ProductsUpstreamServer.URL, "http:", "ws:")},
				{Name: "reviews", URL: setup.ReviewsUpstreamServer.URL},
			}, trackingClient)
			gtw := gateway.HandlerWithCaching(abstractlogger.NoopLogger, poller, trackingClient, false, caches, nil, resolve.CachingOptions{EnableL2Cache: true}, engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						// KEY: EnablePartialCacheLoad is TRUE
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false, EnablePartialCacheLoad: true},
					},
				},
				{
					SubgraphName: "accounts",
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
					},
				},
			}, false)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			poller.Run(ctx)
			return httptest.NewServer(gtw)
		})
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		reviewsHost := reviewsURLParsed.Host

		// Pre-populate cache with ONLY ONE of the two Product entities (top-1)
		// top-2 is NOT cached
		// IMPORTANT: Must use 'authorWithoutProvides' as that's what the query fetches (not 'author' which has @provides)
		product1Data := `{"__typename":"Product","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`
		err := defaultCache.Set(context.Background(), []*resolve.CacheEntry{
			{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Value: []byte(product1Data), TTL: 30 * time.Second},
		})
		require.NoError(t, err)
		seedLog := defaultCache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second}}},
		}, seedLog)

		// Query - should only fetch top-2 from reviews subgraph (top-1 is cached)
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query MultipleServersWithoutProvides {
    topProducts {
        name
        reviews {
            body
            authorWithoutProvides {
                username
            }
        }
    }
}`, nil, t)

		// Response should still be complete
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify reviews subgraph was called with ONLY the missing entity (top-2)
		reviewsRequests := tracker.GetRequests(reviewsHost)
		require.Equal(t, 1, len(reviewsRequests), "reviews subgraph should be called exactly once")

		// The request should only contain top-2, NOT top-1 (partial cache load = only fetch missing)
		// Using exact assertion to verify the request body structure
		assert.Equal(t, `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename reviews {body authorWithoutProvides {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"top-2"}]}}`, reviewsRequests[0], "reviews request should fetch ONLY top-2 (top-1 is cached)")
	})

	t.Run("L2 partial cache loading disabled - all entities fetched even with partial cache hit", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with request body tracking
		tracker := newSubgraphRequestTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		setup := federationtesting.NewFederationSetup(func(setup *federationtesting.FederationSetup) *httptest.Server {
			poller := gateway.NewDatasource([]gateway.ServiceConfig{
				{Name: "accounts", URL: setup.AccountsUpstreamServer.URL},
				{Name: "products", URL: setup.ProductsUpstreamServer.URL, WS: strings.ReplaceAll(setup.ProductsUpstreamServer.URL, "http:", "ws:")},
				{Name: "reviews", URL: setup.ReviewsUpstreamServer.URL},
			}, trackingClient)
			gtw := gateway.HandlerWithCaching(abstractlogger.NoopLogger, poller, trackingClient, false, caches, nil, resolve.CachingOptions{EnableL2Cache: true}, engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						// KEY: EnablePartialCacheLoad is FALSE (default)
						{TypeName: "Product", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false, EnablePartialCacheLoad: false},
					},
				},
				{
					SubgraphName: "accounts",
					EntityCaching: plan.EntityCacheConfigurations{
						{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
					},
				},
			}, false)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			poller.Run(ctx)
			return httptest.NewServer(gtw)
		})
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		reviewsHost := reviewsURLParsed.Host

		// Pre-populate cache with ONLY ONE of the two Product entities (top-1)
		// top-2 is NOT cached
		// IMPORTANT: Must use 'authorWithoutProvides' as that's what the query fetches (not 'author' which has @provides)
		product1Data := `{"__typename":"Product","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`
		err := defaultCache.Set(context.Background(), []*resolve.CacheEntry{
			{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, Value: []byte(product1Data), TTL: 30 * time.Second},
		})
		require.NoError(t, err)
		seedLog := defaultCache.GetLog()
		assert.Equal(t, []CacheLogEntry{
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"Product","key":{"upc":"top-1"}}`, TTL: 30 * time.Second}}},
		}, seedLog)

		// Query - with partial loading DISABLED, should fetch ALL entities (top-1 AND top-2)
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, `query MultipleServersWithoutProvides {
    topProducts {
        name
        reviews {
            body
            authorWithoutProvides {
                username
            }
        }
    }
}`, nil, t)

		// Response should still be complete
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify reviews subgraph was called with BOTH entities (all-or-nothing behavior)
		reviewsRequests := tracker.GetRequests(reviewsHost)
		require.Equal(t, 1, len(reviewsRequests), "reviews subgraph should be called exactly once")

		// The request should contain BOTH top-1 AND top-2 (all-or-nothing mode, partial cache disabled)
		// Using exact assertion to verify the request body structure
		assert.Equal(t, `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename reviews {body authorWithoutProvides {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"top-1"},{"__typename":"Product","upc":"top-2"}]}}`, reviewsRequests[0], "reviews request should fetch BOTH entities (partial cache disabled)")
	})
}
