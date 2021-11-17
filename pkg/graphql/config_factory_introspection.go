package graphql

import (
	"encoding/json"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/introspection_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type IntrospectionConfigFactory struct {
	schema *Schema
}

func NewIntrospectionConfigFactory(schema *Schema) *IntrospectionConfigFactory {
	return &IntrospectionConfigFactory{schema: schema}
}

func (f *IntrospectionConfigFactory) introspection() (data introspection.Data) {
	var report operationreport.Report
	gen := introspection.NewGenerator()
	gen.Generate(&f.schema.document, &report, &data)
	// if report.HasErrors() {
	// 	return introspection.Data{}, report
	// }

	return
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
			// Arguments: plan.ArgumentsConfigurations{
			// 	{
			// 		Name:       "name",
			// 		SourceType: plan.FieldArgumentSource,
			// 	},
			// },
		},
		{
			TypeName:              "__Type",
			FieldName:             "fields",
			DisableDefaultMapping: true,
			// Arguments: plan.ArgumentsConfigurations{
			// 	{
			// 		Name:       "includeDeprecated",
			// 		SourceType: plan.FieldArgumentSource,
			// 	},
			// 	{
			// 		Name:       "typeName",
			// 		SourceType: plan.ObjectFieldSource,
			// 		SourcePath: []string{"name"},
			// 	},
			// },
		},
		{
			TypeName:              "__Type",
			FieldName:             "enumValues",
			DisableDefaultMapping: true,
			// Arguments: plan.ArgumentsConfigurations{
			// 	{
			// 		Name:       "includeDeprecated",
			// 		SourceType: plan.FieldArgumentSource,
			// 	},
			// 	{
			// 		Name:       "typeName",
			// 		SourceType: plan.ObjectFieldSource,
			// 		SourcePath: []string{"name"},
			// 	},
			// },
		},
	}
}

func (f *IntrospectionConfigFactory) engineConfigDataSources() (planDataSources []plan.DataSourceConfiguration) {
	data := f.introspection()

	introspectionDataSource := plan.DataSourceConfiguration{
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
		Factory: introspection_datasource.NewFactory(&data),
		Custom:  json.RawMessage{},
	}

	planDataSources = append(planDataSources, introspectionDataSource)

	return
}
