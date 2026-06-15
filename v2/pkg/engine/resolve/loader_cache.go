package resolve

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/wundergraph/astjson"
)

func (l *Loader) initRequestCaches() {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		l.l1Cache = nil
		return
	}
	if l.l1Cache == nil {
		l.l1Cache = make(map[string]*astjson.Value)
		return
	}
	clear(l.l1Cache)
}

func (l *Loader) prepareCacheKeys(cache *FetchCacheConfiguration, items []*astjson.Value, res *result) error {
	if cache == nil || cache.KeyTemplate == nil {
		return nil
	}
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
	return nil
}

func (l *Loader) prepareBatchCacheKeys(cache *FetchCacheConfiguration, res *result) error {
	if cache == nil || cache.KeyTemplate == nil || !cache.KeyTemplate.IsEntityFetch() {
		return nil
	}
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
	return nil
}

func (l *Loader) tryL1CacheLoad(cache *FetchCacheConfiguration, res *result) bool {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return false
	}
	if cache == nil || !cache.UseL1Cache || cache.KeyTemplate == nil || !cache.KeyTemplate.IsEntityFetch() {
		return false
	}
	if len(res.cacheKeys) == 0 || l.l1Cache == nil {
		return false
	}

	hits := make([]*astjson.Value, len(res.cacheKeys))
	for i, cacheKey := range res.cacheKeys {
		value := l.lookupL1CacheValue(cacheKey)
		if value == nil || !cachedValueContainsProvides(value, cache.ProvidesData) {
			return false
		}
		hits[i] = value
	}
	for i, hit := range hits {
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
	backend := l.caches[cache.CacheName]
	if backend == nil {
		return false
	}

	keys := l.transformedL2CacheKeys(cache, res)
	if len(keys) == 0 {
		return false
	}

	entries, err := backend.Get(ctx, keys)
	if err != nil || len(entries) != len(keys) {
		res.cacheMustBeUpdated = true
		return false
	}

	hits := make([]*astjson.Value, len(entries))
	for i, entry := range entries {
		if entry == nil || len(entry.Value) == 0 {
			res.cacheMustBeUpdated = true
			return false
		}
		value, parseErr := astjson.ParseBytesWithArena(l.jsonArena, entry.Value)
		if parseErr != nil {
			res.cacheMustBeUpdated = true
			return false
		}
		if cache.KeyTemplate.IsEntityFetch() && !cachedValueContainsProvides(value, cache.ProvidesData) {
			res.cacheMustBeUpdated = true
			return false
		}
		hits[i] = value
	}

	for i, hit := range hits {
		res.cacheKeys[i].FromCache = l.structuralCopyDenormalized(hit, cache.ProvidesData)
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
		if l.tryBatchL1CacheKey(cache, cacheKey) || l.tryBatchL2CacheKey(ctx, cache, res, cacheKey) {
			hits++
		}
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

func (l *Loader) tryBatchL1CacheKey(cache *FetchCacheConfiguration, cacheKey *CacheKey) bool {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return false
	}
	if cache == nil || !cache.UseL1Cache || l.l1Cache == nil {
		return false
	}
	value := l.lookupL1CacheValue(cacheKey)
	if value == nil || !cachedValueContainsProvides(value, cache.ProvidesData) {
		return false
	}
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
	entries, err := backend.Get(ctx, keys)
	if err != nil || len(entries) != len(keys) {
		return false
	}
	for _, entry := range entries {
		if entry == nil || len(entry.Value) == 0 {
			continue
		}
		value, parseErr := astjson.ParseBytesWithArena(l.jsonArena, entry.Value)
		if parseErr != nil || !cachedValueContainsProvides(value, cache.ProvidesData) {
			continue
		}
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
	if l.l1Cache == nil {
		l.l1Cache = make(map[string]*astjson.Value, len(res.cacheKeys))
	}

	for _, cacheKey := range res.cacheKeys {
		for _, key := range cacheKey.Keys {
			fresh := l.structuralCopyNormalizedPassthrough(value, cache.ProvidesData)
			existing := l.l1Cache[key]
			if existing == nil {
				l.l1Cache[key] = fresh
				continue
			}
			working := astjson.StructuralCopy(l.jsonArena, existing)
			merged, err := astjson.MergeValues(l.jsonArena, working, fresh)
			if err != nil {
				l.l1Cache[key] = fresh
				continue
			}
			l.l1Cache[key] = merged
		}
	}
}

func (l *Loader) updateL2Cache(ctx context.Context, cache *FetchCacheConfiguration, res *result, value *astjson.Value) {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return
	}
	if cache == nil || !cache.EnableL2Cache || cache.KeyTemplate == nil || !res.cacheMustBeUpdated {
		return
	}
	if len(res.cacheKeys) == 0 || value == nil || len(l.caches) == 0 {
		return
	}
	backend := l.caches[cache.CacheName]
	if backend == nil {
		return
	}

	keys := l.transformedL2CacheKeys(cache, res)
	entries := make([]*CacheEntry, 0, len(keys))
	for _, key := range keys {
		copied := l.structuralCopyNormalized(value, cache.ProvidesData)
		entries = append(entries, &CacheEntry{
			Key:         key,
			Value:       copied.MarshalTo(nil),
			TTL:         cache.TTL,
			WriteReason: CacheWriteReasonRefresh,
		})
	}
	if len(entries) == 0 {
		return
	}
	_ = backend.Set(ctx, entries)
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

func (l *Loader) populateCacheAfterMerge(fetchItem *FetchItem, res *result, value *astjson.Value) {
	cache := fetchCacheConfiguration(fetchItem.Fetch)
	if res.batchStats != nil {
		l.populateBatchCacheAfterMerge(cache, res, value)
		return
	}
	l.populateL1Cache(cache, res, value)
	l.updateL2Cache(l.ctx.ctx, cache, res, value)
}

func (l *Loader) populateBatchCacheAfterMerge(cache *FetchCacheConfiguration, res *result, value *astjson.Value) {
	if cache == nil || len(res.cacheKeys) == 0 || value == nil || value.Type() != astjson.TypeArray {
		return
	}
	batch := value.GetArray()
	if len(batch) != len(res.cacheKeys) {
		return
	}
	for i, item := range batch {
		itemRes := &result{
			ds:                 res.ds,
			cacheMustBeUpdated: res.cacheMustBeUpdated,
			cacheKeys: []*CacheKey{
				res.cacheKeys[i],
			},
		}
		l.populateL1Cache(cache, itemRes, item)
		l.updateL2Cache(l.ctx.ctx, cache, itemRes, item)
	}
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
			key = prefixCacheKey(strconv.FormatUint(hash, 10), key)
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
