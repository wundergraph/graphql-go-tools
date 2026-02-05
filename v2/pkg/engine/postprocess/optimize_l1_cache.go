package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// optimizeL1Cache is a postprocessor that optimizes L1 cache usage by only enabling it
// for fetches that can actually benefit from cache hits. This saves memory and CPU
// by skipping cache key generation, lookup, and population when L1 cannot help.
//
// L1 cache is effective when:
// 1. A prior fetch (parent query) returns the same entity type (current fetch can READ)
// 2. A later fetch needs the same entity type with a subset of fields (current fetch can WRITE)
//
// A fetch never reads AND writes to L1 in the same execution:
// - Cache hit (READ): Fetch reads from L1, skips subgraph fetch, does NOT write
// - Cache miss (WRITE): Fetch cannot read, makes subgraph call, then writes to L1
type optimizeL1Cache struct {
	disable bool
}

// entityFetchInfo stores information about an entity fetch needed for L1 optimization
type entityFetchInfo struct {
	fetchID      int
	entityType   string          // From FetchInfo.RootFields[0].TypeName
	providesData *resolve.Object // From FetchInfo.ProvidesData - the full field tree
	dependsOn    []int           // From FetchDependencies.DependsOnFetchIDs
	fetch        resolve.Fetch   // Reference to the actual fetch for modification
}

// rootFieldProviderInfo stores information about a root field fetch that can provide L1 cache data
type rootFieldProviderInfo struct {
	fetchID      int
	entityTypes  []string        // Entity types this root field can populate L1 for
	providesData *resolve.Object // From FetchInfo.ProvidesData - the full response tree
	fetch        resolve.Fetch   // Reference to the actual fetch for modification
}

func (o *optimizeL1Cache) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if o.disable || root == nil {
		return
	}

	// Phase 1: Collect entity fetch information from entire tree
	entityFetches := o.collectEntityFetches(root)

	// Also collect root field providers (root fields with RootFieldL1EntityCacheKeyTemplates)
	rootFieldProviderInfos := o.collectRootFieldProviders(root)

	// No fetches to optimize
	if len(entityFetches) == 0 && len(rootFieldProviderInfos) == 0 {
		return
	}

	// Phase 2: Build reverse dependency map and group by entity type
	byEntityType := make(map[string][]*entityFetchInfo)
	for _, ef := range entityFetches {
		byEntityType[ef.entityType] = append(byEntityType[ef.entityType], ef)
	}

	// Phase 3: Determine L1 usefulness for each entity fetch
	for _, ef := range entityFetches {
		canRead := o.hasValidProvider(ef, entityFetches, rootFieldProviderInfos)
		canWrite := o.hasValidConsumer(ef, entityFetches)
		useL1Cache := canRead || canWrite
		o.setUseL1Cache(ef.fetch, useL1Cache)
	}

	// Phase 4: Determine L1 usefulness for each root field provider
	// Root fields only write to L1, so they need valid consumers to be useful
	for _, rfp := range rootFieldProviderInfos {
		canWrite := o.rootFieldHasValidConsumer(rfp, entityFetches)
		o.setUseL1Cache(rfp.fetch, canWrite)
	}
}

// collectEntityFetches traverses the fetch tree and collects information about entity fetches
func (o *optimizeL1Cache) collectEntityFetches(node *resolve.FetchTreeNode) []*entityFetchInfo {
	if node == nil {
		return nil
	}

	var result []*entityFetchInfo

	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if ef := o.extractEntityFetchInfo(node.Item.Fetch); ef != nil {
			result = append(result, ef)
		}
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		for _, child := range node.ChildNodes {
			result = append(result, o.collectEntityFetches(child)...)
		}
	}

	return result
}

// extractEntityFetchInfo extracts entity fetch information from a fetch if applicable
func (o *optimizeL1Cache) extractEntityFetchInfo(fetch resolve.Fetch) *entityFetchInfo {
	if fetch == nil {
		return nil
	}

	info := fetch.FetchInfo()
	if info == nil {
		return nil
	}

	deps := fetch.Dependencies()
	if deps == nil {
		return nil
	}

	// Check if this is an entity fetch (has root fields with TypeName)
	if len(info.RootFields) == 0 {
		return nil
	}

	// Only entity fetches (EntityFetch, BatchEntityFetch, or SingleFetch with RequiresEntityFetch)
	// have meaningful L1 cache potential
	isEntityFetch := false
	switch f := fetch.(type) {
	case *resolve.EntityFetch:
		isEntityFetch = true
	case *resolve.BatchEntityFetch:
		isEntityFetch = true
	case *resolve.SingleFetch:
		isEntityFetch = f.RequiresEntityFetch || f.RequiresEntityBatchFetch
	}

	if !isEntityFetch {
		return nil
	}

	entityType := info.RootFields[0].TypeName
	if entityType == "" {
		return nil
	}

	return &entityFetchInfo{
		fetchID:      deps.FetchID,
		entityType:   entityType,
		providesData: info.ProvidesData,
		dependsOn:    deps.DependsOnFetchIDs,
		fetch:        fetch,
	}
}

// collectRootFieldProviders finds root fields that populate L1 cache with entity data
func (o *optimizeL1Cache) collectRootFieldProviders(node *resolve.FetchTreeNode) []*rootFieldProviderInfo {
	var providers []*rootFieldProviderInfo
	o.collectRootFieldProvidersRecursive(node, &providers)
	return providers
}

func (o *optimizeL1Cache) collectRootFieldProvidersRecursive(node *resolve.FetchTreeNode, providers *[]*rootFieldProviderInfo) {
	if node == nil {
		return
	}

	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if node.Item != nil && node.Item.Fetch != nil {
			if sf, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
				if len(sf.Caching.RootFieldL1EntityCacheKeyTemplates) > 0 {
					deps := sf.Dependencies()
					var entityTypes []string
					for entityType := range sf.Caching.RootFieldL1EntityCacheKeyTemplates {
						entityTypes = append(entityTypes, entityType)
					}
					// Get providesData from FetchInfo
					var providesData *resolve.Object
					if sf.Info != nil {
						providesData = sf.Info.ProvidesData
					}
					*providers = append(*providers, &rootFieldProviderInfo{
						fetchID:      deps.FetchID,
						entityTypes:  entityTypes,
						providesData: providesData,
						fetch:        sf,
					})
				}
			}
		}
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		for _, child := range node.ChildNodes {
			o.collectRootFieldProvidersRecursive(child, providers)
		}
	}
}

// rootFieldHasValidConsumer checks if there's a later entity fetch that can benefit from this root field's L1 data
func (o *optimizeL1Cache) rootFieldHasValidConsumer(provider *rootFieldProviderInfo, allEntityFetches []*entityFetchInfo) bool {
	for _, consumer := range allEntityFetches {
		// Check if consumer's entity type matches any type this root field provides
		for _, entityType := range provider.entityTypes {
			if consumer.entityType == entityType {
				// Consumer must execute after provider (fetchID ordering or dependency)
				if provider.fetchID < consumer.fetchID || slices.Contains(consumer.dependsOn, provider.fetchID) {
					// Provider must have all fields that consumer needs (recursive tree search)
					// If providesData is nil, assume provider can provide all fields (runtime validation will reject incomplete data)
					if provider.providesData == nil || o.treeContainsAllFields(provider.providesData, consumer.providesData) {
						return true
					}
				}
			}
		}
	}
	return false
}

// hasValidProvider checks if there's a prior fetch that can provide data for this fetch
// A prior fetch is valid if:
// 1. It provides the same entity type
// 2. It provides a superset of fields (provider has all fields that consumer needs)
// 3. It executes before this fetch (has lower fetchID or is in dependsOn chain)
func (o *optimizeL1Cache) hasValidProvider(consumer *entityFetchInfo, allFetches []*entityFetchInfo, rootFieldProviders []*rootFieldProviderInfo) bool {
	// Check root field providers first
	for _, provider := range rootFieldProviders {
		// Check if provider's entity types include consumer's type
		for _, entityType := range provider.entityTypes {
			if entityType == consumer.entityType {
				// Root field providers always execute before entity fetches that depend on their data
				// Check if this consumer depends (directly or transitively) on the root field
				if provider.fetchID < consumer.fetchID || o.isInDependencyChain(consumer, provider.fetchID, allFetches) {
					// Provider must have all fields that consumer needs (recursive tree search)
					// If providesData is nil, assume provider can provide all fields (runtime validation will reject incomplete data)
					if provider.providesData == nil || o.treeContainsAllFields(provider.providesData, consumer.providesData) {
						return true
					}
				}
			}
		}
	}

	// Check entity fetches
	for _, provider := range allFetches {
		if provider.fetchID == consumer.fetchID {
			continue // Skip self
		}

		// Must be same entity type
		if provider.entityType != consumer.entityType {
			continue
		}

		// Provider must execute before consumer
		if !o.executesBefore(provider, consumer, allFetches) {
			continue
		}

		// Provider must have all fields that consumer needs (recursively)
		if objectProvidesAllFields(provider.providesData, consumer.providesData) {
			return true
		}
	}

	return false
}

// hasValidConsumer checks if there's a later fetch that can benefit from this fetch's L1 data
// A later fetch is a valid consumer if:
// 1. It needs the same entity type
// 2. It needs a subset of fields (consumer needs only fields that provider has)
// 3. It executes after this fetch
func (o *optimizeL1Cache) hasValidConsumer(provider *entityFetchInfo, allFetches []*entityFetchInfo) bool {
	for _, consumer := range allFetches {
		if consumer.fetchID == provider.fetchID {
			continue // Skip self
		}

		// Must be same entity type
		if consumer.entityType != provider.entityType {
			continue
		}

		// Consumer must execute after provider
		if !o.executesBefore(provider, consumer, allFetches) {
			continue
		}

		// Provider must have all fields that consumer needs (recursively)
		if objectProvidesAllFields(provider.providesData, consumer.providesData) {
			return true
		}
	}

	return false
}

// executesBefore returns true if a executes before b based on dependencies
func (o *optimizeL1Cache) executesBefore(a, b *entityFetchInfo, allFetches []*entityFetchInfo) bool {
	// Direct dependency check: b depends on a
	if slices.Contains(b.dependsOn, a.fetchID) {
		return true
	}

	// Transitive dependency check: b depends on something that depends on a
	return o.isInDependencyChain(b, a.fetchID, allFetches)
}

// isInDependencyChain checks if targetID is anywhere in the dependency chain of ef
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

		// Find the fetch with this ID and check its dependencies
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

// setUseL1Cache sets the UseL1Cache flag on the appropriate caching configuration
func (o *optimizeL1Cache) setUseL1Cache(fetch resolve.Fetch, value bool) {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		f.Caching.UseL1Cache = value
	case *resolve.EntityFetch:
		f.Caching.UseL1Cache = value
	case *resolve.BatchEntityFetch:
		f.Caching.UseL1Cache = value
	}
}

// objectProvidesAllFields recursively checks if provider object has all fields that consumer needs.
// This validates the entire field tree, not just top-level fields.
func objectProvidesAllFields(provider, consumer *resolve.Object) bool {
	if consumer == nil {
		return true // Consumer needs nothing
	}
	if provider == nil {
		return len(consumer.Fields) == 0 // Provider has nothing, consumer must need nothing
	}

	// Check each consumer field exists in provider
	for _, consumerField := range consumer.Fields {
		providerField := findFieldByName(provider.Fields, consumerField.Name)
		if providerField == nil {
			return false // Consumer needs field that provider doesn't have
		}

		// Recursively check nested fields
		if !nodeProvidesAllFields(providerField.Value, consumerField.Value) {
			return false
		}
	}

	return true
}

// findFieldByName finds a field by name in a slice of fields
func findFieldByName(fields []*resolve.Field, name []byte) *resolve.Field {
	for _, field := range fields {
		if string(field.Name) == string(name) {
			return field
		}
	}
	return nil
}

// nodeProvidesAllFields recursively checks if provider node has all fields that consumer node needs.
// Handles Object, Array, and scalar types.
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
			return false // Type mismatch
		}
		return objectProvidesAllFields(providerObj, consumerNode)

	case *resolve.Array:
		providerArr, ok := provider.(*resolve.Array)
		if !ok {
			return false // Type mismatch
		}
		// Check the array item type
		return nodeProvidesAllFields(providerArr.Item, consumerNode.Item)

	default:
		// Scalar types (String, Int, Float, Boolean, etc.) - if provider has the field, it's sufficient
		return true
	}
}

// treeContainsAllFields searches the provider tree for any object that provides all fields the target needs.
// This is used for root field providers where entities may be nested anywhere in the response tree.
func (o *optimizeL1Cache) treeContainsAllFields(tree *resolve.Object, target *resolve.Object) bool {
	if target == nil || len(target.Fields) == 0 {
		return true // Consumer needs nothing
	}
	if tree == nil {
		return false // Provider has nothing
	}

	// Check if this object provides all fields
	if objectProvidesAllFields(tree, target) {
		return true
	}

	// Recursively check nested objects in the tree
	for _, field := range tree.Fields {
		if o.nodeContainsAllFields(field.Value, target) {
			return true
		}
	}
	return false
}

// nodeContainsAllFields recursively searches a node for an object that provides all target fields.
func (o *optimizeL1Cache) nodeContainsAllFields(node resolve.Node, target *resolve.Object) bool {
	if node == nil {
		return false
	}

	switch n := node.(type) {
	case *resolve.Object:
		return o.treeContainsAllFields(n, target)
	case *resolve.Array:
		return o.nodeContainsAllFields(n.Item, target)
	}
	return false
}
