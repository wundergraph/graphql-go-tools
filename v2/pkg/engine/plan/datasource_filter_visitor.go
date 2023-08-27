package plan

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type DsMap map[DSHash]DataSourceConfiguration

func FilterDataSources(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) (used, unused DsMap, suggestions NodeSuggestions) {
	usedDataSources := make([]*UsedDataSourceConfiguration, 0, len(dataSources))
	// usedDataSources, err := findBestDataSourceSet(operation, definition, report, dataSources, true)
	// if report.HasErrors() {
	// 	return nil, nil, nil
	// }
	// if err != nil {
	// 	report.AddInternalError(err)
	// 	return nil, nil, nil
	// }

	used = make(DsMap, len(usedDataSources))
	suggestions = make(NodeSuggestions, 0, len(usedDataSources))
	// for _, ds := range usedDataSources {
	// 	used[ds.DataSource.Hash()] = ds.DataSource
	// 	for _, node := range ds.UsedNodes {
	// 		suggestions = append(suggestions, NodeSuggestion{
	// 			TypeName:       node.TypeName,
	// 			FieldName:      node.FieldName,
	// 			DataSourceHash: ds.DataSource.Hash(),
	// 		})
	// 	}
	// }

	unused = make(DsMap, len(dataSources)-len(usedDataSources))
	for i := range dataSources {
		_, found := used[dataSources[i].Hash()]
		if found {
			continue
		}

		// preserve introspection datasource
		if dataSources[i].IsIntrospection {
			used[dataSources[i].Hash()] = dataSources[i]
			continue
		}

		unused[dataSources[i].Hash()] = dataSources[i]

	}

	return used, unused, suggestions
}

type NodeSuggestion struct {
	TypeName       string
	FieldName      string
	DataSourceHash DSHash
	Path           string
	ParentPath     string

	Preserve   bool
	IsRootNode bool

	whyWasChosen []string
}

func (n *NodeSuggestion) WhyWasChosen(reason string) {
	n.whyWasChosen = append(n.whyWasChosen, reason)
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

func (f NodeSuggestions) IsNotPreservedOnOtherSource(idx int) bool {
	for i := range f {
		if i == idx {
			continue
		}
		if f[idx].TypeName == f[i].TypeName && f[idx].FieldName == f[i].FieldName &&
			f[i].DataSourceHash != f[idx].DataSourceHash && f[i].Preserve {
			return false
		}
	}
	return true
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

type UsedNode struct {
	TypeName  string
	FieldName string
}

type UsedDataSourceConfiguration struct {
	DataSource DataSourceConfiguration
	UsedNodes  []*UsedNode
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
		// path := rootNodes[excluded].Path
		// dsHash := rootNodes[excluded].DataSourceHash
		//
		// shouldPreserve := hasPrefixFor(childNodes, path, dsHash)
		// if shouldPreserve {
		// 	continue
		// }

		if nodes[excluded].Preserve {
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

func findBestDataSourceSet(operation *ast.Document, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) ([]*UsedDataSourceConfiguration, error) {
	nodes := collectNodes(operation, definition, report, dataSources)
	if report.HasErrors() {
		return nil, nil
	}

	nodes = preserveMandatoryNodes(nodes)
	nodes = findBestNodes(operation, definition, nodes)

	used := make([]*UsedDataSourceConfiguration, 0, len(dataSources))
	for hash := range nodes.UniqueDataSourceHashes() {
		var ds DataSourceConfiguration
		for i := range dataSources {
			if dataSources[i].Hash() == hash {
				ds = dataSources[i]
				break
			}
		}

		usedNodes := make([]*UsedNode, 0, len(nodes))
		for i := range nodes {
			if nodes[i].DataSourceHash == hash {
				usedNodes = append(usedNodes, &UsedNode{
					TypeName:  nodes[i].TypeName,
					FieldName: nodes[i].FieldName,
				})
			}
		}

		usedDs := &UsedDataSourceConfiguration{
			DataSource: ds,
			UsedNodes:  usedNodes,
		}

		used = append(used, usedDs)
	}

	return used, nil
}

func nodesSubset(suggestions []NodeSuggestion, exclude int) []NodeSuggestion {
	subset := make([]NodeSuggestion, 0, len(suggestions)-1)
	subset = append(subset, suggestions[:exclude]...)
	subset = append(subset, suggestions[exclude+1:]...)
	return subset
}

func preserveMandatoryNodes(nodes NodeSuggestions) []NodeSuggestion {
	for i, n := range nodes {
		_ = n
		if nodes[i].Preserve {
			continue
		}

		isNodeUniq := nodes.IsNodeUniq(i)
		if !isNodeUniq {
			continue
		}

		// uniq nodes are always preserved
		nodes[i].Preserve = true
		nodes[i].WhyWasChosen("uniq")

		// if node parent of the uniq node is on the same source, preserve it too
		parentIdx, ok := nodes.ParentNodeOnSameSource(i)
		if ok {
			nodes[parentIdx].Preserve = true
			nodes[parentIdx].WhyWasChosen("same source parent of uniq node")
		}

		// if node has leaf childs on the same source, preserve them too
		childs := nodes.ChildNodesOnSameSource(i)
		for _, child := range childs {
			if nodes.IsLeaf(child) {
				if nodes.IsNodeUniq(child) || nodes.IsNotPreservedOnOtherSource(child) {
					nodes[child].Preserve = true
					nodes[child].WhyWasChosen("same source leaf child of uniq node")
				}
			}
		}

		// preserve leaf siblings of the node on the same source
		siblings := nodes.SiblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if nodes.IsLeaf(sibling) {
				if nodes.IsNodeUniq(sibling) || nodes.IsNotPreservedOnOtherSource(sibling) {
					nodes[sibling].Preserve = true
					nodes[sibling].WhyWasChosen("same source leaf sibling of uniq node")
				}
			}
		}

	}
	return nodes
}
