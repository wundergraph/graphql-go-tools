package engine_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	accounts "github.com/wundergraph/graphql-go-tools/execution/federationtesting/accounts/graph"
	products "github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph"
	reviews "github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Standard queries and keys used by all extensions cache invalidation tests.
const (
	extInvEntityQuery   = `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
	extInvMutationQuery = `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
	extInvUserKey       = `{"__typename":"User","key":{"id":"1234"}}`

	// Expected gateway responses (exact).
	entityResponseMe       = `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`
	entityResponseUpdated  = `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"UpdatedMe"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"UpdatedMe"}}]}]}}`
	mutationResponse       = `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`
	entitiesSubgraphRespMe = `{"data":{"_entities":[{"__typename":"User","username":"Me"}]}}`
)

// injectCacheInvalidation injects a raw JSON cacheInvalidation object into a subgraph
// response's extensions field and returns the modified response body.
func injectCacheInvalidation(t *testing.T, body []byte, cacheInvalidationJSON string) []byte {
	t.Helper()
	var resp map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &resp))
	resp["extensions"] = json.RawMessage(`{"cacheInvalidation":` + cacheInvalidationJSON + `}`)
	modified, err := json.Marshal(resp)
	require.NoError(t, err)
	return modified
}

// subgraphResponseInterceptor wraps a subgraph HTTP handler and applies a modifier
// function to every response body when set. When modifier is nil, responses pass through.
type subgraphResponseInterceptor struct {
	handler  http.Handler
	mu       sync.RWMutex
	modifier func(body []byte) []byte
}

func newSubgraphResponseInterceptor(handler http.Handler) *subgraphResponseInterceptor {
	return &subgraphResponseInterceptor{handler: handler}
}

func (s *subgraphResponseInterceptor) SetModifier(fn func(body []byte) []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modifier = fn
}

func (s *subgraphResponseInterceptor) ClearModifier() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modifier = nil
}

func (s *subgraphResponseInterceptor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	mod := s.modifier
	s.mu.RUnlock()

	if mod == nil {
		s.handler.ServeHTTP(w, r)
		return
	}

	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, r)

	modified := mod(rec.Body.Bytes())

	maps.Copy(w.Header(), rec.Header())
	w.Header().Set("Content-Length", strconv.Itoa(len(modified)))
	w.WriteHeader(rec.Code)
	_, _ = w.Write(modified)
}

// newFederationSetupWithInterceptor creates a FederationSetup where the accounts subgraph
// is wrapped with the response interceptor.
func newFederationSetupWithInterceptor(
	interceptor *subgraphResponseInterceptor,
	gatewayFn func(*federationtesting.FederationSetup) *httptest.Server,
) *federationtesting.FederationSetup {
	accountsServer := httptest.NewServer(interceptor)
	productsServer := httptest.NewServer(products.GraphQLEndpointHandler(products.TestOptions))
	reviewsServer := httptest.NewServer(reviews.GraphQLEndpointHandler(reviews.TestOptions))

	setup := &federationtesting.FederationSetup{
		AccountsUpstreamServer: accountsServer,
		ProductsUpstreamServer: productsServer,
		ReviewsUpstreamServer:  reviewsServer,
	}

	setup.GatewayServer = gatewayFn(setup)
	return setup
}

// ---------------------------------------------------------------------------
// extInvalidationEnv — test environment for extensions cache invalidation tests
// ---------------------------------------------------------------------------

type extInvalidationOption func(*extInvalidationConfig)

type extInvalidationConfig struct {
	mutationCacheInvalidationField string
	headerPrefixHash               uint64
	useHeaderPrefix                bool
	l2KeyInterceptor               func(ctx context.Context, key string, info resolve.L2CacheKeyInterceptorInfo) string
}

// withMutationCacheInvalidation enables the config-based MutationCacheInvalidation
// mechanism for the given mutation field (e.g. "updateUsername").
func withMutationCacheInvalidation(fieldName string) extInvalidationOption {
	return func(c *extInvalidationConfig) {
		c.mutationCacheInvalidationField = fieldName
	}
}

// withHeaderPrefix enables IncludeSubgraphHeaderPrefix on the User entity config
// and sets up a mockSubgraphHeadersBuilder with the given hash for "accounts".
func withHeaderPrefix(hash uint64) extInvalidationOption {
	return func(c *extInvalidationConfig) {
		c.useHeaderPrefix = true
		c.headerPrefixHash = hash
	}
}

// withL2KeyInterceptor sets an L2CacheKeyInterceptor on the caching options.
func withExtInvL2KeyInterceptor(fn func(ctx context.Context, key string, info resolve.L2CacheKeyInterceptorInfo) string) extInvalidationOption {
	return func(c *extInvalidationConfig) {
		c.l2KeyInterceptor = fn
	}
}

type extInvalidationEnv struct {
	t            *testing.T
	cache        *FakeLoaderCache
	tracker      *subgraphCallTracker
	interceptor  *subgraphResponseInterceptor
	setup        *federationtesting.FederationSetup
	gqlClient    *GraphqlClient
	accountsHost string
	ctx          context.Context
}

// newExtInvalidationEnv creates a fully wired test environment for extensions
// cache invalidation E2E tests. All boilerplate (cache, tracker, interceptor,
// federation setup, gateway, cleanup) is handled here.
func newExtInvalidationEnv(t *testing.T, opts ...extInvalidationOption) *extInvalidationEnv {
	t.Helper()

	accounts.ResetUsers()
	t.Cleanup(accounts.ResetUsers)

	var cfg extInvalidationConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// Build entity cache config.
	entityCfg := plan.EntityCacheConfiguration{
		TypeName:                    "User",
		CacheName:                   "default",
		TTL:                         30 * time.Second,
		IncludeSubgraphHeaderPrefix: cfg.useHeaderPrefix,
	}

	subgraphCfg := engine.SubgraphCachingConfig{
		SubgraphName:  "accounts",
		EntityCaching: plan.EntityCacheConfigurations{entityCfg},
	}
	if cfg.mutationCacheInvalidationField != "" {
		subgraphCfg.MutationCacheInvalidation = plan.MutationCacheInvalidationConfigurations{
			{FieldName: cfg.mutationCacheInvalidationField},
		}
	}

	cachingOpts := resolve.CachingOptions{EnableL2Cache: true}
	if cfg.l2KeyInterceptor != nil {
		cachingOpts.L2CacheKeyInterceptor = cfg.l2KeyInterceptor
	}

	cache := NewFakeLoaderCache()
	caches := map[string]resolve.LoaderCache{"default": cache}
	tracker := newSubgraphCallTracker(http.DefaultTransport)
	trackingClient := &http.Client{Transport: tracker}
	interceptor := newSubgraphResponseInterceptor(accounts.GraphQLEndpointHandler(accounts.TestOptions))

	gatewayOpts := []cachingGatewayOptionsToFunc{
		withCachingEnableART(false),
		withCachingLoaderCache(caches),
		withHTTPClient(trackingClient),
		withCachingOptionsFunc(cachingOpts),
		withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{subgraphCfg}),
	}
	if cfg.useHeaderPrefix {
		gatewayOpts = append(gatewayOpts, withSubgraphHeadersBuilder(&mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{"accounts": cfg.headerPrefixHash},
		}))
	}

	setup := newFederationSetupWithInterceptor(interceptor, addCachingGateway(gatewayOpts...))
	t.Cleanup(setup.Close)

	return &extInvalidationEnv{
		t:            t,
		cache:        cache,
		tracker:      tracker,
		interceptor:  interceptor,
		setup:        setup,
		gqlClient:    NewGraphqlClient(http.DefaultClient),
		accountsHost: mustParseHost(setup.AccountsUpstreamServer.URL),
		ctx:          t.Context(),
	}
}

// resetCounters resets the subgraph call tracker and clears the cache operation log.
func (e *extInvalidationEnv) resetCounters() {
	e.tracker.Reset()
	e.cache.ClearLog()
}

// queryEntity sends the standard entity query, resets counters first.
func (e *extInvalidationEnv) queryEntity() string {
	e.t.Helper()
	e.resetCounters()
	return string(e.gqlClient.QueryString(e.ctx, e.setup.GatewayServer.URL, extInvEntityQuery, nil, e.t))
}

// mutate sends the standard mutation, resets counters first.
func (e *extInvalidationEnv) mutate() string {
	e.t.Helper()
	e.resetCounters()
	return string(e.gqlClient.QueryString(e.ctx, e.setup.GatewayServer.URL, extInvMutationQuery, nil, e.t))
}

// onAccountsResponse sets a modifier on the accounts subgraph interceptor.
func (e *extInvalidationEnv) onAccountsResponse(fn func(body []byte) []byte) {
	e.interceptor.SetModifier(fn)
}

// clearModifier removes the interceptor modifier.
func (e *extInvalidationEnv) clearModifier() {
	e.interceptor.ClearModifier()
}

// cacheLog returns the current cache log with keys sorted for deterministic comparison.
func (e *extInvalidationEnv) cacheLog() []CacheLogEntry {
	return sortCacheLogKeys(e.cache.GetLog())
}

// accountsCalls returns the number of HTTP calls made to the accounts subgraph.
func (e *extInvalidationEnv) accountsCalls() int {
	return e.tracker.GetCount(e.accountsHost)
}

// deleteFromCache manually deletes keys from the L2 cache.
func (e *extInvalidationEnv) deleteFromCache(keys ...string) {
	e.t.Helper()
	err := e.cache.Delete(e.ctx, keys)
	require.NoError(e.t, err)
}
