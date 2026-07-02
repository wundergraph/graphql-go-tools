package introspection_datasource

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type IntrospectionConfigFactory struct {
	introspectionData   *introspection.Data
	serviceCapabilities bool
}

// IntrospectionOptions configures optional introspection surfaces.
type IntrospectionOptions struct {
	// ServiceCapabilities advertises GraphQL Service Capabilities (onError,
	// graphql-spec#1163) via __schema { capabilities }. When false, introspection
	// is byte-identical to today.
	ServiceCapabilities bool
	// DefaultErrorBehavior is the operator-configured default error behavior
	// advertised as graphql.defaultErrorBehavior ("" => "PROPAGATE"). Only used
	// when ServiceCapabilities is true.
	DefaultErrorBehavior string
}

func NewIntrospectionConfigFactory(schema *ast.Document) (*IntrospectionConfigFactory, error) {
	return NewIntrospectionConfigFactoryWithOptions(schema, IntrospectionOptions{})
}

// NewIntrospectionConfigFactoryWithOptions builds the introspection data source
// config factory, optionally advertising Service Capabilities. When
// options.ServiceCapabilities is true, the schema passed in must have been merged
// with asttransform.MergeOptions{ServiceCapabilities: true} so that __Capability
// and __Schema.capabilities are defined.
func NewIntrospectionConfigFactoryWithOptions(schema *ast.Document, options IntrospectionOptions) (*IntrospectionConfigFactory, error) {
	var (
		data   introspection.Data
		report operationreport.Report
	)
	gen := introspection.NewGenerator()
	gen.Generate(schema, &report, &data)
	if report.HasErrors() {
		return nil, report
	}

	if options.ServiceCapabilities {
		data.Schema.Capabilities = introspection.BuildServiceCapabilities(true, options.DefaultErrorBehavior)
	}

	return &IntrospectionConfigFactory{
		introspectionData:   &data,
		serviceCapabilities: options.ServiceCapabilities,
	}, nil
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
	schemaFieldNames := []string{"description", "queryType", "mutationType", "subscriptionType", "types", "directives", "__typename"}
	childNodes := []plan.TypeField{
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
	}

	if f.serviceCapabilities {
		// __schema { capabilities } and the __Capability object fields must be
		// resolvable by this data source when the feature is enabled.
		schemaFieldNames = append(schemaFieldNames, "capabilities")
		childNodes = append(childNodes, plan.TypeField{
			TypeName:   "__Capability",
			FieldNames: []string{"identifier", "description", "value", "__typename"},
		})
	}

	childNodes = append([]plan.TypeField{{
		TypeName:   "__Schema",
		FieldNames: schemaFieldNames,
	}}, childNodes...)

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
			ChildNodes: childNodes,
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
