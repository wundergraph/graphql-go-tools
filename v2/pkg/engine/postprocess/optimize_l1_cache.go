package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type optimizeL1Cache struct {
	disable bool
}

type entityFetchInfo struct {
	cache    *resolve.FetchCacheConfiguration
	entity   string
	provides *resolve.Object
	stage    int
	canRead  bool
	canWrite bool
}

func (o *optimizeL1Cache) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if o.disable {
		return
	}

	stages := collectEntityFetchStages(root)
	if len(stages) == 0 {
		return
	}

	var fetches []*entityFetchInfo
	priorProviders := make(map[string][]*resolve.Object)
	for _, stage := range stages {
		for _, fetch := range stage {
			providers := priorProviders[fetch.entity]
			fetch.canRead = fetch.provides != nil && objectsProvideAllFields(providers, fetch.provides)
		}
		for _, fetch := range stage {
			priorProviders[fetch.entity] = append(priorProviders[fetch.entity], fetch.provides)
			fetches = append(fetches, fetch)
		}
	}

	for _, provider := range fetches {
		for _, consumer := range fetches {
			if consumer.stage <= provider.stage ||
				consumer.entity != provider.entity ||
				!objectIntersectsFields(provider.provides, consumer.provides) {
				continue
			}
			if objectsProvideAllFields(fetchProvidesBeforeStage(fetches, consumer.entity, consumer.stage), consumer.provides) {
				provider.canWrite = true
				break
			}
		}
	}

	for _, fetch := range fetches {
		fetch.cache.UseL1Cache = fetch.canRead || fetch.canWrite
	}
}

func collectEntityFetchStages(root *resolve.FetchTreeNode) [][]*entityFetchInfo {
	var stageIndex int
	var stages [][]*entityFetchInfo
	appendStages(root, &stageIndex, &stages)
	return stages
}

func appendStages(node *resolve.FetchTreeNode, stageIndex *int, stages *[][]*entityFetchInfo) {
	if node == nil {
		return
	}

	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		info := entityInfoForFetch(node.Item)
		if info == nil {
			return
		}
		info.stage = *stageIndex
		*stages = append(*stages, []*entityFetchInfo{info})
		(*stageIndex)++
	case resolve.FetchTreeNodeKindSequence:
		for _, child := range node.ChildNodes {
			appendStages(child, stageIndex, stages)
		}
	case resolve.FetchTreeNodeKindParallel:
		stage := collectParallelEntityFetches(node, *stageIndex)
		if len(stage) == 0 {
			return
		}
		*stages = append(*stages, stage)
		(*stageIndex)++
	case resolve.FetchTreeNodeKindTrigger:
		appendStages(node.Trigger, stageIndex, stages)
		for _, child := range node.ChildNodes {
			appendStages(child, stageIndex, stages)
		}
	}
}

func collectParallelEntityFetches(node *resolve.FetchTreeNode, stage int) []*entityFetchInfo {
	if node == nil {
		return nil
	}
	if node.Kind == resolve.FetchTreeNodeKindSingle {
		info := entityInfoForFetch(node.Item)
		if info == nil {
			return nil
		}
		info.stage = stage
		return []*entityFetchInfo{info}
	}

	var fetches []*entityFetchInfo
	for _, child := range node.ChildNodes {
		fetches = append(fetches, collectParallelEntityFetches(child, stage)...)
	}
	return fetches
}

func entityInfoForFetch(item *resolve.FetchItem) *entityFetchInfo {
	if item == nil {
		return nil
	}

	switch fetch := item.Fetch.(type) {
	case *resolve.EntityFetch:
		return entityInfoForCache(fetch.Cache)
	case *resolve.BatchEntityFetch:
		return entityInfoForCache(fetch.Cache)
	default:
		return nil
	}
}

func entityInfoForCache(cache *resolve.FetchCacheConfiguration) *entityFetchInfo {
	if cache == nil {
		return nil
	}

	entity := entityTypeName(cache)
	if entity == "" {
		return nil
	}

	return &entityFetchInfo{
		cache:    cache,
		entity:   entity,
		provides: cache.ProvidesData,
	}
}

func entityTypeName(cache *resolve.FetchCacheConfiguration) string {
	template, ok := cache.KeyTemplate.(*resolve.EntityQueryCacheKeyTemplate)
	if !ok {
		return ""
	}
	if template.TypeName != "" {
		return template.TypeName
	}
	if cache.ProvidesData != nil {
		return cache.ProvidesData.TypeName
	}
	return ""
}

func fetchProvidesBeforeStage(fetches []*entityFetchInfo, entity string, stage int) []*resolve.Object {
	var providers []*resolve.Object
	for _, fetch := range fetches {
		if fetch.entity == entity && fetch.stage < stage {
			providers = append(providers, fetch.provides)
		}
	}
	return providers
}

func objectProvidesAllFields(provider *resolve.Object, needs *resolve.Object) bool {
	return objectsProvideAllFields([]*resolve.Object{provider}, needs)
}

func objectsProvideAllFields(providers []*resolve.Object, needs *resolve.Object) bool {
	if needs == nil {
		return true
	}
	if len(providers) == 0 {
		return false
	}

	for _, neededField := range needs.Fields {
		if neededField == nil {
			continue
		}
		providerFields := matchingProviderFields(providers, neededField)
		if len(providerFields) == 0 {
			return false
		}
		if !fieldsProvideValue(providerFields, neededField.Value) {
			return false
		}
	}

	return true
}

func matchingProviderFields(providers []*resolve.Object, neededField *resolve.Field) []*resolve.Field {
	var providerFields []*resolve.Field
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		for _, providerField := range provider.Fields {
			if fieldsHaveSameCacheKey(providerField, neededField) {
				providerFields = append(providerFields, providerField)
			}
		}
	}
	return providerFields
}

func fieldsProvideValue(providerFields []*resolve.Field, neededNode resolve.Node) bool {
	switch needed := neededNode.(type) {
	case *resolve.Object:
		var providers []*resolve.Object
		for _, providerField := range providerFields {
			provider, ok := providerField.Value.(*resolve.Object)
			if ok {
				providers = append(providers, provider)
			}
		}
		return objectsProvideAllFields(providers, needed)
	case *resolve.Array:
		neededItem, ok := needed.Item.(*resolve.Object)
		if !ok {
			return true
		}
		var providers []*resolve.Object
		for _, providerField := range providerFields {
			providerArray, ok := providerField.Value.(*resolve.Array)
			if !ok {
				continue
			}
			providerItem, ok := providerArray.Item.(*resolve.Object)
			if ok {
				providers = append(providers, providerItem)
			}
		}
		return objectsProvideAllFields(providers, neededItem)
	default:
		return true
	}
}

func objectIntersectsFields(provider *resolve.Object, needs *resolve.Object) bool {
	if provider == nil || needs == nil {
		return false
	}

	for _, neededField := range needs.Fields {
		if neededField == nil {
			continue
		}
		for _, providerField := range provider.Fields {
			if !fieldsHaveSameCacheKey(providerField, neededField) {
				continue
			}
			if fieldsIntersectValue(providerField.Value, neededField.Value) {
				return true
			}
		}
	}

	return false
}

func fieldsIntersectValue(providerNode resolve.Node, neededNode resolve.Node) bool {
	switch needed := neededNode.(type) {
	case *resolve.Object:
		provider, ok := providerNode.(*resolve.Object)
		return ok && objectIntersectsFields(provider, needed)
	case *resolve.Array:
		neededItem, ok := needed.Item.(*resolve.Object)
		if !ok {
			return true
		}
		providerArray, ok := providerNode.(*resolve.Array)
		if !ok {
			return false
		}
		providerItem, ok := providerArray.Item.(*resolve.Object)
		return ok && objectIntersectsFields(providerItem, neededItem)
	default:
		return true
	}
}

func fieldsHaveSameCacheKey(providerField *resolve.Field, neededField *resolve.Field) bool {
	if providerField == nil || neededField == nil {
		return false
	}
	return fieldSchemaName(providerField) == fieldSchemaName(neededField) &&
		slices.Equal(providerField.CacheArgs, neededField.CacheArgs)
}

func fieldSchemaName(field *resolve.Field) string {
	if len(field.OriginalName) > 0 {
		return string(field.OriginalName)
	}
	return string(field.Name)
}
