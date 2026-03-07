package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// queryWithRawVariables sends a GraphQL query with raw JSON variables (no key reordering by json.Marshal).
// This is needed to test that different JSON key orderings of the same input produce the same cache hash.
func queryWithRawVariables(t *testing.T, ctx context.Context, addr, query string, rawVariablesJSON string) []byte {
	t.Helper()

	queryJSON, err := json.Marshal(query)
	require.NoError(t, err)

	var bodyBytes []byte
	if rawVariablesJSON != "" {
		bodyBytes = []byte(`{"query":` + string(queryJSON) + `,"variables":` + rawVariablesJSON + `}`)
	} else {
		bodyBytes = []byte(`{"query":` + string(queryJSON) + `}`)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	return respBody
}

// entityFieldArgsSetup holds common test infrastructure for entity field args caching tests.
type entityFieldArgsSetup struct {
	setup        *federationtesting.FederationSetup
	gqlClient    *GraphqlClient
	ctx          context.Context
	cancel       context.CancelFunc
	defaultCache *FakeLoaderCache
	tracker      *subgraphCallTracker
	accountsHost string
	productsHost string
	reviewsHost  string
}

func newEntityFieldArgsSetup(t *testing.T) *entityFieldArgsSetup {
	t.Helper()

	defaultCache := NewFakeLoaderCache()
	caches := map[string]resolve.LoaderCache{
		"default": defaultCache,
	}

	tracker := newSubgraphCallTracker(http.DefaultTransport)
	trackingClient := &http.Client{
		Transport: tracker,
	}

	subgraphCachingConfigs := engine.SubgraphCachingConfigs{
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
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: false},
			},
		},
	}

	setup := federationtesting.NewFederationSetup(addCachingGateway(
		withCachingEnableART(false),
		withCachingLoaderCache(caches),
		withHTTPClient(trackingClient),
		withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
		withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
	))
	t.Cleanup(setup.Close)

	gqlClient := NewGraphqlClient(http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	accountsURLParsed, err := url.Parse(setup.AccountsUpstreamServer.URL)
	require.NoError(t, err)
	productsURLParsed, err := url.Parse(setup.ProductsUpstreamServer.URL)
	require.NoError(t, err)
	reviewsURLParsed, err := url.Parse(setup.ReviewsUpstreamServer.URL)
	require.NoError(t, err)

	return &entityFieldArgsSetup{
		setup:        setup,
		gqlClient:    gqlClient,
		ctx:          ctx,
		cancel:       cancel,
		defaultCache: defaultCache,
		tracker:      tracker,
		accountsHost: accountsURLParsed.Host,
		productsHost: productsURLParsed.Host,
		reviewsHost:  reviewsURLParsed.Host,
	}
}

func TestEntityFieldArgsCaching(t *testing.T) {
	// peekCache retrieves a cached entry's raw JSON without logging.
	// Returns empty string if the key is not in cache.
	peekCache := func(t *testing.T, s *entityFieldArgsSetup, key string) string {
		t.Helper()
		data, ok := s.defaultCache.Peek(key)
		if !ok {
			return ""
		}
		return string(data)
	}

	t.Run("same args - L2 miss then hit", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		query := `query EntityFieldArgsFormal {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						greeting(style: "formal")
					}
				}
			}
		}`

		// Request 1: greeting(style: "formal") - should miss cache
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, nil, t)

		expectedResp := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedResp, string(resp), "Response should contain formal greeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		assert.Equal(t, 6, len(logAfterFirst), "Should have 6 cache operations (get+set for topProducts, Products, Users)")

		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts - MISS
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			// Product entity fetches - MISS
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			// User entity fetches - MISS (entity key unchanged by field args)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request cache log should show all misses")

		assert.Equal(t, 1, s.tracker.GetCount(s.productsHost), "First request should call products subgraph once")
		assert.Equal(t, 1, s.tracker.GetCount(s.reviewsHost), "First request should call reviews subgraph once")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "First request should call accounts subgraph once")

		// Request 2: same query - should hit cache
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp = s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t, expectedResp, string(resp), "Second request should return identical response from cache")

		// Cache content after Request 2 (unchanged - all hits):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		assert.Equal(t, 3, len(logAfterSecond), "Should have 3 cache get operations (all hits)")

		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HITS
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity fetches - HIT (greeting_<hash> found in cached entity)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request should show all cache hits")

		assert.Equal(t, 0, s.tracker.GetCount(s.productsHost), "Second request should skip products subgraph")
		assert.Equal(t, 0, s.tracker.GetCount(s.reviewsHost), "Second request should skip reviews subgraph")
		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Second request should skip accounts subgraph")
	})

	t.Run("different args - no data mixing", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		queryFormal := `query EntityFieldArgsFormal {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						greeting(style: "formal")
					}
				}
			}
		}`

		queryCasual := `query EntityFieldArgsCasual {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						greeting(style: "casual")
					}
				}
			}
		}`

		// Request 1: greeting(style: "formal")
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)

		expectedFormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedFormal, string(resp1), "First request should return formal greeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		assert.Equal(t, 6, len(logAfterFirst), "Should have 6 cache operations for first request")

		wantLogFirst := []CacheLogEntry{
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request cache log")

		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "First request should call accounts once")

		// Request 2: greeting(style: "casual") - different args, should miss User cache
		// The entity key is the same, but the cached entity lacks greeting_<casualHash>
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCasual, nil, t)

		expectedCasual := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedCasual, string(resp2), "Second request should return casual greeting, not formal")

		// Cache content after Request 2 (User merged: both formal and casual variants present):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User","greeting_e4956d127c0d173e":"Hey, Me!"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()

		// The L2 cache GET returns the User entity (key exists → FakeLoaderCache reports HIT),
		// but the Loader's validateItemHasRequiredData fails because greeting_<casualHash>
		// is missing from the cached entity. The Loader treats it as a miss, re-fetches from
		// accounts, and merges the new data with the old cached entity. So we expect: GET (hit at L2 layer) + SET.
		wantLogSecond := []CacheLogEntry{
			// topProducts root field - HIT
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entities - HIT
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (missing casual field) → re-fetch → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request: User entity found in L2 but missing casual field → re-fetch + re-store")

		// Accounts must be called because the cached entity lacked the casual greeting variant
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called again for different args")
		// topProducts and Products should still hit cache
		assert.Equal(t, 0, s.tracker.GetCount(s.productsHost), "Products should hit cache")
		assert.Equal(t, 0, s.tracker.GetCount(s.reviewsHost), "Reviews should hit cache")
	})

	t.Run("aliases with different args - both cached together", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		query := `query EntityFieldArgsAliases {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						formalGreeting: greeting(style: "formal")
						casualGreeting: greeting(style: "casual")
					}
				}
			}
		}`

		// Request 1: formalGreeting + casualGreeting aliases - both variants in single fetch
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, nil, t)

		expectedAliases := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedAliases, string(resp1), "First request should return both greeting variants")

		// Cache content after Request 1 (both alias variants stored with their respective arg-hash suffixes):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","greeting_e4956d127c0d173e":"Hey, Me!","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			// Product entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			// User entity fetches - MISS (first request, L2 empty; entity stored with both arg-suffixed fields)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request should show all misses")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called once (single entity batch)")

		// Request 2: same aliases query - should fully hit cache
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, nil, t)
		assert.Equal(t, expectedAliases, string(resp2), "Second request should return identical response from cache")

		// Cache content after Request 2 (unchanged - all hits):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","greeting_e4956d127c0d173e":"Hey, Me!","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		assert.Equal(t, 3, len(logAfterSecond), "Should have 3 cache get operations (all hits)")

		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request should show all cache hits")

		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Accounts should not be called on cache hit")
	})

	t.Run("aliases cached then single field hits cache", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		queryAliases := `query EntityFieldArgsAliases {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						formalGreeting: greeting(style: "formal")
						casualGreeting: greeting(style: "casual")
					}
				}
			}
		}`

		queryFormal := `query EntityFieldArgsFormal {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						greeting(style: "formal")
					}
				}
			}
		}`

		// Request 1: cache both variants via aliases
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryAliases, nil, t)

		expectedAliases := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedAliases, string(resp1), "Aliases request should return both greeting variants")

		// Cache content after Request 1 (entity has both greeting variants):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","greeting_e4956d127c0d173e":"Hey, Me!","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			// Product entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			// User entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request should show all misses")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called once")

		// Request 2: single field greeting(style: "formal") - should hit cache
		// The cached entity has both greeting_<formalHash> and greeting_<casualHash>
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)

		expectedFormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedFormal, string(resp2), "Single field request should return formal greeting from cache")

		// Cache content after Request 2 (unchanged - entity still has both variants):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","greeting_e4956d127c0d173e":"Hey, Me!","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		assert.Equal(t, 3, len(logAfterSecond), "Should have 3 cache get operations (all hits)")

		wantLogSecond := []CacheLogEntry{
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// Cached entity has both suffixed fields; formal variant found -> HIT
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Single field request should hit cache with entity that has both variants")

		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Accounts should not be called when formal variant exists in cache")
	})

	t.Run("enum argument - miss then hit", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		query := `query EntityFieldArgsCustomGreeting($input: GreetingInput!) {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						customGreeting(input: $input)
					}
				}
			}
		}`

		vars := queryVariables{"input": map[string]interface{}{"style": "FORMAL"}}

		// Request 1: customGreeting with enum FORMAL - should miss
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, vars, t)

		expectedResp := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedResp, string(resp1), "First request should return formal customGreeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_5c96b2bdff7784c6":"Good day, Me","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			// Product entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			// User entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request should show all misses")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called once")

		// Request 2: same enum value - should hit cache
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, vars, t)
		assert.Equal(t, expectedResp, string(resp2), "Second request should return identical response from cache")

		// Cache content after Request 2 (unchanged - all hits):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_5c96b2bdff7784c6":"Good day, Me","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity fetches - HIT (customGreeting_<formalHash> found in cached entity)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request should show all cache hits")
		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Accounts should not be called on cache hit")
	})

	t.Run("enum argument - different enum values different cache entries", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		query := `query EntityFieldArgsCustomGreeting($input: GreetingInput!) {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						customGreeting(input: $input)
					}
				}
			}
		}`

		varsFormal := queryVariables{"input": map[string]interface{}{"style": "FORMAL"}}
		varsCasual := queryVariables{"input": map[string]interface{}{"style": "CASUAL"}}

		expectedFormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]}]}}`
		expectedCasual := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"Hey, Me!"}}]}]}}`

		// Request 1: FORMAL enum
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, varsFormal, t)
		assert.Equal(t, expectedFormal, string(resp1), "FORMAL should produce formal greeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_5c96b2bdff7784c6":"Good day, Me","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			// Product entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			// User entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request should show all misses")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called once for FORMAL")

		// Request 2: CASUAL enum - different hash, should miss User cache
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, varsCasual, t)
		assert.Equal(t, expectedCasual, string(resp2), "CASUAL should produce casual greeting, not formal")

		// Cache content after Request 2 (User merged: both FORMAL and CASUAL variants present):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_5c96b2bdff7784c6":"Good day, Me","__typename":"User","customGreeting_3fe84620597916f8":"Hey, Me!"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (missing casual enum hash) → re-fetch + merge → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request: User entity found but missing casual enum variant → re-fetch + re-store")

		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called again for different enum value")
		assert.Equal(t, 0, s.tracker.GetCount(s.productsHost), "Products should hit cache")
	})

	t.Run("nested input object - changing nested field produces different hash", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		query := `query EntityFieldArgsCustomGreeting($input: GreetingInput!) {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						customGreeting(input: $input)
					}
				}
			}
		}`

		varsUppercase := queryVariables{"input": map[string]interface{}{
			"style":      "FORMAL",
			"formatting": map[string]interface{}{"uppercase": true},
		}}
		varsNoUppercase := queryVariables{"input": map[string]interface{}{
			"style":      "FORMAL",
			"formatting": map[string]interface{}{"uppercase": false},
		}}

		expectedUppercase := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"GOOD DAY, ME"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"GOOD DAY, ME"}}]}]}}`
		expectedNormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]}]}}`

		// Request 1: uppercase=true
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, varsUppercase, t)
		assert.Equal(t, expectedUppercase, string(resp1), "uppercase=true should produce uppercased greeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_f26a2578aca5e6a1":"GOOD DAY, ME","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			// Product entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			// User entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request should show all misses")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called once")

		// Request 2: uppercase=false - different nested field value, different hash
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, varsNoUppercase, t)
		assert.Equal(t, expectedNormal, string(resp2), "uppercase=false should produce normal greeting")

		// Cache content after Request 2 (User merged: both uppercase=true and uppercase=false variants present):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_f26a2578aca5e6a1":"GOOD DAY, ME","__typename":"User","customGreeting_e5bb1eb0d1896f64":"Good day, Me"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (different nested field hash) → re-fetch + merge → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request: User entity found but missing uppercase=false variant → re-fetch + re-store")

		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called again for different nested field value")
	})

	t.Run("nested input object - different nested fields present", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		query := `query EntityFieldArgsCustomGreeting($input: GreetingInput!) {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						customGreeting(input: $input)
					}
				}
			}
		}`

		varsUppercase := queryVariables{"input": map[string]interface{}{
			"style":      "FORMAL",
			"formatting": map[string]interface{}{"uppercase": true},
		}}
		varsPrefix := queryVariables{"input": map[string]interface{}{
			"style":      "FORMAL",
			"formatting": map[string]interface{}{"prefix": "Dr."},
		}}

		expectedUppercase := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"GOOD DAY, ME"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"GOOD DAY, ME"}}]}]}}`
		expectedPrefix := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"Dr. Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"Dr. Good day, Me"}}]}]}}`

		// Request 1: formatting with uppercase
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, varsUppercase, t)
		assert.Equal(t, expectedUppercase, string(resp1), "uppercase should produce uppercased greeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_f26a2578aca5e6a1":"GOOD DAY, ME","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			// Product entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			// User entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request should show all misses")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called once")

		// Request 2: formatting with prefix - different fields present, different hash
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, query, varsPrefix, t)
		assert.Equal(t, expectedPrefix, string(resp2), "prefix should produce prefixed greeting")

		// Cache content after Request 2 (User merged: both uppercase and prefix variants present):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_f26a2578aca5e6a1":"GOOD DAY, ME","__typename":"User","customGreeting_cc61634e04b7fbf6":"Dr. Good day, Me"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (different nested fields hash) → re-fetch + merge → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request: User entity found but missing prefix variant → re-fetch + re-store")

		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called again for different nested fields")
	})

	t.Run("nested input object - same fields different key order produces same hash", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		query := `query EntityFieldArgsCustomGreeting($input: GreetingInput!) {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						customGreeting(input: $input)
					}
				}
			}
		}`

		expectedResp := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"GOOD DAY, ME"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"GOOD DAY, ME"}}]}]}}`

		// Request 1: style first, then formatting (raw JSON to preserve key order)
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := queryWithRawVariables(t, s.ctx, s.setup.GatewayServer.URL,
			query,
			`{"input":{"style":"FORMAL","formatting":{"uppercase":true}}}`)
		assert.Equal(t, expectedResp, string(resp1), "Order 1 should produce uppercased greeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_f26a2578aca5e6a1":"GOOD DAY, ME","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// Root field Query.topProducts - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			// Product entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			// User entity fetches - MISS (first request, L2 empty)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "First request should show all misses")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called once for order 1")

		// Request 2: formatting first, then style (same logical input, different JSON key order)
		// Raw JSON ensures the key order is preserved as-is (Go's json.Marshal would sort keys)
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := queryWithRawVariables(t, s.ctx, s.setup.GatewayServer.URL,
			query,
			`{"input":{"formatting":{"uppercase":true},"style":"FORMAL"}}`)
		assert.Equal(t, expectedResp, string(resp2), "Order 2 should produce same uppercased greeting")

		// Cache content after Request 2 (unchanged - canonical JSON hashing makes key order irrelevant):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","customGreeting_f26a2578aca5e6a1":"GOOD DAY, ME","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - HIT (canonical JSON hashing makes key order irrelevant)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request should show all cache hits (key order canonicalized)")

		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Accounts should NOT be called when same input is sent with different key order")
	})

	t.Run("different args merge enables third request cache hit", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		queryFormal := `query EntityFieldArgsFormal {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						greeting(style: "formal")
					}
				}
			}
		}`

		queryCasual := `query EntityFieldArgsCasual {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						greeting(style: "casual")
					}
				}
			}
		}`

		// Request 1: greeting(style: "formal") → L2 miss → fetch → store
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)

		expectedFormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedFormal, string(resp1), "Request 1 should return formal greeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// All misses on first request - L2 empty
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Request 1: all misses, populate cache")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Request 1 should call accounts once")

		// Request 2: greeting(style: "casual") → L2 validation fails → fetch → merge-store
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCasual, nil, t)

		expectedCasual := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedCasual, string(resp2), "Request 2 should return casual greeting")

		// Cache content after Request 2 (merged: both formal and casual variants present):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User","greeting_e4956d127c0d173e":"Hey, Me!"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// topProducts and Products - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (missing casual field) → re-fetch + merge → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Request 2: User entity found but missing casual field → re-fetch + merge")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Request 2 should call accounts once (casual variant missing)")

		// Request 3: greeting(style: "formal") again → L2 HIT (formal variant exists in merged entity)
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp3 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)
		assert.Equal(t, expectedFormal, string(resp3), "Request 3 should return formal greeting from cache")

		// Cache content after Request 3 (unchanged - full cache hit, no write):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User","greeting_e4956d127c0d173e":"Hey, Me!"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterThird := s.defaultCache.GetLog()
		wantLogThird := []CacheLogEntry{
			// All GETs are hits - no SETs needed
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - HIT (formal variant exists in merged entity from Request 2)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogThird), sortCacheLogKeys(logAfterThird), "Request 3: all cache hits, no fetches needed")

		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Request 3 should NOT call accounts (formal variant in merged cache)")
	})

	t.Run("different args merge enables combined alias cache hit", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		queryFormal := `query EntityFieldArgsFormal {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						greeting(style: "formal")
					}
				}
			}
		}`

		queryCasual := `query EntityFieldArgsCasual {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						greeting(style: "casual")
					}
				}
			}
		}`

		queryBothAliases := `query EntityFieldArgsBothAliases {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						formalGreeting: greeting(style: "formal")
						casualGreeting: greeting(style: "casual")
					}
				}
			}
		}`

		// Request 1: greeting(style: "formal") → L2 miss → fetch → store
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)

		expectedFormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedFormal, string(resp1), "Request 1 should return formal greeting")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// All misses on first request - L2 empty
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Request 1: all misses, populate cache")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Request 1 should call accounts once")

		// Request 2: greeting(style: "casual") → L2 validation fails → fetch → merge-store
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCasual, nil, t)

		expectedCasual := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedCasual, string(resp2), "Request 2 should return casual greeting")

		// Cache content after Request 2 (merged: both variants present):
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User","greeting_e4956d127c0d173e":"Hey, Me!"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// topProducts and Products - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (missing casual field) → re-fetch + merge → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Request 2: User entity found but missing casual field → re-fetch + merge")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Request 2 should call accounts once (casual variant missing)")

		// Request 3: combined alias query with both variants → L2 HIT (both variants exist in merged entity)
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp3 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryBothAliases, nil, t)

		expectedBoth := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedBoth, string(resp3), "Request 3 should return both greeting variants from cache")

		// Cache content after Request 3 (unchanged - full cache hit, no write):
		assert.Equal(t,
			`{"username":"Me","greeting_1dc2e714f80c47e8":"Good day, Me","__typename":"User","greeting_e4956d127c0d173e":"Hey, Me!"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterThird := s.defaultCache.GetLog()
		wantLogThird := []CacheLogEntry{
			// All GETs are hits - no SETs needed
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - HIT (both variants exist in merged entity)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogThird), sortCacheLogKeys(logAfterThird), "Request 3: all cache hits, both variants served from merged entity")

		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Request 3 should NOT call accounts (both variants in merged cache)")
	})

	t.Run("non-arg fields merge across fetches", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		queryUsernameOnly := `query UsernameOnly {
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

		queryUsernameAndNickname := `query UsernameAndNickname {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						username
						nickname
					}
				}
			}
		}`

		queryNicknameOnly := `query NicknameOnly {
			topProducts {
				name
				reviews {
					body
					authorWithoutProvides {
						nickname
					}
				}
			}
		}`

		// Request 1: username only → L2 miss → fetch → store
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryUsernameOnly, nil, t)

		expectedUsernameOnly := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`
		assert.Equal(t, expectedUsernameOnly, string(resp1), "Request 1 should return username only")

		// Cache content after Request 1:
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"__typename":"User","id":"1234","username":"Me"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterFirst := s.defaultCache.GetLog()
		wantLogFirst := []CacheLogEntry{
			// All misses on first request - L2 empty
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{false, false}},
			{Operation: "set", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}},
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{false}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst), "Request 1: all misses, populate cache")
		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Request 1 should call accounts once")

		// Request 2: username + nickname → L2 validation fails (missing nickname) → fetch → merge-store
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryUsernameAndNickname, nil, t)

		expectedUsernameAndNickname := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","nickname":"nick-Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","nickname":"nick-Me"}}]}]}}`
		assert.Equal(t, expectedUsernameAndNickname, string(resp2), "Request 2 should return username and nickname")

		// Cache content after Request 2 (merged: both username and nickname present):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"__typename":"User","id":"1234","username":"Me","nickname":"nick-Me"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (missing nickname) → re-fetch + merge → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Request 2: User entity found but missing nickname → re-fetch + merge")

		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Request 2 should call accounts once (nickname missing)")

		// Request 3: nickname only → L2 HIT (nickname exists in merged entity)
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp3 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryNicknameOnly, nil, t)

		expectedNicknameOnly := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"nickname":"nick-Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"nickname":"nick-Me"}}]}]}}`
		assert.Equal(t, expectedNicknameOnly, string(resp3), "Request 3 should return nickname from cache")

		// Cache content after Request 3 (unchanged - full cache hit, no write):
		assert.Equal(t,
			`{"topProducts":[{"name":"Trilby","__typename":"Product","upc":"top-1"},{"name":"Fedora","__typename":"Product","upc":"top-2"}]}`,
			peekCache(t, s, `{"__typename":"Query","field":"topProducts"}`))
		assert.Equal(t,
			`{"name":"Trilby","__typename":"Product","upc":"top-1","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-1"}}`))
		assert.Equal(t,
			`{"name":"Fedora","__typename":"Product","upc":"top-2","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"__typename":"User","id":"1234"}}]}`,
			peekCache(t, s, `{"__typename":"Product","key":{"upc":"top-2"}}`))
		assert.Equal(t,
			`{"__typename":"User","id":"1234","username":"Me","nickname":"nick-Me"}`,
			peekCache(t, s, `{"__typename":"User","key":{"id":"1234"}}`))

		logAfterThird := s.defaultCache.GetLog()
		wantLogThird := []CacheLogEntry{
			// All GETs are hits - no SETs needed
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - HIT (nickname exists in merged entity from Request 2)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogThird), sortCacheLogKeys(logAfterThird), "Request 3: all cache hits, nickname served from merged entity")

		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Request 3 should NOT call accounts (nickname in merged cache)")
	})
}
