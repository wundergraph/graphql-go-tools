package plan

import (
	"fmt"
	"slices"

	"github.com/pkg/errors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const typeNameField = "__typename"

type DataSourceFilter struct {
	operation  *ast.Document
	definition *ast.Document
	report     *operationreport.Report

	nodes *NodeSuggestions
}

func NewDataSourceFilter(operation, definition *ast.Document, report *operationreport.Report) *DataSourceFilter {
	return &DataSourceFilter{
		operation:  operation,
		definition: definition,
		report:     report,
	}
}

func (f *DataSourceFilter) FilterDataSources(dataSources []DataSourceConfiguration, existingNodes *NodeSuggestions, hints ...NodeSuggestionHint) (used []DataSourceConfiguration, suggestions *NodeSuggestions) {
	suggestions = f.findBestDataSourceSet(dataSources, existingNodes, hints...)
	if f.report.HasErrors() {
		return
	}

	dsInUse := suggestions.uniqueDataSourceHashes()

	used = make([]DataSourceConfiguration, 0, len(dsInUse))

	for i := range dataSources {
		_, inUse := dsInUse[dataSources[i].Hash()]
		if inUse {
			used = append(used, dataSources[i])
		}
	}

	return used, suggestions
}

func (f *DataSourceFilter) findBestDataSourceSet(dataSources []DataSourceConfiguration, existingNodes *NodeSuggestions, hints ...NodeSuggestionHint) *NodeSuggestions {
	f.nodes = f.collectNodes(dataSources, existingNodes)
	if f.report.HasErrors() {
		return nil
	}

	// f.nodes.printNodes("initial nodes")

	f.applySuggestionHints(hints)
	// f.nodes.printNodes("nodes after applying hints")

	f.selectUniqNodes()
	// f.nodes.printNodes("uniq nodes")
	f.selectDuplicateNodes(false)
	// f.nodes.printNodes("duplicate nodes")
	f.selectDuplicateNodes(true)
	// f.nodes.printNodes("duplicate nodes after second run")

	f.nodes.populateHasSuggestions()

	f.isResolvable(f.nodes)
	if f.report.HasErrors() {
		return nil
	}

	return f.nodes
}

func (f *DataSourceFilter) collectNodes(dataSources []DataSourceConfiguration, existingNodes *NodeSuggestions, hints ...NodeSuggestionHint) (nodes *NodeSuggestions) {
	secondaryRun := existingNodes != nil

	walker := astvisitor.NewWalker(32)
	visitor := &collectNodesVisitor{
		operation:    f.operation,
		definition:   f.definition,
		walker:       &walker,
		dataSources:  dataSources,
		secondaryRun: secondaryRun,
		nodes:        existingNodes,
		hints:        hints,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(f.operation, f.definition, f.report)
	return visitor.nodes
}

func (f *DataSourceFilter) isResolvable(nodes *NodeSuggestions) {
	walker := astvisitor.NewWalker(32)
	visitor := &nodesResolvableVisitor{
		operation:  f.operation,
		definition: f.definition,
		walker:     &walker,
		nodes:      nodes,
	}
	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(f.operation, f.definition, f.report)
}

type NodeSuggestion struct {
	TypeName       string
	FieldName      string
	DataSourceHash DSHash
	Path           string
	ParentPath     string
	IsRootNode     bool
	LessPreferable bool // is true in case the node is an entity root node and has a key with disabled resolver

	parentPathWithoutFragment *string
	onFragment                bool
	selected                  bool
	selectionReasons          []string

	fieldRef int
}

func (n *NodeSuggestion) appendSelectionReason(reason string) {
	// fmt.Println("ds:", n.DataSourceHash, fmt.Sprintf("%s.%s", n.TypeName, n.FieldName), "reason:", reason) // NOTE: debug do not remove
	n.selectionReasons = append(n.selectionReasons, reason)
}

func (n *NodeSuggestion) selectWithReason(reason string) {
	if n.selected {
		return
	}
	// n.appendSelectionReason(reason) // NOTE: debug do not remove
	n.selected = true
}

func (n *NodeSuggestion) String() string {
	return fmt.Sprintf(`{"ds":%d,"path":"%s","typeName":"%s","fieldName":"%s","isRootNode":%t, "isSelected": %v,"select reason": %v}`,
		n.DataSourceHash, n.Path, n.TypeName, n.FieldName, n.IsRootNode, n.selected, n.selectionReasons)
}

type NodeSuggestionHint struct {
	fieldRef int
	dsHash   DSHash

	fieldName  string
	parentPath string
}

type NodeSuggestions struct {
	items           []*NodeSuggestion
	hasher          *xxhash.Digest
	pathSuggestions map[string][]*NodeSuggestion
	seenFields map[int]struct{}
}

func NewNodeSuggestions() *NodeSuggestions {
	return NewNodeSuggestionsWithSize(32)
}

func NewNodeSuggestionsWithSize(size int) *NodeSuggestions {
	return &NodeSuggestions{
		items:      make([]*NodeSuggestion, 0, size),
		seenFields: make(map[int]struct{}, size),
		pathSuggestions: make(map[string][]*NodeSuggestion),
	}
}

func (f *NodeSuggestions) IsFieldSeen(fieldRef int) bool {
	_, ok := f.seenFields[fieldRef]
	return ok
}

func (f *NodeSuggestions) AddSeenField(fieldRef int) {
	f.seenFields[fieldRef] = struct{}{}
}

func (f *NodeSuggestions) addSuggestion(node *NodeSuggestion) {
	f.items = append(f.items, node)
}

func (f *NodeSuggestions) SuggestionsForPath(typeName, fieldName, path string) (dsHashes []DSHash) {
	items, ok := f.pathSuggestions[path]
	if !ok {
		return nil
	}

	for i := range items {
		if typeName == items[i].TypeName && fieldName == items[i].FieldName && items[i].selected {
			dsHashes = append(dsHashes, items[i].DataSourceHash)
		}
	}

	return dsHashes
}

func (f *NodeSuggestions) HasSuggestionForPath(typeName, fieldName, path string) (dsHash DSHash, ok bool) {
	items, ok := f.pathSuggestions[path]
	if !ok {
		return 0, false
	}

	for i := range items {
		if typeName == items[i].TypeName && fieldName == items[i].FieldName && items[i].selected {
			return items[i].DataSourceHash, true
		}
	}

	return 0, false
}

func (f *NodeSuggestions) isNodeUniq(idx int) bool {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[idx].TypeName == f.items[i].TypeName && f.items[idx].FieldName == f.items[i].FieldName && f.items[idx].Path == f.items[i].Path {
			return false
		}
	}
	return true
}

func (f *NodeSuggestions) isSelectedOnOtherSource(idx int) bool {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[idx].TypeName == f.items[i].TypeName &&
			f.items[idx].FieldName == f.items[i].FieldName &&
			f.items[idx].Path == f.items[i].Path &&
			f.items[idx].DataSourceHash != f.items[i].DataSourceHash &&
			f.items[i].selected {

			return true
		}
	}
	return false
}

func (f *NodeSuggestions) duplicatesOf(idx int) (out []int) {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[idx].TypeName == f.items[i].TypeName &&
			f.items[idx].FieldName == f.items[i].FieldName &&
			f.items[idx].Path == f.items[i].Path {
			out = append(out, i)
		}
	}
	return
}

func (f *NodeSuggestions) childNodesOnSameSource(idx int) (out []int) {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[i].DataSourceHash != f.items[idx].DataSourceHash {
			continue
		}

		if f.items[i].ParentPath == f.items[idx].Path || (f.items[i].parentPathWithoutFragment != nil && *f.items[i].parentPathWithoutFragment == f.items[idx].Path) {
			out = append(out, i)
		}
	}
	return
}

func (f *NodeSuggestions) siblingNodesOnSameSource(idx int) (out []int) {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[i].DataSourceHash != f.items[idx].DataSourceHash {
			continue
		}

		hasMatch := false
		switch {
		case f.items[i].parentPathWithoutFragment != nil && f.items[idx].parentPathWithoutFragment != nil:
			hasMatch = *f.items[i].parentPathWithoutFragment == *f.items[idx].parentPathWithoutFragment
		case f.items[i].parentPathWithoutFragment != nil && f.items[idx].parentPathWithoutFragment == nil:
			hasMatch = *f.items[i].parentPathWithoutFragment == f.items[idx].ParentPath
		case f.items[i].parentPathWithoutFragment == nil && f.items[idx].parentPathWithoutFragment != nil:
			hasMatch = f.items[i].ParentPath == *f.items[idx].parentPathWithoutFragment
		default:
			hasMatch = f.items[i].ParentPath == f.items[idx].ParentPath
		}

		if hasMatch {
			out = append(out, i)
		}
	}
	return
}

func (f *NodeSuggestions) isLeaf(idx int) bool {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[i].ParentPath == f.items[idx].Path {
			return false
		}
	}
	return true
}

func (f *NodeSuggestions) parentNodeOnSameSource(idx int) (parentIdx int, ok bool) {
	for i := range f.items {
		if i == idx {
			continue
		}
		if f.items[i].DataSourceHash != f.items[idx].DataSourceHash {
			continue
		}

		if f.items[i].Path == f.items[idx].ParentPath || (f.items[idx].parentPathWithoutFragment != nil && f.items[i].Path == *f.items[idx].parentPathWithoutFragment) {
			return i, true
		}
	}
	return -1, false
}

func (f *NodeSuggestions) uniqueDataSourceHashes() map[DSHash]struct{} {
	if len(f.items) == 0 {
		return nil
	}

	unique := make(map[DSHash]struct{})
	for i := range f.items {
		unique[f.items[i].DataSourceHash] = struct{}{}
	}

	return unique
}

func (f *NodeSuggestions) printNodes(msg string) {
	if msg != "" {
		fmt.Println(msg)
	}
	for i := range f.items {
		fmt.Println(f.items[i].String())
	}
}

func (f *NodeSuggestions) populateHasSuggestions() {
	for i := range f.items {
		if !f.items[i].selected {
			continue
		}

		suggestions, _ := f.pathSuggestions[f.items[i].Path]
		suggestions = append(f.pathSuggestions[f.items[i].Path], f.items[i])
		f.pathSuggestions[f.items[i].Path] = suggestions
	}
}

type nodesResolvableVisitor struct {
	operation  *ast.Document
	definition *ast.Document
	walker     *astvisitor.Walker

	nodes *NodeSuggestions
}

func (f *nodesResolvableVisitor) EnterField(ref int) {
	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)
	fieldName := f.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := f.operation.FieldAliasOrNameString(ref)

	isTypeName := fieldName == typeNameField
	isUnionParent := f.walker.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition

	if isUnionParent && isTypeName {
		// typename field on union parent is always resolvable
		return
	}

	parentPath := f.walker.Path.DotDelimitedString()
	currentPath := parentPath + "." + fieldAliasOrName

	_, found := f.nodes.HasSuggestionForPath(typeName, fieldName, currentPath)
	if !found {
		f.walker.StopWithInternalErr(errors.Wrap(&errOperationFieldNotResolved{TypeName: typeName, FieldName: fieldName, Path: currentPath}, "nodesResolvableVisitor"))
	}
}

type collectNodesVisitor struct {
	operation    *ast.Document
	definition   *ast.Document
	walker       *astvisitor.Walker
	secondaryRun bool

	dataSources []DataSourceConfiguration
	nodes       *NodeSuggestions
	hints       []NodeSuggestionHint
}

func (f *collectNodesVisitor) EnterDocument(_, _ *ast.Document) {
	if !f.secondaryRun {
		f.nodes = NewNodeSuggestions()
		return
	}

	if f.nodes == nil {
		panic("nodes should not be nil")
	}
}

func (f *collectNodesVisitor) EnterField(ref int) {
	if f.nodes.IsFieldSeen(ref) {
		return
	}
	f.nodes.AddSeenField(ref)

	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)
	fieldName := f.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := f.operation.FieldAliasOrNameString(ref)

	isTypeName := fieldName == typeNameField
	parentPath := f.walker.Path.DotDelimitedString()
	onFragment := f.walker.Path.EndsWithFragment()
	var parentPathWithoutFragment *string
	if onFragment {
		p := f.walker.Path[:len(f.walker.Path)-1].DotDelimitedString()
		parentPathWithoutFragment = &p
	}

	currentPath := parentPath + "." + fieldAliasOrName

	for _, v := range f.dataSources {
		hasRootNode := v.HasRootNode(typeName, fieldName) || (isTypeName && v.HasRootNodeWithTypename(typeName))
		hasChildNode := v.HasChildNode(typeName, fieldName) || (isTypeName && v.HasChildNodeWithTypename(typeName))

		allowTypeName := true
		// we should not select a typename on the interface object
		for _, k := range v.FederationMetaData.InterfaceObjects {
			if k.InterfaceTypeName == typeName || slices.Contains(k.ConcreteTypeNames, typeName) {
				allowTypeName = false
				break
			}
		}

		lessPreferable := false
		if hasRootNode {
			for _, k := range v.FederationMetaData.Keys {
				if k.TypeName == typeName && k.DisableEntityResolver {
					lessPreferable = true
					break
				}
			}
		}

		if !allowTypeName && isTypeName {
			continue
		}

		if hasRootNode || hasChildNode {
			node := NodeSuggestion{
				TypeName:                  typeName,
				FieldName:                 fieldName,
				DataSourceHash:            v.Hash(),
				Path:                      currentPath,
				ParentPath:                parentPath,
				IsRootNode:                hasRootNode,
				onFragment:                onFragment,
				parentPathWithoutFragment: parentPathWithoutFragment,
				fieldRef:                  ref,
				LessPreferable:            lessPreferable,
			}

			f.nodes.addSuggestion(&node)
		}
	}
}

type errOperationFieldNotResolved struct {
	TypeName  string
	FieldName string
	Path      string
}

func (e *errOperationFieldNotResolved) Error() string {
	return fmt.Sprintf("could not select the datasource to resolve %s.%s on a path %s", e.TypeName, e.FieldName, e.Path)
}

const (
	ReasonStage1Uniq                  = "stage1: uniq"
	ReasonStage1SameSourceParent      = "stage1: same source parent of uniq node"
	ReasonStage1SameSourceLeafChild   = "stage1: same source leaf child of uniq node"
	ReasonStage1SameSourceLeafSibling = "stage1: same source leaf sibling of uniq node"

	ReasonStage2SameSourceNodeOfSelectedParent  = "stage2: node on the same source as selected parent"
	ReasonStage2SameSourceNodeOfSelectedChild   = "stage2: node on the same source as selected child"
	ReasonStage2SameSourceNodeOfSelectedSibling = "stage2: node on the same source as selected sibling"

	ReasonStage3SelectAvailableNode = "stage3: select first available node"

	ReasonKeyRequirementProvidedByPlanner = "provided by planner as required by @key"
)

func (f *DataSourceFilter) applySuggestionHints(hints []NodeSuggestionHint) {
	if len(hints) == 0 {
		return
	}

	for i := range f.nodes.items {
		var dsHashHint *DSHash
		for _, hint := range hints {
			if hint.fieldRef == f.nodes.items[i].fieldRef {
				dsHashHint = &hint.dsHash
				break
			}
		}

		if dsHashHint == nil {
			continue
		}

		if f.nodes.items[i].DataSourceHash != *dsHashHint {
			if f.nodes.items[i].selected {
				// if the node was already selected by another datasource
				// we unselect it
				f.nodes.items[i].selected = false
				f.nodes.items[i].selectionReasons = nil
			}
		} else {
			f.nodes.items[i].selectWithReason(ReasonKeyRequirementProvidedByPlanner)
		}
	}
}

// selectUniqNodes - selects nodes (e.g. fields) which are unique to a single datasource
// In addition we select:
//   - parent of such node if the node is a leaf and not nested under the fragment
//   - siblings nodes
func (f *DataSourceFilter) selectUniqNodes() {
	for i := range f.nodes.items {
		if f.nodes.items[i].selected {
			continue
		}

		isNodeUnique := f.nodes.isNodeUniq(i)
		if !isNodeUnique {
			continue
		}

		// unique nodes always have priority
		f.nodes.items[i].selectWithReason(ReasonStage1Uniq)

		if !f.nodes.items[i].onFragment { // on a first stage do not select parent of nodes on fragments
			// if node parents of the unique node is on the same source, prioritize it too
			f.selectUniqNodeParentsUpToRootNode(i)
		}

		// if node has leaf children on the same source, prioritize them too
		children := f.nodes.childNodesOnSameSource(i)
		for _, child := range children {
			if f.nodes.isLeaf(child) && f.nodes.isNodeUniq(child) {
				f.nodes.items[child].selectWithReason(ReasonStage1SameSourceLeafChild)
			}
		}

		// prioritize leaf siblings of the node on the same source
		siblings := f.nodes.siblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if f.nodes.isLeaf(sibling) && f.nodes.isNodeUniq(sibling) {
				f.nodes.items[sibling].selectWithReason(ReasonStage1SameSourceLeafSibling)
			}
		}
	}
}

func (f *DataSourceFilter) selectUniqNodeParentsUpToRootNode(i int) {
	// When we have a chain of datasource child nodes, we should select every parent until we reach the root node
	// as root node is a starting point from where we could get all theese child nodes

	if f.nodes.items[i].IsRootNode {
		// no need to select parent of a root node here
		// as it could be resolved by itself
		return
	}

	current := i
	for {
		parentIdx, ok := f.nodes.parentNodeOnSameSource(current)
		if !ok {
			break
		}
		f.nodes.items[parentIdx].selectWithReason(ReasonStage1SameSourceParent)

		current = parentIdx
		if f.nodes.items[current].IsRootNode {
			break
		}
	}
}

// selectDuplicateNodes - selects nodes (e.g. fields) which are not unique to a single datasource,
// e.g. could be resolved by multiple datasources
// This method checks only nodes not already selected on the other datasource
// On a first run we are doing set of checks of surrounding nodes selection for the current analyzed node and each of its duplicates:
//   - check for selected parent of a current node or its duplicates
//   - check for selected childs of a current node or its duplicates
//   - check for selected siblings of a current node or its duplicates
//
// On a second run in additional to all the checks from the first run
// we select nodes which was not choosen by previous stages, so we just pick first available datasource
func (f *DataSourceFilter) selectDuplicateNodes(secondRun bool) {
	for i := range f.nodes.items {
		if f.nodes.items[i].selected {
			continue
		}

		if f.nodes.isSelectedOnOtherSource(i) {
			continue
		}

		nodeDuplicates := f.nodes.duplicatesOf(i)

		// check for selected parent of a current node or its duplicates
		if f.checkNodeParent(i) {
			continue
		}
		if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeParent) {
			continue
		}

		// check for selected childs of a current node or its duplicates
		if f.checkNodeChilds(i) {
			continue
		}
		if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeChilds) {
			continue
		}

		// check for selected siblings of a current node or its duplicates
		if f.checkNodeSiblings(i) {
			continue
		}
		if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeSiblings) {
			continue
		}

		// if after all checks node was not selected, select it
		// this could happen in case choises are fully equal
		if secondRun {
			// in case current node suggestion is an entity root node, and it contains key with disabled resolver
			// it makes such node less preferable for selection
			if f.nodes[i].LessPreferable {
				continue
			}
			f.nodes[i].selectWithReason(ReasonStage3SelectAvailableNode)
		}
	}
}

func (f *DataSourceFilter) checkNodeDuplicates(duplicates []int, callback func(nodeIdx int) (nodeIsSelected bool)) (nodeIsSelected bool) {
	for _, duplicate := range duplicates {
		if callback(duplicate) {
			nodeIsSelected = true
			break
		}
	}
	return
}

func (f *DataSourceFilter) checkNodeChilds(i int) (nodeIsSelected bool) {
	childs := f.nodes.childNodesOnSameSource(i)
	for _, child := range childs {
		if f.nodes.items[child].selected {
			f.nodes.items[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedChild)
			nodeIsSelected = true
			break
		}
	}
	return
}

func (f *DataSourceFilter) checkNodeSiblings(i int) (nodeIsSelected bool) {
	siblings := f.nodes.siblingNodesOnSameSource(i)
	for _, sibling := range siblings {
		if f.nodes.items[sibling].selected {
			f.nodes.items[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedSibling)
			nodeIsSelected = true
			break
		}
	}
	return
}

func (f *DataSourceFilter) checkNodeParent(i int) (nodeIsSelected bool) {
	parentIdx, ok := f.nodes.parentNodeOnSameSource(i)
	if ok && f.nodes.items[parentIdx].selected {
		f.nodes.items[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedParent)
		nodeIsSelected = true
	}

	return
}
