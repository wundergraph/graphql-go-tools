package engine_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	accounts "github.com/wundergraph/graphql-go-tools/execution/federationtesting/accounts/graph"
	products "github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph"
	reviews "github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// injectCacheInvalidation injects a raw JSON cacheInvalidation object into a subgraph
// response's extensions field and returns the modified response body.
//
// cacheInvalidationJSON is the complete cacheInvalidation object value, e.g.:
//
//	`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`
//
// Given a subgraph response like:
//
//	{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}
//
// The result will be:
//
//	{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}},"extensions":{"cacheInvalidation":{"keys":[...]}}}
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
//
// Usage in tests:
//
//	interceptor.SetModifier(func(body []byte) []byte {
//	    assert.Equal(t, expectedResponse, string(body))
//	    return injectCacheInvalidation(t, body, `{"keys":[...]}`)
//	})
type subgraphResponseInterceptor struct {
	handler  http.Handler
	mu       sync.RWMutex
	modifier func(body []byte) []byte
}

func newSubgraphResponseInterceptor(handler http.Handler) *subgraphResponseInterceptor {
	return &subgraphResponseInterceptor{handler: handler}
}

// SetModifier sets a function that will be applied to every subsequent subgraph response.
// The function receives the raw response body and returns the modified body.
func (s *subgraphResponseInterceptor) SetModifier(fn func(body []byte) []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modifier = fn
}

// ClearModifier removes the modifier — responses pass through unmodified.
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
// is wrapped with the response interceptor. Products and reviews are unmodified.
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

func TestFederationCaching_ExtensionsInvalidation(t *testing.T) {
	// Query that resolves User entity via _entities (no @provides) to populate L2 cache.
	entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
	mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`

	t.Run("mutation with extensions invalidation clears L2 cache", func(t *testing.T) {
		accounts.ResetUsers()
		t.Cleanup(accounts.ResetUsers)

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		interceptor := newSubgraphResponseInterceptor(accounts.GraphQLEndpointHandler(accounts.TestOptions))

		setup := newFederationSetupWithInterceptor(interceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx := t.Context()

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Step 1: Query populates L2 cache with User:1234 entity.
		// Accounts subgraph returns normal response (no extensions).
		tracker.Reset()
		defaultCache.ClearLog()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Contains(t, string(resp), `"username":"Me"`)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts subgraph once")

		// Step 2: Same query — L2 hit, no accounts call.
		tracker.Reset()
		defaultCache.ClearLog()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Contains(t, string(resp), `"username":"Me"`)
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Step 2: should NOT call accounts (L2 hit)")

		// Step 3: Mutation — inject cache invalidation into the accounts subgraph response.
		// The modifier function shows exactly what the subgraph returns and what we inject.
		interceptor.SetModifier(func(body []byte) []byte {
			// Verify the subgraph mutation response.
			assert.Equal(t,
				`{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`,
				string(body),
			)
			// Inject cache invalidation extensions into the response.
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`,
			)
		})

		tracker.Reset()
		defaultCache.ClearLog()
		respMut := gqlClient.QueryString(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)
		assert.Contains(t, string(respMut), `"UpdatedMe"`)

		interceptor.ClearModifier()

		// Verify cache delete operation occurred.
		mutationLog := defaultCache.GetLog()
		hasDelete := false
		for _, entry := range mutationLog {
			if entry.Operation == "delete" {
				hasDelete = true
				assert.Len(t, entry.Keys, 1, "delete should have exactly 1 key")
				assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, entry.Keys[0])
			}
		}
		assert.True(t, hasDelete, "mutation should trigger a cache delete from extensions")

		// Step 4: Same query — L2 miss (entry deleted), re-fetch from accounts.
		tracker.Reset()
		defaultCache.ClearLog()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Contains(t, string(resp), `"username":"UpdatedMe"`)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 4: should call accounts (L2 was invalidated)")
	})

	t.Run("invalidation of entity not in cache is a no-op", func(t *testing.T) {
		accounts.ResetUsers()
		t.Cleanup(accounts.ResetUsers)

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		interceptor := newSubgraphResponseInterceptor(accounts.GraphQLEndpointHandler(accounts.TestOptions))

		setup := newFederationSetupWithInterceptor(interceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx := t.Context()

		// Populate cache with User:1234.
		tracker.Reset()
		defaultCache.ClearLog()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Contains(t, string(resp), `"username":"Me"`)

		// Mutation invalidates User:9999 (never cached).
		// The modifier shows exactly what the subgraph returns and what invalidation we inject.
		interceptor.SetModifier(func(body []byte) []byte {
			assert.Equal(t,
				`{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`,
				string(body),
			)
			// Invalidate User:9999 which was never cached — should be a no-op.
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"9999"}}]}`,
			)
		})

		tracker.Reset()
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)

		interceptor.ClearModifier()

		// Verify delete was called (even though entry didn't exist in cache).
		mutationLog := defaultCache.GetLog()
		hasDelete := false
		for _, entry := range mutationLog {
			if entry.Operation == "delete" {
				hasDelete = true
			}
		}
		assert.True(t, hasDelete, "delete should still be called for non-existent entry")

		// Verify User:1234 is still cached (unaffected by User:9999 invalidation).
		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)
		tracker.Reset()
		defaultCache.ClearLog()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Contains(t, string(resp), `"username":"Me"`)
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "User:1234 should still be cached")
	})

	t.Run("multiple entities invalidated in single response", func(t *testing.T) {
		accounts.ResetUsers()
		t.Cleanup(accounts.ResetUsers)

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		interceptor := newSubgraphResponseInterceptor(accounts.GraphQLEndpointHandler(accounts.TestOptions))

		setup := newFederationSetupWithInterceptor(interceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx := t.Context()

		// Populate cache.
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)

		// Mutation invalidates both User:1234 and User:2345 in a single response.
		interceptor.SetModifier(func(body []byte) []byte {
			assert.Equal(t,
				`{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`,
				string(body),
			)
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}},{"typename":"User","key":{"id":"2345"}}]}`,
			)
		})

		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)

		interceptor.ClearModifier()

		mutationLog := defaultCache.GetLog()
		var deleteKeys []string
		for _, entry := range mutationLog {
			if entry.Operation == "delete" {
				deleteKeys = append(deleteKeys, entry.Keys...)
			}
		}
		slices.Sort(deleteKeys)
		assert.Equal(t, []string{
			`{"__typename":"User","key":{"id":"1234"}}`,
			`{"__typename":"User","key":{"id":"2345"}}`,
		}, deleteKeys)

		// Verify User:1234 is re-fetched.
		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)
		tracker.Reset()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "User:1234 should be re-fetched after invalidation")
	})

	t.Run("mutation without extensions does not delete", func(t *testing.T) {
		accounts.ResetUsers()
		t.Cleanup(accounts.ResetUsers)

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		// No interceptor — accounts subgraph returns normal responses without extensions.
		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx := t.Context()

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Populate cache.
		tracker.Reset()
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)

		// Verify cache hit.
		tracker.Reset()
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "should hit L2 cache")

		// Mutation WITHOUT extensions — should NOT delete cache.
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)

		mutationLog := defaultCache.GetLog()
		for _, entry := range mutationLog {
			assert.NotEqual(t, "delete", entry.Operation, "no delete should occur without extensions or MutationCacheInvalidation")
		}

		// Cache should still be valid.
		tracker.Reset()
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "cache should still be valid")
	})

	t.Run("coexistence with detectMutationEntityImpact", func(t *testing.T) {
		accounts.ResetUsers()
		t.Cleanup(accounts.ResetUsers)

		// Configure BOTH MutationCacheInvalidation (existing config-based feature)
		// AND entity cache configs (for extensions-based invalidation).
		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
				MutationCacheInvalidation: plan.MutationCacheInvalidationConfigurations{
					{FieldName: "updateUsername"},
				},
			},
		}

		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		interceptor := newSubgraphResponseInterceptor(accounts.GraphQLEndpointHandler(accounts.TestOptions))

		setup := newFederationSetupWithInterceptor(interceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx := t.Context()

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Populate cache.
		tracker.Reset()
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "should call accounts")

		// Verify cache hit.
		tracker.Reset()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "should hit cache")

		// Mutation triggers BOTH mechanisms:
		// 1. detectMutationEntityImpact fires because MutationCacheInvalidation is configured
		// 2. extensions-based invalidation fires because we inject cacheInvalidation extensions
		interceptor.SetModifier(func(body []byte) []byte {
			assert.Equal(t,
				`{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`,
				string(body),
			)
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`,
			)
		})

		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)

		interceptor.ClearModifier()

		// Both mechanisms should fire — one delete from detectMutationEntityImpact
		// and one from extensions-based invalidation.
		mutationLog := defaultCache.GetLog()
		deleteCount := 0
		for _, entry := range mutationLog {
			if entry.Operation == "delete" {
				deleteCount++
			}
		}
		assert.Equal(t, 2, deleteCount, "should have exactly 2 delete calls: one from mutation impact, one from extensions")

		// Cache should be invalidated — query should re-fetch.
		tracker.Reset()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "should re-fetch after combined invalidation")
	})

	t.Run("query response triggers invalidation", func(t *testing.T) {
		accounts.ResetUsers()
		t.Cleanup(accounts.ResetUsers)

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		interceptor := newSubgraphResponseInterceptor(accounts.GraphQLEndpointHandler(accounts.TestOptions))

		setup := newFederationSetupWithInterceptor(interceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx := t.Context()

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Step 1: Query populates L2 cache (no extensions).
		tracker.Reset()
		defaultCache.ClearLog()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Contains(t, string(resp), `"username":"Me"`)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 1: should call accounts")

		// Step 2: Verify cache hit.
		tracker.Reset()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "Step 2: should hit cache")

		// Step 3: Clear the cache entry so the next query calls accounts again.
		// Then enable extension injection to verify that a QUERY response (not mutation)
		// can also trigger invalidation.
		_ = defaultCache.Delete(ctx, []string{`{"__typename":"User","key":{"id":"1234"}}`})

		// The _entities query response will include invalidation extensions.
		// This proves invalidation is NOT restricted to mutations.
		interceptor.SetModifier(func(body []byte) []byte {
			// This is an _entities response for User:1234.
			assert.Contains(t, string(body), `"username":"Me"`)
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`,
			)
		})

		tracker.Reset()
		defaultCache.ClearLog()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Contains(t, string(resp), `"username":"Me"`)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "Step 3: should call accounts (cache cleared)")

		interceptor.ClearModifier()

		// Verify: extensions-based delete occurred during this QUERY (not mutation).
		queryLog := defaultCache.GetLog()
		hasDelete := false
		for _, entry := range queryLog {
			if entry.Operation == "delete" {
				hasDelete = true
				assert.Equal(t, `{"__typename":"User","key":{"id":"1234"}}`, entry.Keys[0])
			}
		}
		assert.True(t, hasDelete, "query response should trigger cache delete via extensions")
	})

	t.Run("with subgraph header prefix", func(t *testing.T) {
		accounts.ResetUsers()
		t.Cleanup(accounts.ResetUsers)

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second, IncludeSubgraphHeaderPrefix: true},
				},
			},
		}

		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		headerBuilder := &mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{"accounts": 55555},
		}

		interceptor := newSubgraphResponseInterceptor(accounts.GraphQLEndpointHandler(accounts.TestOptions))

		setup := newFederationSetupWithInterceptor(interceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
			withSubgraphHeadersBuilder(headerBuilder),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx := t.Context()

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Populate cache (keys will include header prefix "55555:").
		tracker.Reset()
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 1, tracker.GetCount(accountsHost))

		// Verify cache hit.
		tracker.Reset()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "should hit cache")

		// Mutation with extensions invalidation.
		interceptor.SetModifier(func(body []byte) []byte {
			assert.Equal(t,
				`{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`,
				string(body),
			)
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`,
			)
		})

		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)

		interceptor.ClearModifier()

		// Verify the delete key includes the header prefix "55555:".
		mutationLog := defaultCache.GetLog()
		hasDelete := false
		for _, entry := range mutationLog {
			if entry.Operation == "delete" {
				hasDelete = true
				assert.Len(t, entry.Keys, 1)
				assert.Equal(t, `55555:{"__typename":"User","key":{"id":"1234"}}`, entry.Keys[0],
					"delete key should include header prefix")
			}
		}
		assert.True(t, hasDelete, "should have delete operation")

		// Cache should be invalidated.
		tracker.Reset()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "should re-fetch after invalidation")
	})

	t.Run("with L2CacheKeyInterceptor", func(t *testing.T) {
		accounts.ResetUsers()
		t.Cleanup(accounts.ResetUsers)

		subgraphCachingConfigs := engine.SubgraphCachingConfigs{
			{
				SubgraphName: "accounts",
				EntityCaching: plan.EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				},
			},
		}

		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{"default": defaultCache}
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{Transport: tracker}

		interceptor := newSubgraphResponseInterceptor(accounts.GraphQLEndpointHandler(accounts.TestOptions))

		setup := newFederationSetupWithInterceptor(interceptor, addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(caches),
			withHTTPClient(trackingClient),
			withCachingOptionsFunc(resolve.CachingOptions{
				EnableL2Cache: true,
				L2CacheKeyInterceptor: func(_ context.Context, key string, _ resolve.L2CacheKeyInterceptorInfo) string {
					return "tenant-X:" + key
				},
			}),
			withSubgraphEntityCachingConfigs(subgraphCachingConfigs),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx := t.Context()

		accountsHost := mustParseHost(setup.AccountsUpstreamServer.URL)

		// Populate cache (keys will include interceptor prefix "tenant-X:").
		tracker.Reset()
		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 1, tracker.GetCount(accountsHost))

		// Verify cache hit.
		tracker.Reset()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "should hit cache")

		// Mutation with extensions invalidation.
		interceptor.SetModifier(func(body []byte) []byte {
			assert.Equal(t,
				`{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`,
				string(body),
			)
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`,
			)
		})

		defaultCache.ClearLog()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, mutationQuery, nil, t)

		interceptor.ClearModifier()

		// Verify the delete key includes the interceptor prefix "tenant-X:".
		mutationLog := defaultCache.GetLog()
		hasDelete := false
		for _, entry := range mutationLog {
			if entry.Operation == "delete" {
				hasDelete = true
				assert.Len(t, entry.Keys, 1)
				assert.Equal(t, `tenant-X:{"__typename":"User","key":{"id":"1234"}}`, entry.Keys[0],
					"delete key should include interceptor prefix")
			}
		}
		assert.True(t, hasDelete, "should have delete operation")

		// Cache should be invalidated.
		tracker.Reset()
		gqlClient.QueryString(ctx, setup.GatewayServer.URL, entityQuery, nil, t)
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "should re-fetch after invalidation")
	})
}
