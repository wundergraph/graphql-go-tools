package resolve

import (
	"strconv"
	"strings"

	"github.com/wundergraph/astjson"
)

// triggerEntityCacheConfig holds the per-trigger snapshot of the configuration
// needed to populate or invalidate L2 cache entries on each subscription event.
//
// The fields are computed once at trigger creation from the first subscription's
// EntityCachePopulation config and the request context that established the
// trigger. They MUST NOT be mutated after construction — they are read on the
// trigger event-delivery path (which may run concurrently with subscriber adds).
type triggerEntityCacheConfig struct {
	pop         *SubscriptionEntityCachePopulation
	resolveCtx  *Context
	postProcess PostProcessingConfiguration
}

// buildTriggerCacheConfig returns a triggerEntityCacheConfig when subscription
// entity-cache integration is configured AND all preconditions hold:
//
//   - the subscription provides a non-nil EntityCachePopulation with a CacheKeyTemplate
//   - per-request L2 caching is enabled (ExecutionOptions.Caching.EnableL2Cache)
//   - the configured cache name is registered on the resolver
//
// Otherwise it returns nil and the trigger has no cache integration.
//
// Spec: SUBSCRIPTION_CACHE_SPEC R8 (per-request L2 disable bypass) and R9 (missing
// cache name is a silent no-op).
func (r *Resolver) buildTriggerCacheConfig(c *Context, s *GraphQLSubscription) *triggerEntityCacheConfig {
	pop := s.EntityCachePopulation
	if pop == nil || pop.CacheKeyTemplate == nil {
		return nil
	}
	if !c.ExecutionOptions.Caching.EnableL2Cache {
		return nil
	}
	if _, ok := r.options.Caches[pop.CacheName]; !ok {
		return nil
	}
	return &triggerEntityCacheConfig{
		pop:         pop,
		resolveCtx:  c,
		postProcess: s.Trigger.PostProcessing,
	}
}

// runTriggerEntityCache performs the L2 populate or invalidate operation for the
// entities carried in `data`. It is invoked exactly once per trigger event,
// regardless of how many subscribers will receive the event afterwards.
//
// Returns when the cache backend call has completed (so callers can fan the event
// out to subscribers afterwards and rely on the cache being populated — see
// SUBSCRIPTION_CACHE_SPEC R4). Cache backend errors are intentionally swallowed
// (R10).
//
// THREADING: this method runs on the goroutine that delivers the trigger event
// (not on the trigger source goroutine). It reads `config.resolveCtx`, which was
// captured at trigger creation time. The fields it accesses (Request.ID,
// SubgraphHeadersBuilder, ExecutionOptions, Variables, RemapVariables) are
// invariant after subscription creation — do NOT write to resolveCtx from here.
func (r *Resolver) runTriggerEntityCache(config *triggerEntityCacheConfig, data []byte) {
	cache, ok := r.options.Caches[config.pop.CacheName]
	if !ok {
		// Spec R9: silent no-op on missing cache name.
		return
	}

	// Spec R7: cache key prefix construction matches prepareCacheKeys() and
	// processExtensionsCacheInvalidation(): global prefix → header hash prefix.
	var prefix string
	globalPrefix := config.resolveCtx.ExecutionOptions.Caching.GlobalCacheKeyPrefix
	if config.pop.IncludeSubgraphHeaderPrefix && config.resolveCtx.SubgraphHeadersBuilder != nil {
		_, hash := config.resolveCtx.SubgraphHeadersBuilder.HeadersForSubgraph(config.pop.DataSourceName)
		var buf [20]byte
		b := strconv.AppendUint(buf[:0], hash, 10)
		if globalPrefix != "" {
			prefix = globalPrefix + ":" + string(b)
		} else {
			prefix = string(b)
		}
	} else if globalPrefix != "" {
		prefix = globalPrefix
	}

	// We need a temporary resolvable to parse the subscription data and extract entity items.
	// Mirrors how the resolver parses subscription bodies for normal delivery, but on its
	// own arena so we don't perturb the trigger event-delivery arena lifecycle.
	resolveArena := r.resolveArenaPool.Acquire(config.resolveCtx.Request.ID)
	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena)
	defer func() {
		t.resolvable.Reset()
		t.loader.Free()
		r.resolveArenaPool.Release(resolveArena)
	}()
	if err := t.resolvable.InitSubscription(config.resolveCtx, data, config.postProcess); err != nil {
		return
	}

	entityData := t.resolvable.data
	if entityData == nil {
		return
	}
	if config.pop.SubscriptionFieldName != "" {
		entityData = entityData.Get(config.pop.SubscriptionFieldName)
	}
	if entityData == nil {
		return
	}

	// Collect entity items (single entity or array of entities).
	var items []*astjson.Value
	if entityData.Type() == astjson.TypeArray {
		items = entityData.GetArray()
	} else if entityData.Type() == astjson.TypeObject {
		items = []*astjson.Value{entityData}
	}
	if len(items) == 0 {
		return
	}

	// Spec R5 + R6:
	//   R6 — items missing __typename get pop.EntityTypeName injected.
	//   R5 — items whose __typename does not match are skipped (union/interface).
	//
	// Allocate a NEW slice rather than items[:0] — items shares the backing array
	// with entityData.GetArray(); overwriting it would corrupt the parsed JSON.
	if config.pop.EntityTypeName != "" {
		filtered := make([]*astjson.Value, 0, len(items))
		for _, item := range items {
			existing := item.Get("__typename")
			if existing == nil {
				item.Set(resolveArena.Arena, "__typename", astjson.StringValue(resolveArena.Arena, config.pop.EntityTypeName))
				filtered = append(filtered, item)
			} else if string(existing.GetStringBytes()) == config.pop.EntityTypeName {
				filtered = append(filtered, item)
			}
		}
		items = filtered
		if len(items) == 0 {
			return
		}
	}

	// Render cache keys against the items.
	cacheKeys, err := config.pop.CacheKeyTemplate.RenderCacheKeys(resolveArena.Arena, config.resolveCtx, items, prefix)
	if err != nil || len(cacheKeys) == 0 {
		return
	}

	// Spec R7 step 4: apply L2CacheKeyInterceptor when configured.
	if interceptor := config.resolveCtx.ExecutionOptions.Caching.L2CacheKeyInterceptor; interceptor != nil {
		interceptorInfo := L2CacheKeyInterceptorInfo{
			SubgraphName: config.pop.DataSourceName,
			CacheName:    config.pop.CacheName,
		}
		for _, ck := range cacheKeys {
			for i, key := range ck.Keys {
				ck.Keys[i] = interceptor(config.resolveCtx.ctx, key, interceptorInfo)
			}
		}
	}

	// Use the resolver context (not client context) since this is a trigger-level
	// operation that outlives any individual subscriber.
	ctx := r.ctx

	switch config.pop.Mode {
	case SubscriptionCacheModePopulate:
		entries := make([]*CacheEntry, 0, len(cacheKeys))
		for _, ck := range cacheKeys {
			if len(ck.Keys) == 0 || ck.Item == nil {
				continue
			}
			value := ck.Item.MarshalTo(nil)
			entries = append(entries, &CacheEntry{
				// Spec R13: clone keys off the arena before handing to the cache backend.
				Key:   strings.Clone(ck.Keys[0]),
				Value: value,
				TTL:   config.pop.TTL,
			})
		}
		if len(entries) > 0 {
			// Spec R10: cache errors must not block subscription delivery.
			_ = cache.Set(ctx, entries)
			// Spec R11: OnSubscriptionCacheWrite fires once per cached entry.
			if r.options.OnSubscriptionCacheWrite != nil {
				for _, entry := range entries {
					r.options.OnSubscriptionCacheWrite(CacheWriteEvent{
						CacheKey:   entry.Key,
						EntityType: config.pop.EntityTypeName,
						ByteSize:   len(entry.Value),
						DataSource: config.pop.DataSourceName,
						CacheLevel: CacheLevelL2,
						TTL:        config.pop.TTL,
						Source:     CacheSourceSubscription,
					})
				}
			}
		}
	case SubscriptionCacheModeInvalidate:
		keys := make([]string, 0, len(cacheKeys))
		for _, ck := range cacheKeys {
			if len(ck.Keys) > 0 {
				// Spec R13: clone keys off the arena.
				keys = append(keys, strings.Clone(ck.Keys[0]))
			}
		}
		if len(keys) > 0 {
			// Spec R10: cache errors must not block subscription delivery.
			_ = cache.Delete(ctx, keys)
			// Spec R12: OnSubscriptionCacheInvalidate fires once with all deleted keys.
			if r.options.OnSubscriptionCacheInvalidate != nil {
				r.options.OnSubscriptionCacheInvalidate(config.pop.EntityTypeName, keys)
			}
		}
	}
}
