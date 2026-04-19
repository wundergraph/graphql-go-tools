package resolve

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// CacheWriteReason identifies why a cache entry was written.
type CacheWriteReason string

const (
	// CacheWriteReasonRefresh means an existing cached key was rewritten with fresh or merged data.
	CacheWriteReasonRefresh CacheWriteReason = "refresh"
	// CacheWriteReasonBackfill means a requested key that missed on read was proven by final entity data.
	CacheWriteReasonBackfill CacheWriteReason = "backfill"
	// CacheWriteReasonDerived means a new key was derived from final entity data that was not in the original request.
	CacheWriteReasonDerived CacheWriteReason = "derived"
)

type CacheEntry struct {
	Key          string
	Value        []byte
	RemainingTTL time.Duration    // remaining TTL from cache (0 = unknown/not supported)
	WriteReason  CacheWriteReason // why this entry was written (empty for reads)
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

// l1AnalyticsSize returns the byte size of an L1 entry for analytics purposes.
// Returns 0 (avoiding the marshal cost) when analytics are disabled.
func l1AnalyticsSize(enabled bool, v *astjson.Value) int {
	if !enabled || v == nil {
		return 0
	}
	return len(v.MarshalTo(nil))
}

// hasNonEmptyKey reports whether any entry in keys is a non-empty string.
// Used as a defensive guard before issuing an L2 Get — a batch of entirely
// empty strings is never a legitimate lookup, so skip it cleanly.
func hasNonEmptyKey(keys []string) bool {
	for _, k := range keys {
		if k != "" {
			return true
		}
	}
	return false
}

// extractCacheKeysStrings extracts all unique cache key strings from CacheKeys
func (l *Loader) extractCacheKeysStrings(a arena.Arena, cacheKeys []*CacheKey) []string {
	if len(cacheKeys) == 0 {
		return nil
	}
	out := make([]string, 0, len(cacheKeys))
	seen := make(map[string]struct{}, len(cacheKeys))
	for i := range cacheKeys {
		for j := range cacheKeys[i].Keys {
			keyStr := cacheKeys[i].Keys[j]
			if _, ok := seen[keyStr]; ok {
				continue
			}
			seen[keyStr] = struct{}{}
			out = append(out, keyStr)
		}
	}
	return out
}

// countUniqueCacheKeyStrings counts unique cache key strings across CacheKeys
// without allocating the strings slice. Used by analytics/tracing call sites
// that only need the count.
func countUniqueCacheKeyStrings(cacheKeys []*CacheKey) int {
	if len(cacheKeys) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(cacheKeys))
	for i := range cacheKeys {
		for j := range cacheKeys[i].Keys {
			seen[cacheKeys[i].Keys[j]] = struct{}{}
		}
	}
	return len(seen)
}

// populateFromCache populates CacheKey.FromCache fields from cache entries.
// Parses each candidate VERBATIM via l.parser onto the given arena.
// Denormalization (alias re-application) happens LATER at the materialization
// site via structuralCopyDenormalized.
func (l *Loader) populateFromCache(a arena.Arena, cacheKeys []*CacheKey, entries []*CacheEntry) error {
	return l.populateCacheKeysFromIndex(a, cacheKeys, indexCacheEntriesByKey(entries))
}

// indexCacheEntriesByKey builds a map[key]*CacheEntry from a raw cache-Get response.
// Nil entries are filtered. Later entries with duplicate keys overwrite earlier ones
// (matches existing behavior at the bulk-L2 call site).
func indexCacheEntriesByKey(entries []*CacheEntry) map[string]*CacheEntry {
	if len(entries) == 0 {
		return nil
	}
	byKey := make(map[string]*CacheEntry, len(entries))
	for _, e := range entries {
		if e != nil {
			byKey[e.Key] = e
		}
	}
	return byKey
}

// populateCacheKeysFromIndex is the shared per-CacheKey match+parse loop used by
// both populateFromCache (sequential path) and populateFromCacheBulk (parallel
// path). It resets the cache-read state on each CacheKey, collects candidates
// from byKey, records missingKeys, sorts by freshness, and parses the freshest
// candidate verbatim onto the arena.
func (l *Loader) populateCacheKeysFromIndex(a arena.Arena, cacheKeys []*CacheKey, byKey map[string]*CacheEntry) error {
	for j := range cacheKeys {
		ck := cacheKeys[j]
		ck.FromCache = nil
		ck.missingKeys = nil
		ck.cachedData = cachedData{}

		var candidates []fromCacheCandidate
		matchedKeys := make(map[string]struct{}, len(ck.Keys))
		for _, key := range ck.Keys {
			entry, ok := byKey[key]
			if !ok || entry == nil || entry.Value == nil {
				continue
			}
			matchedKeys[key] = struct{}{}
			candidates = append(candidates, fromCacheCandidate{
				value:        entry.Value,
				remainingTTL: entry.RemainingTTL,
			})
		}
		for _, key := range ck.Keys {
			if _, ok := matchedKeys[key]; !ok {
				ck.missingKeys = append(ck.missingKeys, key)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		slices.SortStableFunc(candidates, func(a, b fromCacheCandidate) int {
			return compareCacheCandidateFreshness(a.remainingTTL, b.remainingTTL)
		})
		ck.fromCacheCandidates = candidates
		// Safe: guarded by len(candidates) == 0 continue above, so candidates[0] exists.
		ck.fromCacheRemainingTTL = candidates[0].remainingTTL
		parsed, err := l.parseL2Bytes(a, candidates[0].value)
		if err != nil {
			return errors.WithStack(err)
		}
		ck.FromCache = parsed
	}
	return nil
}

// parseL2Bytes parses an L2 cache entry's bytes into a *astjson.Value on the
// given arena, VERBATIM (no Transform). Uses l.parser — main thread only.
// Denormalization is applied separately at the materialization site via
// structuralCopyDenormalized.
func (l *Loader) parseL2Bytes(a arena.Arena, bytes []byte) (*astjson.Value, error) {
	return l.parser.ParseBytesWithArena(a, bytes)
}

func compareCacheCandidateFreshness(a, b time.Duration) int {
	aKnown := a > 0
	bKnown := b > 0
	switch {
	case aKnown && bKnown:
		return cmp.Compare(b, a)
	case aKnown:
		return -1
	case bKnown:
		return 1
	default:
		return 0
	}
}

func wrapCacheValueAtMergePath(a arena.Arena, value *astjson.Value, mergePath []string) *astjson.Value {
	if value == nil || len(mergePath) == 0 {
		return value
	}
	wrapped := value
	for i := len(mergePath) - 1; i >= 0; i-- {
		obj := astjson.ObjectValue(a)
		obj.Set(a, mergePath[i], wrapped)
		wrapped = obj
	}
	return wrapped
}

func (l *Loader) reorderCacheValueToSelectionOrder(a arena.Arena, value *astjson.Value, node Node) *astjson.Value {
	if value == nil || node == nil {
		return value
	}

	switch n := node.(type) {
	case *Object:
		if value.Type() != astjson.TypeObject {
			return value
		}
		reordered := astjson.ObjectValue(a)
		seen := make(map[string]struct{}, len(n.Fields))
		for _, field := range n.Fields {
			fieldName := l.cacheFieldName(field)
			fieldValue := value.Get(fieldName)
			if fieldValue == nil {
				continue
			}
			reordered.Set(a, fieldName, l.reorderCacheValueToSelectionOrder(a, fieldValue, field.Value))
			seen[fieldName] = struct{}{}
		}

		obj, err := value.Object()
		if err != nil {
			return value
		}
		obj.Visit(func(key []byte, fieldValue *astjson.Value) {
			fieldName := string(key)
			if _, ok := seen[fieldName]; ok {
				return
			}
			reordered.Set(a, fieldName, fieldValue)
		})
		return reordered
	case *Array:
		if value.Type() != astjson.TypeArray {
			return value
		}
		items, err := value.Array()
		if err != nil {
			return value
		}
		reordered := astjson.ArrayValue(a)
		for i, item := range items {
			reordered.SetArrayItem(a, i, l.reorderCacheValueToSelectionOrder(a, item, n.Item))
		}
		return reordered
	default:
		return value
	}
}

func (l *Loader) resolveMultiCandidateCacheValue(a arena.Arena, ck *CacheKey, providesData *Object) bool {
	if ck.FromCache == nil {
		return false
	}
	if providesData == nil || l.validateItemHasRequiredData(ck.FromCache, providesData) {
		return true
	}
	if len(ck.fromCacheCandidates) <= 1 {
		return false
	}

	var merged *astjson.Value
	for i := len(ck.fromCacheCandidates) - 1; i >= 0; i-- {
		parsed, err := l.parseL2Bytes(a, ck.fromCacheCandidates[i].value)
		if err != nil {
			continue
		}
		parsed = wrapCacheValueAtMergePath(a, parsed, ck.EntityMergePath)
		if merged == nil {
			merged = parsed
			continue
		}
		if _, err = astjson.MergeValues(a, merged, parsed); err != nil {
			merged = nil
			break
		}
	}
	if merged != nil && l.validateItemHasRequiredData(merged, providesData) {
		ck.FromCache = l.reorderCacheValueToSelectionOrder(a, merged, providesData)
		ck.fromCacheNeedsWriteback = true
		return true
	}

	for i := 1; i < len(ck.fromCacheCandidates); i++ {
		parsed, err := l.parseL2Bytes(a, ck.fromCacheCandidates[i].value)
		if err != nil {
			continue
		}
		parsed = wrapCacheValueAtMergePath(a, parsed, ck.EntityMergePath)
		if l.validateItemHasRequiredData(parsed, providesData) {
			ck.FromCache = l.reorderCacheValueToSelectionOrder(a, parsed, providesData)
			ck.fromCacheRemainingTTL = ck.fromCacheCandidates[i].remainingTTL
			ck.fromCacheNeedsWriteback = true
			return true
		}
	}

	return false
}

func batchEntityValidationObject(providesData *Object, entityMergePath []string) *Object {
	if providesData == nil {
		return nil
	}
	if len(entityMergePath) == 0 {
		return providesData
	}

	current := providesData
	for i, segment := range entityMergePath {
		var next Node
		for _, field := range current.Fields {
			if string(field.Name) == segment || string(field.OriginalName) == segment {
				next = field.Value
				break
			}
		}
		if next == nil {
			return nil
		}
		if i == len(entityMergePath)-1 {
			switch value := next.(type) {
			case *Object:
				return value
			case *Array:
				obj, _ := value.Item.(*Object)
				return obj
			default:
				return nil
			}
		}
		switch value := next.(type) {
		case *Object:
			current = value
		case *Array:
			obj, ok := value.Item.(*Object)
			if !ok {
				return nil
			}
			current = obj
		default:
			return nil
		}
	}

	return current
}

func (l *Loader) resolveBatchEntityCacheValue(a arena.Arena, ck *CacheKey, providesData *Object) bool {
	if ck.FromCache == nil {
		return false
	}
	if providesData == nil || l.validateItemHasRequiredData(ck.FromCache, providesData) {
		return true
	}
	if len(ck.fromCacheCandidates) <= 1 {
		return false
	}

	var merged *astjson.Value
	for i := len(ck.fromCacheCandidates) - 1; i >= 0; i-- {
		parsed, err := l.parseL2Bytes(a, ck.fromCacheCandidates[i].value)
		if err != nil {
			continue
		}
		if merged == nil {
			merged = parsed
			continue
		}
		if _, err = astjson.MergeValues(a, merged, parsed); err != nil {
			merged = nil
			break
		}
	}
	if merged != nil && l.validateItemHasRequiredData(merged, providesData) {
		ck.FromCache = l.reorderCacheValueToSelectionOrder(a, merged, providesData)
		ck.fromCacheNeedsWriteback = true
		return true
	}

	for i := 1; i < len(ck.fromCacheCandidates); i++ {
		parsed, err := l.parseL2Bytes(a, ck.fromCacheCandidates[i].value)
		if err != nil {
			continue
		}
		if l.validateItemHasRequiredData(parsed, providesData) {
			ck.FromCache = l.reorderCacheValueToSelectionOrder(a, parsed, providesData)
			ck.fromCacheRemainingTTL = ck.fromCacheCandidates[i].remainingTTL
			ck.fromCacheNeedsWriteback = true
			return true
		}
	}

	return false
}

func hasMissingRequestedKeys(cacheKeys []*CacheKey) bool {
	for _, ck := range cacheKeys {
		if len(ck.missingKeys) > 0 {
			return true
		}
	}
	return false
}

func needsResolvedCacheWriteback(cacheKeys []*CacheKey) bool {
	for _, ck := range cacheKeys {
		if ck.fromCacheNeedsWriteback {
			return true
		}
	}
	return false
}

// cacheKeysToEntries converts CacheKeys to CacheEntries for storage
// For each CacheKey, creates entries for all its KeyEntries with the same value
func (l *Loader) cacheKeysToEntries(a arena.Arena, cacheKeys []*CacheKey) ([]*CacheEntry, error) {
	// Use heap slice for []*CacheEntry — arena memory is noscan, so GC cannot
	// trace *CacheEntry pointers stored there, risking premature collection.
	out := make([]*CacheEntry, 0, len(cacheKeys))
	buf := arena.AllocateSlice[byte](a, 64, 64)
	seen := make(map[string]struct{}, len(cacheKeys))
	for i := range cacheKeys {
		for j := range cacheKeys[i].Keys {
			if cacheKeys[i].Item == nil || cacheKeys[i].NegativeCacheHit {
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
			// Preserve fields from the previously cached object when this writeback only
			// contains a narrower entity projection. Without this merge, a follow-up fetch
			// can overwrite shared entity/root cache state with partial data and turn the
			// next request into an incorrect cache hit.
			//
			// The pointer check avoids re-merging when itemToStore already points at the
			// cached AST value.
			if cacheKeys[i].FromCache != nil && itemToStore != cacheKeys[i].FromCache {
				if merged := mergeCachedValueForWrite(a, cacheKeys[i].FromCache, itemToStore); merged != nil {
					itemToStore = merged
				}
			}
			buf = itemToStore.MarshalTo(buf[:0])
			// Value must be heap-allocated: it is handed to the L2 cache (e.g. ristretto)
			// which retains the slice across requests. The arena `a` (jsonArena) is reset
			// at the end of the request, so an arena-backed slice would be overwritten and
			// subsequent cache reads would return corrupted bytes.
			entryValue := make([]byte, len(buf))
			copy(entryValue, buf)
			out = append(out, &CacheEntry{
				Key:   cacheKeys[i].Keys[j],
				Value: entryValue,
			})
		}
	}
	return out, nil
}

// mergeCachedValueForWrite preserves fields from the older cached object when a
// follow-up writeback only contains a narrower entity projection for the same key.
// The fresh payload still wins on overlapping fields.
func mergeCachedValueForWrite(a arena.Arena, cachedValue, freshValue *astjson.Value) *astjson.Value {
	if cachedValue == nil || freshValue == nil {
		return freshValue
	}
	if cachedValue.Type() != astjson.TypeObject || freshValue.Type() != astjson.TypeObject {
		return freshValue
	}
	merged, err := astjson.MergeValues(a, cachedValue, freshValue)
	if err != nil {
		return freshValue
	}
	return merged
}

// cacheKeysToNegativeEntries collects L2 cache entries for null entity responses (negative caching).
// Only entries flagged with NegativeCacheHit are included.
// Most negative-cache entries store the literal null sentinel. When the same cache key already has
// positive entity data beyond its key fields, keep that object shape and materialize the requested
// nullable fields as explicit nulls. That lets later shared-key reads preserve the parent/root shape
// without turning key-only scaffolding into a false positive cache hit.
func (l *Loader) cacheKeysToNegativeEntries(a arena.Arena, res *result, cacheKeys []*CacheKey) []*CacheEntry {
	var out []*CacheEntry
	seen := make(map[string]struct{})
	for i := range cacheKeys {
		if !cacheKeys[i].NegativeCacheHit {
			continue
		}
		value := l.negativeCachePositiveValue(a, res, cacheKeys[i])
		if len(value) == 0 {
			value = []byte("null")
		}
		for _, keyStr := range cacheKeys[i].Keys {
			if _, ok := seen[keyStr]; ok {
				continue
			}
			seen[keyStr] = struct{}{}
			// Clone per entry: multiple keys in the same iteration would otherwise
			// alias one slice, letting external cache implementations that retain
			// Value leak mutations across keys.
			out = append(out, &CacheEntry{
				Key:   keyStr,
				Value: bytes.Clone(value),
			})
		}
	}
	return out
}

// negativeCachePositiveValue reuses an existing object-shaped payload for negative-cache writes
// only when it carries data beyond the entity key fields. Key-only payloads still collapse to the
// literal null sentinel so later reads do not treat bare identity scaffolding as a full entity hit.
func (l *Loader) negativeCachePositiveValue(a arena.Arena, res *result, ck *CacheKey) []byte {
	if !cacheKeyHasPositiveEntityData(ck) {
		return nil
	}
	entity := ck.Item
	if entity == nil {
		entity = ck.FromCache
	}
	if entity == nil {
		return nil
	}
	if len(ck.EntityMergePath) > 0 {
		entity = entity.Get(ck.EntityMergePath...)
	}
	if entity == nil || entity.Type() != astjson.TypeObject {
		return nil
	}
	cloned, err := astjson.ParseBytesWithArena(a, entity.MarshalTo(nil))
	if err != nil {
		return nil
	}
	l.materializeNullableFieldsAsNull(a, cloned, res.providesData)
	return cloned.MarshalTo(nil)
}

// materializeNullableFieldsAsNull fills in missing nullable fields before storing an object-shaped
// negative-cache value. Later validation can then satisfy the same selection set from cache, while
// still leaving non-null or otherwise unproven fields absent so they continue to force a refetch.
func (l *Loader) materializeNullableFieldsAsNull(a arena.Arena, entity *astjson.Value, obj *Object) {
	if entity == nil || obj == nil || entity.Type() != astjson.TypeObject {
		return
	}
	for _, field := range obj.Fields {
		fieldName := l.cacheFieldName(field)
		fieldValue := entity.Get(fieldName)
		if fieldValue != nil {
			if nested, ok := field.Value.(*Object); ok {
				l.materializeNullableFieldsAsNull(a, fieldValue, nested)
			}
			continue
		}
		if field.Value.NodeNullable() {
			entity.Set(a, fieldName, astjson.NullValue)
		}
	}
}

// cacheKeyHasPositiveEntityData reports whether either cached or fresh payload already contains
// fields beyond the entity key itself, making it safe to preserve an object shape for negative caching.
func cacheKeyHasPositiveEntityData(ck *CacheKey) bool {
	if ck == nil {
		return false
	}
	return entityValueHasNonKeyFields(ck.FromCache, ck) || entityValueHasNonKeyFields(ck.Item, ck)
}

func entityValueHasNonKeyFields(value *astjson.Value, ck *CacheKey) bool {
	if value == nil {
		return false
	}
	entity := value
	if len(ck.EntityMergePath) > 0 {
		entity = value.Get(ck.EntityMergePath...)
	}
	if entity == nil || entity.Type() != astjson.TypeObject {
		return false
	}
	allowed := allowedEntityKeyFields(ck.Keys)
	entityObject := map[string]json.RawMessage{}
	if err := json.Unmarshal(entity.MarshalTo(nil), &entityObject); err != nil {
		return false
	}
	for fieldName := range entityObject {
		if _, ok := allowed[fieldName]; !ok {
			return true
		}
	}
	return false
}

func allowedEntityKeyFields(keys []string) map[string]struct{} {
	allowed := map[string]struct{}{
		"__typename": {},
	}
	if len(keys) == 0 {
		return allowed
	}
	entityKey := keys[0]
	start := strings.IndexByte(entityKey, '{')
	if start == -1 {
		return allowed
	}
	var decoded struct {
		Key map[string]json.RawMessage `json:"key"`
	}
	if err := json.Unmarshal([]byte(entityKey[start:]), &decoded); err != nil {
		return allowed
	}
	for fieldName := range decoded.Key {
		allowed[fieldName] = struct{}{}
	}
	return allowed
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
	isEntity := cfg.isEntityFetch()

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
			// Calculate prefix for L2 (global prefix + subgraph header isolation)
			var prefix string
			globalPrefix := l.ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix
			if cfg.IncludeSubgraphHeaderPrefix && l.ctx.SubgraphHeadersBuilder != nil {
				_, headersHash := l.ctx.SubgraphHeadersBuilder.HeadersForSubgraph(info.DataSourceName)
				var buf [20]byte
				b := strconv.AppendUint(buf[:0], headersHash, 10)
				if globalPrefix != "" {
					prefix = globalPrefix + ":" + string(b)
				} else {
					prefix = string(b)
				}
				res.headerHash = headersHash
				// Record that header partitioning is active so the WRITE path
				// (rootFieldL2CachePrefix) can build the same prefix even when
				// headersHash == 0 (no headers forwarded but partitioning is on).
				res.includeHeaderPrefix = true
			} else if globalPrefix != "" {
				prefix = globalPrefix
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

	if cfg.hasBatchEntityKey() {
		cacheKeys := res.l1CacheKeys
		if len(cacheKeys) == 0 {
			cacheKeys = res.l2CacheKeys
		}
		if len(cacheKeys) == 0 || (len(cacheKeys) > 0 && cacheKeys[0] != nil && cacheKeys[0].Item == nil) {
			res.batchEntityKeyMode = true
			res.batchMergePath = res.postProcessing.MergePath
			if cfg.PartialBatchLoad && !cfg.ShadowMode {
				res.batchPartialFetchEnabled = true
			}
		}
	}

	// When root field uses entity key mapping, set EntityMergePath so that
	// store/load can extract/wrap entity-level data at the merge path.
	if entityPath := cfg.entityMergePath(res.postProcessing); len(entityPath) > 0 {
		// Determine the path to extract entity data from the merged response.
		// If MergePath is set (e.g. ["user"]), use it directly.
		// Otherwise, the entity data is nested under the root field name in the response
		// (e.g. for field "user", response is {"user":{...}} and entity data is at ["user"]).
		for _, ck := range res.l1CacheKeys {
			ck.EntityMergePath = entityPath
		}
		for _, ck := range res.l2CacheKeys {
			ck.EntityMergePath = entityPath
		}
	}

	// Transform construction is now ephemeral — built and consumed
	// inline at each cache operation site via structuralCopyNormalized /
	// structuralCopyDenormalized. No need to pre-build and store on res.

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
	tracingCache := l.ctx.TracingOptions.Enable && !l.ctx.TracingOptions.ExcludeCacheStats
	if tracingCache {
		res.cacheTraceDurationSinceStartNano = GetDurationNanoSinceTraceStart(l.ctx.ctx)
		defer func() {
			res.cacheTraceDurationNano = GetDurationNanoSinceTraceStart(l.ctx.ctx) - res.cacheTraceDurationSinceStartNano
		}()
	}

	// Step 1: Prepare cache keys for L1 and L2
	isEntityFetch, err := l.prepareCacheKeys(info, cfg, inputItems, res)
	if err != nil {
		return false, err
	}

	// Set entity count from cache keys
	if len(res.l2CacheKeys) > 0 {
		for _, ck := range res.l2CacheKeys {
			res.cacheTraceEntityCount += len(ck.Keys)
		}
	} else if len(res.l1CacheKeys) > 0 {
		for _, ck := range res.l1CacheKeys {
			res.cacheTraceEntityCount += len(ck.Keys)
		}
	}

	// No cache keys generated - nothing to do
	if len(res.l1CacheKeys) == 0 && len(res.l2CacheKeys) == 0 {
		if res.batchEntityKeyMode {
			res.cacheSkipFetch = true
			return true, nil
		}
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

		if res.batchPartialFetchEnabled && len(res.batchCachedIndices) > 0 {
			// Batch partial hit: some entities cached, some need fetching
			// Keep FromCache values, return false to proceed with fetch for missing IDs
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

	tracingCache := l.ctx.TracingOptions.Enable && !l.ctx.TracingOptions.ExcludeCacheStats

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
			if cachedValue, ok := l.l1Cache[keyStr]; ok {
				if cachedValue == nil {
					continue
				}
				// Widening check operates on the stored cache pointer directly (read-only).
				if info.ProvidesData != nil && !l.validateItemHasRequiredData(cachedValue, info.ProvidesData) {
					continue
				}
				// L1 READ: structural copy with denormalize passthrough (schema→alias).
				// L1 stores schema-shape names with all fields (passthrough write).
				// Denormalize renames known fields back to aliases while keeping
				// unlisted fields intact — they may be needed by later fetches.
				ck.FromCache = l.structuralCopyDenormalizedPassthrough(cachedValue, res.providesData)

				analyticsEnabled := l.ctx.cacheAnalyticsEnabled()
				var byteSize int
				if analyticsEnabled || tracingCache {
					byteSize = len(cachedValue.MarshalTo(nil))
				}
				if analyticsEnabled {
					l.ctx.cacheAnalytics.RecordL1KeyEvent(CacheKeyHit, entityType, keyStr, dataSource, byteSize)
					if len(res.cacheConfig.KeyFields) > 0 {
						keyJSON := buildEntityKeyJSON(cachedValue, res.cacheConfig.KeyFields)
						if len(keyJSON) > 0 {
							l.ctx.cacheAnalytics.RecordEntitySource(entityType, string(keyJSON), FieldSourceL1)
						}
					}
				}
				if tracingCache {
					res.cacheTraceL1Hits++
					if !l.ctx.TracingOptions.ExcludeRawInputData && len(ck.Keys) > 0 {
						res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, CacheTraceEntity{
							Key:      ck.Keys[0],
							Source:   "l1",
							ByteSize: byteSize,
						})
					}
				}
				foundComplete = true
				break
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
			if tracingCache {
				res.cacheTraceL1Misses++
			}
			// Track fetch item index when partial loading enabled
			if res.partialCacheEnabled {
				res.fetchItemIndices = append(res.fetchItemIndices, i)
			}
		}
	}
	return allComplete
}

type l2CacheLookupState struct {
	analyticsEnabled        bool
	tracingCache            bool
	shadowMode              bool
	hasAliases              bool
	entityType              string
	dataSource              string
	remainingTTLs           map[string]time.Duration
	batchEntityProvidesData *Object
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

	tracingCache := l.ctx.TracingOptions.Enable && !l.ctx.TracingOptions.ExcludeCacheStats

	cacheKeyStrings := l.extractCacheKeysStrings(l.jsonArena, res.l2CacheKeys)
	// Skip the L2 round-trip when there's nothing to look up.
	// The empty-slice case is the "no keys wired up" path; the all-empty-string case
	// guards against CacheKey entries that never got rendered (e.g., a template missed
	// a required variable). Either way, sending empty keys to the backend is at best
	// a wasted round-trip and at worst interpreted by a backend as a request for an
	// entry keyed by "" — skip cleanly instead.
	if len(cacheKeyStrings) == 0 || !hasNonEmptyKey(cacheKeyStrings) {
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

	// Get cache entries from L2
	var l2GetStart time.Time
	if analyticsEnabled || tracingCache {
		l2GetStart = time.Now()
	}
	if tracingCache {
		res.cacheTraceL2GetAttempted = true
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
	if tracingCache {
		res.cacheTraceL2GetDuration = time.Since(l2GetStart)
	}
	if err != nil {
		// L2 cache errors are non-fatal, continue to fetch.
		// Circuit-breaker-open is not a backend error — skip analytics/trace error recording.
		if !errors.Is(err, ErrCircuitBreakerOpen) {
			if analyticsEnabled {
				res.l2CacheOpErrors = append(res.l2CacheOpErrors, CacheOperationError{
					Operation:  "get",
					CacheName:  res.cacheConfig.CacheName,
					EntityType: entityType,
					DataSource: dataSource,
					Message:    truncateErrorMessage(err.Error(), 256),
					ItemCount:  len(cacheKeyStrings),
				})
			}
			if tracingCache {
				res.cacheTraceL2GetError = err.Error()
			}
		}
		res.cacheMustBeUpdated = true
		return false, nil
	}

	// Populate FromCache fields in L2 CacheKeys (which have prefixed keys)
	err = l.populateFromCache(l.jsonArena, res.l2CacheKeys, cacheEntries)
	if err != nil {
		res.cacheMustBeUpdated = true
		return false, nil
	}

	state := l.prepareL2LookupState(info, res, cacheEntries, analyticsEnabled, tracingCache, entityType, dataSource)

	// Copy FromCache values from L2 keys to L1 keys (if L1 keys exist) and track per-entity hits/misses
	// The keys have the same structure, just different key strings.
	var allComplete bool
	if len(res.l1CacheKeys) > 0 && !res.batchEntityKeyMode {
		allComplete = l.applyEntityFetchL2Results(info, res, state)
	} else {
		allComplete = l.applyRootFetchL2Results(info, res, state)
	}

	// Shadow mode: even if all items were found in cache, we still need to fetch
	// fresh data for comparison. Clear FromCache and force fetch.
	if state.shadowMode {
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
		if hasMissingRequestedKeys(res.l2CacheKeys) || needsResolvedCacheWriteback(res.l2CacheKeys) {
			res.cacheMustBeUpdated = true
		}
		return true, nil
	}

	res.cacheMustBeUpdated = true
	return false, nil
}

// bulkL2Lookup performs the L2 cache read for a parallel batch of fetches in
// a single bulk cache.Get per cache instance, on the main thread, using
// l.parser and l.jsonArena. After this call, every result in `results` has
// res.cacheSkipFetch set correctly and the L2 analytics events accumulated.
//
// Skipped per result:
//   - res.cache == nil (no L2 enabled for this fetch)
//   - res.fetchSkipped (Phase 1.5 already satisfied via @requestScoped)
//   - res.cacheSkipFetch (L1 was a complete hit in Phase 1)
//   - mutation root operation (l.info.OperationType == ast.OperationTypeMutation)
//
// Behavior on bulk Get failure: every fetch that requested the failing cache
// instance gets res.cacheMustBeUpdated = true and proceeds to subgraph fetch.
func (l *Loader) bulkL2Lookup(ctx context.Context, nodes []*FetchTreeNode, results []*result) error {
	if len(results) == 0 {
		return nil
	}
	if l.info != nil && l.info.OperationType == ast.OperationTypeMutation {
		// Mutations skip L2 reads (existing behavior, see tryL2CacheLoad).
		for _, res := range results {
			if res != nil {
				res.cacheMustBeUpdated = true
			}
		}
		return nil
	}

	// Phase A: build per-cache-instance plans.
	type planEntry struct {
		cache  LoaderCache
		keys   []string         // deduplicated, deterministic order
		owners map[string][]int // key -> list of fetch indices that requested it
	}
	plans := make(map[LoaderCache]*planEntry)

	for i, res := range results {
		if res == nil || res.cache == nil {
			continue
		}
		if res.fetchSkipped || res.cacheSkipFetch {
			continue
		}
		if len(res.l2CacheKeys) == 0 {
			res.cacheMustBeUpdated = true
			continue
		}
		plan, ok := plans[res.cache]
		if !ok {
			plan = &planEntry{cache: res.cache, owners: make(map[string][]int)}
			plans[res.cache] = plan
		}
		for _, ck := range res.l2CacheKeys {
			for _, key := range ck.Keys {
				if _, seen := plan.owners[key]; !seen {
					plan.keys = append(plan.keys, key)
				}
				plan.owners[key] = append(plan.owners[key], i)
			}
		}
	}
	if len(plans) == 0 {
		return nil
	}

	type indexedEntries struct {
		byKey map[string]*CacheEntry
	}
	indexes := make(map[LoaderCache]indexedEntries, len(plans))
	tracingCache := l.ctx.TracingOptions.Enable && !l.ctx.TracingOptions.ExcludeCacheStats
	analyticsEnabled := l.ctx.cacheAnalyticsEnabled()

	for _, plan := range plans {
		// Pre-compute unique fetch indices for this plan.
		seenFetchIdx := make(map[int]struct{}, 8)
		for _, fetchIndices := range plan.owners {
			for _, i := range fetchIndices {
				seenFetchIdx[i] = struct{}{}
			}
		}

		if tracingCache {
			for i := range seenFetchIdx {
				if i >= 0 && i < len(results) && results[i] != nil {
					results[i].cacheTraceL2GetAttempted = true
				}
			}
		}
		var bulkGetStart time.Time
		if analyticsEnabled || tracingCache {
			bulkGetStart = time.Now()
		}

		entries, err := plan.cache.Get(ctx, plan.keys)

		var bulkGetDuration time.Duration
		if analyticsEnabled || tracingCache {
			bulkGetDuration = time.Since(bulkGetStart)
		}

		// Attribute timing per-fetch.
		if tracingCache {
			for i := range seenFetchIdx {
				if i >= 0 && i < len(results) && results[i] != nil {
					results[i].cacheTraceL2GetDuration = bulkGetDuration
				}
			}
		}
		if analyticsEnabled {
			for i := range seenFetchIdx {
				if i < 0 || i >= len(results) || results[i] == nil {
					continue
				}
				res := results[i]
				perFetchKeyCount := countUniqueCacheKeyStrings(res.l2CacheKeys)
				res.l2FetchTimings = append(res.l2FetchTimings, FetchTimingEvent{
					DataSource:    res.ds.Name,
					EntityType:    res.analyticsEntityType,
					DurationMs:    bulkGetDuration.Milliseconds(),
					Source:        FieldSourceL2,
					ItemCount:     perFetchKeyCount,
					IsEntityFetch: len(res.l1CacheKeys) > 0,
				})
			}
		}

		if err != nil {
			// Circuit-breaker-open is not a backend error — treat as a clean miss.
			breakerOpen := errors.Is(err, ErrCircuitBreakerOpen)
			for i := range seenFetchIdx {
				if i < 0 || i >= len(results) || results[i] == nil {
					continue
				}
				res := results[i]
				res.cacheMustBeUpdated = true
				if breakerOpen {
					continue
				}
				if tracingCache {
					res.cacheTraceL2GetError = err.Error()
				}
				if analyticsEnabled {
					perFetchKeyCount := countUniqueCacheKeyStrings(res.l2CacheKeys)
					res.l2CacheOpErrors = append(res.l2CacheOpErrors, CacheOperationError{
						Operation:  "get",
						CacheName:  res.cacheConfig.CacheName,
						EntityType: res.analyticsEntityType,
						DataSource: res.ds.Name,
						Message:    truncateErrorMessage(err.Error(), 256),
						ItemCount:  perFetchKeyCount,
					})
				}
			}
			continue
		}
		idx := indexedEntries{byKey: make(map[string]*CacheEntry, len(entries))}
		for _, e := range entries {
			if e != nil {
				idx.byKey[e.Key] = e
			}
		}
		indexes[plan.cache] = idx
	}

	// Phase C: per-fetch — populate FromCache, parse VERBATIM on l.parser/l.jsonArena.
	for i, res := range results {
		if res == nil || res.cache == nil {
			continue
		}
		if res.fetchSkipped || res.cacheSkipFetch {
			continue
		}
		if len(res.l2CacheKeys) == 0 {
			continue
		}
		idx, ok := indexes[res.cache]
		if !ok {
			// Get failed earlier — already marked cacheMustBeUpdated above.
			continue
		}

		info := res.fetchInfo

		if err := l.populateFromCacheBulk(l.jsonArena, res, idx.byKey); err != nil {
			res.cacheMustBeUpdated = true
			continue
		}

		state := l.prepareL2LookupState(info, res, nil, analyticsEnabled, tracingCache, res.analyticsEntityType, res.ds.Name)

		var allComplete bool
		if len(res.l1CacheKeys) > 0 && !res.batchEntityKeyMode {
			allComplete = l.applyEntityFetchL2Results(info, res, state)
		} else {
			allComplete = l.applyRootFetchL2Results(info, res, state)
		}

		if state.shadowMode {
			for _, ck := range res.l1CacheKeys {
				ck.FromCache = nil
			}
			res.cachedItemIndices = nil
			res.fetchItemIndices = nil
			res.cacheSkipFetch = false
			res.cacheMustBeUpdated = true
			continue
		}

		if allComplete {
			res.cacheSkipFetch = true
			// Attach cached output to trace — previously done in loadFetchL2Only.
			if i >= 0 && i < len(nodes) && nodes[i] != nil && nodes[i].Item != nil {
				l.attachCachedOutputToTrace(nodes[i].Item.Fetch, res)
			}
			if hasMissingRequestedKeys(res.l2CacheKeys) || needsResolvedCacheWriteback(res.l2CacheKeys) {
				res.cacheMustBeUpdated = true
			}
			continue
		}
		res.cacheMustBeUpdated = true
	}

	return nil
}

// populateFromCacheBulk fills cacheKeys[].FromCache / fromCacheCandidates /
// missingKeys from a pre-indexed map of cache entries. Parses each candidate
// VERBATIM (no Transform) onto the given arena via l.parser.
func (l *Loader) populateFromCacheBulk(a arena.Arena, res *result, byKey map[string]*CacheEntry) error {
	return l.populateCacheKeysFromIndex(a, res.l2CacheKeys, byKey)
}

func (l *Loader) prepareL2LookupState(info *FetchInfo, res *result, cacheEntries []*CacheEntry, analyticsEnabled, tracingCache bool, entityType, dataSource string) l2CacheLookupState {
	state := l2CacheLookupState{
		analyticsEnabled: analyticsEnabled,
		tracingCache:     tracingCache,
		shadowMode:       res.cacheConfig.ShadowMode,
		hasAliases:       info != nil && info.ProvidesData != nil && info.ProvidesData.HasAliases,
		entityType:       entityType,
		dataSource:       dataSource,
	}

	if res.batchEntityKeyMode && len(res.l2CacheKeys) > 0 {
		state.batchEntityProvidesData = batchEntityValidationObject(info.ProvidesData, res.l2CacheKeys[0].EntityMergePath)
	}

	// When EntityMergePath is set, the cache stores entity-level data (e.g. {"id":"1234","username":"Me"}).
	// Root field fetches need response-level data (e.g. {"user":{"id":"1234","username":"Me"}}),
	// so wrap the cached entity data back at the merge path before validation.
	// Batch entity key lookups keep entity-level values because each cache entry represents
	// one array element rather than a complete root field response.
	if !res.batchEntityKeyMode {
		for _, ck := range res.l2CacheKeys {
			if len(ck.EntityMergePath) > 0 && ck.FromCache != nil {
				ck.FromCache = wrapCacheValueAtMergePath(l.jsonArena, ck.FromCache, ck.EntityMergePath)
			}
		}
	}

	if analyticsEnabled {
		if cacheEntries != nil {
			// Sequential path: build from the raw entries returned by tryL2CacheLoad.
			state.remainingTTLs = make(map[string]time.Duration, len(cacheEntries))
			for _, entry := range cacheEntries {
				if entry != nil && entry.RemainingTTL > 0 {
					state.remainingTTLs[entry.Key] = entry.RemainingTTL
				}
			}
		} else {
			// Bulk path: derive from the freshest candidate already attached to
			// each CacheKey by populateFromCacheBulk.
			state.remainingTTLs = make(map[string]time.Duration, len(res.l2CacheKeys))
			for _, ck := range res.l2CacheKeys {
				if ck == nil || ck.fromCacheRemainingTTL <= 0 || len(ck.Keys) == 0 {
					continue
				}
				state.remainingTTLs[ck.Keys[0]] = ck.fromCacheRemainingTTL
			}
		}
	}

	return state
}

// selectBestCacheCandidate decides whether the freshest candidate already
// attached to ck.FromCache is usable as a full hit (true) or must be treated
// as a partial hit (false). When ProvidesData is absent the check is a no-op
// and the value is accepted as-is. When ProvidesData is present the multi-
// candidate walk runs, which may swap ck.FromCache to an older candidate that
// covers the required fields.
func (l *Loader) selectBestCacheCandidate(info *FetchInfo, ck *CacheKey) bool {
	if info == nil || info.ProvidesData == nil {
		return true
	}
	return l.resolveMultiCandidateCacheValue(l.jsonArena, ck, info.ProvidesData)
}

func (l *Loader) applyEntityFetchL2Results(info *FetchInfo, res *result, state l2CacheLookupState) bool {
	allComplete := true

	for i := range res.l1CacheKeys {
		if i >= len(res.l2CacheKeys) {
			continue
		}

		res.l1CacheKeys[i].FromCache = res.l2CacheKeys[i].FromCache
		res.l1CacheKeys[i].missingKeys = res.l2CacheKeys[i].missingKeys
		res.l1CacheKeys[i].cachedData = res.l2CacheKeys[i].cachedData

		if res.l1CacheKeys[i].FromCache == nil {
			if state.analyticsEnabled && len(res.l1CacheKeys[i].Keys) > 0 {
				res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
					CacheKey: res.l1CacheKeys[i].Keys[0], EntityType: state.entityType,
					Kind: CacheKeyMiss, DataSource: state.dataSource, ByteSize: 0,
					Shadow: state.shadowMode,
				})
			}
			if state.tracingCache {
				res.cacheTraceL2Misses++
			}
			allComplete = false
			if res.partialCacheEnabled {
				res.fetchItemIndices = append(res.fetchItemIndices, i)
			}
			continue
		}

		if res.l1CacheKeys[i].FromCache.Type() == astjson.TypeNull && res.cacheConfig.NegativeCacheTTL > 0 {
			if state.analyticsEnabled && len(res.l1CacheKeys[i].Keys) > 0 {
				res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
					CacheKey: res.l1CacheKeys[i].Keys[0], EntityType: state.entityType,
					Kind: CacheKeyHit, DataSource: state.dataSource, ByteSize: 4,
					Shadow: state.shadowMode,
				})
			}
			if state.tracingCache {
				res.cacheTraceNegativeHits++
				if !l.ctx.TracingOptions.ExcludeRawInputData && len(res.l1CacheKeys[i].Keys) > 0 {
					res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, CacheTraceEntity{
						Key:    res.l1CacheKeys[i].Keys[0],
						Source: "negative_cache",
					})
				}
			}
			if res.partialCacheEnabled {
				res.cachedItemIndices = append(res.cachedItemIndices, i)
			}
			continue
		}

		if !l.selectBestCacheCandidate(info, res.l1CacheKeys[i]) {
			res.l2CacheKeys[i].FromCache = res.l1CacheKeys[i].FromCache
			res.l2CacheKeys[i].cachedData = res.l1CacheKeys[i].cachedData
			if state.analyticsEnabled && len(res.l1CacheKeys[i].Keys) > 0 {
				res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
					CacheKey: res.l1CacheKeys[i].Keys[0], EntityType: state.entityType,
					Kind: CacheKeyPartialHit, DataSource: state.dataSource, ByteSize: 0,
					Shadow: state.shadowMode,
				})
			}
			allComplete = false
			if res.partialCacheEnabled {
				res.fetchItemIndices = append(res.fetchItemIndices, i)
			}
			continue
		}

		res.l2CacheKeys[i].FromCache = res.l1CacheKeys[i].FromCache
		res.l2CacheKeys[i].fromCacheRemainingTTL = res.l1CacheKeys[i].fromCacheRemainingTTL
		res.l2CacheKeys[i].fromCacheNeedsWriteback = res.l1CacheKeys[i].fromCacheNeedsWriteback

		if state.hasAliases {
			res.l1CacheKeys[i].FromCache = l.structuralCopyDenormalizedPassthrough(res.l1CacheKeys[i].FromCache, res.providesData)
		}

		var byteSize int
		if (state.analyticsEnabled || state.tracingCache) && len(res.l1CacheKeys[i].Keys) > 0 {
			byteSize = len(res.l1CacheKeys[i].FromCache.MarshalTo(nil))
		}

		if state.analyticsEnabled && len(res.l1CacheKeys[i].Keys) > 0 {
			var cacheAgeMs int64
			if len(res.l2CacheKeys[i].Keys) > 0 {
				cacheAgeMs = computeCacheAgeMs(state.remainingTTLs[res.l2CacheKeys[i].Keys[0]], res.cacheConfig.TTL)
			}
			res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
				CacheKey: res.l1CacheKeys[i].Keys[0], EntityType: state.entityType,
				Kind: CacheKeyHit, DataSource: state.dataSource, ByteSize: byteSize,
				CacheAgeMs: cacheAgeMs, Shadow: state.shadowMode,
			})
			if len(res.cacheConfig.KeyFields) > 0 {
				keyJSON := buildEntityKeyJSON(res.l1CacheKeys[i].FromCache, res.cacheConfig.KeyFields)
				if len(keyJSON) > 0 {
					res.l2EntitySources = append(res.l2EntitySources, entitySourceRecord{
						entityType: state.entityType, keyJSON: string(keyJSON), source: FieldSourceL2,
					})
				}
			}
		}

		if state.shadowMode {
			var remaining time.Duration
			if len(res.l2CacheKeys[i].Keys) > 0 {
				remaining = state.remainingTTLs[res.l2CacheKeys[i].Keys[0]]
			}
			l.saveShadowCachedValue(res, i, res.l1CacheKeys[i].FromCache, res.l1CacheKeys[i].Keys[0], remaining)
			if state.tracingCache {
				res.cacheTraceShadowHit = true
			}
		}

		if state.tracingCache {
			res.cacheTraceL2Hits++
			if !l.ctx.TracingOptions.ExcludeRawInputData && len(res.l1CacheKeys[i].Keys) > 0 {
				entity := CacheTraceEntity{
					Key:      res.l1CacheKeys[i].Keys[0],
					Source:   "l2",
					ByteSize: byteSize,
				}
				if res.l2CacheKeys[i].fromCacheRemainingTTL > 0 {
					entity.RemainingTTLSeconds = res.l2CacheKeys[i].fromCacheRemainingTTL.Seconds()
				}
				res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, entity)
			}
		}

		if res.partialCacheEnabled {
			res.cachedItemIndices = append(res.cachedItemIndices, i)
		}
	}

	return allComplete
}

func (l *Loader) applyRootFetchL2Results(info *FetchInfo, res *result, state l2CacheLookupState) bool {
	allComplete := true

	for i, ck := range res.l2CacheKeys {
		if ck.FromCache == nil {
			if state.analyticsEnabled && len(ck.Keys) > 0 {
				res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
					CacheKey: ck.Keys[0], EntityType: state.entityType,
					Kind: CacheKeyMiss, DataSource: state.dataSource, ByteSize: 0,
					Shadow: state.shadowMode,
				})
			}
			if state.tracingCache {
				res.cacheTraceL2Misses++
			}
			allComplete = false
			if res.partialCacheEnabled {
				res.fetchItemIndices = append(res.fetchItemIndices, i)
			}
			if res.batchPartialFetchEnabled {
				res.batchMissedIndices = append(res.batchMissedIndices, ck.BatchIndex)
			}
			continue
		}

		if ck.FromCache.Type() == astjson.TypeNull && res.cacheConfig.NegativeCacheTTL > 0 {
			if state.analyticsEnabled && len(ck.Keys) > 0 {
				res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
					CacheKey: ck.Keys[0], EntityType: state.entityType,
					Kind: CacheKeyHit, DataSource: state.dataSource, ByteSize: 4,
					Shadow: state.shadowMode,
				})
			}
			if state.tracingCache {
				res.cacheTraceNegativeHits++
				if !l.ctx.TracingOptions.ExcludeRawInputData && len(ck.Keys) > 0 {
					res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, CacheTraceEntity{
						Key:    ck.Keys[0],
						Source: "negative_cache",
					})
				}
			}
			if res.partialCacheEnabled {
				res.cachedItemIndices = append(res.cachedItemIndices, i)
			}
			if res.batchPartialFetchEnabled {
				res.batchCachedIndices = append(res.batchCachedIndices, ck.BatchIndex)
			}
			continue
		}

		providesDataForValidation := info != nil && info.ProvidesData != nil
		cacheHit := !providesDataForValidation || l.resolveMultiCandidateCacheValue(l.jsonArena, ck, info.ProvidesData)
		if res.batchEntityKeyMode {
			cacheHit = state.batchEntityProvidesData == nil || l.resolveBatchEntityCacheValue(l.jsonArena, ck, state.batchEntityProvidesData)
		}
		if !cacheHit {
			if state.analyticsEnabled && len(ck.Keys) > 0 {
				res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
					CacheKey: ck.Keys[0], EntityType: state.entityType,
					Kind: CacheKeyPartialHit, DataSource: state.dataSource, ByteSize: 0,
					Shadow: state.shadowMode,
				})
			}
			allComplete = false
			if res.partialCacheEnabled {
				res.fetchItemIndices = append(res.fetchItemIndices, i)
			}
			if res.batchPartialFetchEnabled {
				res.batchMissedIndices = append(res.batchMissedIndices, ck.BatchIndex)
			}
			continue
		}

		if state.hasAliases {
			if res.batchEntityKeyMode && state.batchEntityProvidesData != nil {
				res.l2CacheKeys[i].FromCache = l.structuralCopyDenormalized(ck.FromCache, state.batchEntityProvidesData)
			} else {
				res.l2CacheKeys[i].FromCache = l.structuralCopyDenormalized(ck.FromCache, res.providesData)
			}
		}

		var byteSize int
		if (state.analyticsEnabled || state.tracingCache) && len(ck.Keys) > 0 {
			byteSize = len(res.l2CacheKeys[i].FromCache.MarshalTo(nil))
		}

		if state.analyticsEnabled && len(ck.Keys) > 0 {
			cacheAgeMs := computeCacheAgeMs(state.remainingTTLs[ck.Keys[0]], res.cacheConfig.TTL)
			res.l2AnalyticsEvents = append(res.l2AnalyticsEvents, CacheKeyEvent{
				CacheKey: ck.Keys[0], EntityType: state.entityType,
				Kind: CacheKeyHit, DataSource: state.dataSource, ByteSize: byteSize,
				CacheAgeMs: cacheAgeMs, Shadow: state.shadowMode,
			})
			if len(res.cacheConfig.KeyFields) > 0 {
				walkCachedResponseForSources(res.l2CacheKeys[i].FromCache, res.cacheConfig.KeyFields, state.entityType, FieldSourceL2, &res.l2EntitySources)
			}
		}

		if state.tracingCache {
			res.cacheTraceL2Hits++
			if !l.ctx.TracingOptions.ExcludeRawInputData && len(ck.Keys) > 0 {
				entity := CacheTraceEntity{
					Key:      ck.Keys[0],
					Source:   "l2",
					ByteSize: byteSize,
				}
				if ck.fromCacheRemainingTTL > 0 {
					entity.RemainingTTLSeconds = ck.fromCacheRemainingTTL.Seconds()
				}
				res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, entity)
			}
		}

		if res.partialCacheEnabled {
			res.cachedItemIndices = append(res.cachedItemIndices, i)
		}
		if res.batchPartialFetchEnabled {
			res.batchCachedIndices = append(res.batchCachedIndices, ck.BatchIndex)
		}
	}

	return allComplete
}

// populateL1Cache stores entity data in the L1 (per-request) cache for later reuse.
// Always DeepCopies onto l.jsonArena so the stored value is independent of the
// source tree. When there are aliases / arg-suffix fields, uses the per-fetch
// normalize Transform to produce a cache-shape (schema-named) value upfront.
func (l *Loader) populateL1Cache(fetchItem *FetchItem, res *result) {
	if !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return
	}
	cfg := getFetchCaching(fetchItem.Fetch)
	if !cfg.UseL1Cache {
		l.populateL1CacheForRootFieldEntities(fetchItem)
		return
	}

	info := getFetchInfo(fetchItem.Fetch)
	var entityType, dataSource string
	if l.ctx.cacheAnalyticsEnabled() && info != nil {
		if len(info.RootFields) > 0 {
			entityType = info.RootFields[0].TypeName
		}
		dataSource = info.DataSourceName
	}

	analyticsEnabled := l.ctx.cacheAnalyticsEnabled()

	for _, ck := range res.l1CacheKeys {
		if ck.Item == nil {
			continue
		}
		// L1 WRITE: structural copy with rename but no projection.
		// L1 stores the complete entity (all fields, schema-shape names)
		// so subsequent fetches can merge additional fields into it.
		// Passthrough mode renames aliased fields to schema names while
		// keeping unlisted fields (e.g. @key fields) intact.
		stored := l.structuralCopyNormalizedPassthrough(ck.Item, res.providesData)
		if stored == nil {
			continue
		}

		for _, keyStr := range ck.Keys {
			byteSize := l1AnalyticsSize(analyticsEnabled, stored)
			if existingVal, loaded := l.l1Cache[keyStr]; loaded && existingVal != nil {
				// SAFETY: merge into a working copy, never the live cache
				// entry. astjson.MergeValues mutates its first argument in
				// place and failures are NOT atomic (verified at
				// astjson/mergevalues.go:30–74). Merging in place could
				// corrupt every sibling L1 key pointing at the same entry.
				working := l.parser.StructuralCopy(l.jsonArena, existingVal)
				_, err := astjson.MergeValues(l.jsonArena, working, stored)
				if err != nil {
					l.l1Cache[keyStr] = stored
				} else {
					l.l1Cache[keyStr] = working
					byteSize = l1AnalyticsSize(analyticsEnabled, working)
				}
			} else {
				l.l1Cache[keyStr] = stored
			}
			if analyticsEnabled {
				l.ctx.cacheAnalytics.RecordWrite(CacheWriteEvent{
					CacheKey: keyStr, EntityType: entityType, ByteSize: byteSize,
					DataSource: dataSource, CacheLevel: CacheLevelL1, Source: l.cacheOperationSource(),
				})
			}
		}
	}
	l.populateL1CacheForRootFieldEntities(fetchItem)
}

// populateL1CacheForRootFieldEntities populates the L1 cache with entities returned by root fields.
// This allows subsequent entity fetches to benefit from L1 cache hits when the same entities
// were already fetched as part of a root field query.
//
// Root-field L1 promotion requires planner ProvidesData in order to derive the
// entity-shaped Object and build a normalize Transform. When ProvidesData is
// unavailable, promotion is silently skipped rather than storing response-shape
// (aliased) values, which would corrupt subsequent entity-fetch L1 reads.
// rootFieldL1PathGroup collects all entity-type templates that share a response
// field path, so the Transform and entity-Object can be derived once per group.
type rootFieldL1PathGroup struct {
	fieldPath []string
	// entityType → template
	templates map[string]*EntityQueryCacheKeyTemplate
}

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

	// Fetch-level guard: ProvidesData is required for normalize-on-write.
	if singleFetch.Info == nil || singleFetch.Info.ProvidesData == nil {
		return
	}

	// Get response data
	data := l.resolvable.data
	if data == nil {
		return
	}

	groups := groupRootFieldL1Templates(templates)

	l.processNestedL1Items(singleFetch, data, groups)
}

// groupRootFieldL1Templates buckets the per-composite-key templates by the
// response field path their entity Object is rooted at, so the Transform and
// entity-shape Object can be derived once per path instead of once per key.
func groupRootFieldL1Templates(templates map[string]CacheKeyTemplate) map[string]*rootFieldL1PathGroup {
	groups := map[string]*rootFieldL1PathGroup{} // keyed by joined fieldPath

	for compositeKey, template := range templates {
		entityTemplate, ok := template.(*EntityQueryCacheKeyTemplate)
		if !ok || entityTemplate.Keys == nil || entityTemplate.Keys.Renderer == nil {
			continue
		}
		obj, ok := entityTemplate.Keys.Renderer.Node.(*Object)
		if !ok || len(obj.Path) == 0 {
			continue
		}

		// Extract entity type from composite key "fieldName:entityType"
		_, entityType, ok := strings.Cut(compositeKey, ":")
		if !ok {
			entityType = compositeKey
		}

		pathKey := strings.Join(obj.Path, "/")
		g, exists := groups[pathKey]
		if !exists {
			g = &rootFieldL1PathGroup{
				fieldPath: obj.Path,
				templates: map[string]*EntityQueryCacheKeyTemplate{},
			}
			groups[pathKey] = g
		}
		g.templates[entityType] = entityTemplate
	}

	return groups
}

// processNestedL1Items walks each path group, resolves the entity-shape Object
// from the fetch's ProvidesData once per group, then delegates to storeL1Entity
// for each individual entity discovered under that path in the response data.
func (l *Loader) processNestedL1Items(singleFetch *SingleFetch, data *astjson.Value, groups map[string]*rootFieldL1PathGroup) {
	for _, g := range groups {
		entityObj := batchEntityValidationObject(singleFetch.Info.ProvidesData, g.fieldPath)
		if entityObj == nil {
			continue
		}
		entitiesValue := data.Get(g.fieldPath...)
		if entitiesValue == nil {
			continue
		}

		var entities []*astjson.Value
		switch entitiesValue.Type() {
		case astjson.TypeArray:
			entities = entitiesValue.GetArray()
		case astjson.TypeObject:
			entities = []*astjson.Value{entitiesValue}
		default:
			continue
		}

		for _, entity := range entities {
			l.storeL1Entity(entity, entityObj, g.templates)
		}
	}
}

// storeL1Entity renders the cache keys for a single response entity and
// performs the first-writer-wins L1 write. Skips entities that are nil, lack a
// __typename, have no matching template, or fail normalization/rendering.
func (l *Loader) storeL1Entity(entity *astjson.Value, entityObj *Object, templatesByType map[string]*EntityQueryCacheKeyTemplate) {
	if entity == nil {
		return
	}
	typenameValue := entity.Get("__typename")
	if typenameValue == nil {
		return
	}
	entityTemplate, ok := templatesByType[string(typenameValue.GetStringBytes())]
	if !ok {
		return
	}

	// L1 WRITE: structural copy with rename but no projection.
	stored := l.structuralCopyNormalizedPassthrough(entity, entityObj)
	if stored == nil {
		return
	}

	cacheKeys, err := entityTemplate.RenderCacheKeys(l.jsonArena, l.ctx, []*astjson.Value{entity}, "")
	if err != nil || len(cacheKeys) == 0 {
		return
	}

	// First-writer-wins semantics: a previous entity-fetch L1 write to the
	// same key is not overwritten.
	for _, ck := range cacheKeys {
		if ck == nil {
			continue
		}
		for _, keyStr := range ck.Keys {
			if _, exists := l.l1Cache[keyStr]; !exists {
				l.l1Cache[keyStr] = stored
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

func getFetchPostProcessing(fetch Fetch) PostProcessingConfiguration {
	switch f := fetch.(type) {
	case *SingleFetch:
		return f.PostProcessing
	case *EntityFetch:
		return f.PostProcessing
	case *BatchEntityFetch:
		return f.PostProcessing
	}
	return PostProcessingConfiguration{}
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

	keysToStore := l.prepareL2WriteKeys(res)
	if len(keysToStore) == 0 {
		return
	}

	// Convert CacheKeys to CacheEntries
	cacheEntries, err := l.cacheKeysToEntriesForUpdate(l.jsonArena, res, keysToStore)
	if err != nil {
		// Cache update errors are non-fatal - silently ignore
		return
	}

	// Determine effective TTL: use mutation override if set, otherwise entity default
	ttl := res.cacheConfig.TTL
	if l.enableMutationL2CachePopulation && l.mutationCacheTTLOverride > 0 {
		ttl = l.mutationCacheTTLOverride
	}

	writtenEntries := l.writeL2CacheEntries(res, keysToStore, cacheEntries, ttl)
	if len(writtenEntries) == 0 {
		return
	}

	l.recordL2WriteAnalytics(res, writtenEntries, cacheEntries, ttl)
}

// prepareL2WriteKeys chooses the write-set of CacheKeys for updateL2Cache,
// syncs entity-fetch L1/L2 keys, normalizes aliased fields on ck.Item, and
// merges any existing cached value into ck.FromCache (for writeback).
// Returns nil when there is nothing to store.
func (l *Loader) prepareL2WriteKeys(res *result) []*CacheKey {
	// Use l2CacheKeys (with prefix) if available, otherwise fall back to cacheKeys
	// prepareCacheKeys renders both cache-key slices from the same input item pointers,
	// so skip-fetch mergeResult updates are visible through res.l2CacheKeys as well.
	// Fetch paths additionally rebind both slices to merged objects inside mergeResult.
	keysToStore := res.l2CacheKeys
	if len(keysToStore) == 0 {
		keysToStore = res.l1CacheKeys
	}
	if len(keysToStore) == 0 {
		return nil
	}

	// For entity fetches, l1CacheKeys carry the authoritative cached context used during
	// resolution while l2CacheKeys carry the external-cache key strings (with prefix/header
	// isolation). Build the write set from the L1 context and graft on the L2 keys.
	if res.cacheConfig.CacheKeyTemplate != nil &&
		res.cacheConfig.CacheKeyTemplate.IsEntityFetch() &&
		len(res.l1CacheKeys) == len(res.l2CacheKeys) &&
		len(res.l2CacheKeys) > 0 {
		syncedKeys := make([]*CacheKey, 0, len(res.l2CacheKeys))
		for i := range res.l2CacheKeys {
			if res.l2CacheKeys[i] == nil {
				continue
			}
			if res.l1CacheKeys[i] == nil {
				syncedKeys = append(syncedKeys, res.l2CacheKeys[i])
				continue
			}
			cloned := *res.l1CacheKeys[i]
			cloned.Keys = res.l2CacheKeys[i].Keys
			cloned.BatchIndex = res.l2CacheKeys[i].BatchIndex
			cloned.EntityMergePath = res.l2CacheKeys[i].EntityMergePath
			cloned.NegativeCacheHit = res.l2CacheKeys[i].NegativeCacheHit
			syncedKeys = append(syncedKeys, &cloned)
		}
		keysToStore = syncedKeys
	}

	// Normalize aliased fields to original schema names before storing. Only
	// runs when HasAliases is true: StructuralCopyWithTransform produces a
	// cache-shape working tree owned by l.jsonArena (renamed + independent of
	// the response tree). When there are no aliases, ck.Item is left as-is —
	// the downstream MergeValues writeback operates on ck.FromCache (not
	// ck.Item), and cacheKeysToEntriesForUpdate materializes via MarshalTo
	// which produces independent bytes, so no extra StructuralCopy is needed
	// for isolation in the no-alias path.
	if res.providesData != nil && res.providesData.HasAliases {
		for _, ck := range keysToStore {
			if ck.Item != nil {
				ck.Item = l.structuralCopyNormalized(ck.Item, res.providesData)
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
			_, err := astjson.MergeValues(l.jsonArena, ck.FromCache, ck.Item)
			if err == nil {
				ck.Item = ck.FromCache
			}
		}
	}

	return keysToStore
}

// writeL2CacheEntries issues the regular + negative Set calls against the
// configured L2 cache, records tracing and per-set errors, and returns the
// entries that the cache accepted so recordL2WriteAnalytics can emit write
// events for exactly those.
func (l *Loader) writeL2CacheEntries(res *result, keysToStore []*CacheKey, cacheEntries []*CacheEntry, ttl time.Duration) []*CacheEntry {
	tracingCache := l.ctx.TracingOptions.Enable && !l.ctx.TracingOptions.ExcludeCacheStats
	ctx := l.ctx.ctx

	var writtenEntries []*CacheEntry

	// Store regular (non-null) cache entries
	if len(cacheEntries) > 0 {
		var l2SetStart time.Time
		if tracingCache {
			l2SetStart = time.Now()
			res.cacheTraceL2SetAttempted = true
		}
		if setErr := res.cache.Set(ctx, cacheEntries, ttl); setErr != nil {
			if tracingCache {
				res.cacheTraceL2SetDuration = time.Since(l2SetStart)
				if !errors.Is(setErr, ErrCircuitBreakerOpen) {
					res.cacheTraceL2SetError = setErr.Error()
				}
			}
			if l.ctx.cacheAnalyticsEnabled() && !errors.Is(setErr, ErrCircuitBreakerOpen) {
				l.ctx.cacheAnalytics.RecordCacheOperationError(CacheOperationError{
					Operation:  "set",
					CacheName:  res.cacheConfig.CacheName,
					EntityType: res.analyticsEntityType,
					DataSource: res.ds.Name,
					Message:    truncateErrorMessage(setErr.Error(), 256),
					ItemCount:  len(cacheEntries),
				})
			}
		} else {
			if tracingCache {
				res.cacheTraceL2SetDuration = time.Since(l2SetStart)
			}
			writtenEntries = append(writtenEntries, cacheEntries...)
		}
	}

	// Negative caching: store null sentinels with separate TTL for entities the subgraph returned null for
	if res.cacheConfig.NegativeCacheTTL > 0 {
		negEntries := l.cacheKeysToNegativeEntries(l.jsonArena, res, keysToStore)
		if len(negEntries) > 0 {
			var l2SetNegStart time.Time
			if tracingCache {
				l2SetNegStart = time.Now()
				res.cacheTraceL2SetNegAttempted = true
			}
			if setErr := res.cache.Set(ctx, negEntries, res.cacheConfig.NegativeCacheTTL); setErr != nil {
				if tracingCache {
					res.cacheTraceL2SetNegDuration = time.Since(l2SetNegStart)
					if !errors.Is(setErr, ErrCircuitBreakerOpen) {
						res.cacheTraceL2SetNegError = setErr.Error()
					}
				}
				if l.ctx.cacheAnalyticsEnabled() && !errors.Is(setErr, ErrCircuitBreakerOpen) {
					l.ctx.cacheAnalytics.RecordCacheOperationError(CacheOperationError{
						Operation:  "set_negative",
						CacheName:  res.cacheConfig.CacheName,
						EntityType: res.analyticsEntityType,
						DataSource: res.ds.Name,
						Message:    truncateErrorMessage(setErr.Error(), 256),
						ItemCount:  len(negEntries),
					})
				}
			} else {
				if tracingCache {
					res.cacheTraceL2SetNegDuration = time.Since(l2SetNegStart)
				}
				writtenEntries = append(writtenEntries, negEntries...)
			}
		}
	}

	return writtenEntries
}

// recordL2WriteAnalytics emits the CacheWriteEvent per written entry and, when
// subgraph-header isolation is active, the header-impact hashes that feed
// cross-request analytics. Only the regular cacheEntries are hashed for header
// impact — negative-cache sentinels are not meaningful there.
func (l *Loader) recordL2WriteAnalytics(res *result, writtenEntries []*CacheEntry, cacheEntries []*CacheEntry, ttl time.Duration) {
	// Record L2 write events for analytics
	if l.ctx.cacheAnalyticsEnabled() {
		for _, entry := range writtenEntries {
			if entry == nil {
				continue
			}
			l.ctx.cacheAnalytics.RecordWrite(CacheWriteEvent{
				CacheKey: entry.Key, EntityType: res.analyticsEntityType, ByteSize: len(entry.Value),
				DataSource: res.ds.Name, CacheLevel: CacheLevelL2, TTL: ttl,
				Source: l.cacheOperationSource(), WriteReason: entry.WriteReason,
			})
		}
	}

	// Record header impact events for cross-request analysis.
	// Only when IncludeSubgraphHeaderPrefix is active (headerHash != 0).
	if l.ctx.cacheAnalyticsEnabled() && res.headerHash != 0 && len(res.l1CacheKeys) > 0 {
		// Build L2-to-L1 key mapping. L1 and L2 cache keys are generated from the same
		// inputItems in prepareCacheKeys, so they have matching indices.
		l2ToBaseKey := make(map[string]string, len(res.l2CacheKeys))
		for i, l2ck := range res.l2CacheKeys {
			if i < len(res.l1CacheKeys) {
				for j, l2key := range l2ck.Keys {
					if j < len(res.l1CacheKeys[i].Keys) {
						l2ToBaseKey[l2key] = res.l1CacheKeys[i].Keys[j]
					}
				}
			}
		}

		xxh := l.ctx.cacheAnalytics.xxh
		for _, entry := range cacheEntries {
			if entry == nil {
				continue
			}
			baseKey, ok := l2ToBaseKey[entry.Key]
			if !ok {
				continue
			}
			xxh.Reset()
			_, _ = xxh.Write(entry.Value)
			l.ctx.cacheAnalytics.RecordHeaderImpactEvent(HeaderImpactEvent{
				BaseKey:      baseKey,
				HeaderHash:   res.headerHash,
				ResponseHash: xxh.Sum64(),
				EntityType:   res.analyticsEntityType,
				DataSource:   res.ds.Name,
			})
		}
	}
}

func (l *Loader) cacheKeysToEntriesForUpdate(a arena.Arena, res *result, cacheKeys []*CacheKey) ([]*CacheEntry, error) {
	rootTemplate, ok := res.cacheConfig.CacheKeyTemplate.(*RootQueryCacheKeyTemplate)
	if ok && len(rootTemplate.EntityKeyMappings) > 0 {
		return l.cacheKeysToExactRootFieldEntityEntries(a, res, cacheKeys, rootTemplate), nil
	}
	return l.cacheKeysToEntries(a, cacheKeys)
}

func (l *Loader) cacheKeysToExactRootFieldEntityEntries(a arena.Arena, res *result, cacheKeys []*CacheKey, rootTemplate *RootQueryCacheKeyTemplate) []*CacheEntry {
	// Batch entity key mode: each CacheKey already has the correct L2 key in ck.Keys[0]
	// and ck.Item points to the individual entity. Use simplified write path.
	if res.batchEntityKeyMode {
		return l.cacheKeysToEntriesBatch(a, res, cacheKeys)
	}

	// Key-format parity assumption: rendering a key from final entity data must produce
	// the same string as rendering the requested key from input args when the values match.
	prefix := l.rootFieldL2CachePrefix(res)
	seen := make(map[string]struct{}, len(cacheKeys))
	out := make([]*CacheEntry, 0, len(cacheKeys))

	for _, ck := range cacheKeys {
		if ck == nil || ck.Item == nil || ck.NegativeCacheHit {
			continue
		}

		entity := ck.Item
		if len(ck.EntityMergePath) > 0 {
			entity = ck.Item.Get(ck.EntityMergePath...)
		}
		if entity == nil || entity.Type() != astjson.TypeObject {
			continue
		}

		missingKeys := make(map[string]struct{}, len(ck.missingKeys))
		for _, key := range ck.missingKeys {
			missingKeys[key] = struct{}{}
		}

		valueBytes := entity.MarshalTo(nil)
		requestKeyBuf := arena.AllocateSlice[byte](a, 0, 64)
		renderedKeyBuf := arena.AllocateSlice[byte](a, 0, 64)
		for _, mapping := range rootTemplate.EntityKeyMappings {
			requestedKey, requestKeyBufOut := rootTemplate.renderDerivedEntityKey(a, l.ctx, requestKeyBuf, mapping, prefix)
			requestKeyBuf = requestKeyBufOut
			if requestedKey != "" {
				requestedKey = l.applyL2CacheKeyInterceptor(requestedKey, res)
			}

			renderedKey, renderedKeyBufOut := rootTemplate.renderDerivedEntityKeyFromValue(a, entity, renderedKeyBuf, mapping, prefix)
			renderedKeyBuf = renderedKeyBufOut
			if renderedKey != "" {
				renderedKey = l.applyL2CacheKeyInterceptor(renderedKey, res)
			}

			// Requested key: write with appropriate reason (refresh or backfill).
			if requestedKey != "" && shouldWriteRequestedKey(res.cacheSkipFetch, ck.fromCacheNeedsWriteback, requestedKey, renderedKey, missingKeys) {
				if _, ok := seen[requestedKey]; !ok {
					seen[requestedKey] = struct{}{}
					reason := requestedKeyWriteReason(requestedKey, missingKeys)
					out = append(out, cacheEntryFromValueBytesWithReason(a, requestedKey, valueBytes, reason))
				}
			}
			// Rendered key: write when the entity data proves it.
			// On the fetch path: always write — the subgraph is the source of truth.
			// On the skip-fetch path: only write if the key is genuinely new
			// (not an existing cached key that we'd redundantly rewrite).
			if renderedKey != "" && shouldWriteRenderedKey(res.cacheSkipFetch, ck.fromCacheNeedsWriteback, renderedKey, missingKeys) {
				if _, ok := seen[renderedKey]; !ok {
					seen[renderedKey] = struct{}{}
					reason := renderedKeyWriteReason(renderedKey, missingKeys)
					out = append(out, cacheEntryFromValueBytesWithReason(a, renderedKey, valueBytes, reason))
				}
			}
		}
	}

	return out
}

// cacheKeysToEntriesBatch converts batch CacheKeys to CacheEntries.
// For batch mode, each CacheKey already has the correct L2 key and Item pointing to entity data.
func (l *Loader) cacheKeysToEntriesBatch(a arena.Arena, res *result, cacheKeys []*CacheKey) []*CacheEntry {
	out := make([]*CacheEntry, 0, len(cacheKeys))
	seen := make(map[string]struct{}, len(cacheKeys))
	for _, ck := range cacheKeys {
		if ck == nil || ck.Item == nil || ck.NegativeCacheHit {
			continue
		}
		if ck.Item.Type() != astjson.TypeObject {
			continue
		}
		for _, key := range ck.Keys {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			valueBytes := ck.Item.MarshalTo(nil)
			entryBuf := make([]byte, len(valueBytes))
			copy(entryBuf, valueBytes)
			out = append(out, &CacheEntry{
				Key:   key,
				Value: entryBuf,
			})
		}
	}
	return out
}

func shouldWriteRequestedKey(cacheSkipFetch bool, fromCacheNeedsWriteback bool, requestedKey string, renderedKey string, missingKeys map[string]struct{}) bool {
	if _, wasMissing := missingKeys[requestedKey]; wasMissing {
		if cacheSkipFetch {
			// Skip-fetch path: the entity data came from cache, not from a subgraph, so
			// there is no fresh proof that this entity matches `requestedKey`. Only write
			// when the rendered-from-data key matches — meaning the cached entity itself
			// confirms the mapping.
			return requestedKey == renderedKey
		}
		// Fetch path: the subgraph returned this entity for a request whose arguments
		// produced `requestedKey`. The subgraph contract — "return the entity that matches
		// the supplied args" — is sufficient to write under `requestedKey` even when the
		// response payload doesn't carry the @key field (the client selected only non-key
		// fields). Suppressing the write here was the cause of the nested-key cache-miss
		// bug: every cached read would miss because every write was suppressed.
		// Still suppress when both keys are non-empty and disagree (true key skew —
		// subgraph returned an entity whose key value differs from the requested one).
		return renderedKey == "" || requestedKey == renderedKey
	}
	if cacheSkipFetch {
		return fromCacheNeedsWriteback
	}
	return true
}

// shouldWriteRenderedKey decides whether a rendered key (derived from final entity data)
// should be written to L2. On the fetch path, always write — the subgraph returned this data.
// On the skip-fetch path, only write if the key is new (missing or not previously requested),
// not an existing cached key that would be redundantly rewritten.
func shouldWriteRenderedKey(cacheSkipFetch bool, fromCacheNeedsWriteback bool, renderedKey string, missingKeys map[string]struct{}) bool {
	if !cacheSkipFetch {
		return true
	}
	// Skip-fetch path: the entity data came from cache, not from a subgraph.
	// Write if the key was missing on read (backfill) or if writeback is needed.
	if _, wasMissing := missingKeys[renderedKey]; wasMissing {
		return true
	}
	return fromCacheNeedsWriteback
}

func cacheEntryFromValueBytesWithReason(_ arena.Arena, key string, valueBytes []byte, reason CacheWriteReason) *CacheEntry {
	// Value must be heap-allocated: it is handed to the L2 cache (e.g. ristretto)
	// which retains the slice across requests. An arena-backed slice would be overwritten
	// once the request's arena is reset, producing corrupted cache reads on later requests.
	entryValue := make([]byte, len(valueBytes))
	copy(entryValue, valueBytes)
	return &CacheEntry{
		Key:         key,
		Value:       entryValue,
		WriteReason: reason,
	}
}

// requestedKeyWriteReason returns the write reason for a requested key.
// If the key was missing on read, it's a backfill; otherwise it's a refresh.
func requestedKeyWriteReason(key string, missingKeys map[string]struct{}) CacheWriteReason {
	if _, wasMissing := missingKeys[key]; wasMissing {
		return CacheWriteReasonBackfill
	}
	return CacheWriteReasonRefresh
}

// renderedKeyWriteReason returns the write reason for a rendered (entity-data-derived) key.
// If the key was missing on read, it's a backfill; otherwise it's a derived expansion.
func renderedKeyWriteReason(key string, missingKeys map[string]struct{}) CacheWriteReason {
	if _, wasMissing := missingKeys[key]; wasMissing {
		return CacheWriteReasonBackfill
	}
	return CacheWriteReasonDerived
}

func (l *Loader) rootFieldL2CachePrefix(res *result) string {
	globalPrefix := l.ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix
	// includeHeaderPrefix is the source of truth: it tells us "header partitioning
	// is on for this fetch" regardless of whether the actual hash happens to be 0.
	// Using `headerHash != 0` here was the bug — requests with `IncludeSubgraphHeaderPrefix=true`
	// but no headers forwarded computed hash=0 and silently dropped the prefix on writes,
	// producing write keys that never matched the read keys (which always built "0:...").
	if res.includeHeaderPrefix {
		headerPrefix := strconv.FormatUint(res.headerHash, 10)
		if globalPrefix != "" {
			return globalPrefix + ":" + headerPrefix
		}
		return headerPrefix
	}
	return globalPrefix
}

func (l *Loader) applyL2CacheKeyInterceptor(key string, res *result) string {
	interceptor := l.ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor
	if interceptor == nil {
		return key
	}
	info := L2CacheKeyInterceptorInfo{
		SubgraphName: res.ds.Name,
		CacheName:    res.cacheConfig.CacheName,
	}
	if res.fetchInfo != nil && res.fetchInfo.DataSourceName != "" {
		info.SubgraphName = res.fetchInfo.DataSourceName
	}
	return interceptor(l.ctx.ctx, key, info)
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
// Uses structuralCopyProjected to extract only ProvidesData fields, then hashes
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
		cachedProvides := l.structuralCopyProjected(entry.cachedValue, info.ProvidesData)
		freshProvides := l.structuralCopyProjected(freshValue, info.ProvidesData)

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
// Handles both single-entity (object) and list (array) mutation responses.
func (l *Loader) detectMutationEntityImpact(res *result, info *FetchInfo, responseData *astjson.Value) map[string]struct{} {
	if info == nil || info.OperationType != ast.OperationTypeMutation {
		return nil
	}
	cfg := res.cacheConfig.MutationEntityImpactConfig
	if cfg == nil {
		return nil
	}
	// Proceed if invalidation, populate, or analytics is configured
	if !cfg.InvalidateCache && !cfg.PopulateCache && !l.ctx.cacheAnalyticsEnabled() {
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
	// or for list mutations: responseData = {"deleteUsers": [{"id":"1"},{"id":"2"}]}
	entityData := responseData.Get(mutationFieldName)
	if entityData == nil {
		return nil
	}

	// Navigate ProvidesData to the entity level.
	// ProvidesData describes the mutation response structure: {updateUsername: {id, username}}.
	// We need the inner Object that describes the entity's fields.
	entityProvidesData := navigateProvidesDataToField(info.ProvidesData, mutationFieldName)
	if entityProvidesData == nil {
		return nil
	}

	switch entityData.Type() {
	case astjson.TypeObject:
		return l.detectSingleMutationEntityImpact(cache, cfg, info, entityData, entityProvidesData, mutationFieldName)
	case astjson.TypeArray:
		items, _ := entityData.Array()
		var deletedKeys map[string]struct{}
		for _, item := range items {
			if item == nil || item.Type() != astjson.TypeObject {
				continue
			}
			itemDeleted := l.detectSingleMutationEntityImpact(cache, cfg, info, item, entityProvidesData, mutationFieldName)
			for k, v := range itemDeleted {
				if deletedKeys == nil {
					deletedKeys = make(map[string]struct{})
				}
				deletedKeys[k] = v
			}
		}
		return deletedKeys
	default:
		return nil
	}
}

// detectSingleMutationEntityImpact handles invalidation and analytics for a single entity
// returned by a mutation. Called by detectMutationEntityImpact for each entity.
func (l *Loader) detectSingleMutationEntityImpact(
	cache LoaderCache,
	cfg *MutationEntityImpactConfig,
	info *FetchInfo,
	entityData *astjson.Value,
	entityProvidesData *Object,
	mutationFieldName string,
) map[string]struct{} {
	// Build L2 cache key for lookup
	cacheKey := l.buildMutationEntityCacheKey(cfg, entityData, info)
	if cacheKey == "" {
		return nil
	}

	// Invalidate L2 cache entry if configured
	var deletedKeys map[string]struct{}
	if cfg.InvalidateCache {
		if delErr := cache.Delete(l.ctx.ctx, []string{cacheKey}); delErr != nil {
			if l.ctx.cacheAnalyticsEnabled() {
				l.ctx.cacheAnalytics.RecordCacheOperationError(CacheOperationError{
					Operation:  "delete",
					CacheName:  cfg.CacheName,
					EntityType: cfg.EntityTypeName,
					Message:    truncateErrorMessage(delErr.Error(), 256),
					ItemCount:  1,
				})
			}
		} else {
			deletedKeys = map[string]struct{}{cacheKey: {}}
		}
	}

	// Populate L2 cache entry from the mutation response if configured.
	// `@cachePopulate` on a single-subgraph mutation has no follow-up entity fetch
	// to inherit EnableMutationL2CachePopulation, so the standard updateL2Cache write
	// path never fires. Write the entity payload here using the same cache key the
	// read path will construct.
	if cfg.PopulateCache && l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		// Project the entity through the entity-level ProvidesData (already navigated
		// by the caller) so the cached payload exactly matches what an entity fetch
		// would have returned — no extra mutation-side fields like __typename wrappers
		// that the read path doesn't expect.
		entityToCache := entityData
		if entityProvidesData != nil {
			entityToCache = l.structuralCopyProjected(entityData, entityProvidesData)
		}
		// Heap-allocate: the L2 cache may retain the byte slice across requests.
		raw := entityToCache.MarshalTo(nil)
		valueBytes := make([]byte, len(raw))
		copy(valueBytes, raw)
		if setErr := cache.Set(l.ctx.ctx, []*CacheEntry{{
			Key:   cacheKey,
			Value: valueBytes,
		}}, cfg.PopulateTTL); setErr != nil {
			if l.ctx.cacheAnalyticsEnabled() {
				l.ctx.cacheAnalytics.RecordCacheOperationError(CacheOperationError{
					Operation:  "set",
					CacheName:  cfg.CacheName,
					EntityType: cfg.EntityTypeName,
					Message:    truncateErrorMessage(setErr.Error(), 256),
					ItemCount:  1,
				})
			}
		}
	}

	// Analytics comparison requires cacheAnalytics to be enabled
	if !l.ctx.cacheAnalyticsEnabled() {
		return deletedKeys
	}

	// Build display key (without prefix) for analytics
	displayKey := l.buildEntityBaseKeyJSON(cfg.EntityTypeName, entityData, cfg.KeyFields)

	// Hash the fresh (mutation response) value
	freshProvides := l.structuralCopyProjected(entityData, entityProvidesData)
	freshBytes := freshProvides.MarshalTo(nil)
	xxh := l.ctx.cacheAnalytics.xxh
	xxh.Reset()
	_, _ = xxh.Write(freshBytes)
	freshHash := xxh.Sum64()

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

// buildEntityBaseKeyJSON builds the base JSON key for an entity: {"__typename":"...","key":{...}}.
func (l *Loader) buildEntityBaseKeyJSON(entityTypeName string, entityData *astjson.Value, keyFields []KeyField) string {
	keyObj := l.newEntityKeyStruct(entityTypeName, l.buildEntityKeyValue(entityData, keyFields))
	return string(keyObj.MarshalTo(nil))
}

// newEntityKeyStruct builds {"__typename":"<typeName>","key":<keyValue>} on l.jsonArena.
// Used by buildEntityBaseKeyJSON (keyValue derived from KeyFields) and by the
// extension-based invalidation path (keyValue already carried by the extension).
func (l *Loader) newEntityKeyStruct(typeName string, keyValue *astjson.Value) *astjson.Value {
	keyObj := astjson.ObjectValue(l.jsonArena)
	keyObj.Set(l.jsonArena, "__typename", astjson.StringValue(l.jsonArena, typeName))
	keyObj.Set(l.jsonArena, "key", keyValue)
	return keyObj
}

// buildMutationEntityCacheKey builds the L2 cache key for a mutation-returned entity.
// Format: [prefix:]{"__typename":"User","key":{"id":"1234"}}
func (l *Loader) buildMutationEntityCacheKey(cfg *MutationEntityImpactConfig, entityData *astjson.Value, info *FetchInfo) string {
	keyJSON := l.buildEntityBaseKeyJSON(cfg.EntityTypeName, entityData, cfg.KeyFields)

	// Apply global prefix and subgraph header prefix to mirror prepareCacheKeys().
	var cacheKey string
	globalPrefix := l.ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix
	if cfg.IncludeSubgraphHeaderPrefix && l.ctx.SubgraphHeadersBuilder != nil {
		_, headersHash := l.ctx.SubgraphHeadersBuilder.HeadersForSubgraph(info.DataSourceName)
		prefix := strconv.FormatUint(headersHash, 10)
		if globalPrefix != "" {
			cacheKey = globalPrefix + ":" + prefix + ":" + keyJSON
		} else {
			cacheKey = prefix + ":" + keyJSON
		}
	} else if globalPrefix != "" {
		cacheKey = globalPrefix + ":" + keyJSON
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

// buildEntityKeyValue recursively builds a JSON object from entity data using only key fields.
func (l *Loader) buildEntityKeyValue(data *astjson.Value, keyFields []KeyField) *astjson.Value {
	obj := astjson.ObjectValue(l.jsonArena)
	for _, kf := range keyFields {
		if len(kf.Children) > 0 {
			childData := data.Get(kf.Name)
			obj.Set(l.jsonArena, kf.Name, l.buildEntityKeyValue(childData, kf.Children))
		} else {
			val := data.Get(kf.Name)
			if val != nil {
				obj.Set(l.jsonArena, kf.Name, val)
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
		baseKey := string(l.newEntityKeyStruct(typename, keyVal).MarshalTo(nil))
		cacheKey := baseKey

		// Apply global prefix and subgraph header prefix to mirror prepareCacheKeys().
		// Order: global prefix → header hash prefix → interceptor.
		globalPrefix := l.ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix
		if entityConfig.IncludeSubgraphHeaderPrefix && l.ctx.SubgraphHeadersBuilder != nil {
			_, headersHash := l.ctx.SubgraphHeadersBuilder.HeadersForSubgraph(subgraphName)
			var buf [20]byte
			b := strconv.AppendUint(buf[:0], headersHash, 10)
			if globalPrefix != "" {
				cacheKey = globalPrefix + ":" + string(b) + ":" + cacheKey
			} else {
				cacheKey = string(b) + ":" + cacheKey
			}
		} else if globalPrefix != "" {
			cacheKey = globalPrefix + ":" + cacheKey
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
	for cacheName, batch := range batches {
		if delErr := batch.cache.Delete(l.ctx.ctx, batch.keys); delErr != nil &&
			!errors.Is(delErr, ErrCircuitBreakerOpen) &&
			l.ctx.cacheAnalyticsEnabled() {
			l.ctx.cacheAnalytics.RecordCacheOperationError(CacheOperationError{
				Operation: "delete",
				CacheName: cacheName,
				Message:   truncateErrorMessage(delErr.Error(), 256),
				ItemCount: len(batch.keys),
			})
		}
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

// tryRequestScopedInjection checks the per-request requestScopedL1 cache for
// all hints in the fetch configuration. If every hinted field is found, it
// injects the cached values onto each entity item and returns true to signal
// the fetch can be skipped.
func (l *Loader) tryRequestScopedInjection(res *result, cfg FetchCacheConfiguration, items []*astjson.Value) bool {
	if len(cfg.RequestScopedFields) == 0 {
		return false
	}
	// Gate on L1 being enabled when the context is set (production path).
	// Tests may construct a Loader without a ctx — treat that as enabled.
	if l.ctx != nil && !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return false
	}

	// Phase 1: Collect all cached values, verify all hints are satisfiable.
	// Do NOT mutate items until we know all hints can be satisfied.
	type pendingInjection struct {
		fieldName string
		value     *astjson.Value
	}
	pending := make([]pendingInjection, 0, len(cfg.RequestScopedFields))
	for _, hint := range cfg.RequestScopedFields {
		cachedValue, ok := l.requestScopedL1[hint.L1Key]
		if !ok || cachedValue == nil {
			return false
		}
		// Widening check: does the cached (normalized, schema-named) value have all
		// fields the current query needs? Uses the same validator as entity L1.
		if hint.ProvidesData != nil {
			if !l.validateItemHasRequiredData(cachedValue, hint.ProvidesData) {
				return false
			}
		}
		// Denormalized read: structural copy onto l.jsonArena with optional
		// denormalize transform. Materialized value is independent of the
		// stored cache value, so the response tree can mutate freely.
		injectValue := l.structuralCopyDenormalized(cachedValue, hint.ProvidesData)
		if injectValue == nil {
			return false
		}
		pending = append(pending, pendingInjection{
			fieldName: hint.FieldName,
			value:     injectValue,
		})
	}

	// Phase 2: All hints satisfied — inject into items.
	// For multiple items sharing the same hint, each item gets its own copy
	// to avoid pointer aliasing between entity items.
	for _, p := range pending {
		if len(items) == 1 {
			items[0].Set(l.jsonArena, p.fieldName, p.value)
			continue
		}
		for _, item := range items {
			copied := l.parser.StructuralCopy(l.jsonArena, p.value)
			if copied == nil {
				return false
			}
			item.Set(l.jsonArena, p.fieldName, copied)
		}
	}

	// All requestScoped fields injected — the planner only adds hints when
	// the fetch's only non-key fields are requestScoped, so we can skip.
	res.fetchSkipped = true
	return true
}

// exportRequestScopedFields extracts requestScoped field values from the first
// entity in the response and stores them in the per-request requestScopedL1
// cache. Since @requestScoped fields have the same value across all entities
// in a request, only the first entity is sampled.
func (l *Loader) exportRequestScopedFields(res *result, cfg FetchCacheConfiguration, items []*astjson.Value) {
	if len(cfg.RequestScopedFields) == 0 {
		return
	}
	if l.ctx != nil && !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return
	}

	// Build the list of sources to search: items first, then the root data
	// Root field fetches have empty items but the data is in l.resolvable.data
	sources := items
	if len(sources) == 0 && l.resolvable != nil && l.resolvable.data != nil {
		sources = []*astjson.Value{l.resolvable.data}
	}

	for _, field := range cfg.RequestScopedFields {
		for _, item := range sources {
			value := item.Get(field.FieldPath...)
			if value == nil || value.Type() == astjson.TypeNull {
				continue
			}
			// Normalize for cache: rename aliases to schema names, apply arg-hash
			// suffixes for arg-variant fields, walk nested objects/arrays.
			normalized := l.structuralCopyNormalized(value, field.ProvidesData)
			if normalized == nil {
				continue
			}
			if existingVal, loaded := l.requestScopedL1[field.L1Key]; loaded && existingVal != nil {
				// SAFETY: merge into a working copy of existingVal and
				// swap on success. astjson.MergeValues mutates in place
				// and failures are non-atomic; merging directly into the
				// live cache entry could corrupt it.
				working := l.parser.StructuralCopy(l.jsonArena, existingVal)
				_, err := astjson.MergeValues(l.jsonArena, working, normalized)
				if err == nil {
					l.requestScopedL1[field.L1Key] = working
				}
				// On failure, keep the existing entry intact (drop the working copy).
			} else {
				l.requestScopedL1[field.L1Key] = normalized
			}
			break
		}
	}
}
