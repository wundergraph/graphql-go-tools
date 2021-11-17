package graphql

import (
	"encoding/json"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/introspection_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type IntrospectionConfigFactory struct {
	introspectionData *introspection.Data
}

func NewIntrospectionConfigFactory(schema *Schema) (*IntrospectionConfigFactory, error) {
	var (
		data   introspection.Data
		report operationreport.Report
	)
	gen := introspection.NewGenerator()
	gen.Generate(&schema.document, &report, &data)
	if report.HasErrors() {
		return nil, report
	}

	return &IntrospectionConfigFactory{introspectionData: &data}, nil
}

func (f *IntrospectionConfigFactory) engineConfigFieldConfigs() (planFields plan.FieldConfigurations) {
	return plan.FieldConfigurations{
		{
			TypeName:              "Query",
			FieldName:             "__schema",
			DisableDefaultMapping: true,
		},
		{
			TypeName:              "Query",
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

func (f *IntrospectionConfigFactory) engineConfigDataSource() plan.DataSourceConfiguration {
	return plan.DataSourceConfiguration{
		RootNodes: []plan.TypeField{
			{
				TypeName:   "Query",
				FieldNames: []string{"__schema", "__type"},
			},
			{
				TypeName:   "__Type",
				FieldNames: []string{"fields", "enumValues"},
			},
		},
		Factory: introspection_datasource.NewFactory(f.introspectionData),
		Custom:  json.RawMessage{},
	}
}
