package resolve

import (
	"context"
	"strings"

	"github.com/wundergraph/astjson"
)

func (r *Resolver) processSubscriptionEntityCache(ctx *Context, t *tools, subscription *GraphQLSubscription) {
	if r == nil || ctx == nil || t == nil || t.resolvable == nil || t.loader == nil || subscription == nil {
		return
	}
	t.loader.ctx = ctx
	if !ctx.ExecutionOptions.Caching.EnableL2Cache {
		return
	}
	cfg := subscription.EntityCachePopulation
	if cfg == nil || cfg.CacheKeyTemplate == nil || cfg.CacheName == "" {
		return
	}
	if len(r.options.Caches) == 0 {
		return
	}
	backend := r.options.Caches[cfg.CacheName]
	if backend == nil {
		return
	}

	root := subscriptionCacheRootValue(t.resolvable.data, cfg.SubscriptionFieldName)
	entities := subscriptionCacheEntityValues(root)
	if len(entities) == 0 {
		return
	}

	cache := &FetchCacheConfiguration{
		CacheName:                   cfg.CacheName,
		EnableL2Cache:               true,
		IncludeSubgraphHeaderPrefix: cfg.IncludeSubgraphHeaderPrefix,
		KeyTemplate:                 cfg.CacheKeyTemplate,
		TTL:                         cfg.TTL,
	}

	entries := make([]*CacheEntry, 0, len(entities))
	keysToDelete := make([]string, 0, len(entities))
	for _, entity := range entities {
		item := subscriptionCacheEntityCopy(t.loader, entity, cfg.EntityTypeName)
		if item == nil {
			continue
		}
		if !subscriptionCacheEntityMatchesType(item, cfg.EntityTypeName) {
			continue
		}
		cacheKeys, err := cfg.CacheKeyTemplate.RenderCacheKeys(t.loader.jsonArena, ctx, []*astjson.Value{item}, "")
		if err != nil || len(cacheKeys) == 0 {
			continue
		}
		transformedKeys := subscriptionTransformedL2CacheKeys(t.loader, cache, cfg.DataSourceName, cacheKeys)
		if len(transformedKeys) == 0 {
			continue
		}
		keyOnly := subscriptionCacheEntityHasOnlyKeyFields(item, cfg.CacheKeyTemplate.KeyFields())
		if keyOnly {
			if !cfg.EnableInvalidationOnKeyOnly {
				continue
			}
			keysToDelete = append(keysToDelete, transformedKeys...)
			continue
		}
		for _, key := range transformedKeys {
			value := item.MarshalTo(nil)
			entries = append(entries, &CacheEntry{
				Key:         key,
				Value:       value,
				TTL:         cfg.TTL,
				WriteReason: CacheWriteReasonRefresh,
			})
		}
	}

	if len(entries) > 0 {
		r.populateSubscriptionEntityCache(contextForLoader(t.loader), t.loader, backend, cfg, entries)
		return
	}
	if len(keysToDelete) > 0 {
		r.invalidateSubscriptionEntityCache(contextForLoader(t.loader), t.loader, backend, cfg, keysToDelete)
	}
}

func subscriptionCacheRootValue(data *astjson.Value, fieldName string) *astjson.Value {
	if data == nil {
		return nil
	}
	if fieldName == "" {
		return data
	}
	return data.Get(fieldName)
}

func subscriptionCacheEntityValues(value *astjson.Value) []*astjson.Value {
	if value == nil {
		return nil
	}
	switch value.Type() {
	case astjson.TypeObject:
		return []*astjson.Value{value}
	case astjson.TypeArray:
		out := make([]*astjson.Value, 0, len(value.GetArray()))
		for _, item := range value.GetArray() {
			if item != nil && item.Type() == astjson.TypeObject {
				out = append(out, item)
			}
		}
		return out
	default:
		return nil
	}
}

func subscriptionCacheEntityCopy(loader *Loader, entity *astjson.Value, entityTypeName string) *astjson.Value {
	if loader == nil || entity == nil || entity.Type() != astjson.TypeObject {
		return nil
	}
	copied, err := astjson.ParseBytesWithArena(loader.jsonArena, entity.MarshalTo(nil))
	if err != nil {
		return nil
	}
	if copied.Get("__typename") == nil && entityTypeName != "" {
		copied.GetObject().Set(loader.jsonArena, "__typename", astjson.StringValue(loader.jsonArena, entityTypeName))
	}
	return copied
}

func subscriptionCacheEntityMatchesType(entity *astjson.Value, entityTypeName string) bool {
	if entityTypeName == "" {
		return true
	}
	value := entity.Get("__typename")
	if value == nil || value.Type() == astjson.TypeNull {
		return true
	}
	if value.Type() != astjson.TypeString {
		return string(value.MarshalTo(nil)) == entityTypeName
	}
	return string(value.GetStringBytes()) == entityTypeName
}

func subscriptionTransformedL2CacheKeys(loader *Loader, cache *FetchCacheConfiguration, subgraphName string, cacheKeys []*CacheKey) []string {
	keys := make([]string, 0, len(cacheKeys))
	for _, cacheKey := range cacheKeys {
		if cacheKey == nil {
			continue
		}
		for _, key := range cacheKey.Keys {
			keys = append(keys, strings.Clone(loader.transformL2CacheKey(cache, subgraphName, key)))
		}
	}
	return keys
}

func subscriptionCacheEntityHasOnlyKeyFields(entity *astjson.Value, keyFields []KeyField) bool {
	if entity == nil || entity.Type() != astjson.TypeObject || len(keyFields) == 0 {
		return false
	}
	onlyKeyFields := true
	entity.GetObject().Visit(func(key []byte, value *astjson.Value) {
		if !onlyKeyFields {
			return
		}
		fieldName := string(key)
		if fieldName == "__typename" {
			return
		}
		field, ok := subscriptionCacheKeyFieldByName(keyFields, fieldName)
		if !ok {
			onlyKeyFields = false
			return
		}
		if len(field.Children) > 0 && !subscriptionCacheEntityHasOnlyKeyFields(value, field.Children) {
			onlyKeyFields = false
		}
	})
	return onlyKeyFields
}

func subscriptionCacheKeyFieldByName(fields []KeyField, name string) (KeyField, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field, true
		}
	}
	return KeyField{}, false
}

func (r *Resolver) populateSubscriptionEntityCache(ctx context.Context, loader *Loader, backend LoaderCache, cfg *SubscriptionEntityCachePopulation, entries []*CacheEntry) {
	if err := loader.cacheAnalyticsL2Set(ctx, backend, entries, cfg.DataSourceName, cfg.CacheName); err != nil {
		loader.recordCacheOperationError("l2_set", cfg.CacheName, cacheAnalyticsFirstEntryKey(entries), err)
		return
	}
	for _, entry := range entries {
		if r.options.OnSubscriptionCacheWrite == nil {
			continue
		}
		r.options.OnSubscriptionCacheWrite(CacheWriteEvent{
			Key:        entry.Key,
			CacheKey:   entry.Key,
			EntityType: cfg.EntityTypeName,
			Kind:       CacheAnalyticsEventKindL2Write,
			Bytes:      len(entry.Value),
			ByteSize:   len(entry.Value),
			TTL:        entry.TTL,
			Reason:     entry.WriteReason,
			DataSource: cfg.DataSourceName,
			CacheLevel: CacheLevelL2,
			Source:     CacheSourceSubscription,
		})
	}
}

func (r *Resolver) invalidateSubscriptionEntityCache(ctx context.Context, loader *Loader, backend LoaderCache, cfg *SubscriptionEntityCachePopulation, keys []string) {
	if err := backend.Delete(ctx, keys); err != nil {
		loader.recordCacheOperationError("l2_delete", cfg.CacheName, cacheAnalyticsFirstKey(keys), err)
		return
	}
	if r.options.OnSubscriptionCacheInvalidate != nil {
		r.options.OnSubscriptionCacheInvalidate(cfg.EntityTypeName, append([]string(nil), keys...))
	}
}
