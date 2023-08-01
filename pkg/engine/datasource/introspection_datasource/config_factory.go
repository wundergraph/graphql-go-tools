package introspection_datasource

import (
	"encoding/json"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/pkg/introspection"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type IntrospectionConfigFactory struct {
	introspectionData *introspection.Data
}

func NewIntrospectionConfigFactory(schema *ast.Document) (*IntrospectionConfigFactory, error) {
	var (
		data   introspection.Data
		report operationreport.Report
	)
	gen := introspection.NewGenerator()
	gen.Generate(schema, &report, &data)
	if report.HasErrors() {
		return nil, report
	}

	return &IntrospectionConfigFactory{introspectionData: &data}, nil
}

func (f *IntrospectionConfigFactory) BuildFieldConfigurations() (planFields plan.FieldConfigurations) {
	return plan.FieldConfigurations{
		{
			TypeName:              f.introspectionData.Schema.QueryType.Name,
			FieldName:             "__schema",
			DisableDefaultMapping: true,
		},
		{
			TypeName:              f.introspectionData.Schema.QueryType.Name,
			FieldName:             "__type",
			DisableDefaultMapping: true,
		},
		{
			TypeName:              "__Type",
			FieldName:             "fields",
			DisableDefaultMapping: true,
		},
		{
			TypeName:              "__Type",
			FieldName:             "enumValues",
			DisableDefaultMapping: true,
		},
	}
}

func (f *IntrospectionConfigFactory) BuildDataSourceConfiguration() plan.DataSourceConfiguration {
	return plan.DataSourceConfiguration{
		RootNodes: []plan.TypeField{
			{
				TypeName:   f.dataSourceConfigQueryTypeName(),
				FieldNames: []string{"__schema", "__type"},
			},
			{
				TypeName:   "__Type",
				FieldNames: []string{"fields", "enumValues"},
			},
		},
		Factory: NewFactory(f.introspectionData),
		Custom:  json.RawMessage{},
	}
}

func (f *IntrospectionConfigFactory) dataSourceConfigQueryTypeName() string {
	if f.introspectionData.Schema.QueryType == nil || len(f.introspectionData.Schema.QueryType.Name) == 0 {
		return "Query"
	}
	return f.introspectionData.Schema.QueryType.Name
}
