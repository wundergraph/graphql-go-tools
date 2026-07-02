package cache

import (
	"cmp"
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// optimizeL1Cache narrows cfg.L1 CROSS-TREE after all fetch configs are set:
// L1 stays enabled only where a request-lifetime provider/consumer pair exists
// — a fetch whose value no later fetch can reuse (canWrite) and that no prior
// fetch can serve (canRead) drops its eligibility. The configurator is the
// SOLE eligibility setter; this pass NEVER turns L1 on. Wrong narrowing costs
// only a missed hit, never correctness.
type optimizeL1Cache struct {
	// treeParents is the per-run tree ancestry (index of the enclosing tree,
	// -1 for roots); see optimize.
	treeParents []int
}

// entityFetchInfo is the narrowing view of one L1-relevant entity fetch.
type entityFetchInfo struct {
	fetchID    int
	coordinate string
	entityType string
	// treeIndex is the fetch's tree position in the facade's execution-ordered
	// tree list: 0 is the initial response tree, 1+ are defer groups.
	treeIndex    int
	providesData *resolve.Object
	dependsOn    []int
	fetch        resolve.Fetch
}

// optimize runs the cross-tree narrowing over ALL fetch trees of one response
// (the root tree and every defer group — the L1 store is request-lifetime, so
// provider/consumer pairs span trees).
func (o *optimizeL1Cache) optimize(trees []*resolve.FetchTreeNode, treeParents []int) {
	// treeParents encodes the defer-group ancestry (parent tree index per
	// tree, -1 for roots); nil defaults to "tree 0 encloses every other
	// tree" — the plain root-before-defers rule.
	o.treeParents = treeParents
	if len(o.treeParents) != len(trees) {
		o.treeParents = make([]int, len(trees))
		for i := range o.treeParents {
			o.treeParents[i] = 0
		}
		if len(o.treeParents) > 0 {
			o.treeParents[0] = -1
		}
	}
	var entities []*entityFetchInfo
	// dependencies indexes EVERY fetch in the trees (cached or not): a
	// provider/consumer chain routinely passes THROUGH unconfigured fetches
	// (products -> reviews -> products), and a chain walk restricted to cached
	// entity fetches would break at the middle hop.
	dependencies := make(map[int][]int)
	for treeIndex, tree := range trees {
		collectFetchDependencies(tree, dependencies)
		collected := o.collectEntityFetches(tree)
		for _, entity := range collected {
			entity.treeIndex = treeIndex
		}
		entities = append(entities, collected...)
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

	eligible := make([]*entityFetchInfo, 0, len(entities))
	for _, entity := range entities {
		if cfg := entity.fetch.CacheConfig(); cfg != nil && cfg.L1 {
			eligible = append(eligible, entity)
		}
	}

	for _, entity := range eligible {
		if o.hasValidProvider(entity, eligible, dependencies) || o.hasValidConsumer(entity, eligible, dependencies) {
			continue
		}
		cfg := entity.fetch.CacheConfig()
		cfg.L1 = false
		if !cfg.L2 && !cfg.ShadowMode {
			// The config enables nothing anymore: re-nil it so the loader's
			// per-fetch gate skips the fetch entirely (tidy, not correctness).
			entity.fetch.SetCacheConfig(nil)
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
	if fetch == nil || fetch.CacheConfig() == nil {
		return nil
	}
	if !fetch.IsEntityFetch() && !fetch.IsBatchEntityFetch() {
		return nil
	}
	info := fetch.FetchInfo()
	if info == nil || len(info.RootFields) == 0 || info.RootFields[0].TypeName == "" {
		return nil
	}
	deps := fetch.Dependencies()
	if deps == nil {
		return nil
	}
	return &entityFetchInfo{
		fetchID:      deps.FetchID,
		coordinate:   info.RootFields[0].TypeName + "." + info.RootFields[0].FieldName,
		entityType:   info.RootFields[0].TypeName,
		providesData: fetch.CacheConfig().ProvidesData,
		dependsOn:    deps.DependsOnFetchIDs,
		fetch:        fetch,
	}
}

// hasValidProvider reports canRead: a prior fetch of the same entity type (or
// the union of all priors) provides a SUPERSET of this fetch's tree.
func (o *optimizeL1Cache) hasValidProvider(consumer *entityFetchInfo, candidates []*entityFetchInfo, dependencies map[int][]int) bool {
	for _, provider := range candidates {
		if provider.fetchID == consumer.fetchID {
			continue
		}
		if provider.entityType != consumer.entityType {
			continue
		}
		if !o.executesBefore(provider, consumer, dependencies) {
			continue
		}
		if objectProvidesAllFields(provider.providesData, consumer.providesData) {
			return true
		}
	}
	union := o.collectAncestorUnion(consumer, candidates, dependencies)
	return union != nil && objectProvidesAllFields(union, consumer.providesData)
}

// hasValidConsumer reports canWrite: a later fetch of the same entity type
// needs a SUBSET of this fetch's tree, either from this fetch alone or from a
// union THIS FETCH CONTRIBUTES TO. The contribution gate is the fix for the
// OLD union-fallback flaw: without it, a provider sharing a consumer with
// other, sufficient providers stayed L1-eligible although nothing ever reused
// its cached data.
func (o *optimizeL1Cache) hasValidConsumer(provider *entityFetchInfo, candidates []*entityFetchInfo, dependencies map[int][]int) bool {
	for _, consumer := range candidates {
		if consumer.fetchID == provider.fetchID {
			continue
		}
		if consumer.entityType != provider.entityType {
			continue
		}
		if !o.executesBefore(provider, consumer, dependencies) {
			continue
		}
		if objectProvidesAllFields(provider.providesData, consumer.providesData) {
			return true
		}
		if !objectSharesAnyField(provider.providesData, consumer.providesData) {
			continue
		}
		union := o.collectAncestorUnion(consumer, candidates, dependencies)
		if union != nil && objectProvidesAllFields(union, consumer.providesData) {
			return true
		}
	}
	return false
}

// executesBefore resolves ordering from two sources: the dependency edges
// (direct or transitive), and TREE order — the initial response tree always
// completes before any defer group starts, so an initial-tree fetch executes
// before every defer-group fetch even across branches with no dependency
// edge. Defer groups among themselves stay unordered (conservative: they may
// run in parallel).
func (o *optimizeL1Cache) executesBefore(a, b *entityFetchInfo, dependencies map[int][]int) bool {
	if o.treeEncloses(a.treeIndex, b.treeIndex) {
		return true
	}
	visited := make(map[int]bool)
	return o.isInDependencyChain(b.dependsOn, a.fetchID, dependencies, visited)
}

// treeEncloses reports whether tree a is a strict ancestor of tree b: the
// resolver resolves a parent defer group fully before its children (and the
// initial tree before every group), so ancestor-tree fetches execute before
// every descendant-tree fetch. SIBLING groups stay unordered (parallel).
// The walk is bounded by the tree count: treeParents crosses the public
// ConfigureCaching API, and a malformed parent cycle must terminate, never
// hang. Within such a cycle direct parent edges may still report enclosure —
// harmless: L1 is only an eligibility hint, and the runtime still gates every
// serve on key and coverage matches against actually-cached values.
func (o *optimizeL1Cache) treeEncloses(a, b int) bool {
	if a == b {
		return false
	}
	cur := b
	for range o.treeParents {
		if cur < 0 || cur >= len(o.treeParents) {
			return false
		}
		parent := o.treeParents[cur]
		if parent == a {
			return true
		}
		if parent == cur {
			return false
		}
		cur = parent
	}
	return false
}

func (o *optimizeL1Cache) isInDependencyChain(dependsOn []int, targetID int, dependencies map[int][]int, visited map[int]bool) bool {
	for _, depID := range dependsOn {
		if depID == targetID {
			return true
		}
		if visited[depID] {
			continue
		}
		visited[depID] = true
		if o.isInDependencyChain(dependencies[depID], targetID, dependencies, visited) {
			return true
		}
	}
	return false
}

// collectFetchDependencies records fetchID -> DependsOnFetchIDs for every
// fetch under node.
func collectFetchDependencies(node *resolve.FetchTreeNode, out map[int][]int) {
	if node == nil {
		return
	}
	if node.Item != nil && node.Item.Fetch != nil {
		if deps := node.Item.Fetch.Dependencies(); deps != nil {
			out[deps.FetchID] = deps.DependsOnFetchIDs
		}
	}
	for _, child := range node.ChildNodes {
		collectFetchDependencies(child, out)
	}
}

// collectAncestorUnion unions the trees of every same-type provider executing
// before the consumer.
func (o *optimizeL1Cache) collectAncestorUnion(consumer *entityFetchInfo, candidates []*entityFetchInfo, dependencies map[int][]int) *resolve.Object {
	var union *resolve.Object
	for _, provider := range candidates {
		if provider.fetchID == consumer.fetchID {
			continue
		}
		if provider.entityType != consumer.entityType {
			continue
		}
		if !o.executesBefore(provider, consumer, dependencies) {
			continue
		}
		if provider.providesData != nil {
			union = unionObjects(union, provider.providesData)
		}
	}
	return union
}

// fieldNarrowingName identifies a field for provider/consumer matching: the
// SCHEMA name (alias-independent — the L1 store holds normalized values) plus
// the argument bindings (fields selected with different arguments are
// different cache fields). CacheArgs are sorted at capture time (task 05), so
// plain concatenation is deterministic.
func fieldNarrowingName(field *resolve.Field) string {
	name := string(field.Name)
	if len(field.OriginalName) > 0 {
		name = string(field.OriginalName)
	}
	if len(field.CacheArgs) == 0 {
		return name
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('(')
	for i, arg := range field.CacheArgs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(arg.Name)
		b.WriteByte(':')
		b.WriteString(arg.VariableName)
	}
	b.WriteByte(')')
	return b.String()
}

func findFieldByNarrowingName(fields []*resolve.Field, name string) *resolve.Field {
	for _, field := range fields {
		if fieldNarrowingName(field) == name {
			return field
		}
	}
	return nil
}

// objectProvidesAllFields reports whether the provider tree contains EVERY
// field of the consumer tree, recursively.
func objectProvidesAllFields(provider, consumer *resolve.Object) bool {
	if consumer == nil {
		return true
	}
	if provider == nil {
		return len(consumer.Fields) == 0
	}
	for _, consumerField := range consumer.Fields {
		providerField := findFieldByNarrowingName(provider.Fields, fieldNarrowingName(consumerField))
		if providerField == nil {
			return false
		}
		if !nodeProvidesAllFields(providerField.Value, consumerField.Value) {
			return false
		}
	}
	return true
}

// objectSharesAnyField reports whether the provider supplies at least one
// field PATH the consumer needs — the union-fallback contribution gate. The
// walk is recursive: sharing only a nested object's NAME while providing none
// of its leaves the consumer reads is not a contribution.
func objectSharesAnyField(provider, consumer *resolve.Object) bool {
	if provider == nil || consumer == nil {
		return false
	}
	for _, consumerField := range consumer.Fields {
		providerField := findFieldByNarrowingName(provider.Fields, fieldNarrowingName(consumerField))
		if providerField != nil && nodeSharesAnyField(providerField.Value, consumerField.Value) {
			return true
		}
	}
	return false
}

func nodeSharesAnyField(provider, consumer resolve.Node) bool {
	if provider == nil || consumer == nil {
		return false
	}
	switch consumerNode := consumer.(type) {
	case *resolve.Object:
		providerObj, ok := provider.(*resolve.Object)
		return ok && objectSharesAnyField(providerObj, consumerNode)
	case *resolve.Array:
		providerArr, ok := provider.(*resolve.Array)
		return ok && nodeSharesAnyField(providerArr.Item, consumerNode.Item)
	default:
		return true
	}
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

// unionObjects merges two trees into a NEW object. Overlapping object fields
// merge recursively into COPIED field structs — the inputs are live
// ProvidesData trees on the fetch configs, and mutating them here would
// corrupt the configs (a first-pass aliasing bug this port fixes).
func unionObjects(a, b *resolve.Object) *resolve.Object {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	merged := make([]*resolve.Field, 0, len(a.Fields)+len(b.Fields))
	merged = append(merged, a.Fields...)
	for _, bField := range b.Fields {
		name := fieldNarrowingName(bField)
		existingIdx := slices.IndexFunc(merged, func(f *resolve.Field) bool {
			return fieldNarrowingName(f) == name
		})
		if existingIdx < 0 {
			merged = append(merged, bField)
			continue
		}
		existingObj, existingIsObj := merged[existingIdx].Value.(*resolve.Object)
		bObj, bIsObj := bField.Value.(*resolve.Object)
		if existingIsObj && bIsObj {
			fieldCopy := *merged[existingIdx]
			fieldCopy.Value = unionObjects(existingObj, bObj)
			merged[existingIdx] = &fieldCopy
		}
	}
	return &resolve.Object{Fields: merged}
}
