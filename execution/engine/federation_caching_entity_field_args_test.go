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

	accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
	productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
	reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)

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
	const queryFormal = `query EntityFieldArgsFormal {
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

	const queryCasual = `query EntityFieldArgsCasual {
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

	const queryAliases = `query EntityFieldArgsAliases {
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

	const queryCustomGreeting = `query EntityFieldArgsCustomGreeting($input: GreetingInput!) {
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

	t.Run("same args - L2 miss then hit", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		// Request 1: greeting(style: "formal") - should miss cache
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)

		expectedResp := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedResp, string(resp), "Response should contain formal greeting")

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
		resp = s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)
		assert.Equal(t, expectedResp, string(resp), "Second request should return identical response from cache")

		logAfterSecond := s.defaultCache.GetLog()
		assert.Equal(t, 3, len(logAfterSecond), "Should have 3 cache get operations (all hits)")

		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HITS
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity fetches - HIT (greeting_xxh<hash> found in cached entity)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request should show all cache hits")

		assert.Equal(t, 0, s.tracker.GetCount(s.productsHost), "Second request should skip products subgraph")
		assert.Equal(t, 0, s.tracker.GetCount(s.reviewsHost), "Second request should skip reviews subgraph")
		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Second request should skip accounts subgraph")
	})

	t.Run("different args - no data mixing", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		// Request 1: greeting(style: "formal")
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)

		expectedFormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedFormal, string(resp1), "First request should return formal greeting")

		logAfterFirst := s.defaultCache.GetLog()
		assert.Equal(t, 6, len(logAfterFirst), "Should have 6 cache operations for first request")

		// Verify the User entity cache key is the standard entity key (no arg suffix)
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
		// The entity key is the same, but the cached entity lacks greeting_xxh<casualHash>
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCasual, nil, t)

		expectedCasual := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedCasual, string(resp2), "Second request should return casual greeting, not formal")

		logAfterSecond := s.defaultCache.GetLog()

		// The L2 cache GET returns the User entity (key exists → FakeLoaderCache reports HIT),
		// but the Loader's validateItemHasRequiredData fails because greeting_xxh<casualHash>
		// is missing from the cached entity. The Loader treats it as a miss, re-fetches from
		// accounts, and stores the new data. So we expect: GET (hit at L2 layer) + SET.
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

		// Request 1: formalGreeting + casualGreeting aliases - both variants in single fetch
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryAliases, nil, t)

		expectedAliases := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedAliases, string(resp1), "First request should return both greeting variants")

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
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryAliases, nil, t)
		assert.Equal(t, expectedAliases, string(resp2), "Second request should return identical response from cache")

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

		// Request 1: cache both variants via aliases
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryAliases, nil, t)

		expectedAliases := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","formalGreeting":"Good day, Me","casualGreeting":"Hey, Me!"}}]}]}}`
		assert.Equal(t, expectedAliases, string(resp1), "Aliases request should return both greeting variants")

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
		// The cached entity has both greeting_xxh<formalHash> and greeting_xxh<casualHash>
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryFormal, nil, t)

		expectedFormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","greeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedFormal, string(resp2), "Single field request should return formal greeting from cache")

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

		vars := queryVariables{"input": map[string]interface{}{"style": "FORMAL"}}

		// Request 1: customGreeting with enum FORMAL - should miss
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCustomGreeting, vars, t)

		expectedResp := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]}]}}`
		assert.Equal(t, expectedResp, string(resp1), "First request should return formal customGreeting")

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
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCustomGreeting, vars, t)
		assert.Equal(t, expectedResp, string(resp2), "Second request should return identical response from cache")

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity fetches - HIT (customGreeting_xxh<formalHash> found in cached entity)
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request should show all cache hits")
		assert.Equal(t, 0, s.tracker.GetCount(s.accountsHost), "Accounts should not be called on cache hit")
	})

	t.Run("enum argument - different enum values different cache entries", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		varsFormal := queryVariables{"input": map[string]interface{}{"style": "FORMAL"}}
		varsCasual := queryVariables{"input": map[string]interface{}{"style": "CASUAL"}}

		expectedFormal := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"Good day, Me"}}]}]}}`
		expectedCasual := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"Hey, Me!"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"Hey, Me!"}}]}]}}`

		// Request 1: FORMAL enum
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCustomGreeting, varsFormal, t)
		assert.Equal(t, expectedFormal, string(resp1), "FORMAL should produce formal greeting")

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
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCustomGreeting, varsCasual, t)
		assert.Equal(t, expectedCasual, string(resp2), "CASUAL should produce casual greeting, not formal")

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (missing casual enum hash) → re-fetch → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request: User entity found but missing casual enum variant → re-fetch + re-store")

		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called again for different enum value")
		assert.Equal(t, 0, s.tracker.GetCount(s.productsHost), "Products should hit cache")
	})

	t.Run("nested input object - changing nested field produces different hash", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

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
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCustomGreeting, varsUppercase, t)
		assert.Equal(t, expectedUppercase, string(resp1), "uppercase=true should produce uppercased greeting")

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
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCustomGreeting, varsNoUppercase, t)
		assert.Equal(t, expectedNormal, string(resp2), "uppercase=false should produce normal greeting")

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (different nested field hash) → re-fetch → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request: User entity found but missing uppercase=false variant → re-fetch + re-store")

		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called again for different nested field value")
	})

	t.Run("nested input object - different nested fields present", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

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
		resp1 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCustomGreeting, varsUppercase, t)
		assert.Equal(t, expectedUppercase, string(resp1), "uppercase should produce uppercased greeting")

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
		resp2 := s.gqlClient.QueryString(s.ctx, s.setup.GatewayServer.URL, queryCustomGreeting, varsPrefix, t)
		assert.Equal(t, expectedPrefix, string(resp2), "prefix should produce prefixed greeting")

		logAfterSecond := s.defaultCache.GetLog()
		wantLogSecond := []CacheLogEntry{
			// Root field Query.topProducts - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Query","field":"topProducts"}`}, Hits: []bool{true}},
			// Product entity fetches - HIT (populated by Request 1)
			{Operation: "get", Keys: []string{`{"__typename":"Product","key":{"upc":"top-1"}}`, `{"__typename":"Product","key":{"upc":"top-2"}}`}, Hits: []bool{true, true}},
			// User entity - L2 returns data (HIT) but Loader rejects it (different nested fields hash) → re-fetch → SET
			{Operation: "get", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}, Hits: []bool{true}},
			{Operation: "set", Keys: []string{`{"__typename":"User","key":{"id":"1234"}}`}},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond), "Second request: User entity found but missing prefix variant → re-fetch + re-store")

		assert.Equal(t, 1, s.tracker.GetCount(s.accountsHost), "Accounts should be called again for different nested fields")
	})

	t.Run("nested input object - same fields different key order produces same hash", func(t *testing.T) {
		s := newEntityFieldArgsSetup(t)

		expectedResp := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me","customGreeting":"GOOD DAY, ME"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me","customGreeting":"GOOD DAY, ME"}}]}]}}`

		// Request 1: style first, then formatting (raw JSON to preserve key order)
		s.defaultCache.ClearLog()
		s.tracker.Reset()
		resp1 := queryWithRawVariables(t, s.ctx, s.setup.GatewayServer.URL,
			queryCustomGreeting,
			`{"input":{"style":"FORMAL","formatting":{"uppercase":true}}}`)
		assert.Equal(t, expectedResp, string(resp1), "Order 1 should produce uppercased greeting")

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
			queryCustomGreeting,
			`{"input":{"formatting":{"uppercase":true},"style":"FORMAL"}}`)
		assert.Equal(t, expectedResp, string(resp2), "Order 2 should produce same uppercased greeting")

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
}
