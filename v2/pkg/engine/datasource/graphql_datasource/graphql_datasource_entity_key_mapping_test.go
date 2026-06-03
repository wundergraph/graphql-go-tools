package graphql_datasource

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// planAndExtractCacheConfig runs the planner on the given schema/query/config and returns
// the FetchCacheConfiguration for each SingleFetch in the plan, keyed by fetch index.
func planAndExtractCacheConfig(t *testing.T, definition, operation, operationName string, config plan.Configuration) []resolve.FetchCacheConfiguration {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	require.NoError(t, err)
	norm := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	var report operationreport.Report
	norm.NormalizeOperation(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())

	valid := astvalidation.DefaultOperationValidator()
	valid.Validate(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())

	p, err := plan.NewPlanner(config)
	require.NoError(t, err)

	actualPlan := p.Plan(&op, &def, operationName, &report)
	require.False(t, report.HasErrors(), report.Error())

	processor := postprocess.NewProcessor(
		postprocess.DisableResolveInputTemplates(),
		postprocess.DisableCreateConcreteSingleFetchTypes(),
		postprocess.DisableCreateParallelNodes(),
		postprocess.DisableMergeFields(),
	)
	processor.Process(actualPlan)

	syncPlan, ok := actualPlan.(*plan.SynchronousResponsePlan)
	require.True(t, ok, "expected SynchronousResponsePlan")
	require.NotNil(t, syncPlan.Response)
	require.NotNil(t, syncPlan.Response.Fetches)

	var configs []resolve.FetchCacheConfiguration
	collectCacheConfigs(syncPlan.Response.Fetches, &configs)
	return configs
}

func collectCacheConfigs(node *resolve.FetchTreeNode, out *[]resolve.FetchCacheConfiguration) {
	if node == nil {
		return
	}
	if node.Item != nil && node.Item.Fetch != nil {
		if sf, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
			*out = append(*out, sf.FetchConfiguration.Caching)
		}
	}
	if node.Trigger != nil {
		collectCacheConfigs(node.Trigger, out)
	}
	for _, child := range node.ChildNodes {
		collectCacheConfigs(child, out)
	}
}

func newExpectedRootQueryCacheKeyTemplate(rootFields []resolve.QueryField, entityKeyMappings []resolve.EntityKeyMappingConfig) *resolve.RootQueryCacheKeyTemplate {
	return resolve.NewRootQueryCacheKeyTemplate(rootFields, entityKeyMappings)
}

// newEntityKeyMappingTestConfig creates a plan.Configuration for entity key mapping tests
// with a single "accounts" subgraph that has a User entity.
func newEntityKeyMappingTestConfig(t *testing.T, rootFieldCaching plan.RootFieldCacheConfigurations, entityCaching plan.EntityCacheConfigurations, sdl string, keys plan.FederationFieldConfigurations) plan.Configuration {
	t.Helper()

	ds := mustDataSourceConfiguration(t,
		"accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"user", "userByIdAndName"}},
				{TypeName: "User", FieldNames: []string{"id", "username"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys:             keys,
				RootFieldCaching: rootFieldCaching,
				EntityCaching:    entityCaching,
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://accounts.service"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: sdl},
					sdl,
				),
			},
		),
	)

	return plan.Configuration{
		DataSources:                     []plan.DataSource{ds},
		DisableIncludeInfo:              false,
		DisableIncludeFieldDependencies: false,
		DisableEntityCaching:            false,
		DisableFetchProvidesData:        false,
		Fields: plan.FieldConfigurations{
			{TypeName: "Query", FieldName: "user", Arguments: plan.ArgumentsConfigurations{
				{Name: "id", SourceType: plan.FieldArgumentSource, SourcePath: []string{"id"}},
			}},
			{TypeName: "Query", FieldName: "userByIdAndName", Arguments: plan.ArgumentsConfigurations{
				{Name: "id", SourceType: plan.FieldArgumentSource, SourcePath: []string{"id"}},
				{Name: "username", SourceType: plan.FieldArgumentSource, SourcePath: []string{"username"}},
			}},
		},
	}
}

func TestEntityKeyMappingPlanning(t *testing.T) {
	definition := `
		type User {
			id: ID!
			username: String!
		}
		type Query {
			user(id: ID!): User
			userByIdAndName(id: ID!, username: String!): User
		}
	`

	sdl := `
		type Query {
			user(id: ID!): User
			userByIdAndName(id: ID!, username: String!): User
		}
		type User @key(fields: "id") {
			id: ID!
			username: String!
		}
	`

	keys := plan.FederationFieldConfigurations{
		{TypeName: "User", SelectionSet: "id"},
	}

	t.Run("simple scalar key", func(t *testing.T) {
		// Root field user(id) with single EntityKeyMapping for @key(fields: "id")
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "user",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdl, keys)
		cacheConfigs := planAndExtractCacheConfig(t, definition, `query Q($id: ID!) { user(id: $id) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs), "should have 1 fetch")
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"user:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"user"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("composite scalar keys", func(t *testing.T) {
		// Root field userByIdAndName(id, username) with single EntityKeyMapping
		// that has 2 FieldMappings (composite key: id + username)
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "userByIdAndName",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							{EntityKeyField: "username", ArgumentPath: []string{"username"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdl, keys)
		cacheConfigs := planAndExtractCacheConfig(t, definition, `query Q($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs), "should have 1 fetch")
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "userByIdAndName"},
					ResponseKey: "userByIdAndName",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
						{Name: "username", Variable: &resolve.ContextVariable{Path: []string{"username"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						{EntityKeyField: "username", ArgumentPath: []string{"username"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"userByIdAndName:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"userByIdAndName"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("cross-lookup setup", func(t *testing.T) {
		// Both root field entity key mapping AND entity caching for same type
		// Verifies the planner produces both templates for cross-lookup
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "user",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
				},
			},
		}
		entityCaching := plan.EntityCacheConfigurations{
			{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, entityCaching, sdl, keys)
		cacheConfigs := planAndExtractCacheConfig(t, definition, `query Q($id: ID!) { user(id: $id) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs), "should have 1 fetch (root field only, no entity fetch for same subgraph)")
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"user:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"user"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("with header prefix", func(t *testing.T) {
		// Same as simple scalar key but with IncludeSubgraphHeaderPrefix
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:                    "Query",
				FieldName:                   "user",
				CacheName:                   "default",
				TTL:                         30 * time.Second,
				IncludeSubgraphHeaderPrefix: true,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdl, keys)
		cacheConfigs := planAndExtractCacheConfig(t, definition, `query Q($id: ID!) { user(id: $id) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs))
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:                     true,
			CacheName:                   "default",
			TTL:                         30 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"user:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"user"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("without entity key mapping regression", func(t *testing.T) {
		// Root field caching WITHOUT EntityKeyMappings → should use root field format
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "user",
				CacheName: "default",
				TTL:       30 * time.Second,
				// No EntityKeyMappings
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdl, keys)
		cacheConfigs := planAndExtractCacheConfig(t, definition, `query Q($id: ID!) { user(id: $id) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs))
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"user:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"user"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("caching globally disabled", func(t *testing.T) {
		// DisableEntityCaching: true → CacheKeyTemplate preserved for L1 but Enabled: false
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "user",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdl, keys)
		config.DisableEntityCaching = true
		cacheConfigs := planAndExtractCacheConfig(t, definition, `query Q($id: ID!) { user(id: $id) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs))
		assert.Equal(t, resolve.FetchCacheConfiguration{
			// When entity caching is globally disabled, Enabled is false but CacheKeyTemplate
			// is preserved for L1 cache (which is controlled separately)
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"user:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"user"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("multiple keys single mapping", func(t *testing.T) {
		// Entity with @key(fields: "id") @key(fields: "username"), but root field user(id)
		// maps only to the "id" key. The config only has 1 EntityKeyMapping.
		sdlMultiKey := `
			type Query {
				user(id: ID!): User
				userByIdAndName(id: ID!, username: String!): User
			}
			type User @key(fields: "id") @key(fields: "username") {
				id: ID!
				username: String!
			}
		`
		keysMulti := plan.FederationFieldConfigurations{
			{TypeName: "User", SelectionSet: "id"},
			{TypeName: "User", SelectionSet: "username"},
		}

		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "user",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdlMultiKey, keysMulti)
		cacheConfigs := planAndExtractCacheConfig(t, definition, `query Q($id: ID!) { user(id: $id) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs))
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"user:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"user"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("username"),
								Value:       &resolve.String{Path: []string{"username"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("multiple keys multiple mappings", func(t *testing.T) {
		// Entity with @key(fields: "id") @key(fields: "username"),
		// root field userByIdAndName(id, username) maps to BOTH keys.
		// Config has 2 EntityKeyMappings.
		sdlMultiKey := `
			type Query {
				user(id: ID!): User
				userByIdAndName(id: ID!, username: String!): User
			}
			type User @key(fields: "id") @key(fields: "username") {
				id: ID!
				username: String!
			}
		`
		keysMulti := plan.FederationFieldConfigurations{
			{TypeName: "User", SelectionSet: "id"},
			{TypeName: "User", SelectionSet: "username"},
		}

		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "userByIdAndName",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "username", ArgumentPath: []string{"username"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdlMultiKey, keysMulti)
		cacheConfigs := planAndExtractCacheConfig(t, definition, `query Q($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs))
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "userByIdAndName"},
					ResponseKey: "userByIdAndName",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
						{Name: "username", Variable: &resolve.ContextVariable{Path: []string{"username"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "username", ArgumentPath: []string{"username"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"userByIdAndName:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"userByIdAndName"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("username"),
								Value:       &resolve.String{Path: []string{"username"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("aliased root fields get separate cache tracking", func(t *testing.T) {
		// When query has `a: user(id: $id1) { ... } b: user(id: $id2) { ... }`,
		// each aliased root field produces a separate fetch with its own RootFields entry and Args.
		// The planner creates separate fetches because the aliases have different variables.
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "user",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdl, keys)
		cacheConfigs := planAndExtractCacheConfig(t, definition,
			`query Q($id1: ID!, $id2: ID!) { a: user(id: $id1) { id username } b: user(id: $id2) { id username } }`, "Q", config)

		// Each alias gets its own fetch because they have different variables,
		// so the planner creates 2 separate fetches with 1 root field entry each.
		require.Equal(t, 2, len(cacheConfigs), "should have 2 fetches (one per alias)")

		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "a", // aliased as `a: user(...)`
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id1"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id1"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"a:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"a"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])

		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "b", // aliased as `b: user(...)`
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id2"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id2"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"b:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"b"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[1])
	})

	t.Run("aliased root fields use alias in entity cache key path", func(t *testing.T) {
		// When a query uses aliases like `a: user(id: $id1) { ... }`, the
		// RootFieldL1EntityCacheKeyTemplates must use the alias ("a") as the
		// response path, not the schema field name ("user"). The response JSON
		// is keyed by alias, so the template path must match.
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "user",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdl, keys)
		cacheConfigs := planAndExtractCacheConfig(t, definition,
			`query Q($id: ID!) { myUser: user(id: $id) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs), "should have 1 fetch")

		// The entity cache key template path must use the alias "myUser", not "user"
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "myUser", // aliased as `myUser: user(...)`
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"myUser:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"myUser"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})

	t.Run("multi-arg root field keeps args together", func(t *testing.T) {
		// Regression: a root field with multiple arguments (e.g., userByIdAndName(id, username))
		// must produce exactly 1 RootFields entry with both args, not split them into separate entries.
		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "userByIdAndName",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							{EntityKeyField: "username", ArgumentPath: []string{"username"}},
						},
					},
				},
			},
		}

		config := newEntityKeyMappingTestConfig(t, rootFieldCaching, nil, sdl, keys)
		cacheConfigs := planAndExtractCacheConfig(t, definition,
			`query Q($id: ID!, $username: String!) { userByIdAndName(id: $id, username: $username) { id username } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs), "should have 1 fetch")
		cc := cacheConfigs[0]

		// Exactly 1 root field entry (not split by args)
		require.Equal(t, 1, len(cc.CacheKeyTemplate.(*resolve.RootQueryCacheKeyTemplate).RootFields),
			"multi-arg field must produce exactly 1 RootFields entry, not split by args")

		// The entry has both args
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "userByIdAndName"},
					ResponseKey: "userByIdAndName",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
						{Name: "username", Variable: &resolve.ContextVariable{Path: []string{"username"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						{EntityKeyField: "username", ArgumentPath: []string{"username"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"userByIdAndName:User": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "User",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"userByIdAndName"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					}),
				},
			},
		}, cc)
	})

	t.Run("nested object key", func(t *testing.T) {
		// Entity with @key(fields: "id info {a b}"), root field provides
		// arguments that map to the nested key structure
		definitionNested := `
			type Info {
				a: ID!
				b: ID!
			}
			type Account {
				id: ID!
				info: Info
				name: String!
			}
			type Query {
				account(id: ID!, a: ID!, b: ID!): Account
			}
		`
		sdlNested := `
			type Query {
				account(id: ID!, a: ID!, b: ID!): Account
			}
			type Account @key(fields: "id info {a b}") {
				id: ID!
				info: Info
				name: String!
			}
			type Info {
				a: ID!
				b: ID!
			}
		`
		keysNested := plan.FederationFieldConfigurations{
			{TypeName: "Account", SelectionSet: "id info {a b}"},
		}

		rootFieldCaching := plan.RootFieldCacheConfigurations{
			{
				TypeName:  "Query",
				FieldName: "account",
				CacheName: "default",
				TTL:       30 * time.Second,
				EntityKeyMappings: []plan.EntityKeyMapping{
					{
						EntityTypeName: "Account",
						FieldMappings: []plan.FieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
							{EntityKeyField: "a", ArgumentPath: []string{"a"}},
							{EntityKeyField: "b", ArgumentPath: []string{"b"}},
						},
					},
				},
			},
		}

		ds := mustDataSourceConfiguration(t,
			"accounts",
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"account"}},
					{TypeName: "Account", FieldNames: []string{"id", "info", "name"}},
				},
				ChildNodes: []plan.TypeField{
					{TypeName: "Info", FieldNames: []string{"a", "b"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys:             keysNested,
					RootFieldCaching: rootFieldCaching,
				},
			},
			mustCustomConfiguration(t,
				ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://accounts.service"},
					SchemaConfiguration: mustSchema(t,
						&FederationConfiguration{Enabled: true, ServiceSDL: sdlNested},
						sdlNested,
					),
				},
			),
		)

		config := plan.Configuration{
			DataSources:                     []plan.DataSource{ds},
			DisableIncludeInfo:              false,
			DisableIncludeFieldDependencies: false,
			DisableEntityCaching:            false,
			DisableFetchProvidesData:        false,
			Fields: plan.FieldConfigurations{
				{TypeName: "Query", FieldName: "account", Arguments: plan.ArgumentsConfigurations{
					{Name: "id", SourceType: plan.FieldArgumentSource, SourcePath: []string{"id"}},
					{Name: "a", SourceType: plan.FieldArgumentSource, SourcePath: []string{"a"}},
					{Name: "b", SourceType: plan.FieldArgumentSource, SourcePath: []string{"b"}},
				}},
			},
		}

		cacheConfigs := planAndExtractCacheConfig(t, definitionNested, `query Q($id: ID!, $a: ID!, $b: ID!) { account(id: $id, a: $a, b: $b) { id name } }`, "Q", config)

		require.Equal(t, 1, len(cacheConfigs))
		assert.Equal(t, resolve.FetchCacheConfiguration{
			Enabled:   true,
			CacheName: "default",
			TTL:       30 * time.Second,
			CacheKeyTemplate: newExpectedRootQueryCacheKeyTemplate([]resolve.QueryField{
				{
					Coordinate:  resolve.GraphCoordinate{TypeName: "Query", FieldName: "account"},
					ResponseKey: "account",
					Args: []resolve.FieldArgument{
						{Name: "id", Variable: &resolve.ContextVariable{Path: []string{"id"}, Renderer: resolve.NewJSONVariableRenderer()}},
						{Name: "a", Variable: &resolve.ContextVariable{Path: []string{"a"}, Renderer: resolve.NewJSONVariableRenderer()}},
						{Name: "b", Variable: &resolve.ContextVariable{Path: []string{"b"}, Renderer: resolve.NewJSONVariableRenderer()}},
					},
				},
			}, []resolve.EntityKeyMappingConfig{
				{
					EntityTypeName: "Account",
					FieldMappings: []resolve.EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						{EntityKeyField: "a", ArgumentPath: []string{"a"}},
						{EntityKeyField: "b", ArgumentPath: []string{"b"}},
					},
				},
			}),
			RootFieldL1EntityCacheKeyTemplates: map[string]resolve.CacheKeyTemplate{
				"account:Account": &resolve.EntityQueryCacheKeyTemplate{
					TypeName: "Account",
					Keys: resolve.NewResolvableObjectVariable(&resolve.Object{
						Nullable: true,
						Path:     []string{"account"},
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("Account")},
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: [][]byte{[]byte("Account")},
							},
							{
								Name: []byte("info"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"info"},
									Fields: []*resolve.Field{
										{
											Name:  []byte("a"),
											Value: &resolve.Scalar{Path: []string{"a"}},
										},
										{
											Name:  []byte("b"),
											Value: &resolve.Scalar{Path: []string{"b"}},
										},
									},
								},
								OnTypeNames: [][]byte{[]byte("Account")},
							},
						},
					}),
				},
			},
		}, cacheConfigs[0])
	})
}
