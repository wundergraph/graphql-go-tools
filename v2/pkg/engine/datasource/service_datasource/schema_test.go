package service_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
)

func TestServiceSDLIsValidGraphQL(t *testing.T) {
	// Test that ServiceSDL parses as valid GraphQL
	schema, report := astparser.ParseGraphqlDocumentString(ServiceSDL)
	require.False(t, report.HasErrors(), "ServiceSDL should be valid GraphQL: %s", report.Error())

	// Verify _Service type exists
	serviceNode, found := schema.Index.FirstNodeByNameStr("_Service")
	assert.True(t, found, "_Service type should exist")
	assert.Equal(t, ast.NodeKindObjectTypeDefinition, serviceNode.Kind)

	// Verify _Capability type exists
	capabilityNode, found := schema.Index.FirstNodeByNameStr("_Capability")
	assert.True(t, found, "_Capability type should exist")
	assert.Equal(t, ast.NodeKindObjectTypeDefinition, capabilityNode.Kind)
}

func TestExtendSchemaWithServiceTypes(t *testing.T) {
	t.Run("extends schema with service types", func(t *testing.T) {
		// Start with a simple user schema
		userSchemaSDL := `
			type Query {
				user(id: ID!): User
			}
			type User {
				id: ID!
				name: String!
			}
		`

		schema, report := astparser.ParseGraphqlDocumentString(userSchemaSDL)
		require.False(t, report.HasErrors())

		// Extend with service types
		err := ExtendSchemaWithServiceTypes(&schema)
		require.NoError(t, err)

		// Verify _Service type was added
		serviceNode, found := schema.Index.FirstNodeByNameStr("_Service")
		assert.True(t, found, "_Service type should exist after extension")
		assert.Equal(t, ast.NodeKindObjectTypeDefinition, serviceNode.Kind)

		// Verify _Capability type was added
		capabilityNode, found := schema.Index.FirstNodeByNameStr("_Capability")
		assert.True(t, found, "_Capability type should exist after extension")
		assert.Equal(t, ast.NodeKindObjectTypeDefinition, capabilityNode.Kind)

		// Verify __service field was added to Query
		queryNode, found := schema.Index.FirstNodeByNameStr("Query")
		require.True(t, found, "Query type should exist")
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("__service")),
			"Query should have __service field")

		// Verify original fields still exist
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("user")),
			"Query should still have user field")
	})

	t.Run("does not duplicate __service field if already exists", func(t *testing.T) {
		// Schema that already has __service field
		schemaSDL := `
			type Query {
				user: User
				__service: _Service!
			}
			type User {
				id: ID!
			}
			type _Service {
				capabilities: [_Capability!]!
			}
			type _Capability {
				identifier: String!
			}
		`

		schema, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors())

		// Get field count before
		queryNode, _ := schema.Index.FirstNodeByNameStr("Query")
		fieldCountBefore := len(schema.ObjectTypeDefinitions[queryNode.Ref].FieldsDefinition.Refs)

		// Extend with service types (should not duplicate)
		err := ExtendSchemaWithServiceTypes(&schema)
		require.NoError(t, err)

		// Field count should be the same (no duplicate __service)
		fieldCountAfter := len(schema.ObjectTypeDefinitions[queryNode.Ref].FieldsDefinition.Refs)
		assert.Equal(t, fieldCountBefore, fieldCountAfter, "should not duplicate __service field")
	})

	t.Run("returns error if Query type not found", func(t *testing.T) {
		// Schema without Query type
		schemaSDL := `
			type Mutation {
				createUser(name: String!): User
			}
			type User {
				id: ID!
			}
		`

		schema, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors())

		err := ExtendSchemaWithServiceTypes(&schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Query type not found")
	})

	t.Run("works with custom query type name", func(t *testing.T) {
		// Schema with custom query type name via schema definition
		schemaSDL := `
			schema {
				query: RootQuery
			}
			type RootQuery {
				user: User
			}
			type User {
				id: ID!
			}
		`

		schema, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors())

		err := ExtendSchemaWithServiceTypes(&schema)
		require.NoError(t, err)

		// Verify __service field was added to RootQuery
		queryNode, found := schema.Index.FirstNodeByNameStr("RootQuery")
		require.True(t, found, "RootQuery type should exist")
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("__service")),
			"RootQuery should have __service field")
	})
}

func TestExtendSchemaWithServiceTypes_CosmoRouterPattern(t *testing.T) {
	// This test mimics exactly how Cosmo router integrates:
	// 1. Start with a user schema (no service types)
	// 2. Parse it
	// 3. Merge with base schema (adds introspection types)
	// 4. Extend with service types
	// 5. Verify both introspection and service types exist

	t.Run("full integration pattern", func(t *testing.T) {
		// User's schema - does NOT include _Service, _Capability, or __service
		userSchemaSDL := `
			type Query {
				user(id: ID!): User
				users: [User!]!
			}
			type User {
				id: ID!
				name: String!
				email: String
			}
		`

		// 1. Parse user schema
		schema, report := astparser.ParseGraphqlDocumentString(userSchemaSDL)
		require.False(t, report.HasErrors())

		// 2. Merge with base schema (like Cosmo does - adds introspection types)
		err := asttransform.MergeDefinitionWithBaseSchema(&schema)
		require.NoError(t, err)

		// Verify introspection types were added by MergeDefinitionWithBaseSchema
		_, foundSchema := schema.Index.FirstNodeByNameStr("__Schema")
		assert.True(t, foundSchema, "__Schema type should exist after base schema merge")

		// 3. Extend with service types (NEW API)
		err = ExtendSchemaWithServiceTypes(&schema)
		require.NoError(t, err)

		// 4. Verify service types were added
		_, foundService := schema.Index.FirstNodeByNameStr("_Service")
		assert.True(t, foundService, "_Service type should exist")

		_, foundCapability := schema.Index.FirstNodeByNameStr("_Capability")
		assert.True(t, foundCapability, "_Capability type should exist")

		// 5. Verify __service field exists on Query
		queryNode, found := schema.Index.FirstNodeByNameStr("Query")
		require.True(t, found)
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("__service")),
			"Query should have __service field")

		// 6. Verify introspection fields still exist
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("__schema")),
			"Query should have __schema field")
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("__type")),
			"Query should have __type field")

		// 7. Verify original user fields still exist
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("user")),
			"Query should still have user field")
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("users")),
			"Query should still have users field")
	})
}

func TestNewServiceConfigFactoryWithSchema(t *testing.T) {
	t.Run("creates factory and extends schema", func(t *testing.T) {
		userSchemaSDL := `
			type Query {
				user: User
			}
			type User {
				id: ID!
			}
		`

		schema, report := astparser.ParseGraphqlDocumentString(userSchemaSDL)
		require.False(t, report.HasErrors())

		factory, err := NewServiceConfigFactoryWithSchema(&schema, ServiceOptions{
			DefaultErrorBehavior: "PROPAGATE",
		})
		require.NoError(t, err)
		require.NotNil(t, factory)

		// Verify schema was extended
		_, found := schema.Index.FirstNodeByNameStr("_Service")
		assert.True(t, found, "_Service type should exist")

		queryNode, _ := schema.Index.FirstNodeByNameStr("Query")
		assert.True(t, schema.ObjectTypeDefinitionHasField(queryNode.Ref, []byte("__service")))

		// Verify factory works
		fieldConfigs := factory.BuildFieldConfigurations()
		assert.Len(t, fieldConfigs, 1)
		assert.Equal(t, "__service", fieldConfigs[0].FieldName)

		dataSources := factory.BuildDataSourceConfigurations()
		assert.Len(t, dataSources, 1)
	})

	t.Run("returns error if schema extension fails", func(t *testing.T) {
		// Schema without Query type
		schemaSDL := `
			type Mutation {
				doSomething: Boolean
			}
		`

		schema, report := astparser.ParseGraphqlDocumentString(schemaSDL)
		require.False(t, report.HasErrors())

		factory, err := NewServiceConfigFactoryWithSchema(&schema, ServiceOptions{})
		assert.Error(t, err)
		assert.Nil(t, factory)
		assert.Contains(t, err.Error(), "Query type not found")
	})
}
