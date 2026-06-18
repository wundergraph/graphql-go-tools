package resolve

import (
	"context"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func navigateProvidesDataToField(providesData *Object, rootFieldName string) *Object {
	field := mutationProvidesDataRootField(providesData, rootFieldName)
	if field == nil {
		return nil
	}
	object, ok := field.Value.(*Object)
	if !ok {
		return nil
	}
	return object
}

func buildEntityKeyValue(a arena.Arena, entityData *astjson.Value, keyFields []KeyField) (*astjson.Value, bool) {
	if len(keyFields) == 0 || entityData == nil || entityData.Type() != astjson.TypeObject {
		return nil, false
	}
	keyObject := astjson.ObjectValue(a)
	for _, field := range keyFields {
		value, ok := extractKeyFieldValue(a, entityData, field)
		if !ok {
			return nil, false
		}
		keyObject.Set(a, field.Name, value)
	}
	if objectLen(keyObject) == 0 {
		return nil, false
	}
	return keyObject, true
}

func buildMutationEntityCacheKey(loader *Loader, cache *FetchCacheConfiguration, entityData *astjson.Value, subgraphName string) (string, bool) {
	if loader == nil || cache == nil || cache.MutationEntityImpactConfig == nil {
		return "", false
	}
	cfg := cache.MutationEntityImpactConfig
	keyValue, ok := buildEntityKeyValue(loader.jsonArena, entityData, cfg.KeyFields)
	if !ok {
		return "", false
	}
	key := buildEntityKeyString(loader.jsonArena, cfg.EntityTypeName, keyValue, "")
	transformCache := &FetchCacheConfiguration{
		CacheName:                   cfg.CacheName,
		IncludeSubgraphHeaderPrefix: cfg.IncludeSubgraphHeaderPrefix,
	}
	return loader.transformL2CacheKey(transformCache, subgraphName, key), true
}

func (l *Loader) detectMutationEntityImpact(cache *FetchCacheConfiguration, responseData *astjson.Value, subgraphName string, writtenKeys map[string]struct{}) map[string]struct{} {
	deletedKeys := map[string]struct{}{}
	if l == nil || l.info == nil || l.info.OperationType != ast.OperationTypeMutation {
		return deletedKeys
	}
	if cache == nil || cache.MutationEntityImpactConfig == nil || responseData == nil || responseData.Type() != astjson.TypeObject {
		return deletedKeys
	}
	cfg := cache.MutationEntityImpactConfig
	if !cfg.InvalidateCache && !cfg.PopulateCache {
		return deletedKeys
	}
	if cache.ProvidesData == nil || len(l.caches) == 0 {
		return deletedKeys
	}
	backend := l.caches[cfg.CacheName]
	if backend == nil {
		return deletedKeys
	}
	rootField := mutationProvidesDataRootField(cache.ProvidesData, "")
	if rootField == nil {
		return deletedKeys
	}
	entityShape, ok := rootField.Value.(*Object)
	if !ok {
		return deletedKeys
	}
	rootFieldName := fieldSchemaName(rootField)
	entityData := responseData.Get(string(rootField.Name))
	if entityData == nil || entityData.Type() == astjson.TypeNull {
		return deletedKeys
	}
	entities := mutationEntityImpactValues(entityData)
	if len(entities) == 0 {
		return deletedKeys
	}

	keysToDelete := make([]string, 0, len(entities))
	for _, entity := range entities {
		key, ok := buildMutationEntityCacheKey(l, cache, entity, subgraphName)
		if !ok {
			continue
		}
		if cfg.PopulateCache && l.ctx != nil && l.ctx.ExecutionOptions.Caching.EnableL2Cache {
			copied := l.structuralCopyNormalized(entity, entityShape)
			entry := &CacheEntry{
				Key:         key,
				Value:       copied.MarshalTo(nil),
				TTL:         cfg.PopulateTTL,
				WriteReason: CacheWriteReasonRefresh,
			}
			if err := l.cacheAnalyticsL2Set(contextForLoader(l), backend, []*CacheEntry{entry}, subgraphName, cfg.CacheName); err != nil {
				l.recordCacheOperationError("l2_set", cfg.CacheName, key, err)
			}
			l.recordL2Write(cache, entry)
			if writtenKeys == nil {
				writtenKeys = map[string]struct{}{}
			}
			writtenKeys[key] = struct{}{}
			l.recordMutationEvent(MutationEvent{
				EntityType: cfg.EntityTypeName,
				Operation:  rootFieldName,
				Key:        key,
				Written:    true,
			})
		}
		if !cfg.InvalidateCache {
			continue
		}
		if _, written := writtenKeys[key]; written {
			continue
		}
		keysToDelete = append(keysToDelete, key)
		deletedKeys[key] = struct{}{}
		l.recordMutationEvent(MutationEvent{
			EntityType: cfg.EntityTypeName,
			Operation:  rootFieldName,
			Key:        key,
			Deleted:    true,
		})
	}
	if len(keysToDelete) == 0 {
		return deletedKeys
	}
	if err := backend.Delete(contextForLoader(l), keysToDelete); err != nil {
		l.recordCacheOperationError("l2_delete", cfg.CacheName, cacheAnalyticsFirstKey(keysToDelete), err)
	}
	return deletedKeys
}

func mutationProvidesDataRootField(providesData *Object, rootFieldName string) *Field {
	if providesData == nil {
		return nil
	}
	for _, field := range providesData.Fields {
		if field == nil {
			continue
		}
		if rootFieldName == "" || fieldSchemaName(field) == rootFieldName || string(field.Name) == rootFieldName {
			return field
		}
	}
	return nil
}

func mutationEntityImpactValues(value *astjson.Value) []*astjson.Value {
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

func (l *Loader) recordMutationEvent(event MutationEvent) {
	collector := l.cacheAnalytics()
	if collector == nil {
		return
	}
	collector.recordMutationEvent(event)
}

func contextForLoader(l *Loader) context.Context {
	if l == nil || l.ctx == nil || l.ctx.ctx == nil {
		return context.Background()
	}
	return l.ctx.ctx
}
