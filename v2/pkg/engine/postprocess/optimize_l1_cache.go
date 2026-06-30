package postprocess

import (
	"bytes"
	"cmp"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type optimizeL1Cache struct {
	disable bool
}

type entityFetchInfo struct {
	fetchID      int
	coordinate   string
	entityType   string
	providesData *resolve.Object
	dependsOn    []int
	fetch        resolve.Fetch
}

func (o *optimizeL1Cache) processTrees(roots ...*resolve.FetchTreeNode) {
	if o.disable {
		return
	}

	var entities []*entityFetchInfo
	for _, root := range roots {
		entities = append(entities, o.collectEntityFetches(root)...)
	}
	if len(entities) == 0 {
		return
	}

	slices.SortFunc(entities, func(a, b *entityFetchInfo) int {
		return cmp.Or(
			cmp.Compare(a.fetchID, b.fetchID),
			cmp.Compare(a.coordinate, b.coordinate),
		)
	})

	eligibleEntities := make([]*entityFetchInfo, 0, len(entities))
	for _, entity := range entities {
		if cacheL1Eligible(entity.fetch) {
			eligibleEntities = append(eligibleEntities, entity)
		}
	}

	for _, entity := range eligibleEntities {
		if !o.hasValidProvider(entity, eligibleEntities, entities) && !o.hasValidConsumer(entity, eligibleEntities, entities) {
			o.setL1(entity.fetch, false)
		}
	}
}

func (o *optimizeL1Cache) collectEntityFetches(node *resolve.FetchTreeNode) []*entityFetchInfo {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if node.Item == nil {
			return nil
		}
		if entity := o.extractEntityFetchInfo(node.Item.Fetch); entity != nil {
			return []*entityFetchInfo{entity}
		}
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		var result []*entityFetchInfo
		for _, child := range node.ChildNodes {
			result = append(result, o.collectEntityFetches(child)...)
		}
		return result
	}

	return nil
}

func (o *optimizeL1Cache) extractEntityFetchInfo(fetch resolve.Fetch) *entityFetchInfo {
	if fetch == nil {
		return nil
	}

	info := fetch.FetchInfo()
	if info == nil || len(info.RootFields) == 0 {
		return nil
	}

	deps := fetch.Dependencies()
	if deps == nil {
		return nil
	}

	if !isEntityFetch(fetch) {
		return nil
	}

	entityType := info.RootFields[0].TypeName
	if entityType == "" {
		return nil
	}

	cfg := cacheConfig(fetch)
	if cfg == nil {
		return nil
	}

	return &entityFetchInfo{
		fetchID:      deps.FetchID,
		coordinate:   info.RootFields[0].TypeName + "." + info.RootFields[0].FieldName,
		entityType:   entityType,
		providesData: cfg.ProvidesData,
		dependsOn:    deps.DependsOnFetchIDs,
		fetch:        fetch,
	}
}

func isEntityFetch(fetch resolve.Fetch) bool {
	switch f := fetch.(type) {
	case *resolve.EntityFetch:
		return true
	case *resolve.BatchEntityFetch:
		return true
	case *resolve.SingleFetch:
		return f.RequiresEntityFetch || f.RequiresEntityBatchFetch
	default:
		return false
	}
}

func cacheConfig(fetch resolve.Fetch) *resolve.FetchCacheConfig {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return f.Cache
	case *resolve.EntityFetch:
		return f.Cache
	case *resolve.BatchEntityFetch:
		return f.Cache
	default:
		return nil
	}
}

func cacheL1Eligible(fetch resolve.Fetch) bool {
	cfg := cacheConfig(fetch)
	return cfg != nil && cfg.L1
}

func (o *optimizeL1Cache) hasValidProvider(consumer *entityFetchInfo, candidates, allFetches []*entityFetchInfo) bool {
	for _, provider := range candidates {
		if provider.fetchID == consumer.fetchID {
			continue
		}
		if provider.entityType != consumer.entityType {
			continue
		}
		if !o.executesBefore(provider, consumer, allFetches) {
			continue
		}
		if objectProvidesAllFields(provider.providesData, consumer.providesData) {
			return true
		}
	}

	union := o.collectAncestorUnion(consumer, candidates, allFetches)
	return union != nil && objectProvidesAllFields(union, consumer.providesData)
}

func (o *optimizeL1Cache) hasValidConsumer(provider *entityFetchInfo, candidates, allFetches []*entityFetchInfo) bool {
	for _, consumer := range candidates {
		if consumer.fetchID == provider.fetchID {
			continue
		}
		if consumer.entityType != provider.entityType {
			continue
		}
		if !o.executesBefore(provider, consumer, allFetches) {
			continue
		}
		if objectProvidesAllFields(provider.providesData, consumer.providesData) {
			return true
		}

		union := o.collectAncestorUnion(consumer, candidates, allFetches)
		if union != nil && objectProvidesAllFields(union, consumer.providesData) {
			return true
		}
	}

	return false
}

func (o *optimizeL1Cache) executesBefore(a, b *entityFetchInfo, allFetches []*entityFetchInfo) bool {
	if slices.Contains(b.dependsOn, a.fetchID) {
		return true
	}
	return o.isInDependencyChain(b, a.fetchID, allFetches)
}

func (o *optimizeL1Cache) isInDependencyChain(ef *entityFetchInfo, targetID int, allFetches []*entityFetchInfo) bool {
	visited := make(map[int]bool)
	return o.isInDependencyChainRecursive(ef.dependsOn, targetID, allFetches, visited)
}

func (o *optimizeL1Cache) isInDependencyChainRecursive(dependsOn []int, targetID int, allFetches []*entityFetchInfo, visited map[int]bool) bool {
	for _, depID := range dependsOn {
		if depID == targetID {
			return true
		}
		if visited[depID] {
			continue
		}
		visited[depID] = true

		for _, fetch := range allFetches {
			if fetch.fetchID == depID {
				if o.isInDependencyChainRecursive(fetch.dependsOn, targetID, allFetches, visited) {
					return true
				}
				break
			}
		}
	}
	return false
}

func (o *optimizeL1Cache) setL1(fetch resolve.Fetch, value bool) {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		if f.Cache != nil {
			f.Cache.L1 = value
		}
	case *resolve.EntityFetch:
		if f.Cache != nil {
			f.Cache.L1 = value
		}
	case *resolve.BatchEntityFetch:
		if f.Cache != nil {
			f.Cache.L1 = value
		}
	}
}

func objectProvidesAllFields(provider, consumer *resolve.Object) bool {
	if consumer == nil {
		return true
	}
	if provider == nil {
		return len(consumer.Fields) == 0
	}

	for _, consumerField := range consumer.Fields {
		providerField := findFieldByName(provider.Fields, consumerField.Name)
		if providerField == nil {
			return false
		}
		if !nodeProvidesAllFields(providerField.Value, consumerField.Value) {
			return false
		}
	}

	return true
}

func findFieldByName(fields []*resolve.Field, name []byte) *resolve.Field {
	for _, field := range fields {
		if bytes.Equal(field.Name, name) {
			return field
		}
	}
	return nil
}

func nodeProvidesAllFields(provider, consumer resolve.Node) bool {
	if consumer == nil {
		return true
	}
	if provider == nil {
		return false
	}

	switch consumerNode := consumer.(type) {
	case *resolve.Object:
		providerObj, ok := provider.(*resolve.Object)
		if !ok {
			return false
		}
		return objectProvidesAllFields(providerObj, consumerNode)
	case *resolve.Array:
		providerArr, ok := provider.(*resolve.Array)
		if !ok {
			return false
		}
		return nodeProvidesAllFields(providerArr.Item, consumerNode.Item)
	default:
		return true
	}
}

func (o *optimizeL1Cache) treeContainsAllFields(tree *resolve.Object, target *resolve.Object) bool {
	if target == nil || len(target.Fields) == 0 {
		return true
	}
	if tree == nil {
		return false
	}

	if objectProvidesAllFields(tree, target) {
		return true
	}

	for _, field := range tree.Fields {
		if o.nodeContainsAllFields(field.Value, target) {
			return true
		}
	}
	return false
}

func (o *optimizeL1Cache) nodeContainsAllFields(node resolve.Node, target *resolve.Object) bool {
	if node == nil {
		return false
	}

	switch n := node.(type) {
	case *resolve.Object:
		return o.treeContainsAllFields(n, target)
	case *resolve.Array:
		return o.nodeContainsAllFields(n.Item, target)
	default:
		return false
	}
}

func unionObjects(a, b *resolve.Object) *resolve.Object {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	merged := make([]*resolve.Field, 0, len(a.Fields)+len(b.Fields))
	merged = append(merged, a.Fields...)

	for _, bf := range b.Fields {
		existing := findFieldByName(merged, bf.Name)
		if existing == nil {
			merged = append(merged, bf)
			continue
		}

		existingObj, existingIsObj := existing.Value.(*resolve.Object)
		bObj, bIsObj := bf.Value.(*resolve.Object)
		if existingIsObj && bIsObj {
			existing.Value = unionObjects(existingObj, bObj)
		}
	}

	return &resolve.Object{Fields: merged}
}

func (o *optimizeL1Cache) collectAncestorUnion(consumer *entityFetchInfo, candidates, allFetches []*entityFetchInfo) *resolve.Object {
	var union *resolve.Object

	for _, provider := range candidates {
		if provider.fetchID == consumer.fetchID {
			continue
		}
		if provider.entityType != consumer.entityType {
			continue
		}
		if !o.executesBefore(provider, consumer, allFetches) {
			continue
		}
		if provider.providesData != nil {
			union = unionObjects(union, provider.providesData)
		}
	}

	return union
}
