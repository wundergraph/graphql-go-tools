package service_datasource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestNewService(t *testing.T) {
	t.Run("with default error behavior", func(t *testing.T) {
		opts := ServiceOptions{
			DefaultErrorBehavior: "PROPAGATE",
		}
		service := NewService(opts)

		assert.Len(t, service.Capabilities, 2)

		// First capability should be onError support
		assert.Equal(t, "graphql.onError", service.Capabilities[0].Identifier)
		assert.NotNil(t, service.Capabilities[0].Description)

		// Second capability should be default error behavior
		assert.Equal(t, "graphql.defaultErrorBehavior", service.Capabilities[1].Identifier)
		assert.NotNil(t, service.Capabilities[1].Value)
		assert.Equal(t, "PROPAGATE", *service.Capabilities[1].Value)
	})

	t.Run("without default error behavior", func(t *testing.T) {
		opts := ServiceOptions{}
		service := NewService(opts)

		assert.Len(t, service.Capabilities, 1)
		assert.Equal(t, "graphql.onError", service.Capabilities[0].Identifier)
	})
}

func TestSource_Load(t *testing.T) {
	service := NewService(ServiceOptions{
		DefaultErrorBehavior: "NULL",
	})
	source := NewSource(service)

	data, err := source.Load(context.Background(), nil, []byte(`{}`))
	require.NoError(t, err)

	var result Service
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Len(t, result.Capabilities, 2)
	assert.Equal(t, "graphql.onError", result.Capabilities[0].Identifier)
	assert.Equal(t, "graphql.defaultErrorBehavior", result.Capabilities[1].Identifier)
	assert.Equal(t, "NULL", *result.Capabilities[1].Value)
}

func TestSource_LoadWithFiles(t *testing.T) {
	service := NewService(ServiceOptions{})
	source := NewSource(service)

	_, err := source.LoadWithFiles(context.Background(), nil, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support file uploads")
}

func TestServiceConfigFactory(t *testing.T) {
	factory := NewServiceConfigFactory(ServiceOptions{
		DefaultErrorBehavior: "HALT",
	})

	t.Run("field configurations", func(t *testing.T) {
		fieldConfigs := factory.BuildFieldConfigurations()
		assert.Len(t, fieldConfigs, 1)
		assert.Equal(t, "Query", fieldConfigs[0].TypeName)
		assert.Equal(t, "__service", fieldConfigs[0].FieldName)
	})

	t.Run("datasource configurations", func(t *testing.T) {
		dsConfigs := factory.BuildDataSourceConfigurations()
		assert.Len(t, dsConfigs, 1)
	})

	t.Run("service accessor", func(t *testing.T) {
		service := factory.Service()
		assert.NotNil(t, service)
		assert.Len(t, service.Capabilities, 2)
	})
}

func TestServiceConfigFactoryWithSchema_CustomQueryRoot(t *testing.T) {
	// Regression: ExtendSchemaWithServiceTypes adds __service to whatever
	// type the schema declares as the query root. The factory's field config
	// and datasource metadata used to hardcode "Query", which prevented the
	// planner from attaching the datasource when the schema used a custom
	// root like `schema { query: RootQuery }`.

	const customRootSDL = `schema { query: RootQuery }
type RootQuery {
	hello: String
}`

	doc, report := astparser.ParseGraphqlDocumentString(customRootSDL)
	require.False(t, report.HasErrors(), "parse error: %s", report.Error())
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&doc))

	factory, err := NewServiceConfigFactoryWithSchema(&doc, ServiceOptions{DefaultErrorBehavior: "PROPAGATE"})
	require.NoError(t, err)

	fieldConfigs := factory.BuildFieldConfigurations()
	assert.Equal(t, plan.FieldConfigurations{
		{TypeName: "RootQuery", FieldName: "__service"},
	}, fieldConfigs)

	dsConfigs := factory.BuildDataSourceConfigurations()
	require.Len(t, dsConfigs, 1)
	assert.True(t, dsConfigs[0].HasRootNode("RootQuery", "__service"),
		"datasource must register __service under the custom query root, not Query")
	assert.False(t, dsConfigs[0].HasRootNode("Query", "__service"),
		"datasource must NOT register against the default Query type when a custom root is configured")
}

func TestCapability_JSON(t *testing.T) {
	cap := Capability{
		Identifier:  "test.capability",
		Value:       ptr("test-value"),
		Description: ptr("A test capability"),
	}

	data, err := json.Marshal(cap)
	require.NoError(t, err)

	var result Capability
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "test.capability", result.Identifier)
	assert.NotNil(t, result.Value)
	assert.Equal(t, "test-value", *result.Value)
	assert.NotNil(t, result.Description)
	assert.Equal(t, "A test capability", *result.Description)
}

func TestCapability_JSON_WithNils(t *testing.T) {
	cap := Capability{
		Identifier: "test.capability",
	}

	data, err := json.Marshal(cap)
	require.NoError(t, err)

	// Verify that nil fields are omitted from JSON
	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	assert.Contains(t, raw, "identifier")
	assert.NotContains(t, raw, "value")
	assert.NotContains(t, raw, "description")
}
