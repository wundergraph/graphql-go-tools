package introspection_datasource

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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
			TypeName:  f.dataSourceConfigQueryTypeName(),
			FieldName: "__schema",
		},
		{
			TypeName:  f.dataSourceConfigQueryTypeName(),
			FieldName: "__type",
			Arguments: plan.ArgumentsConfigurations{
				{
					Name:       "name",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "__Type",
			FieldName: "fields",
			Arguments: plan.ArgumentsConfigurations{
				{
					Name:       "includeDeprecated",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "__Type",
			FieldName: "enumValues",
			Arguments: plan.ArgumentsConfigurations{
				{
					Name:       "includeDeprecated",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
	}
}

func (f *IntrospectionConfigFactory) BuildDataSourceConfigurations() []plan.DataSource {
	root, _ := f.buildRootDataSourceConfiguration()
	fields, _ := f.buildFieldsConfiguration()
	enums, _ := f.buildEnumsConfiguration()

	return []plan.DataSource{
		root,
		fields,
		enums,
	}
}

func (f *IntrospectionConfigFactory) buildRootDataSourceConfiguration() (plan.DataSourceConfiguration[Configuration], error) {
	return plan.NewDataSourceConfiguration[Configuration](
		resolve.IntrospectionSchemaTypeDataSourceID,
		NewFactory[Configuration](f.introspectionData),
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   f.dataSourceConfigQueryTypeName(),
					FieldNames: []string{"__schema", "__type"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "__Schema",
					FieldNames: []string{"queryType", "mutationType", "subscriptionType", "types", "directives"},
				},
				{
					TypeName:   "__Type",
					FieldNames: []string{"kind", "name", "description", "interfaces", "possibleTypes", "inputFields", "ofType"},
				},
				{
					TypeName:   "__Field",
					FieldNames: []string{"name", "description", "args", "type", "isDeprecated", "deprecationReason"},
				},
				{
					TypeName:   "__InputValue",
					FieldNames: []string{"name", "description", "type", "defaultValue"},
				},
				{
					TypeName:   "__Directive",
					FieldNames: []string{"name", "description", "locations", "args", "isRepeatable"},
				},
			},
		},
		Configuration{"Introspection: __schema __type"},
	)
}

func (f *IntrospectionConfigFactory) buildFieldsConfiguration() (plan.DataSourceConfiguration[Configuration], error) {
	return plan.NewDataSourceConfiguration[Configuration](
		resolve.IntrospectionTypeFieldsDataSourceID,
		NewFactory[Configuration](f.introspectionData),
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "__Type",
					FieldNames: []string{"fields"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "__Type",
					FieldNames: []string{"kind", "name", "description", "interfaces", "possibleTypes", "inputFields", "ofType"},
				},
				{
					TypeName:   "__Field",
					FieldNames: []string{"name", "description", "args", "type", "isDeprecated", "deprecationReason"},
				},
				{
					TypeName:   "__InputValue",
					FieldNames: []string{"name", "description", "type", "defaultValue"},
				},
			},
		},
		Configuration{"Introspection: __Type.fields"},
	)
}

func (f *IntrospectionConfigFactory) buildEnumsConfiguration() (plan.DataSourceConfiguration[Configuration], error) {
	return plan.NewDataSourceConfiguration[Configuration](
		resolve.IntrospectionTypeEnumValuesDataSourceID,
		NewFactory[Configuration](f.introspectionData),
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "__Type",
					FieldNames: []string{"enumValues"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "__EnumValue",
					FieldNames: []string{"name", "description", "isDeprecated", "deprecationReason"},
				},
			},
		},
		Configuration{"Introspection: __Type.enumValues"},
	)
}

func (f *IntrospectionConfigFactory) dataSourceConfigQueryTypeName() string {
	if f.introspectionData.Schema.QueryType == nil || len(f.introspectionData.Schema.QueryType.Name) == 0 {
		return "Query"
	}
	return f.introspectionData.Schema.QueryType.Name
}
