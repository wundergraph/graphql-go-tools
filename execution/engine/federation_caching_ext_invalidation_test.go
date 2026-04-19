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

// TestFederationCaching_ExtensionsInvalidation verifies end-to-end extensions-based cache
// invalidation: a mutation response with cacheInvalidation extensions deletes the L2 entry.
func TestFederationCaching_ExtensionsInvalidation(t *testing.T) {
	t.Parallel()
	t.Run("mutation with extensions invalidation clears L2 cache", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		entityResponseMe := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`
		entityResponseUpdated := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"UpdatedMe"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"UpdatedMe"}}]}]}}`
		mutationResponse := `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`

		// Verify that a mutation response with cacheInvalidation extensions
		// deletes the corresponding L2 cache entry, forcing a re-fetch.
		env := newExtInvalidationEnv(t)

		// Step 1: Query populates L2 cache.
		resp := env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 1, env.accountsCalls(), "first request fetches from accounts")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{userKey}, Hits: []bool{false}}, // L2 empty on first request
			{Operation: "set", Keys: []string{userKey}},                      // populate L2 after fetch
		}), env.cacheLog())

		// Step 2: Same query — L2 hit, no subgraph call.
		resp = env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{userKey}, Hits: []bool{true}}, // L2 hit from Step 1
		}), env.cacheLog())

		// Step 3: Mutation with cacheInvalidation extensions deletes User:1234.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		mutResp := env.mutate(mutationQuery)
		assert.Equal(t, mutationResponse, mutResp)
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{userKey}}, // extensions-based invalidation
		}), env.cacheLog())

		// Step 4: Re-query — L2 miss after invalidation, fetches updated username.
		resp = env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseUpdated, resp)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{userKey}, Hits: []bool{false}}, // L2 miss because Step 3 deleted it
			{Operation: "set", Keys: []string{userKey}},                      // re-populate L2 after re-fetch
		}), env.cacheLog())
	})

	t.Run("invalidation of entity not in cache is a no-op", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		entityResponseMe := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`
		mutationResponse := `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`

		// Invalidating a different entity (User:9999) should not affect
		// the cached entity (User:1234).
		env := newExtInvalidationEnv(t)

		// Populate cache with User:1234.
		env.queryEntity(entityQuery)

		// Mutation invalidates User:9999 (never cached).
		user9999Key := `{"__typename":"User","key":{"id":"9999"}}`
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"9999"}}]}`)
		})
		mutResp := env.mutate(mutationQuery)
		assert.Equal(t, mutationResponse, mutResp)
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{user9999Key}}, // delete called even though entry doesn't exist
		}), env.cacheLog())

		// User:1234 should still be cached (unaffected by User:9999 invalidation).
		resp := env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "User:1234 still cached")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{userKey}, Hits: []bool{true}}, // User:1234 still in L2
		}), env.cacheLog())
	})

	t.Run("multiple entities invalidated in single response", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		mutationResponse := `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`

		// A single mutation response can invalidate multiple entities at once.
		env := newExtInvalidationEnv(t)

		// Populate cache with User:1234.
		env.queryEntity(entityQuery)

		// Mutation invalidates both User:1234 and User:2345 in one response.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}},{"typename":"User","key":{"id":"2345"}}]}`)
		})
		env.mutate(mutationQuery)
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{
				`{"__typename":"User","key":{"id":"1234"}}`,
				`{"__typename":"User","key":{"id":"2345"}}`,
			}}, // both entities deleted in single batch
		}), env.cacheLog())

		// User:1234 must be re-fetched after invalidation.
		env.queryEntity(entityQuery)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{userKey}, Hits: []bool{false}}, // L2 miss because mutation deleted it
			{Operation: "set", Keys: []string{userKey}},                      // re-populate L2
		}), env.cacheLog())
	})

	t.Run("mutation without extensions does not delete", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		entityResponseMe := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`

		// A mutation without cacheInvalidation extensions should not
		// trigger any cache deletes — cached data survives.
		env := newExtInvalidationEnv(t)

		// Populate cache.
		env.queryEntity(entityQuery)

		// Verify cache hit.
		resp := env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")

		// Mutation WITHOUT extensions — no cache operations.
		env.mutate(mutationQuery)
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{}), env.cacheLog(), "no cache operations for mutation without extensions")

		// Cache should still be valid.
		resp = env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "cache still valid")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{userKey}, Hits: []bool{true}}, // L2 still valid
		}), env.cacheLog())
	})

	t.Run("coexistence with detectMutationEntityImpact", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		mutationResponse := `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`

		// When BOTH config-based MutationCacheInvalidation AND extensions-based
		// invalidation target the same key, the delete should be deduplicated
		// to a single cache.Delete() call.
		env := newExtInvalidationEnv(t, withMutationCacheInvalidation("updateUsername"))

		// Populate cache.
		env.queryEntity(entityQuery)
		assert.Equal(t, 1, env.accountsCalls())

		// Verify cache hit.
		env.queryEntity(entityQuery)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")

		// Mutation triggers BOTH mechanisms on User:1234.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		env.mutate(mutationQuery)
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{userKey}}, // deduplicated: detectMutationEntityImpact fires, extensions-based skipped
		}), env.cacheLog(), "single delete despite both mechanisms targeting same key")

		// Cache invalidated — query should re-fetch.
		env.queryEntity(entityQuery)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after combined invalidation")
	})

	t.Run("query response triggers invalidation", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		entityResponseMe := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`
		entitiesSubgraphRespMe := `{"data":{"_entities":[{"__typename":"User","username":"Me"}]}}`

		// Cache invalidation via extensions is NOT restricted to mutations.
		// A query (e.g. _entities) response can also carry invalidation extensions.
		env := newExtInvalidationEnv(t)

		// Step 1: Populate L2 cache.
		resp := env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 1, env.accountsCalls())

		// Step 2: Verify cache hit.
		env.queryEntity(entityQuery)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")

		// Step 3: Manually delete cache entry, then inject invalidation into the
		// _entities query response. This proves invalidation works on queries too.
		env.deleteFromCache(userKey)
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, entitiesSubgraphRespMe, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})

		resp = env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after manual delete")
		env.clearModifier()

		// Extensions-based delete is skipped because updateL2Cache will set the same
		// key with fresh data — only get(miss) + set remain.
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{userKey}, Hits: []bool{false}}, // L2 miss because we manually deleted it
			{Operation: "set", Keys: []string{userKey}},                      // re-populate L2 (delete skipped: same key about to be set)
		}), env.cacheLog())
	})

	t.Run("with subgraph header prefix", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		mutationResponse := `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`

		// When IncludeSubgraphHeaderPrefix is enabled, cache keys include a
		// hash prefix (e.g. "55555:"). Invalidation must use the same prefix.
		env := newExtInvalidationEnv(t, withHeaderPrefix(55555))
		prefixedKey := `55555:` + userKey

		// Populate cache (keys include header prefix).
		env.queryEntity(entityQuery)
		assert.Equal(t, 1, env.accountsCalls())
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{prefixedKey}, Hits: []bool{false}}, // L2 miss, prefixed key
			{Operation: "set", Keys: []string{prefixedKey}},                      // populate L2 with prefixed key
		}), env.cacheLog())

		// Verify cache hit.
		env.queryEntity(entityQuery)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{prefixedKey}, Hits: []bool{true}}, // L2 hit with prefixed key
		}), env.cacheLog())

		// Mutation with extensions invalidation.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		env.mutate(mutationQuery)
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{prefixedKey}}, // delete key includes header prefix
		}), env.cacheLog())

		// Cache invalidated — re-fetch.
		env.queryEntity(entityQuery)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{prefixedKey}, Hits: []bool{false}}, // L2 miss after delete
			{Operation: "set", Keys: []string{prefixedKey}},                      // re-populate L2
		}), env.cacheLog())
	})

	t.Run("with L2CacheKeyInterceptor", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		mutationResponse := `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`

		// When an L2CacheKeyInterceptor is configured, cache keys are transformed
		// (e.g. "tenant-X:" prefix). Invalidation must use the same transformation.
		env := newExtInvalidationEnv(t, withExtInvL2KeyInterceptor(
			func(_ context.Context, key string, _ resolve.L2CacheKeyInterceptorInfo) string {
				return "tenant-X:" + key
			},
		))
		interceptedKey := `tenant-X:` + userKey

		// Populate cache (keys include interceptor prefix).
		env.queryEntity(entityQuery)
		assert.Equal(t, 1, env.accountsCalls())
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{interceptedKey}, Hits: []bool{false}}, // L2 miss, intercepted key
			{Operation: "set", Keys: []string{interceptedKey}},                      // populate L2 with intercepted key
		}), env.cacheLog())

		// Verify cache hit.
		env.queryEntity(entityQuery)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{interceptedKey}, Hits: []bool{true}}, // L2 hit with intercepted key
		}), env.cacheLog())

		// Mutation with extensions invalidation.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		env.mutate(mutationQuery)
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{interceptedKey}}, // delete key includes interceptor prefix
		}), env.cacheLog())

		// Cache invalidated — re-fetch.
		env.queryEntity(entityQuery)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{interceptedKey}, Hits: []bool{false}}, // L2 miss after delete
			{Operation: "set", Keys: []string{interceptedKey}},                      // re-populate L2
		}), env.cacheLog())
	})

	// -------------------------------------------------------------------------
	// Error handling: cache invalidation must run even when errors are present.
	// -------------------------------------------------------------------------

	t.Run("error response with invalidation extensions still invalidates cache", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		entityResponseMe := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"Me"}}]}]}}`
		entityResponseUpdated := `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","authorWithoutProvides":{"username":"UpdatedMe"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","authorWithoutProvides":{"username":"UpdatedMe"}}]}]}}`

		// When a mutation returns BOTH errors AND extensions.cacheInvalidation,
		// the cache invalidation should still run despite the errors.
		env := newExtInvalidationEnv(t)

		// Populate L2 cache.
		resp := env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 1, env.accountsCalls())

		// Verify cache hit.
		resp = env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")

		// Mutation returns errors alongside cacheInvalidation extensions.
		env.onAccountsResponse(func(body []byte) []byte {
			return injectErrorsAndCacheInvalidation(t, body,
				`[{"message":"partial error"}]`,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		env.mutate(mutationQuery)
		env.clearModifier()

		// Cache should be invalidated despite errors in response.
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{userKey}}, // invalidation runs despite errors
		}), env.cacheLog())

		// Re-query — L2 miss after invalidation, re-fetches updated data.
		resp = env.queryEntity(entityQuery)
		assert.Equal(t, entityResponseUpdated, resp)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
	})

	// -------------------------------------------------------------------------
	// Analytics: MutationEvent correctness with cache invalidation.
	// -------------------------------------------------------------------------

	t.Run("coexistence with analytics reports correct staleness", func(t *testing.T) {
		t.Parallel()
		entityQuery := `query { topProducts { name reviews { body authorWithoutProvides { username } } } }`
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		mutationResponse := `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`

		// When both config-based and extensions-based invalidation target the same
		// entity, analytics should correctly report the entity was cached and stale.
		env := newExtInvalidationEnv(t,
			withMutationCacheInvalidation("updateUsername"),
			withExtInvAnalytics(),
		)

		// Populate L2 cache with User:1234 (username="Me").
		env.queryEntity(entityQuery)
		assert.Equal(t, 1, env.accountsCalls())

		// Mutation with BOTH mechanisms targeting User:1234.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		mutResp, headers := env.mutateWithHeaders(mutationQuery)
		assert.Equal(t, mutationResponse, mutResp)
		env.clearModifier()

		// Analytics should still identify the mutation entity, but must not read L2.
		snap := normalizeSnapshot(parseCacheAnalytics(t, headers))
		require.Equal(t, 1, len(snap.MutationEvents), "should have exactly 1 mutation impact event")

		event := snap.MutationEvents[0]
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			FieldHashes: []resolve.EntityFieldHash{
				// Hash of "UpdatedMe" (post-mutation username)
				{EntityType: "User", FieldName: "username", FieldHash: 16932466035575627600, KeyRaw: `{"id":"1234"}`},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 1, UniqueKeys: 1}, // Mutation returned 1 User entity
			},
			MutationEvents: []resolve.MutationEvent{
				{
					MutationRootField: "updateUsername",
					EntityType:        "User",
					EntityCacheKey:    userKey,
					HadCachedValue:    false, // Mutation analytics must not read L2
					IsStale:           false, // No cache read means no stale comparison
					CachedHash:        event.CachedHash,
					FreshHash:         event.FreshHash,
					CachedBytes:       event.CachedBytes,
					FreshBytes:        event.FreshBytes,
				},
			},
		}), snap)

		// Verify dedup still works — single delete despite both mechanisms.
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{userKey}}, // config-based delete (extensions-based skipped via dedup)
		}), env.cacheLog(), "single delete despite both mechanisms; analytics must not read cache")
	})

	t.Run("analytics without prior cache reports no-cache event", func(t *testing.T) {
		t.Parallel()
		mutationQuery := `mutation { updateUsername(id: "1234", newUsername: "UpdatedMe") { id username } }`
		userKey := `{"__typename":"User","key":{"id":"1234"}}`
		mutationResponse := `{"data":{"updateUsername":{"id":"1234","username":"UpdatedMe"}}}`

		// When mutation triggers invalidation but entity was never cached,
		// MutationEvent should show HadCachedValue=false, IsStale=false.
		env := newExtInvalidationEnv(t,
			withMutationCacheInvalidation("updateUsername"),
			withExtInvAnalytics(),
		)

		// No prior query — L2 cache is empty.
		// Mutation with extensions invalidation targeting User:1234.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		mutResp, headers := env.mutateWithHeaders(mutationQuery)
		assert.Equal(t, mutationResponse, mutResp)
		env.clearModifier()

		// Analytics should report no cached value.
		snap := normalizeSnapshot(parseCacheAnalytics(t, headers))
		require.Equal(t, 1, len(snap.MutationEvents), "should have exactly 1 mutation impact event")

		event := snap.MutationEvents[0]
		assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
			FieldHashes: []resolve.EntityFieldHash{
				// Hash of "UpdatedMe" (post-mutation username)
				{EntityType: "User", FieldName: "username", FieldHash: 16932466035575627600, KeyRaw: `{"id":"1234"}`},
			},
			EntityTypes: []resolve.EntityTypeInfo{
				{TypeName: "User", Count: 1, UniqueKeys: 1}, // Mutation returned 1 User entity
			},
			MutationEvents: []resolve.MutationEvent{
				{
					MutationRootField: "updateUsername",
					EntityType:        "User",
					EntityCacheKey:    userKey,
					HadCachedValue:    false, // No prior query, L2 cache was empty
					IsStale:           false, // Cannot be stale without a cached value to compare
					FreshHash:         event.FreshHash,
					FreshBytes:        event.FreshBytes,
				},
			},
		}), snap)
	})
}

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

// injectErrorsAndCacheInvalidation injects both errors and cacheInvalidation extensions
// into a subgraph response body. Used to test that invalidation runs even when errors are present.
func injectErrorsAndCacheInvalidation(t *testing.T, body []byte, errorsJSON string, cacheInvalidationJSON string) []byte {
	t.Helper()
	var resp map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &resp))
	resp["errors"] = json.RawMessage(errorsJSON)
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

// newFederationSetupWithReviewInterceptor creates a FederationSetup where the reviews
// subgraph is wrapped with the response interceptor.
func newFederationSetupWithReviewInterceptor(
	interceptor *subgraphResponseInterceptor,
	gatewayFn func(*federationtesting.FederationSetup) *httptest.Server,
) *federationtesting.FederationSetup {
	accountsServer := httptest.NewServer(accounts.GraphQLEndpointHandler(accounts.TestOptions))
	productsServer := httptest.NewServer(products.GraphQLEndpointHandler(products.TestOptions))
	reviewsServer := httptest.NewServer(interceptor)

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
	enableAnalytics                bool
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

// withExtInvAnalytics enables cache analytics collection on the gateway,
// allowing tests to assert on MutationEvent and other analytics data.
func withExtInvAnalytics() extInvalidationOption {
	return func(c *extInvalidationConfig) {
		c.enableAnalytics = true
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
	if cfg.enableAnalytics {
		cachingOpts.EnableCacheAnalytics = true
	}
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

// queryEntity sends an entity query, resets counters first.
func (e *extInvalidationEnv) queryEntity(query string) string {
	e.t.Helper()
	e.resetCounters()
	return string(e.gqlClient.QueryString(e.ctx, e.setup.GatewayServer.URL, query, nil, e.t))
}

// mutate sends a mutation, resets counters first.
func (e *extInvalidationEnv) mutate(mutation string) string {
	e.t.Helper()
	e.resetCounters()
	return string(e.gqlClient.QueryString(e.ctx, e.setup.GatewayServer.URL, mutation, nil, e.t))
}

// mutateWithHeaders sends a mutation and returns both the response body
// and HTTP headers (for cache analytics inspection). Resets counters first.
func (e *extInvalidationEnv) mutateWithHeaders(mutation string) (string, http.Header) {
	e.t.Helper()
	e.resetCounters()
	resp, headers := e.gqlClient.QueryStringWithHeaders(e.ctx, e.setup.GatewayServer.URL, mutation, nil, e.t)
	return string(resp), headers
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
