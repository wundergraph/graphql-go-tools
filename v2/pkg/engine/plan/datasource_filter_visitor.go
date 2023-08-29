package plan

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func FilterDataSources(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) (used, unused []DataSourceConfiguration, suggestions NodeSuggestions) {
	suggestions = findBestDataSourceSet(operation, definition, report, dataSources)
	if report.HasErrors() {
		return
	}

	dsInUse := suggestions.UniqueDataSourceHashes()

	used = make([]DataSourceConfiguration, 0, len(dsInUse))
	unused = make([]DataSourceConfiguration, 0, len(dataSources)-len(dsInUse))

	for i := range dataSources {
		_, inUse := dsInUse[dataSources[i].Hash()]
		if inUse {
			used = append(used, dataSources[i])
		} else {
			unused = append(unused, dataSources[i])
		}
	}

	return used, unused, suggestions
}

type NodeSuggestion struct {
	TypeName       string
	FieldName      string
	DataSourceHash DSHash
	Path           string
	ParentPath     string
	IsRootNode     bool
	IsProvided     bool

	hasPriority  bool
	whyWasChosen []string
}

func (n *NodeSuggestion) WhyWasChosen(reason string) {
	// fmt.Println("ds:", n.DataSourceHash, fmt.Sprintf("%s.%s", n.TypeName, n.FieldName), "reason:", reason) // NOTE: debug do not remove
	n.whyWasChosen = append(n.whyWasChosen, reason)
}

func (n *NodeSuggestion) SetPriorityWithReason(reason string) {
	if n.hasPriority {
		return
	}
	n.hasPriority = true
	// n.WhyWasChosen(reason) // NOTE: debug do not remove
}

type NodeSuggestions []NodeSuggestion

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

func (f NodeSuggestions) IsNodeUniq(idx int) bool {
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

func (f NodeSuggestions) HasPriorityOnOtherSource(idx int) bool {
	for i := range f {
		if i == idx {
			continue
		}
		if f[idx].TypeName == f[i].TypeName &&
			f[idx].FieldName == f[i].FieldName &&
			f[idx].Path == f[i].Path &&
			f[idx].DataSourceHash != f[i].DataSourceHash &&
			f[i].hasPriority {

			return true
		}
	}
	return false
}

func (f NodeSuggestions) DuplicatesOf(idx int) (out []int) {
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

func (f NodeSuggestions) ChildNodesOnSameSource(idx int) (out []int) {
	for i := range f {
		if i == idx {
			continue
		}
		if f[i].DataSourceHash == f[idx].DataSourceHash && f[i].ParentPath == f[idx].Path {
			out = append(out, i)
		}
	}
	return
}

func (f NodeSuggestions) SiblingNodesOnSameSource(idx int) (out []int) {
	for i := range f {
		if i == idx {
			continue
		}
		if f[i].DataSourceHash == f[idx].DataSourceHash && f[i].ParentPath == f[idx].ParentPath {
			out = append(out, i)
		}
	}
	return
}

func (f NodeSuggestions) IsLeaf(idx int) bool {
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

func (f NodeSuggestions) ParentNodeOnSameSource(idx int) (parentIdx int, ok bool) {
	for i := range f {
		if i == idx {
			continue
		}
		if f[i].DataSourceHash == f[idx].DataSourceHash && f[i].Path == f[idx].ParentPath {
			return i, true
		}
	}
	return -1, false
}

func (f NodeSuggestions) DataSourceCount() int {
	return len(f.UniqueDataSourceHashes())
}

func (f NodeSuggestions) Count() int {
	return len(f)
}

func (f NodeSuggestions) UniqueDataSourceHashes() map[DSHash]struct{} {
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

	parentPath := f.walker.Path.DotDelimitedString()
	currentPath := parentPath + "." + fieldAliasOrName

	_, found := f.nodes.HasSuggestionForPath(typeName, fieldName, currentPath)
	if !found {
		f.walker.StopWithInternalErr(&errOperationFieldNotResolved{TypeName: typeName, FieldName: fieldName, Path: currentPath})
	}
}

func isResolvable(operation, definition *ast.Document, report *operationreport.Report, nodes []NodeSuggestion) {
	walker := astvisitor.NewWalker(32)
	visitor := &nodesResolvableVisitor{
		operation:  operation,
		definition: definition,
		walker:     &walker,
		nodes:      nodes,
	}
	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(operation, definition, report)
}

type collectNodesVisitor struct {
	operation  *ast.Document
	definition *ast.Document
	walker     *astvisitor.Walker

	dataSources []DataSourceConfiguration

	nodes []NodeSuggestion
}

func (f *collectNodesVisitor) EnterDocument(_, _ *ast.Document) {
	f.nodes = make([]NodeSuggestion, 0)
}

func (f *collectNodesVisitor) EnterField(ref int) {
	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)
	fieldName := f.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := f.operation.FieldAliasOrNameString(ref)

	parentPath := f.walker.Path.DotDelimitedString()
	currentPath := parentPath + "." + fieldAliasOrName

	for _, v := range f.dataSources {
		if v.HasRootNode(typeName, fieldName) {
			f.nodes = append(f.nodes, NodeSuggestion{
				TypeName:       typeName,
				FieldName:      fieldName,
				DataSourceHash: v.Hash(),
				Path:           currentPath,
				IsRootNode:     true,
				ParentPath:     parentPath,
			})
		}
		if v.HasChildNode(typeName, fieldName) {
			f.nodes = append(f.nodes, NodeSuggestion{
				TypeName:       typeName,
				FieldName:      fieldName,
				DataSourceHash: v.Hash(),
				Path:           currentPath,
				ParentPath:     parentPath,
			})
		}
	}
}

func collectNodes(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) (nodes NodeSuggestions) {
	walker := astvisitor.NewWalker(32)
	visitor := &collectNodesVisitor{
		operation:   operation,
		definition:  definition,
		walker:      &walker,
		dataSources: dataSources,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(operation, definition, report)
	return visitor.nodes
}

type errOperationFieldNotResolved struct {
	TypeName  string
	FieldName string
	Path      string
}

func (e *errOperationFieldNotResolved) Error() string {
	return fmt.Sprintf("could not find datasource to resolve %s.%s on a path %s", e.TypeName, e.FieldName, e.Path)
}

// func findBestNodes(operation, definition *ast.Document, nodes NodeSuggestions) NodeSuggestions {
// 	current := nodes
// 	currentDsCount := nodes.DataSourceCount()
// 	currentNodeCount := nodes.Count()
// 	for excluded := range nodes {
// 		if nodes[excluded].hasPriority {
// 			continue
// 		}
//
// 		subset := nodesSubset(nodes, excluded)
// 		if !isResolvable(operation, definition, subset) {
// 			continue
// 		}
//
// 		resultNodes := findBestNodes(operation, definition, subset)
// 		resultDsCount := resultNodes.DataSourceCount()
// 		resultNodeCount := resultNodes.Count()
//
// 		if resultNodeCount < currentNodeCount && resultDsCount <= currentDsCount {
// 			current = resultNodes
// 			currentDsCount = resultDsCount
// 			currentNodeCount = resultNodeCount
// 		}
// 	}
//
// 	return current
// }

func findBestDataSourceSet(operation *ast.Document, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) NodeSuggestions {
	nodes := collectNodes(operation, definition, report, dataSources)
	if report.HasErrors() {
		return nil
	}

	nodes = preserveUniqNodes(nodes)
	nodes = preserveDuplicateNodes(nodes, false)
	nodes = preserveDuplicateNodes(nodes, true)

	// nodes = findBestNodes(operation, definition, nodes)

	nodes = nodesWithPriority(nodes)
	isResolvable(operation, definition, report, nodes)
	if report.HasErrors() {
		return nil
	}

	return nodes
}

func nodesSubset(suggestions []NodeSuggestion, exclude int) []NodeSuggestion {
	subset := make([]NodeSuggestion, 0, len(suggestions)-1)
	subset = append(subset, suggestions[:exclude]...)
	subset = append(subset, suggestions[exclude+1:]...)
	return subset
}

const (
	ReasonStage1Uniq                  = "stage1: uniq"
	ReasonStage1SameSourceParent      = "stage1: same source parent of uniq node"
	ReasonStage1SameSourceLeafChild   = "stage1: same source leaf child of uniq node"
	ReasonStage1SameSourceLeafSibling = "stage1: same source leaf sibling of uniq node"

	ReasonStage2SameSourceNodeOfPreservedParent          = "stage2: node on the same source as preserved parent"
	ReasonStage2SameSourceDuplicateNodeOfPreservedParent = "stage2: duplicate node on the same source as preserved parent"
	ReasonStage2SameSourceNodeOfPreservedChild           = "stage2: node on the same source as preserved child"
	ReasonStage2SameSourceNodeOfPreservedSibling         = "stage2: node on the same source as preserved sibling"

	PreserveReasonStage3ChooseAvailableNode = "stage3: choose first available node"
)

func preserveUniqNodes(nodes NodeSuggestions) []NodeSuggestion {
	for i := range nodes {
		if nodes[i].hasPriority {
			continue
		}

		isNodeUniq := nodes.IsNodeUniq(i)
		if !isNodeUniq {
			continue
		}

		// uniq nodes are always has priority
		nodes[i].SetPriorityWithReason(ReasonStage1Uniq)

		// if node parent of the uniq node is on the same source, prioritize it too
		parentIdx, ok := nodes.ParentNodeOnSameSource(i)
		if ok {
			nodes[parentIdx].SetPriorityWithReason(ReasonStage1SameSourceParent)
		}

		// if node has leaf childs on the same source, prioritize them too
		childs := nodes.ChildNodesOnSameSource(i)
		for _, child := range childs {
			if nodes.IsLeaf(child) && nodes.IsNodeUniq(child) {
				nodes[child].SetPriorityWithReason(ReasonStage1SameSourceLeafChild)
			}
		}

		// prioritize leaf siblings of the node on the same source
		siblings := nodes.SiblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if nodes.IsLeaf(sibling) && nodes.IsNodeUniq(sibling) {
				nodes[sibling].SetPriorityWithReason(ReasonStage1SameSourceLeafSibling)
			}
		}
	}
	return nodes
}

func preserveDuplicateNodes(nodes NodeSuggestions, secondRun bool) []NodeSuggestion {
	for i := range nodes {
		if nodes[i].hasPriority {
			continue
		}

		if nodes.HasPriorityOnOtherSource(i) {
			continue
		}

		// if node parent on the same source as the current node
		parentIdx, ok := nodes.ParentNodeOnSameSource(i)
		if ok && nodes[parentIdx].hasPriority {
			nodes[i].SetPriorityWithReason(ReasonStage2SameSourceNodeOfPreservedParent)
			continue
		}

		priorityIsSet := false

		// check if duplicates are on the same source as parent node
		nodeDuplicates := nodes.DuplicatesOf(i)
		for _, duplicate := range nodeDuplicates {
			parentIdx, ok := nodes.ParentNodeOnSameSource(duplicate)
			if ok && nodes[parentIdx].hasPriority {
				nodes[duplicate].SetPriorityWithReason(ReasonStage2SameSourceDuplicateNodeOfPreservedParent)
				priorityIsSet = true
				break
			}
		}
		if priorityIsSet {
			continue
		}

		childs := nodes.ChildNodesOnSameSource(i)
		for _, child := range childs {
			if nodes[child].hasPriority {
				nodes[i].SetPriorityWithReason(ReasonStage2SameSourceNodeOfPreservedChild)
				priorityIsSet = true
				break
			}
		}
		if priorityIsSet {
			continue
		}

		siblings := nodes.SiblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if nodes[sibling].hasPriority {
				nodes[i].SetPriorityWithReason(ReasonStage2SameSourceNodeOfPreservedSibling)
				priorityIsSet = true
				break
			}
		}
		if priorityIsSet {
			continue
		}

		if secondRun {
			nodes[i].SetPriorityWithReason(PreserveReasonStage3ChooseAvailableNode)
		}
	}
	return nodes
}

func nodesWithPriority(nodes NodeSuggestions) (out NodeSuggestions) {
	for i := range nodes {
		if nodes[i].hasPriority {
			out = append(out, nodes[i])
		}
	}
	return
}
