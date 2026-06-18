package resolve

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/pkg/errors"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func (l *Loader) initRequestCaches() {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		l.l1Cache = nil
		l.requestScopedL1 = nil
		return
	}
	if l.l1Cache == nil {
		l.l1Cache = make(map[string]*astjson.Value)
	} else {
		clear(l.l1Cache)
	}
	if l.requestScopedL1 == nil {
		l.requestScopedL1 = make(map[string]*astjson.Value)
		return
	}
	clear(l.requestScopedL1)
}

func (l *Loader) prepareCacheKeys(cache *FetchCacheConfiguration, items []*astjson.Value, res *result) error {
	if cache == nil || cache.KeyTemplate == nil {
		l.captureMutationL2PopulationConfig(cache)
		return nil
	}
	l.captureMutationL2PopulationConfig(cache)
	if !l.cacheReadOrWriteEnabled(cache) {
		return nil
	}
	if cache.KeyTemplate.BatchEntityKeyArgumentPath() != nil {
		return nil
	}

	keys, err := cache.KeyTemplate.RenderCacheKeys(l.jsonArena, l.ctx, items, "")
	if err != nil {
		return err
	}
	res.cacheKeys = keys
	res.cacheTraceEntityCount = len(keys)
	return nil
}

func (l *Loader) prepareBatchCacheKeys(cache *FetchCacheConfiguration, res *result) error {
	if cache == nil || cache.KeyTemplate == nil || !cache.KeyTemplate.IsEntityFetch() {
		l.captureMutationL2PopulationConfig(cache)
		return nil
	}
	l.captureMutationL2PopulationConfig(cache)
	if !l.cacheReadOrWriteEnabled(cache) {
		return nil
	}
	if len(res.batchStats) == 0 {
		return nil
	}

	items := make([]*astjson.Value, 0, len(res.batchStats))
	for _, targets := range res.batchStats {
		if len(targets) == 0 {
			return nil
		}
		items = append(items, targets[0])
	}
	keys, err := cache.KeyTemplate.RenderCacheKeys(l.jsonArena, l.ctx, items, "")
	if err != nil {
		return err
	}
	if len(keys) != len(items) {
		return nil
	}
	res.cacheKeys = keys
	res.cacheTraceEntityCount = len(keys)
	return nil
}

func (l *Loader) tryL1CacheLoad(cache *FetchCacheConfiguration, res *result) bool {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return false
	}
	if cache != nil && cache.ShadowMode {
		return false
	}
	if cache == nil || !cache.UseL1Cache || cache.KeyTemplate == nil || !cache.KeyTemplate.IsEntityFetch() {
		return false
	}
	if len(res.cacheKeys) == 0 || l.l1Cache == nil {
		return false
	}

	hits := make([]*astjson.Value, len(res.cacheKeys))
	negativeHits := make([]bool, len(res.cacheKeys))
	for i, cacheKey := range res.cacheKeys {
		value := l.lookupL1CacheValue(cacheKey)
		if isNegativeCacheSentinel(cache, value) {
			l.recordL1Read(cache, cacheKey, true, true, value)
			l.recordCacheTraceL1Read(res, cacheKey, true, true, value)
			hits[i] = value
			negativeHits[i] = true
			continue
		}
		if value == nil || !cachedValueContainsProvides(value, cache.ProvidesData) {
			l.recordL1Read(cache, cacheKey, false, false, nil)
			l.recordCacheTraceL1Read(res, cacheKey, false, false, nil)
			return false
		}
		l.recordL1Read(cache, cacheKey, true, false, value)
		l.recordCacheTraceL1Read(res, cacheKey, true, false, value)
		hits[i] = value
	}
	for i, hit := range hits {
		if negativeHits[i] {
			res.cacheKeys[i].FromCache = astjson.StructuralCopy(l.jsonArena, hit)
			res.cacheKeys[i].NegativeCacheHit = true
			continue
		}
		res.cacheKeys[i].FromCache = l.structuralCopyDenormalizedPassthrough(hit, cache.ProvidesData)
	}
	res.cacheSkipFetch = true
	return true
}

func (l *Loader) tryL2CacheLoad(ctx context.Context, cache *FetchCacheConfiguration, res *result) bool {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return false
	}
	if cache == nil || !cache.EnableL2Cache || cache.KeyTemplate == nil {
		return false
	}
	if len(res.cacheKeys) == 0 || len(l.caches) == 0 {
		return false
	}
	if l.isMutationOperation() {
		res.cacheMustBeUpdated = true
		return false
	}
	backend := l.caches[cache.CacheName]
	if backend == nil {
		return false
	}

	keys := l.transformedL2CacheKeys(cache, res)
	if len(keys) == 0 {
		return false
	}

	res.cacheTraceL2GetAttempted = true
	l2GetStart := time.Now()
	entries, err := l.cacheAnalyticsL2Get(ctx, backend, keys, res.ds.Name, cache.CacheName)
	res.cacheTraceL2GetDuration += time.Since(l2GetStart)
	if err != nil || len(entries) != len(keys) {
		if err != nil {
			res.cacheTraceL2GetError = err.Error()
		}
		l.recordCacheOperationError("l2_get", cache.CacheName, cacheAnalyticsFirstKey(keys), err)
		if err == nil {
			l.recordCacheOperationError("l2_get", cache.CacheName, cacheAnalyticsFirstKey(keys), errors.Errorf("expected %d cache entries, got %d", len(keys), len(entries)))
		}
		res.cacheMustBeUpdated = true
		return false
	}

	hits := make([]*astjson.Value, len(entries))
	negativeHits := make([]bool, len(entries))
	for i, entry := range entries {
		if entry == nil || len(entry.Value) == 0 {
			l.recordL2Read(cache, keys[i], false, false, 0)
			l.recordCacheTraceL2Read(res, keys[i], false, false, 0, 0)
			res.cacheMustBeUpdated = true
			return false
		}
		value, parseErr := astjson.ParseBytesWithArena(l.jsonArena, entry.Value)
		if parseErr != nil {
			l.recordL2Read(cache, keys[i], false, false, 0)
			l.recordCacheTraceL2Read(res, keys[i], false, false, 0, 0)
			l.recordCacheOperationError("l2_parse", cache.CacheName, keys[i], parseErr)
			res.cacheMustBeUpdated = true
			return false
		}
		if isNegativeCacheSentinel(cache, value) {
			l.recordL2Read(cache, keys[i], true, true, len(entry.Value))
			l.recordCacheTraceL2Read(res, keys[i], true, true, len(entry.Value), entry.RemainingTTL)
			hits[i] = value
			negativeHits[i] = true
			continue
		}
		if cache.KeyTemplate.IsEntityFetch() && !cachedValueContainsProvides(value, cache.ProvidesData) {
			l.recordL2Read(cache, keys[i], false, false, len(entry.Value))
			l.recordCacheTraceL2Read(res, keys[i], false, false, len(entry.Value), 0)
			res.cacheMustBeUpdated = true
			return false
		}
		l.recordL2Read(cache, keys[i], true, false, len(entry.Value))
		l.recordCacheTraceL2Read(res, keys[i], true, false, len(entry.Value), entry.RemainingTTL)
		hits[i] = value
	}

	for i, hit := range hits {
		if negativeHits[i] {
			res.cacheKeys[i].FromCache = astjson.StructuralCopy(l.jsonArena, hit)
			res.cacheKeys[i].NegativeCacheHit = true
			continue
		}
		res.cacheKeys[i].FromCache = l.structuralCopyDenormalized(hit, cache.ProvidesData)
	}
	if cache.ShadowMode {
		res.cacheMustBeUpdated = true
		return false
	}
	res.cacheSkipFetch = true
	return true
}

func (l *Loader) tryBatchCacheLoad(ctx context.Context, cache *FetchCacheConfiguration, res *result) (allHit bool, partialHit bool) {
	if cache == nil || cache.KeyTemplate == nil || !cache.KeyTemplate.IsEntityFetch() {
		return false, false
	}
	if len(res.cacheKeys) == 0 {
		return false, false
	}

	hits := 0
	for _, cacheKey := range res.cacheKeys {
		if cache.ShadowMode {
			l.tryBatchL2CacheKey(ctx, cache, res, cacheKey)
			continue
		}
		if l.tryBatchL1CacheKey(cache, res, cacheKey) || l.tryBatchL2CacheKey(ctx, cache, res, cacheKey) {
			hits++
		}
	}
	if cache.ShadowMode {
		res.cacheMustBeUpdated = true
		return false, false
	}
	if hits == len(res.cacheKeys) {
		res.cacheSkipFetch = true
		return true, false
	}
	if hits == 0 {
		res.cacheMustBeUpdated = true
		return false, false
	}
	res.cacheMustBeUpdated = true
	return false, cache.EnablePartialCacheLoad
}

func (l *Loader) tryBatchL1CacheKey(cache *FetchCacheConfiguration, res *result, cacheKey *CacheKey) bool {
	if cache != nil && cache.ShadowMode {
		return false
	}
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return false
	}
	if cache == nil || !cache.UseL1Cache || l.l1Cache == nil {
		return false
	}
	value := l.lookupL1CacheValue(cacheKey)
	if isNegativeCacheSentinel(cache, value) {
		l.recordL1Read(cache, cacheKey, true, true, value)
		l.recordCacheTraceL1Read(res, cacheKey, true, true, value)
		cacheKey.FromCache = astjson.StructuralCopy(l.jsonArena, value)
		cacheKey.NegativeCacheHit = true
		return true
	}
	if value == nil || !cachedValueContainsProvides(value, cache.ProvidesData) {
		l.recordL1Read(cache, cacheKey, false, false, nil)
		l.recordCacheTraceL1Read(res, cacheKey, false, false, nil)
		return false
	}
	l.recordL1Read(cache, cacheKey, true, false, value)
	l.recordCacheTraceL1Read(res, cacheKey, true, false, value)
	cacheKey.FromCache = l.structuralCopyDenormalizedPassthrough(value, cache.ProvidesData)
	return true
}

func (l *Loader) tryBatchL2CacheKey(ctx context.Context, cache *FetchCacheConfiguration, res *result, cacheKey *CacheKey) bool {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return false
	}
	if cache == nil || !cache.EnableL2Cache || len(l.caches) == 0 {
		return false
	}
	backend := l.caches[cache.CacheName]
	if backend == nil || cacheKey == nil || len(cacheKey.Keys) == 0 {
		return false
	}

	keys := make([]string, 0, len(cacheKey.Keys))
	for _, key := range cacheKey.Keys {
		keys = append(keys, l.transformL2CacheKey(cache, res.ds.Name, key))
	}
	res.cacheTraceL2GetAttempted = true
	l2GetStart := time.Now()
	entries, err := l.cacheAnalyticsL2Get(ctx, backend, keys, res.ds.Name, cache.CacheName)
	res.cacheTraceL2GetDuration += time.Since(l2GetStart)
	if err != nil || len(entries) != len(keys) {
		if err != nil {
			res.cacheTraceL2GetError = err.Error()
		}
		l.recordCacheOperationError("l2_get", cache.CacheName, cacheAnalyticsFirstKey(keys), err)
		if err == nil {
			l.recordCacheOperationError("l2_get", cache.CacheName, cacheAnalyticsFirstKey(keys), errors.Errorf("expected %d cache entries, got %d", len(keys), len(entries)))
		}
		return false
	}
	for i, entry := range entries {
		if entry == nil || len(entry.Value) == 0 {
			l.recordL2Read(cache, keys[i], false, false, 0)
			l.recordCacheTraceL2Read(res, keys[i], false, false, 0, 0)
			continue
		}
		value, parseErr := astjson.ParseBytesWithArena(l.jsonArena, entry.Value)
		if parseErr != nil || !cachedValueContainsProvides(value, cache.ProvidesData) {
			if isNegativeCacheSentinel(cache, value) {
				l.recordL2Read(cache, keys[i], true, true, len(entry.Value))
				l.recordCacheTraceL2Read(res, keys[i], true, true, len(entry.Value), entry.RemainingTTL)
				cacheKey.FromCache = astjson.StructuralCopy(l.jsonArena, value)
				cacheKey.NegativeCacheHit = true
				return true
			}
			l.recordL2Read(cache, keys[i], false, false, len(entry.Value))
			l.recordCacheTraceL2Read(res, keys[i], false, false, len(entry.Value), 0)
			if parseErr != nil {
				l.recordCacheOperationError("l2_parse", cache.CacheName, keys[i], parseErr)
			}
			continue
		}
		l.recordL2Read(cache, keys[i], true, false, len(entry.Value))
		l.recordCacheTraceL2Read(res, keys[i], true, false, len(entry.Value), entry.RemainingTTL)
		cacheKey.FromCache = l.structuralCopyDenormalized(value, cache.ProvidesData)
		return true
	}
	return false
}

func (l *Loader) populateL1Cache(cache *FetchCacheConfiguration, res *result, value *astjson.Value) {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return
	}
	if cache == nil || !cache.UseL1Cache || cache.KeyTemplate == nil || !cache.KeyTemplate.IsEntityFetch() {
		return
	}
	if len(res.cacheKeys) == 0 || value == nil {
		return
	}
	if value.Type() == astjson.TypeNull && !negativeCacheEnabled(cache) {
		return
	}
	if l.l1Cache == nil {
		l.l1Cache = make(map[string]*astjson.Value, len(res.cacheKeys))
	}

	collector := l.cacheAnalytics()
	for _, cacheKey := range res.cacheKeys {
		for _, key := range cacheKey.Keys {
			if value.Type() == astjson.TypeNull {
				l.l1Cache[key] = astjson.StructuralCopy(l.jsonArena, value)
				l.recordL1Write(collector, cache, key, value, CacheWriteReasonRefresh, true)
				continue
			}
			fresh := l.structuralCopyNormalizedPassthrough(value, cache.ProvidesData)
			existing := l.l1Cache[key]
			if existing == nil {
				l.l1Cache[key] = fresh
				l.recordL1Write(collector, cache, key, fresh, CacheWriteReasonRefresh, false)
				continue
			}
			working := astjson.StructuralCopy(l.jsonArena, existing)
			merged, err := astjson.MergeValues(l.jsonArena, working, fresh)
			if err != nil {
				l.l1Cache[key] = fresh
				l.recordL1Write(collector, cache, key, fresh, CacheWriteReasonRefresh, false)
				continue
			}
			l.l1Cache[key] = merged
			l.recordL1Write(collector, cache, key, merged, CacheWriteReasonRefresh, false)
		}
	}
}

func (l *Loader) updateL2Cache(ctx context.Context, cache *FetchCacheConfiguration, res *result, value *astjson.Value) map[string]struct{} {
	writtenKeys := map[string]struct{}{}
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return writtenKeys
	}
	if cache == nil || !cache.EnableL2Cache || cache.KeyTemplate == nil || !res.cacheMustBeUpdated {
		return writtenKeys
	}
	if l.isMutationOperation() && !l.mutationL2PopulationEnabled(cache) {
		return writtenKeys
	}
	if len(res.cacheKeys) == 0 || value == nil || len(l.caches) == 0 {
		return writtenKeys
	}
	if value.Type() == astjson.TypeNull && !negativeCacheEnabled(cache) {
		return writtenKeys
	}
	backend := l.caches[cache.CacheName]
	if backend == nil {
		return writtenKeys
	}

	keys := l.transformedL2CacheKeys(cache, res)
	ttl := cache.TTL
	if l.isMutationOperation() && l.mutationTTLOverride(cache) != 0 {
		ttl = l.mutationTTLOverride(cache)
	}
	entries := make([]*CacheEntry, 0, len(keys))
	for _, key := range keys {
		if value.Type() == astjson.TypeNull {
			entries = append(entries, &CacheEntry{
				Key:         key,
				Value:       []byte("null"),
				TTL:         cache.NegativeCacheTTL,
				WriteReason: CacheWriteReasonRefresh,
			})
			continue
		}
		copied := l.structuralCopyNormalized(value, cache.ProvidesData)
		entries = append(entries, &CacheEntry{
			Key:         key,
			Value:       copied.MarshalTo(nil),
			TTL:         ttl,
			WriteReason: CacheWriteReasonRefresh,
		})
	}
	if len(entries) == 0 {
		return writtenKeys
	}
	res.cacheTraceL2SetAttempted = true
	l2SetStart := time.Now()
	err := l.cacheAnalyticsL2Set(ctx, backend, entries, res.ds.Name, cache.CacheName)
	res.cacheTraceL2SetDuration += time.Since(l2SetStart)
	for _, entry := range entries {
		l.recordL2Write(cache, entry)
		writtenKeys[entry.Key] = struct{}{}
	}
	if err != nil {
		res.cacheTraceL2SetError = err.Error()
		l.recordCacheOperationError("l2_set", cache.CacheName, cacheAnalyticsFirstEntryKey(entries), err)
	}
	return writtenKeys
}

func (l *Loader) mergeCacheResult(fetchItem *FetchItem, res *result, items []*astjson.Value) error {
	if res.batchStats != nil {
		return l.mergeBatchCacheHits(fetchItem, res)
	}
	cache := fetchCacheConfiguration(fetchItem.Fetch)
	if len(items) == 0 {
		value := firstCachedValue(res)
		if value == nil {
			return nil
		}
		value = astjson.StructuralCopy(l.jsonArena, value)
		l.resolvable.data = value
		l.populateL1Cache(cache, res, value)
		return nil
	}
	if len(items) == 1 {
		value := firstCachedValue(res)
		if value == nil {
			return nil
		}
		value = astjson.StructuralCopy(l.jsonArena, value)
		if firstCachedValueIsNegative(res) {
			setValueToNull(items[0])
			l.populateL1Cache(cache, res, value)
			return nil
		}
		var err error
		items[0], err = astjson.MergeValuesWithPath(l.jsonArena, items[0], value, res.postProcessing.MergePath...)
		if err != nil {
			return err
		}
		l.populateL1Cache(cache, res, value)
		return nil
	}
	for i, item := range items {
		if i >= len(res.cacheKeys) || res.cacheKeys[i].FromCache == nil {
			continue
		}
		value := astjson.StructuralCopy(l.jsonArena, res.cacheKeys[i].FromCache)
		if res.cacheKeys[i].NegativeCacheHit {
			setValueToNull(item)
			continue
		}
		_, err := astjson.MergeValuesWithPath(l.jsonArena, item, value, res.postProcessing.MergePath...)
		if err != nil {
			return err
		}
	}
	l.populateL1Cache(cache, res, firstCachedValue(res))
	return nil
}

func (l *Loader) mergeBatchCacheHits(fetchItem *FetchItem, res *result) error {
	for batchIndex, cacheKey := range res.cacheKeys {
		if cacheKey == nil || cacheKey.FromCache == nil {
			continue
		}
		if err := l.mergeBatchFetchedValue(fetchItem, res, batchIndex, cacheKey.FromCache); err != nil {
			return err
		}
	}
	return nil
}

func (l *Loader) mergeBatchFetchedValue(fetchItem *FetchItem, res *result, batchIndex int, value *astjson.Value) error {
	if batchIndex >= len(res.batchStats) || value == nil {
		return nil
	}
	for _, target := range res.batchStats[batchIndex] {
		if value.Type() == astjson.TypeNull {
			setValueToNull(target)
			continue
		}
		copied := astjson.StructuralCopy(l.jsonArena, value)
		_, err := astjson.MergeValuesWithPath(l.jsonArena, target, copied, res.postProcessing.MergePath...)
		if err != nil {
			return errors.WithStack(ErrMergeResult{
				Subgraph: res.ds.Name,
				Reason:   err,
				Path:     fetchItem.ResponsePath,
			})
		}
	}
	return nil
}

func (l *Loader) populateCacheAfterMerge(fetchItem *FetchItem, res *result, value *astjson.Value) map[string]struct{} {
	cache := fetchCacheConfiguration(fetchItem.Fetch)
	if res.batchStats != nil {
		return l.populateBatchCacheAfterMerge(cache, res, value)
	}
	l.recordCacheTraceSubgraphSource(res, value)
	l.recordShadowComparisons(cache, res, value)
	l.populateL1Cache(cache, res, value)
	writtenKeys := l.updateL2Cache(l.ctx.ctx, cache, res, value)
	subgraphName := ""
	if fetchInfo := fetchItem.Fetch.FetchInfo(); fetchInfo != nil {
		subgraphName = fetchInfo.DataSourceName
	}
	if !res.hasErrors {
		l.detectMutationEntityImpact(cache, value, subgraphName, writtenKeys)
	}
	return writtenKeys
}

func (l *Loader) populateBatchCacheAfterMerge(cache *FetchCacheConfiguration, res *result, value *astjson.Value) map[string]struct{} {
	writtenKeys := map[string]struct{}{}
	if cache == nil || len(res.cacheKeys) == 0 || value == nil || value.Type() != astjson.TypeArray {
		return writtenKeys
	}
	batch := value.GetArray()
	if len(batch) != len(res.cacheKeys) {
		return writtenKeys
	}
	for i, item := range batch {
		itemRes := &result{
			ds:                 res.ds,
			cacheMustBeUpdated: res.cacheMustBeUpdated,
			cacheKeys: []*CacheKey{
				res.cacheKeys[i],
			},
		}
		l.recordCacheTraceSubgraphSource(itemRes, item)
		l.recordShadowComparison(cache, res, res.cacheKeys[i], item)
		l.populateL1Cache(cache, itemRes, item)
		for key := range l.updateL2Cache(l.ctx.ctx, cache, itemRes, item) {
			writtenKeys[key] = struct{}{}
		}
		res.cacheTraceL2SetAttempted = res.cacheTraceL2SetAttempted || itemRes.cacheTraceL2SetAttempted
		res.cacheTraceL2SetDuration += itemRes.cacheTraceL2SetDuration
		if itemRes.cacheTraceL2SetError != "" {
			res.cacheTraceL2SetError = itemRes.cacheTraceL2SetError
		}
		res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, itemRes.cacheTraceEntityDetails...)
	}
	return writtenKeys
}

func (l *Loader) recordShadowComparisons(cache *FetchCacheConfiguration, res *result, value *astjson.Value) {
	if cache == nil || !cache.ShadowMode || value == nil {
		return
	}
	if len(res.cacheKeys) == 0 {
		return
	}
	if value.Type() == astjson.TypeArray {
		batch := value.GetArray()
		if len(batch) != len(res.cacheKeys) {
			return
		}
		for i, item := range batch {
			l.recordShadowComparison(cache, res, res.cacheKeys[i], item)
		}
		return
	}
	for _, cacheKey := range res.cacheKeys {
		l.recordShadowComparison(cache, res, cacheKey, value)
	}
}

func (l *Loader) recordShadowComparison(cache *FetchCacheConfiguration, res *result, cacheKey *CacheKey, fresh *astjson.Value) {
	if cache == nil || !cache.ShadowMode || cacheKey == nil || cacheKey.FromCache == nil || fresh == nil {
		return
	}
	collector := l.cacheAnalytics()
	if collector == nil {
		return
	}

	cachedBytes := l.shadowComparisonBytes(cache, cacheKey.FromCache)
	freshBytes := l.shadowComparisonBytes(cache, fresh)
	collector.recordShadowComparison(ShadowComparisonEvent{
		Key:        l.shadowComparisonKey(cache, res, cacheKey),
		EntityType: cacheAnalyticsEntityType(cache),
		Matched:    bytes.Equal(cachedBytes, freshBytes),
		CachedHash: xxhash.Sum64(cachedBytes),
		FreshHash:  xxhash.Sum64(freshBytes),
		CachedSize: len(cachedBytes),
		FreshSize:  len(freshBytes),
	})
}

func (l *Loader) shadowComparisonBytes(cache *FetchCacheConfiguration, value *astjson.Value) []byte {
	if value == nil {
		return nil
	}
	return l.structuralCopyNormalized(value, cache.ProvidesData).MarshalTo(nil)
}

func (l *Loader) shadowComparisonKey(cache *FetchCacheConfiguration, res *result, cacheKey *CacheKey) string {
	key := cacheAnalyticsPrimaryKey(cacheKey)
	if res == nil {
		return key
	}
	return l.transformL2CacheKey(cache, res.ds.Name, key)
}

type extensionInvalidationDeleteGroup struct {
	cacheName string
	backend   LoaderCache
	keys      []string
}

func (l *Loader) invalidateL2FromExtensions(response *astjson.Value, subgraphName string, writtenKeys map[string]struct{}) {
	if l == nil || response == nil {
		return
	}
	extensions := response.Get("extensions")
	if extensions == nil || extensions.Type() != astjson.TypeObject {
		return
	}
	cacheInvalidation := extensions.Get("cacheInvalidation")
	if cacheInvalidation == nil || cacheInvalidation.Type() != astjson.TypeObject {
		return
	}
	keysValue := cacheInvalidation.Get("keys")
	if keysValue == nil || keysValue.Type() != astjson.TypeArray {
		return
	}
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL2Cache || len(l.entityCacheConfigs) == 0 || len(l.caches) == 0 {
		return
	}
	typeConfigs := l.entityCacheConfigs[subgraphName]
	if len(typeConfigs) == 0 {
		return
	}

	groups := make([]extensionInvalidationDeleteGroup, 0)
	groupIndexByCacheName := map[string]int{}
	seenByCacheName := map[string]map[string]struct{}{}
	for _, item := range keysValue.GetArray() {
		if item == nil || item.Type() != astjson.TypeObject {
			continue
		}
		typenameValue := item.Get("typename")
		if typenameValue == nil || typenameValue.Type() != astjson.TypeString {
			continue
		}
		typename := string(typenameValue.GetStringBytes())
		if typename == "" {
			continue
		}
		cfg := typeConfigs[typename]
		if cfg == nil {
			continue
		}
		backend := l.caches[cfg.CacheName]
		if backend == nil {
			continue
		}
		keyObject := l.extensionInvalidationKeyObject(item.Get("key"))
		if keyObject == nil || objectLen(keyObject) == 0 {
			continue
		}
		key := buildEntityKeyString(l.jsonArena, typename, keyObject, "")
		transformCache := &FetchCacheConfiguration{
			CacheName:                   cfg.CacheName,
			IncludeSubgraphHeaderPrefix: cfg.IncludeSubgraphHeaderPrefix,
		}
		key = l.transformL2CacheKey(transformCache, subgraphName, key)
		if _, written := writtenKeys[key]; written {
			continue
		}
		seen := seenByCacheName[cfg.CacheName]
		if seen == nil {
			seen = map[string]struct{}{}
			seenByCacheName[cfg.CacheName] = seen
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		groupIndex, ok := groupIndexByCacheName[cfg.CacheName]
		if !ok {
			groupIndex = len(groups)
			groupIndexByCacheName[cfg.CacheName] = groupIndex
			groups = append(groups, extensionInvalidationDeleteGroup{
				cacheName: cfg.CacheName,
				backend:   backend,
			})
		}
		groups[groupIndex].keys = append(groups[groupIndex].keys, key)
		l.recordCacheInvalidation(CacheInvalidationEvent{
			EntityType:   typename,
			SubgraphName: subgraphName,
			CacheName:    cfg.CacheName,
			Key:          key,
			Source:       "extension",
			Deleted:      true,
		})
	}

	for _, group := range groups {
		if len(group.keys) == 0 {
			continue
		}
		if err := group.backend.Delete(contextForLoader(l), group.keys); err != nil {
			l.recordCacheOperationError("l2_delete", group.cacheName, cacheAnalyticsFirstKey(group.keys), err)
		}
	}
}

func (l *Loader) extensionInvalidationKeyObject(value *astjson.Value) *astjson.Value {
	if value == nil || value.Type() != astjson.TypeObject {
		return nil
	}
	object := astjson.ObjectValue(l.jsonArena)
	value.GetObject().Visit(func(key []byte, child *astjson.Value) {
		object.Set(l.jsonArena, string(key), copyEntityKeyValue(l.jsonArena, child))
	})
	return object
}

func (l *Loader) cacheReadOrWriteEnabled(cache *FetchCacheConfiguration) bool {
	if l.ctx == nil {
		return false
	}
	if l.ctx.ExecutionOptions.Caching.EnableL1Cache && cache.UseL1Cache {
		return true
	}
	return l.ctx.ExecutionOptions.Caching.EnableL2Cache && cache.EnableL2Cache
}

func (l *Loader) captureMutationL2PopulationConfig(cache *FetchCacheConfiguration) {
	if !l.isMutationOperation() || cache == nil || !cache.EnableMutationL2CachePopulation {
		return
	}
	l.enableMutationL2CachePopulation = true
	if cache.MutationCacheTTLOverride != 0 {
		l.mutationCacheTTLOverride = cache.MutationCacheTTLOverride
	}
}

func (l *Loader) isMutationOperation() bool {
	return l != nil && l.info != nil && l.info.OperationType == ast.OperationTypeMutation
}

func (l *Loader) mutationL2PopulationEnabled(cache *FetchCacheConfiguration) bool {
	return cache != nil && (cache.EnableMutationL2CachePopulation || l.enableMutationL2CachePopulation)
}

func (l *Loader) mutationTTLOverride(cache *FetchCacheConfiguration) time.Duration {
	if cache != nil && cache.MutationCacheTTLOverride != 0 {
		return cache.MutationCacheTTLOverride
	}
	return l.mutationCacheTTLOverride
}

func (l *Loader) cacheAnalytics() *cacheAnalyticsCollector {
	if l == nil || l.ctx == nil {
		return nil
	}
	return l.ctx.cacheAnalytics()
}

func (l *Loader) recordL1Read(cache *FetchCacheConfiguration, cacheKey *CacheKey, hit bool, negative bool, value *astjson.Value) {
	collector := l.cacheAnalytics()
	if collector == nil {
		return
	}
	event := CacheKeyEvent{
		Key:        cacheAnalyticsPrimaryKey(cacheKey),
		EntityType: cacheAnalyticsEntityType(cache),
		Hit:        hit,
		Negative:   negative,
		Shadow:     cache != nil && cache.ShadowMode,
	}
	if hit {
		event.Bytes = cacheAnalyticsValueBytes(value)
	}
	collector.recordL1Read(event)
}

func (l *Loader) recordL2Read(cache *FetchCacheConfiguration, key string, hit bool, negative bool, bytes int) {
	collector := l.cacheAnalytics()
	if collector == nil {
		return
	}
	collector.recordL2Read(CacheKeyEvent{
		Key:        key,
		EntityType: cacheAnalyticsEntityType(cache),
		Hit:        hit,
		Negative:   negative,
		Shadow:     cache != nil && cache.ShadowMode,
		Bytes:      bytes,
	})
}

func (l *Loader) recordL1Write(collector *cacheAnalyticsCollector, cache *FetchCacheConfiguration, key string, value *astjson.Value, reason CacheWriteReason, negative bool) {
	if collector == nil {
		return
	}
	collector.recordL1Write(CacheWriteEvent{
		Key:        key,
		EntityType: cacheAnalyticsEntityType(cache),
		Bytes:      cacheAnalyticsValueBytes(value),
		Reason:     reason,
		Negative:   negative,
	})
}

func (l *Loader) recordL2Write(cache *FetchCacheConfiguration, entry *CacheEntry) {
	collector := l.cacheAnalytics()
	if collector == nil || entry == nil {
		return
	}
	collector.recordL2Write(CacheWriteEvent{
		Key:        entry.Key,
		EntityType: cacheAnalyticsEntityType(cache),
		Bytes:      len(entry.Value),
		TTL:        entry.TTL,
		Reason:     entry.WriteReason,
		Negative:   string(entry.Value) == "null",
	})
}

func (l *Loader) recordHeaderImpact(subgraphName string, cacheName string, headerPrefix string) {
	collector := l.cacheAnalytics()
	if collector == nil {
		return
	}
	collector.recordHeaderImpact(HeaderImpactEvent{
		SubgraphName: subgraphName,
		CacheName:    cacheName,
		HeaderHash:   headerPrefix,
		KeyPrefix:    headerPrefix,
	})
}

func (l *Loader) recordCacheOperationError(operation string, cacheName string, key string, err error) {
	collector := l.cacheAnalytics()
	if collector == nil || err == nil {
		return
	}
	collector.recordCacheOperationError(CacheOperationError{
		Operation: operation,
		CacheName: cacheName,
		Key:       key,
		Error:     err.Error(),
	})
}

func (l *Loader) recordCacheInvalidation(event CacheInvalidationEvent) {
	collector := l.cacheAnalytics()
	if collector == nil {
		return
	}
	collector.recordCacheInvalidation(event)
}

func (l *Loader) cacheAnalyticsL2Get(ctx context.Context, backend LoaderCache, keys []string, subgraphName string, cacheName string) ([]*CacheEntry, error) {
	collector := l.cacheAnalytics()
	if collector == nil {
		return backend.Get(ctx, keys)
	}
	start := time.Now()
	entries, err := backend.Get(ctx, keys)
	collector.recordFetchTiming(FetchTimingEvent{
		SubgraphName: subgraphName,
		CacheName:    cacheName,
		Operation:    "l2_get",
		Duration:     time.Since(start),
		Bytes:        cacheAnalyticsEntriesBytes(entries),
	})
	return entries, err
}

func (l *Loader) cacheAnalyticsL2Set(ctx context.Context, backend LoaderCache, entries []*CacheEntry, subgraphName string, cacheName string) error {
	collector := l.cacheAnalytics()
	if collector == nil {
		return backend.Set(ctx, entries)
	}
	start := time.Now()
	err := backend.Set(ctx, entries)
	collector.recordFetchTiming(FetchTimingEvent{
		SubgraphName: subgraphName,
		CacheName:    cacheName,
		Operation:    "l2_set",
		Duration:     time.Since(start),
		Bytes:        cacheAnalyticsEntriesBytes(entries),
	})
	return err
}

func (l *Loader) lookupL1CacheValue(cacheKey *CacheKey) *astjson.Value {
	if cacheKey == nil {
		return nil
	}
	for _, key := range cacheKey.Keys {
		if value := l.l1Cache[key]; value != nil {
			return value
		}
	}
	return nil
}

func (l *Loader) transformedL2CacheKeys(cache *FetchCacheConfiguration, res *result) []string {
	keys := make([]string, 0, len(res.cacheKeys))
	for _, cacheKey := range res.cacheKeys {
		for _, key := range cacheKey.Keys {
			keys = append(keys, l.transformL2CacheKey(cache, res.ds.Name, key))
		}
	}
	return keys
}

func (l *Loader) transformL2CacheKey(cache *FetchCacheConfiguration, subgraphName string, key string) string {
	if l.ctx == nil {
		return key
	}
	if prefix := l.ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix; prefix != "" {
		key = prefix + key
	}
	if cache != nil && cache.IncludeSubgraphHeaderPrefix {
		_, hash := l.ctx.HeadersForSubgraphRequest(subgraphName)
		if hash != 0 {
			headerPrefix := strconv.FormatUint(hash, 10)
			key = prefixCacheKey(headerPrefix, key)
			l.recordHeaderImpact(subgraphName, cache.CacheName, headerPrefix)
		}
	}
	if interceptor := l.ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor; interceptor != nil {
		key = interceptor(L2CacheKeyInterceptorInfo{
			SubgraphName: subgraphName,
			CacheName:    cache.CacheName,
		}, key)
	}
	return key
}

func fetchCacheConfiguration(fetch Fetch) *FetchCacheConfiguration {
	switch f := fetch.(type) {
	case *SingleFetch:
		return f.Cache
	case *EntityFetch:
		return f.Cache
	case *BatchEntityFetch:
		return f.Cache
	default:
		return nil
	}
}

func firstCachedValue(res *result) *astjson.Value {
	for _, cacheKey := range res.cacheKeys {
		if cacheKey.FromCache != nil {
			return cacheKey.FromCache
		}
	}
	return nil
}

func cacheAnalyticsPrimaryKey(cacheKey *CacheKey) string {
	if cacheKey == nil || len(cacheKey.Keys) == 0 {
		return ""
	}
	return cacheKey.Keys[0]
}

func cacheAnalyticsFirstKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func cacheAnalyticsFirstEntryKey(entries []*CacheEntry) string {
	for _, entry := range entries {
		if entry != nil {
			return entry.Key
		}
	}
	return ""
}

func cacheAnalyticsEntityType(cache *FetchCacheConfiguration) string {
	if cache == nil || cache.KeyTemplate == nil {
		return ""
	}
	switch template := cache.KeyTemplate.(type) {
	case *EntityQueryCacheKeyTemplate:
		return template.TypeName
	case *RootQueryCacheKeyTemplate:
		if len(template.EntityKeyMappings) == 1 {
			return template.EntityKeyMappings[0].EntityTypeName
		}
	}
	return ""
}

func cacheAnalyticsValueBytes(value *astjson.Value) int {
	if value == nil {
		return 0
	}
	return len(value.MarshalTo(nil))
}

func cacheAnalyticsEntriesBytes(entries []*CacheEntry) int {
	bytes := 0
	for _, entry := range entries {
		if entry != nil {
			bytes += len(entry.Value)
		}
	}
	return bytes
}

func (l *Loader) beginCacheTrace(res *result) func() {
	if l == nil || l.ctx == nil || res == nil || !l.ctx.TracingOptions.Enable || l.ctx.TracingOptions.ExcludeCacheStats {
		return func() {}
	}
	if res.cacheTraceDurationSinceStartNano == 0 {
		res.cacheTraceDurationSinceStartNano = GetDurationNanoSinceTraceStart(l.ctx.Context())
	}
	start := time.Now()
	return func() {
		res.cacheTraceDurationNano += time.Since(start).Nanoseconds()
	}
}

func (l *Loader) recordCacheTraceL1Read(res *result, cacheKey *CacheKey, hit bool, negative bool, value *astjson.Value) {
	if res == nil {
		return
	}
	if hit {
		res.cacheTraceL1Hits++
		if negative {
			res.cacheTraceNegativeHits++
			res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, CacheTraceEntity{
				Key:      cacheAnalyticsPrimaryKey(cacheKey),
				Source:   "negative_cache",
				ByteSize: cacheAnalyticsValueBytes(value),
			})
			return
		}
		res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, CacheTraceEntity{
			Key:      cacheAnalyticsPrimaryKey(cacheKey),
			Source:   "l1",
			ByteSize: cacheAnalyticsValueBytes(value),
		})
		return
	}
	res.cacheTraceL1Misses++
}

func (l *Loader) recordCacheTraceL2Read(res *result, key string, hit bool, negative bool, bytes int, remainingTTL time.Duration) {
	if res == nil {
		return
	}
	if hit {
		res.cacheTraceL2Hits++
		source := "l2"
		if negative {
			source = "negative_cache"
			res.cacheTraceNegativeHits++
		}
		entity := CacheTraceEntity{
			Key:      key,
			Source:   source,
			ByteSize: bytes,
		}
		if remainingTTL > 0 {
			entity.RemainingTTLSeconds = remainingTTL.Seconds()
		}
		res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, entity)
		return
	}
	res.cacheTraceL2Misses++
}

func (l *Loader) recordCacheTraceSubgraphSource(res *result, value *astjson.Value) {
	if res == nil || value == nil || len(res.cacheKeys) == 0 {
		return
	}
	for _, cacheKey := range res.cacheKeys {
		if cacheKey == nil || cacheKey.FromCache != nil {
			continue
		}
		res.cacheTraceEntityDetails = append(res.cacheTraceEntityDetails, CacheTraceEntity{
			Key:      cacheAnalyticsPrimaryKey(cacheKey),
			Source:   "subgraph",
			ByteSize: cacheAnalyticsValueBytes(value),
		})
	}
}

func cachedValueContainsProvides(value *astjson.Value, provides *Object) bool {
	if provides == nil || value == nil {
		return true
	}
	if value.Type() != astjson.TypeObject {
		return false
	}
	for _, field := range provides.Fields {
		if field == nil {
			continue
		}
		child := value.Get(fieldSchemaName(field))
		if child == nil {
			return false
		}
		if !cachedChildContainsProvides(child, field.Value) {
			return false
		}
	}
	return true
}

func negativeCacheEnabled(cache *FetchCacheConfiguration) bool {
	return cache != nil &&
		cache.NegativeCacheTTL > 0 &&
		cache.KeyTemplate != nil &&
		cache.KeyTemplate.IsEntityFetch()
}

func isNegativeCacheSentinel(cache *FetchCacheConfiguration, value *astjson.Value) bool {
	return negativeCacheEnabled(cache) && value != nil && value.Type() == astjson.TypeNull
}

func firstCachedValueIsNegative(res *result) bool {
	for _, cacheKey := range res.cacheKeys {
		if cacheKey.FromCache != nil {
			return cacheKey.NegativeCacheHit
		}
	}
	return false
}

func setValueToNull(value *astjson.Value) {
	if value == nil {
		return
	}
	*value = *astjson.NullValue
}

func cachedChildContainsProvides(value *astjson.Value, node Node) bool {
	switch typed := node.(type) {
	case *Object:
		return cachedValueContainsProvides(value, typed)
	case *Array:
		if value.Type() != astjson.TypeArray {
			return false
		}
		item, ok := typed.Item.(*Object)
		if !ok {
			return true
		}
		for _, child := range value.GetArray() {
			if child == nil || child.Type() == astjson.TypeNull {
				continue
			}
			if !cachedValueContainsProvides(child, item) {
				return false
			}
		}
	}
	return true
}
