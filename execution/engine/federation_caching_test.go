package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestFederationCaching(t *testing.T) {
	t.Run("two subgraphs - miss then hit", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true})))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking (URL.Host includes host:port)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - should miss cache and then set
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Cache operations: products (get/set), reviews (get/set), accounts User entity (get/set)
		assert.Equal(t, 6, len(logAfterFirst))

		wantLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			// User entity resolution from accounts (author.username requires entity fetch)
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(logAfterFirst))

		// Verify subgraph calls for first query
		// First query should call products (topProducts), reviews (reviews), and accounts (User entity)
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph exactly once")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph exactly once")
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph for User entity resolution")

		// Second query - should hit cache and then set
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// All three entity types should hit L2 cache
		assert.Equal(t, 3, len(logAfterSecond))

		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true}, // Should be a hit now
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{true, true}, // Should be hits now, no misses
			},
			// User entity also hits L2 cache
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{true, true}, // Should be hits now
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond))

		// Verify subgraph calls for second query
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		assert.Equal(t, 0, productsCallsSecond, "Second query should hit cache and not call products subgraph again")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should hit cache and not call reviews subgraph again")
		assert.Equal(t, 0, accountsCallsSecond, "Second query should hit cache and not call accounts subgraph again")
	})

	t.Run("two subgraphs - partial fields then full fields", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient), withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true})))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking (URL.Host includes host:port)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - only ask for name field (products subgraph only)
		defaultCache.ClearLog()
		tracker.Reset()
		firstQuery := `query {
			topProducts {
				name
			}
		}`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, firstQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby"},{"name":"Fedora"}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst))

		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst))

		// Verify first query calls products subgraph only
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query calls products subgraph once")
		assert.Equal(t, 0, reviewsCallsFirst, "First query does not call reviews subgraph")
		assert.Equal(t, 0, accountsCallsFirst, "First query does not call accounts subgraph")

		// Second query - ask for full fields including reviews (products + reviews + accounts)
		defaultCache.ClearLog()
		tracker.Reset()
		secondQuery := `query {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
					}
				}
			}
		}`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, secondQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// Cache operations: products (get/set), reviews (get/set), accounts User entity (get/set)
		assert.Equal(t, 6, len(logAfterSecond))

		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true}, // Should be a hit from first query
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false}, // Miss because second query requests different fields (reviews)
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			// User entity resolution from accounts (author.username requires entity fetch)
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond))

		// Verify second query: products name is cached, but reviews and User entity still need to be fetched
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsSecond, "Second query calls products subgraph once (for reviews data)")
		assert.Equal(t, 1, reviewsCallsSecond, "Second query calls reviews subgraph once (reviews not cached)")
		assert.Equal(t, 1, accountsCallsSecond, "Second query calls accounts subgraph for User entity resolution")

		// Third query - repeat the second query (full fields)
		defaultCache.ClearLog()
		tracker.Reset()
		thirdQuery := `query {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
					}
				}
			}
		}`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, thirdQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterThird := defaultCache.GetLog()
		// All three entity types should hit L2 cache
		assert.Equal(t, 3, len(logAfterThird))

		wantLogThird := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true}, // Should be a hit from second query
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{true, true}, // Should be hits from second query
			},
			// User entity also hits L2 cache
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"User","key":{"id":"1234"}}`,
					`{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{true, true}, // Should be hits from second query
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogThird), sortCacheLogKeys(logAfterThird))

		// Verify third query: all data should be cached, no subgraph calls
		productsCallsThird := tracker.GetCount(productsHost)
		reviewsCallsThird := tracker.GetCount(reviewsHost)
		accountsCallsThird := tracker.GetCount(accountsHost)

		// All cache entries show hits, so no subgraph calls should be made
		assert.Equal(t, 0, productsCallsThird, "Third query does not call products subgraph (all cache hits)")
		assert.Equal(t, 0, reviewsCallsThird, "Third query does not call reviews subgraph (all cache hits)")
		assert.Equal(t, 0, accountsCallsThird, "Third query does not call accounts subgraph (all cache hits)")
	})

	t.Run("two subgraphs - with subgraph header prefix", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Create mock SubgraphHeadersBuilder that returns a fixed hash for each subgraph
		// The composition library generates numeric datasource IDs (0, 1, 2, ...) based on subgraph order:
		// - "0" = accounts
		// - "1" = products (handles topProducts query) -> prefix 11111 for Query cache keys
		// - "2" = reviews (handles Product entity fetch for reviews data) -> prefix 22222 for Product cache keys
		mockHeadersBuilder := &mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{
				"0": 33333, // accounts
				"1": 11111, // products
				"2": 22222, // reviews
			},
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withSubgraphHeadersBuilder(mockHeadersBuilder),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking (URL.Host includes host:port)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - should miss cache and then set with prefixed keys
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Cache operations: products (get/set), reviews (get/set), accounts User entity (get/set)
		assert.Equal(t, 6, len(logAfterFirst))

		wantLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`11111:{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`11111:{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`22222:{"__typename":"Product","key":{"upc":"top-1"}}`,
					`22222:{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`22222:{"__typename":"Product","key":{"upc":"top-1"}}`,
					`22222:{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			// User entity resolution from accounts (author.username requires entity fetch)
			{
				Operation: "get",
				Keys: []string{
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(logAfterFirst))

		// Verify subgraph calls for first query
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph exactly once")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph exactly once")
		// Accounts IS called for User entity resolution (author.username requires entity fetch)
		assert.Equal(t, 1, accountsCallsFirst, "First query should call accounts subgraph for User entity resolution")

		// Second query - should hit cache with prefixed keys
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		// All three entity types should hit L2 cache (products, reviews products, user entities)
		assert.Equal(t, 3, len(logAfterSecond))

		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`11111:{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true}, // Should be a hit now
			},
			{
				Operation: "get",
				Keys: []string{
					`22222:{"__typename":"Product","key":{"upc":"top-1"}}`,
					`22222:{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
				Hits: []bool{true, true}, // Should be hits now
			},
			// User entity also hits L2 cache
			{
				Operation: "get",
				Keys: []string{
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
					`33333:{"__typename":"User","key":{"id":"1234"}}`,
				},
				Hits: []bool{true, true}, // Should be hits now
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond))

		// Verify subgraph calls for second query
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		assert.Equal(t, 0, productsCallsSecond, "Second query should hit cache and not call products subgraph again")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should hit cache and not call reviews subgraph again")
		assert.Equal(t, 0, accountsCallsSecond, "Second query should hit cache and not call accounts subgraph again")
	})
}

// subgraphCallTracker tracks HTTP requests made to subgraph servers
type subgraphCallTracker struct {
	mu       sync.RWMutex
	counts   map[string]int // Maps subgraph URL to call count
	original http.RoundTripper
}

func newSubgraphCallTracker(original http.RoundTripper) *subgraphCallTracker {
	return &subgraphCallTracker{
		counts:   make(map[string]int),
		original: original,
	}
}

func (t *subgraphCallTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	host := req.URL.Host
	t.counts[host]++
	t.mu.Unlock()
	return t.original.RoundTrip(req)
}

func (t *subgraphCallTracker) GetCount(url string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.counts[url]
}

func (t *subgraphCallTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts = make(map[string]int)
}

func (t *subgraphCallTracker) GetCounts() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]int)
	for k, v := range t.counts {
		result[k] = v
	}
	return result
}

func (t *subgraphCallTracker) DebugPrint() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return fmt.Sprintf("%v", t.counts)
}

// Helper functions for gateway setup with HTTP client support
type cachingGatewayOptions struct {
	enableART              bool
	withLoaderCache        map[string]resolve.LoaderCache
	httpClient             *http.Client
	subgraphHeadersBuilder resolve.SubgraphHeadersBuilder
	cachingOptions         resolve.CachingOptions
}

func withCachingEnableART(enableART bool) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.enableART = enableART
	}
}

func withCachingLoaderCache(loaderCache map[string]resolve.LoaderCache) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.withLoaderCache = loaderCache
	}
}

func withHTTPClient(client *http.Client) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.httpClient = client
	}
}

func withSubgraphHeadersBuilder(builder resolve.SubgraphHeadersBuilder) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.subgraphHeadersBuilder = builder
	}
}

func withCachingOptionsFunc(cachingOpts resolve.CachingOptions) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.cachingOptions = cachingOpts
	}
}

type cachingGatewayOptionsToFunc func(opts *cachingGatewayOptions)

func addCachingGateway(options ...cachingGatewayOptionsToFunc) func(setup *federationtesting.FederationSetup) *httptest.Server {
	opts := &cachingGatewayOptions{}
	for _, option := range options {
		option(opts)
	}
	return func(setup *federationtesting.FederationSetup) *httptest.Server {
		httpClient := opts.httpClient
		if httpClient == nil {
			httpClient = http.DefaultClient
		}

		poller := gateway.NewDatasource([]gateway.ServiceConfig{
			{Name: "accounts", URL: setup.AccountsUpstreamServer.URL},
			{Name: "products", URL: setup.ProductsUpstreamServer.URL, WS: strings.ReplaceAll(setup.ProductsUpstreamServer.URL, "http:", "ws:")},
			{Name: "reviews", URL: setup.ReviewsUpstreamServer.URL},
		}, httpClient)

		gtw := gateway.HandlerWithCaching(abstractlogger.NoopLogger, poller, httpClient, opts.enableART, opts.withLoaderCache, opts.subgraphHeadersBuilder, opts.cachingOptions)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		poller.Run(ctx)
		return httptest.NewServer(gtw)
	}
}

// mockSubgraphHeadersBuilder is a mock implementation of SubgraphHeadersBuilder
type mockSubgraphHeadersBuilder struct {
	hashes map[string]uint64
}

func (m *mockSubgraphHeadersBuilder) HeadersForSubgraph(subgraphName string) (http.Header, uint64) {
	hash := m.hashes[subgraphName]
	if hash == 0 {
		// Return default hash if not found
		return nil, 99999
	}
	return nil, hash
}

func (m *mockSubgraphHeadersBuilder) HashAll() uint64 {
	// Return a simple hash of all subgraph hashes combined
	var result uint64
	for _, hash := range m.hashes {
		result ^= hash
	}
	return result
}

func cachingTestQueryPath(name string) string {
	return path.Join("..", "federationtesting", "testdata", name)
}

type CacheLogEntry struct {
	Operation string   // "get", "set", "delete"
	Keys      []string // Keys involved in the operation
	Hits      []bool   // For Get: whether each key was a hit (true) or miss (false)
}

// normalizeCacheLog creates a copy of log entries without timestamps for comparison
func normalizeCacheLog(log []CacheLogEntry) []CacheLogEntry {
	normalized := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		normalized[i] = CacheLogEntry{
			Operation: entry.Operation,
			Keys:      entry.Keys,
			Hits:      entry.Hits,
			// Timestamp is zero value for comparison
		}
	}
	return normalized
}

// sortCacheLogKeys sorts the keys (and corresponding hits) in each cache log entry
// This makes comparisons order-independent when multiple keys are present
func sortCacheLogKeys(log []CacheLogEntry) []CacheLogEntry {
	sorted := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		// Only sort if there are multiple keys
		if len(entry.Keys) <= 1 {
			sorted[i] = entry
			continue
		}

		// Create pairs of (key, hit) to sort together
		pairs := make([]struct {
			key string
			hit bool
		}, len(entry.Keys))
		for j := range entry.Keys {
			pairs[j].key = entry.Keys[j]
			if entry.Hits != nil && j < len(entry.Hits) {
				pairs[j].hit = entry.Hits[j]
			}
		}

		// Sort pairs by key
		sort.Slice(pairs, func(a, b int) bool {
			return pairs[a].key < pairs[b].key
		})

		// Extract sorted keys and hits
		sorted[i] = CacheLogEntry{
			Operation: entry.Operation,
			Keys:      make([]string, len(pairs)),
			Hits:      nil,
		}
		if entry.Hits != nil && len(entry.Hits) > 0 {
			sorted[i].Hits = make([]bool, len(pairs))
		}
		for j := range pairs {
			sorted[i].Keys[j] = pairs[j].key
			if sorted[i].Hits != nil {
				sorted[i].Hits[j] = pairs[j].hit
			}
		}
	}
	return sorted
}

type cacheEntry struct {
	data      []byte
	expiresAt *time.Time
}

type FakeLoaderCache struct {
	mu      sync.RWMutex
	storage map[string]cacheEntry
	log     []CacheLogEntry
}

func NewFakeLoaderCache() *FakeLoaderCache {
	return &FakeLoaderCache{
		storage: make(map[string]cacheEntry),
		log:     make([]CacheLogEntry, 0),
	}
}

func (f *FakeLoaderCache) cleanupExpired() {
	now := time.Now()
	for key, entry := range f.storage {
		if entry.expiresAt != nil && now.After(*entry.expiresAt) {
			delete(f.storage, key)
		}
	}
}

func (f *FakeLoaderCache) Get(ctx context.Context, keys []string) ([]*resolve.CacheEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	hits := make([]bool, len(keys))
	result := make([]*resolve.CacheEntry, len(keys))
	for i, key := range keys {
		if entry, exists := f.storage[key]; exists {
			// Make a copy of the data to prevent external modifications
			dataCopy := make([]byte, len(entry.data))
			copy(dataCopy, entry.data)
			result[i] = &resolve.CacheEntry{
				Key:   key,
				Value: dataCopy,
			}
			hits[i] = true
		} else {
			result[i] = nil
			hits[i] = false
		}
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "get",
		Keys:      keys,
		Hits:      hits,
	})

	return result, nil
}

func (f *FakeLoaderCache) Set(ctx context.Context, entries []*resolve.CacheEntry, ttl time.Duration) error {
	if len(entries) == 0 {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		cacheEntry := cacheEntry{
			// Make a copy of the data to prevent external modifications
			data: make([]byte, len(entry.Value)),
		}
		copy(cacheEntry.data, entry.Value)

		// If ttl is 0, store without expiration
		if ttl > 0 {
			expiresAt := time.Now().Add(ttl)
			cacheEntry.expiresAt = &expiresAt
		}

		f.storage[entry.Key] = cacheEntry
		keys = append(keys, entry.Key)
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "set",
		Keys:      keys,
		Hits:      nil, // Set operations don't have hits/misses
	})

	return nil
}

func (f *FakeLoaderCache) Delete(ctx context.Context, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	for _, key := range keys {
		delete(f.storage, key)
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "delete",
		Keys:      keys,
		Hits:      nil, // Delete operations don't have hits/misses
	})

	return nil
}

// GetLog returns a copy of the cache operation log
func (f *FakeLoaderCache) GetLog() []CacheLogEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	logCopy := make([]CacheLogEntry, len(f.log))
	copy(logCopy, f.log)
	return logCopy
}

// ClearLog clears the cache operation log
func (f *FakeLoaderCache) ClearLog() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = make([]CacheLogEntry, 0)
}

// TestFakeLoaderCache tests the cache implementation itself
func TestFakeLoaderCache(t *testing.T) {
	ctx := context.Background()
	cache := NewFakeLoaderCache()

	t.Run("SetAndGet", func(t *testing.T) {
		// Test basic set and get
		keys := []string{"key1", "key2", "key3"}
		entries := []*resolve.CacheEntry{
			{Key: "key1", Value: []byte("value1")},
			{Key: "key2", Value: []byte("value2")},
			{Key: "key3", Value: []byte("value3")},
		}

		err := cache.Set(ctx, entries, 0) // No TTL
		require.NoError(t, err)

		// Get all keys
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.NotNil(t, result[0])
		assert.Equal(t, "value1", string(result[0].Value))
		assert.NotNil(t, result[1])
		assert.Equal(t, "value2", string(result[1].Value))
		assert.NotNil(t, result[2])
		assert.Equal(t, "value3", string(result[2].Value))

		// Get partial keys
		result, err = cache.Get(ctx, []string{"key2", "key4", "key1"})
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.NotNil(t, result[0])
		assert.Equal(t, "value2", string(result[0].Value))
		assert.Nil(t, result[1]) // key4 doesn't exist
		assert.NotNil(t, result[2])
		assert.Equal(t, "value1", string(result[2].Value))
	})

	t.Run("Delete", func(t *testing.T) {
		// Set some keys
		entries := []*resolve.CacheEntry{
			{Key: "del1", Value: []byte("v1")},
			{Key: "del2", Value: []byte("v2")},
			{Key: "del3", Value: []byte("v3")},
		}
		err := cache.Set(ctx, entries, 0)
		require.NoError(t, err)

		// Delete some keys
		err = cache.Delete(ctx, []string{"del1", "del3"})
		require.NoError(t, err)

		// Check remaining keys
		result, err := cache.Get(ctx, []string{"del1", "del2", "del3"})
		require.NoError(t, err)
		assert.Nil(t, result[0])    // del1 was deleted
		assert.NotNil(t, result[1]) // del2 still exists
		assert.Equal(t, "v2", string(result[1].Value))
		assert.Nil(t, result[2]) // del3 was deleted
	})

	t.Run("TTL", func(t *testing.T) {
		// Set with 50ms TTL
		entries := []*resolve.CacheEntry{
			{Key: "ttl1", Value: []byte("expire1")},
			{Key: "ttl2", Value: []byte("expire2")},
		}
		err := cache.Set(ctx, entries, 50*time.Millisecond)
		require.NoError(t, err)

		// Immediately get - should exist
		result, err := cache.Get(ctx, []string{"ttl1", "ttl2"})
		require.NoError(t, err)
		assert.NotNil(t, result[0])
		assert.Equal(t, "expire1", string(result[0].Value))
		assert.NotNil(t, result[1])
		assert.Equal(t, "expire2", string(result[1].Value))

		// Wait for expiration
		time.Sleep(60 * time.Millisecond)

		// Get again - should be nil
		result, err = cache.Get(ctx, []string{"ttl1", "ttl2"})
		require.NoError(t, err)
		assert.Nil(t, result[0])
		assert.Nil(t, result[1])
	})

	t.Run("MixedTTL", func(t *testing.T) {
		// Set some with TTL, some without
		err := cache.Set(ctx, []*resolve.CacheEntry{{Key: "perm1", Value: []byte("permanent")}}, 0)
		require.NoError(t, err)

		err = cache.Set(ctx, []*resolve.CacheEntry{{Key: "temp1", Value: []byte("temporary")}}, 50*time.Millisecond)
		require.NoError(t, err)

		// Wait for temporary to expire
		time.Sleep(60 * time.Millisecond)

		// Check both
		result, err := cache.Get(ctx, []string{"perm1", "temp1"})
		require.NoError(t, err)
		assert.NotNil(t, result[0])
		assert.Equal(t, "permanent", string(result[0].Value)) // Still exists
		assert.Nil(t, result[1])                              // Expired
	})

	t.Run("ThreadSafety", func(t *testing.T) {
		// Test concurrent access
		done := make(chan bool)

		// Writer goroutine
		go func() {
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("concurrent_%d", i)
				value := fmt.Sprintf("value_%d", i)
				err := cache.Set(ctx, []*resolve.CacheEntry{{Key: key, Value: []byte(value)}}, 0)
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Reader goroutine
		go func() {
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("concurrent_%d", i%50)
				_, err := cache.Get(ctx, []string{key})
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Deleter goroutine
		go func() {
			for i := 0; i < 50; i++ {
				key := fmt.Sprintf("concurrent_%d", i*2)
				err := cache.Delete(ctx, []string{key})
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Wait for all goroutines
		<-done
		<-done
		<-done
	})

	t.Run("ResultLengthMatchesKeysLength", func(t *testing.T) {
		// Test that result length always matches input keys length

		// Set some data
		err := cache.Set(ctx, []*resolve.CacheEntry{
			{Key: "exist1", Value: []byte("data1")},
			{Key: "exist3", Value: []byte("data3")},
		}, 0)
		require.NoError(t, err)

		// Request mix of existing and non-existing keys
		keys := []string{"exist1", "missing1", "exist3", "missing2", "missing3"}
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)

		// Verify length matches exactly
		assert.Len(t, result, len(keys), "Result length must match keys length")
		assert.Len(t, result, 5, "Should return exactly 5 results")

		// Verify correct values
		assert.NotNil(t, result[0])
		assert.Equal(t, "data1", string(result[0].Value)) // exist1
		assert.Nil(t, result[1])                          // missing1
		assert.NotNil(t, result[2])
		assert.Equal(t, "data3", string(result[2].Value)) // exist3
		assert.Nil(t, result[3])                          // missing2
		assert.Nil(t, result[4])                          // missing3

		// Test with all missing keys
		allMissingKeys := []string{"missing4", "missing5", "missing6"}
		result, err = cache.Get(ctx, allMissingKeys)
		require.NoError(t, err)
		assert.Len(t, result, 3, "Should return 3 results for 3 keys")
		assert.Nil(t, result[0])
		assert.Nil(t, result[1])
		assert.Nil(t, result[2])

		// Test with empty keys
		result, err = cache.Get(ctx, []string{})
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should return empty slice for empty keys")
	})
}

// =============================================================================
// L1/L2 CACHE END-TO-END TESTS
// =============================================================================
//
// These tests verify the L1 (per-request in-memory) and L2 (external cross-request)
// caching behavior in a federated GraphQL setup.
//
// L1 Cache: Prevents redundant fetches for the same entity within a single request
// L2 Cache: Shares entity data across requests via external cache (e.g., Redis)
//
// Lookup Order (entity fetches): L1 -> L2 -> Subgraph Fetch
// Lookup Order (root fetches): L2 -> Subgraph Fetch (no L1)

func TestL1CacheReducesHTTPCalls(t *testing.T) {
	// This test demonstrates that L1 cache actually reduces HTTP calls.
	//
	// Query structure traversing through different paths to reach the same User:
	// - me query returns User 1234 (just ID from reviews service)
	// - Gateway fetches User 1234 from accounts for username → populates L1
	// - me.reviews.product.reviews.authorWithoutProvides returns User 1234 again
	// - Gateway needs username for authorWithoutProvides
	//
	// The key insight: authorWithoutProvides returns the same User 1234 that was
	// already fetched for the `me` query. Since this is a different traversal path
	// (not a self-referential field), there's no circular reference in the cached data.
	//
	// With L1 enabled: authorWithoutProvides.username is L1 HIT → 1 accounts call total
	// With L1 disabled: authorWithoutProvides.username needs fetch → 2 accounts calls total

	query := `query {
		me {
			id
			username
			reviews {
				body
				product {
					upc
					reviews {
						authorWithoutProvides {
							id
							username
						}
					}
				}
			}
		}
	}`

	expectedResponse := `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me"}}]}}]}}}`

	t.Run("L1 enabled - reduces accounts calls via cache hit", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Verify L1 hits occurred (authorWithoutProvides entities are batched together, 2 fields hit = id + username)
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		assert.Equal(t, int64(2), l1HitsInt, "Should have 2 L1 hits (id + username for authorWithoutProvides batch)")

		// KEY ASSERTION: With L1 enabled, only 1 accounts call!
		// The authorWithoutProvides.username is served from L1 cache (User 1234 already fetched for me.username).
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, accountsCalls,
			"With L1 enabled, should make only 1 accounts call (authorWithoutProvides is L1 hit)")
	})

	t.Run("L1 disabled - more accounts calls without cache", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		tracker.Reset()
		out, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))

		// Verify NO L1 activity
		l1Hits := headers.Get("X-Cache-L1-Hits")
		l1Misses := headers.Get("X-Cache-L1-Misses")
		l1HitsInt, _ := strconv.ParseInt(l1Hits, 10, 64)
		l1MissesInt, _ := strconv.ParseInt(l1Misses, 10, 64)
		assert.Equal(t, int64(0), l1HitsInt, "L1 hits should be 0 when disabled")
		assert.Equal(t, int64(0), l1MissesInt, "L1 misses should be 0 when disabled")

		// KEY ASSERTION: With L1 disabled, 2 accounts calls!
		// The authorWithoutProvides.username requires another fetch since L1 is disabled.
		accountsCalls := tracker.GetCount(accountsHost)
		assert.Equal(t, 2, accountsCalls,
			"With L1 disabled, should make 2 accounts calls (no cache reuse)")
	})
}

func TestL1CacheSelfReferentialEntity(t *testing.T) {
	// This test verifies that self-referential entities don't cause
	// stack overflow when L1 cache is enabled.
	//
	// Background: When an entity type has a field that returns the same type
	// (e.g., User.sameUserReviewers returning [User]), and L1 cache stores
	// a pointer to the entity, both key.Item and key.FromCache can point to
	// the same memory location. Without a fix, calling MergeValues(ptr, ptr)
	// causes infinite recursion and stack overflow.
	//
	// The sameUserReviewers field has @requires(fields: "username") which forces
	// sequential execution: the User entity is first fetched from accounts
	// (populating L1), then sameUserReviewers is resolved, returning the same
	// User entity that's already in L1 cache.

	query := `query {
		topProducts {
			reviews {
				authorWithoutProvides {
					id
					username
					sameUserReviewers {
						id
						username
					}
				}
			}
		}
	}`

	// This response shows User 1234 appearing both at authorWithoutProvides level
	// and inside sameUserReviewers (which returns the same user for testing)
	expectedResponse := `{"data":{"topProducts":[{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]},{"reviews":[{"authorWithoutProvides":{"id":"1234","username":"Me","sameUserReviewers":[{"id":"1234","username":"Me"}]}}]}]}}`

	t.Run("self-referential entity should not cause stack overflow", func(t *testing.T) {
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// This should complete without stack overflow
		// Before the fix, this would crash with "fatal error: stack overflow"
		out, _ := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL, query, nil, t)

		assert.Equal(t, expectedResponse, string(out))
	})
}

func TestL2CacheOnly(t *testing.T) {
	t.Run("L2 enabled - miss then hit across requests", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable L2 cache only
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: true,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - should miss cache
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		// Cache operations: get/set for Query.topProducts, Product entities, User entities = 6 operations
		assert.Equal(t, 6, len(logAfterFirst), "Should have exactly 6 cache operations (get/set for Query, Products, Users)")

		// Verify subgraph calls for first query
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph exactly once")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph exactly once")

		// Second query - should hit cache
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify L2 cache hits
		logAfterSecond := defaultCache.GetLog()
		hasHit := false
		for _, entry := range logAfterSecond {
			if entry.Operation == "get" {
				for _, hit := range entry.Hits {
					if hit {
						hasHit = true
						break
					}
				}
			}
		}
		assert.True(t, hasHit, "Second query should have at least one cache hit")

		// Verify no subgraph calls for second query (all cached)
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		assert.Equal(t, 0, productsCallsSecond, "Second query should not call products subgraph (cache hit)")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should not call reviews subgraph (cache hit)")
	})

	t.Run("L2 disabled - no external cache operations", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Disable L2 cache
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: false,
			EnableL2Cache: false,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// First query
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify no cache operations
		log := defaultCache.GetLog()
		assert.Empty(t, log, "No L2 cache operations should occur when L2 is disabled")
	})
}

func TestL1L2CacheCombined(t *testing.T) {
	t.Run("L1+L2 enabled - L1 within request, L2 across requests", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable both L1 and L2 cache
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - L1 helps within request, L2 populates for later
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify subgraph calls for first query
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph")

		// Second query - new request means fresh L1, but L2 should hit
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()

		// Verify L2 cache hits on second request
		hasHit := false
		for _, entry := range logAfterSecond {
			if entry.Operation == "get" {
				for _, hit := range entry.Hits {
					if hit {
						hasHit = true
						break
					}
				}
			}
		}
		assert.True(t, hasHit, "Second query should have L2 cache hits")

		// Verify no subgraph calls for second query (L2 cache hits)
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		assert.Equal(t, 0, productsCallsSecond, "Second query should not call products subgraph (L2 hit)")
		assert.Equal(t, 0, reviewsCallsSecond, "Second query should not call reviews subgraph (L2 hit)")
	})

	t.Run("L1+L2 - cross-request isolation: L1 per-request, L2 shared", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		// Enable both L1 and L2
		cachingOpts := resolve.CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(cachingOpts),
		))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// First request - populates L2 cache
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify L2 has set operations
		logAfterFirst := defaultCache.GetLog()
		hasSet := false
		for _, entry := range logAfterFirst {
			if entry.Operation == "set" {
				hasSet = true
				break
			}
		}
		assert.True(t, hasSet, "First request should populate L2 cache")

		// Second request - L1 is fresh (new request), but L2 should provide data
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream_without_provides.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`, string(resp))

		// Verify L2 has get operations with hits
		logAfterSecond := defaultCache.GetLog()
		getCount := 0
		hitCount := 0
		for _, entry := range logAfterSecond {
			if entry.Operation == "get" {
				getCount++
				for _, hit := range entry.Hits {
					if hit {
						hitCount++
					}
				}
			}
		}
		assert.Greater(t, getCount, 0, "Second request should have L2 get operations")
		assert.Greater(t, hitCount, 0, "Second request should have L2 cache hits")
	})
}
