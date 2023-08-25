package plan

import (
	"errors"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type DsMap map[DSHash]DataSourceConfiguration

func FilterDataSources(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) (used, unused DsMap, suggestions NodeSuggestions) {
	usedDataSources, err := findBestDataSourceSet(operation, definition, report, dataSources)
	if report.HasErrors() {
		return nil, nil, nil
	}
	if err != nil {
		report.AddInternalError(err)
		return nil, nil, nil
	}

	used = make(DsMap, len(usedDataSources))
	suggestions = make(NodeSuggestions, 0, len(usedDataSources))
	for _, ds := range usedDataSources {
		used[ds.DataSource.Hash()] = ds.DataSource
		for _, node := range ds.UsedNodes {
			suggestions = append(suggestions, NodeSuggestion{
				TypeName:       node.TypeName,
				FieldName:      node.FieldName,
				DataSourceHash: ds.DataSource.Hash(),
			})
		}
	}

	unused = make(DsMap, len(dataSources)-len(usedDataSources))
	for i := range dataSources {
		_, found := used[dataSources[i].Hash()]
		if !found {
			unused[dataSources[i].Hash()] = dataSources[i]
		}
	}

	return used, unused, suggestions
}

type NodeSuggestion struct {
	TypeName       string
	FieldName      string
	DataSourceHash DSHash
}

type NodeSuggestions []NodeSuggestion

func (f NodeSuggestions) HasSuggestion(typeName, fieldName string) (dsHash DSHash, ok bool) {
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

type UsedNode struct {
	TypeName  string
	FieldName string
}

type UsedDataSourceConfiguration struct {
	DataSource DataSourceConfiguration
	UsedNodes  []*UsedNode
}

type findUsedDataSourceVisitor struct {
	operation   *ast.Document
	definition  *ast.Document
	walker      *astvisitor.Walker
	dataSources []*UsedDataSourceConfiguration
	err         error
}

func (v *findUsedDataSourceVisitor) EnterField(ref int) {
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.definition)
	fieldName := v.operation.FieldNameUnsafeString(ref)
	found := false
	for _, v := range v.dataSources {
		ds := v.DataSource
		if ds.HasRootNode(typeName, fieldName) || ds.HasChildNode(typeName, fieldName) {
			v.UsedNodes = append(v.UsedNodes, &UsedNode{
				TypeName:  typeName,
				FieldName: fieldName,
			})
			found = true
			break
		}
	}

	if !found {
		v.err = &errOperationFieldNotResolved{TypeName: typeName, FieldName: fieldName}
	}
}

type errOperationFieldNotResolved struct {
	TypeName  string
	FieldName string
}

func (e *errOperationFieldNotResolved) Error() string {
	return fmt.Sprintf("could not find datasource to resolve %s.%s", e.TypeName, e.FieldName)
}

func findUsedDataSources(operation *ast.Document, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) ([]*UsedDataSourceConfiguration, error) {
	walker := astvisitor.NewWalker(32)
	dataSourcesToVisit := make([]*UsedDataSourceConfiguration, len(dataSources))
	for ii, v := range dataSources {
		v := v
		dataSourcesToVisit[ii] = &UsedDataSourceConfiguration{
			DataSource: v,
		}
	}
	visitor := &findUsedDataSourceVisitor{
		operation:   operation,
		definition:  definition,
		walker:      &walker,
		dataSources: dataSourcesToVisit,
	}
	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil, report
	}
	if visitor.err != nil {
		return nil, visitor.err
	}
	var usedDataSources []*UsedDataSourceConfiguration
	for _, v := range dataSourcesToVisit {
		if len(v.UsedNodes) > 0 {
			usedDataSources = append(usedDataSources, v)
		}
	}
	return usedDataSources, nil
}

func findBestDataSourceSet(operation *ast.Document, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) ([]*UsedDataSourceConfiguration, error) {
	if report == nil {
		report = &operationreport.Report{}
	}
	planned, err := findUsedDataSources(operation, definition, report, dataSources)
	if err != nil {
		return nil, err
	}
	if len(planned) == 1 {
		return planned, nil
	}
	best := planned
	for excluded := range dataSources {
		subset := dataSourcesSubset(dataSources, excluded)

		result, err := findBestDataSourceSet(operation, definition, report, subset)
		if err != nil {
			var rerr *errOperationFieldNotResolved
			if errors.As(err, &rerr) {
				// We removed a data source that causes the resolution to fail
				continue
			}
			return nil, err
		}
		if result != nil && len(result) < len(best) {
			best = result
		}
	}
	return best, nil
}

func dataSourcesSubset(dataSources []DataSourceConfiguration, exclude int) []DataSourceConfiguration {
	subset := make([]DataSourceConfiguration, 0, len(dataSources)-1)
	subset = append(subset, dataSources[:exclude]...)
	subset = append(subset, dataSources[exclude+1:]...)
	return subset
}
