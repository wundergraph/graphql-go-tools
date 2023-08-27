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

func (f NodeSuggestions) DataSourceCount() int {
	return len(f.UniqueDataSourceHashes())
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

	rootNodes  NodeSuggestions
	childNodes NodeSuggestions
	err        error
}

func (f *nodesResolvableVisitor) EnterField(ref int) {
	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)
	fieldName := f.operation.FieldNameUnsafeString(ref)

	_, found := f.rootNodes.HasNode(typeName, fieldName)
	if !found {
		_, found = f.childNodes.HasNode(typeName, fieldName)
	}

	if !found {
		f.walker.Stop()
		f.err = &errOperationFieldNotResolved{TypeName: typeName, FieldName: fieldName}
	}
}

func isResolvable(operation, definition *ast.Document, rootNodes, childNodes []NodeSuggestion) bool {
	walker := astvisitor.NewWalker(32)
	visitor := &nodesResolvableVisitor{
		operation:  operation,
		definition: definition,
		walker:     &walker,
		rootNodes:  rootNodes,
		childNodes: childNodes,
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

	rootNodes  []NodeSuggestion
	childNodes []NodeSuggestion
}

func (f *collectNodesVisitor) EnterDocument(_, _ *ast.Document) {
	f.rootNodes = make([]NodeSuggestion, 0)
	f.childNodes = make([]NodeSuggestion, 0)
}

func (f *collectNodesVisitor) EnterField(ref int) {
	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)
	fieldName := f.operation.FieldNameUnsafeString(ref)
	currentPath := f.walker.Path.DotDelimitedString() + "." + fieldName
	for _, v := range f.dataSources {
		if v.HasRootNode(typeName, fieldName) {
			f.rootNodes = append(f.rootNodes, NodeSuggestion{
				TypeName:       typeName,
				FieldName:      fieldName,
				DataSourceHash: v.Hash(),
				Path:           currentPath,
			})
		}
		if v.HasChildNode(typeName, fieldName) {
			f.childNodes = append(f.childNodes, NodeSuggestion{
				TypeName:       typeName,
				FieldName:      fieldName,
				DataSourceHash: v.Hash(),
				Path:           currentPath,
			})
		}
	}
}

func collectNodes(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) (rootNodes, childNodes []NodeSuggestion) {
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
	return visitor.rootNodes, visitor.childNodes
}

type errOperationFieldNotResolved struct {
	TypeName  string
	FieldName string
}

func (e *errOperationFieldNotResolved) Error() string {
	return fmt.Sprintf("could not find datasource to resolve %s.%s", e.TypeName, e.FieldName)
}

func findBestDataSourceSet(operation *ast.Document, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) ([]*UsedDataSourceConfiguration, error) {
	rootNodes, childNodes := collectNodes(operation, definition, report, dataSources)
	if report.HasErrors() {
		return nil, nil
	}

	filteredChildNodes := make([]NodeSuggestion, 0, len(childNodes))
	for excluded := range childNodes {
		subset := nodesSubset(childNodes, excluded)
		if !isResolvable(operation, definition, rootNodes, subset) {
			filteredChildNodes = append(filteredChildNodes, childNodes[excluded])
		}
	}

	filteredRootNodes := make([]NodeSuggestion, 0, len(rootNodes))
	for excluded, node := range rootNodes {
		n := fmt.Sprint(node.TypeName, node.FieldName)
		fmt.Println(n)
		subset := nodesSubset(rootNodes, excluded)
		if isResolvable(operation, definition, subset, filteredChildNodes) {
			path := rootNodes[excluded].Path
			dsHash := rootNodes[excluded].DataSourceHash

			preserve := false
			for i := range filteredChildNodes {
				if strings.HasPrefix(filteredChildNodes[i].Path, path) && filteredChildNodes[i].DataSourceHash == dsHash {
					preserve = true
					break
				}
			}
			if preserve {
				filteredRootNodes = append(filteredRootNodes, rootNodes[excluded])
			}
		} else {
			filteredRootNodes = append(filteredRootNodes, rootNodes[excluded])
		}
	}

	nodes := make(NodeSuggestions, 0, len(filteredRootNodes)+len(filteredChildNodes))
	nodes = append(nodes, filteredRootNodes...)
	nodes = append(nodes, filteredChildNodes...)

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
