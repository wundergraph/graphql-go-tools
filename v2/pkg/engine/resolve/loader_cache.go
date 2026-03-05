package resolve

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type CacheEntry struct {
	Key          string
	Value        []byte
	RemainingTTL time.Duration // remaining TTL from cache (0 = unknown/not supported)
}

// EntityCacheInvalidationConfig holds the minimal cache settings needed to build
// invalidation keys for a specific entity type on a specific subgraph.
// Separate from plan.EntityCacheConfiguration to avoid a resolve → plan dependency;
// only CacheName and IncludeSubgraphHeaderPrefix are needed at invalidation time.
type EntityCacheInvalidationConfig struct {
	CacheName                   string
	IncludeSubgraphHeaderPrefix bool
}

type LoaderCache interface {
	Get(ctx context.Context, keys []string) ([]*CacheEntry, error)
	Set(ctx context.Context, entries []*CacheEntry, ttl time.Duration) error
	Delete(ctx context.Context, keys []string) error
}

// extractCacheKeysStrings extracts all unique cache key strings from CacheKeys
// If includePrefix is true and subgraphName is provided, keys are prefixed with the subgraph header hash.
func (l *Loader) extractCacheKeysStrings(a arena.Arena, cacheKeys []*CacheKey) []string {
	if len(cacheKeys) == 0 {
		return nil
	}
	out := arena.AllocateSlice[string](a, 0, len(cacheKeys))
	seen := make(map[string]struct{}, len(cacheKeys))
	for i := range cacheKeys {
		for j := range cacheKeys[i].Keys {
			keyStr := cacheKeys[i].Keys[j]
			if _, ok := seen[keyStr]; ok {
				continue
			}
			seen[keyStr] = struct{}{}
			keyLen := len(keyStr)
			key := arena.AllocateSlice[byte](a, 0, keyLen)
			key = arena.SliceAppend(a, key, unsafebytes.StringToBytes(keyStr)...)
			out = arena.SliceAppend(a, out, string(key))
		}
	}
	return out
}

// populateFromCache populates CacheKey.FromCache fields from cache entries
// If includePrefix is true and subgraphName is provided, keys are looked up with the subgraph header hash prefix.
func (l *Loader) populateFromCache(a arena.Arena, cacheKeys []*CacheKey, entries []*CacheEntry) (err error) {
	for i := range entries {
		if entries[i] == nil || entries[i].Value == nil {
			continue
		}
		for j := range cacheKeys {
			for k := range cacheKeys[j].Keys {
				if cacheKeys[j].Keys[k] == entries[i].Key {
					cacheKeys[j].FromCache, err = astjson.ParseBytesWithArena(a, entries[i].Value)
					if err != nil {
						return errors.WithStack(err)
					}
				}
			}
		}
	}
	return nil
}

// cacheKeysToEntries converts CacheKeys to CacheEntries for storage
// For each CacheKey, creates entries for all its KeyEntries with the same value
// If includePrefix is true and subgraphName is provided, keys are prefixed with the subgraph header hash.
func (l *Loader) cacheKeysToEntries(a arena.Arena, cacheKeys []*CacheKey) ([]*CacheEntry, error) {
	// Use heap slice for []*CacheEntry — arena memory is noscan, so GC cannot
	// trace *CacheEntry pointers stored there, risking premature collection.
	out := make([]*CacheEntry, 0, len(cacheKeys))
	buf := arena.AllocateSlice[byte](a, 64, 64)
	seen := make(map[string]struct{}, len(cacheKeys))
	for i := range cacheKeys {
		for j := range cacheKeys[i].Keys {
			if cacheKeys[i].Item == nil {
				continue
			}
			keyStr := cacheKeys[i].Keys[j]
			if _, ok := seen[keyStr]; ok {
				continue
			}
			seen[keyStr] = struct{}{}
			// When EntityMergePath is set, store entity-level data (extracted at merge path)
			// instead of response-level data, so entity fetches can read it directly.
			itemToStore := cacheKeys[i].Item
			if len(cacheKeys[i].EntityMergePath) > 0 {
				if entityData := cacheKeys[i].Item.Get(cacheKeys[i].EntityMergePath...); entityData != nil {
					itemToStore = entityData
				}
			}
			buf = itemToStore.MarshalTo(buf[:0])
			entry := &CacheEntry{
				Key:   cacheKeys[i].Keys[j],
				Value: arena.AllocateSlice[byte](a, len(buf), len(buf)),
			}
			copy(entry.Value, buf)
			out = append(out, entry)
		}
	}
	return out, nil
}

// prepareCacheKeys generates cache keys for L1 and/or L2 based on configuration.
// Called on main thread before any cache lookups.
// Sets res.l1CacheKeys for L1 lookup (no prefix) and res.l2CacheKeys for L2 lookup (with prefix).
// Returns isEntityFetch to indicate if this fetch supports L1 caching.
func (l *Loader) prepareCacheKeys(info *FetchInfo, cfg FetchCacheConfiguration, inputItems []*astjson.Value, res *result) (isEntityFetch bool, err error) {
	if cfg.CacheKeyTemplate == nil {
		return false, nil
	}

	// Skip all cache operations if both L1 and L2 are disabled
	if !l.ctx.ExecutionOptions.Caching.EnableL1Cache && !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return false, nil
	}

	res.cacheConfig = cfg
	if info != nil {
		res.providesData = info.ProvidesData
	}

	// Check if this is an entity fetch (L1 only applies to entity fetches)
	_, isEntity := cfg.CacheKeyTemplate.(*EntityQueryCacheKeyTemplate)

	// Set analytics entity type for cache event recording
	if l.ctx.cacheAnalyticsEnabled() && info != nil && len(info.RootFields) > 0 {
		res.analyticsEntityType = info.RootFields[0].TypeName
	}

	// Always generate cache keys (needed for merging cached data into response)
	// For entity fetches and root fetches: uses keys without prefix for L1
	res.l1CacheKeys, err = cfg.CacheKeyTemplate.RenderCacheKeys(l.jsonArena, l.ctx, inputItems, "")
	if err != nil {
		return false, err
	}

	// Generate L2 keys (with prefix for cache isolation)
	if l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		// Get cache first to ensure it exists
		if l.caches != nil {
			res.cache = l.caches[cfg.CacheName]
		}
		if res.cache != nil {
			// Calculate prefix for L2 (subgraph header isolation)
			var prefix string
			if cfg.IncludeSubgraphHeaderPrefix && l.ctx.SubgraphHeadersBuilder != nil {
				_, headersHash := l.ctx.SubgraphHeadersBuilder.HeadersForSubgraph(info.DataSourceName)
				var buf [20]byte
				b := strconv.AppendUint(buf[:0], headersHash, 10)
				prefix = string(b)
			}

			// Render L2 cache keys with prefix
			res.l2CacheKeys, err = cfg.CacheKeyTemplate.RenderCacheKeys(l.jsonArena, l.ctx, inputItems, prefix)
			if err != nil {
				return false, err
			}

			// Apply user-provided L2 cache key interceptor
			if interceptor := l.ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor; interceptor != nil {
				interceptorInfo := L2CacheKeyInterceptorInfo{
					SubgraphName: info.DataSourceName,
					CacheName:    cfg.CacheName,
				}
				for _, ck := range res.l2CacheKeys {
					for i, key := range ck.Keys {
						ck.Keys[i] = interceptor(l.ctx.ctx, key, interceptorInfo)
					}
				}
			}
		}
	}

	// When root field uses entity key mapping, set EntityMergePath so that
	// store/load can extract/wrap entity-level data at the merge path.
	if rootTemplate, ok := cfg.CacheKeyTemplate.(*RootQueryCacheKeyTemplate); ok && len(rootTemplate.EntityKeyMappings) > 0 {
		// Determine the path to extract entity data from the merged response.
		// If MergePath is set (e.g. ["user"]), use it directly.
		// Otherwise, the entity data is nested under the root field name in the response
		// (e.g. for field "user", response is {"user":{...}} and entity data is at ["user"]).
		entityPath := res.postProcessing.MergePath
		if len(entityPath) == 0 && len(rootTemplate.RootFields) == 1 {
			entityPath = []string{rootTemplate.RootFields[0].Coordinate.FieldName}
		}
		if len(entityPath) > 0 {
			for _, ck := range res.l1CacheKeys {
				ck.EntityMergePath = entityPath
			}
			for _, ck := range res.l2CacheKeys {
				ck.EntityMergePath = entityPath
			}
		}
	}

	return isEntity, nil
}

// tryCacheLoad orchestrates cache lookups for sequential execution paths.
// Uses the 3-function approach: prepareCacheKeys -> tryL1CacheLoad -> tryL2CacheLoad
// Returns skipFetch=true if cache provides complete data.
//
// IMPORTANT: This function is for SEQUENTIAL execution only (main thread).
// For PARALLEL execution, use prepareCacheKeys + tryL1CacheLoad on main thread,
// then tryL2CacheLoad in goroutines.
//
// Lookup Order (entity fetches): L1 -> L2 -> Subgraph Fetch
// Lookup Order (root fetches): L2 -> Subgraph Fetch (no L1)
func (l *Loader) tryCacheLoad(ctx context.Context, info *FetchInfo, cfg FetchCacheConfiguration, inputItems []*astjson.Value, res *result) (skipFetch bool, err error) {
	// Step 1: Prepare cache keys for L1 and L2
	isEntityFetch, err := l.prepareCacheKeys(info, cfg, inputItems, res)
	if err != nil {
		return false, err
	}

	// No cache keys generated - nothing to do
	if len(res.l1CacheKeys) == 0 && len(res.l2CacheKeys) == 0 {
		return false, nil
	}

	// Set partial loading flag BEFORE cache lookup so tracking arrays are populated
	// Shadow mode forces partial loading off - all items always fetched
	if cfg.ShadowMode {
		res.partialCacheEnabled = false
	} else {
		res.partialCacheEnabled = cfg.EnablePartialCacheLoad
	}

	// Step 2: L1 Check (per-request, in-memory) - entity fetches only
	// Safe to call: this is sequential execution on main thread
	// UseL1Cache flag is set by postprocessor to optimize L1 usage
	if isEntityFetch && l.ctx.ExecutionOptions.Caching.EnableL1Cache && cfg.UseL1Cache && len(res.l1CacheKeys) > 0 {
		allComplete := l.tryL1CacheLoad(info, res.l1CacheKeys, res)
		if allComplete {
			// All entities found in L1 with complete data - skip fetch
			res.cacheSkipFetch = true
			return true, nil
		}

		if res.partialCacheEnabled && len(res.cachedItemIndices) > 0 {
			// Partial hit with partial loading enabled
			// cachedItemIndices and fetchItemIndices already populated by tryL1CacheLoad
			// Keep FromCache values for cached items, proceed to fetch only missing items
			res.cacheMustBeUpdated = true
			return false, nil
		}

		// All-or-nothing mode OR no hits - clear FromCache and try L2
		for _, ck := range res.l1CacheKeys {
			ck.FromCache = nil
		}
		res.cachedItemIndices = nil
		res.fetchItemIndices = nil
	}

	// Step 3: L2 Check (external cache) - if L1 missed
	// Safe to call: this is sequential execution on main thread
	if l.ctx.ExecutionOptions.Caching.EnableL2Cache && len(res.l2CacheKeys) > 0 {
		skipFetch, err = l.tryL2CacheLoad(ctx, info, res)
		// Merge L2 analytics events and entity sources (sequential path, always on main thread)
		if l.ctx.cacheAnalyticsEnabled() {
			if len(res.l2AnalyticsEvents) > 0 {
				l.ctx.cacheAnalytics.MergeL2Events(res.l2AnalyticsEvents)
				res.l2AnalyticsEvents = nil
			}
			if len(res.l2EntitySources) > 0 {
				l.ctx.cacheAnalytics.MergeEntitySources(res.l2EntitySources)
				res.l2EntitySources = nil
			}
			if len(res.l2FetchTimings) > 0 {
				l.ctx.cacheAnalytics.MergeL2FetchTimings(res.l2FetchTimings)
				res.l2FetchTimings = nil
			}
			if len(res.l2ErrorEvents) > 0 {
				l.ctx.cacheAnalytics.MergeL2Errors(res.l2ErrorEvents)
				res.l2ErrorEvents = nil
			}
		}
		if err != nil || skipFetch {
			return skipFetch, err
		}

		if res.partialCacheEnabled && len(res.cachedItemIndices) > 0 {
			// Partial hit from L2 with partial loading enabled
			// Keep FromCache values, return false to proceed with fetch for missing items
			return false, nil
		}
	}

	// Both missed - fetch required
	res.cacheMustBeUpdated = true
	return false, nil
}

// tryL1CacheLoad attempts to load all items from the L1 (per-request) cache.
// MUST be called from main thread only (L1 stats are not atomic).
// Tracks per-entity hits/misses: HIT if entity found with complete data, MISS otherwise.
// Returns true only if ALL items are found in cache with complete data for the fetch.
// L1 uses cache keys WITHOUT subgraph header prefix (same request context).
// NOTE: Only called for entity fetches, not root fetches.
// When res.partialCacheEnabled is true, populates res.cachedItemIndices and res.fetchItemIndices
// to track which items were cached vs need fetching.
func (l *Loader) tryL1CacheLoad(info *FetchInfo, cacheKeys []*CacheKey, res *result) bool {
	if info == nil || info.OperationType != ast.OperationTypeQuery {
		return false
	}

	// Extract entity type and data source for analytics
	var entityType, dataSource string
	if l.ctx.cacheAnalyticsEnabled() {
		if len(info.RootFields) > 0 {
			entityType = info.RootFields[0].TypeName
		}
		dataSource = info.DataSourceName
	}

	allComplete := true
	for i, ck := range cacheKeys {
		var foundComplete bool
		for _, keyStr := range ck.Keys {
			if cached, ok := l.l1Cache.Load(keyStr); ok {
				cachedValue := cached.(*astjson.Value)
				// Check if cached entity has all required fields for this fetch
				if info.ProvidesData != nil && l.validateItemHasRequiredData(cachedValue, info.ProvidesData) {
					// Entity found with complete data - L1 HIT
					// Use shallow copy to prevent pointer aliasing with self-referential entities
					ck.FromCache = l.shallowCopyProvidedFields(cachedValue, info.ProvidesData)
					if l.ctx.cacheAnalyticsEnabled() {
						byteSize := len(cachedValue.MarshalTo(nil))
						l.ctx.cacheAnalytics.RecordL1KeyEvent(CacheKeyHit, entityType, keyStr, dataSource, byteSize)
						// Record entity source using plan-time KeyFields
						if len(res.cacheConfig.KeyFields) > 0 {
							keyJSON := buildEntityKeyJSON(cachedValue, res.cacheConfig.KeyFields)
							if len(keyJSON) > 0 {
								l.ctx.cacheAnalytics.RecordEntitySource(entityType, string(keyJSON), FieldSourceL1)
							}
						}
					}
					foundComplete = true
					break
				}
			}
		}

		if foundComplete {
			// Track cached item index when partial loading enabled
			if res.partialCacheEnabled {
				res.cachedItemIndices = append(res.cachedItemIndices, i)
			}
		} else {
			allComplete = false
			if l.ctx.cacheAnalyticsEnabled() && len(ck.Keys) > 0 {
				l.ctx.cacheAnalytics.RecordL1KeyEvent(CacheKeyMiss, entityType, ck.Keys[0], dataSource, 0)
			}
			// Track fetch item index when partial loading enabled
			if res.partialCacheEnabled {
				res.fetchItemIndices = append(res.fetchItemIndices, i)
			}
		}
	}
	return allComplete
}

// tryL2CacheLoad checks the external (L2) cache for entity data.
// Thread-safe: can be called from parallel goroutines (uses atomic L2 stats).
// Expects res.l2CacheKeys to be pre-populated by prepareCacheKeys().
// Uses subgraph header prefix for cache key isolation across different configurations.
func (l *Loader) tryL2CacheLoad(ctx context.Context, info *FetchInfo, res *result) (skipFetch bool, err error) {
	// Skip L2 cache reads for mutations - always fetch fresh data from subgraph.
	// We check l.info (root operation type), not info (per-fetch type), because
	// nested entity fetches within mutations have OperationType=Query.
	// NOTE: L2 cache WRITES are NOT skipped for mutations (see updateL2Cache).
	// This is intentional: mutations produce fresh data that should populate L2
	// so subsequent queries benefit from the updated cache.
	// Subscriptions are allowed to read from L2 cache because their child entity
	// fetches are read operations, just like queries.
	if l.info != nil && l.info.OperationType == ast.OperationTypeMutation {
		res.cacheMustBeUpdated = true
		return false, nil
	}

	// L2 keys should be pre-populated by prepareCacheKeys
	if len(res.l2CacheKeys) == 0 || res.cache == nil {
		res.cacheMustBeUpdated = true
		return false, nil
	}

	cacheKeyStrings := l.extractCacheKeysStrings(l.jsonArena, res.l2CacheKeys)
	if len(cacheKeyStrings) == 0 {
		res.cacheMustBeUpdated = true
		return false, nil
	}

	// Extract entity type and data source for analytics (read-only, goroutine-safe)
	analyticsEnabled := l.ctx.cacheAnalyticsEnabled()
	var entityType, dataSource string
	if analyticsEnabled && info != nil {
		if len(info.RootFields) > 0 {
			entityType = info.RootFields[0].TypeName
		}
		dataSource = info.DataSourceName
	}

	// Enrich context with fetch identity when debug mode is enabled
	if l.ctx.Debug {
		ctx = WithCacheFetchInfo(ctx, info, res.cacheConfig)
	}

	// Get cache entries from L2
	var l2GetStart time.Time
	if analyticsEnabled {
		l2GetStart = time.Now()
	}
	cacheEntries, err := res.cache.Get(ctx, cacheKeyStrings)
	if analyticsEnabled {
		res.l2FetchTimings = append(res.l2FetchTimings, FetchTimingEvent{
			DataSource:    dataSource,
			EntityType:    entityType,
			DurationMs:    time.Since(l2GetStart).Milliseconds(),
			Source:        FieldSourceL2,
			ItemCount:     len(cacheKeyStrings),
			IsEntityFetch: len(res.l1CacheKeys) > 0,
		})
	}
	if err != nil {
		// L2 cache errors are non-fatal, continue to fetch
		res.cacheMustBeUpdated = true
		return false, nil
	}

	// Populate FromCache fields in L2 CacheKeys (which have prefixed keys)
	err = l.populateFromCache(l.jsonArena, res.l2CacheKeys, cacheEntries)
	if err != nil {
		res.cacheMustBeUpdated = true
		return false, nil
	}

	// When EntityMergePath is set, the cache stores entity-level data (e.g. {"id":"1234","username":"Me"}).
	// Root field fetches need response-level data (e.g. {"user":{"id":"1234","username":"Me"}}),
	// so wrap the cached entity data back at the merge path before validation.
	for _, ck := range res.l2CacheKeys {
		if len(ck.EntityMergePath) > 0 && ck.FromCache != nil {
			wrapped := ck.FromCache
			for i := len(ck.EntityMergePath) - 1; i >= 0; i-- {
				obj := astjson.ObjectValue(l.jsonArena)
				obj.Set(l.jsonArena, ck.EntityMergePath[i], wrapped)
				wrapped = obj
			}
			ck.FromCache = wrapped
		}
	}

	// Build map of L2 cache key → RemainingTTL for cache age computation
	var remainingTTLs map[string]time.Duration
	if analyticsEnabled {
		remainingTTLs = make(map[string]time.Duration, len(cacheEntries))
		for _, entry := range cacheEntries {
			if entry != nil && entry.RemainingTTL > 0 {
				remainingTTLs[entry.Key] = entry.RemainingTTL
			}
		}
	}

	shadowMode := res.cacheConfig.ShadowMode

	// Copy FromCache values from L2 keys to L1 keys (if L1 keys exist) and track per-entity hits/misses
	// The keys have the same structure, just different key strings
	allComplete := true
	hasAliases := info != nil && info.ProvidesData != nil && info.ProvidesData.HasAliases
	if len(res.l1CacheKeys) > 0 {
		// Entity fetch with L1 keys - copy to L1 keys for merging
		for i := range res.l1CacheKeys {
			if i < len(res.l2CacheKeys) {
				res.l1CacheKeys[i].FromCache = res.l2CacheKeys[i].FromCache
				// Track per-entity L2 hit/miss (atomic operations - thread-safe)
				if res.l1CacheKeys[i].FromCache != nil {
					if info != nil && info.ProvidesData != nil && l.validateItemHasRequiredData(res.l1CacheKeys[i].FromCache, info.ProvidesData) {
						// Denormalize from original field names to current query aliases for merging
						if hasAliases {
							res.l1CacheKeys[i].FromCache = l.denormalizeFromCache(res.l1CacheKeys[i].FromCache, info.ProvidesData)
						}
						if analyticsEnabled && len(res.l1CacheKeys[i].Keys) > 0 {
							byteSize := len(res.l1CacheKeys[i].FromCache.MarshalTo(nil))
							var cacheAgeMs int64
							if i < len(res.l2CacheKeys) && len(res.l2CacheKeys[i].Keys) > 0 {
								cacheAgeMs = computeCacheAgeMs(remainingTTLs[res.l2CacheKeys[i].Keys[0]], res.cacheConfig.TTL)
							}
							res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
								CacheKey: res.l1CacheKeys[i].Keys[0], EntityType: entityType,
								Kind: CacheKeyHit, DataSource: dataSource, ByteSize: byteSize,
								CacheAgeMs: cacheAgeMs, Shadow: shadowMode,
							})
							// Record entity source for L2 hit
							if len(res.cacheConfig.KeyFields) > 0 {
								keyJSON := buildEntityKeyJSON(res.l1CacheKeys[i].FromCache, res.cacheConfig.KeyFields)
								if len(keyJSON) > 0 {
									res.l2EntitySources = append(res.l2EntitySources, entitySourceRecord{
										entityType: entityType, keyJSON: string(keyJSON), source: FieldSourceL2,
									})
								}
							}
						}
						// In shadow mode, save cached value for staleness comparison
						if shadowMode {
							var remaining time.Duration
							if i < len(res.l2CacheKeys) && len(res.l2CacheKeys[i].Keys) > 0 {
								remaining = remainingTTLs[res.l2CacheKeys[i].Keys[0]]
							}
							l.saveShadowCachedValue(res, i, res.l1CacheKeys[i].FromCache, res.l1CacheKeys[i].Keys[0], remaining)
						}
						// Track cached item index when partial loading enabled
						if res.partialCacheEnabled {
							res.cachedItemIndices = append(res.cachedItemIndices, i)
						}
					} else {
						// FromCache is non-nil but missing required fields -> partial hit
						if analyticsEnabled && len(res.l1CacheKeys[i].Keys) > 0 {
							res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
								CacheKey: res.l1CacheKeys[i].Keys[0], EntityType: entityType,
								Kind: CacheKeyPartialHit, DataSource: dataSource, ByteSize: 0,
								Shadow: shadowMode,
							})
						}
						allComplete = false
						// Track fetch item index when partial loading enabled
						if res.partialCacheEnabled {
							res.fetchItemIndices = append(res.fetchItemIndices, i)
						}
					}
				} else {
					if analyticsEnabled && len(res.l1CacheKeys[i].Keys) > 0 {
						res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
							CacheKey: res.l1CacheKeys[i].Keys[0], EntityType: entityType,
							Kind: CacheKeyMiss, DataSource: dataSource, ByteSize: 0,
							Shadow: shadowMode,
						})
					}
					allComplete = false
					// Track fetch item index when partial loading enabled
					if res.partialCacheEnabled {
						res.fetchItemIndices = append(res.fetchItemIndices, i)
					}
				}
			}
		}
	} else {
		// Root fetch (no L1 keys) - track directly from L2 keys
		for i, ck := range res.l2CacheKeys {
			if ck.FromCache != nil {
				if info != nil && info.ProvidesData != nil && l.validateItemHasRequiredData(ck.FromCache, info.ProvidesData) {
					// Denormalize from original field names to current query aliases for merging
					if hasAliases {
						res.l2CacheKeys[i].FromCache = l.denormalizeFromCache(ck.FromCache, info.ProvidesData)
					}
					if analyticsEnabled && len(ck.Keys) > 0 {
						byteSize := len(res.l2CacheKeys[i].FromCache.MarshalTo(nil))
						cacheAgeMs := computeCacheAgeMs(remainingTTLs[ck.Keys[0]], res.cacheConfig.TTL)
						res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
							CacheKey: ck.Keys[0], EntityType: entityType,
							Kind: CacheKeyHit, DataSource: dataSource, ByteSize: byteSize,
							CacheAgeMs: cacheAgeMs, Shadow: shadowMode,
						})
						// Record entity sources from cached root field response
						if len(res.cacheConfig.KeyFields) > 0 {
							walkCachedResponseForSources(res.l2CacheKeys[i].FromCache, res.cacheConfig.KeyFields, entityType, FieldSourceL2, &res.l2EntitySources)
						}
					}
					// Track cached item index when partial loading enabled
					if res.partialCacheEnabled {
						res.cachedItemIndices = append(res.cachedItemIndices, i)
					}
				} else {
					// FromCache is non-nil but missing required fields -> partial hit
					if analyticsEnabled && len(ck.Keys) > 0 {
						res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
							CacheKey: ck.Keys[0], EntityType: entityType,
							Kind: CacheKeyPartialHit, DataSource: dataSource, ByteSize: 0,
							Shadow: shadowMode,
						})
					}
					allComplete = false
					// Track fetch item index when partial loading enabled
					if res.partialCacheEnabled {
						res.fetchItemIndices = append(res.fetchItemIndices, i)
					}
				}
			} else {
				if analyticsEnabled && len(ck.Keys) > 0 {
					res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
						CacheKey: ck.Keys[0], EntityType: entityType,
						Kind: CacheKeyMiss, DataSource: dataSource, ByteSize: 0,
						Shadow: shadowMode,
					})
				}
				allComplete = false
				// Track fetch item index when partial loading enabled
				if res.partialCacheEnabled {
					res.fetchItemIndices = append(res.fetchItemIndices, i)
				}
			}
		}
	}

	// Shadow mode: even if all items were found in cache, we still need to fetch
	// fresh data for comparison. Clear FromCache and force fetch.
	if shadowMode {
		for _, ck := range res.l1CacheKeys {
			ck.FromCache = nil
		}
		res.cachedItemIndices = nil
		res.fetchItemIndices = nil
		res.cacheSkipFetch = false
		res.cacheMustBeUpdated = true
		return false, nil
	}

	if allComplete {
		res.cacheSkipFetch = true
		return true, nil
	}

	res.cacheMustBeUpdated = true
	return false, nil
}

// populateL1Cache stores entity data in the L1 (per-request) cache for later reuse.
// Called after successful fetch and merge for entity fetches only.
// OPTIMIZATION: Only stores if key is missing - existing entries are pointers
// to the same arena data, so no update needed. This minimizes sync.Map calls.
func (l *Loader) populateL1Cache(fetchItem *FetchItem, res *result, _ []*astjson.Value) {
	if !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return
	}
	// Check if UseL1Cache is enabled for this fetch
	cfg := getFetchCaching(fetchItem.Fetch)
	if !cfg.UseL1Cache {
		// Still need to check for root field entity population
		l.populateL1CacheForRootFieldEntities(fetchItem)
		return
	}
	// Extract fetch info (used for both analytics and alias normalization)
	info := getFetchInfo(fetchItem.Fetch)
	var entityType, dataSource string
	if l.ctx.cacheAnalyticsEnabled() && info != nil {
		if len(info.RootFields) > 0 {
			entityType = info.RootFields[0].TypeName
		}
		dataSource = info.DataSourceName
	}
	for _, ck := range res.l1CacheKeys {
		if ck.Item == nil {
			continue
		}
		itemToStore := ck.Item
		if info != nil && info.ProvidesData != nil && info.ProvidesData.HasAliases {
			itemToStore = l.normalizeForCache(ck.Item, info.ProvidesData)
		}
		for _, keyStr := range ck.Keys {
			// Merge new fields into existing cached entity so that different arg suffixes
			// (e.g., friends_AAA and friends_BBB) coexist in the same entity.
			// L1 is only accessed from the main thread, so Load+merge+Store is safe.
			if existing, loaded := l.l1Cache.Load(keyStr); loaded {
				if existingVal, ok := existing.(*astjson.Value); ok {
					l.mergeEntityFields(existingVal, itemToStore)
				}
			} else {
				l.l1Cache.Store(keyStr, itemToStore)
			}
			if l.ctx.cacheAnalyticsEnabled() {
				byteSize := len(ck.Item.MarshalTo(nil))
				l.ctx.cacheAnalytics.RecordWrite(CacheLevelL1, entityType, keyStr, dataSource, byteSize, 0)
			}
		}
	}
	// Also populate L1 cache for root fields that return entities
	l.populateL1CacheForRootFieldEntities(fetchItem)
}

// populateL1CacheForRootFieldEntities populates the L1 cache with entities returned by root fields.
// This allows subsequent entity fetches to benefit from L1 cache hits when the same entities
// were already fetched as part of a root field query.
func (l *Loader) populateL1CacheForRootFieldEntities(fetchItem *FetchItem) {
	// Only applies to SingleFetch (root field fetches)
	singleFetch, ok := fetchItem.Fetch.(*SingleFetch)
	if !ok {
		return
	}

	templates := singleFetch.Caching.RootFieldL1EntityCacheKeyTemplates
	if len(templates) == 0 {
		return
	}

	// Get response data
	data := l.resolvable.data
	if data == nil {
		return
	}

	// Get the path from any template to find where entities are located
	// (all templates for the same root field have the same path)
	var fieldPath []string
	for _, template := range templates {
		entityTemplate, ok := template.(*EntityQueryCacheKeyTemplate)
		if !ok || entityTemplate.Keys == nil || entityTemplate.Keys.Renderer == nil {
			continue
		}
		obj, ok := entityTemplate.Keys.Renderer.Node.(*Object)
		if !ok {
			continue
		}
		fieldPath = obj.Path
		break
	}

	if len(fieldPath) == 0 {
		return
	}

	// Navigate to the entities using the path
	entitiesValue := data.Get(fieldPath...)
	if entitiesValue == nil {
		return
	}

	// Handle both single entity (object) and array of entities
	var entities []*astjson.Value
	switch entitiesValue.Type() {
	case astjson.TypeArray:
		entities = entitiesValue.GetArray()
	case astjson.TypeObject:
		entities = []*astjson.Value{entitiesValue}
	default:
		return
	}

	// For each entity, render cache key and store in L1 cache
	for _, entity := range entities {
		if entity == nil {
			continue
		}

		// Extract __typename to find the right template
		typenameValue := entity.Get("__typename")
		if typenameValue == nil {
			continue
		}
		// Look up template for this typename
		template, ok := templates[string(typenameValue.GetStringBytes())]
		if !ok {
			continue
		}

		entityTemplate, ok := template.(*EntityQueryCacheKeyTemplate)
		if !ok {
			continue
		}

		// Render cache key(s) for this entity
		cacheKeys, err := entityTemplate.RenderCacheKeys(l.jsonArena, l.ctx, []*astjson.Value{entity}, "")
		if err != nil || len(cacheKeys) == 0 {
			continue
		}

		// Store in L1 cache
		for _, ck := range cacheKeys {
			if ck == nil {
				continue
			}
			for _, keyStr := range ck.Keys {
				// Use the entity directly as the cache value
				l.l1Cache.LoadOrStore(keyStr, entity)
			}
		}
	}
}

// getFetchInfo extracts FetchInfo from a Fetch interface
func getFetchInfo(fetch Fetch) *FetchInfo {
	switch f := fetch.(type) {
	case *SingleFetch:
		return f.Info
	case *EntityFetch:
		return f.Info
	case *BatchEntityFetch:
		return f.Info
	}
	return nil
}

// getFetchCaching extracts FetchCacheConfiguration from a Fetch interface
func getFetchCaching(fetch Fetch) FetchCacheConfiguration {
	switch f := fetch.(type) {
	case *SingleFetch:
		return f.Caching
	case *EntityFetch:
		return f.Caching
	case *BatchEntityFetch:
		return f.Caching
	}
	return FetchCacheConfiguration{}
}

// updateL2Cache writes entity data to the L2 (external) cache.
// This enables cross-request caching via external stores like Redis.
func (l *Loader) updateL2Cache(res *result) {
	if !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return
	}
	// Skip L2 cache writes for mutations unless explicitly opted in per-mutation-field.
	// The flag is set in resolveSingle when processing the mutation root fetch.
	if l.info != nil && l.info.OperationType == ast.OperationTypeMutation &&
		!l.enableMutationL2CachePopulation {
		return
	}
	if res.cache == nil || !res.cacheMustBeUpdated {
		return
	}

	// Use l2CacheKeys (with prefix) if available, otherwise fall back to cacheKeys
	keysToStore := res.l2CacheKeys
	if len(keysToStore) == 0 {
		keysToStore = res.l1CacheKeys
	}
	if len(keysToStore) == 0 {
		return
	}

	// Normalize aliased fields to original schema names before storing
	if res.providesData != nil && res.providesData.HasAliases {
		for _, ck := range keysToStore {
			if ck.Item != nil {
				ck.Item = l.normalizeForCache(ck.Item, res.providesData)
			}
		}
	}

	// Merge existing cached fields to preserve other arg variants.
	// ck.FromCache holds the old L2 entity (set by tryL2CacheLoad when validation failed),
	// ck.Item holds the newly fetched and normalized entity.
	// MergeValues merges ck.Item fields into ck.FromCache (mutates first arg);
	// existing old fields are preserved, new fields win on conflicts.
	// On error, skip merge and store only the fresh item (pre-merge behavior).
	for _, ck := range keysToStore {
		if ck.Item != nil && ck.FromCache != nil {
			_, _, err := astjson.MergeValues(l.jsonArena, ck.FromCache, ck.Item)
			if err == nil {
				ck.Item = ck.FromCache
			}
		}
	}

	// Convert CacheKeys to CacheEntries
	cacheEntries, err := l.cacheKeysToEntries(l.jsonArena, keysToStore)
	if err != nil {
		// Cache update errors are non-fatal - silently ignore
		return
	}

	if len(cacheEntries) == 0 {
		return
	}

	// Enrich context with fetch identity when debug mode is enabled
	ctx := l.ctx.ctx
	if l.ctx.Debug {
		ctx = WithCacheFetchInfo(ctx, res.fetchInfo, res.cacheConfig)
	}

	// Cache set errors are non-fatal - silently ignore
	_ = res.cache.Set(ctx, cacheEntries, res.cacheConfig.TTL)

	// Record L2 write events for analytics
	if l.ctx.cacheAnalyticsEnabled() {
		for _, entry := range cacheEntries {
			if entry == nil {
				continue
			}
			l.ctx.cacheAnalytics.RecordWrite(CacheLevelL2, res.analyticsEntityType, entry.Key, res.ds.Name, len(entry.Value), res.cacheConfig.TTL)
		}
	}
}

// saveShadowCachedValue saves a cached L2 value for later staleness comparison in shadow mode.
func (l *Loader) saveShadowCachedValue(res *result, index int, cachedValue *astjson.Value, cacheKey string, remainingTTL time.Duration) {
	if res.shadowCachedValues == nil {
		res.shadowCachedValues = make(map[int]shadowCacheEntry, len(res.l1CacheKeys))
	}
	res.shadowCachedValues[index] = shadowCacheEntry{
		cachedValue:  cachedValue,
		cacheKey:     cacheKey,
		remainingTTL: remainingTTL,
	}
}

// compareShadowValues compares cached L2 values with fresh data after a fetch completes.
// Uses shallowCopyProvidedFields to extract only ProvidesData fields, then hashes
// both values with xxhash. Records ShadowComparisonEvent for each comparison.
// Also records per-field hashes of the cached value (FieldSourceShadowCached) so consumers
// can diff individual fields against the fresh-data hashes recorded during resolution.
// Called from mergeResult on the main thread.
func (l *Loader) compareShadowValues(res *result, info *FetchInfo) {
	if len(res.shadowCachedValues) == 0 || !l.ctx.cacheAnalyticsEnabled() || info == nil || info.ProvidesData == nil {
		return
	}

	dataSource := info.DataSourceName
	var entityType string
	if len(info.RootFields) > 0 {
		entityType = info.RootFields[0].TypeName
	}

	xxh := l.ctx.cacheAnalytics.xxh

	for i, entry := range res.shadowCachedValues {
		if i >= len(res.l1CacheKeys) || res.l1CacheKeys[i].Item == nil {
			continue
		}

		freshValue := res.l1CacheKeys[i].Item

		// Extract only ProvidesData fields from both cached and fresh values
		cachedProvides := l.shallowCopyProvidedFields(entry.cachedValue, info.ProvidesData)
		freshProvides := l.shallowCopyProvidedFields(freshValue, info.ProvidesData)

		// Marshal and hash
		cachedBytes := cachedProvides.MarshalTo(nil)
		freshBytes := freshProvides.MarshalTo(nil)

		xxh.Reset()
		_, _ = xxh.Write(cachedBytes)
		cachedHash := xxh.Sum64()

		xxh.Reset()
		_, _ = xxh.Write(freshBytes)
		freshHash := xxh.Sum64()

		// Compute cache age from stored remainingTTL
		cacheAgeMs := computeCacheAgeMs(entry.remainingTTL, res.cacheConfig.TTL)

		l.ctx.cacheAnalytics.RecordShadowComparison(ShadowComparisonEvent{
			CacheKey:      entry.cacheKey,
			EntityType:    entityType,
			IsFresh:       cachedHash == freshHash,
			CachedHash:    cachedHash,
			FreshHash:     freshHash,
			CachedBytes:   len(cachedBytes),
			FreshBytes:    len(freshBytes),
			DataSource:    dataSource,
			CacheAgeMs:    cacheAgeMs,
			ConfiguredTTL: res.cacheConfig.TTL,
		})

		// Per-field hashing of cached value for field-level change detection.
		// Fresh field hashes are already recorded during resolution (FieldSourceSubgraph).
		// Here we record cached field hashes so the consumer can diff per-field.
		if info.ProvidesData != nil {
			// Build entity key for correlation with resolution-time hashes
			var keyRaw string
			if len(res.cacheConfig.KeyFields) > 0 {
				if keyJSON := buildEntityKeyJSON(entry.cachedValue, res.cacheConfig.KeyFields); len(keyJSON) > 0 {
					keyRaw = string(keyJSON)
				}
			}
			for _, field := range info.ProvidesData.Fields {
				fieldName := string(field.Name)
				fieldVal := cachedProvides.Get(fieldName)
				if fieldVal != nil {
					fieldBytes := fieldVal.MarshalTo(nil)
					l.ctx.cacheAnalytics.HashFieldValue(
						entityType, fieldName, fieldBytes,
						keyRaw, 0, FieldSourceShadowCached,
					)
				}
			}
		}
	}
}

// detectMutationEntityImpact checks if a mutation response contains a cached entity
// and either invalidates (deletes) the L2 cache entry or compares it for staleness analytics.
// Called from mergeResult on the main thread after the mutation fetch completes.
func (l *Loader) detectMutationEntityImpact(res *result, info *FetchInfo, responseData *astjson.Value) map[string]struct{} {
	if info == nil || info.OperationType != ast.OperationTypeMutation {
		return nil
	}
	cfg := res.cacheConfig.MutationEntityImpactConfig
	if cfg == nil {
		return nil
	}
	// Proceed if invalidation is configured or analytics is enabled
	if !cfg.InvalidateCache && !l.ctx.cacheAnalyticsEnabled() {
		return nil
	}
	if info.ProvidesData == nil || len(info.RootFields) == 0 {
		return nil
	}

	// Get the LoaderCache for this entity's cache name
	if l.caches == nil {
		return nil
	}
	cache := l.caches[cfg.CacheName]
	if cache == nil {
		return nil
	}

	mutationFieldName := info.RootFields[0].FieldName

	// Extract entity data from mutation response
	// For root mutation: responseData = {"updateUsername": {"id":"1234","username":"UpdatedMe"}}
	entityData := responseData.Get(mutationFieldName)
	if entityData == nil || entityData.Type() != astjson.TypeObject {
		return nil
	}

	// Navigate ProvidesData to the entity level.
	// ProvidesData describes the mutation response structure: {updateUsername: {id, username}}.
	// We need the inner Object that describes the entity's fields.
	entityProvidesData := navigateProvidesDataToField(info.ProvidesData, mutationFieldName)
	if entityProvidesData == nil {
		return nil
	}

	// Build L2 cache key for lookup
	cacheKey := l.buildMutationEntityCacheKey(cfg, entityData, info)
	if cacheKey == "" {
		return nil
	}

	// Read cached value for analytics BEFORE deleting, so analytics sees the real pre-delete value.
	var analyticsEntries []*CacheEntry
	if l.ctx.cacheAnalyticsEnabled() {
		analyticsEntries, _ = cache.Get(l.ctx.ctx, []string{cacheKey})
	}

	// Invalidate L2 cache entry if configured
	var deletedKeys map[string]struct{}
	if cfg.InvalidateCache {
		_ = cache.Delete(l.ctx.ctx, []string{cacheKey})
		deletedKeys = map[string]struct{}{cacheKey: {}}
	}

	// Analytics comparison requires cacheAnalytics to be enabled
	if !l.ctx.cacheAnalyticsEnabled() {
		return deletedKeys
	}

	// Build display key (without prefix) for analytics
	displayKey := l.buildMutationEntityDisplayKey(cfg, entityData)

	// Hash the fresh (mutation response) value
	freshProvides := l.shallowCopyProvidedFields(entityData, entityProvidesData)
	freshBytes := freshProvides.MarshalTo(nil)
	xxh := l.ctx.cacheAnalytics.xxh
	xxh.Reset()
	_, _ = xxh.Write(freshBytes)
	freshHash := xxh.Sum64()

	// Use the pre-delete cached value for analytics comparison
	hadCachedValue := len(analyticsEntries) > 0 && analyticsEntries[0] != nil && len(analyticsEntries[0].Value) > 0

	if !hadCachedValue {
		// No cached value — record event showing entity was returned but not previously cached
		l.ctx.cacheAnalytics.RecordMutationEvent(MutationEvent{
			MutationRootField: mutationFieldName,
			EntityType:        cfg.EntityTypeName,
			EntityCacheKey:    displayKey,
			HadCachedValue:    false,
			IsStale:           false,
			FreshHash:         freshHash,
			FreshBytes:        len(freshBytes),
		})
		return deletedKeys
	}

	// Parse cached value and compare
	cachedValue, parseErr := astjson.ParseBytesWithArena(l.jsonArena, analyticsEntries[0].Value)
	if parseErr != nil {
		return deletedKeys
	}

	cachedProvides := l.shallowCopyProvidedFields(cachedValue, entityProvidesData)
	cachedBytes := cachedProvides.MarshalTo(nil)
	xxh.Reset()
	_, _ = xxh.Write(cachedBytes)
	cachedHash := xxh.Sum64()

	l.ctx.cacheAnalytics.RecordMutationEvent(MutationEvent{
		MutationRootField: mutationFieldName,
		EntityType:        cfg.EntityTypeName,
		EntityCacheKey:    displayKey,
		HadCachedValue:    true,
		IsStale:           cachedHash != freshHash,
		CachedHash:        cachedHash,
		FreshHash:         freshHash,
		CachedBytes:       len(cachedBytes),
		FreshBytes:        len(freshBytes),
	})
	return deletedKeys
}

// buildMutationEntityCacheKey builds the L2 cache key for a mutation-returned entity.
// Format: [prefix:]{"__typename":"User","key":{"id":"1234"}}
func (l *Loader) buildMutationEntityCacheKey(cfg *MutationEntityImpactConfig, entityData *astjson.Value, info *FetchInfo) string {
	keyObj := astjson.ObjectValue(l.jsonArena)
	keyObj.Set(l.jsonArena, "__typename", astjson.StringValue(l.jsonArena, cfg.EntityTypeName))
	keysObj := buildEntityKeyValue(l.jsonArena, entityData, cfg.KeyFields)
	keyObj.Set(l.jsonArena, "key", keysObj)
	keyJSON := string(keyObj.MarshalTo(nil))

	// Add prefix if needed
	var cacheKey string
	if cfg.IncludeSubgraphHeaderPrefix && l.ctx.SubgraphHeadersBuilder != nil {
		_, headersHash := l.ctx.SubgraphHeadersBuilder.HeadersForSubgraph(info.DataSourceName)
		prefix := strconv.FormatUint(headersHash, 10)
		cacheKey = prefix + ":" + keyJSON
	} else {
		cacheKey = keyJSON
	}

	// Apply user-provided L2 cache key interceptor
	if interceptor := l.ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor; interceptor != nil {
		cacheKey = interceptor(l.ctx.ctx, cacheKey, L2CacheKeyInterceptorInfo{
			SubgraphName: info.DataSourceName,
			CacheName:    cfg.CacheName,
		})
	}
	return cacheKey
}

// buildMutationEntityDisplayKey builds a display key (without prefix) for analytics.
// Format: {"__typename":"User","key":{"id":"1234"}}
func (l *Loader) buildMutationEntityDisplayKey(cfg *MutationEntityImpactConfig, entityData *astjson.Value) string {
	keyObj := astjson.ObjectValue(l.jsonArena)
	keyObj.Set(l.jsonArena, "__typename", astjson.StringValue(l.jsonArena, cfg.EntityTypeName))
	keysObj := buildEntityKeyValue(l.jsonArena, entityData, cfg.KeyFields)
	keyObj.Set(l.jsonArena, "key", keysObj)
	return string(keyObj.MarshalTo(nil))
}

// buildEntityKeyValue recursively builds a JSON object from entity data using only key fields.
func buildEntityKeyValue(a arena.Arena, data *astjson.Value, keyFields []KeyField) *astjson.Value {
	obj := astjson.ObjectValue(a)
	for _, kf := range keyFields {
		if len(kf.Children) > 0 {
			childData := data.Get(kf.Name)
			obj.Set(a, kf.Name, buildEntityKeyValue(a, childData, kf.Children))
		} else {
			val := data.Get(kf.Name)
			if val != nil {
				obj.Set(a, kf.Name, val)
			}
		}
	}
	return obj
}

// processExtensionsCacheInvalidation handles cache invalidation signals from subgraph response extensions.
//
// Subgraphs can signal cache invalidation by including an extensions field in their response:
//
//	{"extensions": {"cacheInvalidation": {"keys": [{"typename": "User", "key": {"id": "1"}}]}}}
//
// This function parses the keys array and deletes the corresponding L2 cache entries.
// Works for both query and mutation responses — not restricted to mutations.
//
// The cache key construction pipeline mirrors the storage pipeline:
//
//	typename + key fields → build JSON → apply header prefix → apply interceptor → cache.Delete()
func (l *Loader) processExtensionsCacheInvalidation(res *result, cacheInvalidation *astjson.Value, deletedKeys map[string]struct{}) {
	// No invalidation data in the response extensions.
	if cacheInvalidation == nil {
		return
	}
	// Extensions-based invalidation only applies when L2 caching is enabled,
	// since L2 is the cross-request cache that benefits from explicit invalidation.
	if !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return
	}
	// entityCacheConfigs maps subgraph name → entity type → config (CacheName, IncludeSubgraphHeaderPrefix).
	// Without this mapping, we don't know which cache to delete from or how to build the key.
	if l.entityCacheConfigs == nil || l.caches == nil {
		return
	}

	// Extract the "keys" array from the cacheInvalidation object.
	// Each entry has {"typename": "User", "key": {"id": "1"}}.
	keysArray := cacheInvalidation.GetArray("keys")
	if len(keysArray) == 0 {
		return
	}

	// Look up the entity cache config for the responding subgraph.
	// The subgraph that sent the invalidation signal is the same one whose entity configs we use,
	// because in federation, the subgraph that caches an entity is the one that resolves it.
	subgraphName := res.ds.Name
	subgraphConfigs := l.entityCacheConfigs[subgraphName]
	if subgraphConfigs == nil {
		return
	}

	// Build set of L2 keys that updateL2Cache will set after this function returns.
	// Deleting a key that's about to be re-set with fresh data is redundant.
	keysAboutToBeSet := l.l2KeysAboutToBeSet(res)

	// Group invalidation keys by cache name so we can batch-delete per cache instance.
	type cacheDeleteBatch struct {
		cache LoaderCache
		keys  []string
	}
	batches := map[string]*cacheDeleteBatch{}

	for _, entry := range keysArray {
		// Skip malformed entries (must be JSON objects).
		if entry == nil || entry.Type() != astjson.TypeObject {
			continue
		}

		// Extract "typename" (string) and "key" (JSON object) from each invalidation entry.
		typenameVal := entry.Get("typename")
		keyVal := entry.Get("key")
		if typenameVal == nil || keyVal == nil || keyVal.Type() != astjson.TypeObject {
			continue
		}
		typename := string(typenameVal.GetStringBytes())
		if typename == "" {
			continue
		}

		// Look up the entity cache config for this typename from the responding subgraph.
		// This tells us which cache instance to use and whether to apply header prefix.
		// Unknown typenames are silently skipped — the subgraph may send invalidation
		// for types that aren't configured for caching on this router.
		entityConfig := subgraphConfigs[typename]
		if entityConfig == nil {
			continue
		}

		// Resolve the cache instance by name.
		cache := l.caches[entityConfig.CacheName]
		if cache == nil {
			continue
		}

		// Build the base cache key JSON matching the format used during cache population:
		// {"__typename":"User","key":{"id":"1"}}
		// The "key" value is taken directly from the extensions — it's already a JSON object
		// with the entity's @key field values.
		keyObj := astjson.ObjectValue(l.jsonArena)
		keyObj.Set(l.jsonArena, "__typename", astjson.StringValue(l.jsonArena, typename))
		keyObj.Set(l.jsonArena, "key", keyVal)
		baseKey := string(keyObj.MarshalTo(nil))
		cacheKey := baseKey

		// Apply subgraph header prefix if configured for this entity type.
		// This mirrors prepareCacheKeys() which prefixes L2 keys with a hash of the
		// HTTP headers sent to the subgraph, enabling per-tenant cache isolation.
		// Result: "55555:{"__typename":"User","key":{"id":"1"}}"
		if entityConfig.IncludeSubgraphHeaderPrefix && l.ctx.SubgraphHeadersBuilder != nil {
			_, headersHash := l.ctx.SubgraphHeadersBuilder.HeadersForSubgraph(subgraphName)
			var buf [20]byte
			b := strconv.AppendUint(buf[:0], headersHash, 10)
			cacheKey = string(b) + ":" + cacheKey
		}

		// Apply user-provided L2 cache key interceptor if set.
		// This allows user-defined key transformations (e.g., tenant isolation prefixes)
		// and mirrors the same interceptor applied during cache population.
		if interceptor := l.ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor; interceptor != nil {
			cacheKey = interceptor(l.ctx.ctx, cacheKey, L2CacheKeyInterceptorInfo{
				SubgraphName: subgraphName,
				CacheName:    entityConfig.CacheName,
			})
		}

		// Skip L2 delete if:
		// - already deleted by detectMutationEntityImpact (deduplication)
		// - about to be re-set by updateL2Cache (redundant delete before set)
		if _, alreadyDone := deletedKeys[cacheKey]; alreadyDone {
			continue
		}
		if _, aboutToBeSet := keysAboutToBeSet[cacheKey]; aboutToBeSet {
			continue
		}

		// Accumulate the key into the batch for this cache name.
		batch, ok := batches[entityConfig.CacheName]
		if !ok {
			batch = &cacheDeleteBatch{cache: cache}
			batches[entityConfig.CacheName] = batch
		}
		batch.keys = append(batch.keys, cacheKey)
	}

	// Execute batched L2 cache deletes — one Delete call per cache instance.
	for _, batch := range batches {
		_ = batch.cache.Delete(l.ctx.ctx, batch.keys)
	}
}

// l2KeysAboutToBeSet returns the set of L2 cache keys that updateL2Cache will store
// after the current fetch. Returns nil if updateL2Cache won't run (e.g., mutations
// without explicit L2 population, or no cache misses to populate).
func (l *Loader) l2KeysAboutToBeSet(res *result) map[string]struct{} {
	// updateL2Cache skips for mutations unless L2 population is explicitly enabled.
	if l.info != nil && l.info.OperationType == ast.OperationTypeMutation &&
		!l.enableMutationL2CachePopulation {
		return nil
	}
	if res.cache == nil || !res.cacheMustBeUpdated {
		return nil
	}
	keys := res.l2CacheKeys
	if len(keys) == 0 {
		keys = res.l1CacheKeys
	}
	if len(keys) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(keys))
	for _, ck := range keys {
		// Skip keys whose Item is nil — updateL2Cache won't store them
		// (can happen if an entity failed to merge during batch processing).
		if ck == nil || ck.Item == nil {
			continue
		}
		for _, k := range ck.Keys {
			set[k] = struct{}{}
		}
	}
	return set
}

// navigateProvidesDataToField finds the Object within ProvidesData that corresponds
// to a specific field name. For root mutations, ProvidesData describes the full response
// (e.g., {updateUsername: {id, username}}) and we need the inner Object for comparison.
func navigateProvidesDataToField(providesData *Object, fieldName string) *Object {
	if providesData == nil {
		return nil
	}
	for _, field := range providesData.Fields {
		if string(field.Name) == fieldName {
			if obj, ok := field.Value.(*Object); ok {
				return obj
			}
		}
	}
	return nil
}

// validateItemHasRequiredData checks if the given item contains all required data
// as specified by the provided Object schema
func (l *Loader) validateItemHasRequiredData(item *astjson.Value, obj *Object) bool {
	if item == nil {
		return false
	}
	// Validate each field in the object
	for _, field := range obj.Fields {
		if !l.validateFieldData(item, field) {
			return false
		}
	}

	return true
}

// validateFieldData validates a single field against the item data.
// Uses cacheFieldName() to look up by original name + arg suffix since cached data is normalized.
func (l *Loader) validateFieldData(item *astjson.Value, field *Field) bool {
	fieldValue := item.Get(l.cacheFieldName(field))

	// Check if field exists
	if fieldValue == nil {
		// Field is missing - this fails validation regardless of nullability
		// Even nullable fields must be present (can be null, but not missing)
		return false
	}

	// Validate the field value against its specification
	return l.validateNodeValue(fieldValue, field.Value)
}

// validateScalarData validates scalar field data
func (l *Loader) validateScalarData(value *astjson.Value, scalar *Scalar) bool {
	if value.Type() == astjson.TypeNull {
		// Null is only allowed if the scalar is nullable
		return scalar.Nullable
	}

	// Any non-null value is acceptable for a scalar
	return true
}

// validateObjectData validates object field data
func (l *Loader) validateObjectData(value *astjson.Value, obj *Object) bool {
	if value.Type() == astjson.TypeNull {
		// Null is only allowed if the object is nullable
		return obj.Nullable
	}

	if value.Type() != astjson.TypeObject {
		// Must be an object (or null if nullable)
		return false
	}

	// Recursively validate the object's fields
	return l.validateItemHasRequiredData(value, obj)
}

// validateArrayData validates array field data
func (l *Loader) validateArrayData(value *astjson.Value, arr *Array) bool {
	if value.Type() == astjson.TypeNull {
		// Null is only allowed if the array is nullable
		return arr.Nullable
	}

	if value.Type() != astjson.TypeArray {
		// Must be an array (or null if nullable)
		return false
	}

	// If there's no item specification, we just validate the array exists
	if arr.Item == nil {
		return true
	}

	// Validate each item in the array
	arrayItems, err := value.Array()
	if err != nil {
		return false
	}

	for _, item := range arrayItems {
		if !l.validateNodeValue(item, arr.Item) {
			return false
		}
	}

	return true
}

// validateNodeValue validates a value against a Node specification
func (l *Loader) validateNodeValue(value *astjson.Value, nodeSpec Node) bool {
	switch v := nodeSpec.(type) {
	case *Scalar:
		return l.validateScalarData(value, v)
	case *Object:
		return l.validateObjectData(value, v)
	case *Array:
		return l.validateArrayData(value, v)
	default:
		// Unknown type - assume invalid
		return false
	}
}

// normalizeForCache transforms field keys for cache storage: renames aliases to original
// schema field names, and appends xxhash suffixes for fields with arguments.
// Returns input unchanged if obj.HasAliases is false (fast path — no aliases or CacheArgs).
func (l *Loader) normalizeForCache(item *astjson.Value, obj *Object) *astjson.Value {
	if item == nil || obj == nil || !obj.HasAliases {
		return item
	}
	if item.Type() != astjson.TypeObject {
		return item
	}
	result := astjson.ObjectValue(l.jsonArena)
	for _, field := range obj.Fields {
		aliasName := unsafebytes.BytesToString(field.Name)
		fieldValue := item.Get(aliasName)
		if fieldValue == nil {
			continue
		}
		normalizedValue := l.normalizeNode(fieldValue, field.Value)
		result.Set(l.jsonArena, l.cacheFieldName(field), normalizedValue)
	}
	// Preserve __typename if present and not already in fields
	if typenameValue := item.Get("__typename"); typenameValue != nil {
		hasTypenameField := false
		for _, field := range obj.Fields {
			if l.cacheFieldName(field) == "__typename" {
				hasTypenameField = true
				break
			}
		}
		if !hasTypenameField {
			result.Set(l.jsonArena, "__typename", typenameValue)
		}
	}
	return result
}

// normalizeNode recursively normalizes nested objects/arrays.
func (l *Loader) normalizeNode(val *astjson.Value, node Node) *astjson.Value {
	if val == nil || node == nil {
		return val
	}
	switch n := node.(type) {
	case *Object:
		return l.normalizeForCache(val, n)
	case *Array:
		if n.Item != nil && val.Type() == astjson.TypeArray {
			arr := astjson.ArrayValue(l.jsonArena)
			for i, item := range val.GetArray() {
				arr.SetArrayItem(l.jsonArena, i, l.normalizeNode(item, n.Item))
			}
			return arr
		}
	}
	return val
}

// denormalizeFromCache reverses normalizeForCache: maps suffixed schema field names back
// to query aliases. Returns input unchanged if obj.HasAliases is false (fast path).
func (l *Loader) denormalizeFromCache(item *astjson.Value, obj *Object) *astjson.Value {
	if item == nil || obj == nil || !obj.HasAliases {
		return item
	}
	if item.Type() != astjson.TypeObject {
		return item
	}
	result := astjson.ObjectValue(l.jsonArena)
	for _, field := range obj.Fields {
		lookupName := l.cacheFieldName(field)
		outputName := unsafebytes.BytesToString(field.Name)
		fieldValue := item.Get(lookupName)
		if fieldValue == nil {
			continue
		}
		denormalizedValue := l.denormalizeNode(fieldValue, field.Value)
		result.Set(l.jsonArena, outputName, denormalizedValue)
	}
	// Preserve __typename if present
	if typenameValue := item.Get("__typename"); typenameValue != nil {
		hasTypenameField := false
		for _, field := range obj.Fields {
			if l.cacheFieldName(field) == "__typename" {
				hasTypenameField = true
				break
			}
		}
		if !hasTypenameField {
			result.Set(l.jsonArena, "__typename", typenameValue)
		}
	}
	return result
}

// denormalizeNode recursively denormalizes nested objects/arrays.
func (l *Loader) denormalizeNode(val *astjson.Value, node Node) *astjson.Value {
	if val == nil || node == nil {
		return val
	}
	switch n := node.(type) {
	case *Object:
		return l.denormalizeFromCache(val, n)
	case *Array:
		if n.Item != nil && val.Type() == astjson.TypeArray {
			arr := astjson.ArrayValue(l.jsonArena)
			for i, item := range val.GetArray() {
				arr.SetArrayItem(l.jsonArena, i, l.denormalizeNode(item, n.Item))
			}
			return arr
		}
	}
	return val
}

// cacheFieldName returns the field name to use in cached entity data.
// For fields without arguments, returns SchemaFieldName() (zero overhead).
// For fields with arguments, appends an xxhash suffix based on resolved arg values,
// ensuring that e.g. friends(first:5) and friends(first:20) use different cache field names.
func (l *Loader) cacheFieldName(field *Field) string {
	if len(field.CacheArgs) == 0 {
		return field.SchemaFieldName()
	}
	return field.SchemaFieldName() + l.computeArgSuffix(field.CacheArgs)
}

// computeArgSuffix computes "_<16-hex-chars>" from resolved argument values.
// Args are sorted by ArgName for deterministic output (guaranteed at plan time).
// Each arg value is resolved from ctx.Variables (with RemapVariables support)
// and serialized as JSON for hashing.
func (l *Loader) computeArgSuffix(args []CacheFieldArg) string {
	// Ensure sorted by arg name (should already be sorted at plan time)
	sorted := args
	if !slices.IsSortedFunc(sorted, func(a, b CacheFieldArg) int {
		return cmp.Compare(a.ArgName, b.ArgName)
	}) {
		sorted = slices.Clone(args)
		slices.SortFunc(sorted, func(a, b CacheFieldArg) int {
			return cmp.Compare(a.ArgName, b.ArgName)
		})
	}

	h := pool.Hash64.Get()
	for i, arg := range sorted {
		if i > 0 {
			_, _ = h.WriteString(",")
		}
		_, _ = h.WriteString(arg.ArgName)
		_, _ = h.WriteString(":")

		// Resolve variable from ctx.Variables, applying RemapVariables
		varName := arg.VariableName
		if l.ctx.RemapVariables != nil {
			if nameToUse, hasMapping := l.ctx.RemapVariables[varName]; hasMapping {
				varName = nameToUse
			}
		}

		argValue := l.ctx.Variables.Get(varName)
		if argValue == nil {
			_, _ = h.WriteString("null")
		} else {
			writeCanonicalJSON(h, argValue)
		}
	}

	sum := h.Sum64()
	pool.Hash64.Put(h)

	// Format as "_" + 16 zero-padded hex digits without fmt.Sprintf
	var buf [17]byte
	buf[0] = '_'
	const hexDigits = "0123456789abcdef"
	for i := 15; i >= 0; i-- {
		buf[1+i] = hexDigits[sum&0xf]
		sum >>= 4
	}
	return string(buf[:])
}

// writeCanonicalJSON writes a deterministic JSON representation of v to w.
// For objects, keys are sorted alphabetically to ensure the same logical value
// always produces the same hash regardless of JSON key ordering from the client.
// For arrays, elements are written in order. Scalars are written as-is.
func writeCanonicalJSON(w interface{ WriteString(string) (int, error) }, v *astjson.Value) {
	switch v.Type() {
	case astjson.TypeObject:
		obj, err := v.Object()
		if err != nil {
			_, _ = w.WriteString("null")
			return
		}
		// Collect keys and sort them
		type kv struct {
			key string
			val *astjson.Value
		}
		var pairs []kv
		obj.Visit(func(key []byte, val *astjson.Value) {
			pairs = append(pairs, kv{key: string(key), val: val})
		})
		slices.SortFunc(pairs, func(a, b kv) int {
			return cmp.Compare(a.key, b.key)
		})
		_, _ = w.WriteString("{")
		for i, p := range pairs {
			if i > 0 {
				_, _ = w.WriteString(",")
			}
			_, _ = w.WriteString(strconv.Quote(p.key))
			_, _ = w.WriteString(":")
			writeCanonicalJSON(w, p.val)
		}
		_, _ = w.WriteString("}")
	case astjson.TypeArray:
		arr := v.GetArray()
		_, _ = w.WriteString("[")
		for i, elem := range arr {
			if i > 0 {
				_, _ = w.WriteString(",")
			}
			writeCanonicalJSON(w, elem)
		}
		_, _ = w.WriteString("]")
	default:
		// Scalars (string, number, bool, null): MarshalTo produces canonical output
		var buf [64]byte
		_, _ = w.WriteString(string(v.MarshalTo(buf[:0])))
	}
}

// mergeEntityFields copies all fields from src into dst that aren't already present.
// Used during L1 cache population to accumulate fields with different arg suffixes
// (e.g., friends_AAA and friends_BBBB coexist in the same cached entity).
// First-writer-wins: for suffixed fields each arg variant has a unique suffix so no conflict;
// for key fields (id, __typename) values are identical across fetches for the same entity.
func (l *Loader) mergeEntityFields(dst, src *astjson.Value) {
	if dst == nil || src == nil {
		return
	}
	if dst.Type() != astjson.TypeObject || src.Type() != astjson.TypeObject {
		return
	}
	srcObj, _ := src.Object()
	srcObj.Visit(func(key []byte, v *astjson.Value) {
		if dst.Get(string(key)) == nil {
			dst.Set(l.jsonArena, string(key), v)
		}
	})
}
