package plan

import (
	"fmt"
	"slices"

	"github.com/pkg/errors"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

const typeNameField = "__typename"

type DataSourceFilter struct {
	operation  *ast.Document
	definition *ast.Document
	report     *operationreport.Report

	nodes NodeSuggestions
}

func NewDataSourceFilter(operation, definition *ast.Document, report *operationreport.Report) *DataSourceFilter {
	return &DataSourceFilter{
		operation:  operation,
		definition: definition,
		report:     report,
	}
}

func (f *DataSourceFilter) FilterDataSources(dataSources []DataSourceConfiguration, existingNodes NodeSuggestions, hints ...NodeSuggestionHint) (used []DataSourceConfiguration, suggestions NodeSuggestions) {
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

func (f *DataSourceFilter) findBestDataSourceSet(dataSources []DataSourceConfiguration, existingNodes NodeSuggestions, hints ...NodeSuggestionHint) NodeSuggestions {
	f.nodes = f.collectNodes(dataSources, existingNodes, hints...)
	if f.report.HasErrors() {
		return nil
	}

	f.selectUniqNodes()
	// f.printNodes("uniq nodes")
	f.selectDuplicateNodes(false)
	// f.printNodes("duplicate nodes")
	f.selectDuplicateNodes(true)
	// f.printNodes("duplicate nodes after second run")

	f.nodes = f.selectedNodes()

	f.isResolvable(f.nodes)
	if f.report.HasErrors() {
		return nil
	}

	return f.nodes
}

func (f *DataSourceFilter) printNodes(msg string) {
	fmt.Println(msg)
	for i := range f.nodes {
		fmt.Println(f.nodes[i].String())
	}
}

func (f *DataSourceFilter) collectNodes(dataSources []DataSourceConfiguration, existingNodes NodeSuggestions, hints ...NodeSuggestionHint) (nodes NodeSuggestions) {
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

func (f *DataSourceFilter) isResolvable(nodes []NodeSuggestion) {
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

	parentPathWithoutFragment *string
	onFragment                bool
	selected                  bool
	selectionReasons          []string
}

func (n *NodeSuggestion) appendSelectionReason(reason string) {
	// fmt.Println("ds:", n.DataSourceHash, fmt.Sprintf("%s.%s", n.TypeName, n.FieldName), "reason:", reason) // NOTE: debug do not remove
	n.selectionReasons = append(n.selectionReasons, reason)
}

func (n *NodeSuggestion) selectWithReason(reason string) {
	// n.appendSelectionReason(reason) // NOTE: debug do not remove
	if n.selected {
		return
	}
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

type NodeSuggestions []NodeSuggestion

func appendSuggestionWithPresenceCheck(nodes NodeSuggestions, node NodeSuggestion) NodeSuggestions {
	for i := range nodes {
		if nodes[i].TypeName == node.TypeName &&
			nodes[i].FieldName == node.FieldName &&
			nodes[i].Path == node.Path &&
			nodes[i].DataSourceHash == node.DataSourceHash {
			return nodes
		}
	}
	return append(nodes, node)
}

func (f NodeSuggestions) SuggestionsForPath(typeName, fieldName, path string) (dsHashes []DSHash) {
	for i := range f {
		if typeName == f[i].TypeName && fieldName == f[i].FieldName && path == f[i].Path {
			dsHashes = append(dsHashes, f[i].DataSourceHash)
		}
	}

	return dsHashes
}

func (f NodeSuggestions) HasSuggestionForPath(typeName, fieldName, path string) (dsHash DSHash, ok bool) {
	for i := range f {
		if typeName == f[i].TypeName && fieldName == f[i].FieldName && path == f[i].Path {
			return f[i].DataSourceHash, true
		}
	}

	return 0, false
}

func (f NodeSuggestions) isNodeUniq(idx int) bool {
	for i := range f {
		if i == idx {
			continue
		}
		if f[idx].TypeName == f[i].TypeName && f[idx].FieldName == f[i].FieldName && f[idx].Path == f[i].Path {
			return false
		}
	}
	return true
}

func (f NodeSuggestions) isSelectedOnOtherSource(idx int) bool {
	for i := range f {
		if i == idx {
			continue
		}
		if f[idx].TypeName == f[i].TypeName &&
			f[idx].FieldName == f[i].FieldName &&
			f[idx].Path == f[i].Path &&
			f[idx].DataSourceHash != f[i].DataSourceHash &&
			f[i].selected {

			return true
		}
	}
	return false
}

func (f NodeSuggestions) duplicatesOf(idx int) (out []int) {
	for i := range f {
		if i == idx {
			continue
		}
		if f[idx].TypeName == f[i].TypeName &&
			f[idx].FieldName == f[i].FieldName &&
			f[idx].Path == f[i].Path {
			out = append(out, i)
		}
	}
	return
}

func (f NodeSuggestions) childNodesOnSameSource(idx int) (out []int) {
	for i := range f {
		if i == idx {
			continue
		}
		if f[i].DataSourceHash != f[idx].DataSourceHash {
			continue
		}

		if f[i].ParentPath == f[idx].Path || (f[i].parentPathWithoutFragment != nil && *f[i].parentPathWithoutFragment == f[idx].Path) {
			out = append(out, i)
		}
	}
	return
}

func (f NodeSuggestions) siblingNodesOnSameSource(idx int) (out []int) {
	for i := range f {
		if i == idx {
			continue
		}
		if f[i].DataSourceHash != f[idx].DataSourceHash {
			continue
		}

		hasMatch := false
		switch {
		case f[i].parentPathWithoutFragment != nil && f[idx].parentPathWithoutFragment != nil:
			hasMatch = *f[i].parentPathWithoutFragment == *f[idx].parentPathWithoutFragment
		case f[i].parentPathWithoutFragment != nil && f[idx].parentPathWithoutFragment == nil:
			hasMatch = *f[i].parentPathWithoutFragment == f[idx].ParentPath
		case f[i].parentPathWithoutFragment == nil && f[idx].parentPathWithoutFragment != nil:
			hasMatch = f[i].ParentPath == *f[idx].parentPathWithoutFragment
		default:
			hasMatch = f[i].ParentPath == f[idx].ParentPath
		}

		if hasMatch {
			out = append(out, i)
		}
	}
	return
}

func (f NodeSuggestions) isLeaf(idx int) bool {
	for i := range f {
		if i == idx {
			continue
		}
		if f[i].ParentPath == f[idx].Path {
			return false
		}
	}
	return true
}

func (f NodeSuggestions) parentNodeOnSameSource(idx int) (parentIdx int, ok bool) {
	for i := range f {
		if i == idx {
			continue
		}
		if f[i].DataSourceHash != f[idx].DataSourceHash {
			continue
		}

		if f[i].Path == f[idx].ParentPath || (f[idx].parentPathWithoutFragment != nil && f[i].Path == *f[idx].parentPathWithoutFragment) {
			return i, true
		}
	}
	return -1, false
}

func (f NodeSuggestions) uniqueDataSourceHashes() map[DSHash]struct{} {
	if len(f) == 0 {
		return nil
	}

	unique := make(map[DSHash]struct{})
	for i := range f {
		unique[f[i].DataSourceHash] = struct{}{}
	}

	return unique
}

type nodesResolvableVisitor struct {
	operation  *ast.Document
	definition *ast.Document
	walker     *astvisitor.Walker

	nodes NodeSuggestions
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
	nodes       NodeSuggestions
	hints       []NodeSuggestionHint
}

func (f *collectNodesVisitor) EnterDocument(_, _ *ast.Document) {
	if !f.secondaryRun {
		f.nodes = make([]NodeSuggestion, 0, 32)
		return
	}

	if f.nodes == nil {
		panic("nodes should not be nil")
	}
}

func (f *collectNodesVisitor) EnterField(ref int) {
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

	var dsHashHint *DSHash
	for _, hint := range f.hints {
		if hint.fieldRef == ref {
			dsHashHint = &hint.dsHash
			break
		}
	}

	for _, v := range f.dataSources {
		if dsHashHint != nil && v.Hash() != *dsHashHint {
			continue
		}

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
			}

			if dsHashHint != nil {
				node.selectWithReason(ReasonKeyRequirementProvidedByPlanner)
			}

			if f.secondaryRun {
				f.nodes = appendSuggestionWithPresenceCheck(f.nodes, node)
			} else {
				f.nodes = append(f.nodes, node)
			}
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

// selectUniqNodes - selects nodes (e.g. fields) which are unique to a single datasource
// In addition we select:
//   - parent of such node if the node is a leaf and not nested under the fragment
//   - siblings nodes
func (f *DataSourceFilter) selectUniqNodes() {
	for i := range f.nodes {
		if f.nodes[i].selected {
			continue
		}

		isNodeUnique := f.nodes.isNodeUniq(i)
		if !isNodeUnique {
			continue
		}

		// unique nodes always have priority
		f.nodes[i].selectWithReason(ReasonStage1Uniq)

		if !f.nodes[i].onFragment { // on a first stage do not select parent of nodes on fragments
			// if node parents of the unique node is on the same source, prioritize it too
			f.selectUniqNodeParentsUpToRootNode(i)
		}

		// if node has leaf children on the same source, prioritize them too
		children := f.nodes.childNodesOnSameSource(i)
		for _, child := range children {
			if f.nodes.isLeaf(child) && f.nodes.isNodeUniq(child) {
				f.nodes[child].selectWithReason(ReasonStage1SameSourceLeafChild)
			}
		}

		// prioritize leaf siblings of the node on the same source
		siblings := f.nodes.siblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if f.nodes.isLeaf(sibling) && f.nodes.isNodeUniq(sibling) {
				f.nodes[sibling].selectWithReason(ReasonStage1SameSourceLeafSibling)
			}
		}
	}
}

func (f *DataSourceFilter) selectUniqNodeParentsUpToRootNode(i int) {
	// When we have a chain of datasource child nodes, we should select every parent until we reach the root node
	// as root node is a starting point from where we could get all theese child nodes

	if f.nodes[i].IsRootNode {
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
		f.nodes[parentIdx].selectWithReason(ReasonStage1SameSourceParent)

		current = parentIdx
		if f.nodes[current].IsRootNode {
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
	for i := range f.nodes {
		if f.nodes[i].selected {
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
			f.nodes[i].selectWithReason(ReasonStage3SelectAvailableNode)
		}
	}
}

func (f *DataSourceFilter) selectedNodes() (out NodeSuggestions) {
	return slices.DeleteFunc(f.nodes, func(e NodeSuggestion) bool {
		return !e.selected
	})
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
		if f.nodes[child].selected {
			f.nodes[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedChild)
			nodeIsSelected = true
			break
		}
	}
	return
}

func (f *DataSourceFilter) checkNodeSiblings(i int) (nodeIsSelected bool) {
	siblings := f.nodes.siblingNodesOnSameSource(i)
	for _, sibling := range siblings {
		if f.nodes[sibling].selected {
			f.nodes[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedSibling)
			nodeIsSelected = true
			break
		}
	}
	return
}

func (f *DataSourceFilter) checkNodeParent(i int) (nodeIsSelected bool) {
	parentIdx, ok := f.nodes.parentNodeOnSameSource(i)
	if ok && f.nodes[parentIdx].selected {
		f.nodes[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedParent)
		nodeIsSelected = true
	}

	return
}
