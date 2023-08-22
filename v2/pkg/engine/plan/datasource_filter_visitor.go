package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func FilterDataSources(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) []DataSourceConfiguration {
	walker := astvisitor.NewWalker(48)

	ds := make([]*DataSourceConfiguration, 0, len(dataSources))
	for _, source := range dataSources {
		s := source
		ds = append(ds, &s)
	}

	visitor := &UsedDatasourcesVisitor{
		walker:               &walker,
		dataSources:          ds,
		operation:            operation,
		definition:           definition,
		filteredDataSourcess: make(map[*DataSourceConfiguration]struct{}),
	}
	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil
	}

	return visitor.Results()
}

type UsedDatasourcesVisitor struct {
	walker                *astvisitor.Walker
	operation, definition *ast.Document
	dataSources           []*DataSourceConfiguration
	filteredDataSourcess  map[*DataSourceConfiguration]struct{}
}

func (u *UsedDatasourcesVisitor) Results() []DataSourceConfiguration {
	out := make([]DataSourceConfiguration, 0, len(u.filteredDataSourcess))
	for ds := range u.filteredDataSourcess {
		out = append(out, *ds)
	}
	return out
}

func (u *UsedDatasourcesVisitor) EnterField(ref int) {
	fieldName := u.operation.FieldNameUnsafeString(ref)
	typeName := u.walker.EnclosingTypeDefinition.NameString(u.definition)

	for _, config := range u.dataSources {
		if config.HasRootNode(typeName, fieldName) || config.HasChildNode(typeName, fieldName) {
			u.filteredDataSourcess[config] = struct{}{}
		}
	}
}
