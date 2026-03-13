package engine_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestFederationCaching_ExtensionsInvalidation(t *testing.T) {
	t.Run("mutation with extensions invalidation clears L2 cache", func(t *testing.T) {
		// Verify that a mutation response with cacheInvalidation extensions
		// deletes the corresponding L2 cache entry, forcing a re-fetch.
		env := newExtInvalidationEnv(t)

		// Step 1: Query populates L2 cache.
		resp := env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 1, env.accountsCalls(), "first request fetches from accounts")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{extInvUserKey}, Hits: []bool{false}}, // L2 empty on first request
			{Operation: "set", Keys: []string{extInvUserKey}},                      // populate L2 after fetch
		}), env.cacheLog())

		// Step 2: Same query — L2 hit, no subgraph call.
		resp = env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{extInvUserKey}, Hits: []bool{true}}, // L2 hit from Step 1
		}), env.cacheLog())

		// Step 3: Mutation with cacheInvalidation extensions deletes User:1234.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		mutResp := env.mutate()
		assert.Equal(t, mutationResponse, mutResp)
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{extInvUserKey}}, // extensions-based invalidation
		}), env.cacheLog())

		// Step 4: Re-query — L2 miss after invalidation, fetches updated username.
		resp = env.queryEntity()
		assert.Equal(t, entityResponseUpdated, resp)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{extInvUserKey}, Hits: []bool{false}}, // L2 miss because Step 3 deleted it
			{Operation: "set", Keys: []string{extInvUserKey}},                      // re-populate L2 after re-fetch
		}), env.cacheLog())
	})

	t.Run("invalidation of entity not in cache is a no-op", func(t *testing.T) {
		// Invalidating a different entity (User:9999) should not affect
		// the cached entity (User:1234).
		env := newExtInvalidationEnv(t)

		// Populate cache with User:1234.
		env.queryEntity()

		// Mutation invalidates User:9999 (never cached).
		user9999Key := `{"__typename":"User","key":{"id":"9999"}}`
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"9999"}}]}`)
		})
		mutResp := env.mutate()
		assert.Equal(t, mutationResponse, mutResp)
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{user9999Key}}, // delete called even though entry doesn't exist
		}), env.cacheLog())

		// User:1234 should still be cached (unaffected by User:9999 invalidation).
		resp := env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "User:1234 still cached")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{extInvUserKey}, Hits: []bool{true}}, // User:1234 still in L2
		}), env.cacheLog())
	})

	t.Run("multiple entities invalidated in single response", func(t *testing.T) {
		// A single mutation response can invalidate multiple entities at once.
		env := newExtInvalidationEnv(t)

		// Populate cache with User:1234.
		env.queryEntity()

		// Mutation invalidates both User:1234 and User:2345 in one response.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}},{"typename":"User","key":{"id":"2345"}}]}`)
		})
		env.mutate()
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{
				`{"__typename":"User","key":{"id":"1234"}}`,
				`{"__typename":"User","key":{"id":"2345"}}`,
			}}, // both entities deleted in single batch
		}), env.cacheLog())

		// User:1234 must be re-fetched after invalidation.
		env.queryEntity()
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{extInvUserKey}, Hits: []bool{false}}, // L2 miss because mutation deleted it
			{Operation: "set", Keys: []string{extInvUserKey}},                      // re-populate L2
		}), env.cacheLog())
	})

	t.Run("mutation without extensions does not delete", func(t *testing.T) {
		// A mutation without cacheInvalidation extensions should not
		// trigger any cache deletes — cached data survives.
		env := newExtInvalidationEnv(t)

		// Populate cache.
		env.queryEntity()

		// Verify cache hit.
		resp := env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")

		// Mutation WITHOUT extensions — no cache operations.
		env.mutate()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{}), env.cacheLog(), "no cache operations for mutation without extensions")

		// Cache should still be valid.
		resp = env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "cache still valid")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{extInvUserKey}, Hits: []bool{true}}, // L2 still valid
		}), env.cacheLog())
	})

	t.Run("coexistence with detectMutationEntityImpact", func(t *testing.T) {
		// When BOTH config-based MutationCacheInvalidation AND extensions-based
		// invalidation target the same key, the delete should be deduplicated
		// to a single cache.Delete() call.
		env := newExtInvalidationEnv(t, withMutationCacheInvalidation("updateUsername"))

		// Populate cache.
		env.queryEntity()
		assert.Equal(t, 1, env.accountsCalls())

		// Verify cache hit.
		env.queryEntity()
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")

		// Mutation triggers BOTH mechanisms on User:1234.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		env.mutate()
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{extInvUserKey}}, // deduplicated: detectMutationEntityImpact fires, extensions-based skipped
		}), env.cacheLog(), "single delete despite both mechanisms targeting same key")

		// Cache invalidated — query should re-fetch.
		env.queryEntity()
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after combined invalidation")
	})

	t.Run("query response triggers invalidation", func(t *testing.T) {
		// Cache invalidation via extensions is NOT restricted to mutations.
		// A query (e.g. _entities) response can also carry invalidation extensions.
		env := newExtInvalidationEnv(t)

		// Step 1: Populate L2 cache.
		resp := env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 1, env.accountsCalls())

		// Step 2: Verify cache hit.
		env.queryEntity()
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")

		// Step 3: Manually delete cache entry, then inject invalidation into the
		// _entities query response. This proves invalidation works on queries too.
		env.deleteFromCache(extInvUserKey)
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, entitiesSubgraphRespMe, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})

		resp = env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after manual delete")
		env.clearModifier()

		// Extensions-based delete is skipped because updateL2Cache will set the same
		// key with fresh data — only get(miss) + set remain.
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{extInvUserKey}, Hits: []bool{false}}, // L2 miss because we manually deleted it
			{Operation: "set", Keys: []string{extInvUserKey}},                      // re-populate L2 (delete skipped: same key about to be set)
		}), env.cacheLog())
	})

	t.Run("with subgraph header prefix", func(t *testing.T) {
		// When IncludeSubgraphHeaderPrefix is enabled, cache keys include a
		// hash prefix (e.g. "55555:"). Invalidation must use the same prefix.
		env := newExtInvalidationEnv(t, withHeaderPrefix(55555))
		prefixedKey := `55555:` + extInvUserKey

		// Populate cache (keys include header prefix).
		env.queryEntity()
		assert.Equal(t, 1, env.accountsCalls())
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{prefixedKey}, Hits: []bool{false}}, // L2 miss, prefixed key
			{Operation: "set", Keys: []string{prefixedKey}},                      // populate L2 with prefixed key
		}), env.cacheLog())

		// Verify cache hit.
		env.queryEntity()
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
		env.mutate()
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{prefixedKey}}, // delete key includes header prefix
		}), env.cacheLog())

		// Cache invalidated — re-fetch.
		env.queryEntity()
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{prefixedKey}, Hits: []bool{false}}, // L2 miss after delete
			{Operation: "set", Keys: []string{prefixedKey}},                      // re-populate L2
		}), env.cacheLog())
	})

	t.Run("with L2CacheKeyInterceptor", func(t *testing.T) {
		// When an L2CacheKeyInterceptor is configured, cache keys are transformed
		// (e.g. "tenant-X:" prefix). Invalidation must use the same transformation.
		env := newExtInvalidationEnv(t, withExtInvL2KeyInterceptor(
			func(_ context.Context, key string, _ resolve.L2CacheKeyInterceptorInfo) string {
				return "tenant-X:" + key
			},
		))
		interceptedKey := `tenant-X:` + extInvUserKey

		// Populate cache (keys include interceptor prefix).
		env.queryEntity()
		assert.Equal(t, 1, env.accountsCalls())
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{interceptedKey}, Hits: []bool{false}}, // L2 miss, intercepted key
			{Operation: "set", Keys: []string{interceptedKey}},                      // populate L2 with intercepted key
		}), env.cacheLog())

		// Verify cache hit.
		env.queryEntity()
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
		env.mutate()
		env.clearModifier()
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{interceptedKey}}, // delete key includes interceptor prefix
		}), env.cacheLog())

		// Cache invalidated — re-fetch.
		env.queryEntity()
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
		// When a mutation returns BOTH errors AND extensions.cacheInvalidation,
		// the cache invalidation should still run despite the errors.
		env := newExtInvalidationEnv(t)

		// Populate L2 cache.
		resp := env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 1, env.accountsCalls())

		// Verify cache hit.
		resp = env.queryEntity()
		assert.Equal(t, entityResponseMe, resp)
		assert.Equal(t, 0, env.accountsCalls(), "L2 cache hit")

		// Mutation returns errors alongside cacheInvalidation extensions.
		env.onAccountsResponse(func(body []byte) []byte {
			return injectErrorsAndCacheInvalidation(t, body,
				`[{"message":"partial error"}]`,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		env.mutate()
		env.clearModifier()

		// Cache should be invalidated despite errors in response.
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "delete", Keys: []string{extInvUserKey}}, // invalidation runs despite errors
		}), env.cacheLog())

		// Re-query — L2 miss after invalidation, re-fetches updated data.
		resp = env.queryEntity()
		assert.Equal(t, entityResponseUpdated, resp)
		assert.Equal(t, 1, env.accountsCalls(), "re-fetched after invalidation")
	})

	// -------------------------------------------------------------------------
	// Analytics: MutationEvent correctness with cache invalidation.
	// -------------------------------------------------------------------------

	t.Run("coexistence with analytics reports correct staleness", func(t *testing.T) {
		// When both config-based and extensions-based invalidation target the same
		// entity, analytics should correctly report the entity was cached and stale.
		env := newExtInvalidationEnv(t,
			withMutationCacheInvalidation("updateUsername"),
			withExtInvAnalytics(),
		)

		// Populate L2 cache with User:1234 (username="Me").
		env.queryEntity()
		assert.Equal(t, 1, env.accountsCalls())

		// Mutation with BOTH mechanisms targeting User:1234.
		env.onAccountsResponse(func(body []byte) []byte {
			assert.Equal(t, mutationResponse, string(body))
			return injectCacheInvalidation(t, body,
				`{"keys":[{"typename":"User","key":{"id":"1234"}}]}`)
		})
		mutResp, headers := env.mutateWithHeaders()
		assert.Equal(t, mutationResponse, mutResp)
		env.clearModifier()

		// Analytics should report correct staleness detection.
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
					EntityCacheKey:    extInvUserKey,
					HadCachedValue:    true, // L2 had cached value from prior query
					IsStale:           true, // Cached "Me" differs from fresh "UpdatedMe"
					CachedHash:        event.CachedHash,
					FreshHash:         event.FreshHash,
					CachedBytes:       event.CachedBytes,
					FreshBytes:        event.FreshBytes,
				},
			},
		}), snap)

		// Verify dedup still works — single delete despite both mechanisms.
		assert.Equal(t, sortCacheLogKeys([]CacheLogEntry{
			{Operation: "get", Keys: []string{extInvUserKey}, Hits: []bool{true}}, // analytics reads cached value before delete
			{Operation: "delete", Keys: []string{extInvUserKey}},                  // config-based delete (extensions-based skipped via dedup)
		}), env.cacheLog(), "analytics read before delete, single delete despite both mechanisms")
	})

	t.Run("analytics without prior cache reports no-cache event", func(t *testing.T) {
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
		mutResp, headers := env.mutateWithHeaders()
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
					EntityCacheKey:    extInvUserKey,
					HadCachedValue:    false, // No prior query, L2 cache was empty
					IsStale:           false, // Cannot be stale without a cached value to compare
					FreshHash:         event.FreshHash,
					FreshBytes:        event.FreshBytes,
				},
			},
		}), snap)
	})
}
