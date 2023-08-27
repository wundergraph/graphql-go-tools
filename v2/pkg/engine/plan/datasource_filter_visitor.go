package plan

import (
	"fmt"
	"strings"

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
	Preserve       bool
	IsRootNode     bool
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

func (f NodeSuggestions) IsNodeUniq(typeName, fieldName string, except int) bool {
	for i := range f {
		if i == except {
			continue
		}
		if typeName == f[i].TypeName && fieldName == f[i].FieldName {
			return false
		}
	}
	return true
}

func (f NodeSuggestions) hasPathPrefixFor(path string, dsHash DSHash, except int) bool {
	for i := range f {
		if i == except {
			continue
		}
		if strings.HasPrefix(f[i].Path, path) && f[i].DataSourceHash == dsHash {
			return true
		}
	}
	return false
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
	currentPath := f.walker.Path.DotDelimitedString() + "." + fieldName
	for _, v := range f.dataSources {
		if v.HasRootNode(typeName, fieldName) {
			f.nodes = append(f.nodes, NodeSuggestion{
				TypeName:       typeName,
				FieldName:      fieldName,
				DataSourceHash: v.Hash(),
				Path:           currentPath,
				IsRootNode:     true,
			})
		}
		if v.HasChildNode(typeName, fieldName) {
			f.nodes = append(f.nodes, NodeSuggestion{
				TypeName:       typeName,
				FieldName:      fieldName,
				DataSourceHash: v.Hash(),
				Path:           currentPath,
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
	nodes = setNodesPriority(nodes)
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
	for i := range nodes {
		if nodes.IsNodeUniq(nodes[i].TypeName, nodes[i].FieldName, i) {
			nodes[i].Preserve = true
		}
	}
	return nodes
}

func setNodesPriority(nodes NodeSuggestions) []NodeSuggestion {
	for i := range nodes {
		if nodes.hasPathPrefixFor(nodes[i].Path, nodes[i].DataSourceHash, i) {
			nodes[i].Preserve = true
		}
	}
	return nodes
}
