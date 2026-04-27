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

// TestRemapVariablesEntityCacheKey is a smoke test verifying that the
// RemapVariables plumbing works end-to-end through the execution engine.
//
// In production, the router's VariablesMapper renames AST variable references
// ($id → $a) while keeping the variables JSON unchanged. This creates a split
// that renderDerivedEntityKey bridges via forward lookup on RemapVariables.
// However, the execution engine test infrastructure cannot replicate this split
// because the engine validates query+variables together — using $a in the query
// with {"id": "1234"} in the variables fails validation.
//
// So this test sends the original query (with $id) plus RemapVariables: {"a": "id"}.
// The planner produces ArgumentPath ["id"] (matching the variable name directly),
// so the remap forward lookup is a no-op. The test verifies the entity cache key
// derivation and L2 miss/hit cycle work correctly with RemapVariables configured.
//
// The RemapVariables forward-lookup branch in renderDerivedEntityKey is covered
// by unit tests in cache_key_test.go, which can directly construct the
// production-realistic ArgumentPath/Variables/RemapVariables combination.
func TestRemapVariablesEntityCacheKey(t *testing.T) {
	t.Parallel()

	// Subtest name: the engine-level scenario this test can actually express is
	// "RemapVariables plumbing produces a valid entity cache key and L2 miss→hit
	// cycle." The RemapVariables forward-lookup branch itself is covered directly
	// in v2/pkg/engine/resolve/cache_key_test.go, which can construct the
	// ArgumentPath/Variables/RemapVariables split without engine validation getting
	// in the way.
	t.Run("entity cache key derivation works end-to-end with RemapVariables configured", func(t *testing.T) {
		t.Parallel()
		defaultCache := NewFakeLoaderCache()
		tracker := newSubgraphCallTracker(http.DefaultTransport)

		setup := federationtesting.NewFederationSetup(addCachingGateway(
			withCachingEnableART(false),
			withCachingLoaderCache(map[string]resolve.LoaderCache{"default": defaultCache}),
			withHTTPClient(&http.Client{Transport: tracker}),
			withCachingOptionsFunc(resolve.CachingOptions{EnableL2Cache: true}),
			withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "accounts",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "user",
							CacheName: "default",
							TTL:       30 * time.Second,
							EntityKeyMappings: []plan.EntityKeyMapping{
								{
									EntityTypeName: "User",
									FieldMappings: []plan.FieldMapping{
										{EntityKeyField: "id", ArgumentPath: []string{"id"}},
									},
								},
							},
						},
					},
				},
			}),
			// Simulate VariablesMapper: $id was renamed to $a in the AST.
			// RemapVariables maps newName → oldName so the resolver can find
			// the original variable value in the un-renamed variables JSON.
			withRemapVariables(map[string]string{"a": "id"}),
		))
		t.Cleanup(setup.Close)

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host

		// Query 1: cache miss.
		// Variables use the original name "id" (as in production — the JSON is not renamed).
		// The query also uses $id because the execution engine validates variable declarations
		// against the variables JSON. In production, the AST would have been rewritten to $a
		// before reaching the planner, but validation happened on the original query.
		// The RemapVariables map still exercises renderDerivedEntityKey's forward lookup:
		// ArgumentPath ["a"] (from resolveArgumentPath resolving through ContextVariable)
		// is remapped via RemapVariables["a"] → "id" before looking up Variables["id"].
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL,
			`query UserById($id: ID!) { user(id: $id) { id username } }`,
			queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: false}}},
			{Operation: "set", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, TTL: 30 * time.Second}}},
		}), sortCacheLogEntries(logAfterFirst))
		assert.Equal(t, 1, tracker.GetCount(accountsHost), "first query should fetch from accounts")

		// Query 2: cache hit — same entity key, served from L2.
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL,
			`query UserById($id: ID!) { user(id: $id) { id username } }`,
			queryVariables{"id": "1234"}, t)
		assert.Equal(t, `{"data":{"user":{"id":"1234","username":"Me"}}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, sortCacheLogEntries([]CacheLogEntry{
			{Operation: "get", Items: []CacheLogItem{{Key: `{"__typename":"User","key":{"id":"1234"}}`, Hit: true}}},
		}), sortCacheLogEntries(logAfterSecond))
		assert.Equal(t, 0, tracker.GetCount(accountsHost), "second query should skip accounts (cache hit)")
	})
}
