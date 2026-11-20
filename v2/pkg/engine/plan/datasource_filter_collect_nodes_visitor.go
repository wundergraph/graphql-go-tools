package plan

import (
	"fmt"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type nodesCollector struct {
	operation   *ast.Document
	definition  *ast.Document
	dataSources []DataSource
	nodes       *NodeSuggestions
	report      *operationreport.Report
	keys        []DSKeyInfo

	seenKeys             map[SeenKeyPath]struct{}
	fieldInfo            map[int]fieldInfo
	unfetchableFieldRefs map[int]struct{}

	// dsIndexesByTypeName are the data sources which know a type, by typeName; see assignField.
	dsIndexesByTypeName map[string][]int
	// providesDS are the data sources with a @provides on a field, by "TypeName.FieldName"
	providesDS map[string][]int
	// providesScopeDS are the data sources with a @provides at the field or above it, by field ref
	providesScopeDS map[int][]int

	dsCollectors        []*dsNodesCollector
	dsCollectorsReports []*operationreport.Report
}

type DSKeyInfo struct {
	DSHash   DSHash
	TypeName string
	Path     string
	Keys     []KeyInfo
}

type KeyIndex struct {
	Path     string
	TypeName string
}

type SeenKeyPath struct {
	Path     string
	TypeName string
	DSHash   DSHash
}

// Collect collects nodes for the fields which are new in the response tree:
// all fields on the first run, later the fields added to the operation
// or made new again by NodeSuggestions.RemoveSeenFields.
func (c *nodesCollector) Collect() (keys []DSKeyInfo) {
	newFieldRefs := c.buildTree()
	if c.report.HasErrors() {
		return nil
	}

	// reset keys to not preserve already collected keys from previous iterations
	c.keys = c.keys[:0]

	c.collectNodes(newFieldRefs)
	if c.report.HasErrors() {
		return nil
	}

	return c.keys
}

func (c *nodesCollector) init() {
	c.keys = make([]DSKeyInfo, 0, len(c.dataSources))
	c.dsCollectors = make([]*dsNodesCollector, 0, len(c.dataSources))
	c.dsCollectorsReports = make([]*operationreport.Report, 0, len(c.dataSources))

	// prepare collectors for each data source
	for _, dataSource := range c.dataSources {
		collector := &dsNodesCollector{
			operation:               c.operation,
			definition:              c.definition,
			nodes:                   c.nodes,
			info:                    c.fieldInfo,
			keys:                    make([]DSKeyInfo, 0, 2),
			localSeenKeys:           make(map[SeenKeyPath]struct{}),
			availableProvidedFields: make(map[int]providesSelection),
			usedProvidedFields:      make(map[int]providesSelection),
			globalSeenKeys:          c.seenKeys,
			dataSource:              dataSource,
			notExternalKeyPaths:     make(map[string]struct{}),
		}
		metadata := dataSource.FederationConfiguration()
		if len(metadata.ContextArguments) > 0 {
			collector.contextMetadata = &metadata
		}
		c.dsCollectors = append(c.dsCollectors, collector)
		c.dsCollectorsReports = append(c.dsCollectorsReports, operationreport.NewReport())
	}

	c.dsIndexesByTypeName = make(map[string][]int)
	c.providesDS = make(map[string][]int)
	c.providesScopeDS = make(map[int][]int)
	for i, dataSource := range c.dataSources {
		for _, provide := range dataSource.FederationConfiguration().Provides {
			key := provide.TypeName + "." + provide.FieldName
			c.providesDS[key] = append(c.providesDS[key], i)
		}
	}
}

func (c *nodesCollector) collectNodes(newFieldRefs map[int]struct{}) {
	// collect fields to visit, in breadth-first order
	fieldsToVisit := make([]int, 0, len(newFieldRefs))
	for fieldRef := range traverseBFS(c.nodes.responseTree) {
		if fieldRef == treeRootID {
			continue
		}
		if _, ok := newFieldRefs[fieldRef]; !ok {
			continue
		}

		fieldsToVisit = append(fieldsToVisit, fieldRef)
	}

	c.assignFields(fieldsToVisit)

	wg := &sync.WaitGroup{}
	for i, collector := range c.dsCollectors {
		// cleanup data from previous runs, but preserve indexes.
		// A collector without fields is reset too,
		// so it does not apply the suggestions of an earlier run again.
		collector.reset()

		if len(collector.assignedFieldRefs) == 0 {
			continue
		}

		wg.Go(func() {
			defer func() {
				// recover from panic and add it to the report
				if r := recover(); r != nil {
					c.dsCollectorsReports[i].AddInternalError(fmt.Errorf("panic: %v stack: %s", r, debug.Stack()))
				}
			}()
			for _, fieldRef := range collector.assignedFieldRefs {
				if err := collector.collectField(fieldRef); err != nil {
					c.dsCollectorsReports[i].AddInternalError(fmt.Errorf("data source %s: %w", collector.dataSource.Name(), err))
					// stop processing on error
					return
				}
			}
		})
	}
	wg.Wait()

	for _, report := range c.dsCollectorsReports {
		if report.HasErrors() {
			for i := range report.ExternalErrors {
				c.report.AddExternalError(report.ExternalErrors[i])
			}
			for i := range report.InternalErrors {
				c.report.AddInternalError(report.InternalErrors[i])
			}
			return
		}
	}

	// NOTE: collect nodes should never modify the tree, nodes or seen keys during the walk
	// it will be a data race
	for _, collector := range c.dsCollectors {
		collector.applySuggestions()
		c.keys = append(c.keys, collector.keys...)

		for key := range collector.localSeenKeys {
			c.seenKeys[key] = struct{}{}
		}
	}
}

// assignFields splits the fields of the current run between the collectors.
// A collector gets the fields of the types its data source knows,
// and the fields at or under a field with a @provides of its data source.
// Fields keep their order, as the collector builds state during the walk.
func (c *nodesCollector) assignFields(fieldRefs []int) {
	for _, collector := range c.dsCollectors {
		collector.assignedFieldRefs = collector.assignedFieldRefs[:0]
	}

	for _, fieldRef := range fieldRefs {
		c.assignProvidesScope(fieldRef)
		if _, ok := c.unfetchableFieldRefs[fieldRef]; ok {
			// the field is intentionally left unplanned - it is resolved to null,
			// so we should not collect suggestions for it
			continue
		}
		info, ok := c.fieldInfo[fieldRef]
		if !ok {
			continue
		}
		c.assignField(fieldRef, info.typeName)
		// keys for the possible types of an abstract type
		for _, possibleTypeName := range info.possibleTypeNames {
			c.assignField(fieldRef, possibleTypeName)
		}
	}
}

// assignProvidesScope gives the field to each data source with a @provides on it or on a field above it.
// The parent scope is always known: a parent comes before its children,
// or was assigned in an earlier Collect call, and scopes are kept.
func (c *nodesCollector) assignProvidesScope(fieldRef int) {
	info, ok := c.fieldInfo[fieldRef]
	if !ok {
		return
	}
	scope := c.providesScopeDS[info.parentFieldRef]
	if dsIndexes := c.providesDS[info.typeName+"."+info.fieldName]; len(dsIndexes) > 0 {
		scope = slices.Clone(scope)
		for _, i := range dsIndexes {
			if !slices.Contains(scope, i) {
				scope = append(scope, i)
			}
		}
	}
	if len(scope) == 0 {
		return
	}
	c.providesScopeDS[fieldRef] = scope
	if _, ok := c.unfetchableFieldRefs[fieldRef]; ok {
		return
	}
	for _, i := range scope {
		c.dsCollectors[i].assignedFieldRefs = append(c.dsCollectors[i].assignedFieldRefs, fieldRef)
	}
}

// assignField gives the field to each data source that knows the type, once.
// Fields come in order, so a field given before is the collector's last one.
// The data sources of each type are cached for the operation.
func (c *nodesCollector) assignField(fieldRef int, typeName string) {
	dsIndexes, ok := c.dsIndexesByTypeName[typeName]
	if !ok {
		for i, collector := range c.dsCollectors {
			if collector.knowsType(typeName) {
				dsIndexes = append(dsIndexes, i)
			}
		}
		c.dsIndexesByTypeName[typeName] = dsIndexes
	}

	for _, i := range dsIndexes {
		collector := c.dsCollectors[i]
		fieldRefs := collector.assignedFieldRefs
		if len(fieldRefs) > 0 && fieldRefs[len(fieldRefs)-1] == fieldRef {
			continue
		}
		collector.assignedFieldRefs = append(fieldRefs, fieldRef)
	}
}

// buildTree adds the fields not seen yet to the response tree and returns them
func (c *nodesCollector) buildTree() (newFieldRefs map[int]struct{}) {
	walker := astvisitor.NewWalkerWithID(32, "TreeBuilderVisitor")
	visitor := &treeBuilderVisitor{
		operation:    c.operation,
		definition:   c.definition,
		walker:       &walker,
		nodes:        c.nodes,
		fieldInfo:    c.fieldInfo,
		newFieldRefs: make(map[int]struct{}),
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.Walk(c.operation, c.definition, c.report)

	return visitor.newFieldRefs
}

type treeBuilderVisitor struct {
	walker          *astvisitor.Walker
	operation       *ast.Document
	definition      *ast.Document
	nodes           *NodeSuggestions
	parentFieldRefs []int // ancestor field refs, ast.InvalidRef sentinel at the root
	fieldInfo       map[int]fieldInfo
	newFieldRefs    map[int]struct{} // fields seen for the first time in this walk
}

func (f *treeBuilderVisitor) EnterDocument(_, _ *ast.Document) {
	f.parentFieldRefs = []int{ast.InvalidRef}
}

func (f *treeBuilderVisitor) EnterField(fieldRef int) {
	if f.nodes.IsFieldSeen(fieldRef) {
		f.parentFieldRefs = append(f.parentFieldRefs, fieldRef)
		return
	}
	f.nodes.AddSeenField(fieldRef)
	f.newFieldRefs[fieldRef] = struct{}{}

	// the parent is the nearest ancestor field, the tree root for a root field
	parentID := f.parentFieldRefs[len(f.parentFieldRefs)-1]
	if parentID == ast.InvalidRef {
		parentID = treeRootID
	}
	f.nodes.responseTree.add(fieldRef, parentID)

	// must push before collectFieldInfo: it reads the parent field ref at len-2
	f.parentFieldRefs = append(f.parentFieldRefs, fieldRef)

	f.collectFieldInfo(fieldRef)
}

func (f *treeBuilderVisitor) LeaveField(ref int) {
	if f.parentFieldRefs[len(f.parentFieldRefs)-1] == ref {
		f.parentFieldRefs = f.parentFieldRefs[:len(f.parentFieldRefs)-1]
	}
}

type dsNodesCollector struct {
	rejectedContextOwners map[int]struct{}
	operation             *ast.Document
	definition            *ast.Document
	dataSource            DataSource

	contextMetadata        *FederationMetaData
	rootContextEligibility map[rootContextKey]bool

	// assignedFieldRefs are the fields this collector visits in the current run, in the visit order.
	assignedFieldRefs []int

	// local suggestions stores suggestions for the current run of collecting fields
	// they are reset after each run, because each time we collect suggestion only for new field refs
	partialContextScopes []int
	localSuggestions     []*NodeSuggestion

	// availableProvidedFields stores, per field ref carrying a @provides directive,
	// the selection tree of fields the directive makes available under that field -
	// regardless of what the query selects. Survives reset because unique field refs
	// are collected only once.
	availableProvidedFields map[int]providesSelection

	// usedProvidedFields stores provided fields used in the query - fields of the
	// operation matched against availableProvidedFields of an enclosing @provides
	// field. Presence of a field ref means the field is provided; the value is the
	// position inside the available selection tree applying to the field's children
	// (nil for leaf fields), so a child resolves with a single lookup. Fields are
	// visited parents-first, so an entry is always resolved before the children
	// need it. Survives reset like availableProvidedFields.
	usedProvidedFields map[int]providesSelection

	// global node suggestion, we append to them after each run
	nodes *NodeSuggestions

	// notExternalKeyPaths - stores paths of fields used in keys, which marked external
	// but semantically are not true external
	notExternalKeyPaths map[string]struct{}

	// reference to a global cache of field info shared between all collector instances
	info map[int]fieldInfo

	// information about keys available for a given path collected during the current run
	keys []DSKeyInfo

	// globalSeenKeys - stores key information which is shared globally between collectors, needed to avoid collecting keys on the same path
	// between different runs, as evaluating keys are very expensive
	// as it is shared between goroutines of collector we read it during the run, and write after the run finished
	globalSeenKeys map[SeenKeyPath]struct{}
	// seen keys local to the current run
	localSeenKeys map[SeenKeyPath]struct{}
}

// reset - cleanups only data which should not be persisted between runs
func (f *dsNodesCollector) reset() {
	f.localSuggestions = f.localSuggestions[:0]
	f.keys = f.keys[:0]
}

// knowsType reports whether collectField could have an effect on a field of the type:
// the data source has a root or child node of the type, keys for it,
// or a union of that name in its upstream schema.
// The fields under a @provides are assigned by nodesCollector.assignProvidesScope.
func (f *dsNodesCollector) knowsType(typeName string) bool {
	if f.dataSource.HasRootNodeWithTypename(typeName) ||
		f.dataSource.HasChildNodeWithTypename(typeName) ||
		f.dataSource.HasEntity(typeName) {
		return true
	}
	schema, ok := f.dataSource.UpstreamSchema()
	if !ok {
		return false
	}
	node, ok := schema.NodeByNameStr(typeName)
	return ok && node.Kind == ast.NodeKindUnionTypeDefinition
}

func (f *dsNodesCollector) hasProvidesConfiguration(typeName, fieldName string) (selectionSet string, ok bool) {
	providesIdx := slices.IndexFunc(f.dataSource.FederationConfiguration().Provides, func(provide FederationFieldConfiguration) bool {
		return provide.TypeName == typeName && provide.FieldName == fieldName
	})
	if providesIdx == -1 {
		return "", false
	}
	return f.dataSource.FederationConfiguration().Provides[providesIdx].SelectionSet, true
}

func (f *dsNodesCollector) isEntityInterface(typeName string) bool {
	cfg := f.dataSource.FederationConfiguration()
	return cfg.HasEntityInterface(typeName)
}

func (f *dsNodesCollector) isInterfaceObject(typeName string) bool {
	cfg := f.dataSource.FederationConfiguration()
	return cfg.HasInterfaceObject(typeName)
}

// has disabled entity resolver
func (f *dsNodesCollector) allKeysHasDisabledEntityResolver(typeName string) bool {
	keys := f.dataSource.FederationConfiguration().Keys

	if len(keys) == 0 {
		return false
	}

	keysForType := keys.FilterByTypeAndResolvability(typeName, false)

	// for the root query nodes there will be no keys
	if len(keysForType) == 0 {
		return false
	}

	return !slices.ContainsFunc(keysForType, func(k FederationFieldConfiguration) bool {
		return !k.DisableEntityResolver
	})
}

func (f *dsNodesCollector) handleProvidesSuggestions(fieldRef int, typeName, fieldName, currentPath string, enclosingTypeDefinition ast.Node) error {
	if !f.operation.FieldHasSelections(fieldRef) {
		return nil
	}

	providesSelectionSet, hasProvides := f.hasProvidesConfiguration(typeName, fieldName)
	if !hasProvides {
		return nil
	}

	if enclosingTypeDefinition.Kind != ast.NodeKindObjectTypeDefinition {
		return nil
	}

	fieldDefRef, ok := f.definition.ObjectTypeDefinitionFieldWithName(enclosingTypeDefinition.Ref, f.operation.FieldNameBytes(fieldRef))
	if !ok {
		return nil
	}
	fieldTypeName := f.definition.FieldDefinitionTypeNameString(fieldDefRef)

	_, ok = f.operation.FieldSelectionSet(fieldRef)
	if !ok {
		return fmt.Errorf("failed to get selection set ref for %s.%s at path %s. Field with provides directive should have a selections", typeName, fieldName, currentPath)
	}

	input := &providesInput{
		parentTypeName:       fieldTypeName,
		providesSelectionSet: providesSelectionSet,
		definition:           f.definition,
	}
	selection, report := providesSuggestions(input)
	if report.HasErrors() {
		return fmt.Errorf("failed to get provides suggestions for %s.%s at path %s: %w", typeName, fieldName, currentPath, report)
	}

	if len(selection) > 0 {
		f.availableProvidedFields[fieldRef] = selection
	}

	return nil
}

// childSelection returns the provided selection applying to the children of the given
// field: the field's own @provides selection when it is an anchor, otherwise the
// nested selection the field got from an enclosing @provides directive.
func (f *dsNodesCollector) childSelection(fieldRef int) providesSelection {
	if fieldRef == ast.InvalidRef {
		return nil
	}
	if selection, ok := f.availableProvidedFields[fieldRef]; ok {
		return selection
	}
	return f.usedProvidedFields[fieldRef]
}

// resolveProvided reports whether the field is provided by an enclosing @provides
// directive of this data source and memoizes the provided selection for the field's
// children. A field is provided when its parent's provided selection has a branch
// for the field name whose allowed types include the field's enclosing type - so a
// provides pinned to one concrete type at some level can not match a sibling
// concrete type at the same position.
func (f *dsNodesCollector) resolveProvided(fieldRef int, info fieldInfo) bool {
	if _, ok := f.usedProvidedFields[fieldRef]; ok {
		return true
	}

	parentSelection := f.childSelection(info.parentFieldRef)
	if parentSelection == nil {
		return false
	}

	childSelection, provided := parentSelection.providedTypeSelection(info.fieldName, info.typeName)
	if !provided {
		return false
	}

	f.usedProvidedFields[fieldRef] = childSelection
	return true
}

func (f *dsNodesCollector) shouldAddUnionTypenameFieldSuggestion(info fieldInfo) bool {
	if !info.isTypeName {
		return false
	}

	if info.enclosingTypeDefinition.Kind != ast.NodeKindUnionTypeDefinition {
		return false
	}

	// check if datasource has an upstream schema
	// currently only graphql datasource has an upstream schema
	dsDef, ok := f.dataSource.UpstreamSchema()
	if !ok {
		return false
	}

	// check if datasource has a union type with such name
	node, ok := dsDef.NodeByNameStr(info.typeName)
	if !ok {
		return false
	}

	return node.Kind == ast.NodeKindUnionTypeDefinition
}

func (f *dsNodesCollector) isNotExternalKeyField(currentPath string) bool {
	_, ok := f.notExternalKeyPaths[currentPath]
	return ok
}

type rootContextKey struct {
	trigger   int
	selection string
}

func (f *dsNodesCollector) rootFieldProvidesContext(trigger int, root, selection string) bool {
	key := rootContextKey{trigger, selection}
	if available, checked := f.rootContextEligibility[key]; checked {
		return available
	}
	doc, report := astparser.ParseGraphqlDocumentString(selection)
	available := !report.HasErrors() && len(doc.OperationDefinitions) == 1 &&
		rootFieldCanProject(f.definition, root, f.operation, trigger, &doc, contextFieldForType(f.definition, &doc, doc.OperationDefinitions[0].SelectionSet, root))
	if f.rootContextEligibility == nil {
		f.rootContextEligibility = make(map[rootContextKey]bool)
	}
	f.rootContextEligibility[key] = available
	return available
}

// A contextual resolver is eligible only if every required context is available on
// a strict ancestor. Narrow a partially covered abstract scope to its concrete types
// so each branch can choose an eligible resolver independently.
func (f *dsNodesCollector) hasRequiredContexts(info fieldInfo) bool {
	for _, argument := range f.contextMetadata.ContextArguments {
		if argument.TypeName != info.typeName || argument.FieldName != info.fieldName {
			continue
		}

		covered := false
		partialScope := ast.InvalidRef
		for ref := info.parentFieldRef; ref != ast.InvalidRef; {
			ancestor, ok := f.info[ref]
			if !ok {
				break
			}
			matches, all := f.contextMetadata.providerTypes(f.definition, argument.ContextName, ancestor.typeName)
			if all {
				if ancestor.parentFieldRef == ast.InvalidRef && rootContextUsesActiveField(f.definition, ancestor.typeName) &&
					!f.rootFieldProvidesContext(ref, ancestor.typeName, argument.SelectionSet) {
					break
				}
				covered = true
				break
			}
			if len(matches) != 0 && partialScope == ast.InvalidRef {
				partialScope = ancestor.parentFieldRef
			}
			ref = ancestor.parentFieldRef
		}
		if !covered {
			if partialScope != ast.InvalidRef {
				f.partialContextScopes = append(f.partialContextScopes, partialScope)
			}
			return false
		}
	}
	return true
}

func (f *dsNodesCollector) collectField(fieldRef int) error {
	if _, rejected := f.rejectedContextOwners[fieldRef]; rejected {
		return nil
	}
	info, ok := f.info[fieldRef]
	if !ok {
		return nil
	}

	if err := f.handleProvidesSuggestions(fieldRef, info.typeName, info.fieldName, info.currentPath, info.enclosingTypeDefinition); err != nil {
		return err
	}

	// provided selection applying to the field and its siblings - the fields of the
	// entity at the parent path - used to treat provided key fields as not external
	parentProvidedSelection := f.childSelection(info.parentFieldRef)

	// For pubsub entities could also be a child node, so checking for only root nodes is not enough, so we check for entity keys existence
	// when we have no keys, it is still expensive to create an index entry for a seen key path,
	// so we skip check as a whole when there is no entity with such a name
	if f.dataSource.HasEntity(info.typeName) {
		// should be done after handling provides
		if err := f.collectKeysForPath(info.typeName, info.parentPath, parentProvidedSelection); err != nil {
			return err
		}
	}
	// We need to collect keys for all possible types of an abstract type too
	// because during initial planning we do not know yet if the abstract selection will be rewritten,
	// This means that in the unmodified query we could try to match abstract to concrete type, which won't match
	// So we have to add possible choices for each of the concrete types, to make this match possible
	for _, possibleTypeName := range info.possibleTypeNames {
		// for each of the possible typenames we also check if we have an entity
		if f.dataSource.HasEntity(possibleTypeName) {
			if err := f.collectKeysForPath(possibleTypeName, info.parentPath, parentProvidedSelection); err != nil {
				return err
			}
		}
	}

	isProvided := f.resolveProvided(fieldRef, info)

	if info.isTypeName && f.isInterfaceObject(info.typeName) {
		// we should not add a typename on the interface object
		// to not select it during node suggestions calculation
		// we will add a typename field to the interface object query in the datasource planner

		// at the same time we should allow to select a typename on the entity interface
		return nil
	}

	hasRootNodeWithTypename := f.dataSource.HasRootNodeWithTypename(info.typeName)
	// hasRootNode is true when:
	// - ds config has a root node for the field
	// - we have a root node with typename and the field is a __typename field
	// we no longer add a typename field for the root query nodes, as it is now handled by the planning visitor
	hasRootNode := f.dataSource.HasRootNode(info.typeName, info.fieldName) || (info.isTypeName && hasRootNodeWithTypename && !IsMutationOrQueryRootType(info.typeName))

	// hasChildNode is true when:
	// - ds config has a child node for the field
	// - we have a child node with typename and the field is a __typename field
	// - the field is __typename field on a union, and we have a suggestion for the parent field
	hasChildNode := f.dataSource.HasChildNode(info.typeName, info.fieldName) || (info.isTypeName && f.dataSource.HasChildNodeWithTypename(info.typeName))

	// external root node is a node having external directive, to be resolvable it needs to be provided or be part of a key
	// So the node will not be external if it is mentioned in both fields and external fields
	isExternalRootNode := f.dataSource.HasExternalRootNode(info.typeName, info.fieldName)
	isExternalChildNode := f.dataSource.HasExternalChildNode(info.typeName, info.fieldName)
	isExternal := isExternalRootNode || isExternalChildNode

	hasChildNode = hasChildNode || f.shouldAddUnionTypenameFieldSuggestion(info)
	isLeaf := !f.operation.FieldHasSelections(fieldRef)

	if isExternal && f.isNotExternalKeyField(info.currentPath) {
		// external fields which are part of the key should not be marked as external
		isExternal = false
	}

	if hasRootNode || hasChildNode || isExternal || isProvided {
		if f.contextMetadata != nil && !isProvided && !f.hasRequiredContexts(info) {
			return nil
		}
		disabledEntityResolver := hasRootNode && f.allKeysHasDisabledEntityResolver(info.typeName)

		node := NodeSuggestion{
			FieldRef:                  fieldRef,
			TypeName:                  info.typeName,
			possibleTypeNames:         info.possibleTypeNames,
			FieldName:                 info.fieldName,
			DataSourceHash:            f.dataSource.Hash(),
			DataSourceID:              f.dataSource.Id(),
			DataSourceName:            f.dataSource.Name(),
			Path:                      info.currentPath,
			ParentPath:                info.parentPath,
			IsRootNode:                hasRootNode,
			onFragment:                info.onFragment,
			parentPathWithoutFragment: info.parentPathWithoutFragment,
			DisabledEntityResolver:    disabledEntityResolver,
			IsEntityInterfaceTypeName: info.isTypeName && f.isEntityInterface(info.typeName),
			IsExternal:                isExternal,
			IsProvided:                isProvided,
			IsLeaf:                    isLeaf,
			isTypeName:                info.isTypeName,
			deferInfo:                 info.deferInfo,
			hasUnionReturnType:        info.hasUnionReturnType,
		}

		f.localSuggestions = append(f.localSuggestions, &node)
	}

	return nil
}

func (f *dsNodesCollector) applySuggestions() {
	for _, fieldRef := range f.partialContextScopes {
		if f.nodes.partialContextScopes == nil {
			f.nodes.partialContextScopes = make(map[int]struct{})
		}
		f.nodes.partialContextScopes[fieldRef] = struct{}{}
	}
	f.partialContextScopes = f.partialContextScopes[:0]

	// copy local suggestions to the global nodes suggestions
	for _, suggestion := range f.localSuggestions {
		f.nodes.addSuggestion(suggestion)
		field, _ := f.nodes.responseTree.find(suggestion.FieldRef)
		newID := len(f.nodes.items) - 1
		field.suggestionIndexes = append(field.suggestionIndexes, newID)
	}

	// apply provided selections: used provided fields first, available after,
	// as the nearest field with a @provides directive defines what is provided
	// for its children
	for fieldRef, selection := range f.usedProvidedFields {
		f.nodes.addProvidedSelection(f.dataSource.Hash(), fieldRef, selection)
	}
	for fieldRef, selection := range f.availableProvidedFields {
		f.nodes.addProvidedSelection(f.dataSource.Hash(), fieldRef, selection)
	}
}

const (
	queryTypeName    = "Query"
	mutationTypeName = "Mutation"
)

func IsMutationOrQueryRootType(typeName string) bool {
	return queryTypeName == typeName || mutationTypeName == typeName
}

type fieldInfo struct {
	typeName, fieldName, parentPath, currentPath string
	onFragment, isTypeName                       bool
	parentPathWithoutFragment                    string
	possibleTypeNames                            []string
	enclosingTypeDefinition                      ast.Node
	// parentFieldRef is the nearest ancestor field ref (skipping inline fragments),
	// or ast.InvalidRef for a root field. Used to walk the typed ancestry for provides chains.
	parentFieldRef     int
	deferInfo          *DeferInfo
	hasUnionReturnType bool
}

func (f *treeBuilderVisitor) collectFieldInfo(fieldRef int) {
	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)

	var possibleTypes []string
	switch f.walker.EnclosingTypeDefinition.Kind {
	case ast.NodeKindUnionTypeDefinition:
		possibleTypes, _ = f.definition.UnionTypeDefinitionMemberTypeNames(f.walker.EnclosingTypeDefinition.Ref)
	case ast.NodeKindInterfaceTypeDefinition:
		possibleTypes, _ = f.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(f.walker.EnclosingTypeDefinition.Ref)
	default:
	}

	fieldName := f.operation.FieldNameUnsafeString(fieldRef)
	fieldAliasOrName := f.operation.FieldAliasOrNameString(fieldRef)
	isTypeName := fieldName == typeNameField
	parentPath := f.walker.Path.DotDelimitedString()
	onFragment := f.walker.Path.EndsWithFragment()
	parentPathWithoutFragment := f.walker.Path.WithoutInlineFragmentNames().DotDelimitedString()

	currentPath := fmt.Sprintf("%s.%s", parentPath, fieldAliasOrName)

	// nearest ancestor field ref (the tree has only fields, so the tree
	// parent is the nearest ancestor field, skipping inline fragments), ast.InvalidRef
	// at the root; len-1 is the current field pushed by EnterField
	parentFieldRef := f.parentFieldRefs[len(f.parentFieldRefs)-2]

	f.fieldInfo[fieldRef] = fieldInfo{
		typeName:                  typeName,
		possibleTypeNames:         possibleTypes,
		fieldName:                 fieldName,
		parentPath:                parentPath,
		currentPath:               currentPath,
		onFragment:                onFragment,
		parentPathWithoutFragment: parentPathWithoutFragment,
		isTypeName:                isTypeName,
		enclosingTypeDefinition:   f.walker.EnclosingTypeDefinition,
		parentFieldRef:            parentFieldRef,
		deferInfo:                 f.deferInfo(fieldRef),
		hasUnionReturnType:        f.fieldHasUnionReturnType(fieldRef),
	}
}

func (f *treeBuilderVisitor) fieldHasUnionReturnType(fieldRef int) bool {
	definition, ok := f.definition.NodeFieldDefinitionByName(f.walker.EnclosingTypeDefinition, f.operation.FieldNameBytes(fieldRef))
	if !ok {
		return false
	}

	fieldDefinitionTypeNode := f.definition.FieldDefinitionTypeNode(definition)

	return fieldDefinitionTypeNode.Kind == ast.NodeKindUnionTypeDefinition
}
func (f *treeBuilderVisitor) deferInfo(fieldRef int) *DeferInfo {
	deferDirectiveRef, exists := f.operation.Fields[fieldRef].Directives.HasDirectiveByNameBytes(f.operation, literal.DEFER_INTERNAL)
	if !exists {
		return nil
	}

	info := &DeferInfo{}

	idValue, idExists := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("id"))
	if idExists && idValue.Kind == ast.ValueKindInteger {
		info.ID = int(f.operation.IntValueAsInt(idValue.Ref))
	}

	parentIdValue, exists := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("parentDeferId"))
	if exists && parentIdValue.Kind == ast.ValueKindInteger {
		info.ParentID = int(f.operation.IntValueAsInt(parentIdValue.Ref))
	}

	labelValue, exists := f.operation.DirectiveArgumentValueByName(deferDirectiveRef, []byte("label"))
	if exists {
		info.Label = f.operation.StringValueContentString(labelValue.Ref)
	}

	return info
}
