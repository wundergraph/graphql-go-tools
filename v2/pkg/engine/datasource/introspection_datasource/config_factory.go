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
	}
}

func (f *IntrospectionConfigFactory) BuildDataSourceConfigurations() []plan.DataSource {
	root, _ := f.buildRootDataSourceConfiguration()

	return []plan.DataSource{
		root,
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
					FieldNames: []string{"description", "queryType", "mutationType", "subscriptionType", "types", "directives", "__typename"},
				},
				{
					TypeName:   "__Type",
					FieldNames: []string{"kind", "name", "description", "fields", "interfaces", "possibleTypes", "enumValues", "inputFields", "ofType", "specifiedByURL", "__typename"},
				},
				{
					TypeName:   "__Field",
					FieldNames: []string{"name", "description", "args", "type", "isDeprecated", "deprecationReason", "__typename"},
				},
				{
					TypeName:   "__InputValue",
					FieldNames: []string{"name", "description", "type", "defaultValue", "isDeprecated", "deprecationReason", "__typename"},
				},
				{
					TypeName:   "__Directive",
					FieldNames: []string{"name", "description", "locations", "args", "isRepeatable", "__typename"},
				},
				{
					TypeName:   "__EnumValue",
					FieldNames: []string{"name", "description", "isDeprecated", "deprecationReason", "__typename"},
				},
			},
		},
		Configuration{"Introspection: __schema __type"},
	)
}

func (f *IntrospectionConfigFactory) dataSourceConfigQueryTypeName() string {
	if len(f.introspectionData.Schema.QueryType.Name) == 0 {
		return "Query"
	}
	return f.introspectionData.Schema.QueryType.Name
}
