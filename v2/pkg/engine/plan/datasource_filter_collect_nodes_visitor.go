package plan

import (
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"unique"

	"github.com/kingledion/go-tools/tree"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type nodesCollector struct {
	operation   *ast.Document
	definition  *ast.Document
	dataSources []DataSource
	nodes       *NodeSuggestions
	report      *operationreport.Report
	keys        []DSKeyInfo

	maxConcurrency uint
	seenKeys       map[SeenKeyPath]struct{}
	fieldInfo      map[int]fieldInfo
	newFieldRefs   map[int]struct{}
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

	c.collectNodes()
	if c.report.HasErrors() {
		return nil
	}

	return c.keys
}

type workerTask struct {
	fieldRef   int
	treeNode   tree.Node[[]int]
	treeNodeId uint
}

func (c *nodesCollector) collectNodes() {
	visitors := make([]*collectNodesVisitor, len(c.dataSources))
	reports := make([]*operationreport.Report, len(c.dataSources))

	// prepare visitors for each data source
	for i, dataSource := range c.dataSources {
		// walker := astvisitor.WalkerFromPool2("B")
		visitor := &collectNodesVisitor{
			operation:  c.operation,
			definition: c.definition,
			// walker:         walker,
			nodes:                 c.nodes,
			info:                  c.fieldInfo,
			keys:                  make([]DSKeyInfo, 0, 2),
			localSeenKeys:         make(map[SeenKeyPath]struct{}),
			localSuggestionLookup: make(map[int]struct{}),
			globalSeenKeys:        c.seenKeys,
		}
		// walker.RegisterFieldVisitor(visitor)
		visitor.dataSource = dataSource
		visitor.notExternalKeyPaths = make(map[string]struct{})
		visitors[i] = visitor
		report := operationreport.Report{}
		reports[i] = &report
	}

	treeNodes := c.nodes.responseTree.Traverse(tree.TraverseBreadthFirst)

	tasks := make([]workerTask, 0, 100)
	for treeNode := range treeNodes {
		if treeNode.GetID() == treeRootID {
			continue
		}

		treeNodeID := treeNode.GetID()
		fieldRef := TreeNodeFieldRef(treeNodeID)

		if len(c.newFieldRefs) > 0 {
			if _, ok := c.newFieldRefs[fieldRef]; !ok {
				// skip field refs which were not added during the current iteration
				continue
			}
		}

		task := workerTask{
			fieldRef:   fieldRef,
			treeNode:   treeNode,
			treeNodeId: treeNodeID,
		}

		tasks = append(tasks, task)
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(visitors))

	for i, visitor := range visitors {
		go func(visitor *collectNodesVisitor, report *operationreport.Report, tasks []workerTask) {
			defer func() {
				// recover from panic and add it to the report
				if r := recover(); r != nil {
					report.AddInternalError(fmt.Errorf("panic: %v stack: %s", r, debug.Stack()))
				}
			}()
			defer func() {
				wg.Done()
			}()

			for _, task := range tasks {
				if err := visitor.EnterField(task.fieldRef, task.treeNode, task.treeNodeId); err != nil {
					report.AddInternalError(fmt.Errorf("data source %s: %v", visitor.dataSource.Name(), err))
				}
			}
		}(visitor, reports[i], tasks)
	}

	wg.Wait()

	for _, report := range reports {
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

	c.keys = make([]DSKeyInfo, 0, len(c.dataSources))

	// NOTE: collect nodes should never modify the tree, nodes or seen keys during the walk
	// it will be a data race
	for _, visitor := range visitors {
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
	walker        *astvisitor.Walker
	operation     *ast.Document
	definition    *ast.Document
	nodes         *NodeSuggestions
	parentNodeIds []uint
	fieldInfo     map[int]fieldInfo
}

func (f *treeBuilderVisitor) EnterDocument(_, _ *ast.Document) {
	f.parentNodeIds = []uint{treeRootID}
}

func (f *treeBuilderVisitor) EnterField(fieldRef int) {
	if f.nodes.IsFieldSeen(fieldRef) {
		currentNodeId := TreeNodeID(fieldRef)
		f.parentNodeIds = append(f.parentNodeIds, currentNodeId)
		return
	}
	f.nodes.AddSeenField(fieldRef)

	parentNodeId := f.currentParentID()
	currentNodeId := TreeNodeID(fieldRef)

	// we intentionally ignore the return values added, exists
	// because we do not recheck the same field refs, so all added nodes should be new and unique
	_, _ = f.nodes.responseTree.Add(currentNodeId, parentNodeId, nil)
	f.parentNodeIds = append(f.parentNodeIds, currentNodeId)

	f.collectFieldInfo(fieldRef)
}

func (f *treeBuilderVisitor) currentParentID() uint {
	return f.parentNodeIds[len(f.parentNodeIds)-1]
}

func (f *treeBuilderVisitor) LeaveField(ref int) {
	parentNodeId := f.currentParentID()
	currentNodeId := TreeNodeID(ref)

	if parentNodeId == currentNodeId {
		f.parentNodeIds = f.parentNodeIds[:len(f.parentNodeIds)-1]
	}
}

type collectNodesVisitor struct {
	// walker     *astvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
	dataSource DataSource

	localSuggestions      []*NodeSuggestion
	localSuggestionLookup map[int]struct{}

	providesEntries []*NodeSuggestion

	nodes *NodeSuggestions

	notExternalKeyPaths map[string]struct{}
	info                map[int]fieldInfo

	keys []DSKeyInfo

	globalSeenKeys map[SeenKeyPath]struct{}
	localSeenKeys  map[SeenKeyPath]struct{}
}

func (f *collectNodesVisitor) hasSuggestionForFieldOnCurrentDataSource(itemIds []int, ref int) (itemID int, ok bool) {
	idx := slices.IndexFunc(itemIds, func(i int) bool {
		suggestion := f.nodes.items[i]
		return suggestion.FieldRef == ref && suggestion.DataSourceHash == f.dataSource.Hash()
	})

	if idx != -1 {
		return itemIds[idx], true
	}

	return -1, false
}

func (f *collectNodesVisitor) hasLocalSuggestion(ref int) (ok bool) {
	_, ok = f.localSuggestionLookup[ref]
	return ok
}

func (f *collectNodesVisitor) hasProvidesConfiguration(typeName, fieldName string) (selectionSet string, ok bool) {
	providesIdx := slices.IndexFunc(f.dataSource.FederationConfiguration().Provides, func(provide FederationFieldConfiguration) bool {
		return provide.TypeName == typeName && provide.FieldName == fieldName
	})
	if providesIdx == -1 {
		return "", false
	}
	return f.dataSource.FederationConfiguration().Provides[providesIdx].SelectionSet, true
}

func (f *collectNodesVisitor) isEntityInterface(typeName string) bool {
	cfg := f.dataSource.FederationConfiguration()
	return cfg.HasEntityInterface(typeName)
}

func (f *collectNodesVisitor) isInterfaceObject(typeName string) bool {
	cfg := f.dataSource.FederationConfiguration()
	return cfg.HasInterfaceObject(typeName)
}

// has disabled entity resolver
func (f *collectNodesVisitor) allKeysHasDisabledEntityResolver(typeName string) bool {
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

func (f *collectNodesVisitor) handleProvidesSuggestions(fieldRef int, typeName, fieldName, currentPath string, enclosingTypeDefinition ast.Node) error {
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

	providesFieldSet, report := providesFragment(fieldTypeName, providesSelectionSet, f.definition)
	if report.HasErrors() {
		return fmt.Errorf("failed to parse provides fields for %s.%s at path %s: %v", typeName, fieldName, currentPath, report)
	}

	selectionSetRef, ok := f.operation.FieldSelectionSet(fieldRef)
	if !ok {
		return fmt.Errorf("failed to get selection set ref for %s.%s at path %s. Field with provides directive should have a selections", typeName, fieldName, currentPath)
	}

	input := &providesInput{
		providesFieldSet:      providesFieldSet,
		operation:             f.operation,
		definition:            f.definition,
		operationSelectionSet: selectionSetRef,
		report:                report,
		parentPath:            currentPath,
		dataSource:            f.dataSource,
	}
	providesSuggestions := providesSuggestions(input)
	if report.HasErrors() {
		return fmt.Errorf("failed to get provides suggestions for %s.%s at path %s: %v", typeName, fieldName, currentPath, report)
	}

	f.providesEntries = append(f.providesEntries, providesSuggestions...)
	return nil
}

func (f *collectNodesVisitor) shouldAddUnionTypenameFieldSuggestion(info fieldInfo) bool {
	if !info.isTypeName {
		return false
	}

	if info.EnclosingTypeDefinition.Kind != ast.NodeKindUnionTypeDefinition {
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

func (f *collectNodesVisitor) isNotExternalKeyField(currentPath string) bool {
	_, ok := f.notExternalKeyPaths[currentPath]
	return ok
}

func (f *collectNodesVisitor) EnterField(fieldRef int, treeNode tree.Node[[]int], treeNodeId uint) error {
	info, ok := f.info[fieldRef]
	if !ok {
		return nil
	}

	// add fields from provides directive on the current field
	// it needs to be done each time we enter a field
	// because we add provides suggestion only for a fields present in the query - TODO: we do not evaluate only fields present in a query anymore, so probably we could make it once, but currently provides is not cached anywhere
	if err := f.handleProvidesSuggestions(fieldRef, info.typeName, info.fieldName, info.currentPath, info.EnclosingTypeDefinition); err != nil {
		return err
	}

	hasRootNodeWithTypename := f.dataSource.HasRootNodeWithTypenameHandle(info.typeNameHandle)
	if hasRootNodeWithTypename {
		// should be done after handling provides
		f.collectKeysForPath(info.typeName, info.parentPath)
		if info.possibleTypeNames != nil {
			// We need to collect keys for all possible types of abstract type too
			// because during initial planning we do not know yet if the abstract selection will be rewritten,
			// This means that in the unmodified query we could try to match abstract to concrete type, which won't match
			// So we have to add possible choices for each of concrete types, to make this match possible
			for _, possibleTypeName := range info.possibleTypeNames {
				f.collectKeysForPath(possibleTypeName, info.parentPath)
			}
		}
	}

	// currentNodeId := TreeNodeID(fieldRef)
	// treeNode, _ := f.nodes.responseTree.Find(currentNodeId)
	itemIds := treeNode.GetData()

	// TODO: use local seen fields - which survives between iterations

	// this is the check for the global suggestions
	if _, ok := f.hasSuggestionForFieldOnCurrentDataSource(itemIds, fieldRef); ok {
		return nil
	}

	// this is the check for the current collect nodes iterations suggestions
	// whether we already added a suggestion for the field with a typename
	if ok := f.hasLocalSuggestion(fieldRef); ok {
		return nil
	}

	isProvided := slices.ContainsFunc(f.providesEntries, func(suggestion *NodeSuggestion) bool {
		return suggestion.TypeName == info.typeName && suggestion.FieldName == info.fieldName && suggestion.Path == info.currentPath
	})

	if info.isTypeName && f.isInterfaceObject(info.typeName) {
		// we should not add a typename on the interface object
		// to not select it during node suggestions calculation
		// we will add a typename field to the interface object query in the datasource planner

		// at the same time we should allow to select a typename on the entity interface
		return nil
	}

	// hasRootNode is true when:
	// - ds config has a root node for the field
	// - we have a root node with typename and the field is a __typename field
	// we no longer add a typename field for the root query nodes, as it is now handled by the planning visitor
	hasRootNode := f.dataSource.HasRootNodeHandle(info.typeNameHandle, info.fieldNameHandle) || (info.isTypeName && hasRootNodeWithTypename && !IsMutationOrQueryRootType(info.typeName))

	// hasChildNode is true when:
	// - ds config has a child node for the field
	// - we have a child node with typename and the field is a __typename field
	// - the field is __typename field on a union, and we have a suggestion for the parent field
	hasChildNode := f.dataSource.HasChildNodeHandle(info.typeNameHandle, info.fieldNameHandle) || (info.isTypeName && f.dataSource.HasChildNodeWithTypenameHandle(info.typeNameHandle))

	// external root node is a node having external directive, to be resolvable it needs to be provided or be part of a key
	// So the node will not be external if it is mentioned in both fields and external fields
	isExternalRootNode := f.dataSource.HasExternalRootNodeHandle(info.typeNameHandle, info.fieldNameHandle)
	isExternalChildNode := f.dataSource.HasExternalChildNodeHandle(info.typeNameHandle, info.fieldNameHandle)
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
		}

		f.localSuggestions = append(f.localSuggestions, &node)
		f.localSuggestionLookup[fieldRef] = struct{}{}
	}

	return nil
}

func (f *collectNodesVisitor) LeaveField(ref int) {

}

func (f *collectNodesVisitor) applySuggestions() {
	for _, suggestion := range f.localSuggestions {
		f.nodes.addSuggestion(suggestion)
		itemId := len(f.nodes.items) - 1

		treeNode, _ := f.nodes.responseTree.Find(suggestion.treeNodeId)
		itemIds := treeNode.GetData()
		itemIds = append(itemIds, itemId)
		treeNode.SetData(itemIds)
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
	EnclosingTypeDefinition                                        ast.Node
	typeNameHandle                                                 unique.Handle[string]
	fieldNameHandle                                                unique.Handle[string]
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

	f.fieldInfo[fieldRef] = fieldInfo{
		typeName:                    typeName,
		typeNameHandle:              unique.Make(typeName),
		possibleTypeNames:           possibleTypes,
		fieldName:                   fieldName,
		fieldNameHandle:             unique.Make(fieldName),
		fieldAliasOrName:            fieldAliasOrName,
		parentPath:                  parentPath,
		currentPath:                 currentPath,
		onFragment:                  onFragment,
		parentPathWithoutFragment:   parentPathWithoutFragment,
		currentPathWithoutFragments: currentPathWithoutFragments,
		isTypeName:                  isTypeName,
		EnclosingTypeDefinition:     f.walker.EnclosingTypeDefinition,
	}
}
