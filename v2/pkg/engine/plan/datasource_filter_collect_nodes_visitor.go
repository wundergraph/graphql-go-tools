package plan

import (
	"fmt"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
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

	maxConcurrency       uint
	seenKeys             map[SeenKeyPath]struct{}
	fieldInfo            map[int]fieldInfo
	newFieldRefs         map[int]struct{}
	unfetchableFieldRefs map[int]struct{}

	dsVisitors        []*collectNodesDSVisitor
	dsVisitorsReports []*operationreport.Report
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

func (c *nodesCollector) CollectNodes() (keys []DSKeyInfo) {
	c.buildTree()
	if c.report.HasErrors() {
		return nil
	}

	// reset keys to not preserve already collected keys from previous iterations
	c.keys = c.keys[:0]

	c.collectNodes()
	if c.report.HasErrors() {
		return nil
	}

	return c.keys
}

type nodeVisitTask struct {
	fieldRef     int
	treeNodeData []int
	treeNodeId   uint
}

func (c *nodesCollector) initVisitors() {
	c.keys = make([]DSKeyInfo, 0, len(c.dataSources))
	c.dsVisitors = make([]*collectNodesDSVisitor, 0, len(c.dataSources))
	c.dsVisitorsReports = make([]*operationreport.Report, 0, len(c.dataSources))

	// prepare visitors for each data source
	for _, dataSource := range c.dataSources {
		visitor := &collectNodesDSVisitor{
			operation:               c.operation,
			definition:              c.definition,
			nodes:                   c.nodes,
			info:                    c.fieldInfo,
			keys:                    make([]DSKeyInfo, 0, 2),
			localSeenKeys:           make(map[SeenKeyPath]struct{}),
			localSuggestionLookup:   make(map[int]struct{}),
			availableProvidedFields: make(map[int]providesSelection),
			usedProvidedFields:      make(map[int]providesSelection),
			globalSeenKeys:          c.seenKeys,
			dataSource:              dataSource,
			notExternalKeyPaths:     make(map[string]struct{}),
			unfetchableFieldRefs:    c.unfetchableFieldRefs,
		}
		c.dsVisitors = append(c.dsVisitors, visitor)
		c.dsVisitorsReports = append(c.dsVisitorsReports, operationreport.NewReport())
	}
}

func (c *nodesCollector) collectNodes() {
	// collect fields to visit
	nodesToVisit := make([]nodeVisitTask, 0, len(c.operation.Fields))
	for treeNodeID, treeNode := range TraverseBFS(c.nodes.responseTree) {
		if treeNodeID == treeRootID {
			continue
		}
		fieldRef := TreeNodeFieldRef(treeNodeID)

		if len(c.newFieldRefs) > 0 {
			if _, ok := c.newFieldRefs[fieldRef]; !ok {
				// skip field refs which were not added during the current iteration
				continue
			}
		}

		task := nodeVisitTask{
			fieldRef:     fieldRef,
			treeNodeData: treeNode.GetData(),
			treeNodeId:   treeNodeID,
		}

		nodesToVisit = append(nodesToVisit, task)
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(c.dsVisitors))

	// Create a semaphore if concurrency is limited
	var sem chan struct{}
	if c.maxConcurrency > 0 {
		sem = make(chan struct{}, c.maxConcurrency)
	}

	for i, visitor := range c.dsVisitors {
		if sem != nil {
			sem <- struct{}{}
		}
		go func(visitor *collectNodesDSVisitor, report *operationreport.Report, nodesToVisit []nodeVisitTask) {
			defer func() {
				wg.Done()
			}()
			defer func() {
				if sem != nil {
					<-sem
				}
			}()
			defer func() {
				// recover from panic and add it to the report
				if r := recover(); r != nil {
					report.AddInternalError(fmt.Errorf("panic: %v stack: %s", r, debug.Stack()))
				}
			}()

			// cleanup data from previous runs, but preserve indexes
			visitor.reset()

			for _, node := range nodesToVisit {
				if err := visitor.EnterField(node.fieldRef, node.treeNodeData, node.treeNodeId); err != nil {
					report.AddInternalError(fmt.Errorf("data source %s: %v", visitor.dataSource.Name(), err))
					// stop processing on error
					return
				}
			}
		}(visitor, c.dsVisitorsReports[i], nodesToVisit)
	}

	wg.Wait()

	for _, report := range c.dsVisitorsReports {
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
	for _, visitor := range c.dsVisitors {
		visitor.applySuggestions()

		c.keys = append(c.keys, visitor.keys...)

		for key := range visitor.localSeenKeys {
			c.seenKeys[key] = struct{}{}
		}
	}
}

func (c *nodesCollector) buildTree() {
	walker := astvisitor.NewWalkerWithID(32, "TreeBuilderVisitor")
	visitor := &treeBuilderVisitor{
		operation:  c.operation,
		definition: c.definition,
		walker:     &walker,
		nodes:      c.nodes,
		fieldInfo:  c.fieldInfo,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.Walk(c.operation, c.definition, c.report)
}

type treeBuilderVisitor struct {
	walker          *astvisitor.Walker
	operation       *ast.Document
	definition      *ast.Document
	nodes           *NodeSuggestions
	parentFieldRefs []int // ancestor field refs, ast.InvalidRef sentinel at the root
	fieldInfo       map[int]fieldInfo
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

	// we intentionally ignore the return values added, exists
	// because we do not recheck the same field refs, so all added nodes should be new and unique
	_, _ = f.nodes.responseTree.Add(TreeNodeID(fieldRef), f.currentParentNodeID(), nil)

	// must push before collectFieldInfo: it reads the parent field ref at len-2
	f.parentFieldRefs = append(f.parentFieldRefs, fieldRef)

	f.collectFieldInfo(fieldRef)
}

// currentParentNodeID maps the nearest ancestor field ref to its tree node id,
// treeRootID for a root field
func (f *treeBuilderVisitor) currentParentNodeID() uint {
	if parentFieldRef := f.parentFieldRefs[len(f.parentFieldRefs)-1]; parentFieldRef != ast.InvalidRef {
		return TreeNodeID(parentFieldRef)
	}
	return treeRootID
}

func (f *treeBuilderVisitor) LeaveField(ref int) {
	if f.parentFieldRefs[len(f.parentFieldRefs)-1] == ref {
		f.parentFieldRefs = f.parentFieldRefs[:len(f.parentFieldRefs)-1]
	}
}

type collectNodesDSVisitor struct {
	operation  *ast.Document
	definition *ast.Document
	dataSource DataSource

	// local suggestions stores suggestions for the current run of collecting fields
	// they are reset after each run, because each time we collect suggestion only for new field refs
	localSuggestions      []*NodeSuggestion
	localSuggestionLookup map[int]struct{}

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

	// unfetchableFieldRefs - field refs which must not be planned on any datasource,
	// we should not collect suggestions for them
	unfetchableFieldRefs map[int]struct{}
}

// reset - cleanups only data which should not be persisted between runs
func (f *collectNodesDSVisitor) reset() {
	f.localSuggestions = f.localSuggestions[:0]
	f.keys = f.keys[:0]
}

func (f *collectNodesDSVisitor) hasSuggestionForFieldOnCurrentDataSource(itemIds []int, ref int) (itemID int, ok bool) {
	idx := slices.IndexFunc(itemIds, func(i int) bool {
		suggestion := f.nodes.items[i]
		return suggestion.FieldRef == ref && suggestion.DataSourceHash == f.dataSource.Hash()
	})

	if idx != -1 {
		return itemIds[idx], true
	}

	return -1, false
}

func (f *collectNodesDSVisitor) hasProvidesConfiguration(typeName, fieldName string) (selectionSet string, ok bool) {
	providesIdx := slices.IndexFunc(f.dataSource.FederationConfiguration().Provides, func(provide FederationFieldConfiguration) bool {
		return provide.TypeName == typeName && provide.FieldName == fieldName
	})
	if providesIdx == -1 {
		return "", false
	}
	return f.dataSource.FederationConfiguration().Provides[providesIdx].SelectionSet, true
}

func (f *collectNodesDSVisitor) isEntityInterface(typeName string) bool {
	cfg := f.dataSource.FederationConfiguration()
	return cfg.HasEntityInterface(typeName)
}

func (f *collectNodesDSVisitor) isInterfaceObject(typeName string) bool {
	cfg := f.dataSource.FederationConfiguration()
	return cfg.HasInterfaceObject(typeName)
}

// has disabled entity resolver
func (f *collectNodesDSVisitor) allKeysHasDisabledEntityResolver(typeName string) bool {
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

func (f *collectNodesDSVisitor) handleProvidesSuggestions(fieldRef int, typeName, fieldName, currentPath string, enclosingTypeDefinition ast.Node) error {
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
		return fmt.Errorf("failed to get provides suggestions for %s.%s at path %s: %v", typeName, fieldName, currentPath, report)
	}

	if len(selection) > 0 {
		f.availableProvidedFields[fieldRef] = selection
	}

	return nil
}

// childSelection returns the provided selection applying to the children of the given
// field: the field's own @provides selection when it is an anchor, otherwise the
// nested selection the field got from an enclosing @provides directive.
func (f *collectNodesDSVisitor) childSelection(fieldRef int) providesSelection {
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
func (f *collectNodesDSVisitor) resolveProvided(fieldRef int, info fieldInfo) bool {
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

func (f *collectNodesDSVisitor) shouldAddUnionTypenameFieldSuggestion(info fieldInfo) bool {
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

func (f *collectNodesDSVisitor) isNotExternalKeyField(currentPath string) bool {
	_, ok := f.notExternalKeyPaths[currentPath]
	return ok
}

func (f *collectNodesDSVisitor) EnterField(fieldRef int, itemIds []int, treeNodeId uint) error {
	if _, ok := f.unfetchableFieldRefs[fieldRef]; ok {
		// the field is intentionally left unplanned - it is resolved to null,
		// so we should not collect suggestions for it
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
	if info.possibleTypeNames != nil {
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
	}

	// this is the check for the global suggestions
	if _, ok := f.hasSuggestionForFieldOnCurrentDataSource(itemIds, fieldRef); ok {
		return nil
	}

	// this is the check for the current collect nodes iterations suggestions
	// whether we already added a suggestion for the field with a typename
	if _, hasLocalSuggestion := f.localSuggestionLookup[fieldRef]; hasLocalSuggestion {
		return nil
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
			treeNodeId:                treeNodeId,
			deferInfo:                 info.deferInfo,
			hasUnionReturnType:        info.hasUnionReturnType,
		}

		f.localSuggestions = append(f.localSuggestions, &node)
		f.localSuggestionLookup[fieldRef] = struct{}{}
	}

	return nil
}

func (f *collectNodesDSVisitor) applySuggestions() {
	// copy local suggestions to the global nodes suggestions
	for _, suggestion := range f.localSuggestions {
		f.nodes.addSuggestion(suggestion)
		itemId := len(f.nodes.items) - 1

		treeNode, _ := f.nodes.responseTree.Find(suggestion.treeNodeId)
		itemIds := treeNode.GetData()
		itemIds = append(itemIds, itemId)
		treeNode.SetData(itemIds)
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

func TreeNodeID(fieldRef int) uint {
	// we add 100 to the fieldRef to make sure that the tree node id is never 0
	// cause 0 is a valid field ref
	// but for tree 0 is reserved for the root node
	return uint(100 + fieldRef)
}

func TreeNodeFieldRef(nodeID uint) int {
	return int(nodeID - 100)
}

const (
	queryTypeName    = "Query"
	mutationTypeName = "Mutation"
)

func IsMutationOrQueryRootType(typeName string) bool {
	return queryTypeName == typeName || mutationTypeName == typeName
}

type fieldInfo struct {
	typeName, fieldName, fieldAliasOrName, parentPath, currentPath string
	onFragment, isTypeName                                         bool
	parentPathWithoutFragment                                      string
	possibleTypeNames                                              []string
	currentPathWithoutFragments                                    string
	enclosingTypeDefinition                                        ast.Node
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
	currentPathWithoutFragments := fmt.Sprintf("%s.%s", parentPathWithoutFragment, fieldAliasOrName)

	// nearest ancestor field ref (tree nodes are created only for fields, so the tree
	// parent is the nearest ancestor field, skipping inline fragments), ast.InvalidRef
	// at the root; len-1 is the current field pushed by EnterField
	parentFieldRef := f.parentFieldRefs[len(f.parentFieldRefs)-2]

	f.fieldInfo[fieldRef] = fieldInfo{
		typeName:                    typeName,
		possibleTypeNames:           possibleTypes,
		fieldName:                   fieldName,
		fieldAliasOrName:            fieldAliasOrName,
		parentPath:                  parentPath,
		currentPath:                 currentPath,
		onFragment:                  onFragment,
		parentPathWithoutFragment:   parentPathWithoutFragment,
		currentPathWithoutFragments: currentPathWithoutFragments,
		isTypeName:                  isTypeName,
		enclosingTypeDefinition:     f.walker.EnclosingTypeDefinition,
		parentFieldRef:              parentFieldRef,
		deferInfo:                   f.deferInfo(fieldRef),
		hasUnionReturnType:          f.fieldHasUnionReturnType(fieldRef),
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
