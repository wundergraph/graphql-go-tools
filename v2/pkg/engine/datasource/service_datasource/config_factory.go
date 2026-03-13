package service_datasource

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

const (
	// ServiceDataSourceID is the unique identifier for the service datasource.
	ServiceDataSourceID = "service_datasource"
)

// ServiceConfigFactory creates the datasource configuration for the __service field.
type ServiceConfigFactory struct {
	service *Service
}

// NewServiceConfigFactory creates a new ServiceConfigFactory with the given options.
func NewServiceConfigFactory(opts ServiceOptions) *ServiceConfigFactory {
	return &ServiceConfigFactory{
		service: NewService(opts),
	}
}

// NewServiceConfigFactoryWithSchema creates a factory that also extends
// the provided schema with service capability types (_Service, _Capability)
// and the __service field on the Query type.
//
// This is the recommended method for Cosmo router integration where the schema
// is built programmatically and needs to include service capability types.
//
// Usage:
//
//	// Parse user schema
//	schema, _ := astparser.ParseGraphqlDocumentString(userSchemaSDL)
//
//	// Merge with base schema (adds introspection types)
//	asttransform.MergeDefinitionWithBaseSchema(&schema)
//
//	// Extend with service types (adds _Service, _Capability, __service)
//	factory, err := service_datasource.NewServiceConfigFactoryWithSchema(&schema, opts)
//
//	// Add datasource configurations
//	planConfig.DataSources = append(planConfig.DataSources, factory.BuildDataSourceConfigurations()...)
//	planConfig.Fields = append(planConfig.Fields, factory.BuildFieldConfigurations()...)
func NewServiceConfigFactoryWithSchema(schema *ast.Document, opts ServiceOptions) (*ServiceConfigFactory, error) {
	// Extend schema with _Service, _Capability types and __service field
	if err := ExtendSchemaWithServiceTypes(schema); err != nil {
		return nil, fmt.Errorf("failed to extend schema with service types: %w", err)
	}

	return &ServiceConfigFactory{
		service: NewService(opts),
	}, nil
}

// BuildFieldConfigurations returns the field configurations for the __service field.
func (f *ServiceConfigFactory) BuildFieldConfigurations() plan.FieldConfigurations {
	return plan.FieldConfigurations{
		{
			TypeName:  "Query",
			FieldName: "__service",
		},
	}
}

// BuildDataSourceConfigurations returns the datasource configurations for the __service field.
func (f *ServiceConfigFactory) BuildDataSourceConfigurations() []plan.DataSource {
	ds, _ := f.buildDataSourceConfiguration()
	return []plan.DataSource{ds}
}

func (f *ServiceConfigFactory) buildDataSourceConfiguration() (plan.DataSourceConfiguration[Configuration], error) {
	return plan.NewDataSourceConfiguration[Configuration](
		ServiceDataSourceID,
		NewFactory[Configuration](f.service),
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"__service"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "_Service",
					FieldNames: []string{"capabilities", "__typename"},
				},
				{
					TypeName:   "_Capability",
					FieldNames: []string{"identifier", "value", "description", "__typename"},
				},
			},
		},
		Configuration{SourceType: "Service: __service"},
	)
}

// Service returns the underlying Service for testing purposes.
func (f *ServiceConfigFactory) Service() *Service {
	return f.service
}
