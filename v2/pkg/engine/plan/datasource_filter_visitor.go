package plan

import (
	"errors"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func FilterDataSources(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) []DataSourceConfiguration {
	return nil
}

type UsedNode struct {
	TypeName  string
	FieldName string
}

type UsedDataSourceConfiguration struct {
	DataSource DataSourceConfiguration
	UsedNodes  []*UsedNode
}

type dataSourceVisitor struct {
	operation   *ast.Document
	definition  *ast.Document
	walker      *astvisitor.Walker
	dataSources []*UsedDataSourceConfiguration
	err         error
}

func hasNode(f []TypeField, typeName, fieldName string) bool {
	for i := range f {
		if typeName != f[i].TypeName {
			continue
		}
		for j := range f[i].FieldNames {
			if fieldName == f[i].FieldNames[j] {
				return true
			}
		}
	}
	return false
}

func (v *dataSourceVisitor) EnterField(ref int) {
	if v.err != nil {
		return
	}
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.definition)
	fieldName := v.operation.FieldNameUnsafeString(ref)
	found := false
	for _, v := range v.dataSources {
		ds := v.DataSource
		if ds.HasRootNode(typeName, fieldName) || hasNode(ds.ChildNodes, typeName, fieldName) {
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
	return fmt.Sprintf("could not resolve %s.%s", e.TypeName, e.FieldName)
}

func planDataSources(operation *ast.Document, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) ([]*UsedDataSourceConfiguration, error) {
	if report == nil {
		panic("report can't be nil")
	}
	walker := astvisitor.NewWalker(32)
	dataSourcesToVisit := make([]*UsedDataSourceConfiguration, len(dataSources))
	for ii, v := range dataSources {
		v := v
		dataSourcesToVisit[ii] = &UsedDataSourceConfiguration{
			DataSource: v,
		}
	}
	visitor := &dataSourceVisitor{
		operation:   operation,
		definition:  definition,
		walker:      &walker,
		dataSources: dataSourcesToVisit,
	}
	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil, errors.New(report.Error())
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

func PlanDataSources(operation *ast.Document, definition *ast.Document, report *operationreport.Report, dataSources []DataSourceConfiguration) ([]*UsedDataSourceConfiguration, error) {
	if report == nil {
		report = &operationreport.Report{}
	}
	planned, err := planDataSources(operation, definition, report, dataSources)
	if err != nil {
		return nil, err
	}
	if len(planned) == 1 {
		return planned, nil
	}
	best := planned
	for excluded := range dataSources {
		subset := make([]DataSourceConfiguration, 0, len(dataSources)-1)
		for ii, ds := range dataSources {
			if ii != excluded {
				subset = append(subset, ds)
			}
		}
		result, err := PlanDataSources(operation, definition, report, subset)
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
