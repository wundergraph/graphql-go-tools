package plan

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type DsMap map[DSHash]DataSourceConfiguration

func FilterDataSources(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) (used, unused DsMap, suggestions NodeSuggestions) {
	suggestions = findBestDataSourceSet(operation, definition, report, dataSources)
	if report.HasErrors() {
		return
	}

	dsInUse := suggestions.UniqueDataSourceHashes()

	used = make(DsMap, len(dsInUse)+3 /*3 for introspection*/)
	unused = make(DsMap, len(dataSources)-len(dsInUse))

	for i := range dataSources {
		// preserve introspection datasource
		if dataSources[i].IsIntrospection {
			used[dataSources[i].Hash()] = dataSources[i]
			continue
		}

		_, inUse := dsInUse[dataSources[i].Hash()]
		if inUse {
			used[dataSources[i].Hash()] = dataSources[i]
		} else {
			unused[dataSources[i].Hash()] = dataSources[i]
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

	preserve     bool
	whyWasChosen []string
}

func (n *NodeSuggestion) WhyWasChosen(reason string) {
	// fmt.Println("ds:", n.DataSourceHash, fmt.Sprintf("%s.%s", n.TypeName, n.FieldName), "reason:", reason)
	n.whyWasChosen = append(n.whyWasChosen, reason)
}

func (n *NodeSuggestion) PreserveWithReason(reason string) {
	if n.preserve {
		return
	}
	n.preserve = true
	// n.WhyWasChosen(reason)
}

type NodeSuggestions []NodeSuggestion

func (f NodeSuggestions) HasNode(typeName, fieldName string) (dsHash DSHash, ok bool) {
	if len(f) == 0 {
		return 0, false
	}

	for i := range f {
		if typeName == f[i].TypeName && fieldName == f[i].FieldName {
			return f[i].DataSourceHash, true
		}
	}
	return 0, false
}

func (f NodeSuggestions) IsNodeUniq(idx int) bool {
	for i := range f {
		if i == idx {
			continue
		}
		if f[idx].TypeName == f[i].TypeName && f[idx].FieldName == f[i].FieldName {
			return false
		}
	}
	return true
}

func (f NodeSuggestions) IsPreservedOnOtherSource(idx int) bool {
	for i := range f {
		if i == idx {
			continue
		}
		if f[idx].TypeName == f[i].TypeName && f[idx].FieldName == f[i].FieldName &&
			f[idx].DataSourceHash != f[i].DataSourceHash && f[i].preserve {
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
		if f[idx].TypeName == f[i].TypeName && f[idx].FieldName == f[i].FieldName {
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
	err   error
}

func (f *nodesResolvableVisitor) EnterField(ref int) {
	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)
	fieldName := f.operation.FieldNameUnsafeString(ref)

	_, found := f.nodes.HasNode(typeName, fieldName)

	if !found {
		f.walker.Stop()
		f.err = &errOperationFieldNotResolved{TypeName: typeName, FieldName: fieldName}
	}
}

func isResolvable(operation, definition *ast.Document, nodes []NodeSuggestion) bool {
	walker := astvisitor.NewWalker(32)
	visitor := &nodesResolvableVisitor{
		operation:  operation,
		definition: definition,
		walker:     &walker,
		nodes:      nodes,
	}
	walker.RegisterEnterFieldVisitor(visitor)
	report := &operationreport.Report{}
	walker.Walk(operation, definition, report)
	return visitor.err == nil
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
	parentPath := f.walker.Path.DotDelimitedString()
	currentPath := parentPath + "." + fieldName
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
}

func (e *errOperationFieldNotResolved) Error() string {
	return fmt.Sprintf("could not find datasource to resolve %s.%s", e.TypeName, e.FieldName)
}

func findBestNodes(operation, definition *ast.Document, nodes NodeSuggestions) NodeSuggestions {
	current := nodes
	currentDsCount := nodes.DataSourceCount()
	currentNodeCount := nodes.Count()
	for excluded := range nodes {
		if nodes[excluded].preserve {
			continue
		}

		subset := nodesSubset(nodes, excluded)
		if !isResolvable(operation, definition, subset) {
			continue
		}

		resultNodes := findBestNodes(operation, definition, subset)
		resultDsCount := resultNodes.DataSourceCount()
		resultNodeCount := resultNodes.Count()

		if resultNodeCount < currentNodeCount && resultDsCount <= currentDsCount {
			current = resultNodes
			currentDsCount = resultDsCount
			currentNodeCount = resultNodeCount
		}
	}

	return current
}

func findBestDataSourceSet(operation *ast.Document, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) NodeSuggestions {
	nodes := collectNodes(operation, definition, report, dataSources)
	if report.HasErrors() {
		return nil
	}

	nodes = preserveUniqNodes(nodes)
	nodes = preserveDuplicateNodes(nodes)
	nodes = findBestNodes(operation, definition, nodes)

	return nodes
}

func nodesSubset(suggestions []NodeSuggestion, exclude int) []NodeSuggestion {
	subset := make([]NodeSuggestion, 0, len(suggestions)-1)
	subset = append(subset, suggestions[:exclude]...)
	subset = append(subset, suggestions[exclude+1:]...)
	return subset
}

const (
	PreserveReasonStage1Uniq                  = "stage1: uniq"
	PreserveReasonStage1SameSourceParent      = "stage1: same source parent of uniq node"
	PreserveReasonStage1SameSourceLeafChild   = "stage1: same source leaf child of uniq node"
	PreserveReasonStage1SameSourceLeafSibling = "stage1: same source leaf sibling of uniq node"

	PreserveReasonStage2SameSourceNodeOfPreservedParent          = "stage2: node on the same source as preserved parent"
	PreserveReasonStage2SameSourceDuplicateNodeOfPreservedParent = "stage2: duplicate node on the same source as preserved parent"
	PreserveReasonStage2SameSourceNodeOfPreservedSibling         = "stage2: node on the same source as preserved sibling"
)

func preserveUniqNodes(nodes NodeSuggestions) []NodeSuggestion {
	for i := range nodes {
		if nodes[i].preserve {
			continue
		}

		isNodeUniq := nodes.IsNodeUniq(i)
		if !isNodeUniq {
			continue
		}

		// uniq nodes are always preserved
		nodes[i].PreserveWithReason(PreserveReasonStage1Uniq)

		// if node parent of the uniq node is on the same source, preserve it too
		parentIdx, ok := nodes.ParentNodeOnSameSource(i)
		if ok {
			nodes[parentIdx].PreserveWithReason(PreserveReasonStage1SameSourceParent)
		}

		// if node has leaf childs on the same source, preserve them too
		childs := nodes.ChildNodesOnSameSource(i)
		for _, child := range childs {
			if nodes.IsLeaf(child) && nodes.IsNodeUniq(child) {
				nodes[child].PreserveWithReason(PreserveReasonStage1SameSourceLeafChild)
			}
		}

		// preserve leaf siblings of the node on the same source
		siblings := nodes.SiblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if nodes.IsLeaf(sibling) && nodes.IsNodeUniq(sibling) {
				nodes[sibling].PreserveWithReason(PreserveReasonStage1SameSourceLeafSibling)
			}
		}
	}
	return nodes
}

func preserveDuplicateNodes(nodes NodeSuggestions) []NodeSuggestion {
	for i := range nodes {
		if nodes[i].preserve {
			continue
		}

		if nodes.IsPreservedOnOtherSource(i) {
			continue
		}

		// if node parent on the same source as the current node
		parentIdx, ok := nodes.ParentNodeOnSameSource(i)
		if ok && nodes[parentIdx].preserve {
			nodes[i].PreserveWithReason(PreserveReasonStage2SameSourceNodeOfPreservedParent)
			continue
		}

		// check if duplicates are on the same source as parent node
		nodeDuplicates := nodes.DuplicatesOf(i)
		priorityIsSet := false
		for _, duplicate := range nodeDuplicates {
			parentIdx, ok := nodes.ParentNodeOnSameSource(duplicate)
			if ok && nodes[parentIdx].preserve {
				nodes[duplicate].PreserveWithReason(PreserveReasonStage2SameSourceDuplicateNodeOfPreservedParent)
				priorityIsSet = true
				break
			}
		}
		if priorityIsSet {
			continue
		}

		siblings := nodes.SiblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if nodes[sibling].preserve {
				nodes[i].PreserveWithReason(PreserveReasonStage2SameSourceNodeOfPreservedSibling)
				break
			}
		}
	}
	return nodes
}
