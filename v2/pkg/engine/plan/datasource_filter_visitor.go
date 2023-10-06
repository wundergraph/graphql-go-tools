package plan

import (
	"fmt"

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
}

func NewDataSourceFilter(operation, definition *ast.Document, report *operationreport.Report) *DataSourceFilter {
	return &DataSourceFilter{
		operation:  operation,
		definition: definition,
		report:     report,
	}
}

func (f *DataSourceFilter) FilterDataSources(dataSources []DataSourceConfiguration, existingNodes NodeSuggestions) (used []DataSourceConfiguration, suggestions NodeSuggestions) {
	suggestions = f.findBestDataSourceSet(dataSources, existingNodes)
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

func (f *DataSourceFilter) findBestDataSourceSet(dataSources []DataSourceConfiguration, existingNodes NodeSuggestions) NodeSuggestions {
	nodes := f.collectNodes(dataSources, existingNodes)
	if f.report.HasErrors() {
		return nil
	}

	nodes = selectUniqNodes(nodes)
	nodes = selectDuplicateNodes(nodes, false)
	nodes = selectDuplicateNodes(nodes, true)

	nodes = selectedNodes(nodes)

	f.isResolvable(nodes)
	if f.report.HasErrors() {
		return nil
	}

	return nodes
}

func (f *DataSourceFilter) collectNodes(dataSources []DataSourceConfiguration, existingNodes NodeSuggestions) (nodes NodeSuggestions) {
	secondaryRun := existingNodes != nil

	walker := astvisitor.NewWalker(32)
	visitor := &collectNodesVisitor{
		operation:    f.operation,
		definition:   f.definition,
		walker:       &walker,
		dataSources:  dataSources,
		nodes:        existingNodes,
		secondaryRun: secondaryRun,
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

	parentPathWithoutFragment string
	onFragment                bool
	selected                  bool
	selectionReasons          []string
}

func (n *NodeSuggestion) appendSelectionReason(reason string) {
	// fmt.Println("ds:", n.DataSourceHash, fmt.Sprintf("%s.%s", n.TypeName, n.FieldName), "reason:", reason) // NOTE: debug do not remove
	n.selectionReasons = append(n.selectionReasons, reason)
}

func (n *NodeSuggestion) selectWithReason(reason string) {
	if n.selected {
		return
	}
	n.selected = true
	// n.appendSelectionReason(reason) // NOTE: debug do not remove
}

func (n *NodeSuggestion) String() string {
	return fmt.Sprintf(`{"ds":%d,"path":"%s","typeName":"%s","fieldName":"%s","isRootNode":%t}`, n.DataSourceHash, n.Path, n.TypeName, n.FieldName, n.IsRootNode)
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

func (f NodeSuggestions) SuggestionForPath(typeName, fieldName, path string) (suggestion NodeSuggestion, ok bool) {
	if len(f) == 0 {
		return NodeSuggestion{}, false
	}

	for i := range f {
		if typeName == f[i].TypeName && fieldName == f[i].FieldName && path == f[i].Path {
			return f[i], true
		}
	}
	return NodeSuggestion{}, false
}

func (f NodeSuggestions) HasSuggestionForPath(typeName, fieldName, path string) (dsHash DSHash, ok bool) {
	suggestion, ok := f.SuggestionForPath(typeName, fieldName, path)
	if ok {
		return suggestion.DataSourceHash, true
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

		if f[i].ParentPath == f[idx].Path || f[i].parentPathWithoutFragment == f[idx].Path {
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

		identicalParentPath := f[i].ParentPath == f[idx].ParentPath
		identicalParentPathWithoutFragment := f[i].parentPathWithoutFragment == f[idx].parentPathWithoutFragment
		idxParentOtherFragment := f[i].parentPathWithoutFragment == f[idx].ParentPath
		otherParentIdxFragment := f[i].ParentPath == f[idx].parentPathWithoutFragment

		if identicalParentPath ||
			identicalParentPathWithoutFragment ||
			idxParentOtherFragment ||
			otherParentIdxFragment {

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

		if f[i].Path == f[idx].ParentPath || f[i].Path == f[idx].parentPathWithoutFragment {
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
	operation  *ast.Document
	definition *ast.Document
	walker     *astvisitor.Walker

	dataSources  []DataSourceConfiguration
	nodes        NodeSuggestions
	secondaryRun bool
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
	var parentPathWithoutFragment string
	if onFragment {
		parentPathWithoutFragment = f.walker.Path[:len(f.walker.Path)-1].DotDelimitedString()
	}

	currentPath := parentPath + "." + fieldAliasOrName

	for _, v := range f.dataSources {
		hasRootNode := v.HasRootNode(typeName, fieldName) || (isTypeName && v.HasRootNodeWithTypename(typeName))
		hasChildNode := v.HasChildNode(typeName, fieldName) || (isTypeName && v.HasChildNodeWithTypename(typeName))

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

	ReasonStage2SameSourceNodeOfSelectedParent          = "stage2: node on the same source as selected parent"
	ReasonStage2SameSourceDuplicateNodeOfSelectedParent = "stage2: duplicate node on the same source as selected parent"
	ReasonStage2SameSourceNodeOfSelectedChild           = "stage2: node on the same source as selected child"
	ReasonStage2SameSourceNodeOfSelectedSibling         = "stage2: node on the same source as selected sibling"

	ReasonStage3SelectAvailableNode = "stage3: select first available node"
)

func selectUniqNodes(nodes NodeSuggestions) []NodeSuggestion {
	for i := range nodes {
		if nodes[i].selected {
			continue
		}

		isNodeUniq := nodes.isNodeUniq(i)
		if !isNodeUniq {
			continue
		}

		// uniq nodes are always has priority
		nodes[i].selectWithReason(ReasonStage1Uniq)

		if !nodes[i].onFragment { // on a first stage do not select parent of nodes on fragments
			// if node parent of the uniq node is on the same source, prioritize it too
			parentIdx, ok := nodes.parentNodeOnSameSource(i)
			if ok {
				nodes[parentIdx].selectWithReason(ReasonStage1SameSourceParent)
			}
		}

		// if node has leaf childs on the same source, prioritize them too
		childs := nodes.childNodesOnSameSource(i)
		for _, child := range childs {
			if nodes.isLeaf(child) && nodes.isNodeUniq(child) {
				nodes[child].selectWithReason(ReasonStage1SameSourceLeafChild)
			}
		}

		// prioritize leaf siblings of the node on the same source
		siblings := nodes.siblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if nodes.isLeaf(sibling) && nodes.isNodeUniq(sibling) {
				nodes[sibling].selectWithReason(ReasonStage1SameSourceLeafSibling)
			}
		}
	}
	return nodes
}

func selectDuplicateNodes(nodes NodeSuggestions, secondRun bool) []NodeSuggestion {
	for i := range nodes {
		if nodes[i].selected {
			continue
		}

		if nodes.isSelectedOnOtherSource(i) {
			continue
		}

		// if node parent on the same source as the current node
		parentIdx, ok := nodes.parentNodeOnSameSource(i)
		if ok && nodes[parentIdx].selected {
			nodes[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedParent)
			continue
		}

		isSelected := false

		// check if duplicates are on the same source as parent node
		nodeDuplicates := nodes.duplicatesOf(i)
		for _, duplicate := range nodeDuplicates {
			parentIdx, ok := nodes.parentNodeOnSameSource(duplicate)
			if ok && nodes[parentIdx].selected {
				nodes[duplicate].selectWithReason(ReasonStage2SameSourceDuplicateNodeOfSelectedParent)
				isSelected = true
				break
			}
		}
		if isSelected {
			continue
		}

		childs := nodes.childNodesOnSameSource(i)
		for _, child := range childs {
			if nodes[child].selected {
				nodes[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedChild)
				isSelected = true
				break
			}
		}
		if isSelected {
			continue
		}

		siblings := nodes.siblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if nodes[sibling].selected {
				nodes[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedSibling)
				isSelected = true
				break
			}
		}
		if isSelected {
			continue
		}

		if secondRun {
			nodes[i].selectWithReason(ReasonStage3SelectAvailableNode)
		}
	}
	return nodes
}

func selectedNodes(nodes NodeSuggestions) (out NodeSuggestions) {
	for i := range nodes {
		if nodes[i].selected {
			out = append(out, nodes[i])
		}
	}
	return
}
